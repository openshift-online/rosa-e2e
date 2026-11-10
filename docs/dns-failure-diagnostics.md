# DNS failure diagnostics

The "should resolve cluster DNS" data plane spec registers a best-effort cleanup right
after `CreateTestNamespace` returns and before its error is asserted, so namespace and
default ServiceAccount setup failures are covered as well as later pod or DNS failures.
On failure, before the namespace is deleted, it writes
`dns-failure-diagnostics/snapshot-*.json` under `ARTIFACT_DIR` (or `/tmp`). Successful
and skipped specs do not collect snapshots. A snapshot is attribution evidence only.

`CreateTestNamespace` waits up to one minute for the namespace's default ServiceAccount,
retrying only NotFound; any other API error is returned immediately. When that wait
fails the namespace cleanup is still returned, so the spec deletes the namespace after
the snapshot is written.

Collection uses the hosted cluster client only: one namespace GET, one default
ServiceAccount GET and one event list in the test namespace. Requests have a 3-second
timeout and share a 12-second total deadline. No pagination is followed and at most 100
events are kept. `readStatus` records complete, truncated, not-found, timeout,
access-denied or read-failed for `namespace`, `defaultServiceAccount` and `events`.
Missing prerequisites and write failures produce only a fixed warning, never a new test
failure.

The snapshot contains the capture time, namespace and ServiceAccount identity (name,
namespace, UID, generation, creation time), the namespace phase (Active, Terminating or
Unknown) and, per event, its name, involved object name, count and last observed time
(the later of the legacy fields and the event series, which newer recorders use). Event
type is kept only when Normal or Warning; event reason and involved object kind are kept
only when they are identifiers of at most 64 letters and digits starting with a letter.
Event messages, labels, annotations, ServiceAccount secrets and raw API errors are never
saved.

These snapshots are separate from `mc-failure-diagnostics` and its warnings.

Local validation requires no cluster: `go test ./pkg/framework ./pkg/reporting -run 'DNS|ServiceAccount|CreateTestNamespace'`
and `make build`.
