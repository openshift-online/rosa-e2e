# Operator E2E Lease Cluster Enhancement Design

## Summary

Operator E2E jobs use a shared inventory of short-lived ROSA and OSD clusters. A
job checks out a cluster, pauses and backs up the stock operator, installs the
candidate operator, runs its E2E tests, restores the stock operator, and checks
the cluster back in.

Promotion tests are initiated by an app-interface PipelineRun. The bridge
triggers the corresponding Prow periodic job, which runs ci-operator and the
lease workflow.

```text
app-interface PipelineRun
  |
  v
Prow operator E2E job
  |
  v
checkout
  |
  v
install candidate
  |
  v
run tests
  |
  v
restore stock operator
  |
  v
check-in
```

The design introduces four key changes:

| Area | Change |
|---|---|
| Remove in-place cluster upgrades | Replace an idle cluster when its version changes instead of upgrading it in place |
| Clarify controller and health job responsibilities | Controller detects availability problems, owns status, and requests repair; health job performs the requested repair and reports the result |
| Introduce `degraded` status | Block only operators with failed packages while allowing unaffected operators to continue using the cluster; reserve `error` for fatal or persistent widespread failures |
| Add a reusable stock operator backup | Best-effort creation of an immutable, sanitized ConfigMap gives later health-job runs a reusable restore source while the existing job-local backup remains the primary restore path |

## 1. Remove In-Place Cluster Upgrades

Lease clusters are short-lived and already have an age-based replacement path.
The controller does not need a separate in-place upgrade workflow.

Remove upgrade discovery, upgrade scheduling, and the `upgrade-target`
annotation. When a cluster's OpenShift minor version differs from the desired
version, wait until the cluster has no active claims and replace it:

```text
available
  |
  v
maintenance
  |
  v
delete and recreate
  |
  v
provisioning
  |
  v
available
```

`maintenance` is used only for deletion and recreation. Replacement remains
capacity-aware and bounded so the lease pool retains enough usable clusters.

This does not remove managed-upgrade-operator E2E coverage. Its current test
creates a future `UpgradeConfig`, verifies pending behavior, and deletes it; it
does not perform an OpenShift cluster upgrade.

## 2. Responsibility and Ownership

The lease controller and health job currently overlap: both inspect cluster
health, both can influence recovery, and both can change cluster status. The
proposed design gives them separate responsibilities:

```text
controller detects and requests repair
  |
  v
health job performs repair and records result
  |
  v
controller revalidates and updates status
```

### Component ownership

| Component | Owns | Must not do |
|---|---|---|
| Checkout/check-in | Operator claims and atomic `available`/`in-use` allocation transitions | Diagnose shared health or replace clusters |
| Operator install/cleanup | Create the primary job-local backup, attempt the secondary ConfigMap backup, perform the immediate restore, and remove the ConfigMap after successful normal cleanup | Perform long-term cluster recovery |
| Lease controller | Availability checks, failure and recovery status, repair requests, recovery backup cleanup, stale claims, provisioning, replacement, and decommissioning | Repair ClusterPackages, CRDs, RBAC, or operator workloads |
| Health job | Requested repairs using the secondary backup when available, repair verification, and repair results | Change lease status, claims, inventory, or cluster lifecycle |

Each component patches only the fields it owns. Claim changes continue to use
compare-and-swap because multiple operators may share one cluster.

### Status ownership

The `rosa-cluster-lease/status` label remains the checkout source of truth.

| Status | Meaning | Owner of transition |
|---|---|---|
| `provisioning` | The cluster is being created or registered | Controller |
| `available` | The cluster has no active operator claims and may accept work | Checkout/check-in for normal allocation; controller after recovery |
| `in-use` | The cluster has one or more active operator claims | Checkout/check-in for normal allocation; controller after recovery |
| `degraded` | The cluster remains usable, but one or more operators are blocked while repair is attempted | Controller |
| `error` | The cluster has a fatal or persistent widespread failure and must be replaced | Controller |
| `maintenance` | The cluster is locked for deletion and recreation | Controller |

The health job never changes this label. While status is `error`, check-in may
remove completed claims but must not return the cluster to `available`.

