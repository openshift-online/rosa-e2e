package osdgcp

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

func VerifyCluster(ctx context.Context, tc *framework.TestContext, clusterID string) {
	// Verify this is actually a GCP cluster
	if !tc.IsOSDGCP() {
		Skip("CLUSTER_ID is not an OSD-GCP cluster, skipping")
	}

	By("Verifying cluster is ready in OCM")
	Expect(verifiers.VerifyClusterReady(tc.Connection(), clusterID)).To(Succeed())

	By("Verifying cluster health via Kubernetes API")
	kubeConfig, err := framework.GetClusterCredentials(tc.Connection(), clusterID)
	Expect(err).NotTo(HaveOccurred())

	kubeClient, err := framework.NewKubeClient(kubeConfig)
	Expect(err).NotTo(HaveOccurred())

	// For existing clusters, just verify all nodes are ready (don't assert count
	// since autoscaler may have changed it)
	Expect(verifiers.RunVerifiers(ctx, kubeClient,
		verifiers.VerifyAllNodesReady(),
	)).To(Succeed())
}

func DeferDeleteCluster(tc *framework.TestContext, clusterID string) func() {
	return func() {
		if tc.Config().PreserveClusters {
			GinkgoWriter.Printf("PRESERVE_CLUSTERS=true: Skipping deletion of cluster %s\n", clusterID)
			GinkgoWriter.Printf("To delete manually: ocm delete cluster %s\n", clusterID)
			return
		}
		By("Cleaning up: deleting cluster")
		err := framework.DeleteCluster(tc.Connection(), clusterID)
		if err != nil {
			GinkgoWriter.Printf("Warning: failed to delete cluster %s during cleanup: %v\n", clusterID, err)
		}
	}
}

func DeleteCluster(tc *framework.TestContext, clusterID string) {
	if tc.Config().PreserveClusters {
		GinkgoWriter.Printf("PRESERVE_CLUSTERS=true: Cluster %s will not be deleted\n", clusterID)
		return
	}
	By("Deleting the cluster")
	Expect(framework.DeleteCluster(tc.Connection(), clusterID)).To(Succeed())

	By("Verifying cluster is uninstalling")
	Expect(verifiers.VerifyClusterDeleting(tc.Connection(), clusterID)).To(Succeed())
}
