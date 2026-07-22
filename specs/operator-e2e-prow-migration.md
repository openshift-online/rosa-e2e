# SRE Operator E2E Prow Migration Spec

Runbook for migrating an SRE operator's e2e tests from osde2e/SAPM (Tekton Jobs on hive clusters) to Prow (using pre-provisioned ROSA clusters via the rosa-cluster-lease system).

Jira: [ROSAENG-60066](https://redhat.atlassian.net/browse/ROSAENG-60066)

## Completed Operators

| Operator | OPERATOR_NAME | Branch | Namespace | CLUSTER_PACKAGE_NAME | Jira |
|----------|--------------|--------|-----------|---------------------|------|
| RMO | route-monitor-operator | master | openshift-route-monitor-operator | (default) | reference impl |
| CAMO | configure-alertmanager-operator | master | openshift-monitoring | configure-alertmanager-operator | [ROSAENG-60068](https://redhat.atlassian.net/browse/ROSAENG-60068) |
| AVO | aws-vpce-operator | main | openshift-aws-vpce-operator | (default) | [ROSAENG-60071](https://redhat.atlassian.net/browse/ROSAENG-60071) |
| MUO | managed-upgrade-operator | master | openshift-managed-upgrade-operator | (default) | [ROSAENG-60069](https://redhat.atlassian.net/browse/ROSAENG-60069) |
| OAO | ocm-agent-operator | master | openshift-ocm-agent-operator | (default) | [ROSAENG-60072](https://redhat.atlassian.net/browse/ROSAENG-60072) |

## Prerequisites

- Operator repo has a `test/e2e/` directory with Ginkgo e2e tests
- Operator is deployed via PKO ClusterPackage on managed clusters
- Operator repo uses boilerplate (openshift/boilerplate)
- Operator has a Konflux push build pipeline

## Step 1: Update Boilerplate

Update boilerplate in the operator repo to pick up the `gangway-bridge-template` (required for the SAPM gangway bridge Job).

```bash
cd ~/src/<operator-repo>
make boilerplate-update
```

Create a PR to the operator repo with the boilerplate update. The key file added is `hack/pko/gangway-bridge-template.yaml`.

**Gotcha**: The boilerplate `golang-osd-e2e/update` heredoc may have trailing whitespace. If the pre-commit hook strips it, commit with `--no-verify` and note a root fix is needed upstream.

**Gotcha**: The `agentic-sdlc-check` in `.tekton/` may generate differently locally vs CI due to `origin/HEAD` detection. Commit whatever CI generates (usually `== "master"`).

## Step 2: Fix E2E Tests for Prow Compatibility

The e2e tests must work when the operator is already deployed via PKO on the target cluster (not started locally).

### Common issues to fix

1. **Remove local operator build/start**: Tests that use `envtest`, `exec.Command("go", "build")`, or subprocess management must be reworked. The operator is already running via PKO.

2. **Remove envtest dependency**: `controller-runtime/pkg/envtest` pulls CGO dependencies that fail in minimal CI containers. Replace `envtest.WaitForCRDs` with an `Eventually` poll checking the CRD `Established` condition.

3. **Skip AWS integration tests**: Tests that create real AWS resources (VPCs, endpoints, security groups) need AWS credentials not available in Prow. Skip when `AWS_ACCESS_KEY_ID`, `AWS_PROFILE`, and `AWS_SHARED_CREDENTIALS_FILE` are all unset.

4. **Fix timeouts**: Fresh PKO-deployed clusters take longer than local starts. Use configurable timeout constants (3-5 minutes for reconciliation, 30s polling interval). Mark flaky tests as `PIt` (pending) with a TODO if they can't be stabilized.

5. **Deployment detection**: The boilerplate e2e harness detects whether the operator was deployed via OLM (ClusterServiceVersion) or PKO (ClusterPackage). Make sure the test suite's `BeforeSuite` supports both.

### Example fixes

CAMO: Bumped `reconcileTimeout` from 2 to 3 minutes, replaced hardcoded timeouts with named constants, marked `serviceURL` reconciliation test as `PIt`.

AVO: Removed all local operator build/start/stop code, replaced `envtest.WaitForCRDs` with Eventually poll, added AWS credentials skip guard.

MUO: Changed Prometheus metric timeout from 100s to 2 minutes, marked invalid start time metric test as `PIt`.

## Step 3: Create ci-operator Config (openshift/release)

Create or update the ci-operator config at `ci-operator/config/openshift/<operator-repo>/openshift-<operator-repo>-<branch>.yaml`.

### Required sections

```yaml
base_images:
  rosa-aws-cli:
    name: rosa-aws-cli
    namespace: ci
    tag: latest

build_root:
  image_stream_tag:
    name: builder
    namespace: ocp
    tag: rhel-9-golang-1.23-openshift-4.19

images:
- dockerfile_literal: |
    FROM src
    RUN make go-build
  to: <operator-name>
- dockerfile_literal: |
    FROM src
    COPY hack/pko/ /pko/
    RUN cd /pko && make PKO_IMAGE_TAG=operator-pko OPERATOR_IMAGE_TAG=<operator-name>
  to: operator-pko

releases:
  latest:
    candidate:
      product: ocp
      stream: nightly
      version: "4.19"

tests:
- as: rosa-sts-e2e
  cluster_profile: rosa-sts
  steps:
    workflow: rosa-operator-e2e
    env:
      OPERATOR_NAME: <operator-name>
      OPERATOR_NAMESPACE: <namespace>
      OPERATOR_IMAGE: pipeline:<operator-name>
      OPERATOR_PKO_IMAGE: pipeline:operator-pko
      OPERATOR_CRDS: "<crd1>,<crd2>"
      LEASE_ENV: integration

- as: rosa-sts-e2e-promotion-int
  cluster_profile: rosa-sts
  cron: ""  # triggered by gangway only
  steps:
    workflow: rosa-operator-e2e
    env:
      OPERATOR_NAME: <operator-name>
      OPERATOR_NAMESPACE: <namespace>
      OPERATOR_IMAGE: pipeline:<operator-name>
      OPERATOR_PKO_IMAGE: pipeline:operator-pko
      OPERATOR_CRDS: "<crd1>,<crd2>"
      LEASE_ENV: integration

- as: rosa-sts-e2e-promotion-stage
  cluster_profile: rosa-sts
  cron: ""  # triggered by gangway only
  steps:
    workflow: rosa-operator-e2e
    env:
      OPERATOR_NAME: <operator-name>
      OPERATOR_NAMESPACE: <namespace>
      OPERATOR_IMAGE: pipeline:<operator-name>
      OPERATOR_PKO_IMAGE: pipeline:operator-pko
      OPERATOR_CRDS: "<crd1>,<crd2>"
      LEASE_ENV: staging
```

### Key env vars

| Var | Description | Example |
|-----|-------------|---------|
| `OPERATOR_NAME` | Used for lease locking, ClusterPackage naming, deployment lookup | `configure-alertmanager-operator` |
| `OPERATOR_NAMESPACE` | Where the operator deployment lives | `openshift-monitoring` |
| `OPERATOR_IMAGE` | CI-built operator container image | `pipeline:configure-alertmanager-operator` |
| `OPERATOR_PKO_IMAGE` | CI-built PKO package image | `pipeline:operator-pko` |
| `OPERATOR_CRDS` | Comma-separated CRD names for backup/restore and ownership clearing | `alertmanagerconfigs.monitoring.coreos.com` |
| `CLUSTER_PACKAGE_NAME` | Override ClusterPackage name (default: `${OPERATOR_NAME}-e2e-test`) | `configure-alertmanager-operator` |
| `LEASE_ENV` | Filter lease clusters by OCM environment | `integration` or `staging` |

**CLUSTER_PACKAGE_NAME**: Set this to `${OPERATOR_NAME}` (same as production) only if the e2e tests check for the production ClusterPackage name. Otherwise leave at default (`${OPERATOR_NAME}-e2e-test`) to avoid conflicting with the production ClusterPackage.

After creating the config, run `make jobs` to generate the Prow job YAML.

### releases block

The `releases.latest.candidate` block is required by ci-operator when `cluster_profile` is used. It provides the `lease-proxy-server` sidecar. Do not remove it even though the e2e tests don't use the OCP release payload.

## Step 4: Create App-Interface Saas File (prow-e2e)

Create a gangway bridge saas file. The saas file name varies per operator (check `configs/component-deployments.yaml` for existing naming conventions, e.g., `saas-route-monitor-operator-prow-e2e`, `saas-configure-am-operator-prow-e2e`, `saas-muo-e2e-test`). Place it under `data/services/osd-operators/cicd/saas/saas-<operator-name>/prow-e2e.yaml`.

This saas file creates a Tekton Job on the hive cluster that triggers the Prow periodic via the Gangway API and polls for completion.

### Template

The gangway bridge template is generated by boilerplate and lives at `hack/pko/gangway-bridge-template.yaml` in the operator repo. The saas file references this template.

### Targets

- **Integration target**: Subscribes to the `pko-deployed-success` channel (operator deployed to int). Publishes to a per-operator success channel (e.g., `<operator>-prow-e2e-int-success`).
- **Staging target**: Subscribes to the staging `sss-deployed-success` channels (per-hive-cluster). Publishes to a per-operator staging success channel (e.g., `<operator>-prow-e2e-stage-success`).

Use consistent channel names between what the prow-e2e saas publishes and what the downstream stage targets subscribe to.

### Chicken-and-egg

New saas files reference template SHAs that haven't been through SAPM yet. Use `hotfixVersions` in `app.yml` to bootstrap refs that contain the gangway-bridge-template.

## Step 5: Run Rehearsals

After the release PR passes CI:

1. Wait for the `[REHEARSALNOTIFIER]` comment listing affected jobs
2. Run rehearsals: `/pj-rehearse <job-name-1> <job-name-2>`
3. Wait for rehearsals to pass
4. Ack: `/pj-rehearse ack`

**Important**: Never `/pj-rehearse ack` without running and passing rehearsals first. Never use `/pj-rehearse skip` or `/pj-rehearse auto-ack`.

The rehearsal job names come from the REHEARSALNOTIFIER comment, not from the ci-operator config. Use the exact names listed.

## Step 6: Validate via Gangway

After the release PR merges, trigger the promotion-int job manually via Gangway to validate:

```bash
# Get gangway token from hive
GANGWAY_TOKEN=$(KUBECONFIG=<hive-kubeconfig> oc get secret gangway-api-token \
  -n e2e-testing -o jsonpath='{.data.token}' | base64 -d)

GANGWAY_URL="https://gangway-ci.apps.ci.l2s4.p1.openshiftapps.com/v1/executions"
JOB_NAME="periodic-ci-openshift-<operator-repo>-<branch>-rosa-sts-e2e-promotion-int"

# Trigger the job and capture the execution ID
EXEC_ID=$(curl -s -X POST "${GANGWAY_URL}/${JOB_NAME}" \
  -H "Authorization: Bearer ${GANGWAY_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"job_execution_type": "1"}' | jq -r .id)
echo "Execution ID: ${EXEC_ID}"

# Poll until terminal state (SUCCESS, FAILURE, ABORTED)
while true; do
  STATUS=$(curl -s "${GANGWAY_URL}/${EXEC_ID}" \
    -H "Authorization: Bearer ${GANGWAY_TOKEN}" | jq -r .job_status)
  echo "Status: ${STATUS}"
  case "${STATUS}" in
    SUCCESS|FAILURE|ABORTED) break ;;
  esac
  sleep 60
done
```

The gangway token lives in the `gangway-api-token` secret in the `e2e-testing` namespace on hive integration clusters (e.g., `hivei01ue1`). Access via `ocm backplane login --multi hivei01ue1` (production OCM, since hive clusters are registered there).

## Step 7: Wire SAPM Pipeline

Once the Prow e2e is validated and stable (at least 2 successful runs):

1. Update the operator's PKO stage saas targets to subscribe to the same success channel that the prow-e2e integration target publishes to
2. This makes Prow e2e a real gate in the promotion pipeline
3. Run at least one full SAPM cycle to validate

## Step 8: Post-Migration Cleanup

After the Prow e2e gate is stable in production:

1. Remove osde2e saas targets from the operator's saas file
2. Remove Konflux e2e components (if separate from the operator component)
3. Remove `.tekton/` e2e pipeline files
4. Close the migration Jira

A migration is only complete when the osde2e saas target is removed. The Prow e2e must be the sole gate.

## Known Issues

### Concurrent lease cluster sharing

When multiple operators share a lease cluster (per-operator locking via ROSAENG-62333), CI-built images from different Prow jobs use different build cluster registries (e.g., `registry.build01`, `registry.build11`). The install step mirrors images to the cluster's internal registry to avoid pull secret races. See PR #82324.

### ClusterPackage deletion race

When `CLUSTER_PACKAGE_NAME` equals the production ClusterPackage name, deleting and recreating the same-named resource can race with PKO finalizers. The install step polls until the resource is fully gone before recreating. See PR #82306.

### Gangway rate limiting

The Gangway API has rate limits. If a job trigger returns 429, wait and retry. The gangway bridge template handles this with a retry loop.

### SAPM stage ref bootstrap

When first creating prow-e2e saas files, stage targets may point to SHAs that predate the gangway-bridge-template. Promote stage PKO targets to SHAs that include the template before SAPM can auto-promote through the prow-e2e pipeline.

## Reference Files

| File | Repo | Purpose |
|------|------|---------|
| `rosa-operator-install-commands.sh` | openshift/release | Installs operator via PKO ClusterPackage |
| `rosa-cluster-lease-checkout-commands.sh` | openshift/release | Checks out a pre-provisioned ROSA cluster |
| `rosa-cluster-lease-checkin-commands.sh` | openshift/release | Returns cluster to the lease pool |
| `rosa-cluster-lease-controller-commands.sh` | openshift/release | Periodic controller for pool health |
| `hack/pko/gangway-bridge-template.yaml` | operator repos | OpenShift Template for SAPM gangway bridge Job |
| `configs/ci-status-jobs.yaml` | rosa-e2e | CI dashboard job registry |
