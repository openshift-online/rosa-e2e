package framework

import (
	"context"
	"fmt"
	"net/http"
	"time"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

// CreateNodePool creates a node pool for the given cluster via the OCM API.
func CreateNodePool(ctx context.Context, conn *sdk.Connection, clusterID string, nodePool *cmv1.NodePool) (*cmv1.NodePool, error) {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).NodePools().Add().
		Body(nodePool).SendContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("sending node pool creation request: %w", err)
	}
	if resp.Status() != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status creating node pool: %d", resp.Status())
	}
	return resp.Body(), nil
}

// GetNodePool fetches the current state of a node pool.
func GetNodePool(ctx context.Context, conn *sdk.Connection, clusterID, nodePoolID string) (*cmv1.NodePool, error) {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).NodePools().
		NodePool(nodePoolID).Get().SendContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting node pool %s: %w", nodePoolID, err)
	}
	if resp.Status() != http.StatusOK {
		return nil, fmt.Errorf("unexpected status getting node pool %s: %d", nodePoolID, resp.Status())
	}
	return resp.Body(), nil
}

// DeleteNodePool deletes a node pool from the cluster.
func DeleteNodePool(ctx context.Context, conn *sdk.Connection, clusterID, nodePoolID string) error {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).NodePools().
		NodePool(nodePoolID).Delete().SendContext(ctx)
	if err != nil {
		return fmt.Errorf("sending node pool deletion request for %s: %w", nodePoolID, err)
	}
	if resp.Status() != http.StatusNoContent && resp.Status() != http.StatusOK {
		return fmt.Errorf("unexpected status deleting node pool %s: %d", nodePoolID, resp.Status())
	}
	return nil
}

// WaitForNodePoolDeleted polls until the node pool returns 404, so a later
// run reusing the pool ID cannot conflict with a pool still being deleted.
func WaitForNodePoolDeleted(ctx context.Context, conn *sdk.Connection, clusterID, nodePoolID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).NodePools().
			NodePool(nodePoolID).Get().SendContext(ctx)
		if err != nil {
			return fmt.Errorf("sending node pool get request for %s: %w", nodePoolID, err)
		}
		if resp.Status() == http.StatusNotFound {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("node pool %s still exists after %v", nodePoolID, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(30 * time.Second):
		}
	}
}
