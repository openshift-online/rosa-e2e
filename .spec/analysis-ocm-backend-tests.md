# OCM Backend Tests - Analysis

## Overview

**Repository**: `gitlab.cee.redhat.com:service/ocmci` (fork at `gitlab.cee.redhat.com:tiwillia/ocm-backend-tests`)
**Language**: Go 1.23
**Framework**: Ginkgo v2 / Gomega (BDD)
**CI System**: Tekton (Konflux)
**Container Image**: `quay.io/redhat-services-prod/ocmci/ocmci:latest`

## How Tests Are Run

### Execution Tools

- **ocmtest**: Primary test execution tool from `ocmci-common` repo. Manages full job lifecycle (cluster provisioning, test execution, cleanup, reporting).
- **ginkgo CLI**: Direct test execution with label/focus filters for local development.
- **run_profile.py**: Python wrapper for cluster provisioning via profiles.

### Command Patterns

```bash
# Direct Ginkgo (local dev)
ginkgo -v -focus @id_21505 ./cases/cms
ginkgo -v -focus @smoke ./cases/cms
ginkgo -v --timeout 2h -focus @label ./cases/ams

# Full job lifecycle (CI)
ocmtest test --service=cms --job <job_name>

# Cluster provisioning
python3 run_profile.py --profiles $CLUSTER_PROFILE --prepare-only True
python3 run_profile.py --profiles $CLUSTER_PROFILE --just-clean True
```

### Build

```bash
make ginkgo-install    # Install Ginkgo v2
make ocmtest-install   # Install ocmtest
make cmds              # Generate test binaries (cms, ams, osdfm, osl, quotacost)
```

### Configuration

Tests are configured via environment variables:

| Variable | Purpose |
|---|---|
| `OCM_ENV` | Target environment: staging, production, integration, local |
| `CLUSTER_PROFILE` | Cluster template name (e.g., `rosa-hcp-ad`, `rosa-hcp-bkp`) |
| `AWS_SHARED_CREDENTIALS_FILE` | AWS credentials for provisioning |
| `FAKE_CLUSTER` | true/false - use fake clusters for API-only tests |
| `QE_FLAG` | Test run identifier (gate-int, gate-stg, gate-prd) |
| `NAME_PREFIX` | Cluster name prefix (typically "cs-ci") |

### CI Pipeline (Tekton/Konflux)

1. **Pre-merge**: Build container image, validate commit format, run precheck gating jobs
2. **Post-merge**: Build release image, publish to production registry
3. **Gating jobs**: Triggered by component changes, run profile-based test suites in parallel

### Docker Image Contents

The production image bundles: OCM CLI, OpenShift CLI (oc), ocmtest, pre-compiled test binaries, AWS CLI, Google Cloud CLI, Python 3, and Red Hat CA certificates.

## What Tests Are Run

### Services Tested

| Service | Directory | Description |
|---|---|---|
| CMS (Clusters Management Service) | `cases/cms/` | 100+ test files - cluster lifecycle, ROSA, OSD |
| AMS (Account Management Service) | `cases/ams/` | 40+ test files - accounts, subscriptions, quotas |
| OSDFM (OSD Fleet Manager) | `cases/osdfm/` | 60+ test files - fleet orchestration, service clusters |
| OSL (OpenShift Logging) | `cases/osl/` | Logging service validation |

### Test Types

- **Day1**: Cluster provisioning and initial setup
- **Day1-Post**: Verification of provisioned cluster configuration
- **Day2**: Post-creation operations (upgrades, scaling, IDP, networking)
- **Destructive**: Tests that modify/destroy critical resources
- **E2E**: Full cluster lifecycle workflows
- **Smoke**: Basic sanity checks
- **Feature-specific**: Backup, IDMS, SDN migration, region enablement

### Test Lifecycle

