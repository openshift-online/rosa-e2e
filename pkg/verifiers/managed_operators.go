package verifiers

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	routeMonitorGVR = schema.GroupVersionResource{
		Group:    "monitoring.openshift.io",
		Version:  "v1alpha1",
		Resource: "routemonitors",
	}
	vpcEndpointGVR = schema.GroupVersionResource{
		Group:    "avo.openshift.io",
		Version:  "v1alpha2",
		Resource: "vpcendpoints",
	}
)

// VerifyRMORouteMonitors verifies that at least one RouteMonitor CR exists in the namespace.
func VerifyRMORouteMonitors(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	routeMonitors, err := dynamicClient.Resource(routeMonitorGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing RouteMonitors in namespace %s: %w", namespace, err)
	}

	if len(routeMonitors.Items) == 0 {
		return fmt.Errorf("no RouteMonitor CRs found in namespace %s", namespace)
	}

	return nil
}

// VerifyAVOVpcEndpoints verifies that all VpcEndpoint CRs have an available condition with status True.
func VerifyAVOVpcEndpoints(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	vpcEndpoints, err := dynamicClient.Resource(vpcEndpointGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing VpcEndpoints in namespace %s: %w", namespace, err)
	}

	if len(vpcEndpoints.Items) == 0 {
		return fmt.Errorf("no VpcEndpoint CRs found in namespace %s", namespace)
	}

	var unavailable []string
	for _, vpe := range vpcEndpoints.Items {
		name := vpe.GetName()

		conditions, found, err := unstructured.NestedSlice(vpe.Object, "status", "conditions")
		if err != nil {
			return fmt.Errorf("extracting conditions from VpcEndpoint %s: %w", name, err)
		}
		if !found {
			unavailable = append(unavailable, fmt.Sprintf("- %s: no status conditions", name))
			continue
		}

		availableCondFound := false
		for _, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}

			condType, _ := condMap["type"].(string)
			condStatus, _ := condMap["status"].(string)

			if strings.EqualFold(condType, "available") {
				availableCondFound = true
				if strings.EqualFold(condStatus, "true") {
					// VpcEndpoint is available
					break
				} else {
					reason, _ := condMap["reason"].(string)
					message, _ := condMap["message"].(string)
					unavailable = append(unavailable, fmt.Sprintf("- %s: available=%s (reason: %s, message: %s)",
						name, condStatus, reason, message))
				}
			}
		}

		if !availableCondFound {
			unavailable = append(unavailable, fmt.Sprintf("- %s: available condition not found", name))
		}
	}

	if len(unavailable) > 0 {
		return fmt.Errorf("found %d unavailable VpcEndpoints in namespace %s:\n%s",
			len(unavailable), namespace, strings.Join(unavailable, "\n"))
	}

	return nil
}

// VerifyAuditWebhook verifies that the audit-webhook deployment exists and has Available condition True.
func VerifyAuditWebhook(ctx context.Context, client kubernetes.Interface, namespace string) error {
	deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, "audit-webhook", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting audit-webhook deployment in namespace %s: %w", namespace, err)
	}

	// Check if deployment is available
	available := false
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == "Available" && cond.Status == "True" {
			available = true
			break
		}
	}

	if !available {
		return fmt.Errorf("audit-webhook deployment in namespace %s is not Available (ready: %d/%d)",
			namespace, deploy.Status.ReadyReplicas, deploy.Status.Replicas)
	}

	return nil
}
