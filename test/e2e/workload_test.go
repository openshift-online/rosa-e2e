//go:build E2Etests

package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("Data Plane: Workload Deployment", labels.High, labels.Positive, labels.HCP, labels.Classic, labels.DataPlane, func() {
	It("should deploy a workload and verify it is available", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set, skipping data plane test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		namespace := "e2e-workload-test"

		By("Deploying test workload")
		cleanup, err := framework.DeployTestWorkload(ctx, tc.HCKubeClient(), namespace)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanup)

		By("Verifying deployment is available")
		Expect(verifiers.VerifyDeploymentAvailable(ctx, tc.HCKubeClient(), namespace, "test-nginx")).To(Succeed())
	})
})
