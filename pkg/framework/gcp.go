package framework

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"

	"github.com/openshift-online/rosa-e2e/pkg/config"
)

// CreateOSDGCPCluster creates an OSD cluster on GCP via the OCM API and returns its ID.
func CreateOSDGCPCluster(conn *sdk.Connection, cfg *config.Config) (string, error) {
	// Validate required GCP configuration
	if cfg.GCPWifConfig == "" {
		return "", fmt.Errorf("GCP_WIF_CONFIG environment variable or gcp_wif_config in config file is required for OSD GCP cluster creation")
	}
	if cfg.GCPRegion == "" {
		return "", fmt.Errorf("GCP_REGION environment variable or gcp_region in config file is required for OSD GCP cluster creation")
	}

	name := generateClusterName(cfg.ClusterNamePrefix, "osd-gcp")

	versionID, err := resolveVersionForTopology(conn, cfg, "osd-gcp")
	if err != nil {
		return "", fmt.Errorf("resolving version: %w", err)
	}

	// Build GCP configuration with WIF authentication
	gcpBuilder := cmv1.NewGCP().Authentication(
		cmv1.NewGcpAuthentication().
			Kind(cmv1.WifConfigKind).
			Id(cfg.GCPWifConfig),
	)

	// Build cluster
	clusterBuilder := cmv1.NewCluster().
		Name(name).
		Product(cmv1.NewProduct().ID("osd")).
		CloudProvider(cmv1.NewCloudProvider().ID("gcp")).
		Region(cmv1.NewCloudRegion().ID(cfg.GCPRegion)).
		MultiAZ(true). // GCP clusters are typically multi-AZ
		CCS(cmv1.NewCCS().Enabled(true)).
		GCP(gcpBuilder).
		Nodes(cmv1.NewClusterNodes().
			ComputeMachineType(cmv1.NewMachineType().ID(cfg.ComputeMachineType)).
			Compute(cfg.ComputeNodes)).
		Version(cmv1.NewVersion().
			ID(versionID).
			ChannelGroup(cfg.ChannelGroup))

	cluster, err := clusterBuilder.Build()
	if err != nil {
		return "", fmt.Errorf("building cluster object: %w", err)
	}

	resp, err := conn.ClustersMgmt().V1().Clusters().Add().Body(cluster).Send()
	if err != nil {
		return "", fmt.Errorf("creating GCP cluster: %w", err)
	}

	clusterID := resp.Body().ID()
	ginkgo.GinkgoWriter.Printf("OSD GCP cluster %q created with ID %s\n", name, clusterID)
	return clusterID, nil
}
