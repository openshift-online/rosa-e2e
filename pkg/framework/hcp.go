package framework

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// HCPNamespaces holds the resolved namespace names for a hosted cluster on the MC.
type HCPNamespaces struct {
	// HCNamespace is where the HostedCluster CR lives (e.g. "ocm-staging-<cluster-id>")
	HCNamespace string
	// HCPNamespace is where the HCP deployments live (e.g. "ocm-staging-<cluster-id>-<cluster-name>")
	HCPNamespace string
	// ClusterName is the HostedCluster CR name
	ClusterName string
}

var hostedClusterGVR = schema.GroupVersionResource{
	Group:    "hypershift.openshift.io",
	Version:  "v1beta1",
	Resource: "hostedclusters",
}

// ResolveHCPNamespaces finds the HostedCluster CR on the MC that matches the given cluster name
// or cluster ID, and returns the namespace information needed by other tests.
func ResolveHCPNamespaces(ctx context.Context, dynamicClient dynamic.Interface, clusterName, clusterID string) (*HCPNamespaces, error) {
	// List all HostedCluster CRs across all namespaces
	list, err := dynamicClient.Resource(hostedClusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing HostedClusters: %w", err)
	}

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Match by name or by checking if namespace contains the cluster ID
		if name == clusterName || (clusterID != "" && containsID(namespace, clusterID)) {
			// HCP namespace is typically <hc-namespace>-<cluster-name>
			hcpNS := fmt.Sprintf("%s-%s", namespace, name)

			return &HCPNamespaces{
				HCNamespace:  namespace,
				HCPNamespace: hcpNS,
				ClusterName:  name,
			}, nil
		}
	}

	return nil, fmt.Errorf("no HostedCluster found matching name=%q or id=%q", clusterName, clusterID)
}

func containsID(namespace, clusterID string) bool {
	return len(clusterID) > 8 && len(namespace) > len(clusterID) &&
		namespace[len(namespace)-len(clusterID):] == clusterID
}
