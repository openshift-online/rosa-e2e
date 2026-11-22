//go:build E2Etests

package e2e

import (
	"context"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
)

// Zero-egress row of the ROSAENG-308 provision x validate matrix (ROSAENG-66391):
// the cluster is provisioned with properties zero_egress:true in a VPC whose
// private subnets have no route to the internet. Control-plane and payload
// images are served from the provision shard's regional ECR mirror instead of
// quay.io, and the kubelet pulls them with credentials from the
// AmazonEC2ContainerRegistryReadOnly policy attached to the HCP worker role.

var (
	hostedClusterGVR = schema.GroupVersionResource{
		Group:    "hypershift.openshift.io",
		Version:  "v1beta1",
		Resource: "hostedclusters",
	}
	nodePoolGVR = schema.GroupVersionResource{
		Group:    "hypershift.openshift.io",
		Version:  "v1beta1",
		Resource: "nodepools",
	}
)

// skipUnlessZeroEgress gates the suite on the contract flag recorded by the
// provisioner head (feature_flags.zero_egress, exported as FEATURE_ZERO_EGRESS
// by rosa-e2e-validate-customer/-platform).
func skipUnlessZeroEgress() {
	if cfg.ClusterID == "" {
		Skip("CLUSTER_ID not set")
	}
	if os.Getenv("FEATURE_ZERO_EGRESS") != "true" {
		Skip("Cluster is not zero-egress (feature_flags.zero_egress not set)")
	}
}

// ── Customer plane: image pulls without internet egress ────────────────────

var _ = Describe("Zero Egress: Image Pulls",
	labels.High, labels.Positive, labels.HCP, labels.ZeroEgress, labels.DataPlane, func() {
		BeforeEach(func() {
			skipUnlessZeroEgress()
		})

		It("should pull a release-payload image through the ECR mirror", func(ctx context.Context) {
			tc := framework.NewTestContext(cfg, conn)

			By("Initializing hosted cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())
			client := tc.HCKubeClient()

			// A namespace private to this spec: cleanup is asynchronous, so a
			// shared name would let the next spec race a Terminating namespace.
			namespace := "e2e-zero-egress-mirror-pull"

			By("Creating test namespace " + namespace)
			cleanup, err := framework.CreateTestNamespace(ctx, client, namespace)
			if cleanup != nil {
				DeferCleanup(cleanup)
			}
			Expect(err).NotTo(HaveOccurred(), "creating test namespace")

			// Pick a digest-pinned image that already runs on the cluster: every
			// payload image is mirrored to the shard ECR repository, and the
			// quay.io reference is rewritten to the mirror by imageContentSources.
			By("Selecting a release-payload image from openshift-monitoring")
			image := payloadImageFromMonitoring(ctx, client)

			By("Creating a pod with the payload image (imagePullPolicy: Always)")
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zero-egress-mirror-pull",
					Namespace: namespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "mirror-pull",
							Image:           image,
							Command:         []string{"sleep", "600"},
							ImagePullPolicy: corev1.PullAlways,
						},
					},
				},
			}
			_, err = client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "creating mirror-pull pod")

			By("Waiting for the pod to be scheduled and its image pulled")
			// PullAlways forces a fresh pull from the ECR mirror even if the
			// image is cached on the node. Runtime crashes are not pull
			// failures, so any state other than ImagePullBackOff/ErrImagePull
			// proves the mirror path works.
			Eventually(func() bool {
				return podProgressedPastPull(ctx, client, namespace, "zero-egress-mirror-pull")
			}).WithTimeout(6*time.Minute).WithPolling(15*time.Second).Should(BeTrue(),
				"payload image %s should be pulled through the ECR mirror", image)
		})

		It("should not pull non-mirrored public images", func(ctx context.Context) {
			tc := framework.NewTestContext(cfg, conn)

			By("Initializing hosted cluster clients")
			Expect(tc.InitHCClients()).To(Succeed())
			client := tc.HCKubeClient()

			// A namespace private to this spec: cleanup is asynchronous, so a
			// shared name would let the next spec race a Terminating namespace.
			namespace := "e2e-zero-egress-blocked-pull"

			By("Creating test namespace " + namespace)
			cleanup, err := framework.CreateTestNamespace(ctx, client, namespace)
			if cleanup != nil {
				DeferCleanup(cleanup)
			}
			Expect(err).NotTo(HaveOccurred(), "creating test namespace")

			By("Creating a pod with a non-mirrored public image")
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zero-egress-blocked-pull",
					Namespace: namespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "blocked-pull",
							Image:   "docker.io/library/busybox:latest",
							Command: []string{"sleep", "600"},
						},
					},
				},
			}
			_, err = client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "creating blocked-pull pod")

			By("Waiting for the pull to be rejected")
			// With no route to the internet, a registry that is not part of the
			// ECR mirror must be unreachable. If this pull ever succeeds, the
			// cluster silently regained egress (or the mirror config grew too
			// broad).
			Eventually(func() bool {
				p, err := client.CoreV1().Pods(namespace).Get(ctx, "zero-egress-blocked-pull", metav1.GetOptions{})
				if err != nil {
					return false
				}
				for _, status := range p.Status.ContainerStatuses {
					if status.State.Waiting != nil &&
						status.State.Waiting.Reason == "ImagePullBackOff" {
						return true
					}
				}
				return false
			}).WithTimeout(5*time.Minute).WithPolling(15*time.Second).Should(BeTrue(),
				"non-mirrored image pull should end in ImagePullBackOff on a zero-egress cluster")
		})
	})

