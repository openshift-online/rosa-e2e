package verifiers

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// VerifyHCPNamespaceHealthy verifies that all deployments in the HCP namespace on the
// management cluster are healthy (Available=True and expected replicas > 0).
func VerifyHCPNamespaceHealthy(ctx context.Context, mcClient kubernetes.Interface, hcpNamespace string) error {
	deployments, err := mcClient.AppsV1().Deployments(hcpNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing deployments in namespace %s: %w", hcpNamespace, err)
	}

	if len(deployments.Items) == 0 {
		return fmt.Errorf("no deployments found in namespace %s", hcpNamespace)
	}

	var unhealthyDeployments []string
	for _, deploy := range deployments.Items {
		// Skip deployments scaled to 0
		if deploy.Spec.Replicas != nil && *deploy.Spec.Replicas == 0 {
			continue
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
			unhealthyDeployments = append(unhealthyDeployments,
				fmt.Sprintf("- %s: not Available (ready: %d/%d)",
					deploy.Name, deploy.Status.ReadyReplicas, deploy.Status.Replicas))
		}
	}

	if len(unhealthyDeployments) > 0 {
		return fmt.Errorf("found %d unhealthy deployments in namespace %s:\n%s",
			len(unhealthyDeployments), hcpNamespace, strings.Join(unhealthyDeployments, "\n"))
	}

	return nil
}
