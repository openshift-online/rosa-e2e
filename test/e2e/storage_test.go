//go:build E2Etests

package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("Data Plane: Storage", labels.High, labels.Positive, labels.HCP, labels.DataPlane, func() {
	It("should create a PVC and verify it is bound", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set, skipping storage test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		namespace := "e2e-storage-test"

		By("Creating test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		_, err := tc.HCKubeClient().CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			tc.HCKubeClient().CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
		})

		By("Creating a PVC with gp3-csi storage class")
		pvcName, err := framework.CreateTestPVC(ctx, tc.HCKubeClient(), namespace, "gp3-csi")
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
