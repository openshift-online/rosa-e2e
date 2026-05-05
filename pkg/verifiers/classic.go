package verifiers

import (
	"fmt"
	"net/http"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

// VerifyMachinePoolsExist checks that at least one MachinePool exists for a Classic cluster via the OCM API.
func VerifyMachinePoolsExist(conn *sdk.Connection, clusterID string) error {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).
		MachinePools().List().Send()
	if err != nil {
		return fmt.Errorf("listing machine pools for cluster %s: %w", clusterID, err)
	}
	if resp.Status() != http.StatusOK {
		return fmt.Errorf("unexpected status %d listing machine pools for cluster %s", resp.Status(), clusterID)
	}
	if resp.Total() == 0 {
		return fmt.Errorf("no machine pools found for cluster %s", clusterID)
	}
	return nil
}
