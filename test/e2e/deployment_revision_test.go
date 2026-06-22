//go:build E2Etests

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("Deployment Revision Stability", labels.High, labels.Positive, labels.Classic, labels.OSDGCP, labels.ManagedService, func() {
	It("should have stable deployment revisions across all namespaces", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not configured")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		By("Observing deployment revisions for 30 seconds")
		// Will check all the namespace when set it to empty
		Expect(verifiers.VerifyDeploymentRevisionsStable(ctx, tc.HCKubeClient(), "", 30*time.Second)).To(Succeed())
	})
})

var _ = Describe("HCP Namespace Deployment Revision Stability", labels.High, labels.Positive, labels.HCP, labels.ManagedService, labels.MCAccess, func() {
	It("should have stable deployment revisions in HCP namespace on MC", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		if !tc.HasMCAccess() {
			Skip("MC access not configured, skipping HCP namespace revision stability test")
		}

		By("Initializing management cluster clients")
		Expect(tc.InitMCClients()).To(Succeed())

		ns := tc.HCPNamespaces()
		Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

		By("Observing deployment revisions in " + ns.HCPNamespace + " for 30 seconds")
		Expect(verifiers.VerifyDeploymentRevisionsStable(ctx, tc.MCKubeClient(), ns.HCPNamespace, 30*time.Second)).To(Succeed())
	})
})
