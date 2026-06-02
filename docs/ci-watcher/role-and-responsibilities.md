# CI Watcher Role and Responsibilities

## Overview

The CI Watcher is a **weekly rotating role, separate from on-call**, where one person per week is responsible for monitoring ROSA CI job health, triaging failures, and routing fixes to the owning team.

The role exists because CI failures need sustained attention (a week, not a sprint), paging creates expensive routing, and on-call has production incidents competing for attention.

## Key Principles

- **Triage within 24 hours** of a new failure appearing
- **Classify every failure** by category (infra flake, test bug, OCP payload, config drift, STS policy, cluster provisioning)
- **File or update Jira** for persistent failures (2+ consecutive fails)
- **Route to the owning team** with a clear description and reproduction steps
- **Block nothing.** The watcher observes, classifies, and routes
- **Evaluate before routing.** Do a minimal investigation before routing to a team. At minimum: check ROSA stage health, cloud account quotas, and OCM test account status. Do not bounce tickets without context

## What the Watcher Is NOT

- **Not on-call.** No paging, no SLA, no after-hours obligation
- **Not the fixer.** The owning team fixes. The watcher tracks
- **Not a bottleneck.** If the watcher is out, the rotation skips to the next person

## Daily Responsibilities

| Step | Task | Tool |
|------|------|------|
| 1 | Run `/ci-triage` to get automated status, failure diagnosis, and proposed actions | Claude Code |
| 2 | Check CI MC/SC health (error cluster ratio, stuck deletions) | OCM CLI / backplane |
| 3 | Review `/ci-triage` findings against Sippy rosa-stage dashboard | [Sippy](https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage) |
| 4 | Check ROSA component readiness views for regressions. File and track immediately | [Sippy component readiness](https://sippy-auth.dptools.openshift.org) |
| 5 | For any failures the AI could not classify, investigate root cause manually | Prow GCS |
| 6 | Review and approve any Jira stories or fix PRs proposed by `/ci-triage` | Jira / GitHub |
| 7 | File or update Jira for anything `/ci-triage` missed | Jira ([ROSAENG-391](https://redhat.atlassian.net/browse/ROSAENG-391)) |
| 8 | Review the automated [daily status report](../../scripts/ci-status-report.sh) posted to `#wg-rosa-ci-enhancement` | Slack |

The `/ci-triage` skill is the **first step** in the daily workflow. It spawns background agents that check all 22 tracked jobs, pull build logs for failures, classify them, and draft Jira stories and fix PRs. The watcher reviews the AI's output, approves or corrects the classifications, and handles the 20% of cases that require human judgment (org context, escalation decisions, cross-team routing).

## Weekly Cadence

| Day | Task |
|-----|------|
| Monday | Read incoming handover document. Run `/ci-triage` to verify continuity of tracked issues. Post opening status |
| Mon-Thu | Daily triage: run `/ci-triage`, review findings, approve/correct, post status. Shepherd open PRs |
| Friday | Write handover document (CI-HANDOVER-YYYY-MM-DD.md). Post to Slack. Tag next watcher |

## Communication

- Post daily status updates to `#wg-rosa-ci-enhancement`
- Escalate critical/blocking issues in `#wg-rosa-ci-enhancement`
- Use `@rosa-ci-watcher` Slack alias for cross-team coordination
- Before EOB Friday: post weekly CI status summary and handover notes

## Anti-Patterns

- **Do not dismiss failures as "flaky" and retrigger.** Every failure has a root cause. Find it
- **Do not increase timeouts to mask infrastructure problems.** Longer timeouts degrade pipelines without surfacing the real issue
- **Do not leave cluster profiles in a "no test" state.** Private clusters, external auth, and other profiles that consistently fail need investigation, not skip-listing
- Do not page on-call for a red nightly
- Do not self-lgtm PRs authored by your own fork. Ask a reviewer
- Prefer fixing the root cause (PromQL, test assertion, config) over skip-listing. Only skip with a tracked upstream bug
