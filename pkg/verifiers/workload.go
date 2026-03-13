package verifiers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// VerifyDeploymentAvailable verifies that a deployment has Available=True condition.
func VerifyDeploymentAvailable(ctx context.Context, client kubernetes.Interface, namespace, name string) error {
	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting deployment %s/%s: %w", namespace, name, err)
	}

	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			return nil
		}
	}

	return fmt.Errorf("deployment %s/%s is not available, conditions: %v", namespace, name, deployment.Status.Conditions)
}
