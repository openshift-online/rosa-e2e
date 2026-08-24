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

var _ = Describe("Data Plane: Storage", labels.High, labels.Positive, labels.HCP, labels.Classic, labels.OSDGCP, labels.DataPlane, func() {
	It("should create a PVC and verify it is bound", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set, skipping storage test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		namespace := "e2e-storage-test"

		By("Creating test namespace")
		cleanup, err := framework.CreateTestNamespace(ctx, tc.HCKubeClient(), namespace)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanup)

		storageClass := "gp3-csi"
		if tc.IsOSDGCP() {
			storageClass = ""
		}

		By("Creating a PVC")
		pvcName, err := framework.CreateTestPVC(ctx, tc.HCKubeClient(), namespace, storageClass)
		Expect(err).NotTo(HaveOccurred())

		By("Creating a pod that mounts the PVC")
		_, err = framework.CreateTestPodWithPVC(ctx, tc.HCKubeClient(), namespace, pvcName)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying PVC is bound")
		Eventually(func(g Gomega) {
			g.Expect(verifiers.VerifyPVCBound(ctx, tc.HCKubeClient(), namespace, pvcName)).To(Succeed())
		}).WithContext(ctx).Should(Succeed())
	})
})
