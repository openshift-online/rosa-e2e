# HCP revision failure diagnostics

The HCP namespace deployment-revision stability spec registers a best-effort
cleanup after resolving management-cluster clients and namespaces. On failure,
before suite teardown, it writes `mc-failure-diagnostics/snapshot-*.json` under
`ARTIFACT_DIR` (or `/tmp`). Successful and skipped specs do not collect snapshots.
The existing assertions, catalog coverage and 30-second observation are unchanged.
A snapshot provides attribution evidence; it does not establish a revision change's cause.

Collection uses only the existing clients: three namespace-scoped lists in the HCP
namespace and one named HostedCluster GET in its resolved namespace. Requests have
a 3-second timeout and share a 12-second total API deadline. No pagination is followed;
each list includes at most 100 objects, conditions and container restart counters
are bounded, and the JSON artifact is at most 512 KiB including its newline.
`readStatus` records complete, truncated, timeout, access-denied or read-failed
results. Byte truncation removes tail objects and marks the affected list truncated.
Missing prerequisites and write failures produce only a fixed warning, never a new
test failure. Partial capture remains useful and does not imply a healthy resource.

The explicit output schema includes resource identity, generation and creation time,
numeric deployment revision, desired/current/ready/available/updated/unavailable
replica counts, observed generation, allowlisted condition type/status/transition
time, pod phase and restart totals. Controller owner name/UID links ReplicaSets to
Deployments and pods to ReplicaSets. HostedCluster output contains only identity
and allowlisted conditions. No raw specs, labels, arbitrary annotations, condition
messages/reasons, container configuration, logs, events or raw API errors are saved.
The revision annotation is accepted only as a nonnegative decimal int64.

Local validation requires no cluster: `go test ./pkg/reporting ./pkg/verifiers`,
`go test -tags E2Etests ./test/e2e -run TestMCFailureDiagnostics`, and `make build`.
