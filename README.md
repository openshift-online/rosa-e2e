# rosa-e2e

Unified end-to-end test suite for ROSA (Red Hat OpenShift Service on AWS). Validates cluster lifecycle, data plane health, managed service components, customer features, infrastructure tiers, and upgrade operations.

Modeled after the [ARO-HCP e2e methodology](https://github.com/Azure/ARO-HCP/tree/main/test): framework/test/verifier separation, label-based test selection, Ginkgo v2, and composable health checks.

## Overview

ROSA HCP has three infrastructure tiers that need validation:

- **Service Clusters (SC)**: OSD Classic clusters running ACM hub, cert-manager, Hive
- **Management Clusters (MC)**: OSD Classic clusters running HyperShift operator, hosted control planes, RMO, AVO
- **Hosted Clusters (HC)**: Customer data plane with workers and managed operators

This test suite validates all three tiers and the interactions between them.

## Test Areas

The suite is organized into six test areas (use `Area:*` labels to filter):

1. **Cluster Lifecycle** (`Area:ClusterLifecycle`) - Cluster create/delete via OCM API, cluster state transitions
2. **Data Plane** (`Area:DataPlane`) - Workload deployments, storage, PVC, snapshots, node readiness
3. **Managed Service Health** (`Area:ManagedService`) - ClusterOperators, RMO/AVO on MC, CloudTrail IAM validation, infrastructure tags, HostedCluster CRs
4. **Customer Features** (`Area:CustomerFeatures`) - Log forwarding, external OIDC, PrivateLink, KMS, machine pools
5. **Infrastructure Tiers** (`Area:Infrastructure`) - SC health, MC health, cross-tier connectivity
6. **Upgrade Validation** (`Area:Upgrade`) - Control plane upgrades, nodepool upgrades

## Prerequisites

- Go 1.23+
- An OCM offline token (get from https://console.redhat.com/openshift/token)
- For full lifecycle tests: Pre-provisioned AWS infrastructure (VPC, subnets, OIDC config, IAM roles)
- For existing cluster tests: Just the cluster ID

## Quick Start

### Test against an existing cluster

```bash
export OCM_TOKEN="your-ocm-offline-token"
export OCM_ENV=staging
export CLUSTER_ID="your-cluster-id"
make test
```

### Full lifecycle test (create, verify, delete)

```bash
export OCM_TOKEN="your-ocm-offline-token"
export OCM_ENV=staging
export AWS_REGION=us-east-1
export AWS_ACCOUNT_ID=123456789012
export SUBNET_IDS=subnet-abc123,subnet-def456
export OIDC_CONFIG_ID=your-oidc-config-id
export ACCOUNT_ROLE_PREFIX=ManagedOpenShift
export OPERATOR_ROLE_PREFIX=ManagedOpenShift-oper
export BILLING_ACCOUNT_ID=123456789012
make test
```

### Dry run (list tests without executing)

```bash
make dry-run
```

### Run specific test areas

```bash
# Only Managed Service Health tests
LABEL_FILTER="Area:ManagedService" make test

# Only critical importance tests
LABEL_FILTER="Importance:Critical" make test

# Fast health checks (exclude slow tests)
LABEL_FILTER="!Speed:Slow" make test
```

## Configuration

Configuration loads from environment variables with optional YAML file overlay. Set `CLUSTER_CONFIG` to a YAML file path; environment variables take precedence over YAML values.

### OCM Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `OCM_TOKEN` | OCM offline token (required) | - |
| `OCM_ENV` | OCM environment: integration, staging, production | integration |
| `OCM_BASE_URL` | Override OCM API URL | - |

### Cluster Selection

| Variable | Description | Default |
|----------|-------------|---------|
| `CLUSTER_ID` | Existing cluster ID (skips create/delete when set) | - |
| `MANAGEMENT_CLUSTER_ID` | Management cluster ID for HCP namespace checks | - |
| `SECTOR_NAME` | Sector name for persistent sector tests | - |

### AWS Infrastructure (for cluster provisioning)

| Variable | Description | Default |
|----------|-------------|---------|
| `AWS_REGION` | AWS region | us-east-1 |
| `AWS_ACCOUNT_ID` | AWS account ID for STS role ARNs | - |
| `SUBNET_IDS` | Comma-separated VPC subnet IDs | - |
| `OIDC_CONFIG_ID` | Pre-created OIDC config ID | - |
| `ACCOUNT_ROLE_PREFIX` | STS account role prefix | - |
| `OPERATOR_ROLE_PREFIX` | STS operator role prefix | - |
| `BILLING_ACCOUNT_ID` | AWS billing account ID for HCP | - |
| `CREATOR_ARN` | ARN of the IAM entity creating the cluster | - |

### AWS Credentials (for CloudTrail validation and AWS API tests)

| Variable | Description |
|----------|-------------|
| `AWS_ACCESS_KEY_ID` | AWS access key ID |
| `AWS_SECRET_ACCESS_KEY` | AWS secret access key |
| `AWS_SESSION_TOKEN` | AWS session token (for temporary credentials) |

### Cluster Parameters

| Variable | Description | Default |
|----------|-------------|---------|
| `CLUSTER_NAME_PREFIX` | Cluster name prefix | e2e |
| `COMPUTE_MACHINE_TYPE` | EC2 instance type | m5.xlarge |
| `COMPUTE_NODES` | Number of compute nodes | 2 |
| `CHANNEL_GROUP` | OCP version channel group | stable |
| `OPENSHIFT_VERSION` | Specific OCP version (empty = latest in channel) | - |

### YAML Configuration File

Example `configs/rosa-hcp-default.yaml`:

```yaml
ocm_env: integration
aws_region: us-east-1
cluster_name_prefix: e2e
compute_machine_type: m5.xlarge
compute_nodes: 2
channel_group: stable
```

Load it via:

```bash
export CLUSTER_CONFIG=configs/rosa-hcp-default.yaml
make test
```

## Label-Based Test Selection

Tests are labeled using Ginkgo v2 labels. Use `--label-filter` to run subsets of tests.

### Label Categories

**Platform** (product variant):
- `Platform:HCP` - ROSA HCP
- `Platform:Classic` - ROSA Classic
- `Platform:OSD-AWS` - OSD on AWS
- `Platform:OSD-GCP` - OSD on GCP

**Area** (test category):
- `Area:ClusterLifecycle`
- `Area:DataPlane`
- `Area:ManagedService`
- `Area:CustomerFeatures`
- `Area:Infrastructure`
- `Area:Upgrade`

**Importance** (criticality):
- `Importance:Critical` - Must pass for release
- `Importance:High` - Important but not blocking
- `Importance:Medium` - Standard test coverage
- `Importance:Low` - Nice to have

**Speed**:
- `Speed:Slow` - Long-running tests (>5 minutes)

**Positivity**:
- `Positivity:Positive` - Expected to succeed
- `Positivity:Negative` - Expected to fail (error handling tests)

### Filter Examples

```bash
# Run only critical HCP tests
LABEL_FILTER="Platform:HCP && Importance:Critical" make test

# Run Managed Service Health area
LABEL_FILTER="Area:ManagedService" make test

# Run fast health checks (exclude slow tests)
LABEL_FILTER="!Speed:Slow" make test

# Combine filters: HCP critical tests in ManagedService area
LABEL_FILTER="Platform:HCP && Importance:Critical && Area:ManagedService" make test

# Run all except upgrade tests
LABEL_FILTER="!Area:Upgrade" make test

# Dry-run to see which tests match a filter
LABEL_FILTER="Platform:HCP && Importance:Critical" make dry-run
```

## Building and Running

### Local Development

```bash
# Build test binary
make build

# Run tests
make test

# Run unit tests
make unit-test

# Lint code
make lint

# Clean build artifacts
make clean
```

### Container Image

```bash
# Build image
make image

# Run in container
podman run --rm \
  -e OCM_TOKEN \
  -e OCM_ENV=staging \
  -e CLUSTER_ID=your-cluster-id \
  rosa-e2e:latest
```

## Architecture

```
rosa-e2e/
├── cmd/
│   └── rosa-e2e/        # CLI entrypoint (not yet implemented, tests run via ginkgo)
├── pkg/
│   ├── config/          # Configuration loading (env vars + YAML)
│   ├── labels/          # Ginkgo label constants for test selection
│   ├── framework/       # OCM connection, cluster CRUD, Kubernetes client
│   └── verifiers/       # Composable cluster health checks
├── test/
│   └── e2e/             # Ginkgo test suite and test cases
└── configs/             # Example YAML configuration files
```

### Design Principles

- **Framework/Test/Verifier Separation**: Framework handles OCM/Kubernetes connections, tests orchestrate, verifiers perform health checks
- **Label-Based Selection**: Tests tagged with platform, area, importance, speed for flexible filtering
- **Ginkgo v2**: Modern BDD test framework with parallel execution, label filtering, and JUnit reporting
- **OCM SDK Direct**: Uses OCM Go SDK directly (not rosa CLI) for cluster lifecycle operations
- **Composable Verifiers**: Reusable health check functions that can be mixed and matched
- **Per-Test Isolation**: Each test cleans up via `DeferCleanup` to prevent state leakage

Modeled after [ARO-HCP e2e test suite](https://github.com/Azure/ARO-HCP/tree/main/test).

## CI Integration

See [docs/ci-setup.md](docs/ci-setup.md) for running in OpenShift CI (Prow).

## Local Testing

See [docs/local-testing.md](docs/local-testing.md) for running tests from your laptop against staging clusters.

## Related Documentation

- **Planning Document**: [ROSA-E2E-PLAN.md](../hcm-design/rosa-e2e/ROSA-E2E-PLAN.md) - Full architecture, test areas, phasing, execution model
- **Jira**:
  - [ROSA-683](https://issues.redhat.com/browse/ROSA-683) - ROSA Downstream CI Test Coverage and Validation (parent initiative)
  - [SREP-3987](https://issues.redhat.com/browse/SREP-3987) - ROSA HCP Managed Service Conformance Tests (Area 3 epic)

## Contributing

This repository follows the [openshift/release contribution guidelines](https://docs.ci.openshift.org/docs/).

When adding tests:
1. Add appropriate labels (Platform, Area, Importance, Speed)
2. Use composable verifiers from `pkg/verifiers/`
3. Clean up resources with `DeferCleanup`
4. Update this README if adding new configuration options
