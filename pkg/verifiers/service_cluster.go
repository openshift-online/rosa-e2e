package verifiers

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// VerifyACMHub verifies at least one MultiClusterHub CR exists and is Running.
func VerifyACMHub(ctx context.Context, dynamicClient dynamic.Interface) error {
	gvr := schema.GroupVersionResource{
		Group:    "operator.open-cluster-management.io",
		Version:  "v1",
		Resource: "multiclusterhubs",
	}

	list, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing multiclusterhubs: %w", err)
	}

	if len(list.Items) == 0 {
		return fmt.Errorf("no multiclusterhub CRs found")
	}

	// Check if at least one hub is running
	for _, hub := range list.Items {
		status, found, err := getNestedField(hub.Object, "status", "phase")
		if err != nil || !found {
			continue
		}
		if status == "Running" {
			return nil
		}
	}

	return fmt.Errorf("no multiclusterhub CR has status.phase = Running")
}

// VerifyCertManager verifies the cert-manager deployment is available.
func VerifyCertManager(ctx context.Context, client kubernetes.Interface) error {
	return verifyDeploymentInNamespace(ctx, client, "cert-manager", "cert-manager")
}

// VerifyHiveControllers verifies the hive-controllers deployment is available.
func VerifyHiveControllers(ctx context.Context, client kubernetes.Interface) error {
	return verifyDeploymentInNamespace(ctx, client, "hive", "hive-controllers")
}

// VerifyMCE verifies at least one MultiClusterEngine CR exists and is Available.
func VerifyMCE(ctx context.Context, dynamicClient dynamic.Interface) error {
	gvr := schema.GroupVersionResource{
		Group:    "multicluster.openshift.io",
		Version:  "v1",
		Resource: "multiclusterengines",
	}

	list, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing multiclusterengines: %w", err)
	}

	if len(list.Items) == 0 {
		return fmt.Errorf("no multiclusterengine CRs found")
	}

	// Check if at least one MCE is available
	for _, mce := range list.Items {
		status, found, err := getNestedField(mce.Object, "status", "phase")
		if err != nil || !found {
			continue
		}
		if status == "Available" {
			return nil
		}
	}

	return fmt.Errorf("no multiclusterengine CR has status.phase = Available")
}

// getNestedField safely retrieves a nested field from an unstructured object.
func getNestedField(obj map[string]interface{}, fields ...string) (string, bool, error) {
	val, found, err := getNestedValue(obj, fields...)
	if !found || err != nil {
		return "", found, err
	}
	str, ok := val.(string)
	if !ok {
		return "", false, fmt.Errorf("field %v is not a string", fields)
	}
	return str, true, nil
}

// getNestedValue safely retrieves a nested value from an unstructured object.
func getNestedValue(obj map[string]interface{}, fields ...string) (interface{}, bool, error) {
	current := obj
	for i, field := range fields {
		if i == len(fields)-1 {
			val, found := current[field]
			return val, found, nil
		}
		next, found := current[field]
		if !found {
			return nil, false, nil
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("field %s is not a map", field)
		}
		current = nextMap
	}
	return nil, false, nil
}
