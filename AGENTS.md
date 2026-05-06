# AGENTS.md

This file provides guidance to AI coding agents working with code in this repository.

## What This Is

Unified E2E test suite for ROSA and OSD. Go/Ginkgo v2 binary using OCM SDK directly for cluster lifecycle, with composable verifiers for health checks. Supports multiple cluster topologies: ROSA HCP, ROSA Classic STS, and (planned) OSD GCP.

## Build and Test Commands

```bash
make build          # Build test binary (ginkgo build with E2Etests tag)
make test           # Run all e2e tests with JUnit report
make dry-run        # List tests without executing
make unit-test      # Run unit tests (pkg/ only)
make lint           # go vet + golangci-lint
make image          # Build container image
make clean          # Remove build artifacts

# Run filtered tests by topology
LABEL_FILTER="Platform:HCP" make test
LABEL_FILTER="Platform:Classic" make test
LABEL_FILTER="Platform:HCP && Importance:Critical" make test

# Required env vars for test execution
OCM_TOKEN=$(ocm token) OCM_ENV=staging CLUSTER_ID=<id> make test
```

## Architecture

```
pkg/config/       Config loading: YAML file < env var overrides. Includes ClusterTopology field.
pkg/labels/       Ginkgo v2 label constants (Platform, Area, Importance, Speed, Positivity)
pkg/framework/    OCM connection, cluster CRUD (HCP + Classic), topology detection, kube/dynamic clients, AWS clients
pkg/verifiers/    Composable health checks. classic.go for Classic-specific, hostedcluster.go for HCP-specific.
test/e2e/         Ginkgo test specs (build tag: E2Etests)
configs/          YAML config files per environment
```

## Supported Topologies

| Topology | Label | Cluster Creation | Upgrade API | Worker Scaling |
|----------|-------|-----------------|-------------|----------------|
| ROSA HCP | `Platform:HCP` | `CreateRosaHCPCluster()` | `ControlPlane().UpgradePolicies()` | NodePool |
| ROSA Classic STS | `Platform:Classic` | `CreateRosaClassicCluster()` | `UpgradePolicies()` with `UpgradeType("OSD")` | MachinePool |
| OSD GCP | `Platform:OSD-GCP` | Not yet implemented | Not yet implemented | Not yet implemented |

### Topology Detection

The framework auto-detects topology from the OCM API via `DetectClusterTopology()`:
- Checks `Hypershift().Enabled()` and cloud provider
- Override with `CLUSTER_TOPOLOGY` env var (values: `hcp`, `classic`, `osd-gcp`)
- Input is normalized to lowercase

### Topology Helpers

`TestContext` provides `IsHCP()`, `IsClassic()`, `IsOSDGCP()` for conditional test logic. These lazy-initialize topology detection on first call.

## Key Patterns

- **Build tag**: All test files use `//go:build E2Etests` to separate from unit tests
- **Suite-level singletons**: `cfg` and `conn` in `test/e2e/setup.go`, shared across all tests
- **TestContext per test**: `framework.NewTestContext(cfg, conn)` then call `InitHCClients()`, `InitMCClients()`, or `InitAWSClients()` as needed
- **Two verifier patterns**: `ClusterVerifier` interface with `RunVerifiers()` for kube-client checks, standalone functions for OCM/AWS/dynamic client checks
- **Graceful skips**: Tests skip when required access (MC, AWS) isn't configured, or when topology doesn't match
- **DeferCleanup**: Used for test resource cleanup (namespaces, deployments)
- **Dynamic client for CRDs**: ClusterOperator, HostedCluster, NodePool, RouteMonitor, VpcEndpoint are parsed from unstructured to avoid heavy API type imports
- **MC/SC access (HCP only)**: Use `MC_KUBECONFIG`/`SC_KUBECONFIG` env vars or `MANAGEMENT_CLUSTER_ID` for OCM credentials API
- **HCP namespace resolution**: Automatically discovered via HostedCluster CR lookup. Skipped for Classic topology.

## Test Labels

Tests use dual platform labels when they work on both topologies.

**Dual-topology tests** (labeled `Platform:HCP` + `Platform:Classic`):
- Data plane: workload, networking, storage
- Managed service: ClusterOperators, IAM validation, infrastructure tags
- Customer features: RBAC, ingress, log forwarding, KMS, PrivateLink
- Management plane: OCM API health, cluster service health
- Upgrade: post-upgrade verification

**HCP-only tests** (labeled `Platform:HCP` only):
- HostedCluster/NodePool CR health (requires MC)
- MC infrastructure: HyperShift operator, external-dns, CAPI
- SC infrastructure: ACM hub, cert-manager, Hive, MCE
- MC-based managed operators: RMO, AVO, audit-webhook
- OSDFM health
- NodePool upgrades

**Classic-only tests** (labeled `Platform:Classic` only):
- MachinePool list/verify
- Classic cluster upgrade
- Classic full lifecycle (create/verify/delete)

## Adding a New Test

1. Create `test/e2e/<name>_test.go` with `//go:build E2Etests` and `package e2e`
2. Use suite-level `cfg` and `conn` (don't create your own)
3. Apply labels: include both `labels.HCP` and `labels.Classic` if the test works on both topologies
4. Create TestContext, init required clients, skip if access unavailable
5. For topology-specific behavior, use `tc.IsHCP()` / `tc.IsClassic()` to branch or skip
6. Use existing verifiers from `pkg/verifiers/` or create new ones
7. Clean up with `DeferCleanup`

## Adding a New Verifier

- For kube-client checks: implement `ClusterVerifier` interface in `pkg/verifiers/`
- For OCM/AWS/dynamic checks: create standalone `Verify*` function (follow `VerifyClusterReady` pattern)
- Use dynamic client for any OpenShift/HyperShift CRDs to avoid API type imports
- Accept `context.Context` as first parameter and use `SendContext(ctx)` for cancellation support

## Known Issues

- AWS SDK validates credentials lazily. `InitAWSClients` uses eager `Retrieve()` to fail fast.
- golangci-lint v2 with k8s/OCM SDK dependencies can use excessive memory (48+ GB). Use `go vet` for local validation.
- Classic STS clusters take 30-45 min to provision vs 15 min for HCP.

## Jira

- [ROSA-683](https://redhat.atlassian.net/browse/ROSA-683): ROSA Downstream CI Test Coverage and Validation
- [ROSA-727](https://redhat.atlassian.net/browse/ROSA-727): ROSA Canonical E2E Test Suite and Signals
