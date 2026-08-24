package framework

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// isNonTransientError returns true for API errors that will not resolve on retry.
func isNonTransientError(err error) bool {
	return errors.IsForbidden(err) ||
		errors.IsInvalid(err) ||
		errors.IsMethodNotSupported(err) ||
		errors.IsNotAcceptable(err) ||
		errors.IsResourceExpired(err) ||
		errors.IsUnauthorized(err)
}

// CreateTestNamespace creates a namespace with retry to handle transient API errors (e.g. DNS
// timeouts in CI). It polls for up to 2 minutes with 10-second intervals and treats
// AlreadyExists as success. Non-transient errors (Forbidden, Invalid, Unauthorized) cause an
// immediate failure without retry. Returns a cleanup function that deletes the namespace.
func CreateTestNamespace(ctx context.Context, client kubernetes.Interface, name string) (cleanup func() error, err error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"e2e-test":   "true",
				"created-by": "rosa-e2e",
			},
		},
	}

	err = wait.PollUntilContextTimeout(ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		_, createErr := client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if createErr == nil || errors.IsAlreadyExists(createErr) {
			return true, nil
		}
		if isNonTransientError(createErr) {
			return false, createErr
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("creating namespace %s: %w", name, err)
	}

	cleanup = func() error {
		if deleteErr := client.CoreV1().Namespaces().Delete(context.Background(), name, metav1.DeleteOptions{}); deleteErr != nil && !errors.IsNotFound(deleteErr) {
			return fmt.Errorf("deleting test namespace %s: %w", name, deleteErr)
		}
		return nil
	}

	return cleanup, nil
}
