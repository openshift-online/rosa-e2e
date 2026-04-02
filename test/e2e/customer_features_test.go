//go:build E2Etests

package e2e

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
)

var _ = Describe("Customer Features: Machine Pools", labels.High, labels.Positive, labels.HCP, labels.CustomerFeatures, func() {
	It("should list node pools for the cluster", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Querying node pools via OCM API")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).
			NodePools().List().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
		Expect(resp.Total()).To(BeNumerically(">", 0))

		GinkgoWriter.Printf("Found %d node pools\n", resp.Total())
	})

	It("should have node pool with expected instance type", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Getting node pool details")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).
			NodePools().List().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		items := resp.Items().Slice()
		Expect(items).NotTo(BeEmpty())

		instanceType := items[0].AWSNodePool().InstanceType()
		GinkgoWriter.Printf("First node pool instance type: %s\n", instanceType)
		Expect(instanceType).NotTo(BeEmpty())
	})
})

var _ = Describe("Customer Features: Cluster Admin RBAC", labels.High, labels.Positive, labels.HCP, labels.CustomerFeatures, func() {
	It("should have cluster-admin ClusterRoleBinding", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		By("Checking for cluster-admin ClusterRoleBinding")
		crbList, err := tc.HCKubeClient().RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())

		found := false
		for _, crb := range crbList.Items {
			if crb.RoleRef.Name == "cluster-admin" {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "expected a ClusterRoleBinding referencing cluster-admin")
	})
})

var _ = Describe("Customer Features: Network Policies", labels.High, labels.Positive, labels.HCP, labels.CustomerFeatures, func() {
	It("should support NetworkPolicy creation", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		namespace := "e2e-netpol-test"
		By("Creating test namespace")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_, err := tc.HCKubeClient().CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if err == nil {
			DeferCleanup(func() {
				_ = tc.HCKubeClient().CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
			})
		}

		By("Creating a deny-all NetworkPolicy")
		np := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deny-all",
				Namespace: namespace,
			},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: []networkingv1.PolicyType{
					networkingv1.PolicyTypeIngress,
					networkingv1.PolicyTypeEgress,
				},
			},
		}
		_, err = tc.HCKubeClient().NetworkingV1().NetworkPolicies(namespace).Create(ctx, np, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("Customer Features: Ingress", labels.High, labels.Positive, labels.HCP, labels.CustomerFeatures, func() {
	It("should have default IngressController", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		By("Checking for default IngressController")
		gvr := schema.GroupVersionResource{
			Group:    "operator.openshift.io",
			Version:  "v1",
			Resource: "ingresscontrollers",
		}
		_, err := tc.HCDynamicClient().Resource(gvr).Namespace("openshift-ingress-operator").Get(ctx, "default", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("Customer Features: Log Forwarding", labels.Medium, labels.Positive, labels.HCP, labels.CustomerFeatures, func() {
	It("should have ClusterLogForwarder CRD available", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}
		tc := framework.NewTestContext(cfg, conn)
		Expect(tc.InitHCClients()).To(Succeed())

		By("Checking if ClusterLogForwarder CRD exists")

		// Try logging.openshift.io/v1 first (older/common API group)
		gvrLogging := schema.GroupVersionResource{
			Group:    "logging.openshift.io",
			Version:  "v1",
			Resource: "clusterlogforwarders",
		}
		_, err := tc.HCDynamicClient().Resource(gvrLogging).List(ctx, metav1.ListOptions{})
		if err == nil {
			return
		}

		// If not a "not found" error, fail the test
		if !apierrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}

		// Try observability.openshift.io/v1 (newer API group)
		gvrObservability := schema.GroupVersionResource{
			Group:    "observability.openshift.io",
			Version:  "v1",
			Resource: "clusterlogforwarders",
		}
		_, err = tc.HCDynamicClient().Resource(gvrObservability).List(ctx, metav1.ListOptions{})
		if err == nil {
			return
		}

		// If also not found, skip the test
		if apierrors.IsNotFound(err) {
			Skip("ClusterLogForwarder CRD not available on this cluster")
		}

		// Some other error, fail the test
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("Customer Features: External OIDC", labels.Medium, labels.Positive, labels.HCP, labels.CustomerFeatures, func() {
	PIt("should authenticate via external OIDC provider")
})

var _ = Describe("Customer Features: KMS Encryption", labels.Medium, labels.Positive, labels.HCP, labels.CustomerFeatures, func() {
	It("should report KMS configuration via OCM API", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}
		tc := framework.NewTestContext(cfg, conn)

		By("Querying cluster KMS configuration")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		kmsKeyARN := resp.Body().AWS().KMSKeyArn()
		if kmsKeyARN == "" {
			Skip("Cluster does not have KMS encryption configured")
		}
		GinkgoWriter.Printf("KMS Key ARN: %s\n", kmsKeyARN)
		Expect(kmsKeyARN).To(HavePrefix("arn:aws:kms:"), "KMS key ARN should be a valid AWS KMS ARN")
	})
})

var _ = Describe("Customer Features: PrivateLink", labels.Medium, labels.Positive, labels.HCP, labels.CustomerFeatures, func() {
	It("should report PrivateLink status via OCM API", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}
		tc := framework.NewTestContext(cfg, conn)

		By("Querying cluster PrivateLink status")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		isPrivateLink := resp.Body().AWS().PrivateLink()
		GinkgoWriter.Printf("PrivateLink enabled: %v\n", isPrivateLink)
		if !isPrivateLink {
			Skip("Cluster does not have PrivateLink enabled")
		}
	})
})
