//go:build E2Etests

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("Data Plane: No-CNI (BYO CNI)", labels.High, labels.Positive, labels.HCP, labels.DataPlane, labels.NoCNI, func() {
	// skipUnlessNoCNI skips the current spec unless a no-CNI cluster is under test.
	skipUnlessNoCNI := func() {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set, skipping no-CNI test")
		}
		if !cfg.NoCNI {
			Skip("NO_CNI not set, skipping no-CNI test")
		}
	}

	It("should report network type Other for a no-CNI HCP cluster", func(ctx context.Context) {
		skipUnlessNoCNI()

		By("Verifying the OCM-reported network type is Other")
		Expect(verifiers.VerifyNetworkType(conn, cfg.ClusterID, verifiers.OtherNetworkType)).To(Succeed())
	})

	It("should have all nodes Ready once the BYO CNI is installed", func(ctx context.Context) {
		skipUnlessNoCNI()

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		By("Verifying all nodes report Ready")
		Expect(verifiers.RunVerifiers(ctx, tc.HCKubeClient(),
			verifiers.VerifyAllNodesReady(),
		)).To(Succeed())
	})

	It("should assign pods a VPC IP from the machine CIDR", func(ctx context.Context) {
		skipUnlessNoCNI()

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		By("Resolving the cluster machine CIDR from OCM")
		machineCIDR, err := verifiers.GetMachineCIDR(conn, cfg.ClusterID)
		Expect(err).NotTo(HaveOccurred())

		namespace := "e2e-nocni-vpcip-test"

		By("Creating test namespace")
		cleanup, err := framework.CreateTestNamespace(ctx, tc.HCKubeClient(), namespace)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanup)

		By("Creating a test pod")
		podName := "vpcip-test-pod"
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: namespace,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:            "vpcip-test",
						Image:           "registry.access.redhat.com/ubi9/ubi-minimal:latest",
						Command:         []string{"sleep", "3600"},
						SecurityContext: restrictedSecurityContext(),
					},
				},
				RestartPolicy: corev1.RestartPolicyNever,
			},
		}
		_, err = tc.HCKubeClient().CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the pod to be Running with an assigned IP")
		var podIP string
		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			p, err := tc.HCKubeClient().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if p.Status.Phase == corev1.PodRunning && p.Status.PodIP != "" {
				podIP = p.Status.PodIP
				return true, nil
			}
			return false, nil
		})
		Expect(err).NotTo(HaveOccurred(), "test pod did not reach Running with an assigned IP")

		By("Verifying the pod IP falls within the machine CIDR (VPC CNI IPAM)")
		Expect(verifiers.VerifyIPInCIDR(podIP, machineCIDR)).To(Succeed())
	})
})
