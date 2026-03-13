# ARO-HCP E2E Tests - Analysis

## Overview

**Repository**: `github.com/Azure/ARO-HCP`
**Language**: Go
**Framework**: Ginkgo v2 / Gomega (BDD) + OpenShift Test Extension
**CI System**: GitHub Actions + Prow
**Container Image**: Built via `test/Containerfile.e2e`
**Build Tag**: `E2Etests` (separates e2e from unit tests)

## How Tests Are Run

### Execution

```bash
# Local
cd test/e2e
make e2etest  # Build: ginkgo build --tags E2Etests
make run      # Run: ginkgo run --tags E2Etests

# Container
podman run <image> /e2e.test --ginkgo.timeout 1h --ginkgo.junit-report junit-report.xml --ginkgo.trace
```

### Test Runner CLI

The `aro-hcp-tests` binary (`test/cmd/aro-hcp-tests/main.go`) wraps OpenShift Test Extension:

```bash
aro-hcp-tests run-suite <suite-name>
aro-hcp-tests run-test <test-name>
aro-hcp-tests list
```

### Test Suites (with parallelism)

| Suite | Parallelism | Description |
|---|---|---|
| `integration/parallel` | 24 | Fast integration tests |
| `integration/parallel/slow` | 24 | Slow integration tests |
| `stage/parallel` | 34 | Fast stage tests |
| `stage/parallel/slow` | 34 | Slow stage tests |
| `prod/parallel` | 19 | Fast production tests |
| `prod/parallel/slow` | 19 | Slow production tests |
| `dev-cd-check/parallel` | 20 | Validation subset |
| `rp-api-compat-all/parallel` | 24 | RP API compatibility |

### Setup Model

Tests load configuration via:
1. `SETUP_FILEPATH` environment variable pointing to a pre-created setup file
2. Fallback: Bicep template deployment (`FALLBACK_TO_BICEP=true`)

The setup file contains cluster parameters, Azure identities, VNet/subnet configuration, and NSG references.

## What Tests Are Run

### Design Philosophy

ARO-HCP e2e tests follow a distinctive methodology:

1. **Per-test cluster isolation**: Each test creates, uses, and destroys its own cluster. This ensures complete independence and enables high parallelism.
2. **Outside-in testing**: Tests validate user-facing functionality first (cluster creation, operations) before internal infrastructure.
3. **Run anywhere**: Label-based filtering allows the same test suite to run in dev, integration, stage, and production.
4. **API compatibility**: Tests work against both the direct RP API and the ARM API.

### Test Infrastructure

- **TestContext**: Per-test isolated context with automatic cleanup via `DeferCleanup()`
- **HCP SDK**: Generated ARM SDK for cluster operations
- **Verifiers**: Reusable verification components (nodes, RBAC, operators, API services, apps, pull secrets)
- **Framework helpers**: Cluster creation, resource group management, identity management, admin API access

### Test Categories

**Customer Cluster Operations** (per-test cluster):
- Basic cluster creation with nodepool
- Cluster with custom autoscaling
- Cluster without CNI plugin
- Custom OS disk size node pools
- NSG/subnet reuse restrictions
- Multiple clusters in shared resource group
- Pull secret management
- TLS certificate validation
- Version listing and back-level support
- ARM64 and GPU node pools
- Node pool autoscaling and updates
- External auth configuration
- Image registry configuration
- Negative cases (invalid operations)

**Authorization & Connectivity**:
- CIDR-based API access control with VM connectivity testing

**SRE Breakglass**:
- Admin API breakglass session access
- RBAC verification (aro-sre-pso read-only, aro-sre-csa cluster-admin)
- Session TTL expiration
- Cross-resource ownership restrictions

**API Validation** (per-run shared cluster):
- Negative cluster creation (missing location)
- Cluster get/list/update/delete operations
- Nodepool creation errors and retrieval

**Observability**:
- Kusto (Azure Data Explorer) logs availability

## What Tests Are Defined

### Label System

