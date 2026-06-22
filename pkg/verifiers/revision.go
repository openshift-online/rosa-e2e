package verifiers

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const revisionAnnotation = "deployment.kubernetes.io/revision"

type deploymentRef struct {
	namespace string
	name      string
}

// VerifyDeploymentRevisionsStable checks that no deployment revisions change
// during the observation period. Revision changes without any user-initiated
// operation indicate multiple controllers fighting over the same deployment.
// Use namespace="" to check all namespaces, or pass a specific namespace.
func VerifyDeploymentRevisionsStable(ctx context.Context, client kubernetes.Interface, namespace string, observationPeriod time.Duration) error {
	initial, err := snapshotDeploymentRevisions(ctx, client, namespace)
	if err != nil {
		return fmt.Errorf("taking initial revision snapshot: %w", err)
	}

	if len(initial) == 0 {
		return nil
	}

	select {
	case <-time.After(observationPeriod):
	case <-ctx.Done():
		return ctx.Err()
	}

	final, err := snapshotDeploymentRevisions(ctx, client, namespace)
	if err != nil {
		return fmt.Errorf("taking final revision snapshot: %w", err)
	}

	var changed []string
	for ref, initialRev := range initial {
		if finalRev, ok := final[ref]; ok && initialRev != finalRev {
			changed = append(changed, fmt.Sprintf("- %s/%s: revision %s -> %s",
				ref.namespace, ref.name, initialRev, finalRev))
		}
	}

	if len(changed) > 0 {
		return fmt.Errorf("found %d deployment(s) with unexpected revision changes during %s observation:\n%s",
			len(changed), observationPeriod, strings.Join(changed, "\n"))
	}

	return nil
}

func snapshotDeploymentRevisions(ctx context.Context, client kubernetes.Interface, namespace string) (map[deploymentRef]string, error) {
	deployments, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}

	revisions := make(map[deploymentRef]string, len(deployments.Items))
	for _, d := range deployments.Items {
		ref := deploymentRef{namespace: d.Namespace, name: d.Name}
		revisions[ref] = d.Annotations[revisionAnnotation]
	}
	return revisions, nil
}
