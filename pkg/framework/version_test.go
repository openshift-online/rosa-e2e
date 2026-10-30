package framework

import (
	"context"
	"os"
	"testing"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"

	"github.com/openshift-online/rosa-e2e/pkg/config"
)

// Helper function to build a mock cluster for testing
func buildTestCluster(id, version, cloudProvider string, hypershiftEnabled bool) *cmv1.Cluster {
	cluster, _ := cmv1.NewCluster().
		ID(id).
		OpenshiftVersion(version).
		CloudProvider(cmv1.NewCloudProvider().ID(cloudProvider)).
		Hypershift(cmv1.NewHypershift().Enabled(hypershiftEnabled)).
		Build()
	return cluster
}

func TestExtractUniqueVersions(t *testing.T) {
	// Test with sample clusters: versions should be sorted and deduplicated
	clusters := []*cmv1.Cluster{
		buildTestCluster("cluster-a", "4.13.5", "aws", true),
		buildTestCluster("cluster-b", "4.13.1", "aws", true),
		buildTestCluster("cluster-c", "4.14.2", "aws", true),
		buildTestCluster("cluster-d", "4.13.5", "aws", true), // duplicate version
	}

	versions := extractUniqueVersions(clusters)

	expected := []string{"4.13.1", "4.13.5", "4.14.2"}
	if len(versions) != len(expected) {
		t.Fatalf("extractUniqueVersions() returned %d versions, want %d", len(versions), len(expected))
	}

	for i := range expected {
		if versions[i] != expected[i] {
			t.Errorf("versions[%d] = %q, want %q", i, versions[i], expected[i])
		}
	}
}

func TestExtractUniqueVersions_EmptyList(t *testing.T) {
	// Empty cluster list should return empty version list
	versions := extractUniqueVersions([]*cmv1.Cluster{})

	if len(versions) != 0 {
		t.Errorf("extractUniqueVersions([]) returned %d versions, want 0", len(versions))
	}
}

func TestExtractUniqueVersions_MalformedVersions(t *testing.T) {
	// Clusters with malformed versions should be skipped
	clusters := []*cmv1.Cluster{
		buildTestCluster("cluster-a", "4.13.5", "aws", true),
		buildTestCluster("cluster-b", "invalid-version", "aws", true),
		buildTestCluster("cluster-c", "", "aws", true), // empty version
		buildTestCluster("cluster-d", "4.14.2", "aws", true),
	}

	versions := extractUniqueVersions(clusters)

	expected := []string{"4.13.5", "4.14.2"}
	if len(versions) != len(expected) {
		t.Fatalf("extractUniqueVersions() returned %d versions, want %d (malformed versions should be skipped)", len(versions), len(expected))
	}

	for i := range expected {
		if versions[i] != expected[i] {
			t.Errorf("versions[%d] = %q, want %q", i, versions[i], expected[i])
		}
	}
}

