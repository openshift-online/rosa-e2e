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

var _ = Describe("ROSA HCP Managed Operators", labels.Critical, labels.Positive, labels.HCP, labels.ManagedService, func() {
	It("should have all ClusterOperators healthy", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not configured, skipping ClusterOperators test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		By("Verifying all ClusterOperators are healthy")
		Expect(verifiers.VerifyClusterOperatorsHealthy(ctx, tc.HCDynamicClient())).To(Succeed())
	})

	It("should have RMO RouteMonitors on management cluster", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured, skipping RMO test")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		By("Verifying RMO RouteMonitors exist")
		Expect(verifiers.VerifyRMORouteMonitors(ctx, tc.MCDynamicClient(), cfg.ClusterID)).To(Succeed())
	})

	It("should have AVO VpcEndpoints on management cluster", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured, skipping AVO test")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		By("Verifying AVO VpcEndpoints are healthy")
		Expect(verifiers.VerifyAVOVpcEndpoints(ctx, tc.MCDynamicClient(), cfg.ClusterID)).To(Succeed())
	})

	It("should have audit-webhook running on management cluster", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		if !tc.HasMCAccess() {
			Skip("MANAGEMENT_CLUSTER_ID not configured, skipping audit-webhook test")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		By("Verifying audit-webhook deployment is available")
		Expect(verifiers.VerifyAuditWebhook(ctx, tc.MCKubeClient(), cfg.ClusterID)).To(Succeed())
	})
})
