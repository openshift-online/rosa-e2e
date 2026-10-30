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

var _ = Describe("Minimum OCP Version Coverage", labels.Critical, labels.Positive, labels.MinVersion, func() {
	BeforeEach(func() {
		// Require CLUSTER_ID to be set (cluster is pre-provisioned by openshift/release job)
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set; minimum version tests require a pre-provisioned cluster")
		}
	})

	It("verifies HCP cluster health at minimum OCP version from prod:canary", labels.HCP, func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		if !tc.IsHCP() {
			Skip("HCP topology required for minimum version coverage")
		}

		By("Verifying cluster is ready in OCM")
		Expect(verifiers.VerifyClusterReady(tc.Connection(), cfg.ClusterID)).To(Succeed())

		By("Obtaining cluster credentials")
		kubeConfig, err := framework.GetClusterCredentials(tc.Connection(), cfg.ClusterID)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying worker nodes are Ready")
		kubeClient, err := framework.NewKubeClient(kubeConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(verifiers.RunVerifiers(ctx, kubeClient,
			verifiers.VerifyAllNodesReady(),
		)).To(Succeed())

		By("Verifying cluster operators are Ready")
		dynClient, err := framework.NewDynamicClient(kubeConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(verifiers.VerifyClusterOperatorsReady(ctx, dynClient)).To(Succeed())
	})

})
