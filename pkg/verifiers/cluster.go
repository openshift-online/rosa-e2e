package verifiers

import (
	"fmt"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

// VerifyClusterReady asserts that the cluster state is "ready" in OCM.
func VerifyClusterReady(conn *sdk.Connection, clusterID string) error {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
	if err != nil {
		return fmt.Errorf("getting cluster %s: %w", clusterID, err)
	}

	state := resp.Body().State()
	if state != cmv1.ClusterStateReady {
		return fmt.Errorf("expected cluster %s state to be %q, got %q", clusterID, cmv1.ClusterStateReady, state)
	}
	return nil
}

// VerifyClusterDeleting asserts that the cluster state is "uninstalling" in OCM.
func VerifyClusterDeleting(conn *sdk.Connection, clusterID string) error {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
	if err != nil {
		return fmt.Errorf("getting cluster %s: %w", clusterID, err)
	}

	state := resp.Body().State()
	if state != cmv1.ClusterStateUninstalling {
		return fmt.Errorf("expected cluster %s state to be %q, got %q", clusterID, cmv1.ClusterStateUninstalling, state)
	}
	return nil
}
