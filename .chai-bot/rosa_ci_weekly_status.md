# Scheduled report: ROSA CI weekly status

You are running a **cron** scheduled task that produces a weekly CI status update for the ROSA CI working group. **Always produce a report.** **Never** call `no_action_required()`.

## Goal

Provide a comprehensive weekly snapshot of ROSA CI health to #wg-rosa-ci-enhancement. This is the team's primary weekly status update, covering job health across all tracked jobs, Jira progress, and key PRs from the past week.

## Procedure

### 1. Load job registry

Fetch the job registry from:
`https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/ci-status-jobs.yaml`

Use `fetch_web_content` to retrieve this YAML file. It defines all jobs organized into categories with display names and Prow job names.

### 2. Collect 7-day build history

For each job, collect **all builds from the last 7 days** (not a fixed build count). Use Prow CI tools or `fetch_web_content` on the job-history page. Filter builds by timestamp to include only those from the past 7 days. Calculate each job's pass rate over that window.

### 3. Query Jira epic progress

Look up the current status of the epics under initiative [ROSA-727](https://redhat.atlassian.net/browse/ROSA-727) (ROSA Canonical E2E Test Suite and Signals). Use Jira tools to query child epics and their story counts. Report any epics that have active work (stories in progress or recently closed).

Key epics to check:
- ROSAENG-391: E2E Suite Reliability & Component Readiness (main active epic)
- ROSAENG-326: Unified ROSA CI Visibility
- ROSAENG-394: CI Watcher Role and Rotation
- ROSAENG-307: CNCF Conformance to Prow and Enforce Release Gating
- ROSAENG-743: E2E Coverage Gap Improvement
- ROSAENG-308: Consolidate Customer-Facing Tests into rosa-e2e
- ROSAENG-309: E2E Test Ownership Model and Enforcement

For each epic with activity, report: how many stories closed, how many in progress, how many total.

### 4. Find key PRs from the past week

Search for recently opened or merged PRs from ALL contributors (not just one person) across these repos:
- `openshift/release` (ROSA-related changes in step registry or job configs)
- `openshift-online/rosa-e2e`
- OCM backend test repos (FVT test changes)

Use GitHub tools or `fetch_web_content` to find PRs from the last 7 days. Include merged and notable open PRs.

### 5. Channel response

Post the report as your channel response. Format:

```
:fyi: *ROSA CI Weekly Status ({MM/DD})*
<https://redhat.atlassian.net/browse/ROSA-727|*ROSA-727*> Epics:
<https://redhat.atlassian.net/browse/ROSAENG-391|*ROSAENG-391*>: %X%% (%CLOSED%/%TOTAL% closed, %IN_PROGRESS% in progress)
{other active epics with similar format}

*Job Health (since last week):*
:large_green_circle: %CATEGORY% %JOB%: %RATE%%, %JOB%: %RATE%%
:large_green_circle: %CATEGORY% %JOB%: %RATE%%
:large_yellow_circle: %CATEGORY% %JOB%: %RATE%%, %JOB%: %RATE%%
:red_circle: %CATEGORY% %JOB%: %RATE%%, %JOB%: %RATE%%
{job with no builds}: no builds in last 7 days

*Key activity this week:*
- %DESCRIPTION% (<%PR_URL%|#%NUMBER%>)
- %DESCRIPTION% (<%PR_URL%|#%NUMBER%>)
- %N% FVT migration PRs in review: %LIST_WITH_LINKS%
```

### Formatting rules

**Job health section:**
- Group jobs by health tier (:large_green_circle: first, then :large_yellow_circle:, then :red_circle:)
- Within each tier, group by category (rosa-e2e, HCP conformance, Classic STS conformance, OCM FVT, etc.)
- Show per-job pass rates, not just per-category aggregates
- Combine healthy jobs on one line when they share the same category and similar rates (e.g., "rosa-e2e HCP 4.19-4.22 + 5.0: all 100%")
- For jobs with 0 builds in the window, list them separately at the end
- Thresholds: :large_green_circle: 80%+, :large_yellow_circle: 40-79%, :red_circle: below 40%
- Note if the latest run is failing for a job that otherwise has a good rate (e.g., "87%, latest failing")

**Key activity section:**
- Include merged PRs and notable open PRs
- Link PRs as `(<url|#number>)` or `(<url|repo #number>)` for cross-repo
- Group related PRs when it makes sense (e.g., "5 FVT migration PRs in review: ...")
- Keep descriptions brief, one line each

**Overall:**
- Keep the entire report in one message (no threaded replies for the weekly status)
- Use Slack `mrkdwn` formatting
- The report should be scannable in 30 seconds

## Constraints

- Use ALL builds from the last 7 days for pass rates, filtered by date. Do not use a fixed build count.
- Always produce a report, even if all jobs are healthy.
- Skip versions with no builds (e.g., 4.23 if no nightly is configured).
- Verify PR merge status before claiming "merged."
