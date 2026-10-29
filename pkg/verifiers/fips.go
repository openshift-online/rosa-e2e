package verifiers

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/openshift-online/ocm-sdk-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	machineConfigGVR = schema.GroupVersionResource{
		Group:    "machineconfiguration.openshift.io",
		Version:  "v1",
		Resource: "machineconfigs",
	}
	machineConfigPoolGVR = schema.GroupVersionResource{
		Group:    "machineconfiguration.openshift.io",
		Version:  "v1",
		Resource: "machineconfigpools",
	}
	clusterPackageGVR = schema.GroupVersionResource{
		Group:    "package-operator.run",
		Version:  "v1alpha1",
		Resource: "clusterpackages",
	}
)

// CrossPlatformFIPSClusterPackages are ClusterPackage CRs expected on all FIPS clusters.
var CrossPlatformFIPSClusterPackages = []string{
	"managed-node-metadata-operator",
	"managed-upgrade-operator",
	"managed-velero-operator",
	"rbac-permissions-operator",
	"route-monitor-operator",
	"splunk-forwarder-operator",
	"configure-alertmanager-operator",
	"ocm-agent-operator",
}

// AWSFIPSClusterPackages are ClusterPackage CRs expected on AWS FIPS clusters (HCP, Classic).
var AWSFIPSClusterPackages = []string{}

// FIPSOperator defines a managed operator to verify on a FIPS-enabled cluster.
type FIPSOperator struct {
	Namespace  string
	Deployment string
}

// CrossPlatformFIPSOperators are operators expected on all FIPS clusters (HCP, Classic, OSD GCP).
var CrossPlatformFIPSOperators = []FIPSOperator{
	{Namespace: "openshift-managed-node-metadata-operator", Deployment: "managed-node-metadata-operator"},
	{Namespace: "openshift-managed-upgrade-operator", Deployment: "managed-upgrade-operator"},
	{Namespace: "openshift-managed-velero-operator", Deployment: "managed-velero-operator"},
	{Namespace: "openshift-rbac-permissions", Deployment: "rbac-permissions-operator"},
	{Namespace: "openshift-route-monitor-operator", Deployment: "route-monitor-operator"},
	{Namespace: "openshift-splunk-forwarder-operator", Deployment: "splunk-forwarder-operator"},
	{Namespace: "openshift-monitoring", Deployment: "configure-alertmanager-operator"},
}

// AWSFIPSOperators are operators expected on AWS FIPS clusters (HCP, Classic).
var AWSFIPSOperators = []FIPSOperator{}

// FIPSOperatorResult holds the outcome of a single operator health check.
type FIPSOperatorResult struct {
	Operator  FIPSOperator
	Found     bool
	Healthy   bool
	ErrDetail string
}

// verifyOperatorDeploymentHealthy checks a named deployment in a namespace.
// Returns (false, nil) when the deployment is not found — operator is not installed and the
// caller should skip rather than fail. Returns (true, nil) when the deployment is Available.
// Returns (true, error) when it exists but is not Available.
func verifyOperatorDeploymentHealthy(ctx context.Context, client kubernetes.Interface, namespace, deploymentName string) (bool, error) {
	deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("getting deployment %s/%s: %w", namespace, deploymentName, err)
	}

	for _, cond := range deploy.Status.Conditions {
		if cond.Type == "Available" && cond.Status == "True" {
			return true, nil
		}
	}

	return true, fmt.Errorf("deployment %s/%s is not available (ready: %d/%d, replicas: %d)",
		namespace, deploymentName, deploy.Status.ReadyReplicas, deploy.Status.Replicas, deploy.Status.UpdatedReplicas)
}

// VerifyFIPSOperatorHealthy checks whether a single managed operator deployment is
// healthy on a FIPS-enabled cluster. Returns (found, error): (false, nil) means not
// installed, (true, nil) means healthy, (true, error) means unhealthy.
func VerifyFIPSOperatorHealthy(ctx context.Context, client kubernetes.Interface, op FIPSOperator) (bool, error) {
	return verifyOperatorDeploymentHealthy(ctx, client, op.Namespace, op.Deployment)
}