Checkout treats `degraded` as eligible for operators that do not have a recorded
failure. Active claims remain in the `operators` annotation while status is
`degraded`. Checkout and check-in add or remove claims without changing the
status until the controller resolves or escalates the degradation.

### Controller: detect and request repair

The controller determines whether a cluster can participate in the lease pool.
It performs read-only availability checks such as:

- The cluster exists in OCM and is `ready`.
- The cluster API is reachable.
- Required nodes and RBAC are available.
- PKO is functional.
- Expected stock ClusterPackages exist and report available.

When an operator is actively being tested, the controller excludes that
operator's stock package from the cluster-wide package check. It also excludes a
package already recorded in the operator failure flow. Shared API, node, RBAC,
and PKO checks still run.

The controller classifies a failure before changing status:

- One or two failed operator packages, with shared infrastructure still healthy,
  change the cluster to `degraded`. Only those operators are blocked.
- Another repairable partial failure may also use `degraded` when unaffected
  operators can safely continue.
- A fatal shared failure changes the cluster to `error` and requests
  replacement.
- Three or more operator package failures change the cluster to `error` only
  after they persist beyond the configured duration and their repair attempts
  are exhausted.

For a degraded cluster, the controller leaves existing claims in place and
creates a repair request with a unique ID, supported action, exact target,
reason, and timestamp. It performs no repair action.

Example operator package repair request:

```yaml
metadata:
  annotations:
    rosa-cluster-lease/operator-repair-requests: >-
      {"managed-upgrade-operator":{"id":"muo-42","action":"restore-clusterpackage","target":"managed-upgrade-operator","backupNamespace":"openshift-config","backupConfigMap":"rosa-e2e-stock-managed-upgrade-operator","reason":"stock ClusterPackage is unavailable","requestedAt":"2026-09-24T01:10:00Z"}}
```

Each operator repair has its own request ID. Cluster-scoped repair requests are
serialized so only one is active for a cluster at a time.

### Health job: repair and report

The health job scans lease ConfigMaps for repair requests without a matching
final result. The ConfigMap identifies the cluster through `cluster-id` and
`ocm-env`; the request identifies the repair action and target, plus the backup
ConfigMap when secondary backup creation succeeded. The health job does not
infer or guess what should be repaired.

For each request, the health job:

1. Confirms the request ID is still current.
2. Accesses the identified cluster.
3. Executes only the requested, supported repair.
4. Verifies the affected resource.
5. Writes `fixed` or `failed` for the same request ID.

Supported repairs may include restoring a stock ClusterPackage from its durable
backup, correcting PKO CRD ownership, retrying a stuck ClusterPackage
reconciliation, or removing safe E2E leftovers that block the stock package.

Example result:

```yaml
metadata:
  annotations:
    rosa-cluster-lease/operator-repair-results: >-
      {"managed-upgrade-operator":{"id":"muo-42","result":"fixed","reason":"stock ClusterPackage is available","completedAt":"2026-09-24T01:14:00Z"}}
```

If the action is unsupported or the retry budget is exhausted, the health job
writes a matching result with `result=failed`. It does not change `degraded` to
`error` and does not replace the cluster. Escalation remains a controller
decision.

### Controller: consume the repair result

The controller accepts an operator result only when its request ID matches the
current request ID for that operator. Cluster-scoped results follow the same ID
matching rule. This prevents a late result from an older attempt from affecting
the current state.

For a matching result with `result=fixed`, the controller repeats the
availability checks that failed originally and removes the repaired operator
from the failure set:

- If no failures remain, it returns the cluster to `in-use` when claims remain
  or `available` when no claims remain.
- If other operator failures remain, status stays `degraded` and unaffected
  operators remain eligible.
- If validation fails, it creates a new repair request and leaves status as
  `degraded`.

For a matching result with `result=failed`, the controller keeps the affected
operator blocked and leaves status as `degraded` while the failure remains
isolated. One or two operator failures do not cause replacement, even after
their individual repair attempts fail.

The controller changes `degraded` to `error` and requests replacement only when:

- A fatal shared failure makes the cluster unsafe for every operator; or
- At least three operator packages remain failed beyond the configured
  degradation timeout and their repair attempts are exhausted.

Replacement starts only after all active claims have drained or expired.

