# Scheduled report: ROSA CI weekly status

You are running a **cron** scheduled task that produces a weekly CI status update for the ROSA CI working group. **Always produce a report.** **Never** call `no_action_required()`.

## Goal

Provide a weekly snapshot of ROSA CI initiative progress to #wg-rosa-ci-enhancement. Focus on Jira epic progress and PR/MR activity across key repos. Per-job health data is already covered by the daily health report, so do not duplicate it here. Instead, reference notable CI health changes (recoveries, new persistent failures) as context for the Jira and PR activity.

## Procedure

### 1. Query Jira epic progress

Look up the current status of the epics under initiative [ROSA-727](https://redhat.atlassian.net/browse/ROSA-727) (ROSA Canonical E2E Test Suite and Signals). Use Jira tools to query child epics and their story counts. Report any epics that have active work (stories in progress or recently closed this week).

Key epics to check:
- ROSAENG-391: E2E Suite Reliability & Component Readiness (main active epic)
- ROSAENG-326: Unified ROSA CI Visibility
- ROSAENG-394: CI Watcher Role and Rotation
- ROSAENG-307: CNCF Conformance to Prow and Enforce Release Gating
- ROSAENG-743: E2E Coverage Gap Improvement
- ROSAENG-308: Consolidate Customer-Facing Tests into rosa-e2e
- ROSAENG-309: E2E Test Ownership Model and Enforcement

For each epic with activity this week, report: how many stories closed this week, how many in progress, how many total.

Also check initiative [ROSA-714](https://redhat.atlassian.net/browse/ROSA-714) (SRE Operator Production Compliance) for any active epics.

### 2. Find key PRs/MRs from the past week

Search for recently opened or merged PRs/MRs from ALL contributors (not just one person) across these repos:

**GitHub:**
- `openshift-online/rosa-e2e` (test code, configs, ci-status-jobs.yaml)
- `openshift/release` (ROSA-related changes in step registry, job configs, workflow YAMLs)
- `openshift/rosa` (CLI changes affecting CI)
- `openshift/sippy` (ROSA Sippy view changes)
- `openshift/route-monitor-operator` (SRE operator CI)
- `openshift/configure-alertmanager-operator` (SRE operator CI)

**GitLab (gitlab.cee.redhat.com):**
- `service/ocm-backend-tests` (FVT test changes)
- `service/uhc-clusters-service` (CS changes affecting CI)

Use GitHub/GitLab tools to find PRs/MRs from the last 7 days. Include merged and notable open PRs/MRs.

### 3. Summarize CI health changes

Reference the daily health reports from the week (posted in #sd-cicd-rosa). Summarize in 1-2 lines:
- Any categories that recovered or degraded significantly during the week
- Any new persistent failures that emerged
- Overall trend (improving, stable, degrading)

Do NOT reproduce per-job pass rates. The daily report covers that.

### 4. Channel response

Post the report as your channel response. Format:

```
:fyi: *ROSA CI Weekly Status ({MM/DD})*

*Jira Progress:*
<https://redhat.atlassian.net/browse/ROSAENG-391|*ROSAENG-391*> E2E Reliability: {closed_this_week} closed, {in_progress} in progress ({total} total)
{other active epics with similar format}

*Key PRs/MRs this week:*
- {Description} (<{url}|#{number}>) -- merged
- {Description} (<{url}|#{number}>) -- open, needs review
- {N} PRs in review: {brief list with links}

*CI health trend:* {1-2 line summary referencing daily reports}
```

### Formatting rules

**Jira section:**
- Only include epics with activity this week (stories closed or moved to in progress)
- Link epic keys as `<url|*KEY*>`
- Show closed-this-week count, in-progress count, and total

**Key activity section:**
- Include merged PRs/MRs and notable open ones
- Link PRs as `(<url|#number>)` or `(<url|repo#number>)` for cross-repo
- Group related PRs when it makes sense (e.g., "3 FVT migration PRs merged")
- Keep descriptions brief, one line each
- Note status: merged, open/needs review, blocked

**CI health trend:**
- One or two lines max
- Reference the daily reports rather than reproducing data
- Call out significant week-over-week changes (e.g., "GAP 4.21/4.22 recovered after version enablement fix")

**Overall:**
- Keep the entire report in one message (no threaded replies)
- Use Slack `mrkdwn` formatting
- The report should be scannable in 15 seconds

## Constraints

- Always produce a report, even if there was no activity.
- Verify PR/MR merge status before claiming "merged."
- Do not duplicate per-job health data from the daily report.
