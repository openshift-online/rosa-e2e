package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

var diagnosticNamespaces = MCNamespaces{HCPNamespace: "clusters-example", HCNamespace: "clusters", ClusterName: "example"}

func readDiagnosticFile(t *testing.T, directory string) (mcSnapshot, string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(directory, "mc-failure-diagnostics", "snapshot-*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("expected one artifact: %v, %v", paths, err)
	}
	content, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(content) > diagnosticMaxBytes+1 {
		t.Fatal("artifact exceeds size limit")
	}
	info, err := os.Stat(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("unexpected artifact permissions: %v", info.Mode())
	}
	var snapshot mcSnapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot, string(content)
}

func TestMCFailureDiagnosticsSafeArtifact(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("ARTIFACT_DIR", filepath.Join(directory, "artifacts"))
	const sensitive = "SENSITIVE_SENTINEL"
	meta := metav1.ObjectMeta{Name: "catalog", Namespace: diagnosticNamespaces.HCPNamespace, UID: types.UID("deployment-id"), Generation: 4, Annotations: map[string]string{"deployment.kubernetes.io/revision": "4", "private": sensitive}, Labels: map[string]string{"private": sensitive}}
	deployment := &appsv1.Deployment{ObjectMeta: meta, Spec: appsv1.DeploymentSpec{Replicas: ptr.To(int32(1)), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "catalog", Image: sensitive, Args: []string{sensitive}, Env: []corev1.EnvVar{{Name: "TOKEN", Value: sensitive}}}}}}}, Status: appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1, UnavailableReplicas: 1, ObservedGeneration: 4, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse, Reason: sensitive, Message: sensitive}}}}
	replicaSet := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "catalog-new", Namespace: meta.Namespace, UID: "rs-id", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: meta.Name, UID: meta.UID, Controller: ptr.To(true)}}, Annotations: meta.Annotations}, Spec: appsv1.ReplicaSetSpec{Replicas: ptr.To(int32(1))}, Status: appsv1.ReplicaSetStatus{Replicas: 1}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "catalog-new-pod", Namespace: meta.Namespace, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: replicaSet.Name, UID: replicaSet.UID, Controller: ptr.To(true)}}}, Spec: deployment.Spec.Template.Spec, Status: corev1.PodStatus{Phase: corev1.PodPending, Message: sensitive, Reason: sensitive, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Message: sensitive, Reason: sensitive}}, ContainerStatuses: []corev1.ContainerStatus{{Name: "catalog", Image: sensitive, ImageID: sensitive, RestartCount: 2, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: sensitive, Message: sensitive}}}}}}
	hostedCluster := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "hypershift.openshift.io/v1beta1", "kind": "HostedCluster", "metadata": map[string]interface{}{"name": "example", "namespace": "clusters", "annotations": map[string]interface{}{"private": sensitive}}, "spec": map[string]interface{}{"secret": sensitive}, "status": map[string]interface{}{"private": sensitive, "conditions": []interface{}{map[string]interface{}{"type": "Degraded", "status": "True", "reason": sensitive, "message": sensitive, "observedGeneration": int64(4)}, map[string]interface{}{"type": sensitive, "status": "True"}, map[string]interface{}{"type": "Available", "status": sensitive}}}}}
	client := fake.NewClientset(deployment, replicaSet, pod)
	conditions, _, _ := unstructured.NestedSlice(hostedCluster.Object, "status", "conditions")
	for index := 0; index < 20; index++ {
		conditions = append([]interface{}{map[string]interface{}{"type": sensitive, "status": "True"}}, conditions...)
	}
	if err := unstructured.SetNestedSlice(hostedCluster.Object, conditions, "status", "conditions"); err != nil {
		t.Fatal(err)
	}
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), hostedCluster)
	if err := WriteMCFailureDiagnostics(context.Background(), client, dyn, diagnosticNamespaces); err != nil {
		t.Fatal(err)
	}
	snapshot, content := readDiagnosticFile(t, filepath.Join(directory, "artifacts"))
	if strings.Contains(content, sensitive) {
		t.Fatal("sensitive field leaked")
	}
	for _, forbidden := range []string{"annotations", "spec", "env", "args", "message", "reason", "Image", "secret"} {
		if strings.Contains(content, `"`+forbidden+`"`) {
			t.Fatalf("forbidden field %s", forbidden)
		}
	}
	if len(snapshot.Deployments) != 1 || snapshot.Deployments[0].Revision != 4 || snapshot.Deployments[0].Unavailable != 1 || snapshot.Deployments[0].ObservedGeneration != 4 {
		t.Fatalf("missing deployment evidence: %+v", snapshot.Deployments)
	}
	if len(snapshot.ReplicaSets) != 1 || snapshot.ReplicaSets[0].OwnerUID != "deployment-id" || len(snapshot.Pods) != 1 || snapshot.Pods[0].OwnerUID != "rs-id" || snapshot.Pods[0].Restarts != 2 {
		t.Fatal("missing rollout identity/status")
	}
	if len(snapshot.HCConditions) != 1 || snapshot.HCConditions[0].Type != "Degraded" || snapshot.HCConditions[0].Status != "True" {
		t.Fatal("missing HC health")
	}
	for _, action := range client.Actions() {
		if action.GetVerb() != "list" || action.GetNamespace() != diagnosticNamespaces.HCPNamespace {
			t.Fatalf("unexpected access: %v", action)
		}
	}
	for _, action := range dyn.Actions() {
		if action.GetVerb() != "get" || action.GetNamespace() != diagnosticNamespaces.HCNamespace || action.GetResource().Resource != "hostedclusters" {
			t.Fatalf("unexpected dynamic access: %v", action)
		}
	}
}

