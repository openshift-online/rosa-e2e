package framework

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

var nodePoolGVR = schema.GroupVersionResource{
	Group:    "hypershift.openshift.io",
	Version:  "v1beta1",
	Resource: "nodepools",
}

// HCPNodePool is a management-cluster NodePool CR reduced to the fields the
// suite needs for scaling.
type HCPNodePool struct {
	Name     string
	Replicas int32
}

// ListHCPNodePools lists the NodePool CRs of a hosted cluster on the
// management cluster. Pools without an explicit replica count (autoscaled)
// are reported with Replicas 0.
func ListHCPNodePools(ctx context.Context, dyn dynamic.Interface, namespace string) ([]HCPNodePool, error) {
	list, err := dyn.Resource(nodePoolGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing NodePools in %s: %w", namespace, err)
	}

	pools := make([]HCPNodePool, 0, len(list.Items))
	for _, item := range list.Items {
		pool := HCPNodePool{Name: item.GetName()}
		if replicas, found, err := unstructured.NestedInt64(item.Object, "spec", "replicas"); err == nil && found {
			pool.Replicas = int32(replicas)
		}
		pools = append(pools, pool)
	}
	return pools, nil
}

// ScaleHCPNodePool patches a management-cluster NodePool CR to the given
// replica count.
func ScaleHCPNodePool(ctx context.Context, dyn dynamic.Interface, namespace, name string, replicas int32) error {
	patch := map[string]interface{}{
		"spec": map[string]interface{}{"replicas": replicas},
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshalling NodePool %s/%s patch: %w", namespace, name, err)
	}

	// JSON merge patch: the API server rejects strategic merge patches on CRDs.
	_, err = dyn.Resource(nodePoolGVR).Namespace(namespace).Patch(ctx, name,
		types.MergePatchType, raw, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patching NodePool %s/%s replicas to %d: %w", namespace, name, replicas, err)
	}
	return nil
}
