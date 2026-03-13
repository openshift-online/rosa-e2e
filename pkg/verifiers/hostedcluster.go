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
)

// VerifyHostedClusterHealthy verifies that a HostedCluster CR has Available=True and Degraded=False.
func VerifyHostedClusterHealthy(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) error {
	hc, err := dynamicClient.Resource(hostedClusterGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting HostedCluster %s/%s: %w", namespace, name, err)
	}

	return verifyConditions(hc, "HostedCluster", namespace, name)
}

// VerifyNodePoolHealthy verifies that a NodePool CR has Available=True and Degraded=False.
func VerifyNodePoolHealthy(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) error {
	np, err := dynamicClient.Resource(nodePoolGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting NodePool %s/%s: %w", namespace, name, err)
	}

	return verifyConditions(np, "NodePool", namespace, name)
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
