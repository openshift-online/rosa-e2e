//go:build E2Etests

package e2e

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("Infrastructure: Management Cluster Health", labels.High, labels.Positive, labels.HCP, labels.Infrastructure, func() {
	It("should have HyperShift operator running", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		By("Verifying HyperShift operator is available")
		Expect(verifiers.VerifyHyperShiftOperator(ctx, tc.MCKubeClient())).To(Succeed())
	})

	It("should have external-dns running", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		By("Verifying external-dns is available")
		Expect(verifiers.VerifyExternalDNS(ctx, tc.MCKubeClient())).To(Succeed())
	})

	It("should have CAPI provider running in HCP namespace", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		ns := tc.HCPNamespaces()
		Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

		By("Verifying capi-provider in " + ns.HCPNamespace)
		Expect(verifiers.VerifyCAPIProvider(ctx, tc.MCKubeClient(), ns.HCPNamespace)).To(Succeed())
	})

	It("should have hosted control plane namespaces", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		By("Listing HCP namespaces on the management cluster")
		nsList, err := tc.MCKubeClient().CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())

		var hcpNamespaces []string
		for _, ns := range nsList.Items {
			if strings.HasPrefix(ns.Name, "ocm-") {
				hcpNamespaces = append(hcpNamespaces, ns.Name)
			}
		}

		GinkgoWriter.Printf("Found %d HCP-related namespaces\n", len(hcpNamespaces))
		Expect(len(hcpNamespaces)).To(BeNumerically(">", 0), "expected at least one HCP namespace on the MC")
	})
})
