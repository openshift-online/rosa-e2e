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
- If `/tmp/rosa-e2e-cluster.env` exists, a previous run may still have a live
  cluster. Show the CLUSTER_ID/CLUSTER_NAME from the file and ask the user
  whether to deprovision it first or treat it as stale before continuing.

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

Treat 60 minutes as the hard timeout; on timeout or error state, deprovision
and report the tail of the provision log.

## Step 3: Run tests

Source the env file and the OCM token in every shell call:

```bash
set -a; source /tmp/rosa-e2e-cluster.env; set +a
unset KUBECONFIG   # the framework resolves cluster credentials via OCM
export OCM_TOKEN="$(ocm token)"
```

Choose the label filter:

- Feature clusters (`zero-egress`, `kms`, `shared-vpc`, `external-auth`,
  `autoscaling`): `LABEL_FILTER="Feature:<ZeroEgress|KMS|SharedVPC|...>"` —
  the env file already self-selects feature specs where applicable.
- Plain `hcp`:
  - `customer`: plain `make test` (specs needing MC/SC/AWS access skip on
    their own)
  - `platform`: `LABEL_FILTER="Area:ManagementPlane"`

For `test-type=platform`, set up management cluster access first:

```bash
ocm backplane login "${CLUSTER_ID}" --manager
cp ~/.kube/config /tmp/mc-kubeconfig.yaml
export MC_KUBECONFIG=/tmp/mc-kubeconfig.yaml
# MC cluster id for the cluster's region/sector (sector "main" unless the
# provision run pinned another, e.g. rosa-e2e for zero-egress):
ocm get /api/osd_fleet_mgmt/v1/management_clusters \
  --parameter search="status='ready' AND region='${REGION}' AND sector='main'" \
  | jq -r '.items[] | .name + "\t" + .cluster_management_reference.cluster_id'
export MANAGEMENT_CLUSTER_ID=<mc-cluster-id>
```

Run `make test` in the background with output to a log file (e.g.
`/tmp/rosa-e2e-test.log`), then monitor. Report the ginkgo summary
(passed/failed/skipped specs) — not the raw log — when done.

## Step 4: Deprovision — always

Regardless of test outcome (pass, fail, or aborted partway):

```bash
set -a; source /tmp/rosa-e2e-cluster.env; set +a
./scripts/deprovision-e2e-cluster.sh > /tmp/rosa-e2e-deprovision.log 2>&1
```

Run it in the background; it deletes the cluster, roles, OIDC, feature
resources (endpoints, KMS key, PHZ) and the VPC stack, then removes the env
file. If the user interrupts the session before this step completes, state
plainly that a cluster is still live and give them the two commands above to
clean it up.

## Final report

Summarize for the user: cluster type and region, test label filter ran,
pass/fail/skip counts with the names of any failing specs, paths of the
provision/test/deprovision logs, and confirmation that the cluster and AWS
resources were removed.
