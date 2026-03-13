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

var _ = Describe("ROSA HCP Cluster Lifecycle", labels.Critical, labels.Positive, labels.Slow, labels.HCP, labels.ClusterLifecycle, func() {
	It("should create and delete a ROSA HCP cluster", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		var clusterID string

		if tc.Config().ClusterID != "" {
			// Use existing cluster — skip creation and deletion
			clusterID = tc.Config().ClusterID
			GinkgoWriter.Printf("Using existing cluster: %s\n", clusterID)
		} else {
			By("Creating a ROSA HCP cluster")
			var err error
			clusterID, err = framework.CreateRosaHCPCluster(tc.Connection(), tc.Config())
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Printf("Created cluster: %s\n", clusterID)

			// Register cleanup — runs even if test fails
			DeferCleanup(func() {
				By("Cleaning up: deleting cluster")
				err := framework.DeleteCluster(tc.Connection(), clusterID)
				if err != nil {
					GinkgoWriter.Printf("Warning: failed to delete cluster %s during cleanup: %v\n", clusterID, err)
				}
			})

			By("Waiting for cluster to be ready")
			Expect(framework.WaitForClusterReady(tc.Connection(), clusterID, 45*time.Minute)).To(Succeed())
		}

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

		if tc.Config().ClusterID == "" {
			By("Deleting the cluster")
			Expect(framework.DeleteCluster(tc.Connection(), clusterID)).To(Succeed())

			By("Verifying cluster is uninstalling")
			Expect(verifiers.VerifyClusterDeleting(tc.Connection(), clusterID)).To(Succeed())
		}
	})
})
