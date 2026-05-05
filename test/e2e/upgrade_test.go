//go:build E2Etests

package e2e

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("Upgrade: Control Plane", labels.Critical, labels.Positive, labels.Slow, labels.HCP, labels.Upgrade, func() {
	It("should upgrade the control plane to target version", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}
		if cfg.UpgradeTargetVersion == "" {
			Skip("UPGRADE_TARGET_VERSION not set, skipping upgrade test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initiating control plane upgrade to " + cfg.UpgradeTargetVersion)
		Expect(framework.InitiateControlPlaneUpgrade(tc.Connection(), cfg.ClusterID, cfg.UpgradeTargetVersion)).To(Succeed())

		By("Waiting for upgrade to complete (up to 60 minutes)")
		Expect(framework.WaitForUpgradeComplete(tc.Connection(), cfg.ClusterID, cfg.UpgradeTargetVersion, 60*time.Minute)).To(Succeed())

		By("Verifying cluster is ready after upgrade")
		Expect(verifiers.VerifyClusterReady(tc.Connection(), cfg.ClusterID)).To(Succeed())
	})
})

var _ = Describe("Upgrade: NodePool", labels.Critical, labels.Positive, labels.Slow, labels.HCP, labels.Upgrade, func() {
	It("should upgrade node pools to target version", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}
		if cfg.UpgradeTargetVersion == "" {
			Skip("UPGRADE_TARGET_VERSION not set, skipping nodepool upgrade test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Listing node pools")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).
			NodePools().List().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))

		items := resp.Items().Slice()
		Expect(items).NotTo(BeEmpty(), "no node pools found")

		nodePoolID := items[0].ID()
		GinkgoWriter.Printf("Upgrading node pool %s to %s\n", nodePoolID, cfg.UpgradeTargetVersion)

		By("Initiating node pool upgrade")
		Expect(framework.InitiateNodePoolUpgrade(tc.Connection(), cfg.ClusterID, nodePoolID, cfg.UpgradeTargetVersion)).To(Succeed())

		By("Waiting for node pool upgrade (up to 60 minutes)")
		// Poll node pool version until it matches
		Eventually(func(g Gomega) {
			npResp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).
				NodePools().NodePool(nodePoolID).Get().SendContext(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(npResp.Body().Version().RawID()).To(Equal(cfg.UpgradeTargetVersion))
		}).WithContext(ctx).WithTimeout(60 * time.Minute).WithPolling(30 * time.Second).Should(Succeed())

		By("Verifying all nodes are ready after upgrade")
		Expect(tc.InitHCClients()).To(Succeed())
		Expect(verifiers.RunVerifiers(ctx, tc.HCKubeClient(),
			verifiers.VerifyAllNodesReady(),
		)).To(Succeed())
	})
})

var _ = Describe("Upgrade: Classic Cluster", labels.Critical, labels.Positive, labels.Slow, labels.Classic, labels.Upgrade, func() {
	It("should upgrade a Classic cluster to target version", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}
		if cfg.UpgradeTargetVersion == "" {
			Skip("UPGRADE_TARGET_VERSION not set, skipping upgrade test")
		}

		tc := framework.NewTestContext(cfg, conn)
		if !tc.IsClassic() {
			Skip("Not a Classic cluster, skipping Classic upgrade test")
		}

		By("Initiating cluster upgrade to " + cfg.UpgradeTargetVersion)
		Expect(framework.InitiateClusterUpgrade(tc.Connection(), cfg.ClusterID, cfg.UpgradeTargetVersion)).To(Succeed())

		By("Waiting for upgrade to complete (up to 90 minutes)")
		Expect(framework.WaitForUpgradeComplete(tc.Connection(), cfg.ClusterID, cfg.UpgradeTargetVersion, 90*time.Minute)).To(Succeed())

		By("Verifying cluster is ready after upgrade")
		Expect(verifiers.VerifyClusterReady(tc.Connection(), cfg.ClusterID)).To(Succeed())
	})
})

var _ = Describe("Upgrade: Post-Upgrade Verification", labels.High, labels.Positive, labels.HCP, labels.Classic, labels.Upgrade, func() {
	It("should have all ClusterOperators healthy after upgrade", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}
		if cfg.UpgradeTargetVersion == "" {
			Skip("UPGRADE_TARGET_VERSION not set, skipping post-upgrade check")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		By("Verifying ClusterOperators are healthy after upgrade")
		Expect(verifiers.VerifyClusterOperatorsHealthy(ctx, tc.HCDynamicClient())).To(Succeed())
	})

	It("should have workloads accessible after upgrade", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}
		if cfg.UpgradeTargetVersion == "" {
			Skip("UPGRADE_TARGET_VERSION not set, skipping post-upgrade workload check")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		By("Deploying test workload after upgrade")
		cleanup, err := framework.DeployTestWorkload(ctx, tc.HCKubeClient(), "e2e-post-upgrade")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanup)

		By("Verifying workload is available")
		Expect(verifiers.VerifyDeploymentAvailable(ctx, tc.HCKubeClient(), "e2e-post-upgrade", "test-nginx")).To(Succeed())
	})
})
