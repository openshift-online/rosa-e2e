//go:build E2Etests

package e2e

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

const autoscalingNamespace = "e2e-autoscaling-test"

// setupAutoscaling initialises the test context, skipping when the cluster is
// not HCP (node pool autoscaling is an HCP feature).
func setupAutoscaling() *framework.TestContext {
	tc := framework.NewTestContext(cfg, conn)

	By("Detecting cluster topology")
	topology, err := framework.DetectClusterTopology(tc.Connection(), cfg.ClusterID)
	Expect(err).NotTo(HaveOccurred(), "failed to detect cluster topology")
	if topology != "hcp" {
		Skip("Node pool autoscaling is only supported on HCP clusters")
	}

	return tc
}

// defaultAutoscalingPool resolves the cluster's default node pool, skipping
// when it does not have autoscaling enabled: the dedicated job provisions the
// cluster with autoscaling enabled on the default pool, and any other cluster
// has nothing to validate here.
func defaultAutoscalingPool(ctx context.Context, conn *sdk.Connection, clusterID string) *cmv1.NodePool {
	pools, err := framework.ListNodePools(ctx, conn, clusterID)
	Expect(err).NotTo(HaveOccurred(), "failed to list node pools")
	Expect(pools).NotTo(BeEmpty(), "cluster has no node pools")

	var pool *cmv1.NodePool
	ids := make([]string, 0, len(pools))
	for _, p := range pools {
		ids = append(ids, p.ID())
		if strings.HasPrefix(p.ID(), "workers") {
			pool = p
		}
	}
	if pool == nil && len(pools) == 1 {
		pool = pools[0]
	}
	Expect(pool).NotTo(BeNil(), "could not identify the default node pool among: %s", strings.Join(ids, ", "))

	if pool.Autoscaling() == nil {
		Skip("Default node pool does not have autoscaling enabled; provision with autoscaling on the default pool")
	}

	return pool
}

// The spec deliberately carries no Area or Platform label: it is slow and
// provisions extra nodes, so it must only be selected by the dedicated
// Feature:Autoscaling job rather than the general matrix filters
// (Area:DataPlane || Area:CustomerFeatures, Platform:HCP). Non-HCP clusters
// are guarded by the topology skip in setupAutoscaling.
var _ = Describe("HCP Node Pool Autoscaling",
	labels.Medium, labels.Positive, labels.Autoscaling,
	func() {
		It("should scale the default node pool up to max and back down to min based on workload demand", labels.Slow, func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := setupAutoscaling()

			By("Resolving the default node pool and its autoscaling bounds")
			pool := defaultAutoscalingPool(ctx, tc.Connection(), cfg.ClusterID)
			minReplicas := pool.Autoscaling().MinReplica()
			maxReplicas := pool.Autoscaling().MaxReplica()
			GinkgoWriter.Printf("Node pool %s has autoscaling min=%d max=%d\n",
				pool.ID(), minReplicas, maxReplicas)

			currentReplicas := func() int {
				np, err := framework.GetNodePool(ctx, tc.Connection(), cfg.ClusterID, pool.ID())
				Expect(err).NotTo(HaveOccurred())
				return np.Status().CurrentReplicas()
			}

			if tc.HasMCAccess() {
				By("Verifying autoscaling bounds on the NodePool CR in the management cluster")
				Expect(tc.InitMCClients()).To(Succeed())
				ns := tc.HCPNamespaces()
				Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")
				Expect(verifiers.VerifyNodePoolAutoScaling(ctx, tc.MCDynamicClient(), ns.HCNamespace,
					pool.ID(), minReplicas, maxReplicas)).To(Succeed())
				GinkgoWriter.Printf("NodePool %s/%s has autoScaling min=%d max=%d\n",
					ns.HCNamespace, pool.ID(), minReplicas, maxReplicas)
			} else {
				GinkgoWriter.Println("MANAGEMENT_CLUSTER_ID not configured, skipping NodePool CR verification")
			}

			By("Initializing hosted cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Creating test namespace")
			cleanup, err := framework.CreateTestNamespace(ctx, tc.HCKubeClient(), autoscalingNamespace)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			By("Resolving allocatable CPU of the worker nodes")
			var allocatableMilliCPU int64
			Eventually(func(g Gomega) bool {
				nodes, err := tc.HCKubeClient().CoreV1().Nodes().List(ctx, metav1.ListOptions{
					LabelSelector: "node-role.kubernetes.io/worker",
				})
				g.Expect(err).NotTo(HaveOccurred())
				for _, node := range nodes.Items {
					if qty, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok && qty.MilliValue() > 0 {
						allocatableMilliCPU = qty.MilliValue()
						return true
					}
				}
				return false
			}).WithContext(ctx).WithTimeout(10 * time.Minute).WithPolling(30 * time.Second).Should(BeTrue())

			// Each pod requests ~60% of a node's allocatable CPU, so only one
			// pod fits per node and the pool must scale to its maximum to
			// accommodate a replica count equal to the maximum.
			cpuRequest := resource.NewMilliQuantity(allocatableMilliCPU*6/10/100*100, resource.DecimalSI)

			By("Deploying a workload sized to require the maximum replica count")
			replicas := int32(maxReplicas)
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "autoscaling-load",
					Namespace: autoscalingNamespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "autoscaling-load"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "autoscaling-load"},
						},
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{"node-role.kubernetes.io/worker": ""},
							Containers: []corev1.Container{
								{
									Name:  "httpd",
									Image: "registry.access.redhat.com/ubi9/httpd-24:latest",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: *cpuRequest,
										},
									},
								},
							},
						},
					},
				},
			}
			_, err = tc.HCKubeClient().AppsV1().Deployments(autoscalingNamespace).Create(ctx, deployment, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the node pool to scale up to its maximum replica count")
			Eventually(func(g Gomega) int {
				np, err := framework.GetNodePool(ctx, tc.Connection(), cfg.ClusterID, pool.ID())
				g.Expect(err).NotTo(HaveOccurred())
				return np.Status().CurrentReplicas()
			}).WithContext(ctx).WithTimeout(30 * time.Minute).WithPolling(30 * time.Second).
				Should(Equal(maxReplicas))
			GinkgoWriter.Printf("Node pool %s scaled up to %d replicas\n", pool.ID(), currentReplicas())

			By("Deleting the workload")
			Expect(tc.HCKubeClient().AppsV1().Deployments(autoscalingNamespace).
				Delete(ctx, deployment.Name, metav1.DeleteOptions{})).To(Succeed())

			By("Waiting for the node pool to scale back down to its minimum replica count")
			Eventually(func(g Gomega) int {
				np, err := framework.GetNodePool(ctx, tc.Connection(), cfg.ClusterID, pool.ID())
				g.Expect(err).NotTo(HaveOccurred())
				return np.Status().CurrentReplicas()
			}).WithContext(ctx).WithTimeout(60 * time.Minute).WithPolling(30 * time.Second).
				Should(Equal(minReplicas))
			GinkgoWriter.Printf("Node pool %s scaled back down to %d replicas\n", pool.ID(), currentReplicas())
		})
	})