```text
operator package failure
  |
  v
controller sets degraded and requests repair
  |
  v
health job repairs and records result
  |
  v
controller revalidates
  |
  +-- no failures
  |     `-- status=in-use or available
  |
  +-- other failures remain
  |     `-- status=degraded
  |
  `-- fatal or persistent widespread failure
        `-- status=error; replace when idle
```

### Scheduling and concurrency

The jobs keep their current cadence. The controller runs more frequently to
monitor status and consume repair results, while the health job runs every 30
minutes to perform repairs:

| Reconciler | Proposed schedule |
|---|---|
| Lease controller | Every 15 minutes |
| Health job | Every 30 minutes, at minutes 7 and 37 |

Each job uses its own distributed lock. The lock prevents two controller runs or
two health runs from overlapping. The controller and health job may run at the
same time because the controller owns requests and status while the health job
owns results.

## 3. Operator Failure Handling

A candidate test or installation failure is not a lease cluster failure. If
cleanup restores the stock operator successfully, the lease receives no repair
signal.

If stock restoration fails, check-in releases the completed claim and writes an
operator failure observation:

```yaml
metadata:
  annotations:
    rosa-cluster-lease/operator-failures: >-
      {"managed-upgrade-operator":{"clusterPackage":"managed-upgrade-operator","namespace":"openshift-managed-upgrade-operator","backupAvailable":true,"backupNamespace":"openshift-config","backupConfigMap":"rosa-e2e-stock-managed-upgrade-operator","reason":"stock package restore failed","observedAt":"2026-09-24T02:00:00Z"}}
```

`backupNamespace` and `backupConfigMap` are included only when secondary backup
creation succeeded. Otherwise the observation records `backupAvailable=false`,
and the health job may attempt only repairs that can be derived from live
cluster state.

Checkout uses this annotation to block only the named operator on that cluster.
Other operators remain eligible after the controller changes cluster status to
`degraded`.

The operator repair uses the same ownership pattern as cluster repair:

1. Check-in records the failure observation and releases the claim.
2. The controller adds the operator to the failed-operator set, changes status
   to `degraded`, and creates an `operator-repair-requests` entry with a unique
   request ID.
3. The health job attempts the requested repair and writes a matching entry in
   `operator-repair-results`.
4. The controller validates the stock operator.
5. On success, the controller clears the operator block. On failure, the block
   remains and an alert is raised.

```text
operator A restore fails
  |
  +-- block operator A
  |
  +-- keep operators B, C, ... eligible
  |
  v
controller sets degraded and requests repair
  |
  v
health job attempts repair
  |
  v
controller validates
  |
  +-- fixed
  |     `-- unblock operator A
  |
  `-- failed
        `-- keep operator A blocked

One or two failed operators remain degraded.
No replacement is requested.
```

One or two operator-specific failures must never:

- Change cluster status from `degraded` to `error`.
- Create a cluster replacement request.
- Prevent unrelated operators from using the cluster.
- Be escalated only because an individual repair attempt failed.

Three or more failed operator packages may be escalated to `error` only when the
condition persists beyond the configured timeout and repair attempts are
exhausted. The controller may also move directly to `error` when its shared
availability checks detect a fatal API, node, RBAC, or PKO failure.

### Durable stock operator backup

The existing `SHARED_DIR` backup remains the primary restore source. The install
step also attempts to create an immutable secondary ConfigMap in
`openshift-config` on the leased cluster, named
`rosa-e2e-stock-<operator-name>`.

The ConfigMap contains the production ClusterPackage with `status` and other
runtime metadata removed. Creation is best effort: a collision, size problem,
or API failure produces a warning but does not block operator testing.

The lifecycle is:

1. Install creates the `SHARED_DIR` backup and attempts the ConfigMap backup.
2. Normal cleanup restores from `SHARED_DIR` and removes the ConfigMap after
   successful validation.
3. If normal cleanup fails, the ConfigMap remains available to multiple health
   job runs using the same known-good payload.
4. After a health repair succeeds, the controller revalidates the operator and
   removes the ConfigMap.

The failure or repair record includes the ConfigMap name only when creation
succeeded. If the sanitized payload is too large for a ConfigMap, the secondary
backup is skipped and the primary job-local restore path remains unchanged.
