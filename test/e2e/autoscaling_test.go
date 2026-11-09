//go:build E2Etests

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

const (
	autoscalingNodePoolID = "e2e-autoscale"
	autoscalingNamespace  = "e2e-autoscaling-test"
	autoscalingMinNodes   = 1
	autoscalingMaxNodes   = 3
)

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

// The spec deliberately carries no Area or Platform label: it is slow and
// provisions extra nodes, so it must only be selected by the dedicated
// Feature:Autoscaling job rather than the general matrix filters
// (Area:DataPlane || Area:CustomerFeatures, Platform:HCP). Non-HCP clusters
// are guarded by the topology skip in setupAutoscaling.
var _ = Describe("HCP Node Pool Autoscaling",
	labels.Medium, labels.Positive, labels.Autoscaling,
	func() {
		It("should scale a node pool up to max and back down to min based on workload demand", labels.Slow, func(ctx context.Context) {
			if cfg.ClusterID == "" {
				Skip("CLUSTER_ID not set")
			}
			tc := setupAutoscaling()

			// Node pool labels are applied to every node in the pool, so the
			// workload uses this selector to pin its pods to the pool.
			nodePoolLabel := "e2e-autoscale-pool"

			By("Creating a node pool with autoscaling enabled")
			nodePool, err := cmv1.NewNodePool().
				ID(autoscalingNodePoolID).
				Labels(map[string]string{nodePoolLabel: autoscalingNodePoolID}).
				Autoscaling(cmv1.NewNodePoolAutoscaling().
					MinReplica(autoscalingMinNodes).
					MaxReplica(autoscalingMaxNodes)).
				Build()
			Expect(err).NotTo(HaveOccurred())

			created, err := framework.CreateNodePool(ctx, tc.Connection(), cfg.ClusterID, nodePool)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func(ctx context.Context) {
				By("Deleting the autoscaling node pool")
				Expect(framework.DeleteNodePool(ctx, tc.Connection(), cfg.ClusterID, created.ID())).To(Succeed())
				// Wait until the pool is actually gone: the delete request
				// returns before deletion completes, and a later run reusing
				// the pool ID would fail with a conflict.
				Expect(framework.WaitForNodePoolDeleted(ctx, tc.Connection(), cfg.ClusterID,
					created.ID(), 20*time.Minute)).To(Succeed())
			})

			currentReplicas := func() int {
				np, err := framework.GetNodePool(ctx, tc.Connection(), cfg.ClusterID, created.ID())
				Expect(err).NotTo(HaveOccurred())
				return np.Status().CurrentReplicas()
			}

			By("Waiting for the node pool to reach its minimum replica count")
			Eventually(func(g Gomega) int {
				np, err := framework.GetNodePool(ctx, tc.Connection(), cfg.ClusterID, created.ID())
				g.Expect(err).NotTo(HaveOccurred())
				return np.Status().CurrentReplicas()
			}).WithContext(ctx).WithTimeout(20 * time.Minute).WithPolling(30 * time.Second).
				Should(Equal(autoscalingMinNodes))

			if tc.HasMCAccess() {
				By("Verifying autoscaling bounds on the NodePool CR in the management cluster")
				Expect(tc.InitMCClients()).To(Succeed())
				ns := tc.HCPNamespaces()
				Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")
				Expect(verifiers.VerifyNodePoolAutoScaling(ctx, tc.MCDynamicClient(), ns.HCNamespace,
					created.ID(), autoscalingMinNodes, autoscalingMaxNodes)).To(Succeed())
				GinkgoWriter.Printf("NodePool %s/%s has autoScaling min=%d max=%d\n",
					ns.HCNamespace, created.ID(), autoscalingMinNodes, autoscalingMaxNodes)
			} else {
				GinkgoWriter.Println("MANAGEMENT_CLUSTER_ID not configured, skipping NodePool CR verification")
			}

			By("Initializing hosted cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())

			By("Creating test namespace")
			cleanup, err := framework.CreateTestNamespace(ctx, tc.HCKubeClient(), autoscalingNamespace)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			By("Resolving allocatable CPU of the node pool nodes")
			var allocatableMilliCPU int64
			Eventually(func(g Gomega) bool {
				nodes, err := tc.HCKubeClient().CoreV1().Nodes().List(ctx, metav1.ListOptions{
					LabelSelector: fmt.Sprintf("%s=%s", nodePoolLabel, autoscalingNodePoolID),
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
			replicas := int32(autoscalingMaxNodes)
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
							NodeSelector: map[string]string{nodePoolLabel: autoscalingNodePoolID},
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
				np, err := framework.GetNodePool(ctx, tc.Connection(), cfg.ClusterID, created.ID())
				g.Expect(err).NotTo(HaveOccurred())
				return np.Status().CurrentReplicas()
			}).WithContext(ctx).WithTimeout(30 * time.Minute).WithPolling(30 * time.Second).
				Should(Equal(autoscalingMaxNodes))
			GinkgoWriter.Printf("Node pool %s scaled up to %d replicas\n", created.ID(), currentReplicas())

			By("Deleting the workload")
			Expect(tc.HCKubeClient().AppsV1().Deployments(autoscalingNamespace).
				Delete(ctx, deployment.Name, metav1.DeleteOptions{})).To(Succeed())

			By("Waiting for the node pool to scale back down to its minimum replica count")
			Eventually(func(g Gomega) int {
				np, err := framework.GetNodePool(ctx, tc.Connection(), cfg.ClusterID, created.ID())
				g.Expect(err).NotTo(HaveOccurred())
				return np.Status().CurrentReplicas()
			}).WithContext(ctx).WithTimeout(60 * time.Minute).WithPolling(30 * time.Second).
				Should(Equal(autoscalingMinNodes))
			GinkgoWriter.Printf("Node pool %s scaled back down to %d replicas\n", created.ID(), currentReplicas())
		})
	})
