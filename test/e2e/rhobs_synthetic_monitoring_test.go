//go:build E2Etests

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("RHOBS Synthetic Monitoring", labels.High, labels.Positive, labels.HCP, labels.ManagedService, labels.MCAccess, func() {
	var (
		tc                *framework.TestContext
		rhobsConfig       *verifiers.RHOBSConfig
		clusterExternalID string
	)

	BeforeEach(func(ctx context.Context) {
		tc = framework.NewTestContext(cfg, conn)

		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not configured, skipping RHOBS synthetic monitoring tests")
		}

		// Fetch cluster object to get external ID (used by RMO for probe creation)
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to query cluster details from OCM")

		clusterExternalID = resp.Body().ExternalID()
		Expect(clusterExternalID).NotTo(BeEmpty(), "Cluster has no external ID - cannot validate RHOBS")

		// Try to load RHOBS credentials from management cluster if not already set via env vars
		if !tc.HasRHOBSAccess() {
			// Initialize MC clients if we can
			if err := tc.InitMCClients(); err == nil {
				// Try to load credentials from MC ConfigMap
				if err := tc.LoadRHOBSCredentialsFromMC(ctx); err != nil {
					Skip("RHOBS API credentials not available from environment or MC ConfigMap: " + err.Error())
				}
			} else {
				Skip("RHOBS API credentials not configured and MC not accessible (set RHOBS_PROBE_API_URL, RHOBS_OIDC_CLIENT_ID, RHOBS_OIDC_CLIENT_SECRET, RHOBS_OIDC_ISSUER_URL or ensure MC access)")
			}
		}

		// Create RHOBS config after credentials are loaded
		rhobsConfig = verifiers.NewRHOBSConfig(cfg)
	})

	It("should have probe created for cluster", func(ctx context.Context) {
		By("Querying RHOBS API for probe")
		// Try both private=false and private=true as we don't know endpoint access type yet
		err := verifiers.VerifyRHOBSProbeExists(ctx, clusterExternalID, false, rhobsConfig)
		if err != nil {
			err = verifiers.VerifyRHOBSProbeExists(ctx, clusterExternalID, true, rhobsConfig)
		}
		Expect(err).NotTo(HaveOccurred(), "probe should exist for cluster %s", clusterExternalID)
	})

	It("should have probe with correct cluster-id label", func(ctx context.Context) {
		By("Verifying probe has correct cluster-id label")
		expectedLabels := map[string]string{
			"cluster-id": clusterExternalID,
		}
		Expect(verifiers.VerifyProbeLabels(ctx, clusterExternalID, expectedLabels, rhobsConfig)).To(Succeed())
	})

	It("should have probe_success metrics flowing to RHOBS", func(ctx context.Context) {
		By("Querying RHOBS metrics API for probe_success (up to 5 minutes)")
		Eventually(func() error {
			return verifiers.VerifyProbeSuccessMetrics(ctx, clusterExternalID, rhobsConfig)
		}).WithContext(ctx).WithTimeout(5*time.Minute).WithPolling(30*time.Second).Should(Succeed(),
			"probe_success metrics should exist for cluster %s", clusterExternalID)
	})

	It("should have recording rules evaluating", func(ctx context.Context) {
		By("Waiting for sre:hcp:probe_active and sre:hcp:blackbox_probe_active recording rules (up to 5 minutes)")
		Eventually(func() error {
			return verifiers.VerifyRecordingRules(ctx, clusterExternalID, rhobsConfig)
		}).WithContext(ctx).WithTimeout(5*time.Minute).WithPolling(30*time.Second).Should(Succeed(),
			"recording rules should be evaluating for cluster %s", clusterExternalID)
	})

	// Context-specific tests based on cluster endpoint access type
	// These require knowing the cluster's configuration via OCM API

	Context("for Public cluster", func() {
		It("should have probe with private=false label", func(ctx context.Context) {
			resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
			if err != nil {
				Skip("Unable to query cluster details from OCM")
			}
			cluster := resp.Body()

			if !cluster.Hypershift().Enabled() {
				Skip("Cluster is not HCP, skipping HCP-specific test")
			}

			if cluster.AWS().PrivateLink() {
				Skip("Cluster has PrivateLink enabled, not a Public cluster")
			}

			By("Verifying probe has private=false for Public cluster")
			Expect(verifiers.VerifyRHOBSProbeExists(ctx, clusterExternalID, false, rhobsConfig)).To(Succeed())
		})
	})

	Context("for Private cluster", func() {
		It("should have probe with private=true label", func(ctx context.Context) {
			resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
			if err != nil {
				Skip("Unable to query cluster details from OCM")
			}
			cluster := resp.Body()

			if !cluster.Hypershift().Enabled() {
				Skip("Cluster is not HCP, skipping HCP-specific test")
			}

			if !cluster.AWS().PrivateLink() {
				Skip("Cluster is not private, skipping Private cluster test")
			}

			By("Verifying probe has private=true for Private cluster")
			Expect(verifiers.VerifyRHOBSProbeExists(ctx, clusterExternalID, true, rhobsConfig)).To(Succeed())
		})
	})
})
