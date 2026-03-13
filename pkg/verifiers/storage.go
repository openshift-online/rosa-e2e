package verifiers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// VerifyPVCBound verifies that a PVC is in the Bound phase.
func VerifyPVCBound(ctx context.Context, client kubernetes.Interface, namespace, pvcName string) error {
	pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting PVC %s/%s: %w", namespace, pvcName, err)
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		return fmt.Errorf("PVC %s/%s is not bound, phase: %s", namespace, pvcName, pvc.Status.Phase)
	}

	return nil
}

// VerifyVolumeSnapshotReady verifies that a VolumeSnapshot has readyToUse=true.
func VerifyVolumeSnapshotReady(ctx context.Context, dynamicClient dynamic.Interface, namespace, snapshotName string) error {
	gvr := schema.GroupVersionResource{
		Group:    "snapshot.storage.k8s.io",
		Version:  "v1",
		Resource: "volumesnapshots",
	}

	unstructured, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, snapshotName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting volumesnapshot %s/%s: %w", namespace, snapshotName, err)
	}

	status, found := unstructured.Object["status"].(map[string]interface{})
	if !found {
		return fmt.Errorf("volumesnapshot %s/%s has no status", namespace, snapshotName)
	}

	readyToUse, found := status["readyToUse"].(bool)
	if !found {
		return fmt.Errorf("volumesnapshot %s/%s has no readyToUse field", namespace, snapshotName)
	}

	if !readyToUse {
		return fmt.Errorf("volumesnapshot %s/%s is not ready", namespace, snapshotName)
	}

	return nil
}
