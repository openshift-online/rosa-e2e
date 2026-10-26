//go:build E2Etests

package e2e

import (
	"context"
	"fmt"

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

// ── Node-level FIPS (Classic / OSD GCP) ──────────────────────────────────────
// MachineConfig / MachineConfigPool exist on Classic and OSD guests only.
// HCP FIPS jobs should filter Platform:HCP || (Feature:FIPS && !Platform:Classic)
// so this spec is selected on Classic/OSD FIPS jobs but not on HCP.

var _ = Describe("Security: FIPS Node Verification",
	labels.High, labels.Positive, labels.Classic, labels.OSDGCP, labels.FIPS, labels.CustomerFeatures,
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

// ── HostedCluster FIPS (HCP) ─────────────────────────────────────────────────
// ROSA HCP guests have no MachineConfig API. FIPS is HostedCluster.spec.fips
// on the management cluster. Selected by Platform:HCP and Feature:FIPS.

var _ = Describe("Security: FIPS HostedCluster Configuration",
	labels.High, labels.Positive, labels.HCP, labels.FIPS, labels.CustomerFeatures, labels.MCAccess,
	func() {
		It("should have spec.fips enabled on HostedCluster", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := framework.NewTestContext(cfg, conn)
			skipIfNotFIPS(tc, cfg.ClusterID)

			if !tc.HasMCAccess() {
				Skip("MANAGEMENT_CLUSTER_ID not configured, skipping HCP FIPS HostedCluster verification")
			}

			By("Initializing management cluster clients")
			Expect(tc.InitMCClients()).To(Succeed())

			ns := tc.HCPNamespaces()
			Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

			By("Verifying HostedCluster spec.fips is true")
			Expect(verifiers.VerifyHCPFIPSEnabled(ctx, tc.MCDynamicClient(), ns.HCNamespace, ns.ClusterName)).To(Succeed())

			By("Verifying NodePool CRs are healthy")
			Expect(verifiers.VerifyNodePoolHealthy(ctx, tc.MCDynamicClient(), ns.HCNamespace, ns.ClusterName)).To(Succeed())

			GinkgoWriter.Printf("HostedCluster %s/%s has FIPS enabled\n", ns.HCNamespace, ns.ClusterName)
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
		for _, op := range verifiers.CrossPlatformFIPSOperators {
			op := op
			It(fmt.Sprintf("%s should be healthy on FIPS cluster", op.Deployment), func(ctx context.Context) {
				if cfg.ClusterID == "" {
					Skip("CLUSTER_ID not set")
				}
				tc := framework.NewTestContext(cfg, conn)
				skipIfNotFIPS(tc, cfg.ClusterID)

				By("Initializing cluster clients")
				Expect(tc.InitHCClients()).To(Succeed())

				By(fmt.Sprintf("Verifying %s deployment is healthy", op.Deployment))
				found, err := verifiers.VerifyFIPSOperatorHealthy(ctx, tc.HCKubeClient(), op)
				if !found {
					Skip(fmt.Sprintf("%s not installed on this cluster", op.Deployment))
				}
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("%s is healthy on FIPS cluster %s\n", op.Deployment, cfg.ClusterID)
			})
		}
	},
)
