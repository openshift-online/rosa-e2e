package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/onsi/ginkgo/v2"
)

var (
	// sectorNameRe validates sector names to prevent injection attacks.
	sectorNameRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)
	// versionRe validates semantic version strings (anchored to reject pre-release suffixes like -rc1).
	versionRe = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
)

// GetMinimumOCPVersions queries the OCM API for all clusters in the specified sector
// and returns a sorted list of unique OCP versions for the given topology.
// Returns an empty slice (not an error) when the sector is not found or has no clusters
// matching the topology.
func GetMinimumOCPVersions(ctx context.Context, conn *sdk.Connection, sectorName, topology string) ([]string, error) {
	clusters, err := ListClustersInSector(ctx, conn, sectorName, topology)
	if err != nil {
		return nil, fmt.Errorf("listing clusters in sector %s: %w", sectorName, err)
	}

	return extractUniqueVersions(clusters), nil
}

// extractUniqueVersions extracts unique OCP versions from a list of clusters,
// validates them, and returns a sorted deduplicated list.
func extractUniqueVersions(clusters []*cmv1.Cluster) []string {
	versionSet := make(map[string]bool)
	skipped := 0
	for _, cluster := range clusters {
		version := cluster.OpenshiftVersion()
		if version == "" {
			skipped++
			continue
		}
		// Validate version format
		if !versionRe.MatchString(version) {
			skipped++
			continue
		}
		versionSet[version] = true
	}
	if skipped > 0 {
		ginkgo.GinkgoWriter.Printf("WARNING: skipped %d clusters due to empty or malformed versions\n", skipped)
	}

	// Convert to sorted slice using semver comparison
	versions := make([]string, 0, len(versionSet))
	for v := range versionSet {
		versions = append(versions, v)
	}

	// Sort using semantic version comparison (e.g., 4.9.0 < 4.14.0)
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) < 0
	})

	return versions
}

// compareVersions compares two semantic versions (e.g., "4.9.0", "4.14.2").
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
// Handles parse errors gracefully by falling back to string comparison.
func compareVersions(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	// Compare each numeric component
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		aNum, errA := strconv.Atoi(aParts[i])
		bNum, errB := strconv.Atoi(bParts[i])

		// If either part fails to parse (shouldn't happen due to regex validation), fall back to string comparison
		if errA != nil || errB != nil {
			if aParts[i] < bParts[i] {
				return -1
			}
			if aParts[i] > bParts[i] {
				return 1
			}
			continue
		}

		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
	}

	// If all compared parts are equal, shorter version is "less"
	if len(aParts) < len(bParts) {
		return -1
	}
	if len(aParts) > len(bParts) {
		return 1
	}
	return 0
}

