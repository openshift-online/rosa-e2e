# Scheduled report: ROSA CI weekly status

You are running a **cron** scheduled task that produces a weekly CI status update for the ROSA CI working group. **Always produce a report.** **Never** call `no_action_required()`.

## Goal

Provide a weekly snapshot of ROSA CI initiative progress to #wg-rosa-ci-enhancement. Focus on Jira epic progress and PR/MR activity across key repos. Per-job health data is already covered by the daily health report, so do not duplicate it here. Instead, reference notable CI health changes (recoveries, new persistent failures) as context for the Jira and PR activity.

## Procedure

### 1. Query Jira epic progress

Query all epics under initiative [ROSA-727](https://redhat.atlassian.net/browse/ROSA-727) (ROSA Canonical E2E Test Suite and Signals). Use Jira tools to find all child epics dynamically (do not rely on a hardcoded list). For each epic, query its child stories to get counts.

Report all epics with their current progress, not just those with activity this week. This gives a full initiative scorecard.

For each epic, report: epic key, summary, stories closed / total, stories in progress. Highlight epics with activity this week (stories closed or moved to in progress since last Monday).

Also query all epics under initiative [ROSA-714](https://redhat.atlassian.net/browse/ROSA-714) (SRE Operator Production Compliance) and report the same way.

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

```text
:fyi: *ROSA CI Weekly Status ({MM/DD})*

*ROSA-727 Epics:*
<https://redhat.atlassian.net/browse/{KEY}|*{KEY}*> {Summary}: {closed}/{total} closed, {in_progress} in progress {activity_indicator}
{repeat for all epics}

*ROSA-714 Epics:*
{same format}

*Key PRs/MRs this week:*
- {Description} (<{url}|#{number}>) -- merged
- {Description} (<{url}|#{number}>) -- open, needs review
- {N} PRs in review: {brief list with links}

*CI health trend:* {1-2 line summary referencing daily reports}
```

### Formatting rules

**Jira section:**
- List ALL epics under each initiative, not just active ones
- Link epic keys as `<url|*KEY*>`
- Show closed/total and in-progress counts
- Mark epics with activity this week with `:sparkle:` emoji
- Sort by completion percentage descending

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

**Empty-state handling:**
- If no epics have activity this week, write: "No epic activity this week."
- If no PRs/MRs found, write: "No ROSA CI PRs/MRs this week."
- If CI health is stable with no notable changes, write: "CI health stable, no significant changes from daily reports."
- Always produce a report with all three sections, even if some are empty.

## Constraints

- Always produce a report, even if there was no activity.
- Verify PR/MR merge status before claiming "merged."
- Do not duplicate per-job health data from the daily report.