| Category | Values |
|---|---|
| Importance | Critical, High, Medium, Low |
| Positivity | Positive, Negative |
| Environment | RequireNothing, RequireHappyPathInfra, DevelopmentOnly, IntegrationOnly |
| API Compat | AroRpApiCompatible |
| Usage | CoreInfraService, CreateCluster, SetupValidation, TeardownValidation |
| Speed | Slow |

### 33 Test Files

| File | Importance | Description |
|---|---|---|
| `complete_cluster_create.go` | Critical | Basic cluster creation with nodepool |
| `cluster_authorized_cidrs_connectivity.go` | Critical | CIDR-based API access control |
| `cluster_delete_test.go` | Critical | Cluster deletion |
| `admin_api.go` | High | Breakglass SRE access with RBAC |
| `cluster_autoscaling.go` | - | Custom autoscaling |
| `cluster_create_no_cni.go` | - | Cluster without CNI |
| `cluster_create_nodepool_osdisk.go` | - | Custom OS disk size |
| `cluster_create_missing_info.go` | - | Negative: back-level versions |
| `cluster_nsg_subnet_reuse.go` | - | NSG/subnet restrictions |
| `clusters_sharing_resgroup.go` | - | Shared resource group |
| `cluster_pullsecret.go` | - | Pull secret management |
| `cluster_tls_endpoints.go` | - | TLS validation |
| `cluster_versions.go` | - | Version listing |
| `cluster_version_backlevel.go` | - | Back-level version support |
| `arm64_nodepool.go` | - | ARM64 VM node pools |
| `gpu_nodepools_create_delete.go` | - | GPU node pools |
| `nodepool_autoscaling.go` | - | Node pool autoscaling |
| `nodepool_update_nodes.go` | - | Node pool updates |
| `external_auth_create.go` | - | External auth creation |
| `external_auth_list_and_verify.go` | - | External auth lifecycle |
| `image_registry_cluster_create.go` | - | Image registry config |
| `cluster_list_no_infra.go` | - | List without node pools |
| `simple_negative_cases.go` | Medium | Invalid operation cases |
| `admin_credential_lifecycle.go` | - | Admin credential lifecycle |
| `cluster_create_test.go` | - | Invalid creation (negative) |
| `cluster_get_test.go` | - | Cluster retrieval |
| `cluster_list_test.go` | - | List by subscription/RG |
| `cluster_update.go` | - | Invalid updates (negative) |
| `nodepool_create_test.go` | - | Nodepool creation errors |
| `nodepool_get_test.go` | - | Nodepool retrieval |
| `kusto_logs_present.go` | - | Kusto logs availability |

### Verifiers (Reusable Assertions)

Located in `test/util/verifiers/`:
- `basic.go` - List/get operations
- `nodes.go` - Node verification
- `rbac.go` - RBAC access control
- `clusteroperator.go` - Cluster operator status
- `apiservice.go` - API service availability
- `operator.go` - Operator health
- `serving_app.go` - Application deployment
- `image_pull.go` - Image pull verification
- `pullsecret.go` - Pull secret config
- `kusto.go` - Kusto logs
- `kubectl.go` - kubectl access

## Strengths

- **Per-test isolation**: Each test is fully independent, enabling high parallelism
- **Outside-in methodology**: Tests what users actually do first
- **Clean separation**: Framework (test infra) vs. tests (specifications) vs. verifiers (assertions)
- **Runs anywhere**: Same tests work across dev, integration, stage, production
- **Self-contained**: No external tooling dependencies beyond the test binary
- **Highly parallel**: 19-34 parallel workers per suite

## Weaknesses

- Azure-specific: Uses ARM API, Bicep templates, Azure SDK
- Not directly reusable for ROSA (different cloud provider, different APIs)
- Per-test cluster creation is expensive in time (45-min per cluster)

## Key Takeaways for ROSA E2E Design

The ARO-HCP approach is the recommended model. Key patterns to adopt:

1. **Per-test TestContext** with automatic cleanup via `DeferCleanup()`
2. **Reusable verifiers** as composable assertion building blocks
3. **Label-based test selection** for environment-appropriate filtering
4. **Framework/test/verifier separation** for clean architecture
5. **Outside-in test ordering**: customer operations first, internals second
6. **Setup file model**: Pre-created infrastructure for fast per-run tests, with fallback to full provisioning