func TestMatchesTopology(t *testing.T) {
	tests := []struct {
		name              string
		clusterID         string
		version           string
		cloudProvider     string
		hypershiftEnabled bool
		topology          string
		want              bool
	}{
		// HCP topology tests
		{
			name:              "HCP cluster matches hcp topology",
			clusterID:         "hcp-1",
			version:           "4.13.1",
			cloudProvider:     "aws",
			hypershiftEnabled: true,
			topology:          "hcp",
			want:              true,
		},
		{
			name:              "Classic cluster does not match hcp topology",
			clusterID:         "classic-1",
			version:           "4.13.1",
			cloudProvider:     "aws",
			hypershiftEnabled: false,
			topology:          "hcp",
			want:              false,
		},
		{
			name:              "GCP cluster does not match hcp topology",
			clusterID:         "gcp-1",
			version:           "4.13.1",
			cloudProvider:     "gcp",
			hypershiftEnabled: true,
			topology:          "hcp",
			want:              false,
		},
		// Classic topology tests
		{
			name:              "Classic ROSA cluster matches classic topology",
			clusterID:         "classic-1",
			version:           "4.13.1",
			cloudProvider:     "aws",
			hypershiftEnabled: false,
			topology:          "classic",
			want:              true,
		},
		{
			name:              "HCP cluster does not match classic topology",
			clusterID:         "hcp-1",
			version:           "4.13.1",
			cloudProvider:     "aws",
			hypershiftEnabled: true,
			topology:          "classic",
			want:              false,
		},
		{
			name:              "GCP cluster does not match classic topology",
			clusterID:         "gcp-1",
			version:           "4.13.1",
			cloudProvider:     "gcp",
			hypershiftEnabled: false,
			topology:          "classic",
			want:              false,
		},
		// OSD-GCP topology tests
		{
			name:              "GCP cluster matches osd-gcp topology",
			clusterID:         "gcp-1",
			version:           "4.13.1",
			cloudProvider:     "gcp",
			hypershiftEnabled: false,
			topology:          "osd-gcp",
			want:              true,
		},
		{
			name:              "AWS cluster does not match osd-gcp topology",
			clusterID:         "aws-1",
			version:           "4.13.1",
			cloudProvider:     "aws",
			hypershiftEnabled: false,
			topology:          "osd-gcp",
			want:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := buildTestCluster(tt.clusterID, tt.version, tt.cloudProvider, tt.hypershiftEnabled)
			got := matchesTopology(cluster, tt.topology)
			if got != tt.want {
				t.Errorf("matchesTopology() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListClustersInSector_InvalidSectorName(t *testing.T) {
	// Test that invalid sector names are rejected
	ctx := context.Background()

	// Create a minimal mock connection (won't be used due to early validation)
	conn, _ := sdk.NewConnectionBuilder().
		URL("https://api.openshift.com").
		TokenURL("https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token").
		Client("cloud-services", "").
		Tokens("dummy-token-for-build").
		Build()
	if conn != nil {
		defer conn.Close()
	}

	invalidNames := []string{
		"prod'; DROP TABLE clusters;--", // SQL injection attempt
		"prod\ncanary",                   // newline
		"prod|canary",                    // pipe
		"prod&canary",                    // ampersand
	}

	for _, name := range invalidNames {
		t.Run("invalid_"+name, func(t *testing.T) {
			_, err := ListClustersInSector(ctx, conn, name, "hcp")
			if err == nil {
				t.Errorf("ListClustersInSector() with invalid sector name %q should return error", name)
			}
		})
	}
}

func TestListClustersInSector_InvalidTopology(t *testing.T) {
	// Test that invalid topologies are rejected
	ctx := context.Background()

	// Create a minimal mock connection (won't be used due to early validation)
	conn, _ := sdk.NewConnectionBuilder().
		URL("https://api.openshift.com").
		TokenURL("https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token").
		Client("cloud-services", "").
		Tokens("dummy-token-for-build").
		Build()
	if conn != nil {
		defer conn.Close()
	}

	invalidTopologies := []string{
		"invalid",
		"rosa",
		"",
		"HCP", // case-sensitive test (should work after normalization, but testing the original)
	}

	for _, topology := range invalidTopologies {
		if topology == "HCP" {
			continue // This should actually work after case normalization
		}
		t.Run("invalid_"+topology, func(t *testing.T) {
			_, err := ListClustersInSector(ctx, conn, "prod:canary", topology)
			if err == nil {
				t.Errorf("ListClustersInSector() with invalid topology %q should return error", topology)
			}
		})
	}
}

func TestGetMinimumOCPVersions_Integration(t *testing.T) {
	// Integration test using real OCM connection; explicitly gated on opt-in
	if os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("RUN_INTEGRATION not set; skipping live OCM integration test")
	}
	token := os.Getenv("OCM_TOKEN")
	if token == "" {
		t.Skip("OCM_TOKEN not set, skipping integration test")
	}
	ocmEnv := os.Getenv("OCM_ENV")
	if ocmEnv == "" {
		ocmEnv = "production"
	}

	// Create real OCM connection using the configured environment
	cfg := &config.Config{OCMEnv: ocmEnv, OCMToken: token}
	conn, err := NewOCMConnection(cfg)
	if err != nil {
		t.Fatalf("building OCM connection: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// Query prod:canary sector for HCP topology
	versions, err := GetMinimumOCPVersions(ctx, conn, "prod:canary", "hcp")
	if err != nil {
		t.Fatalf("GetMinimumOCPVersions() returned error: %v", err)
	}

	// Verify at least some versions exist (sector should have clusters)
	if len(versions) == 0 {
		t.Log("WARNING: No HCP clusters found in prod:canary sector")
	} else {
		t.Logf("Found %d unique OCP versions in prod:canary HCP: %v", len(versions), versions)

		// Verify versions are non-empty strings
		for i, v := range versions {
			if v == "" {
				t.Errorf("versions[%d] is empty string", i)
			}
		}

		// Verify versions are sorted using semantic comparison (not lexicographic)
		for i := 1; i < len(versions); i++ {
			if compareVersions(versions[i], versions[i-1]) < 0 {
				t.Errorf("versions not sorted: versions[%d]=%q > versions[%d]=%q",
					i-1, versions[i-1], i, versions[i])
			}
		}
	}
}

func TestListClustersInSector_Integration(t *testing.T) {
	// Integration test using real OCM connection; explicitly gated on opt-in
	if os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("RUN_INTEGRATION not set; skipping live OCM integration test")
	}
	token := os.Getenv("OCM_TOKEN")
	if token == "" {
		t.Skip("OCM_TOKEN not set, skipping integration test")
	}
	ocmEnv := os.Getenv("OCM_ENV")
	if ocmEnv == "" {
		ocmEnv = "production"
	}

	// Create real OCM connection using configured environment
	cfg := &config.Config{OCMEnv: ocmEnv, OCMToken: token}
	conn, err := NewOCMConnection(cfg)
	if err != nil {
		t.Fatalf("building OCM connection: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// Query prod:canary sector for HCP topology
	clusters, err := ListClustersInSector(ctx, conn, "prod:canary", "hcp")
	if err != nil {
		t.Fatalf("ListClustersInSector() returned error: %v", err)
	}

	if len(clusters) == 0 {
		t.Log("WARNING: No HCP clusters found in prod:canary sector")
	} else {
		t.Logf("Found %d HCP clusters in prod:canary", len(clusters))

		// Verify all clusters match HCP topology (Hypershift enabled + AWS)
		for i, cluster := range clusters {
			if !cluster.Hypershift().Enabled() {
				t.Errorf("cluster[%d] has Hypershift disabled, want enabled for HCP", i)
			}
			if cluster.CloudProvider().ID() != "aws" {
				t.Errorf("cluster[%d] has cloud provider %s, want aws for HCP",
					i, cluster.CloudProvider().ID())
			}
		}
	}
}
