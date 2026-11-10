# CI accountability metrics

How to measure whether the ROSAENG-309 ownership model is working: are `[ci-failure]` Jiras getting acked, triaged, and fixed within their ladder clocks? All state lives in Jira, so all metrics are JQL over Jira (project ROSAENG). This doc defines the queries; save them as filters in Jira (names below) so dashboards and the weekly review can reuse them.

Reference: the ladder task (`.chai-bot/rosa_ci_permafail_ladder.md`) enforces the clocks daily; the vocabulary (`ci-failure`, `ci-grace`, `ci-triaged-*`, `ci-consequence-eligible`) is defined there.

## Base population

Every `[ci-failure]` Jira filed by the daily remediation flow:

```
project = ROSAENG AND labels = "ci-failure"
```

Active (unresolved, not paused by grace):

```
project = ROSAENG AND labels = "ci-failure" AND resolution = Unresolved AND NOT (labels = "ci-grace" AND duedate >= startOfDay())
```

## Rung health (SLA misses right now)

**Ack overdue** (T+2 business days, still `New`). JQL cannot count business days, so this uses calendar days as an approximation; the ladder task computes business days exactly:

```
project = ROSAENG AND labels = "ci-failure" AND resolution = Unresolved AND status = New AND created <= -3d
```

**Triage overdue** (acked but no `ci-triaged-*` label after T+3 gating / T+5 informing; approximate with -5d / -7d calendar):

```
project = ROSAENG AND labels = "ci-failure" AND resolution = Unresolved AND status != New AND NOT labels IN ("ci-triaged-test-bug", "ci-triaged-product-bug", "ci-triaged-infra", "ci-triaged-upstream") AND created <= -5d
```

**Consequence-eligible** (T+14, still unresolved):

```
project = ROSAENG AND labels = "ci-failure" AND resolution = Unresolved AND created <= -14d
```

## Flow metrics (how fast things move)

**Time-to-ack / time-to-triage**: JQL cannot compute the gap between two transitions. Two views approximate it today:

- Snapshot: the rung-health queries above, watched week over week (count trending down = acks getting faster).
- Per-issue: any issue list view showing `created` next to `updated` (for issues still in `New`, `updated` is the last activity; for acked issues, use the issue history). The ladder task's `[ci-ladder]` comments timestamp each rung firing, so an issue's comment trail gives the ack date without a history export.

Precise transition latency (median time in `New`, median ack-to-triage) needs the Jira issue history export (`updated`-based JQL cannot express it). Once the ladder has run for a few weeks, its daily digests are the cheaper data source: each rung firing is one observation.

**Time-to-green** (fixed and closed, last 7 days):

```
project = ROSAENG AND labels = "ci-failure" AND resolution = Resolved AND resolutiondate >= -7d
```

A list view with both `created` and `resolutiondate` columns gives per-issue time-to-green; resolution type distinguishes fixed vs won't-do vs duplicate.

**Weekly filing volume** (is remediation filing more than teams close?):

```
project = ROSAENG AND labels = "ci-failure" AND created >= -7d
```

## Permafail aging

Oldest open failures first (the weekly review's working queue):

```
project = ROSAENG AND labels = "ci-failure" AND resolution = Unresolved ORDER BY created ASC
```

## Coverage debt

Failures accepted as permanent signal debt (label lands with the T+14 consequence mechanics, ROSAENG-75798):

```
project = ROSAENG AND labels = "ci-failure" AND labels = "coverage-debt"
```

## Per-team splits

The Team field (`customfield_10001`) is not reliably queryable in JQL on Jira Cloud teams. Split any of the queries above by team in a dashboard filter result gadget (group by Team) or in the weekly review, rather than by writing per-team JQL.

## Saved filters and dashboard

Create these as saved filters in ROSAENG (share with the project), then add them to a dashboard named **ROSAENG CI Accountability** (filter result gadgets, one per filter, sorted by created):

| Filter name | Query |
|---|---|
| CI: ack overdue | rung health, ack overdue |
| CI: triage overdue | rung health, triage overdue |
| CI: consequence-eligible | rung health, T+14 |
| CI: time-to-green (7d) | flow, resolved last 7d |
| CI: filed (7d) | flow, filing volume |
| CI: permafail aging | aging, ORDER BY created ASC |

The weekly review (ROSAENG-75800) reads the permafail-aging and consequence-eligible filters; the CI Watcher reads ack/triage-overdue during shifts.
