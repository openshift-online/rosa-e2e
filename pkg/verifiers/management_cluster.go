package verifiers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// VerifyHyperShiftOperator verifies the HyperShift operator deployment is available.
func VerifyHyperShiftOperator(ctx context.Context, client kubernetes.Interface) error {
	return verifyDeploymentInNamespace(ctx, client, "hypershift", "operator")
}

// VerifyCAPAController verifies the CAPA controller manager deployment is available.
func VerifyCAPAController(ctx context.Context, client kubernetes.Interface) error {
	return verifyDeploymentInNamespace(ctx, client, "hypershift", "capa-controller-manager")
}

func verifyDeploymentInNamespace(ctx context.Context, client kubernetes.Interface, namespace, name string) error {
	deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting deployment %s/%s: %w", namespace, name, err)
	}

	for _, cond := range deploy.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
			return nil
		}
	}

	return fmt.Errorf("deployment %s/%s is not available (ready: %d/%d)",
		namespace, name, deploy.Status.ReadyReplicas, deploy.Status.Replicas)
}
