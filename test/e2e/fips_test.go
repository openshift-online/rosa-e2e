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

// skipIfNotFIPS checks OCM API and skips if the cluster does not have FIPS enabled.
// Call at the top of each It block after creating a TestContext.
func skipIfNotFIPS(tc *framework.TestContext, clusterID string) {
	fipsEnabled, err := verifiers.VerifyClusterFIPSEnabled(tc.Connection(), clusterID)
	Expect(err).NotTo(HaveOccurred())
	if !fipsEnabled {
		Skip("Cluster does not have FIPS enabled")
	}
}

// ── Node-level FIPS ──────────────────────────────────────────────────────────

var _ = Describe("Security: FIPS Node Verification",
	labels.High, labels.Positive, labels.FIPS, labels.CustomerFeatures,
	func() {
		It("should have kernel FIPS mode enabled on all nodes", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Verifying rendered MachineConfigs have FIPS enabled")
			Expect(verifiers.VerifyNodeFIPSEnabled(ctx, tc.HCDynamicClient())).To(Succeed())

			GinkgoWriter.Printf("All rendered MachineConfigs on cluster %s have FIPS enabled\n", cfg.ClusterID)
		})
	},
)

// ── Core OpenShift operators ─────────────────────────────────────────────────

var _ = Describe("Security: FIPS Core Operators",
	labels.High, labels.Positive, labels.FIPS, labels.ManagedService,
	func() {
		It("should have crypto-critical ClusterOperators healthy", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			// Verify all ClusterOperators are healthy on the FIPS cluster.
			// Any operator failure here is a signal that a component may not
			// compile/link correctly against FIPS-compliant crypto libraries.
			By("Verifying all ClusterOperators are healthy on FIPS cluster")
			Expect(verifiers.VerifyClusterOperatorsHealthy(ctx, tc.HCDynamicClient())).To(Succeed())

			GinkgoWriter.Println("All ClusterOperators healthy on FIPS cluster")
		})
	},
)

// ── Package Operator ─────────────────────────────────────────────────────────

var _ = Describe("Security: FIPS Package Operator",
	labels.High, labels.Positive, labels.FIPS, labels.ManagedService,
	func() {
		It("package-operator-manager should be healthy on FIPS cluster", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Verifying Package Operator deployment is healthy")
			found, err := verifiers.VerifyPackageOperatorHealthy(ctx, tc.HCKubeClient())
			if !found {
				Skip("Package Operator not installed on this cluster")
			}
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("Package Operator is healthy on FIPS cluster %s\n", cfg.ClusterID)
		})

		It("all cross-platform ClusterPackages should be healthy on FIPS cluster", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Verifying cross-platform ClusterPackages are healthy")
			Expect(verifiers.VerifyFIPSClusterPackagesHealthy(ctx, tc.HCDynamicClient(), verifiers.CrossPlatformFIPSClusterPackages)).To(Succeed())

			GinkgoWriter.Printf("All %d cross-platform ClusterPackages healthy on FIPS cluster %s\n",
				len(verifiers.CrossPlatformFIPSClusterPackages), cfg.ClusterID)
		})
	},
)

// ── Managed operators — cross-platform ───────────────────────────────────────

var _ = Describe("Security: FIPS Managed Operators (Cross-Platform)",
	labels.High, labels.Positive, labels.FIPS, labels.ManagedService,
	func() {
		It("managed-node-metadata-operator should be healthy on FIPS cluster", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Verifying managed-node-metadata-operator deployment is healthy")
			op := verifiers.FIPSOperator{Namespace: "openshift-managed-node-metadata-operator", Deployment: "managed-node-metadata-operator"}
			found, err := verifiers.VerifyFIPSOperatorHealthy(ctx, tc.HCKubeClient(), op)
			if !found {
				Skip("managed-node-metadata-operator not installed on this cluster")
			}
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("managed-node-metadata-operator is healthy on FIPS cluster %s\n", cfg.ClusterID)
		})

		It("managed-upgrade-operator should be healthy on FIPS cluster", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Verifying managed-upgrade-operator deployment is healthy")
			op := verifiers.FIPSOperator{Namespace: "openshift-managed-upgrade-operator", Deployment: "managed-upgrade-operator"}
			found, err := verifiers.VerifyFIPSOperatorHealthy(ctx, tc.HCKubeClient(), op)
			if !found {
				Skip("managed-upgrade-operator not installed on this cluster")
			}
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("managed-upgrade-operator is healthy on FIPS cluster %s\n", cfg.ClusterID)
		})

		It("managed-velero-operator should be healthy on FIPS cluster", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Verifying managed-velero-operator deployment is healthy")
			op := verifiers.FIPSOperator{Namespace: "openshift-managed-velero-operator", Deployment: "managed-velero-operator"}
			found, err := verifiers.VerifyFIPSOperatorHealthy(ctx, tc.HCKubeClient(), op)
			if !found {
				Skip("managed-velero-operator not installed on this cluster")
			}
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("managed-velero-operator is healthy on FIPS cluster %s\n", cfg.ClusterID)
		})

		It("rbac-permissions-operator should be healthy on FIPS cluster", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Verifying rbac-permissions-operator deployment is healthy")
			op := verifiers.FIPSOperator{Namespace: "openshift-rbac-permissions", Deployment: "rbac-permissions-operator"}
			found, err := verifiers.VerifyFIPSOperatorHealthy(ctx, tc.HCKubeClient(), op)
			if !found {
				Skip("rbac-permissions-operator not installed on this cluster")
			}
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("rbac-permissions-operator is healthy on FIPS cluster %s\n", cfg.ClusterID)
		})

		It("route-monitor-operator should be healthy on FIPS cluster", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Verifying route-monitor-operator deployment is healthy")
			op := verifiers.FIPSOperator{Namespace: "openshift-route-monitor-operator", Deployment: "route-monitor-operator"}
			found, err := verifiers.VerifyFIPSOperatorHealthy(ctx, tc.HCKubeClient(), op)
			if !found {
				Skip("route-monitor-operator not installed on this cluster")
			}
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("route-monitor-operator is healthy on FIPS cluster %s\n", cfg.ClusterID)
		})

		It("splunk-forwarder-operator should be healthy on FIPS cluster", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Verifying splunk-forwarder-operator deployment is healthy")
			op := verifiers.FIPSOperator{Namespace: "openshift-splunk-forwarder-operator", Deployment: "splunk-forwarder-operator"}
			found, err := verifiers.VerifyFIPSOperatorHealthy(ctx, tc.HCKubeClient(), op)
			if !found {
				Skip("splunk-forwarder-operator not installed on this cluster")
			}
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("splunk-forwarder-operator is healthy on FIPS cluster %s\n", cfg.ClusterID)
		})

		It("configure-alertmanager-operator should be healthy on FIPS cluster", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			By("Initializing cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Verifying configure-alertmanager-operator deployment is healthy")
			op := verifiers.FIPSOperator{Namespace: "openshift-monitoring", Deployment: "configure-alertmanager-operator"}
			found, err := verifiers.VerifyFIPSOperatorHealthy(ctx, tc.HCKubeClient(), op)
			if !found {
				Skip("configure-alertmanager-operator not installed on this cluster")
			}
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("configure-alertmanager-operator is healthy on FIPS cluster %s\n", cfg.ClusterID)
		})
	},
)