1. **BeforeSuite** - Initialize connections, prepare output directories
2. **PreCheck** - Environment validation
3. **Pre-Cluster-Creation** - Set up VPC, credentials
4. **Cluster Creation** - Provision via OCM API using profile
5. **Day1Post Tests** - Verify initial cluster configuration
6. **Day2 Tests** - Upgrade, scaling, IDP, other operations
7. **Destructive Tests** - Modify critical components
8. **Post-Cleanup** - Remove resources, generate reports
9. **AfterSuite** - Close connections, finalize

### Connection Types

Tests use multiple authenticated connections to test different permission levels:
`OrgAdminConnection`, `OrgMemberConnection`, `SuperAdminConnection`, `RosaConnection`, `TrialConnection`, `STSSupportJumpRoleConnection`, `QuotaConnection`

## What Tests Are Defined

### Label System

Tests are labeled across multiple dimensions:

| Category | Values |
|---|---|
| Importance | Critical, High, Medium, Low |
| Runtime | day1, day1-post, day2, upgrade, destructive, destroy |
| Environment | Prod, Stage, Int (with Canary variants) |
| Features | feature-backup, feature-upgrade, feature-idms, feature-sdn-migration |
| Scope | E2E, Smoke, Destructive |

Label filter syntax: `day1-post&&!Exclude&&!E2E`, `(Critical,High)`, `feature-backup`

### Cluster Profiles

25+ pre-defined profiles in `data/ci/profiles/cms-cluster-profile.yml`:

| Profile | Type | Key Features |
|---|---|---|
| `rosa-hcp-ad` | ROSA HCP | Multi-AZ, autoscale, BYOK, FIPS, KMS, NLB, audit logging |
| `rosa-hcp-bkp` | ROSA HCP | Backup/restore testing (custom schedule: every 5 mins) |
| `rosa-hcp-pl` | ROSA HCP | Private Link |
| `rosa-hcp-zero-egress` | ROSA HCP | Private Link + proxy |
| `rosa-hcp-shared-vpc` | ROSA HCP | Shared VPC, UnManaged OIDC |
| `rosa-sts-ad` | ROSA Classic | STS with all Day1 features |
| `osd-ccs-aws-ad` | OSD AWS | CCS Multi-AZ |
| `osd-ccs-gcp-ad` | OSD GCP | GCP CCS Multi-AZ |

### Key CMS Test Files

| File | Focus |
|---|---|
| `hcp_backup_test.go` | HCP backup schedule verification, DR secrets, BSL |
| `cluster_e2e_test.go` | Full cluster lifecycle |
| `hcp_creation_test.go` | HCP cluster creation |
| `hcp_node_pools_test.go` | Node pool operations |
| `cluster_provision_validation_test.go` | Provision validation |
| `prechecking_test.go` | Environment validation |

### ROSA HCP Backup/Restore Test (Target Test)

**Job**: `cs-rosa-hcp-backup-restore-integration-main`
**Profile**: `rosa-hcp-bkp`
**Label Filter**: `feature-backup`

Key test case (`@id_85063` in `hcp_backup_test.go`):
- Triggers backup from schedule
- Waits for backup completion (30-min timeout)
- Verifies schedule configuration (cron expressions)
- Checks DR secret existence
- Validates backup storage location (BSL) availability
- Confirms backup object creation

### Job Definition Structure

```yaml
tests:
- as: <job-name>
  steps:
    cluster_profile: <profile>
    env: {QE_FLAG, NAME_PREFIX}
    test:
      - scope: 'Day1Post'
        case_label_filter: 'day1-post&&!Exclude&&!E2E'
      - scope: 'Day2'
        case_label_filter: 'day2&&(Critical,High)&&!Exclude'
    workflow: cms-gating-test
```

## Strengths

- Comprehensive API-level testing across all OCM services
- Rich labeling system enables precise test selection
- Profile-based cluster configuration is declarative and reusable
- Multiple connection types test authorization boundaries

## Weaknesses

- Tightly coupled to OCM QE internal tooling (`ocmtest`, Konflux)
- Not designed to run in Prow/ci-operator
- Complex setup requirements (many environment variables, tokens, credentials)
- Test execution depends on the `ocmtest` binary which manages cluster lifecycle externally
