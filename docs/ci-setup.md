# CI Setup Guide

This document explains how rosa-e2e runs in OpenShift CI (Prow) and how to configure it for periodic and pre-submit testing.

## Overview

rosa-e2e runs in Prow via ci-operator as a periodic job against a persistent ROSA sector. The test suite does not provision or destroy clusters on every run -- instead, it validates a long-lived "rosa-e2e" sector in the staging environment.

This approach is required because:
- ROSA HCP architecture does not support ephemeral sector provisioning (requires multi-cluster ACM/Hive setup)
- Full sector standup takes hours and requires manual steps
- Persistent sector allows continuous validation of sector health over time

## Persistent Sector Model

**Sector**: `rosa-e2e` (staging environment)

The sector includes:
- Service Cluster (SC) running ACM hub, cert-manager, Hive
- Management Cluster (MC) running HyperShift operator, RMO, AVO
- One or more Hosted Clusters (HC) for data plane tests

**Test Execution Flow**:
1. Connect to OCM staging API
2. Look up cluster(s) in the rosa-e2e sector via `CLUSTER_ID` or `SECTOR_NAME`
3. Run health checks against all three infrastructure tiers
4. Run data plane validation against HC
5. Report results via JUnit XML

**No cluster provisioning/teardown** - tests assume infrastructure exists.

## Required Prow Secrets

rosa-e2e requires the following secrets in the Prow cluster (namespace `test-credentials`):

### OCM Authentication

```yaml
# Secret: rosa-e2e-ocm-token
# Key: token
OCM_TOKEN: "offline-token-from-console-redhat-com"
```

Get offline tokens from https://console.redhat.com/openshift/token

### AWS Credentials (for CloudTrail validation and AWS API tests)

```yaml
# Secret: rosa-e2e-aws-credentials
# Keys: aws_access_key_id, aws_secret_access_key, aws_session_token
AWS_ACCESS_KEY_ID: "AKIAxxxxxxxxxxxx"
AWS_SECRET_ACCESS_KEY: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
AWS_SESSION_TOKEN: "xxxxxxxxxx"  # Optional, for temporary credentials
```

These credentials are used for:
- CloudTrail event validation (SREP-3822)
- AWS tag verification
- IAM policy validation

### Cluster Identifiers

```yaml
# Secret: rosa-e2e-cluster-config
# Keys: cluster_id, management_cluster_id
CLUSTER_ID: "abc123def456"  # Hosted Cluster ID in rosa-e2e sector
MANAGEMENT_CLUSTER_ID: "xyz789uvw012"  # Management Cluster ID for HCP namespace checks
```

### Management Cluster Access

```yaml
# Secret: rosa-e2e-mc-kubeconfig
# Key: kubeconfig
MC_KUBECONFIG: |
  # Base64-encoded kubeconfig from backplane login
  # Obtained via: ocm backplane login <cluster-id> --manager
```

Backplane integration provides MC access in CI, allowing tests to validate HCP namespace health, HostedCluster CRs, NodePool CRs, and managed operators (RMO, AVO, audit-webhook) running on the management cluster.

## ci-operator Configuration

The rosa-e2e ci-operator config lives in `openshift/release` at:

```
ci-operator/config/openshift-online/rosa-e2e/openshift-online-rosa-e2e-main.yaml
```

### Basic Configuration

```yaml
build_root:
  image_stream_tag:
    name: release
    namespace: openshift
    tag: golang-1.24

images:
- dockerfile_path: Containerfile
  to: rosa-e2e

tests:
- as: unit
  commands: make unit-test
  container:
    from: src

- as: lint
  commands: make lint
  container:
    from: src
```

This provides:
- **Unit tests** - Run on every PR
- **Lint checks** - Run on every PR
- **Container image build** - Validate Containerfile builds

### Periodic E2E Jobs (to be added)

Once the test suite is stable (>95% pass rate), add periodic jobs for each topology:

```yaml
# HCP periodic job
- as: e2e-rosa-hcp-staging
  cron: "0 */6 * * *"  # Every 6 hours
  steps:
    test:
    - as: e2e
      commands: |
        export OCM_ENV=staging
        export AWS_REGION=us-east-1
        export CLUSTER_TOPOLOGY=hcp
        LABEL_FILTER="Platform:HCP" make test
      credentials:
      - mount_path: /secrets/ocm
        name: rosa-e2e-ocm-token
        namespace: test-credentials
      - mount_path: /secrets/aws
        name: rosa-e2e-aws-credentials
        namespace: test-credentials
      - mount_path: /secrets/cluster
        name: rosa-e2e-cluster-config
        namespace: test-credentials
      from: rosa-e2e
      resources:
        requests:
          cpu: 1000m
          memory: 2Gi

# Classic STS periodic job
- as: e2e-rosa-classic-staging
  cron: "0 */6 * * *"
  steps:
    test:
    - as: e2e
      commands: |
        export OCM_ENV=staging
        export AWS_REGION=us-east-1
        export CLUSTER_TOPOLOGY=classic
        LABEL_FILTER="Platform:Classic" make test
      credentials:
      - mount_path: /secrets/ocm
        name: rosa-e2e-ocm-token
        namespace: test-credentials
      - mount_path: /secrets/aws
        name: rosa-e2e-aws-credentials
        namespace: test-credentials
      - mount_path: /secrets/cluster
        name: rosa-e2e-classic-cluster-config
        namespace: test-credentials
      from: rosa-e2e
      resources:
        requests:
          cpu: 1000m
          memory: 2Gi
```