// payloadImageFromMonitoring returns a running digest-pinned payload image,
// preferring the cluster-monitoring-operator deployment and falling back to
// any container image of a running pod in openshift-monitoring.
func payloadImageFromMonitoring(ctx context.Context, client kubernetes.Interface) string {
	deploy, err := client.AppsV1().Deployments("openshift-monitoring").Get(ctx, "cluster-monitoring-operator", metav1.GetOptions{})
	if err == nil && len(deploy.Spec.Template.Spec.Containers) > 0 {
		image := deploy.Spec.Template.Spec.Containers[0].Image
		Expect(image).NotTo(BeEmpty(), "cluster-monitoring-operator has no container image")
		return image
	}

	pods, err := client.CoreV1().Pods("openshift-monitoring").List(ctx, metav1.ListOptions{Limit: 50})
	Expect(err).NotTo(HaveOccurred(), "listing openshift-monitoring pods")
	for _, p := range pods.Items {
		if p.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range p.Spec.Containers {
			if strings.Contains(c.Image, "@sha256:") {
				return c.Image
			}
		}
	}
	Fail("no running digest-pinned payload image found in openshift-monitoring")
	return ""
}

// podProgressedPastPull reports whether the pod's container has definitively
// moved past image pulling (running, crashed after pulling, or terminated) —
// as opposed to being stuck in ErrImagePull/ImagePullBackOff or not scheduled.
func podProgressedPastPull(ctx context.Context, client kubernetes.Interface, namespace, name string) bool {
	p, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false
	}
	if len(p.Status.ContainerStatuses) == 0 {
		return false
	}
	for _, status := range p.Status.ContainerStatuses {
		if status.State.Waiting != nil {
			switch status.State.Waiting.Reason {
			case "ErrImagePull", "ImagePullBackOff":
				return false
			case "ContainerCreating", "PodInitializing":
				return false
			}
			// Any other waiting reason (CrashLoopBackOff, etc.) means the image
			// was pulled and the container is failing at runtime, not on pulls.
		}
		if status.State.Running != nil || status.State.Terminated != nil {
			return true
		}
	}
	return false
}

// ── Platform plane: ECR mirror wiring on the management cluster ────────────