// ListClustersInSector queries the OCM API for all clusters in the specified sector
// filtered by the given topology (hcp, classic, or osd-gcp).
// Returns an empty slice (not an error) when the sector is not found or has no clusters
// matching the topology.
func ListClustersInSector(ctx context.Context, conn *sdk.Connection, sectorName, topology string) ([]*cmv1.Cluster, error) {
	// Validate sector name to prevent injection attacks
	if !sectorNameRe.MatchString(sectorName) {
		return nil, fmt.Errorf("invalid sector name %q (must match [A-Za-z0-9_.:-]+)", sectorName)
	}

	// Normalize and validate topology
	topology = strings.ToLower(topology)
	validTopologies := map[string]bool{"hcp": true, "classic": true, "osd-gcp": true}
	if !validTopologies[topology] {
		return nil, fmt.Errorf("invalid topology %q (must be hcp, classic, or osd-gcp)", topology)
	}

	// Use OSDFM API to find service clusters in the sector, then get their provision shards.
	// Sector is an OSDFM concept, not a clusters_mgmt attribute.
	type shardRef struct {
		ID string `json:"id"`
	}
	type scItem struct {
		ProvisionShardReference shardRef `json:"provision_shard_reference"`
	}
	type scResponse struct {
		Items []scItem `json:"items"`
	}

	// Query OSDFM service clusters for this sector
	// Escape single quotes in sector name per OCM search rules (defense-in-depth, already validated by regex above)
	escapedSector := strings.ReplaceAll(sectorName, "'", "''")
	searchQuery := fmt.Sprintf("sector is '%s'", escapedSector)
	resp, err := conn.Get().
		Path("/api/osd_fleet_mgmt/v1/service_clusters").
		Parameter("search", searchQuery).
		SendContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying OSDFM for sector %s: %w", sectorName, err)
	}

	// Check HTTP status for errors
	if resp.Status() < 200 || resp.Status() >= 300 {
		// Log full response only in debug mode to avoid leaking service internals
		if os.Getenv("DEBUG_OSDFM") != "" {
			ginkgo.GinkgoWriter.Printf("OSDFM response body (truncated): %.512s\n", resp.String())
		}
		return nil, fmt.Errorf("OSDFM query for sector %s returned status %d", sectorName, resp.Status())
	}

	var scResp scResponse
	if err := json.Unmarshal(resp.Bytes(), &scResp); err != nil {
		return nil, fmt.Errorf("parsing OSDFM response for sector %s: %w", sectorName, err)
	}

	if len(scResp.Items) == 0 {
		// No service clusters in sector — return empty list, not error (genuine empty result)
		ginkgo.GinkgoWriter.Printf("No service clusters found in sector %s\n", sectorName)
		return []*cmv1.Cluster{}, nil
	}

	// Extract provision shard IDs from service clusters
	var provisionShardIDs []string
	for _, sc := range scResp.Items {
		if sc.ProvisionShardReference.ID != "" {
			provisionShardIDs = append(provisionShardIDs, sc.ProvisionShardReference.ID)
		}
	}

	if len(provisionShardIDs) == 0 {
		ginkgo.GinkgoWriter.Printf("No provision shards found in service clusters for sector %s\n", sectorName)
		return []*cmv1.Cluster{}, nil
	}

	// Now query clusters_mgmt for clusters on these provision shards
	var allClusters []*cmv1.Cluster
	var queryErrors []error

	// Compile regex once for validation
	shardIDRegex := regexp.MustCompile(`^[A-Za-z0-9-]+$`)

	for _, shardID := range provisionShardIDs {
		// Validate shard ID from API response before interpolating into query
		if !shardIDRegex.MatchString(shardID) {
			queryErrors = append(queryErrors, fmt.Errorf("malformed provision shard ID %q", shardID))
			continue
		}
		// Escape single quotes for safety
		escapedShardID := strings.ReplaceAll(shardID, "'", "''")

		// Search for clusters on this provision shard with pagination (to avoid missing older versions)
		// Filter on managed=true for defense-in-depth, even though all ROSA/OSD clusters are managed
		page := 1
		pageSize := 500
		for {
			listResp, err := conn.ClustersMgmt().V1().Clusters().List().
				Search(fmt.Sprintf("provision_shard_id = '%s' AND managed = 'true'", escapedShardID)).
				Page(page).
				Size(pageSize).
				SendContext(ctx)
			if err != nil {
				queryErrors = append(queryErrors, fmt.Errorf("querying clusters for provision shard %s page %d: %w", shardID, page, err))
				break
			}

			pageItemCount := 0
			versionCount := make(map[string]int)
			listResp.Items().Each(func(cluster *cmv1.Cluster) bool {
				pageItemCount++
				if matchesTopology(cluster, topology) {
					allClusters = append(allClusters, cluster)
					versionCount[cluster.OpenshiftVersion()]++
				}
				return true
			})
			if len(versionCount) > 0 && page == 1 {
				ginkgo.GinkgoWriter.Printf("Found %s clusters in provision shard %s\n", topology, shardID)
			}

			// Check if there are more pages (zero items means we've exhausted results)
			if pageItemCount == 0 {
				break // No items on this page; we've reached the end
			}
			page++
		}
	}

	// Return accumulated errors if any queries failed
	if len(queryErrors) > 0 {
		// If we got some results, log warnings but don't error (partial success)
		if len(allClusters) > 0 {
			ginkgo.GinkgoWriter.Printf("WARNING: encountered %d errors querying provision shards, but found %d clusters\n",
				len(queryErrors), len(allClusters))
		} else {
			// No results and had errors — return the error
			return nil, fmt.Errorf("failed to query any provision shards: %v", queryErrors[0])
		}
	}

	return allClusters, nil
}

// matchesTopology returns true if the cluster matches the given topology.
func matchesTopology(cluster *cmv1.Cluster, topology string) bool {
	cloudProvider := cluster.CloudProvider().ID()

	switch topology {
	case "hcp":
		// HCP: Hypershift enabled and AWS cloud
		return cluster.Hypershift().Enabled() && cloudProvider == "aws"
	case "classic":
		// Classic ROSA: Hypershift disabled and AWS cloud
		return !cluster.Hypershift().Enabled() && cloudProvider == "aws"
	case "osd-gcp":
		// OSD GCP: GCP cloud provider
		return cloudProvider == "gcp"
	default:
		// Should not reach here due to validation in ListClustersInSector
		return false
	}
}