## Prow Job Workflow

1. **Pre-submit (PR checks)**:
   - Unit tests (`make unit-test`)
   - Lint checks (`make lint`)
   - Container image build validation

2. **Periodic (nightly/every 6 hours)**:
   - Full E2E test suite against staging sector
   - JUnit results uploaded to Sippy
   - Alerts on sustained failures

3. **On-demand (manual trigger)**:
   - `/test e2e-rosa-hcp-staging` comment on PR
   - Useful for validating test changes before merge

## Running Locally vs CI

### Local Development

```bash
# Export secrets
export OCM_TOKEN="your-offline-token"
export CLUSTER_ID="your-cluster-id"
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"

# Run tests
make test
```

### CI Execution

In CI, secrets are mounted as files and loaded via environment variables in the Prow job step. The ci-operator config maps secret keys to environment variables automatically.

**Secret mounting**:
- `/secrets/ocm/token` -> `OCM_TOKEN` env var
- `/secrets/aws/aws_access_key_id` -> `AWS_ACCESS_KEY_ID` env var
- `/secrets/cluster/cluster_id` -> `CLUSTER_ID` env var

## Test Results and Reporting

### JUnit XML Output

All tests generate JUnit XML reports via Ginkgo:

```bash
make test  # Generates junit-report.xml
```

Prow uploads these to:
- **Sippy Dashboard**: https://sippy.dptools.openshift.org/
- **Prow Job Artifacts**: https://prow.ci.openshift.org/

### Sippy Integration (SREP-3882)

Once rosa-e2e achieves >95% sustained pass rate, it can be promoted to OCP-blocking status. This requires:

1. Consistent nightly runs with JUnit reporting
2. >95% pass rate over 30 days
3. Sippy dashboard showing test trends
4. Documented test ownership and escalation

## Adding New Tests

When adding new test cases to rosa-e2e:

1. **Add appropriate labels** (Platform, Area, Importance, Speed)
2. **Test locally** against staging sector
3. **Open PR** in openshift-online/rosa-e2e
4. **Wait for pre-submit checks** (unit tests, lint)
5. **Merge** - periodic jobs will pick up new tests automatically

No ci-operator config changes needed unless:
- Adding new secrets
- Changing build dependencies
- Modifying test execution commands

## Troubleshooting

### "OCM_TOKEN not set" error in CI

Check that the secret exists and is mounted correctly:

```bash
# In Prow job pod
ls -la /secrets/ocm/
cat /secrets/ocm/token
```

### "Cluster not found" error

Verify `CLUSTER_ID` points to a cluster in the rosa-e2e sector in staging:

```bash
ocm login --url staging --token "$OCM_TOKEN"
ocm describe cluster "$CLUSTER_ID"
```

### AWS credential errors

Verify AWS credentials are valid and have permission to:
- `cloudtrail:LookupEvents`
- `ec2:DescribeInstances`
- `ec2:DescribeTags`

### Tests fail in CI but pass locally

Common causes:
- Network restrictions in Prow (firewall rules, egress policies)
- Different OCM environment (check `OCM_ENV`)
- Timing issues (CI may be slower, add retries)

## Future Work

### OSD GCP Support

Config fields (`GCP_PROJECT_ID`, `GCP_REGION`) are in place. Remaining work:
- `CreateOSDGCPCluster()` with WIF and non-WIF variants
- GCP-specific verifiers (GCE tags, service accounts)
- `Platform:OSD-GCP` labeled tests
- CI job with GCP credentials

### Pre-submit Testing (RRP)

The ROSA Regional Platform (RRP) team is building pre-submit testing on the new EKS-based architecture. This will allow ephemeral cluster provisioning for PR validation. See [LANDSCAPE-AND-ALIGNMENT.md](../../hcm-design/rosa-e2e/LANDSCAPE-AND-ALIGNMENT.md) for details.

### Multi-Region Testing

Expand periodic jobs to test multiple regions:
- US (us-east-1, us-west-2)
- EU (eu-west-1, eu-central-1)
- APAC (ap-southeast-1, ap-northeast-1)

### Critical Alert Verification

Add `VerifyNoCriticalAlerts` verifier to lifecycle tests (both HCP and Classic) to catch pre-GA alert bugs after cluster creation.

### OCP-Blocking Promotion

Goal: Promote rosa-e2e to OCP-blocking status by demonstrating:
- >95% pass rate for 30 days
- Automated alerting on failures
- Clear ownership and escalation path
- Integration with release-controller

## References

- **Prow Documentation**: https://docs.ci.openshift.org/docs/
- **ci-operator**: https://docs.ci.openshift.org/docs/architecture/ci-operator/
- **Sippy**: https://sippy.dptools.openshift.org/
- **ROSA-683** (Jira): https://issues.redhat.com/browse/ROSA-683
- **SREP-3987** (Jira): https://issues.redhat.com/browse/SREP-3987
