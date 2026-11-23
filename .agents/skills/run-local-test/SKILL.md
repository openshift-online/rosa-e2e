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
  rosa describe cluster -c "${CLUSTER_ID}" -o json 2>/dev/null | jq -r .state || echo "gone"
  ```

  If the cluster exists in any state other than `uninstalling`/`deleting`, it
  is live: refuse to continue and tell the user to deprovision it first
  (Step 4) — provisioning anyway would orphan it by overwriting the tracking
  file. Only treat the file as stale once the cluster is confirmed gone, and
  then also check for leftover AWS resources under the recorded names (VPC
  stack `${CLUSTER_NAME}-vpc`) before removing the file and reusing the name.

Export `AWS_REGION` explicitly for the target region — the user's default AWS
profile may point elsewhere.

## Step 2: Provision

Map cluster-type to environment variables (see the script header for the full
list and docs/local-testing.md for per-feature notes):

| cluster-type | Environment | Extra notes |
|---|---|---|
| `hcp` | *(none)* | Defaults: us-east-2, 2× m5.xlarge |
| `zero-egress` | `ZERO_EGRESS_ENABLED=true REGION=us-west-2` | us-west-2 required (ECR-enabled shard); needs the zero-egress provision support |
| `kms` | `KMS_ENABLED=true` | Creates a KMS key; deprovision schedules its deletion |
| `shared-vpc` | `SHARED_VPC_ENABLED=true` | Creates PHZ + shared role in the user's account |
| `external-auth` | `EXTERNAL_AUTH_ENABLED=true` | After ready, also run `scripts/create-external-auth-provider.sh` |
| `autoscaling` | `ENABLE_AUTOSCALING=true` | Default pool autoscales 2–4 |

Run in the background with output to a log file, e.g.
`ZERO_EGRESS_ENABLED=true REGION=us-west-2 ./scripts/provision-e2e-cluster.sh > /tmp/rosa-e2e-provision.log 2>&1`.
The script writes `/tmp/rosa-e2e-cluster.env` — the contract for every later
step. Do not `cat` the env file or the logs into chat (they contain tokens).

Provisioning takes ~15 minutes. Poll until the state is `ready` (or fail fast
on `error`), roughly every 2–3 minutes:

```bash
source /tmp/rosa-e2e-cluster.env
rosa describe cluster -c "${CLUSTER_ID}" -o json | jq -r .state
```

Treat 60 minutes as the hard timeout. On timeout, an `error` state, or a
provision failure at any point, clean up and report:

- If `/tmp/rosa-e2e-cluster.env` exists, deprovision with it (Step 4).
- If it does not, resources may still have been created — the script writes
  the env file only after cluster creation, so an earlier failure leaves no
  record. Recover with the deterministic names from the run:
  `CLUSTER_NAME` (echoed at the start of the provision log) and the VPC stack
  `${CLUSTER_NAME}-vpc`. Delete the cluster if it was created
  (`rosa delete cluster -c "${CLUSTER_NAME}" -y`), then the stack
  (`aws cloudformation delete-stack --stack-name "${CLUSTER_NAME}-vpc" \
  --region "${AWS_REGION}"`), and scan the log for any KMS key ARN or OIDC
  config id that needs manual cleanup.
- Report a **sanitized** failure summary: extract only the error lines
  (`grep -iE 'error|fatal|failed' /tmp/rosa-e2e-provision.log`). Never quote
  the raw log tail — the log can contain tokens.

## Step 3: Run tests

Source the env file and the OCM token in every shell call:

```bash
set -a; source /tmp/rosa-e2e-cluster.env; set +a
unset KUBECONFIG   # the framework resolves cluster credentials via OCM
export OCM_TOKEN="$(ocm token)"
```

Choose the label filter (ginkgo label expressions are unions with `||`):

- Feature clusters (`zero-egress`, `kms`, `shared-vpc`, `autoscaling`):
  - `customer`: `LABEL_FILTER="Feature:<ZeroEgress|KMS|SharedVPC|Autoscaling>"`
  - `platform`: `LABEL_FILTER="Feature:<X> || Area:ManagementPlane"` — a plain
    intersection would drop the management-plane specs that carry no Feature
    label
- `external-auth`: the customer-facing spec is labeled
  `Area:CustomerFeatures` (it has no `Feature:` label), so use
  `LABEL_FILTER="Area:CustomerFeatures"` for customer runs (plus
  `|| Area:ManagementPlane` for platform runs)
- Plain `hcp`:
  - `customer`: plain `make test` (specs needing MC/SC/AWS access skip on
    their own)
  - `platform`: `LABEL_FILTER="Area:ManagementPlane"`

For `test-type=platform`, set up management cluster access and run the tests
**in the same shell invocation** — shell state does not persist between tool
calls, and sourcing the cluster env file does not restore `MC_KUBECONFIG` or
`MANAGEMENT_CLUSTER_ID`:

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
# MC cluster id for the cluster's region and the sector the provision run
# pinned (CLUSTER_SECTOR is empty unless the script set one, e.g. rosa-e2e
# for zero-egress — default to main):
ocm get /api/osd_fleet_mgmt/v1/management_clusters \
  --parameter search="status='ready' AND region='${AWS_REGION}' AND sector='${CLUSTER_SECTOR:-main}'" \
  | jq -r '.items[] | .name + "\t" + .cluster_management_reference.cluster_id'
export MANAGEMENT_CLUSTER_ID=<mc-cluster-id>

LABEL_FILTER="Feature:<X> || Area:ManagementPlane" make test &
wait; rm -f "${MC_KUBECONFIG}"
```

Run `make test` in the background with output to a log file (e.g.
`/tmp/rosa-e2e-test.log`), then monitor. Report the ginkgo summary
(passed/failed/skipped specs) — not the raw log — when done, and remove the
temporary kubeconfig if the run is interrupted.

## Step 4: Deprovision — always

Regardless of test outcome (pass, fail, or aborted partway):

```bash
set -a; source /tmp/rosa-e2e-cluster.env; set +a
./scripts/deprovision-e2e-cluster.sh > /tmp/rosa-e2e-deprovision.log 2>&1
```

Before starting, note the key identifiers from the env file
(CLUSTER_ID, CLUSTER_NAME, VPC_STACK_NAME, VPC_ID, OIDC_CONFIG_ID,
OPERATOR_ROLE_PREFIX) somewhere outside it: the script removes the env file
at the end even when individual cleanup steps failed, and those identifiers
are what a manual retry needs. Run the deprovision in the background; it
deletes the cluster, roles, OIDC, feature resources (endpoints, KMS key,
PHZ) and the VPC stack. Afterwards, check the log for `WARNING` lines — the
script continues past individual failures — and surface any to the user with
a retry command built from the recorded identifiers instead of reporting
"cleanup complete". If the user interrupts the session before this step
completes, state plainly that a cluster is still live and give them the two
commands above to clean it up.

## Final report

Summarize for the user: cluster type and region, test label filter ran,
pass/fail/skip counts with the names of any failing specs, paths of the
provision/test/deprovision logs, and confirmation that the cluster and AWS
resources were removed.
