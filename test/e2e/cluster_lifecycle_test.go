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

var _ = Describe("ROSA HCP Cluster Lifecycle: Full", labels.Critical, labels.Positive, labels.Slow, labels.HCP, labels.ClusterLifecycle, func() {
	It("should create, verify, and delete a ROSA HCP cluster", func(ctx context.Context) {
		if cfg.ClusterID != "" {
			Skip("CLUSTER_ID is set, skipping full lifecycle test (use existing cluster tests instead)")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Creating a ROSA HCP cluster")
		clusterID, err := framework.CreateRosaHCPCluster(tc.Connection(), tc.Config())
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Printf("Created cluster: %s\n", clusterID)

		DeferCleanup(func() {
			By("Cleaning up: deleting cluster")
			err := framework.DeleteCluster(tc.Connection(), clusterID)
			if err != nil {
				GinkgoWriter.Printf("Warning: failed to delete cluster %s during cleanup: %v\n", clusterID, err)
			}
		})

		By("Waiting for cluster to be ready (up to 45 minutes)")
		Expect(framework.WaitForClusterReady(tc.Connection(), clusterID, 45*time.Minute)).To(Succeed())

		By("Verifying cluster is ready in OCM")
		Expect(verifiers.VerifyClusterReady(tc.Connection(), clusterID)).To(Succeed())

		By("Verifying cluster health via Kubernetes API")
		kubeConfig, err := framework.GetClusterCredentials(tc.Connection(), clusterID)
		Expect(err).NotTo(HaveOccurred())

		kubeClient, err := framework.NewKubeClient(kubeConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(verifiers.RunVerifiers(ctx, kubeClient,
			verifiers.VerifyAllNodesReady(),
			verifiers.VerifyNodeCount(tc.Config().ComputeNodes),
		)).To(Succeed())

		By("Deleting the cluster")
		Expect(framework.DeleteCluster(tc.Connection(), clusterID)).To(Succeed())

		By("Verifying cluster is uninstalling")
		Expect(verifiers.VerifyClusterDeleting(tc.Connection(), clusterID)).To(Succeed())
	})
})

var _ = Describe("ROSA HCP Cluster Lifecycle: Existing Cluster", labels.Critical, labels.Positive, labels.HCP, labels.ClusterLifecycle, func() {
	It("should verify an existing cluster is healthy", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set, skipping existing cluster verification")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Verifying cluster is ready in OCM")
		Expect(verifiers.VerifyClusterReady(tc.Connection(), cfg.ClusterID)).To(Succeed())

		By("Getting expected node count from node pools")
		npResp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).
			NodePools().List().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		expectedNodes := 0
		for _, np := range npResp.Items().Slice() {
			expectedNodes += np.Replicas()
		}
		GinkgoWriter.Printf("Cluster has %d total worker nodes across %d node pools\n", expectedNodes, npResp.Total())

		By("Verifying cluster health via Kubernetes API")
		kubeConfig, err := framework.GetClusterCredentials(tc.Connection(), cfg.ClusterID)
		Expect(err).NotTo(HaveOccurred())

		kubeClient, err := framework.NewKubeClient(kubeConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(verifiers.RunVerifiers(ctx, kubeClient,
			verifiers.VerifyAllNodesReady(),
			verifiers.VerifyNodeCount(expectedNodes),
		)).To(Succeed())
	})
})
