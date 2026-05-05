package framework

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

// InitiateControlPlaneUpgrade starts a control plane upgrade via the OCM API.
func InitiateControlPlaneUpgrade(conn *sdk.Connection, clusterID, targetVersion string) error {
	nextRun := time.Now().Add(5 * time.Minute).UTC()

	policy, err := cmv1.NewControlPlaneUpgradePolicy().
		Version(targetVersion).
		ScheduleType("manual").
		UpgradeType("ControlPlane").
		NextRun(nextRun).
		Build()
	if err != nil {
		return fmt.Errorf("building upgrade policy: %w", err)
	}

	_, err = conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).
		ControlPlane().UpgradePolicies().Add().Body(policy).Send()
	if err != nil {
		return fmt.Errorf("creating control plane upgrade policy: %w", err)
	}

	ginkgo.GinkgoWriter.Printf("Control plane upgrade to %s scheduled for cluster %s\n", targetVersion, clusterID)
	return nil
}

// WaitForUpgradeComplete polls the cluster version until it matches targetVersion or timeout.
func WaitForUpgradeComplete(conn *sdk.Connection, clusterID, targetVersion string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for cluster %s to reach version %s", clusterID, targetVersion)
		case <-ticker.C:
			resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
			if err != nil {
				ginkgo.GinkgoWriter.Printf("Error polling cluster version: %v\n", err)
				continue
			}
			currentVersion := resp.Body().Version().RawID()
			ginkgo.GinkgoWriter.Printf("Cluster version: %s, waiting for %s\n", currentVersion, targetVersion)

			if currentVersion == targetVersion {
				return nil
			}
		}
	}
}

// InitiateClusterUpgrade starts a cluster upgrade for Classic clusters via the OCM API.
// Classic clusters use the top-level UpgradePolicies endpoint (not ControlPlane).
func InitiateClusterUpgrade(conn *sdk.Connection, clusterID, targetVersion string) error {
	nextRun := time.Now().Add(5 * time.Minute).UTC()

	policy, err := cmv1.NewUpgradePolicy().
		Version(targetVersion).
		ScheduleType("manual").
		NextRun(nextRun).
		Build()
	if err != nil {
		return fmt.Errorf("building upgrade policy: %w", err)
	}

	_, err = conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).
		UpgradePolicies().Add().Body(policy).Send()
	if err != nil {
		return fmt.Errorf("creating cluster upgrade policy: %w", err)
	}

	ginkgo.GinkgoWriter.Printf("Classic cluster upgrade to %s scheduled for cluster %s\n", targetVersion, clusterID)
	return nil
}

// InitiateNodePoolUpgrade starts a node pool upgrade via the OCM API.
func InitiateNodePoolUpgrade(conn *sdk.Connection, clusterID, nodePoolID, targetVersion string) error {
	nextRun := time.Now().Add(5 * time.Minute).UTC()

	policy, err := cmv1.NewNodePoolUpgradePolicy().
		Version(targetVersion).
		ScheduleType("manual").
		UpgradeType("NodePool").
		NextRun(nextRun).
		Build()
	if err != nil {
		return fmt.Errorf("building nodepool upgrade policy: %w", err)
	}

	_, err = conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).
		NodePools().NodePool(nodePoolID).UpgradePolicies().Add().Body(policy).Send()
	if err != nil {
		return fmt.Errorf("creating nodepool upgrade policy: %w", err)
	}

	ginkgo.GinkgoWriter.Printf("NodePool %s upgrade to %s scheduled for cluster %s\n", nodePoolID, targetVersion, clusterID)
	return nil
}
