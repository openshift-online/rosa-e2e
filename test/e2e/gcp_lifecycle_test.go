//go:build E2Etests

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/osdgcp"
)

var _ = Describe("OSD GCP Cluster Lifecycle: Full", labels.Critical, labels.Positive, labels.Slow, labels.OSDGCP, labels.ClusterLifecycle, func() {
	It("should create, verify, and delete an OSD GCP cluster", func(ctx context.Context) {
		if cfg.ClusterID != "" {
			Skip("CLUSTER_ID is set, skipping full lifecycle test (use existing cluster tests instead)")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Creating an OSD GCP cluster")
		clusterID, err := framework.CreateOSDGCPCluster(tc.Connection(), tc.Config())
		Expect(err).NotTo(HaveOccurred())
		GinkgoWriter.Printf("Created cluster: %s\n", clusterID)

		DeferCleanup(osdgcp.DeferDeleteCluster(tc, clusterID))

		By("Waiting for cluster to be ready (up to 65 minutes)")
		Expect(framework.WaitForClusterReady(tc.Connection(), clusterID, 65*time.Minute)).To(Succeed())

		osdgcp.VerifyCluster(ctx, tc, clusterID)
		osdgcp.DeleteCluster(tc, clusterID)
	})
})

var _ = Describe("OSD GCP Cluster Lifecycle: Existing Cluster", labels.Critical, labels.Positive, labels.OSDGCP, labels.ClusterLifecycle, func() {
	It("should verify an existing GCP cluster is healthy", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set, skipping existing cluster verification")
		}
		osdgcp.VerifyCluster(ctx, framework.NewTestContext(cfg, conn), cfg.ClusterID)
	})
})
