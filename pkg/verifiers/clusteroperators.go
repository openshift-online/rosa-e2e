package verifiers

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// VerifyClusterOperatorsHealthy verifies that all ClusterOperators have Available=True and Degraded=False.
func VerifyClusterOperatorsHealthy(ctx context.Context, dynamicClient dynamic.Interface, excludeOperators ...string) error {
	// Build exclusion set
	excluded := make(map[string]bool)
	for _, op := range excludeOperators {
		excluded[strings.TrimSpace(op)] = true
	}

	gvr := schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusteroperators",
	}

	unstructuredList, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing clusteroperators: %w", err)
	}

	if len(unstructuredList.Items) == 0 {
		return fmt.Errorf("no clusteroperators found")
	}

	var unhealthy []string
	for _, item := range unstructuredList.Items {
		name := item.GetName()
		if excluded[name] {
			continue
		}
		conditions, found, err := getConditions(item.Object)
		if err != nil {
			return fmt.Errorf("getting conditions for %s: %w", name, err)
		}
		if !found {
			unhealthy = append(unhealthy, fmt.Sprintf("%s: no status.conditions found", name))
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

			if condType == "Available" || condType == "Degraded" || condType == "Progressing" {
				conditionDetails = append(conditionDetails, fmt.Sprintf("%s=%s", condType, condStatus))
			}
		}

		if !available || degraded {
			unhealthy = append(unhealthy, fmt.Sprintf("%s: %v", name, conditionDetails))
		}
	}

	if len(unhealthy) > 0 {
		return fmt.Errorf("unhealthy clusteroperators found:\n  %v", unhealthy)
	}

	return nil
}

// getConditions extracts the status.conditions array from an unstructured object.
func getConditions(obj map[string]interface{}) ([]map[string]interface{}, bool, error) {
	status, found := obj["status"].(map[string]interface{})
	if !found {
		return nil, false, nil
	}

	conditions, found := status["conditions"].([]interface{})
	if !found {
		return nil, false, nil
	}

	var result []map[string]interface{}
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("condition is not a map")
		}
		result = append(result, condMap)
	}

	return result, true, nil
}