// VerifyFIPSOperatorsHealthy checks a list of FIPS-critical operators and returns
// per-operator results. Operators not found are marked as not-found (callers may
// skip them). Any operator that exists but is unhealthy causes an aggregate error.
func VerifyFIPSOperatorsHealthy(ctx context.Context, client kubernetes.Interface, operators []FIPSOperator) ([]FIPSOperatorResult, error) {
	var results []FIPSOperatorResult
	var unhealthy []string

	for _, op := range operators {
		found, err := verifyOperatorDeploymentHealthy(ctx, client, op.Namespace, op.Deployment)
		r := FIPSOperatorResult{Operator: op, Found: found, Healthy: found && err == nil}
		if err != nil {
			r.ErrDetail = err.Error()
			unhealthy = append(unhealthy, fmt.Sprintf("%s/%s: %s", op.Namespace, op.Deployment, err.Error()))
		}
		results = append(results, r)
	}

	if len(unhealthy) > 0 {
		return results, fmt.Errorf("%d FIPS operator(s) unhealthy:\n%s", len(unhealthy), strings.Join(unhealthy, "\n"))
	}
	return results, nil
}

// VerifyPackageOperatorHealthy checks that the Package Operator deployment is
// available in the openshift-package-operator namespace. Returns (false, nil) if
// PKO is not installed on the cluster (caller should skip), (true, nil) if healthy,
// (true, error) if deployed but unhealthy.
func VerifyPackageOperatorHealthy(ctx context.Context, client kubernetes.Interface) (bool, error) {
	return verifyOperatorDeploymentHealthy(ctx, client, "openshift-package-operator", "package-operator-manager")
}

// VerifyClusterPackageHealthy checks that a specific ClusterPackage CR exists and
// has Available=True. Returns (false, nil) when the CR is not found,
// (true, nil) when healthy, (true, error) when it exists but is not Available.
func VerifyClusterPackageHealthy(ctx context.Context, dynamicClient dynamic.Interface, name string) (bool, error) {
	cp, err := dynamicClient.Resource(clusterPackageGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("getting ClusterPackage %s: %w", name, err)
	}

	conditions, found, err := unstructured.NestedSlice(cp.Object, "status", "conditions")
	if err != nil {
		return true, fmt.Errorf("extracting conditions from ClusterPackage %s: %w", name, err)
	}
	if !found {
		return true, fmt.Errorf("ClusterPackage %s has no status conditions", name)
	}

	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := condMap["type"].(string)
		condStatus, _ := condMap["status"].(string)
		if condType == "Available" && condStatus == "True" {
			return true, nil
		}
	}

	return true, fmt.Errorf("ClusterPackage %s is not Available", name)
}

// VerifyFIPSClusterPackagesHealthy checks all FIPS-critical ClusterPackage CRs.
// Packages not found are reported but do not cause failure (PKO migration may be
// in progress). Packages that exist but are unhealthy cause an error.
func VerifyFIPSClusterPackagesHealthy(ctx context.Context, dynamicClient dynamic.Interface, packages []string) error {
	var unhealthy []string

	for _, pkg := range packages {
		found, err := VerifyClusterPackageHealthy(ctx, dynamicClient, pkg)
		if !found {
			continue
		}
		if err != nil {
			unhealthy = append(unhealthy, fmt.Sprintf("%s: %s", pkg, err.Error()))
		}
	}

	if len(unhealthy) > 0 {
		return fmt.Errorf("%d FIPS ClusterPackage(s) unhealthy:\n%s",
			len(unhealthy), strings.Join(unhealthy, "\n"))
	}
	return nil
}

// VerifyClusterFIPSEnabled checks via OCM API whether the cluster has FIPS mode
// enabled. Returns (true, nil) when FIPS is on, (false, nil) when it is off, and
// (false, err) on an API error.
func VerifyClusterFIPSEnabled(conn *sdk.Connection, clusterID string) (bool, error) {
	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
	if err != nil {
		return false, fmt.Errorf("fetching cluster %s: %w", clusterID, err)
	}
	return resp.Body().FIPS(), nil
}

// VerifyHCPFIPSEnabled verifies that an HCP cluster is configured for FIPS by
// checking HostedCluster.spec.fips=true on the management cluster.
// ROSA HCP guests do not expose MachineConfig CRs; HyperShift applies FIPS
// via ignition generated from this field.
func VerifyHCPFIPSEnabled(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) error {
	hc, err := dynamicClient.Resource(hostedClusterGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting HostedCluster %s/%s: %w", namespace, name, err)
	}

	fips, found, err := unstructured.NestedBool(hc.Object, "spec", "fips")
	if err != nil {
		return fmt.Errorf("reading spec.fips from HostedCluster %s/%s: %w", namespace, name, err)
	}
	if !found || !fips {
		return fmt.Errorf("HostedCluster %s/%s does not have spec.fips=true", namespace, name)
	}

	return nil
}

