---
name: run-local-test
description: Provision a dedicated ROSA HCP test cluster in the user's OCM staging environment, run the rosa-e2e suite against it (customer or platform plane), and deprovision everything afterwards. Use for requests like "run local e2e tests", "spin up a test cluster and run the customer plane specs", or "validate feature X locally".
---

# Run Local E2E Test

Provision a throwaway cluster with `scripts/provision-e2e-cluster.sh`, run the
requested tests, and **always deprovision with `scripts/deprovision-e2e-cluster.sh`
at the end — including when tests fail**. A forgotten cluster keeps burning
dev-account quota until it is cleaned up manually.

## Arguments

Parse `$ARGUMENTS` as `{cluster-type} {test-type}`; both are optional.

- **cluster-type** (default `hcp`): `hcp` | `zero-egress` | `kms` | `shared-vpc` | `external-auth` | `autoscaling`
- **test-type** (default `customer`): `customer` | `platform`

The provision script only creates ROSA HCP clusters. If the user asks for
`classic`, do not provision — instead run tests against an existing cluster as
described in docs/local-testing.md ("Quick Start: Test Against Any Existing
Cluster") and confirm which cluster to use.

## Step 1: Preflight — all checks must pass before provisioning

Run these in one shell call (shell state does not persist between calls):

```bash
export PATH="${PATH}:${HOME}/.local/bin:${HOME}/.local/bin/backplane/latest"
for tool in ocm rosa osdctl aws jq go; do command -v "${tool}" >/dev/null || echo "MISSING: ${tool}"; done
ocm token >/dev/null 2>&1 && echo "ocm: logged in" || echo "ocm: NOT logged in"
rosa whoami >/dev/null 2>&1 && echo "rosa: logged in" || echo "rosa: NOT logged in"
aws sts get-caller-identity >/dev/null 2>&1 && echo "aws: ok" || echo "aws: NO valid credentials"
[[ -f /tmp/rosa-e2e-cluster.env ]] && echo "ENV FILE EXISTS (cluster may already be tracked)"
```

- A missing tool, or a failed `ocm`/`rosa` login, needs interactive action the
  skill cannot perform: ask the user to run the login in their own terminal,
  e.g. `ocm login --use-auth-code --url stage` or `rosa login --env staging`,
  then re-check before continuing.
- If AWS credentials are missing, guide the user through the osdctl flow in
  docs/local-testing.md ("Provision Your Own Cluster", Step 1). Never ask the
  user to paste AWS keys into chat.
- If `/tmp/rosa-e2e-cluster.env` exists, a previous run's cluster may still be
  live. Source the file and check the cluster's actual state before doing
  anything else:

  ```bash
  set -a; source /tmp/rosa-e2e-cluster.env; set +a
  if rosa describe cluster -c "${CLUSTER_ID}" -o json 2>/dev/null | jq -e .state >/dev/null 2>&1; then
    rosa describe cluster -c "${CLUSTER_ID}" -o json 2>/dev/null | jq -r .state
  else
    echo "unknown"   # lookup or parse failed — NOT the same as gone
  fi
  ```

  - Any reported state — including `uninstalling`/`deleting` — means the
    cluster or its resources are still there: refuse to continue and tell the
    user to deprovision it first (Step 4). Provisioning anyway would orphan
    the cluster by overwriting the tracking file.
  - `unknown` is not proof the cluster is gone. Keep the file and ask the
    user to verify manually; take the stale path only when rosa definitively
    reports the cluster as not found.
  - Once gone is confirmed, also check for leftover AWS resources under the
    recorded names (VPC stack `${CLUSTER_NAME}-vpc`) before removing the file
    and reusing the cluster name.

Export `AWS_REGION` explicitly for the target region — the user's default AWS
profile may point elsewhere.

## Step 2: Provision

Map cluster-type to environment variables (see the script header for the full
list and docs/local-testing.md for per-feature notes):

| cluster-type | Environment | Extra notes |
|---|---|---|
| `hcp` | *(none)* | Defaults: us-east-2, 2× m5.xlarge |
| `zero-egress` | *(none — do not provision)* | Not available on main: the flag, shard pinning and specs land with #398. Stop and tell the user instead of running the provision script — it ignores `ZERO_EGRESS_ENABLED` today |
| `kms` | `KMS_ENABLED=true` | Creates a KMS key; deprovision schedules its deletion |
| `shared-vpc` | `SHARED_VPC_ENABLED=true` | Creates PHZ + shared role in the user's account |
| `external-auth` | `EXTERNAL_AUTH_ENABLED=true` | After ready, also run `scripts/create-external-auth-provider.sh` |
| `autoscaling` | `ENABLE_AUTOSCALING=true` | Default pool autoscales 2–4 |

Run in the background with output to a log file, and record the process id
for the failure path:

```bash
KMS_ENABLED=true ./scripts/provision-e2e-cluster.sh > /tmp/rosa-e2e-provision.log 2>&1 &
echo "PROVISION_PID=$!"
```

The script writes `/tmp/rosa-e2e-cluster.env` — the contract for every later
step. Do not `cat` the env file or the logs into chat (they contain tokens).

Provisioning takes ~15 minutes. Before each poll, confirm the provision
command is still running or exited successfully — a non-zero exit or a fatal
error in its log means the failure path below, not more polling. Poll until
the state is `ready` (or fail fast on `error`), roughly every 2–3 minutes:

```bash
source /tmp/rosa-e2e-cluster.env
rosa describe cluster -c "${CLUSTER_ID}" -o json | jq -r .state
```

Treat 60 minutes as the hard timeout. On timeout, an `error` state, or a
provision failure at any point, stop the provision process first — `kill
"${PROVISION_PID}"` if it is still running (the id was recorded at launch)
and confirm it has exited — then clean up and report:

- If `/tmp/rosa-e2e-cluster.env` exists, deprovision with it (Step 4).
- If it does not, resources may still have been created — the script writes
  the env file only after cluster creation, so an earlier failure leaves no
  record. Re-export the region chosen for the run
  (`export AWS_REGION=<region>`) first: it is not recoverable from the env
  file. Then recover with the deterministic names from the run:
  `CLUSTER_NAME` (echoed at the start of the provision log) and the VPC stack
  `${CLUSTER_NAME}-vpc`:
  1. If the cluster was created, delete it and wait for the uninstall to
     finish before touching the VPC — cluster-owned resources block stack
     deletion: `rosa delete cluster -c "${CLUSTER_NAME}" -y` then
     `rosa logs uninstall -c "${CLUSTER_NAME}" --watch`.
  2. Delete the stack (`aws cloudformation delete-stack --stack-name
     "${CLUSTER_NAME}-vpc" --region "${AWS_REGION}"`) and confirm it reaches
     `DELETE_COMPLETE`.
  3. Scan the provision log for identifiers of resources this run created —
     KMS key ARN, OIDC config id, shared-VPC hosted zone / roles / policies,
     operator role prefix — and clean those up too. Only delete resources
     the log shows this run created: anything adopted or reused (an existing
     OIDC config, an existing dnsdomain) must be kept.
- Report a **sanitized** failure summary: describe the failure in your own
  words based on the error lines (`grep -iE 'error|fatal|failed'
  /tmp/rosa-e2e-provision.log`) without quoting log text verbatim — an error
  line can itself contain a token. Never quote the raw log tail.

## Step 3: Run tests

Source the env file and the OCM token in every shell call:

```bash
set -a; source /tmp/rosa-e2e-cluster.env; set +a
unset KUBECONFIG   # the framework resolves cluster credentials via OCM
export OCM_TOKEN="$(ocm token)"
```

Choose the label filter (ginkgo label expressions are unions with `||`):

- Feature clusters (`kms`, `shared-vpc`, `autoscaling`):
  - `customer`: `LABEL_FILTER="Feature:<KMS|SharedVPC|Autoscaling>"`
  - `platform`: `LABEL_FILTER="Feature:<X> || Area:ManagementPlane"` — a plain
    intersection would drop the management-plane specs that carry no Feature
    label
- `zero-egress`: the `Feature:ZeroEgress` label and its specs are not on
  main yet — they land with #398. Do not run this cluster type until that
  PR merges; until then the filter would silently select nothing.
- `external-auth`: the customer-facing spec is labeled
  `Area:CustomerFeatures` (it has no `Feature:` label), so use
  `LABEL_FILTER="Area:CustomerFeatures"` for customer runs (plus
  `|| Area:ManagementPlane` for platform runs)
- Plain `hcp`:
  - `customer`: plain `make test` (specs needing MC/SC/AWS access skip on
    their own)
  - `platform`: `LABEL_FILTER="Area:ManagementPlane"`

For `test-type=platform`, first find the management cluster for the region
and the sector the provision run pinned (`CLUSTER_SECTOR` is empty unless
the script set one — default to main), and note its cluster id:

```bash
set -a; source /tmp/rosa-e2e-cluster.env; set +a
ocm get /api/osd_fleet_mgmt/v1/management_clusters \
  --parameter search="status='ready' AND region='${AWS_REGION}' AND sector='${CLUSTER_SECTOR:-main}'" \
  | jq -r '.items[] | .name + "\t" + .cluster_management_reference.cluster_id'
```

Then set up access and run the tests **in the same shell invocation** —
shell state does not persist between tool calls, and sourcing the cluster
env file does not restore `MC_KUBECONFIG` or `MANAGEMENT_CLUSTER_ID`.
Replace `<mc-cluster-id>` with the id chosen from the query output before
running the block:

```bash
set -a; source /tmp/rosa-e2e-cluster.env; set +a
unset KUBECONFIG   # the framework resolves cluster credentials via OCM
export OCM_TOKEN="$(ocm token)"

ocm backplane login "${CLUSTER_ID}" --manager
# Fresh, private kubeconfig — never a predictable shared /tmp path
MC_KUBECONFIG="$(mktemp /tmp/rosa-e2e-mc-kubeconfig.XXXXXX)"
chmod 600 "${MC_KUBECONFIG}"
cp ~/.kube/config "${MC_KUBECONFIG}"
export MC_KUBECONFIG
export MANAGEMENT_CLUSTER_ID=<mc-cluster-id>

# Substitute the feature label for the cluster type (e.g. Feature:KMS for a
# kms run) — the literal below is an example, not a runnable filter:
LABEL_FILTER="Feature:KMS || Area:ManagementPlane" make test > /tmp/rosa-e2e-test.log 2>&1
TEST_RC=$?
rm -f "${MC_KUBECONFIG}"
exit "${TEST_RC}"
```

Run the whole block as a single background command and monitor
`/tmp/rosa-e2e-test.log`; capturing `TEST_RC` keeps the test's exit status
visible after the kubeconfig removal — `wait` without a PID would report
success even for a failed run. Report the ginkgo summary
(passed/failed/skipped specs) — not the raw log — when done, and remove the
temporary kubeconfig if the run is interrupted.

## Step 4: Deprovision — always

Regardless of test outcome (pass, fail, or aborted partway):

```bash
set -a; source /tmp/rosa-e2e-cluster.env; set +a
./scripts/deprovision-e2e-cluster.sh > /tmp/rosa-e2e-deprovision.log 2>&1
```

Before starting, record the identifiers a retry needs somewhere outside the
env file — the script removes the env file at the end even when individual
cleanup steps fail:

- From the env file: CLUSTER_ID, CLUSTER_NAME, VPC_STACK_NAME, VPC_ID,
  OIDC_CONFIG_ID, KMS_KEY_ARN, and the SHARED_VPC_* identifiers when present.
- The operator role prefix is **not** in the env file — capture it from the
  live cluster before the OCM record is deleted:
  `rosa describe cluster -c "${CLUSTER_ID}" -o json |
  jq -r '.aws.sts.operator_iam_roles[].role_arn'`.

Run the deprovision in the background; it deletes the cluster, roles, OIDC,
feature resources (endpoints, KMS key, PHZ) and the VPC stack. Afterwards,
verify removal directly instead of trusting the log — several steps suppress
errors without printing a warning:

```bash
rosa describe cluster -c "${CLUSTER_ID}" -o json 2>&1 | head -1   # must report not found
aws cloudformation describe-stacks --stack-name "${VPC_STACK_NAME}" \
  --region "${AWS_REGION}" 2>&1 | head -1                          # must report it does not exist
```

Surface any non-removed resource to the user with a retry command built from
the recorded identifiers instead of reporting "cleanup complete". If the user
interrupts the session before cleanup completes, state plainly that a cluster
is still live and give them the actual cleanup commands to run themselves:

```bash
set -a; source /tmp/rosa-e2e-cluster.env; set +a
./scripts/deprovision-e2e-cluster.sh
```

If the env file is already gone, rebuild the equivalent from the recorded
identifiers: `rosa delete cluster -c <CLUSTER_ID> -y`, then
`rosa delete operator-roles --prefix <OPERATOR_ROLE_PREFIX> --mode auto
--yes`, then `aws cloudformation delete-stack --stack-name <VPC_STACK_NAME>
--region <REGION>`.

## Final report

Summarize for the user: cluster type and region, test label filter ran,
pass/fail/skip counts with the names of any failing specs, paths of the
provision/test/deprovision logs, and confirmation that the cluster and AWS
resources were removed.
