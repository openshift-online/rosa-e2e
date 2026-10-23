package verifiers

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestVerifyWorkerNodesReady(t *testing.T) {
	ctx := context.Background()

	// Create mock nodes: 2 Ready, 1 NotReady
	node1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name": "worker-node-1",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
					},
				},
			},
		},
	}

	node2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name": "worker-node-2",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
					},
				},
			},
		},
	}

	node3 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name": "worker-node-3",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "False",
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	hcClient := dynamicfake.NewSimpleDynamicClient(scheme, node1, node2, node3)

	err := VerifyWorkerNodesReady(ctx, hcClient)
	if err == nil {
		t.Fatal("expected error for NotReady node, got nil")
	}
	if !contains(err.Error(), "worker-node-3") {
		t.Errorf("error should mention NotReady node worker-node-3, got: %v", err)
	}
}

func TestVerifyWorkerNodesReady_AllReady(t *testing.T) {
	ctx := context.Background()

	// Create mock nodes: all Ready
	node1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name": "worker-node-1",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
					},
				},
			},
		},
	}

	node2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name": "worker-node-2",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	hcClient := dynamicfake.NewSimpleDynamicClient(scheme, node1, node2)

	err := VerifyWorkerNodesReady(ctx, hcClient)
	if err != nil {
		t.Errorf("expected no error for all Ready nodes, got: %v", err)
	}
}

func TestVerifyClusterOperatorsReady(t *testing.T) {
	ctx := context.Background()

	// Create mock ClusterOperators: 1 Available, 1 Degraded
	operator1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "ClusterOperator",
			"metadata": map[string]interface{}{
				"name": "ingress",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Available",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "Degraded",
						"status": "False",
					},
					map[string]interface{}{
						"type":   "Progressing",
						"status": "False",
					},
				},
			},
		},
	}

	operator2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "ClusterOperator",
			"metadata": map[string]interface{}{
				"name": "authentication",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Available",
						"status": "False",
					},
					map[string]interface{}{
						"type":   "Degraded",
						"status": "True",
						"reason": "AuthenticationError",
					},
					map[string]interface{}{
						"type":   "Progressing",
						"status": "False",
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	hcClient := dynamicfake.NewSimpleDynamicClient(scheme, operator1, operator2)

	err := VerifyClusterOperatorsReady(ctx, hcClient)
	if err == nil {
		t.Fatal("expected error for Degraded operator, got nil")
	}
	if !contains(err.Error(), "authentication") {
		t.Errorf("error should mention degraded operator authentication, got: %v", err)
	}
}

func TestVerifyClusterOperatorsReady_AllReady(t *testing.T) {
	ctx := context.Background()

	// Create mock ClusterOperators: all Available, not Degraded, not Progressing
	operator1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "ClusterOperator",
			"metadata": map[string]interface{}{
				"name": "ingress",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Available",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "Degraded",
						"status": "False",
					},
					map[string]interface{}{
						"type":   "Progressing",
						"status": "False",
					},
				},
			},
		},
	}

	operator2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "ClusterOperator",
			"metadata": map[string]interface{}{
				"name": "authentication",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Available",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "Degraded",
						"status": "False",
					},
					map[string]interface{}{
						"type":   "Progressing",
						"status": "False",
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	hcClient := dynamicfake.NewSimpleDynamicClient(scheme, operator1, operator2)

	err := VerifyClusterOperatorsReady(ctx, hcClient)
	if err != nil {
		t.Errorf("expected no error for all ready operators, got: %v", err)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
