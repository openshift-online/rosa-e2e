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

var _ = Describe("Infrastructure: Service Cluster Health", labels.High, labels.Positive, labels.HCP, labels.Infrastructure, func() {
	It("should have ACM hub running", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		if !tc.HasSCAccess() {
			Skip("SERVICE_CLUSTER_ID not configured")
		}

		By("Initializing service cluster clients")
		Expect(tc.InitSCClients()).To(Succeed())

		By("Verifying ACM hub is running")
		Expect(verifiers.VerifyACMHub(ctx, tc.SCDynamicClient())).To(Succeed())
	})

	It("should have cert-manager operational", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		if !tc.HasSCAccess() {
			Skip("SERVICE_CLUSTER_ID not configured")
		}

		By("Initializing service cluster clients")
		Expect(tc.InitSCClients()).To(Succeed())

		By("Verifying cert-manager is available")
		Expect(verifiers.VerifyCertManager(ctx, tc.SCKubeClient())).To(Succeed())
	})

	It("should have Hive controllers running", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		if !tc.HasSCAccess() {
			Skip("SERVICE_CLUSTER_ID not configured")
		}

		By("Initializing service cluster clients")
		Expect(tc.InitSCClients()).To(Succeed())

		By("Verifying hive-controllers is available")
		Expect(verifiers.VerifyHiveControllers(ctx, tc.SCKubeClient())).To(Succeed())
	})

	It("should have MCE components healthy", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		if !tc.HasSCAccess() {
			Skip("SERVICE_CLUSTER_ID not configured")
		}

		By("Initializing service cluster clients")
		Expect(tc.InitSCClients()).To(Succeed())

		By("Verifying MultiClusterEngine is available")
		Expect(verifiers.VerifyMCE(ctx, tc.SCDynamicClient())).To(Succeed())
	})
})
