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

// CreateTestNamespace creates a namespace with retry to handle transient API errors (e.g. DNS
// timeouts in CI). It polls for up to 2 minutes with 10-second intervals and treats
// AlreadyExists as success. Returns a cleanup function that deletes the namespace.
func CreateTestNamespace(ctx context.Context, client kubernetes.Interface, name string) (cleanup func(), err error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}

	err = wait.PollUntilContextTimeout(ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		_, createErr := client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if createErr == nil || errors.IsAlreadyExists(createErr) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("creating namespace %s: %w", name, err)
	}

	cleanup = func() {
		_ = client.CoreV1().Namespaces().Delete(context.Background(), name, metav1.DeleteOptions{})
	}

	return cleanup, nil
}
