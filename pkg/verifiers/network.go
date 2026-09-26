package verifiers

import (
	"fmt"
	"net"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

// Network type values reported by OCM.
const (
	// OVNKubernetesNetworkType is the default managed CNI for ROSA clusters.
	OVNKubernetesNetworkType = "OVNKubernetes"
	// OtherNetworkType is what OCM reports for no-CNI (BYO CNI) clusters,
	// i.e. those created with the `rosa create cluster --no-cni` flag.
	OtherNetworkType = "Other"
)

// VerifyNetworkType asserts that the cluster's network type reported by OCM matches expectedType
// (e.g. "OVNKubernetes" for managed CNI, or "Other" for no-CNI / BYO CNI clusters).
func VerifyNetworkType(conn *sdk.Connection, clusterID, expectedType string) error {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
	if err != nil {
		return fmt.Errorf("getting cluster %s: %w", clusterID, err)
	}

	networkType, ok := resp.Body().Network().GetType()
	if !ok {
		return fmt.Errorf("cluster %s does not report a network type", clusterID)
	}
	if networkType != expectedType {
		return fmt.Errorf("expected cluster %s network type to be %q, got %q", clusterID, expectedType, networkType)
	}
	return nil
}

// GetMachineCIDR returns the cluster's machine (node) CIDR as reported by OCM.
// For AWS VPC CNI clusters this is the range from which pods receive VPC IPs.
func GetMachineCIDR(conn *sdk.Connection, clusterID string) (string, error) {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
	if err != nil {
		return "", fmt.Errorf("getting cluster %s: %w", clusterID, err)
	}

	cidr, ok := resp.Body().Network().GetMachineCIDR()
	if !ok || cidr == "" {
		return "", fmt.Errorf("cluster %s does not report a machine CIDR", clusterID)
	}
	return cidr, nil
}

// VerifyIPInCIDR asserts that ip is a valid IP address contained within cidr.
// Used to confirm a pod received a VPC IP (from the machine CIDR) rather than an
// overlay address, proving AWS VPC CNI IPAM is in effect.
func VerifyIPInCIDR(ip, cidr string) error {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return fmt.Errorf("invalid IP address %q", ip)
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	if !ipNet.Contains(parsedIP) {
		return fmt.Errorf("IP %s is not within machine CIDR %s", ip, cidr)
	}
	return nil
}