// VerifyNodeFIPSEnabled verifies that FIPS is active on cluster nodes by:
//  1. Checking rendered MachineConfig CRs have spec.fips=true (correct configuration)
//  2. Checking MachineConfigPool status to confirm all nodes have applied the config
//     (Updated=True, Degraded=False, machineCount == updatedMachineCount)
//
// When MCO applies a FIPS-enabled config it reboots each node, so an Updated pool
// with a FIPS-enabled rendered config guarantees kernel-level fips_enabled=1.
// This is a read-only operation — no namespaces or workloads are created.
// Classic / OSD only: HCP guests do not have the MachineConfig API; use
// VerifyHCPFIPSEnabled against the management cluster instead.
func VerifyNodeFIPSEnabled(ctx context.Context, dynamicClient dynamic.Interface) error {
	if err := verifyRenderedMachineConfigsFIPS(ctx, dynamicClient); err != nil {
		return err
	}
	return verifyMachineConfigPoolsUpdated(ctx, dynamicClient)
}

// verifyRenderedMachineConfigsFIPS confirms all rendered MachineConfigs have spec.fips=true.
func verifyRenderedMachineConfigsFIPS(ctx context.Context, dynamicClient dynamic.Interface) error {
	mcList, err := dynamicClient.Resource(machineConfigGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing MachineConfig resources: %w", err)
	}

	var renderedConfigs []unstructured.Unstructured
	for _, mc := range mcList.Items {
		if strings.HasPrefix(mc.GetName(), "rendered-") {
			renderedConfigs = append(renderedConfigs, mc)
		}
	}

	if len(renderedConfigs) == 0 {
		return fmt.Errorf("no rendered MachineConfig resources found — cannot verify FIPS at node level")
	}

	var nonFIPS []string
	for _, mc := range renderedConfigs {
		fips, found, err := unstructured.NestedBool(mc.Object, "spec", "fips")
		if err != nil {
			return fmt.Errorf("reading spec.fips from MachineConfig %s: %w", mc.GetName(), err)
		}
		if !found || !fips {
			nonFIPS = append(nonFIPS, mc.GetName())
		}
	}

	if len(nonFIPS) > 0 {
		return fmt.Errorf("FIPS not enabled on %d rendered MachineConfig(s): %s",
			len(nonFIPS), strings.Join(nonFIPS, ", "))
	}

	return nil
}

// verifyMachineConfigPoolsUpdated confirms all MachineConfigPools have rolled out
// their configuration (Updated=True, Degraded=False) with all nodes updated.
func verifyMachineConfigPoolsUpdated(ctx context.Context, dynamicClient dynamic.Interface) error {
	mcpList, err := dynamicClient.Resource(machineConfigPoolGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing MachineConfigPool resources: %w", err)
	}

	if len(mcpList.Items) == 0 {
		return fmt.Errorf("no MachineConfigPool resources found")
	}

	var problems []string
	for _, mcp := range mcpList.Items {
		name := mcp.GetName()

		machineCount, _, _ := unstructured.NestedInt64(mcp.Object, "status", "machineCount")
		updatedCount, _, _ := unstructured.NestedInt64(mcp.Object, "status", "updatedMachineCount")
		readyCount, _, _ := unstructured.NestedInt64(mcp.Object, "status", "readyMachineCount")

		if machineCount == 0 {
			continue
		}

		if updatedCount != machineCount {
			problems = append(problems, fmt.Sprintf(
				"%s: %d/%d nodes updated (rollout incomplete)", name, updatedCount, machineCount))
			continue
		}

		if readyCount != machineCount {
			problems = append(problems, fmt.Sprintf(
				"%s: %d/%d nodes ready", name, readyCount, machineCount))
			continue
		}

		conditions, found, _ := unstructured.NestedSlice(mcp.Object, "status", "conditions")
		if !found {
			continue
		}
		for _, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _ := condMap["type"].(string)
			condStatus, _ := condMap["status"].(string)

			if condType == "Degraded" && condStatus == "True" {
				reason, _ := condMap["reason"].(string)
				problems = append(problems, fmt.Sprintf("%s: Degraded (reason: %s)", name, reason))
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("MachineConfigPool(s) not fully updated — FIPS config may not be applied to all nodes:\n%s",
			strings.Join(problems, "\n"))
	}

	return nil
}
