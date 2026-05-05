package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/onsi/ginkgo/v2"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift-online/rosa-e2e/pkg/config"
)

const (
	tokenURL     = "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token"
	clientID     = "cloud-services"
	clientSecret = ""

	pollInterval       = 30 * time.Second
	defaultReadyTimout = 45 * time.Minute
)

// NewOCMConnection creates a new OCM SDK connection using the provided configuration.
func NewOCMConnection(cfg *config.Config) (*sdk.Connection, error) {
	builder := sdk.NewConnectionBuilder().
		URL(cfg.OCMBaseURL()).
		TokenURL(tokenURL).
		Client(clientID, clientSecret).
		Tokens(cfg.OCMToken)

	conn, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("building OCM connection: %w", err)
	}
	return conn, nil
}

// DetectClusterTopology queries the OCM API and returns "hcp", "classic", or "osd-gcp"
// based on the cluster's product and hypershift configuration.
func DetectClusterTopology(conn *sdk.Connection, clusterID string) (string, error) {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
	if err != nil {
		return "", fmt.Errorf("getting cluster %s for topology detection: %w", clusterID, err)
	}

	cluster := resp.Body()
	cloudProvider := cluster.CloudProvider().ID()

	if cloudProvider == "gcp" {
		return "osd-gcp", nil
	}
	if cluster.Hypershift().Enabled() {
		return "hcp", nil
	}
	return "classic", nil
}

// CreateRosaHCPCluster creates a ROSA HCP cluster via the OCM API and returns its ID.
func CreateRosaHCPCluster(conn *sdk.Connection, cfg *config.Config) (string, error) {
	name := generateClusterName(cfg.ClusterNamePrefix)

	versionID, err := resolveVersionForTopology(conn, cfg, "hcp")
	if err != nil {
		return "", fmt.Errorf("resolving version: %w", err)
	}

	stsBuilder := cmv1.NewSTS().
		RoleARN(fmt.Sprintf("arn:aws:iam::%s:role/%s-Installer-Role", cfg.AWSAccountID, cfg.AccountRolePrefix)).
		SupportRoleARN(fmt.Sprintf("arn:aws:iam::%s:role/%s-Support-Role", cfg.AWSAccountID, cfg.AccountRolePrefix)).
		OperatorRolePrefix(cfg.OperatorRolePrefix).
		OidcConfig(cmv1.NewOidcConfig().ID(cfg.OIDCConfigID))

	awsBuilder := cmv1.NewAWS().
		SubnetIDs(cfg.SubnetIDs...).
		STS(stsBuilder)

	if cfg.BillingAccountID != "" {
		awsBuilder = awsBuilder.BillingAccountID(cfg.BillingAccountID)
	}

	properties := map[string]string{
		"rosa_creator_arn": cfg.CreatorARN,
	}

	// Resolve provision shard from sector if not explicitly set
	if cfg.ProvisionShardID == "" && cfg.SectorName != "" {
		shardID, err := resolveProvisionShard(conn, cfg.SectorName, cfg.AWSRegion)
		if err != nil {
			return "", fmt.Errorf("resolving provision shard for sector %s: %w", cfg.SectorName, err)
		}
		cfg.ProvisionShardID = shardID
	}
	if cfg.ProvisionShardID != "" {
		properties["provision_shard_id"] = cfg.ProvisionShardID
		ginkgo.GinkgoWriter.Printf("Using provision shard: %s\n", cfg.ProvisionShardID)
	}

	clusterBuilder := cmv1.NewCluster().
		Name(name).
		Product(cmv1.NewProduct().ID("rosa")).
		CloudProvider(cmv1.NewCloudProvider().ID("aws")).
		Region(cmv1.NewCloudRegion().ID(cfg.AWSRegion)).
		Hypershift(cmv1.NewHypershift().Enabled(true)).
		MultiAZ(false).
		CCS(cmv1.NewCCS().Enabled(true)).
		AWS(awsBuilder).
		Nodes(cmv1.NewClusterNodes().
			ComputeMachineType(cmv1.NewMachineType().ID(cfg.ComputeMachineType)).
			Compute(cfg.ComputeNodes)).
		Version(cmv1.NewVersion().
			ID(versionID).
			ChannelGroup(cfg.ChannelGroup)).
		Properties(properties)

	cluster, err := clusterBuilder.Build()
	if err != nil {
		return "", fmt.Errorf("building cluster object: %w", err)
	}

	resp, err := conn.ClustersMgmt().V1().Clusters().Add().Body(cluster).Send()
	if err != nil {
		return "", fmt.Errorf("creating cluster: %w", err)
	}

	clusterID := resp.Body().ID()
	ginkgo.GinkgoWriter.Printf("Cluster %q created with ID %s\n", name, clusterID)
	return clusterID, nil
}

