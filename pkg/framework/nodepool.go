package framework

import (
	"context"
	"fmt"
	"net/http"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

// ListNodePools lists the node pools of the cluster.
func ListNodePools(ctx context.Context, conn *sdk.Connection, clusterID string) ([]*cmv1.NodePool, error) {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).NodePools().List().SendContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("sending node pool list request: %w", err)
	}
	if resp.Status() != http.StatusOK {
		return nil, fmt.Errorf("unexpected status listing node pools: %d", resp.Status())
	}
	return resp.Items().Slice(), nil
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
