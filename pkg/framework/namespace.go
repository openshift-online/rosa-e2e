package framework

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultServiceAccountPollInterval = 2 * time.Second
	defaultServiceAccountTimeout      = time.Minute
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

// waitForDefaultServiceAccount polls until the namespace's default ServiceAccount exists. Only
// NotFound is retried: any other error is returned immediately so authorization and API failures
// stay visible instead of being masked until the deadline.
func waitForDefaultServiceAccount(ctx context.Context, client kubernetes.Interface, namespace string, interval, timeout time.Duration) error {
	var getErr error
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		_, getErr = client.CoreV1().ServiceAccounts(namespace).Get(ctx, "default", metav1.GetOptions{})
		if errors.IsNotFound(getErr) {
			return false, nil
		}
		return getErr == nil, getErr
	})
	switch {
	case err == nil:
		return nil
	case errors.IsNotFound(getErr):
		return fmt.Errorf("waiting for default ServiceAccount in namespace %s (timeout %s): still not found: %w", namespace, timeout, err)
	case stderrors.Is(err, context.DeadlineExceeded) || stderrors.Is(err, context.Canceled):
		// The deadline or cancellation interrupted an in-flight GET.
		return fmt.Errorf("waiting for default ServiceAccount in namespace %s (timeout %s): %w", namespace, timeout, err)
	default:
		return fmt.Errorf("getting default ServiceAccount in namespace %s: %w", namespace, err)
	}
}

// CreateTestNamespace creates a namespace with retry to handle transient API errors (e.g. DNS
// timeouts in CI). It polls for up to 2 minutes with 10-second intervals and treats
// AlreadyExists as success. Non-transient errors (Forbidden, Invalid, Unauthorized) cause an
// immediate failure without retry. It then waits up to 1 minute for the namespace's default
// ServiceAccount, retrying only NotFound.
//
// The returned cleanup function deletes the namespace. It is nil only when the namespace could
// not be created; when the ServiceAccount wait fails it is returned together with the error so
// callers can still remove the namespace and capture diagnostics first.
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

	// Wait for the default ServiceAccount: pod admission rejects pods referencing a
	// service account that does not exist yet, and the controller creates the default
	// one asynchronously after namespace creation.
	err = waitForDefaultServiceAccount(ctx, client, name, defaultServiceAccountPollInterval, defaultServiceAccountTimeout)
	return cleanup, err
}
