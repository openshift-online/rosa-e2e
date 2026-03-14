//go:build E2Etests

package e2e

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
)

var _ = Describe("Data Plane: Networking", labels.High, labels.Positive, labels.HCP, labels.DataPlane, func() {
	It("should resolve cluster DNS", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set, skipping data plane test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		namespace := "e2e-dns-test"

		By("Creating test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		_, err := tc.HCKubeClient().CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			tc.HCKubeClient().CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
		})

		By("Creating DNS test pod")
		podName := "dns-test-pod"
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: namespace,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:    "dns-test",
						Image:   "registry.access.redhat.com/ubi9/ubi-minimal:latest",
						Command: []string{"nslookup", "kubernetes.default.svc.cluster.local"},
					},
				},
				RestartPolicy: corev1.RestartPolicyNever,
			},
		}
		_, err = tc.HCKubeClient().CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for DNS test pod to complete")
		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			p, err := tc.HCKubeClient().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed, nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Verifying DNS test pod succeeded")
		finalPod, err := tc.HCKubeClient().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(finalPod.Status.Phase).To(Equal(corev1.PodSucceeded))
	})

	It("should have pod-to-service connectivity", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set, skipping data plane test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		namespace := "e2e-network-test"

		By("Deploying test workload")
		cleanup, err := framework.DeployTestWorkload(ctx, tc.HCKubeClient(), namespace)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanup)

		By("Creating curl test pod")
		podName := "curl-test-pod"
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: namespace,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:    "curl-test",
						Image:   "registry.access.redhat.com/ubi9/ubi-minimal:latest",
						Command: []string{"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "http://test-nginx-svc:80"},
					},
				},
				RestartPolicy: corev1.RestartPolicyNever,
			},
		}
		_, err = tc.HCKubeClient().CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for curl test pod to complete")
		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
			p, err := tc.HCKubeClient().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed, nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Verifying pod-to-service connectivity")
		finalPod, err := tc.HCKubeClient().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(finalPod.Status.Phase).To(Equal(corev1.PodSucceeded))

		// Get pod logs to verify HTTP 200 response
		logs, err := tc.HCKubeClient().CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{}).DoRaw(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(string(logs))).To(Equal("200"))
	})
})
