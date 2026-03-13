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

var _ = Describe("ROSA HCP HostedCluster Health", labels.Critical, labels.Positive, labels.HCP, labels.ManagedService, func() {
	It("should have all HCP namespace deployments healthy", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured, skipping HCP namespace health test")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		ns := tc.HCPNamespaces()
		Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

		By("Verifying all deployments in " + ns.HCPNamespace + " are healthy")
		Expect(verifiers.VerifyHCPNamespaceHealthy(ctx, tc.MCKubeClient(), ns.HCPNamespace)).To(Succeed())
	})

	It("should have healthy HostedCluster CR", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured, skipping HostedCluster CR test")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		ns := tc.HCPNamespaces()
		Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

		By("Verifying HostedCluster CR in " + ns.HCNamespace)
		Expect(verifiers.VerifyHostedClusterHealthy(ctx, tc.MCDynamicClient(), ns.HCNamespace, ns.ClusterName)).To(Succeed())
	})

	It("should have healthy NodePool CRs", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured, skipping NodePool CR test")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		ns := tc.HCPNamespaces()
		Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

		By("Verifying NodePool CRs in " + ns.HCNamespace)
		Expect(verifiers.VerifyNodePoolHealthy(ctx, tc.MCDynamicClient(), ns.HCNamespace, ns.ClusterName)).To(Succeed())
	})
})
