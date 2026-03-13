package verifiers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"k8s.io/client-go/kubernetes"
)

// ClusterVerifier defines a composable cluster health check.
type ClusterVerifier interface {
	Name() string
	Verify(ctx context.Context, client kubernetes.Interface) error
}

// RunVerifiers executes all verifiers in parallel and returns aggregated errors.
func RunVerifiers(ctx context.Context, client kubernetes.Interface, verifiers ...ClusterVerifier) error {
	errCh := make(chan error, len(verifiers))
	wg := sync.WaitGroup{}

	for _, v := range verifiers {
		wg.Add(1)
		go func(verifier ClusterVerifier) {
			defer wg.Done()
			if err := verifier.Verify(ctx, client); err != nil {
				errCh <- fmt.Errorf("%s failed: %w", verifier.Name(), err)
			}
		}(v)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
