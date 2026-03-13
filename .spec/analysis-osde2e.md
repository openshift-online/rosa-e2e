# osde2e - Analysis

## Overview

**Repository**: `github.com/openshift/osde2e`
**Language**: Go 1.24
**Framework**: Ginkgo v2 / Gomega (BDD)
**CI System**: Prow (ci-operator) via `openshift/release`
**Container Image**: `quay.io/redhat-services-prod/osde2e-cicada-tenant/osde2e:latest`

## How Tests Are Run

### CLI Entry Point

The `osde2e` binary (built via `cmd/osde2e/main.go`) provides subcommands:

```bash
./out/osde2e test [options]       # Run e2e tests
./out/osde2e provision [options]  # Provision a cluster only
./out/osde2e healthcheck [options] # Run health checks
./out/osde2e cleanup [options]    # Clean up resources
```

### Configuration Hierarchy (lowest to highest precedence)

1. Pre-canned composable configs in `configs/` directory
2. Custom YAML config files via `--custom-config`
3. Environment variables (e.g., `OCM_CLIENT_ID`, `OCM_CLIENT_SECRET`)
4. CLI flags (e.g., `--cluster-id`, `--focus-tests`, `--label-filter`)

### Test Selection

```bash
# Ginkgo label filter
./out/osde2e test --label-filter "E2E && !Upgrade && !Informing"

# Focus/skip patterns
./out/osde2e test --focus-tests "pattern" --skip-tests "pattern"
```

### Pre-Canned Config Files

Composable YAML configs in `configs/`:

| Config | Purpose |
|---|---|
| `aws.yaml`, `gcp.yaml` | Cloud provider selection |
| `rosa.yaml`, `sts.yaml`, `hypershift.yaml` | Cluster type |
| `stage.yaml`, `prod.yaml`, `int.yaml` | OCM environment |
| `e2e-suite.yaml` | Full E2E suite label filter |
| `sanity.yaml` | Basic smoke tests |
| `informing-suite.yaml` | Informing signal tests |
| `upgrade-to-latest*.yaml` | Upgrade scenarios |
| `fips.yaml`, `fedramp.yaml` | Compliance modes |

### Test Execution Flow (Orchestrator)

1. **Config loading** - Merge files, env vars, CLI flags
2. **Provision cluster** (or reuse existing via `--cluster-id`)
3. **Execute tests** in phases:
   - Install phase (pre-upgrade)
   - Upgrade phase (if configured)
   - Post-upgrade tests
4. **Upload artifacts** to S3
5. **Analyze logs** (AI-powered failure analysis via Claude API)
6. **Report** - JUnit XML, must-gather
7. **Post-process cluster** - Extend expiry, update metadata
8. **Cleanup** - Delete cluster if configured

### CI/CD Integration (Prow)

- CI configs in `openshift/release` at `ci-operator/config/openshift/osde2e/`
- Periodic jobs run daily/nightly for various cluster types
- Results published to TestGrid dashboards
- Slack notifications with AI failure analysis

### Build

```bash
make build       # Build osde2e binary to ./out/osde2e
make build-image # Build container image
make check       # Lint, test, provider check
```

## What Tests Are Run

### Cluster Types Supported

- **ROSA Classic** (STS and non-STS)
- **ROSA HCP** (HyperShift)
- **OSD on AWS** (CCS and Red Hat managed)
- **OSD on GCP** (CCS and WIF)
- **Custom** clusters via kubeconfig

### Provider Interface (SPI)

The `Provider` interface in `pkg/common/spi/provider.go` defines cluster lifecycle operations. Two providers are registered:
- **ROSA Provider** (`pkg/common/providers/rosaprovider/`)
- **OCM Provider** (`pkg/common/providers/ocmprovider/`)

### Provision/Deprovision Flow

1. Load cluster context from environment
2. Provision or reuse cluster via provider
3. Wait for OCM provisioning (60+ min standard, 30 min HyperShift)
4. Run health checks (node readiness, pod readiness, CVO status, certificates, operators)
5. Install addons if configured
6. Run tests
7. Conditionally delete cluster (async)

### Test Categories

**Core Verification** (`pkg/e2e/verify/`):
- Encrypted storage, FIPS mode, ImageStreams
- Load balancers, pod networking, routing
- SCCs, PVC management, user workload monitoring

**OSD-Specific** (`pkg/e2e/osd/`):
- Daemonset health, dedicated admin, inhibitions
- Machine health checks, node labels/taints
- OCM integration (metrics, quay fallback)
- OLM, privileged pods, infra node rebalancing

**Operators** (`pkg/e2e/operators/`):
- RBAC permissions, operator lifecycle, prune jobs

**HyperShift** (`pkg/e2e/openshift/hypershift/`):
- Installation verification

**State/Monitoring** (`pkg/e2e/state/`):
- Prometheus alert validation, cluster state monitoring

**Ad-Hoc Test Images** (`pkg/e2e/adhoctestimages/`):
- External container-based operator tests (AWS VPC Endpoint, Cloud Ingress, MUO, must-gather-operator, etc.)

### Health Checks (`pkg/common/cluster/healthchecks/`)

- Node readiness
- Pod readiness (filters system pods)
- ClusterVersionOperator status
- Certificate validation
- Machine readiness
- Operator status
- Replica set validation

## What Tests Are Defined

### Test Suite Structure

```
pkg/e2e/
├── verify/           # Core cluster verification
│   ├── encrypted_storage.go
│   ├── fips.go
│   ├── imagestreams.go
│   ├── loadbalancers.go
│   ├── pods.go
│   ├── routes.go
│   ├── sccs.go
│   ├── storage.go
│   └── user_workload_monitoring.go
├── osd/              # OSD-specific tests
│   ├── daemonsets.go
│   ├── dedicatedadmin.go
│   ├── inhibitions.go
│   ├── machinehealthcheck.go
│   ├── nodelabels.go
│   ├── ocm.go
│   ├── olm.go
│   ├── privileged.go
│   └── rebalance_infra_nodes.go
├── operators/        # Operator tests
├── proxy/            # Proxy configuration
├── state/            # Prometheus alerts, cluster state
├── openshift/hypershift/ # HyperShift verification
├── adhoctestimages/  # External container tests
├── workloads/        # Custom workloads
└── e2e.go            # Main orchestration
```

### CI Job Types

- **ROSA BYOVPC Proxy** - Install/Post-Install proxy tests
- **OSD AWS Upgrade** - Y-1→Y, Z-1→Z, Y→Y+1 upgrade scenarios
- **OSD AWS SREP Operator Informing Suite** - Operator validation
- **TRT Nightly** - OSD (AWS/GCP) and ROSA (Classic STS/HCP) nightly runs

## Strengths

- Already runs in Prow via ci-operator
- Supports all managed OpenShift cluster types
- Composable configuration system
- Full cluster lifecycle management (provision, test, deprovision)
- AI-powered failure analysis
- Extensive health check library

## Weaknesses

- Heavy framework - manages entire cluster lifecycle, hard to run individual tests quickly
- Platform-focused: tests cluster health and OSD operators, not OCM API contracts
- Complex configuration with many overlapping options
- Monolithic orchestrator couples provisioning, testing, and reporting
- No per-test cluster isolation - all tests share one cluster per run
