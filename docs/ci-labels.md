# ROSA CI Jira Label Reference

Labels used on ROSAENG issues to track CI health and route failures to owning teams. The permafail ladder (`.chai-bot/rosa_ci_permafail_ladder.md`) covers enforcement mechanics; this document covers the full label taxonomy.

## Core labels

Every CI failure issue carries both of these:

| Label | Meaning |
|---|---|
| `ci-failure` | Active CI failure. Set by chai-bot at filing time or manually. |
| `ci-watcher` | Subset actively tracked by the permafail ladder. Every `ci-watcher` issue also has `ci-failure`. |

## Triage classification (one required after ack)

After a team acknowledges an issue, exactly one classification label is required:

| Label | Root cause |
|---|---|
| `ci-triaged-test-bug` | Flaky or broken test code; fix lives in the suite |
| `ci-triaged-product-bug` | Upstream product regression |
| `ci-triaged-infra` | Build farm or cloud infrastructure root cause |
| `ci-triaged-upstream` | OCP or HyperShift upstream issue |

## Ladder lifecycle labels

Set automatically by the permafail ladder:

| Label | When set |
|---|---|
| `ci-grace` | Team is waiting on an external dependency; must have a due date on the issue. Clocks pause while set and due date is in the future. |
| `ci-consequence-eligible` | Issue is unresolved at T+14 calendar days |

## Classifier labels

Set by chai-bot at filing time (or manually during triage) to describe the failure area. Used for per-team dashboard slices and JQL filtering. Multiple classifiers can appear on one issue.

| Label | Scope |
|---|---|
| `ocm-fvt` | OCM FVT suite failures |
| `rosa-e2e` | rosa-e2e suite |
| `gap-analysis` | rosa-gap-analysis jobs |
| `hypershift-ci` | HyperShift CI sector |
| `rosa-hcp` | HCP-specific failures |
| `rosa-cli` | CLI team e2e |
| `rosa-ci` | General ROSA CI infra |
| `rosa-ci-fix` | Needs a CI fix rather than a product fix |
| `credential-rotation` | Credential or token expiry causing the failure |
| `infrastructure` | Build farm or cloud provider root cause |
| `shared-vpc` | Shared VPC topology failures |
| `gcp` | GCP-specific |
| `diagnostic-gap` | Failure is undiagnosable due to missing artifacts or logs |
| `ai-generated-jira` | Filed automatically by chai-bot |

## Autosolve pipeline labels

| Label | Meaning |
|---|---|
| `issue-for-agent-candidate` | Grooming identified this as a good autosolve candidate (small, single-repo, clear done criteria) |
| `issue-for-agent` | Promoted by the pod lead; triggers a chai-bot solve run. Do not apply this label manually. |

## Key fields

| Field | JQL name | Purpose |
|---|---|---|
| Team | `cf[10001]` | Owning team, used for routing. Set to the team's Atlassian team resource ID. |
| Status Summary | `cf[10814]` | Current state narrative, updated during triage and grooming. |
| Priority | `priority` | Blocker/Critical/Major are actionable now. Auto-filed issues land as Undefined and need triage. |

## Filtering by area

Classifier labels are the most reliable way to filter by team area because many auto-filed issues have Team = None until triaged. Use label combinations rather than the Team field alone:

```
# All active OCM FVT failures
labels = ci-failure AND labels = ocm-fvt AND statusCategory != Done

# CLI team failures
labels = ci-failure AND labels = rosa-cli AND statusCategory != Done

# Infra root cause failures (across all areas)
labels = ci-failure AND labels = ci-triaged-infra AND statusCategory != Done

# Unacknowledged failures (not yet triaged by anyone)
labels = ci-failure AND status = New AND statusCategory != Done

# Autosolve pipeline
labels in (issue-for-agent, issue-for-agent-candidate)
```

The [ROSA CI Health dashboard](https://redhat.atlassian.net/secure/Dashboard.jspa) uses `labels in (ci-watcher, ci-failure)` as its core failure signal, combined with the CI initiative children (ROSA-727, ROSA-685, ROSA-714) for the full cross-team view.