var _ = Describe("Zero Egress: ECR Mirror Configuration",
	labels.High, labels.Positive, labels.HCP, labels.ZeroEgress, labels.ManagementPlane, labels.MCAccess, func() {
		var tc *framework.TestContext

		BeforeEach(func() {
			skipUnlessZeroEgress()
			tc = framework.NewTestContext(cfg, conn)
			if !tc.HasMCAccess() {
				Skip("MANAGEMENT_CLUSTER_ID not configured, skipping platform plane test")
			}
			Expect(tc.InitMCClients()).To(Succeed())
		})

		It("should mirror release payload images to the shard ECR repository", func(ctx context.Context) {
			ns := tc.HCPNamespaces()
			Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

			By("Reading the HostedCluster CR in " + ns.HCNamespace)
			hc, err := tc.MCDynamicClient().Resource(hostedClusterGVR).
				Namespace(ns.HCNamespace).Get(ctx, ns.ClusterName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "getting HostedCluster %s", ns.ClusterName)

			spec, ok := hc.Object["spec"].(map[string]any)
			Expect(ok).To(BeTrue(), "HostedCluster has no spec")
			sources, ok := spec["imageContentSources"].([]any)
			Expect(ok).To(BeTrue(), "HostedCluster has no imageContentSources")
			Expect(sources).NotTo(BeEmpty(), "zero-egress HostedCluster must pin image content sources")

			By("Verifying the release payload source is mirrored to ECR")
			found := false
			for _, s := range sources {
				source, ok := s.(map[string]any)
				if !ok {
					continue
				}
				srcName, _ := source["source"].(string)
				mirrors, _ := source["mirrors"].([]any)
				if srcName != "quay.io/openshift-release-dev/ocp-release" && srcName != "quay.io/openshift-release-dev/ocp-v4.0-art-dev" {
					continue
				}
				for _, m := range mirrors {
					mirror, _ := m.(string)
					if isECRMirror(mirror) {
						found = true
					}
				}
			}
			Expect(found).To(BeTrue(),
				"payload sources must be mirrored to the provision shard ECR repository: %v", sources)
		})

		It("should deliver the ECR mirror configuration to worker nodes via ignition", func(ctx context.Context) {
			ns := tc.HCPNamespaces()
			Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

			// The control plane operator renders the image content sources as an
			// ImageDigestMirrorSet into an ignition ConfigMap in the hosted
			// control plane namespace, so nodes can pull payload images through
			// the ECR mirror from the very first boot.
			By("Listing ignition image-content-source ConfigMaps in " + ns.HCPNamespace)
			cms, err := tc.MCKubeClient().CoreV1().ConfigMaps(ns.HCPNamespace).
				List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred(), "listing ConfigMaps in the hosted control plane namespace")

			// Decode each mirror set and require that a release payload source
			// maps to an ECR mirror — not merely that both strings appear
			// somewhere in the document (which unrelated mappings would satisfy).
			type mirrorEntry struct {
				Source  string   `yaml:"source"`
				Mirrors []string `yaml:"mirrors"`
			}
			type mirrorSet struct {
				Spec struct {
					ImageDigestMirrors      []mirrorEntry `yaml:"imageDigestMirrors"`
					RepositoryDigestMirrors []mirrorEntry `yaml:"repositoryDigestMirrors"`
				} `yaml:"spec"`
			}

			found := false
			for _, cm := range cms.Items {
				if !strings.Contains(cm.Name, "image-content-source") {
					continue
				}
				for _, data := range cm.Data {
					var ms mirrorSet
					if err := yaml.Unmarshal([]byte(data), &ms); err != nil {
						continue
					}
					for _, entry := range append(ms.Spec.ImageDigestMirrors, ms.Spec.RepositoryDigestMirrors...) {
						if !strings.HasPrefix(entry.Source, "quay.io/openshift-release-dev/") {
							continue
						}
						for _, mirror := range entry.Mirrors {
							if isECRMirror(mirror) {
								found = true
							}
						}
					}
				}
			}
			Expect(found).To(BeTrue(),
				"worker ignition must map release payload sources to the ECR mirror")
		})

		It("should configure the nodepool kubelet ECR credential provider", func(ctx context.Context) {
			ns := tc.HCPNamespaces()
			Expect(ns).NotTo(BeNil(), "could not resolve HCP namespaces on MC")

			// The ECR credential provider is delivered to node pools as an
			// ignition config referenced from the NodePool spec ("ignition-
			// credential-provider" before 4.19.19, "ignition-credential-
			// provider-aws-api" after).
			By("Reading NodePools in " + ns.HCNamespace)
			pools, err := tc.MCDynamicClient().Resource(nodePoolGVR).
				Namespace(ns.HCNamespace).List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred(), "listing NodePools")
			Expect(pools.Items).NotTo(BeEmpty(), "no NodePools found for the cluster")

			found := false
			for _, pool := range pools.Items {
				spec, ok := pool.Object["spec"].(map[string]any)
				if !ok {
					continue
				}
				configs, _ := spec["config"].([]any)
				for _, c := range configs {
					entry, ok := c.(map[string]any)
					if !ok {
						continue
					}
					name, _ := entry["name"].(string)
					if strings.HasPrefix(name, "ignition-credential-provider") {
						found = true
					}
				}
			}
			Expect(found).To(BeTrue(),
				"no NodePool references the ECR ignition credential provider")
		})
	})

// ecrMirrorHostPattern matches a regional ECR registry hostname, e.g.
// 122610517469.dkr.ecr.us-west-2.amazonaws.com.
var ecrMirrorHostPattern = regexp.MustCompile(`^[0-9]{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com$`)

// isECRMirror reports whether the registry reference points at a regional ECR
// mirror host (the repository below it is the shard's ocp-mirror-N).
func isECRMirror(registry string) bool {
	host, _, found := strings.Cut(registry, "/")
	return found && ecrMirrorHostPattern.MatchString(host)
}
