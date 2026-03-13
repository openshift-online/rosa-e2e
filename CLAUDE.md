# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

Unified ROSA E2E test suite. Go/Ginkgo v2 binary using OCM SDK directly for cluster lifecycle, with composable verifiers for health checks across all three ROSA HCP infrastructure tiers (SC, MC, HC).

## Build and Test Commands

```bash
make build          # Build test binary (ginkgo build with E2Etests tag)
make test           # Run all e2e tests with JUnit report
make dry-run        # List tests without executing
make unit-test      # Run unit tests (pkg/ only)
make lint           # go vet + golangci-lint
make image          # Build container image
make clean          # Remove build artifacts

# Run filtered tests
LABEL_FILTER="Area:ManagedService" make test
LABEL_FILTER="Platform:HCP && Importance:Critical" make test

# Required env vars for test execution
OCM_TOKEN=$(ocm token) OCM_ENV=staging CLUSTER_ID=<id> make test
```

## Architecture

```
pkg/config/       Config loading: YAML file < env var overrides
pkg/labels/       Ginkgo v2 label constants (Platform, Area, Importance, Speed, Positivity)
pkg/framework/    OCM connection, cluster CRUD, kube/dynamic clients, AWS clients, workload/storage helpers
pkg/verifiers/    Composable health checks (standalone functions, not all use ClusterVerifier interface)
test/e2e/         Ginkgo test specs (build tag: E2Etests)
configs/          YAML config files per environment
```

## Key Patterns

- **Build tag**: All test files use `//go:build E2Etests` to separate from unit tests
- **Suite-level singletons**: `cfg` and `conn` in `test/e2e/setup.go`, shared across all tests
- **TestContext per test**: `framework.NewTestContext(cfg, conn)` then call `InitHCClients()`, `InitMCClients()`, or `InitAWSClients()` as needed
- **Two verifier patterns**: `ClusterVerifier` interface with `RunVerifiers()` for kube-client checks, standalone functions for OCM/AWS/dynamic client checks
- **Graceful skips**: Tests skip when required access (MC, AWS) isn't configured
- **DeferCleanup**: Used for test resource cleanup (namespaces, deployments)
- **Dynamic client for CRDs**: ClusterOperator, HostedCluster, NodePool, RouteMonitor, VpcEndpoint -- parsed from unstructured, no heavy API type imports

## Test Areas and Labels

| Area | Label | Tests |
|------|-------|-------|
| Cluster Lifecycle | `Area:ClusterLifecycle` | Create/delete via OCM API |
| Data Plane | `Area:DataPlane` | Workload deployment, storage PVC |
| Managed Service | `Area:ManagedService` | ClusterOperators, CloudTrail IAM, EBS tags, HCP namespace, HostedCluster/NodePool CRs, RMO/AVO/audit-webhook |
| Customer Features | `Area:CustomerFeatures` | Not yet implemented |
| Infrastructure | `Area:Infrastructure` | Not yet implemented |
| Upgrade | `Area:Upgrade` | Not yet implemented |

## Adding a New Test

1. Create `test/e2e/<name>_test.go` with `//go:build E2Etests` and `package e2e`
2. Use suite-level `cfg` and `conn` (don't create your own)
3. Apply labels: `labels.High, labels.Positive, labels.HCP, labels.DataPlane`
4. Create TestContext, init required clients, skip if access unavailable
5. Use existing verifiers from `pkg/verifiers/` or create new ones
6. Clean up with `DeferCleanup`

## Adding a New Verifier

- For kube-client checks: implement `ClusterVerifier` interface in `pkg/verifiers/`
- For OCM/AWS/dynamic checks: create standalone `Verify*` function (follow `VerifyClusterReady` pattern)
- Use dynamic client for any OpenShift/HyperShift CRDs to avoid API type imports

## Known Issues

- HCP namespace on staging is `ocm-staging-<cluster-id>-<cluster-name>`, not just cluster ID
- MC access requires backplane (OCM credentials API doesn't work for MCs)
- AWS SDK validates credentials lazily -- `InitAWSClients` uses eager `Retrieve()` to fail fast

## Jira

- ROSA-683: ROSA Downstream CI Test Coverage and Validation
- SREP-3987: ROSA HCP Managed Service Conformance Tests
