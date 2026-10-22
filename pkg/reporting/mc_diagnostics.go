package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	diagnosticLimit    = 100
	diagnosticMaxBytes = 512 * 1024
)

type MCNamespaces struct {
	HCPNamespace string
	HCNamespace  string
	ClusterName  string
}

type rolloutIdentity struct {
	Name       string      `json:"name"`
	Namespace  string      `json:"namespace"`
	UID        string      `json:"uid"`
	Generation int64       `json:"generation"`
	Created    metav1.Time `json:"created"`
}

type healthCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

type rolloutSnapshot struct {
	rolloutIdentity
	Revision           int64             `json:"revision,omitempty"`
	OwnerName          string            `json:"ownerName,omitempty"`
	OwnerUID           string            `json:"ownerUID,omitempty"`
	Desired            *int32            `json:"desired,omitempty"`
	Replicas           int32             `json:"replicas"`
	Ready              int32             `json:"ready"`
	Available          int32             `json:"available"`
	Updated            int32             `json:"updated,omitempty"`
	Unavailable        int32             `json:"unavailable,omitempty"`
	ObservedGeneration int64             `json:"observedGeneration,omitempty"`
	Conditions         []healthCondition `json:"conditions,omitempty"`
}

type podSnapshot struct {
	rolloutIdentity
	OwnerName  string            `json:"ownerName,omitempty"`
	OwnerUID   string            `json:"ownerUID,omitempty"`
	Phase      string            `json:"phase"`
	Conditions []healthCondition `json:"conditions,omitempty"`
	Restarts   int64             `json:"restarts"`
}

type mcSnapshot struct {
	CapturedAt    time.Time         `json:"capturedAt"`
	Deployments   []rolloutSnapshot `json:"deployments"`
	ReplicaSets   []rolloutSnapshot `json:"replicaSets"`
	Pods          []podSnapshot     `json:"pods"`
	HostedCluster *rolloutIdentity  `json:"hostedCluster,omitempty"`
	HCConditions  []healthCondition `json:"hostedClusterConditions,omitempty"`
	ReadStatus    map[string]string `json:"readStatus"`
}

func diagnosticIdentity(meta metav1.ObjectMeta) rolloutIdentity {
	return rolloutIdentity{Name: boundedIdentity(meta.Name), Namespace: boundedIdentity(meta.Namespace), UID: boundedIdentity(string(meta.UID)), Generation: meta.Generation, Created: meta.CreationTimestamp}
}

func boundedIdentity(value string) string {
	if len(value) > 253 {
		return value[:253]
	}
	return value
}

func rolloutOwner(meta metav1.ObjectMeta, kind string) (string, string) {
	for _, owner := range meta.OwnerReferences {
		if owner.Kind == kind && owner.Controller != nil && *owner.Controller {
			return boundedIdentity(owner.Name), boundedIdentity(string(owner.UID))
		}
	}
	return "", ""
}

func safeCondition(kind, status string, generation int64, transition string) (healthCondition, bool) {
	switch kind {
	case "Available", "Degraded", "Progressing", "ReplicaFailure", "Ready", "ContainersReady", "Initialized", "PodScheduled":
	default:
		return healthCondition{}, false
	}
	if status != "True" && status != "False" && status != "Unknown" {
		return healthCondition{}, false
	}
	if _, err := time.Parse(time.RFC3339, transition); err != nil {
		transition = ""
	}
	return healthCondition{Type: kind, Status: status, ObservedGeneration: generation, LastTransitionTime: transition}, true
}

func readStatus(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return "timeout"
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return "access-denied"
	default:
		return "read-failed"
	}
}

