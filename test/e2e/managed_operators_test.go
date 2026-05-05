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

var _ = Describe("ROSA Managed Operators: ClusterOperators", labels.Critical, labels.Positive, labels.HCP, labels.Classic, labels.ManagedService, func() {
	It("should have all ClusterOperators healthy", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not configured, skipping ClusterOperators test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		By("Verifying all ClusterOperators are healthy")
		Expect(verifiers.VerifyClusterOperatorsHealthy(ctx, tc.HCDynamicClient(), cfg.ExcludeClusterOperators...)).To(Succeed())
	})
})

var _ = Describe("ROSA HCP Managed Operators: MC Components", labels.Critical, labels.Positive, labels.HCP, labels.ManagedService, func() {
	It("should have RMO RouteMonitors on management cluster", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured, skipping RMO test")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		ns := tc.HCPNamespaces()
		Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

		By("Verifying RMO RouteMonitors exist in " + ns.HCPNamespace)
		Expect(verifiers.VerifyRMORouteMonitors(ctx, tc.MCDynamicClient(), ns.HCPNamespace)).To(Succeed())
	})

	It("should have AVO VpcEndpoints on management cluster", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured, skipping AVO test")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		ns := tc.HCPNamespaces()
		Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

		By("Verifying AVO VpcEndpoints are healthy in " + ns.HCPNamespace)
		Expect(verifiers.VerifyAVOVpcEndpoints(ctx, tc.MCDynamicClient(), ns.HCPNamespace)).To(Succeed())
	})

	It("should have audit-webhook running on management cluster", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured, skipping audit-webhook test")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		ns := tc.HCPNamespaces()
		Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

		By("Verifying audit-webhook deployment in " + ns.HCPNamespace)
		Expect(verifiers.VerifyAuditWebhook(ctx, tc.MCKubeClient(), ns.HCPNamespace)).To(Succeed())
	})
})