// CreateRosaClassicCluster creates a ROSA Classic STS cluster via the OCM API and returns its ID.
func CreateRosaClassicCluster(conn *sdk.Connection, cfg *config.Config) (string, error) {
	name := generateClusterName(cfg.ClusterNamePrefix)

	versionID, err := resolveVersionForTopology(conn, cfg, "classic")
	if err != nil {
		return "", fmt.Errorf("resolving version: %w", err)
	}

	stsBuilder := cmv1.NewSTS().
		RoleARN(fmt.Sprintf("arn:aws:iam::%s:role/%s-Installer-Role", cfg.AWSAccountID, cfg.AccountRolePrefix)).
		SupportRoleARN(fmt.Sprintf("arn:aws:iam::%s:role/%s-Support-Role", cfg.AWSAccountID, cfg.AccountRolePrefix)).
		OperatorRolePrefix(cfg.OperatorRolePrefix)

	awsBuilder := cmv1.NewAWS().
		SubnetIDs(cfg.SubnetIDs...).
		STS(stsBuilder)

	if cfg.BillingAccountID != "" {
		awsBuilder = awsBuilder.BillingAccountID(cfg.BillingAccountID)
	}

	properties := map[string]string{
		"rosa_creator_arn": cfg.CreatorARN,
	}

	clusterBuilder := cmv1.NewCluster().
		Name(name).
		Product(cmv1.NewProduct().ID("rosa")).
		CloudProvider(cmv1.NewCloudProvider().ID("aws")).
		Region(cmv1.NewCloudRegion().ID(cfg.AWSRegion)).
		MultiAZ(false).
		CCS(cmv1.NewCCS().Enabled(true)).
		AWS(awsBuilder).
		Nodes(cmv1.NewClusterNodes().
			ComputeMachineType(cmv1.NewMachineType().ID(cfg.ComputeMachineType)).
			Compute(cfg.ComputeNodes)).
		Version(cmv1.NewVersion().
			ID(versionID).
			ChannelGroup(cfg.ChannelGroup)).
		Properties(properties)

	cluster, err := clusterBuilder.Build()
	if err != nil {
		return "", fmt.Errorf("building cluster object: %w", err)
	}

	resp, err := conn.ClustersMgmt().V1().Clusters().Add().Body(cluster).Send()
	if err != nil {
		return "", fmt.Errorf("creating cluster: %w", err)
	}

	clusterID := resp.Body().ID()
	ginkgo.GinkgoWriter.Printf("Classic STS cluster %q created with ID %s\n", name, clusterID)
	return clusterID, nil
}

// WaitForClusterReady polls the cluster status until it reaches "ready" or the timeout expires.
func WaitForClusterReady(conn *sdk.Connection, clusterID string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = defaultReadyTimout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for cluster %s to be ready after %v", clusterID, timeout)
		case <-ticker.C:
			resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
			if err != nil {
				ginkgo.GinkgoWriter.Printf("Error polling cluster %s status: %v\n", clusterID, err)
				continue
			}
			state := resp.Body().State()
			ginkgo.GinkgoWriter.Printf("Cluster %s state: %s\n", clusterID, state)

			switch state {
			case cmv1.ClusterStateReady:
				return nil
			case cmv1.ClusterStateError:
				return fmt.Errorf("cluster %s entered error state", clusterID)
			case cmv1.ClusterStateUninstalling:
				return fmt.Errorf("cluster %s is uninstalling unexpectedly", clusterID)
			}
		}
	}
}

// DeleteCluster deletes a cluster via the OCM API.
func DeleteCluster(conn *sdk.Connection, clusterID string) error {
	_, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Delete().Send()
	if err != nil {
		return fmt.Errorf("deleting cluster %s: %w", clusterID, err)
	}
	ginkgo.GinkgoWriter.Printf("Cluster %s deletion initiated\n", clusterID)
	return nil
}