func WriteMCFailureDiagnostics(ctx context.Context, client kubernetes.Interface, dynamicClient dynamic.Interface, namespaces MCNamespaces) error {
	if client == nil || dynamicClient == nil || namespaces.HCPNamespace == "" || namespaces.HCNamespace == "" || namespaces.ClusterName == "" {
		return errors.New("MC diagnostics require initialized clients and resolved namespaces")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	snapshot := mcSnapshot{CapturedAt: time.Now().UTC(), ReadStatus: make(map[string]string)}
	options := metav1.ListOptions{Limit: diagnosticLimit}
	read := func(resource string, collect func(context.Context) (bool, error)) {
		requestCtx, requestCancel := context.WithTimeout(ctx, 3*time.Second)
		defer requestCancel()
		truncated, err := collect(requestCtx)
		if err != nil {
			snapshot.ReadStatus[resource] = readStatus(err)
		} else if truncated {
			snapshot.ReadStatus[resource] = "truncated"
		} else {
			snapshot.ReadStatus[resource] = "complete"
		}
	}
	read("deployments", func(requestCtx context.Context) (bool, error) {
		list, err := client.AppsV1().Deployments(namespaces.HCPNamespace).List(requestCtx, options)
		if err != nil {
			return false, err
		}
		for _, deployment := range list.Items[:min(len(list.Items), diagnosticLimit)] {
			revision, _ := strconv.ParseInt(deployment.Annotations["deployment.kubernetes.io/revision"], 10, 64)
			item := rolloutSnapshot{rolloutIdentity: diagnosticIdentity(deployment.ObjectMeta), Revision: revision, Desired: deployment.Spec.Replicas, Replicas: deployment.Status.Replicas, Ready: deployment.Status.ReadyReplicas, Available: deployment.Status.AvailableReplicas, Updated: deployment.Status.UpdatedReplicas, Unavailable: deployment.Status.UnavailableReplicas, ObservedGeneration: deployment.Status.ObservedGeneration}
			for _, condition := range deployment.Status.Conditions[:min(len(deployment.Status.Conditions), 16)] {
				if safe, ok := safeCondition(string(condition.Type), string(condition.Status), 0, condition.LastTransitionTime.Format(time.RFC3339)); ok {
					item.Conditions = append(item.Conditions, safe)
				}
			}
			snapshot.Deployments = append(snapshot.Deployments, item)
		}
		return list.Continue != "" || len(list.Items) > diagnosticLimit, nil
	})
	read("replicaSets", func(requestCtx context.Context) (bool, error) {
		list, err := client.AppsV1().ReplicaSets(namespaces.HCPNamespace).List(requestCtx, options)
		if err != nil {
			return false, err
		}
		for _, replicaSet := range list.Items[:min(len(list.Items), diagnosticLimit)] {
			revision, _ := strconv.ParseInt(replicaSet.Annotations["deployment.kubernetes.io/revision"], 10, 64)
			ownerName, ownerUID := rolloutOwner(replicaSet.ObjectMeta, "Deployment")
			snapshot.ReplicaSets = append(snapshot.ReplicaSets, rolloutSnapshot{rolloutIdentity: diagnosticIdentity(replicaSet.ObjectMeta), Revision: revision, OwnerName: ownerName, OwnerUID: ownerUID, Desired: replicaSet.Spec.Replicas, Replicas: replicaSet.Status.Replicas, Ready: replicaSet.Status.ReadyReplicas, Available: replicaSet.Status.AvailableReplicas, ObservedGeneration: replicaSet.Status.ObservedGeneration})
		}
		return list.Continue != "" || len(list.Items) > diagnosticLimit, nil
	})
	read("pods", func(requestCtx context.Context) (bool, error) {
		list, err := client.CoreV1().Pods(namespaces.HCPNamespace).List(requestCtx, options)
		if err != nil {
			return false, err
		}
		for _, pod := range list.Items[:min(len(list.Items), diagnosticLimit)] {
			ownerName, ownerUID := rolloutOwner(pod.ObjectMeta, "ReplicaSet")
			phase := "Unknown"
			switch pod.Status.Phase {
			case corev1.PodPending, corev1.PodRunning, corev1.PodSucceeded, corev1.PodFailed, corev1.PodUnknown:
				phase = string(pod.Status.Phase)
			}
			item := podSnapshot{rolloutIdentity: diagnosticIdentity(pod.ObjectMeta), OwnerName: ownerName, OwnerUID: ownerUID, Phase: phase}
			for _, condition := range pod.Status.Conditions[:min(len(pod.Status.Conditions), 16)] {
				if safe, ok := safeCondition(string(condition.Type), string(condition.Status), condition.ObservedGeneration, condition.LastTransitionTime.Format(time.RFC3339)); ok {
					item.Conditions = append(item.Conditions, safe)
				}
			}
			for _, container := range pod.Status.ContainerStatuses[:min(len(pod.Status.ContainerStatuses), 32)] {
				item.Restarts += int64(container.RestartCount)
			}
			snapshot.Pods = append(snapshot.Pods, item)
		}
		return list.Continue != "" || len(list.Items) > diagnosticLimit, nil
	})
	read("hostedCluster", func(requestCtx context.Context) (bool, error) {
		hostedCluster, err := dynamicClient.Resource(schema.GroupVersionResource{Group: "hypershift.openshift.io", Version: "v1beta1", Resource: "hostedclusters"}).Namespace(namespaces.HCNamespace).Get(requestCtx, namespaces.ClusterName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		identity := diagnosticIdentity(metav1.ObjectMeta{Name: hostedCluster.GetName(), Namespace: hostedCluster.GetNamespace(), UID: hostedCluster.GetUID(), Generation: hostedCluster.GetGeneration(), CreationTimestamp: hostedCluster.GetCreationTimestamp()})
		snapshot.HostedCluster = &identity
		conditions, _, err := unstructured.NestedSlice(hostedCluster.Object, "status", "conditions")
		if err != nil {
			return false, errors.New("invalid conditions")
		}
		seen := make(map[string]bool)
		for _, raw := range conditions[:min(len(conditions), diagnosticLimit)] {
			condition, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			kind, _, _ := unstructured.NestedString(condition, "type")
			status, _, _ := unstructured.NestedString(condition, "status")
			generation, _, _ := unstructured.NestedInt64(condition, "observedGeneration")
			transition, _, _ := unstructured.NestedString(condition, "lastTransitionTime")
			if safe, ok := safeCondition(kind, status, generation, transition); ok && !seen[kind] {
				snapshot.HCConditions = append(snapshot.HCConditions, safe)
				seen[kind] = true
			}
		}
		return len(conditions) > diagnosticLimit, nil
	})
	output, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil || len(output) > diagnosticMaxBytes {
		return errors.New("MC diagnostics serialization failed or exceeded size limit")
	}
	directory := os.Getenv("ARTIFACT_DIR")
	if directory == "" {
		directory = "/tmp"
	}
	directory = filepath.Join(directory, "mc-failure-diagnostics")
	if err := os.MkdirAll(directory, 0755); err != nil {
		return errors.New("MC diagnostics directory unavailable")
	}
	file, err := os.CreateTemp(directory, "snapshot-*.json")
	if err != nil {
		return errors.New("MC diagnostics file unavailable")
	}
	_, writeErr := file.Write(append(output, '\n'))
	modeErr := file.Chmod(0644)
	closeErr := file.Close()
	if writeErr != nil || modeErr != nil || closeErr != nil {
		_ = os.Remove(file.Name())
		return errors.New("MC diagnostics write failed")
	}
	return nil
}
