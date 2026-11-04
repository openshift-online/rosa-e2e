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

// setupKMS resolves the cluster's KMS key ARN and initialises AWS clients for
// the cluster region, skipping when the cluster has no KMS key configured or
// AWS credentials are not available.
func setupKMS(ctx context.Context) (*framework.TestContext, string) {
	tc := framework.NewTestContext(cfg, conn)

	By("Querying cluster KMS configuration")
	resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
	Expect(err).NotTo(HaveOccurred())

	kmsKeyARN := resp.Body().AWS().KMSKeyArn()
	if kmsKeyARN == "" {
		Skip("Cluster does not have KMS encryption configured")
	}

	By("Configuring the AWS client for the cluster region")
	region := resp.Body().Region().ID()
	if region == "" {
		Skip("Cluster region not reported by OCM, skipping KMS validation")
	}
	cfg.AWSRegion = region

	By("Initializing AWS clients")
	err = tc.InitAWSClients(ctx)
	if err != nil || !tc.HasAWSAccess() {
		Skip("AWS credentials not available, skipping KMS validation")
	}

	return tc, kmsKeyARN
}

// ── Data plane: dynamic PV encryption with the customer-managed key ────────

var _ = Describe("Data Plane: KMS Storage Encryption",
	labels.High, labels.Positive, labels.HCP, labels.Classic, labels.DataPlane, labels.KMS,
	func() {
		It("should encrypt dynamically provisioned PVs with the customer-managed KMS key", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc, kmsKeyARN := setupKMS(ctx)

			By("Initializing hosted cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			namespace := "e2e-kms-storage-test"

			By("Creating test namespace")
			cleanup, err := framework.CreateTestNamespace(ctx, tc.HCKubeClient(), namespace)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			By("Creating a storage class encrypted with the cluster KMS key")
			scCleanup, err := framework.CreateKMSStorageClass(ctx, tc.HCKubeClient(), "e2e-kms-gp3-csi", kmsKeyARN)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(scCleanup)

			By("Creating a PVC")
			pvcName, err := framework.CreateTestPVC(ctx, tc.HCKubeClient(), namespace, "e2e-kms-gp3-csi")
			Expect(err).NotTo(HaveOccurred())

			By("Creating a pod that writes data to the PVC")
			_, err = framework.CreateTestPodWithPVC(ctx, tc.HCKubeClient(), namespace, pvcName)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying PVC is bound")
			Eventually(func(g Gomega) {
				g.Expect(verifiers.VerifyPVCBound(ctx, tc.HCKubeClient(), namespace, pvcName)).To(Succeed())
			}).WithContext(ctx).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

			By("Resolving the EBS volume backing the PVC")
			volumeID := ""
			Eventually(func(g Gomega) {
				id, err := framework.GetPVCVolumeID(ctx, tc.HCKubeClient(), namespace, pvcName)
				g.Expect(err).NotTo(HaveOccurred())
				volumeID = id
			}).WithContext(ctx).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
			GinkgoWriter.Printf("PVC %s/%s is backed by volume %s\n", namespace, pvcName, volumeID)

			By("Validating the volume is encrypted with the cluster KMS key")
			Expect(verifiers.VerifyEBSVolumeEncryptedWithKey(ctx, tc.EC2Client(), volumeID, kmsKeyARN)).To(Succeed())
		})
	})

// ── Management plane: KMS key health ────────────────────────────────────────

var _ = Describe("Management Plane: KMS Key Health",
	labels.Medium, labels.Positive, labels.HCP, labels.Classic, labels.ManagementPlane, labels.KMS,
	func() {
		It("should have a healthy KMS key backing cluster encryption", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc, kmsKeyARN := setupKMS(ctx)

			By("Resolving operator role prefix")
			operatorRolePrefix, err := tc.ResolveOperatorRolePrefix()
			Expect(err).NotTo(HaveOccurred(), "failed to resolve operator role prefix from OCM")

			By("Validating the KMS key is enabled, not scheduled for deletion, and usable by the cluster")
			Expect(verifiers.VerifyKMSKeyHealthy(ctx, tc.KMSClient(), kmsKeyARN, operatorRolePrefix)).To(Succeed())
		})
	})

// ── Management plane: root-volume encryption across node pool scale-out ────

var _ = Describe("Management Plane: KMS NodePool Scale-Out",
	labels.High, labels.Positive, labels.Slow, labels.HCP, labels.ManagementPlane, labels.KMS, labels.MCAccess,
	func() {
		It("should encrypt root volumes of newly scaled nodes with the customer-managed KMS key", func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc, kmsKeyARN := setupKMS(ctx)

			if !tc.IsHCP() {
				Skip("Node pool scale-out verification is HCP-only")
			}
			if !tc.HasMCAccess() {
				Skip("MANAGEMENT_CLUSTER_ID not configured, skipping node pool scale-out verification")
			}

			By("Initializing management cluster clients")
			Expect(tc.InitMCClients()).To(Succeed())

			ns := tc.HCPNamespaces()
			Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

			By("Listing node pools")
			pools, err := framework.ListHCPNodePools(ctx, tc.MCDynamicClient(), ns.HCNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(pools).NotTo(BeEmpty(), "no node pools found")

			pool := pools[0]
			if pool.Replicas == 0 {
				Skip("node pool " + pool.Name + " has no static replica count (autoscaling), cannot scale deterministically")
			}
			targetReplicas := pool.Replicas + 1
			GinkgoWriter.Printf("Scaling node pool %s from %d to %d replicas\n", pool.Name, pool.Replicas, targetReplicas)

			DeferCleanup(func() error {
				return framework.ScaleHCPNodePool(context.Background(), tc.MCDynamicClient(), ns.HCNamespace, pool.Name, pool.Replicas)
			})

			By("Counting root volumes before scale-out")
			before, err := verifiers.CountRootVolumes(ctx, tc.EC2Client(), cfg.ClusterID)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Printf("Found %d root volumes before scale-out\n", before)

			By("Scaling the node pool out by one replica")
			Expect(framework.ScaleHCPNodePool(ctx, tc.MCDynamicClient(), ns.HCNamespace, pool.Name, targetReplicas)).To(Succeed())

			By("Waiting for the node pool to become ready with the additional node")
			Eventually(func(g Gomega) {
				g.Expect(verifiers.VerifyNodePoolHealthy(ctx, tc.MCDynamicClient(), ns.HCNamespace, ns.ClusterName)).To(Succeed())
				after, err := verifiers.CountRootVolumes(ctx, tc.EC2Client(), cfg.ClusterID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(after).To(Equal(before + 1))
			}).WithContext(ctx).WithTimeout(30 * time.Minute).WithPolling(30 * time.Second).Should(Succeed())

			By("Validating that all root volumes, including the new node's, use the cluster KMS key")
			Expect(verifiers.VerifyEBSVolumesEncrypted(ctx, tc.EC2Client(), cfg.ClusterID, kmsKeyARN)).To(Succeed())
		})
	})