func TestMCFailureDiagnosticsPartialErrorsAndLimits(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", t.TempDir())
	client := fake.NewClientset()
	client.PrependReactor("list", "deployments", func(clienttesting.Action) (bool, runtime.Object, error) {
		list := &appsv1.DeploymentList{}
		for index := 0; index < diagnosticLimit+1; index++ {
			list.Items = append(list.Items, appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("deployment-%d", index)}})
		}
		return true, list, nil
	})
	client.PrependReactor("list", "replicasets", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "replicasets"}, "private", errors.New("SENSITIVE_ERROR"))
	})
	client.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("SENSITIVE_ERROR")
	})
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if err := WriteMCFailureDiagnostics(context.Background(), client, dyn, diagnosticNamespaces); err != nil {
		t.Fatal(err)
	}
	snapshot, content := readDiagnosticFile(t, os.Getenv("ARTIFACT_DIR"))
	if len(snapshot.Deployments) != diagnosticLimit || snapshot.ReadStatus["deployments"] != "truncated" || snapshot.ReadStatus["replicaSets"] != "access-denied" || snapshot.ReadStatus["pods"] != "read-failed" || snapshot.ReadStatus["hostedCluster"] != "read-failed" {
		t.Fatalf("unexpected partial result: %+v", snapshot.ReadStatus)
	}
	if strings.Contains(content, "SENSITIVE_ERROR") {
		t.Fatal("raw error leaked")
	}
}

func TestMCFailureDiagnosticsTimeout(t *testing.T) {
	for _, budget := range []time.Duration{50 * time.Millisecond, 5 * time.Second} {
		t.Run(budget.String(), func(t *testing.T) {
			t.Setenv("ARTIFACT_DIR", t.TempDir())
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if strings.HasSuffix(request.URL.Path, "/deployments") {
					if request.URL.Query().Get("limit") != "100" {
						t.Error("missing API list limit")
					}
					<-request.Context().Done()
					return
				}
				http.Error(writer, "SENSITIVE_ERROR", http.StatusForbidden)
			}))
			defer server.Close()
			config := &rest.Config{Host: server.URL}
			client, err := kubernetes.NewForConfig(config)
			if err != nil {
				t.Fatal(err)
			}
			dyn, err := dynamic.NewForConfig(config)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			defer cancel()
			started := time.Now()
			if err := WriteMCFailureDiagnostics(ctx, client, dyn, diagnosticNamespaces); err != nil {
				t.Fatal(err)
			}
			if time.Since(started) > min(budget+time.Second, 4*time.Second) {
				t.Fatal("diagnostics ignored parent or per-request deadline")
			}
			snapshot, _ := readDiagnosticFile(t, os.Getenv("ARTIFACT_DIR"))
			if snapshot.ReadStatus["deployments"] != "timeout" {
				t.Fatalf("deployment read: %s", snapshot.ReadStatus["deployments"])
			}
		})
	}
}

