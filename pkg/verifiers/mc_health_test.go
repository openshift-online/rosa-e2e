package verifiers

import (
	"context"
	"fmt"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestMCDeploymentRevisionChangeStillFails(t *testing.T) {
	for _, name := range []string{"redhat-operators-catalog", "certified-operators-catalog", "community-operators-catalog", "redhat-marketplace-catalog", "kube-apiserver"} {
		for _, changed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/changed=%v", name, changed), func(t *testing.T) {
				client := fake.NewClientset()
				revisions := []string{"3", "4"}
				if !changed {
					revisions[1] = "3"
				}
				client.PrependReactor("list", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
					if action.GetNamespace() != "hcp" {
						t.Fatal("unexpected namespace")
					}
					revision := revisions[0]
					revisions = revisions[1:]
					return true, &appsv1.DeploymentList{Items: []appsv1.Deployment{{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "hcp", Annotations: map[string]string{revisionAnnotation: revision}}}}}, nil
				})
				err := VerifyDeploymentRevisionsStable(context.Background(), client, "hcp", 0)
				if changed && (err == nil || !strings.Contains(err.Error(), name+": revision 3 -> 4")) {
					t.Fatalf("revision change must fail: %v", err)
				}
				if !changed && err != nil {
					t.Fatalf("stable revision must pass: %v", err)
				}
			})
		}
	}
}

func TestMCDegradedHostedClusterStillFails(t *testing.T) {
	hostedCluster := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "hypershift.openshift.io/v1beta1", "kind": "HostedCluster",
		"metadata": map[string]interface{}{"name": "example", "namespace": "clusters"},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Available", "status": "True"},
			map[string]interface{}{"type": "Degraded", "status": "True", "reason": "UnavailableReplicas", "message": "unavailable replicas: 1"},
		}},
	}}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), hostedCluster)
	err := VerifyHostedClusterHealthy(context.Background(), client, "clusters", "example")
	if err == nil || !strings.Contains(err.Error(), "Degraded") {
		t.Fatalf("degraded HC must fail: %v", err)
	}
}
