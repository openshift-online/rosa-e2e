package verifiers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type verifyAllNodesReady struct{}

func (v verifyAllNodesReady) Name() string {
	return "VerifyAllNodesReady"
}

func (v verifyAllNodesReady) Verify(ctx context.Context, client kubernetes.Interface) error {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}

	if len(nodes.Items) == 0 {
		return fmt.Errorf("no nodes found in the cluster")
	}

	var notReadyNodes []string
	for _, node := range nodes.Items {
		nodeIsReady := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				nodeIsReady = true
			}
		}
		if !nodeIsReady {
			notReadyNodes = append(notReadyNodes, node.Name)
		}
	}

	if len(notReadyNodes) > 0 {
		return fmt.Errorf("nodes not ready: %v", notReadyNodes)
	}

	return nil
}

// VerifyAllNodesReady returns a verifier that checks all nodes have Ready=True condition.
func VerifyAllNodesReady() ClusterVerifier {
	return verifyAllNodesReady{}
}

type verifyNodeCount struct {
	expected int
}

func (v verifyNodeCount) Name() string {
	return fmt.Sprintf("VerifyNodeCount(%d)", v.expected)
}

func (v verifyNodeCount) Verify(ctx context.Context, client kubernetes.Interface) error {
	// Count only worker nodes (exclude infra/master nodes)
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: "node-role.kubernetes.io/worker",
	})
	if err != nil {
		return fmt.Errorf("listing worker nodes: %w", err)
	}

	if len(nodes.Items) != v.expected {
		return fmt.Errorf("expected %d worker nodes, found %d", v.expected, len(nodes.Items))
	}

	return nil
}

// VerifyNodeCount returns a verifier that checks the worker node count matches expected.
func VerifyNodeCount(expected int) ClusterVerifier {
	return verifyNodeCount{expected: expected}
}
