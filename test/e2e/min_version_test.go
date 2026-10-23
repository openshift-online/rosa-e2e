//go:build E2Etests

package e2e

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("Minimum OCP Version Coverage", labels.Critical, labels.Positive, labels.MinVersion, func() {
	BeforeEach(func() {
		// Only run min-version tests in production OCM environment
		if cfg.OCMEnv != "production" && cfg.OCMEnv != "prod" {
			Skip("Minimum version tests only run against prod OCM environment")
		}
		// Verify endpoint has not been overridden to point to a different service
		if os.Getenv("OCM_BASE_URL") != "" {
			Skip("OCM_BASE_URL override detected; min-version tests must use the configured OCM environment endpoint")
		}

		// Only run if CLUSTER_ID is NOT set (we provision test clusters)
		if cfg.ClusterID != "" {
			Skip("Minimum version tests create temporary clusters; CLUSTER_ID must not be set")
		}

		// Validate required provisioning inputs
		if cfg.AWSAccountID == "" {
			Skip("AWS_ACCOUNT_ID not set; required for cluster provisioning")
		}
		if cfg.AWSRegion == "" {
			Skip("AWS_REGION not set; required for cluster provisioning")
		}
		if cfg.OIDCConfigID == "" {
			Skip("OIDC_CONFIG_ID not set; required for HCP provisioning")
		}
		if len(cfg.SubnetIDs) == 0 {
			Skip("SUBNET_IDS not set; required for cluster provisioning")
		}
		if cfg.AccountRolePrefix == "" {
			Skip("ACCOUNT_ROLE_PREFIX not set; required for STS provisioning")
		}
		if cfg.OperatorRolePrefix == "" {
			Skip("OPERATOR_ROLE_PREFIX not set; required for STS provisioning")
		}
		if cfg.CreatorARN == "" {
			Skip("CREATOR_ARN not set; required for cluster creation tracking")
		}

		// Verify ProvisionShardID is not set (would bypass sector discovery)
		if cfg.ProvisionShardID != "" || os.Getenv("PROVISION_SHARD_ID") != "" {
			Skip("ProvisionShardID is set; min-version tests must discover shards from sector_name, not use a fixed shard")
		}
	})

	It("provisions and verifies HCP cluster at minimum OCP version from prod:canary", labels.HCP, labels.Slow, SpecTimeout(2*time.Hour), func(ctx context.Context) {
		By("Discovering minimum OCP versions for HCP topology")
		versions, err := framework.GetMinimumOCPVersions(ctx, conn, "prod:canary", "hcp")
		Expect(err).NotTo(HaveOccurred())
		Expect(versions).NotTo(BeEmpty(), "No minimum OCP versions found in prod:canary for HCP")

		// Select the oldest installable version (running versions may be disabled for new installs)
		var minVersion string
		for _, v := range versions {
			// Validate this version is actually installable
			if err := framework.ValidateVersionAvailable(conn, cfg, "hcp", v); err == nil {
				minVersion = v
				break
			}
		}
		Expect(minVersion).NotTo(BeEmpty(), "No installable OCP versions found in prod:canary for HCP")
		GinkgoWriter.Printf("Testing HCP cluster at minimum installable version: %s\n", minVersion)

		// Shallow copy config to isolate mutations made by CreateRosaHCPCluster (specifically ProvisionShardID).
		// This prevents the mutation from affecting the global cfg, while allowing CreateRosaHCPCluster
		// to mutate localCfg.ProvisionShardID. SafeMultiAZ and other scalar fields are not affected by
		// the shallow copy. If slice/map fields are added to Config, this would need a deep copy helper.
		localCfg := *cfg
		localCfg.OpenShiftVersion = minVersion
		localCfg.SectorName = "prod:canary" // Pin cluster to prod:canary fleet (discovered from this sector)

		tc := framework.NewTestContext(&localCfg, conn)

		// Register cleanup BEFORE provisioning so even partial creation failures clean up
		var clusterID string
		DeferCleanup(func() {
			if clusterID == "" {
				return // Cluster was never created
			}
			By("Cleaning up: deleting test cluster")
			err := framework.DeleteCluster(tc.Connection(), clusterID)
			Expect(err).NotTo(HaveOccurred(), "failed to delete test cluster %s", clusterID)

			By("Waiting for cluster to be deleted")
			Expect(framework.WaitForClusterDeleted(tc.Connection(), clusterID, 15*time.Minute)).To(Succeed())
		})

		By("Creating temporary HCP test cluster")
		var err2 error
		clusterID, err2 = framework.CreateRosaHCPCluster(tc.Connection(), tc.Config())
		Expect(err2).NotTo(HaveOccurred())
		GinkgoWriter.Printf("Created test cluster: %s\n", clusterID)

		By("Waiting for cluster to be ready (up to 45 minutes)")
		Expect(framework.WaitForClusterReady(tc.Connection(), clusterID, 45*time.Minute)).To(Succeed())

		By("Verifying cluster is ready in OCM")
		Expect(verifiers.VerifyClusterReady(tc.Connection(), clusterID)).To(Succeed())

		By("Obtaining cluster credentials")
		kubeConfig, err := framework.GetClusterCredentials(tc.Connection(), clusterID)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying worker nodes are Ready")
		kubeClient, err := framework.NewKubeClient(kubeConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(verifiers.RunVerifiers(ctx, kubeClient,
			verifiers.VerifyAllNodesReady(),
		)).To(Succeed())

		By("Verifying cluster operators are Ready")
		dynClient, err := framework.NewDynamicClient(kubeConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(verifiers.VerifyClusterOperatorsReady(ctx, dynClient)).To(Succeed())
	})

})