func TestMCFailureDiagnosticsMissingScopeAndWriteError(t *testing.T) {
	client := fake.NewClientset()
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	if err := WriteMCFailureDiagnostics(context.Background(), client, dyn, MCNamespaces{}); err == nil {
		t.Fatal("empty scope accepted")
	}
	if len(client.Actions()) != 0 || len(dyn.Actions()) != 0 {
		t.Fatal("missing scope caused API calls")
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARTIFACT_DIR", file)
	if err := WriteMCFailureDiagnostics(context.Background(), client, dyn, diagnosticNamespaces); err == nil {
		t.Fatal("write error ignored")
	}
}

func TestNumericRevision(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		input string
		want  int64
	}{{"4", 4}, {"0", 0}, {"", 0}, {"-1", 0}, {"+4", 0}, {"private", 0}, {"9223372036854775808", 0}, {strings.Repeat("9", 1000), 0}} {
		t.Run(test.input[:min(len(test.input), 20)], func(t *testing.T) {
			if got := numericRevision(test.input); got != test.want {
				t.Fatalf("got %d want %d", got, test.want)
			}
		})
	}
}

func TestDiagnosticWriteFailureRemovesPartialFile(t *testing.T) {
	t.Parallel()
	file, err := os.CreateTemp(t.TempDir(), "snapshot-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writeDiagnosticFile(file, []byte("{}")); err == nil || err.Error() != "MC diagnostics write failed" {
		t.Fatalf("unexpected write error: %v", err)
	}
	if _, err := os.Stat(file.Name()); !os.IsNotExist(err) {
		t.Fatalf("partial artifact retained: %v", err)
	}
}

func TestMCFailureDiagnosticsByteLimit(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", t.TempDir())
	client := fake.NewClientset()
	client.PrependReactor("list", "deployments", func(clienttesting.Action) (bool, runtime.Object, error) {
		list := &appsv1.DeploymentList{}
		for index := 0; index < diagnosticLimit; index++ {
			item := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("a", 253), Namespace: diagnosticNamespaces.HCPNamespace, UID: types.UID(strings.Repeat("b", 253))}}
			for condition := 0; condition < 16; condition++ {
				item.Status.Conditions = append(item.Status.Conditions, appsv1.DeploymentCondition{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now()})
			}
			list.Items = append(list.Items, item)
		}
		return true, list, nil
	})
	client.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		list := &corev1.PodList{}
		for index := 0; index < diagnosticLimit; index++ {
			item := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: strings.Repeat("a", 253), Namespace: diagnosticNamespaces.HCPNamespace}}
			for condition := 0; condition < 16; condition++ {
				item.Status.Conditions = append(item.Status.Conditions, corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now()})
			}
			list.Items = append(list.Items, item)
		}
		return true, list, nil
	})
	if err := WriteMCFailureDiagnostics(context.Background(), client, dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), diagnosticNamespaces); err != nil {
		t.Fatal(err)
	}
	snapshot, content := readDiagnosticFile(t, os.Getenv("ARTIFACT_DIR"))
	if len(content) > diagnosticMaxBytes || snapshot.ReadStatus["pods"] != "truncated" || len(snapshot.Deployments) != diagnosticLimit {
		t.Fatalf("byte truncation lost priority or status: bytes=%d status=%v deployments=%d", len(content), snapshot.ReadStatus, len(snapshot.Deployments))
	}
}

func TestMCFailureDiagnosticsTotalDeadline(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { <-request.Context().Done() }))
	defer server.Close()
	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	dyn, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := WriteMCFailureDiagnostics(context.Background(), client, dyn, diagnosticNamespaces); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 11*time.Second || elapsed > 14*time.Second {
		t.Fatalf("unexpected total deadline: %s", elapsed)
	}
	snapshot, _ := readDiagnosticFile(t, os.Getenv("ARTIFACT_DIR"))
	for resource, status := range snapshot.ReadStatus {
		if status != "timeout" {
			t.Errorf("%s status=%s", resource, status)
		}
	}
}
