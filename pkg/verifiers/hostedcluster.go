package verifiers

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

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
	nodeGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "nodes",
	}
	clusterOperatorGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusteroperators",
	}
)

// VerifyHostedClusterHealthy verifies that a HostedCluster CR has Available=True and Degraded=False.
func VerifyHostedClusterHealthy(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) error {
	hc, err := dynamicClient.Resource(hostedClusterGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting HostedCluster %s/%s: %w", namespace, name, err)
	}

	return verifyConditions(hc, "HostedCluster", namespace, name)
}

// VerifyNodePoolHealthy verifies that all NodePool CRs in a namespace are healthy.
// NodePools use Ready and AllNodesHealthy conditions (not Available/Degraded like HostedClusters).
func VerifyNodePoolHealthy(ctx context.Context, dynamicClient dynamic.Interface, namespace, _ string) error {
	list, err := dynamicClient.Resource(nodePoolGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing NodePools in %s: %w", namespace, err)
	}

	if len(list.Items) == 0 {
		return fmt.Errorf("no NodePools found in namespace %s", namespace)
	}

	var unhealthy []string
	for _, np := range list.Items {
		name := np.GetName()
		conditions, found, err := getConditions(np.Object)
		if err != nil || !found {
			unhealthy = append(unhealthy, fmt.Sprintf("NodePool %s/%s: no conditions found", namespace, name))
			continue
		}

		ready := false
		nodesHealthy := false
		var details []string

		for _, cond := range conditions {
			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)

			if condType == "Ready" {
				if condStatus == "True" {
					ready = true
				}
				details = append(details, fmt.Sprintf("Ready=%s", condStatus))
			}
			if condType == "AllNodesHealthy" {
				if condStatus == "True" {
					nodesHealthy = true
				}
				details = append(details, fmt.Sprintf("AllNodesHealthy=%s", condStatus))
			}
		}

		if !ready || !nodesHealthy {
			unhealthy = append(unhealthy, fmt.Sprintf("NodePool %s/%s: %v", namespace, name, details))
		}
	}

	if len(unhealthy) > 0 {
		return fmt.Errorf("%d unhealthy NodePools:\n%s", len(unhealthy), strings.Join(unhealthy, "\n"))
	}

	return nil
}

func verifyConditions(obj *unstructured.Unstructured, kind, namespace, name string) error {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return fmt.Errorf("extracting conditions from %s %s/%s: %w", kind, namespace, name, err)
	}
	if !found {
		return fmt.Errorf("%s %s/%s has no status conditions", kind, namespace, name)
	}

	var issues []string
	availableFound := false
	degradedFound := false

	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _ := condMap["type"].(string)
		condStatus, _ := condMap["status"].(string)
		condReason, _ := condMap["reason"].(string)
		condMessage, _ := condMap["message"].(string)

		if condType == "Available" {
			availableFound = true
			if condStatus != "True" {
				issues = append(issues, fmt.Sprintf("Available=%s (reason: %s, message: %s)",
					condStatus, condReason, condMessage))
			}
		}

		if condType == "Degraded" {
			degradedFound = true
			if condStatus != "False" {
				issues = append(issues, fmt.Sprintf("Degraded=%s (reason: %s, message: %s)",
					condStatus, condReason, condMessage))
			}
		}
	}

	if !availableFound {
		issues = append(issues, "Available condition not found")
	}
	if !degradedFound {
		issues = append(issues, "Degraded condition not found")
	}

	if len(issues) > 0 {
		return fmt.Errorf("%s %s/%s is not healthy:\n%s",
			kind, namespace, name, strings.Join(issues, "\n"))
	}

	return nil
}

// VerifyWorkerNodesReady verifies that all worker nodes in the hosted cluster are in Ready state.
// Uses dynamic client to parse unstructured Node objects without importing k8s.io/api/core types.
func VerifyWorkerNodesReady(ctx context.Context, hcClient dynamic.Interface) error {
	nodeList, err := hcClient.Resource(nodeGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}

	if len(nodeList.Items) == 0 {
		return fmt.Errorf("no nodes found in the cluster")
	}

	var notReadyNodes []string
	for _, node := range nodeList.Items {
		nodeName := node.GetName()
		conditions, found, err := getConditions(node.Object)
		if err != nil {
			return fmt.Errorf("getting conditions for node %s: %w", nodeName, err)
		}
		if !found {
			notReadyNodes = append(notReadyNodes, fmt.Sprintf("%s (no conditions found)", nodeName))
			continue
		}

		nodeIsReady := false
		var conditionDetails []string
		for _, cond := range conditions {
			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)

			if condType == "Ready" {
				if condStatus == "True" {
					nodeIsReady = true
				}
				conditionDetails = append(conditionDetails, fmt.Sprintf("Ready=%s", condStatus))
			}
		}

		if !nodeIsReady {
			details := strings.Join(conditionDetails, ", ")
			if details == "" {
				details = "Ready condition not found"
			}
			notReadyNodes = append(notReadyNodes, fmt.Sprintf("%s (%s)", nodeName, details))
		}
	}

	if len(notReadyNodes) > 0 {
		return fmt.Errorf("nodes not ready:\n  %s", strings.Join(notReadyNodes, "\n  "))
	}

	return nil
}

// VerifyClusterOperatorsReady verifies that all ClusterOperators are Available and not Degraded.
// Tolerates Progressing=True (operators may take time to converge).
// Uses dynamic client to parse unstructured ClusterOperator objects.
func VerifyClusterOperatorsReady(ctx context.Context, hcClient dynamic.Interface) error {
	operatorList, err := hcClient.Resource(clusterOperatorGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing clusteroperators: %w", err)
	}

	if len(operatorList.Items) == 0 {
		return fmt.Errorf("no clusteroperators found")
	}

	var unhealthyOperators []string
	for _, operator := range operatorList.Items {
		operatorName := operator.GetName()
		conditions, found, err := getConditions(operator.Object)
		if err != nil {
			return fmt.Errorf("getting conditions for operator %s: %w", operatorName, err)
		}
		if !found {
			unhealthyOperators = append(unhealthyOperators, fmt.Sprintf("%s: no conditions found", operatorName))
			continue
		}

		available := false
		degraded := true
		var conditionDetails []string

		for _, cond := range conditions {
			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)

			if condType == "Available" && condStatus == "True" {
				available = true
			}
			if condType == "Degraded" && condStatus == "False" {
				degraded = false
			}

			// Collect details for Available, Degraded, and Progressing conditions
			if condType == "Available" || condType == "Degraded" || condType == "Progressing" {
				conditionDetails = append(conditionDetails, fmt.Sprintf("%s=%s", condType, condStatus))
			}
		}

		if !available || degraded {
			unhealthyOperators = append(unhealthyOperators, fmt.Sprintf("%s: %s", operatorName, strings.Join(conditionDetails, ", ")))
		}
	}

	if len(unhealthyOperators) > 0 {
		return fmt.Errorf("unhealthy clusteroperators found:\n  %s", strings.Join(unhealthyOperators, "\n  "))
	}

	return nil
}