// GetClusterCredentials retrieves kubeconfig credentials from the OCM API for the given cluster.
func GetClusterCredentials(conn *sdk.Connection, clusterID string) (*rest.Config, error) {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Credentials().Get().Send()
	if err != nil {
		return nil, fmt.Errorf("getting credentials for cluster %s: %w", clusterID, err)
	}

	kubeconfig := resp.Body().Kubeconfig()
	if kubeconfig == "" {
		return nil, fmt.Errorf("no kubeconfig available for cluster %s", clusterID)
	}

	restConfig, err := clientConfigFromKubeconfig([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig for cluster %s: %w", clusterID, err)
	}

	return restConfig, nil
}

// GetClusterState returns the current state of a cluster.
func GetClusterState(conn *sdk.Connection, clusterID string) (cmv1.ClusterState, error) {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
	if err != nil {
		return "", fmt.Errorf("getting cluster %s state: %w", clusterID, err)
	}
	return resp.Body().State(), nil
}

// resolveVersionForTopology finds the latest available version for the given topology.
func resolveVersionForTopology(conn *sdk.Connection, cfg *config.Config, topology string) (string, error) {
	if cfg.OpenShiftVersion != "" {
		return "openshift-v" + cfg.OpenShiftVersion, nil
	}

	query := fmt.Sprintf("channel_group = '%s' AND rosa_enabled = 'true' AND enabled = 'true'", cfg.ChannelGroup)
	if topology == "hcp" {
		query += " AND hosted_control_plane_enabled = 'true'"
	}

	resp, err := conn.ClustersMgmt().V1().Versions().List().
		Search(query).
		Order("raw_id desc").
		Size(1).
		Send()
	if err != nil {
		return "", fmt.Errorf("listing versions: %w", err)
	}

	items := resp.Items().Slice()
	if len(items) == 0 {
		return "", fmt.Errorf("no %s-compatible versions found for channel group %q", topology, cfg.ChannelGroup)
	}

	version := items[0]
	ginkgo.GinkgoWriter.Printf("Resolved %s version: %s\n", topology, version.ID())
	return version.ID(), nil
}

// resolveProvisionShard finds a ready provision shard for the given sector and region.
// This matches the logic in openshift/release rosa-cluster-provision-commands.sh.
func resolveProvisionShard(conn *sdk.Connection, sector, region string) (string, error) {
	shardID, err := findShardForSector(conn, sector, region, "ready")
	if err != nil || shardID == "" {
		// Try maintenance status
		shardID, err = findShardForSector(conn, sector, region, "maintenance")
		if err != nil || shardID == "" {
			return "", fmt.Errorf("no provision shard found for sector %s in region %s", sector, region)
		}
	}
	return shardID, nil
}

func findShardForSector(conn *sdk.Connection, sector, region, status string) (string, error) {
	type scItem struct {
		ProvisionShardRef struct {
			ID string `json:"id"`
		} `json:"provision_shard_reference"`
	}
	type scResponse struct {
		Items []scItem `json:"items"`
	}

	query := fmt.Sprintf("sector is '%s' and region is '%s' and status in ('%s')", sector, region, status)
	resp, err := conn.Get().
		Path("/api/osd_fleet_mgmt/v1/service_clusters").
		Parameter("search", query).
		Send()
	if err != nil {
		return "", fmt.Errorf("querying service clusters: %w", err)
	}

	body := resp.Bytes()
	var scResp scResponse
	if err := json.Unmarshal(body, &scResp); err != nil {
		return "", fmt.Errorf("parsing OSDFM response: %w", err)
	}

	// Find a dedicated topology provision shard
	for _, sc := range scResp.Items {
		shardID := sc.ProvisionShardRef.ID
		if shardID == "" {
			continue
		}

		shardResp, err := conn.ClustersMgmt().V1().ProvisionShards().ProvisionShard(shardID).Get().Send()
		if err != nil {
			return shardID, nil // Can't check topology, use it anyway
		}

		topology := shardResp.Body().HypershiftConfig().Topology()
		if topology == "dedicated" || topology == "dedicated-v2" {
			ginkgo.GinkgoWriter.Printf("Found dedicated shard %s for sector %s\n", shardID, sector)
			return shardID, nil
		}
	}

	// Fall back to first available
	if len(scResp.Items) > 0 && scResp.Items[0].ProvisionShardRef.ID != "" {
		return scResp.Items[0].ProvisionShardRef.ID, nil
	}

	return "", nil
}

func generateClusterName(prefix string) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 5)
	for i := range suffix {
		suffix[i] = charset[rand.IntN(len(charset))]
	}
	return fmt.Sprintf("%s-%s", prefix, string(suffix))
}

// clientConfigFromKubeconfig parses raw kubeconfig YAML into a rest.Config.
func clientConfigFromKubeconfig(kubeconfig []byte) (*rest.Config, error) {
	clientConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return clientConfig, nil
}
