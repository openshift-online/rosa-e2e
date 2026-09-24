# Scheduled report: ROSA CI Watcher weekly handover

You are running a **cron** scheduled task that posts a weekly handover message for the incoming CI Watcher shift. The message tags the `@rosa-ci-watcher` user group in #wg-rosa-cicd so the incoming ICs know what happened last week and what to pick up.

**Always produce a report.** **Never** call `no_action_required()`.

## Goal

Collect all CI triage activity from the past 7 days (failure threads in #rosa-prow-info, ROSA CI team Jiras, merged/open CI fix PRs, current CI health data), and post a handover message with:
1. What was fixed, what's in progress, what's still open
2. Unresolved #rosa-prow-info threads the incoming watcher needs to pick up
3. Merged and open PRs needing attention
4. Links to key dashboards and tools
5. Role reminders from the CI Watcher docs

## Procedure

### 1. Scan #rosa-prow-info failure threads

The primary source of CI triage activity is `#rosa-prow-info` (`C0AT31ERJLS`). Search the channel for messages from the past 7 days using Slack tools.

For each failure notification thread found:
- Read the thread replies to determine its current state
- Classify as: **Resolved** (root cause identified, fix merged or skip-listed), **In Progress** (actively being investigated, has recent replies), or **Unresolved** (no replies, or stale with no conclusion)
- Capture: job name, failure summary, thread link, and current state

Focus on threads with failure notifications (red builds, test failures, infrastructure errors). Skip informational posts and green build notifications.

Unresolved threads are the most important output of this step. These are what the incoming watcher needs to pick up and run to resolution.

### 2. Gather CI triage Jiras

Supplement the #rosa-prow-info thread data with Jira context. Run two queries and merge the results (deduplicate by issue key):

**Query A: ROSA CI team issues (last 7 days)**
```
project = ROSAENG AND "Team" = "ROSA CI" AND updated >= -7d ORDER BY status ASC, updated DESC
```
The `Team` field is `customfield_10001` (value `97412673-7d28-430b-bdee-ce3d1eb702b2`). This catches manually created CI issues and any OCM Bot issues that were assigned to the team.

**Query B: OCM Bot auto-filed failures (last 14 days)**
```
project = ROSAENG AND reporter = "OCM Bot" AND created >= -14d ORDER BY created DESC
```
The OCM Bot reporter account ID is `712020:638f9112-afac-4c1c-9f45-ff7a265267d6`. Not all OCM Bot issues are assigned to ROSA CI, so this query uses a wider 14-day window to catch failures that may not have been triaged yet.

Fields to fetch: `summary`, `status`, `assignee`, `priority`, `labels`, `updated`, `reporter`, `comment` (most recent comment only).

Merge both result sets and deduplicate by issue key. Categorize into three buckets:

- **Fixed** (status = Done or Closed AND `resolved >= -7d`)
- **In Progress** (status = In Progress, Code Review, Fix In Progress, or similar active states)
- **Still Open** (any other non-closed status with recent activity)

For each Jira, capture: key, summary, current status, and the most recent comment (for context in handover).

If neither query returned results, note "No CI triage Jiras this week" and continue to the next step.

### 3. Gather CI fix PRs from GitHub

Search for PRs related to CI work across key repos. Use GitHub tools or `fetch_web_content` on the GitHub API.

**Merged PRs (last 7 days):**
Search for recently merged PRs authored by the bot or referencing CI fix patterns in:
- `openshift-online/rosa-e2e`
- `openshift-online/rosa-backend-tests`
- `openshift/release` (filter to ROSA-related: step registry, rosa workflows)

Look for PRs with `[ci-fix]` in the title or referencing ROSA CI team Jiras.

**Open PRs needing attention:**
Search for open PRs with `[ci-fix]` in the title or referencing ROSA CI team Jiras across the same repos. Also search for open chai-bot PRs using:
```
is:pr author:redhat-chai-bot state:open archived:false sort:updated-desc
```
These are PRs that the incoming watcher may need to shepherd (request review, retest, address CodeRabbit comments).

Keep to the most impactful 5-8 PRs. Skip dependency bumps and unrelated changes.

### 4. Check CI Health dashboard context

Before composing the message, check the current CI health data using the same approach as the daily health report:

Fetch the job registry from `https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/ci-status-jobs.yaml` and query Prow for the latest pass rates per category. Use the same parameters as the daily health report: exclude pending builds from totals, and distinguish jobs that returned a fetch error from jobs with zero runs (the former is an infrastructure issue, the latter means the job hasn't been triggered). Compute per-category health indicators using the same thresholds as the daily report:
- :large_green_circle: >= 80%
- :large_yellow_circle: >= 40% and < 80%
- :red_circle: < 40%

This gives the incoming watcher a quick snapshot of where things stand right now.

### 5. Compose the handover message

Post one Slack message to the `#wg-rosa-cicd` channel. Use Slack `mrkdwn` formatting. The message tags `<!subteam^S0B7Q6G7XQR>` to notify the incoming `@rosa-ci-watcher` ICs.

**Template:**

```
:wave: <!subteam^S0B7Q6G7XQR> CI Watcher Handover (week of {WEEK_START_DATE})

*CI Health snapshot:*
{emoji} *{Category}:* {rate}% ({pass}/{total})
...
{one line per category with data, sorted by pass rate ascending so red/yellow are first}

{IF unresolved threads}
:rotating_light: *{unresolved_count} unresolved #rosa-prow-info threads need attention* (see thread below)
{END}

{IF fixed items}
:large_green_circle: *Fixed this week:*
{FOR each fixed Jira}
- <{jira_url}|{key}>: {summary}
{END}
{END}

{IF in_progress items}
:large_yellow_circle: *In progress / monitoring:*
{FOR each in_progress Jira}
- <{jira_url}|{key}>: {summary} ({status})
{END}
{END}

{IF still_open items}
:red_circle: *Still open:*
{FOR each open Jira}
- <{jira_url}|{key}>: {summary} ({status})
{END}
{END}

{IF merged PRs}
*Merged this week:*
{FOR each merged PR}
- <{url}|{repo}#{number}>: {title}
{END}
{END}

{IF open PRs needing review}
*Open PRs needing review/shepherd:*
{FOR each open PR}
- <{url}|{repo}#{number}>: {title}
{END}
{END}

*Before you start:*
- <https://rosa-eng-dashboard.apps.engineering.openshift.org/executive#ci-health|CI Health dashboard> -- check triage states for in-progress items from last week
- <https://redhat-internal.slack.com/archives/C0AT31ERJLS|#rosa-prow-info> -- weekend/overnight failure notifications
- <https://redhat.atlassian.net/issues?filter=116619|Active CI investigations> -- open ROSA CI issues
- Run `/ci-triage` in Claude Code to get current status (requires <https://github.com/openshift-online/rosa-claude-plugins|rosa-claude-plugins> plugin)

*Role reminders:*
:white_check_mark: Every failure thread in <https://redhat-internal.slack.com/archives/C0AT31ERJLS|#rosa-prow-info> must be run to resolution (root cause identified, fix merged or skip-listed, thread updated)
:white_check_mark: Triage within 24h of new failures
:white_check_mark: Read the logs before routing -- classify every failure first (<https://github.com/openshift-online/rosa-e2e/blob/main/docs/ci-watcher/role-and-responsibilities.md#triage-before-routing|triage checklist>)
:white_check_mark: Post daily status as a reply to the chai-bot thread in <https://redhat-internal.slack.com/archives/C0ADGRNAT8U|#wg-rosa-cicd>
:white_check_mark: Update triage state on the <https://rosa-eng-dashboard.apps.engineering.openshift.org/executive#ci-health|CI Health dashboard> as you investigate
:white_check_mark: File Jiras in the ROSAENG project with the `ci-watcher` label and assign to the ROSA CI team for persistent failures (2+ consecutive)
:white_check_mark: Do not page on-call for a red nightly

*Docs:* <https://github.com/openshift-online/rosa-e2e/tree/main/docs/ci-watcher|CI Watcher docs> · <https://github.com/openshift-online/rosa-e2e/blob/main/docs/ci-watcher/runbook.md|Runbook> · <https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage|Sippy>
```

### Rules

- **Always include the CI Health snapshot, "Before you start", "Role reminders", and "Docs" sections**, even if there are no Jiras or PRs to report.
- If there are no items in a section (no fixed Jiras, no open PRs), omit that section entirely. Do NOT include empty sections.
- Keep Jira summaries to one line. Include the most recent comment only if it adds context the summary doesn't have.
- For open PRs, include only those related to CI health. Skip WIP/draft PRs unless they're blocking something.
- For the CI Health snapshot, use the same job registry and health indicators as the daily report. Show all categories with data. This is a quick snapshot, not a full analysis; do not include threaded replies.
- Keep the main message scannable. If there are unresolved #rosa-prow-info threads, post a single **threaded reply** listing each one with a link and failure summary:
  ```
  *Unresolved #rosa-prow-info threads:*
  - <{thread_url}|{job_name}>: {failure_summary}
  - <{thread_url}|{job_name}>: {failure_summary}
  ...
  ```
  Only the thread reply contains individual links. The main message just shows the count.
- Use `<!subteam^S0B7Q6G7XQR>` (the Slack API syntax for user group mention), not `@rosa-ci-watcher`.

## Constraints

- Always produce a report, even if there was no triage activity (say "No CI triage activity this week" and still include the CI health snapshot and role reminders).
- Keep the message under 2500 characters. The message should be a handover brief, not a report. If there are too many items, summarize and link to the Jira filter.
- Verify PR merge status before claiming "merged."
- Do not include the `[Scheduled task: ...]` metadata line in the output.

---

## Monthly schedule maintenance (first week of month only)

Run this step only if today's day-of-month in UTC is between 1 and 7 (i.e., this is the first Monday handover of the month). Otherwise skip entirely.

### Goal

Keep `data/teams/sd-sre/schedules/rosa-ci-watcher.yml` in app-interface in sync with the current ROSA org. The schedule should include all ICs in the ROSA org except PMs, HyperFleet members, and GovCloud/FedRAMP members.

### 1. Read the current schedule

Fetch the current schedule from app-interface:
`https://gitlab.cee.redhat.com/service/app-interface/-/raw/master/data/teams/sd-sre/schedules/rosa-ci-watcher.yml`

Extract the list of `$ref` paths currently in the schedule.

### 2. Read the eligible IC pool from hybrid-platforms/org

Fetch all team YAML files under `config/structures/hybrid_platforms/rosa/teams/` from the hybrid-platforms/org GitLab repo (`https://gitlab.cee.redhat.com/hybrid-platforms/org`).

**Exclude these teams entirely:**
- `rosa_hyperfleet`
- `rosa_trust_fedramp_core`
- `rosa_trust_govcloud_sre`

For the remaining teams, collect kerberos IDs from two sources:

1. **Explicit role listings**: people in `roles[].people.ids` whose role type is NOT `manager`, `product_manager`, or `program_manager`. Include `staff_engineer` and `team_lead` role types — these are senior ICs, not managers.

2. **LDAP-resolved teams**: for teams that use `group.people.select` without an explicit IC role listing, get the current members from the Jira team page linked in `resources[name=Jira Team].url`. Fetch member account IDs via the Atlassian API and resolve kerberos IDs via LDAP: `ldapsearch -x -H ldap://ldap.corp.redhat.com -b "ou=users,dc=redhat,dc=com" "(displayName=FULL NAME)" uid`. Exclude anyone who appears in the team's `roles[]` with a manager, product_manager, or program_manager type.

Also include `staff_engineer` IDs from the top-level `config/structures/hybrid_platforms/rosa/org.yaml` — these are cross-cutting ICs who should be in the pool.

### 3. Map kerberos IDs to app-interface $ref paths

For each kerberos ID in the eligible pool, find their user YAML file in app-interface. Search `data/teams/*/users/` for files matching the kerberos ID (e.g., `drow.yml`, `drow.yaml`). Use the GitLab API to search:
`GET /api/v4/projects/service%2Fapp-interface/repository/tree?path=data/teams/{team}/users&ref=master`

Try common teams first: `sd-sre`, `ocm`, then search others if not found. Construct the `$ref` as `/teams/{team}/users/{kerberos-id}.{ext}`.

For any kerberos ID where no app-interface user file can be found, add it to a separate **unresolved set** — do not drop it silently. These are reported regardless of whether an MR is opened.

### 4. Compute the diff

Build two sets from step 3:
- **Resolved pool**: `$ref` paths for kerberos IDs with confirmed app-interface files
- **Unresolved**: kerberos IDs with no app-interface file found

Before comparing, extract the kerberos ID from each current schedule `$ref` path by taking the filename without extension (e.g., `/teams/sd-sre/users/drow.yml` -> `drow`). This normalizes both sets to the same key space.

Compare the resolved pool's `$ref` set with the current schedule's `$ref` set:
- **To add**: in resolved pool but not in any schedule entry
- **To remove**: schedule entry whose kerberos ID is NOT in the resolved pool AND NOT in the unresolved set

**Do not remove a person from the schedule if their kerberos ID is in the unresolved set** — a failed lookup is not confirmation they left the org. Only remove entries when the person is confirmed absent from the eligible pool (their team was excluded or their role type is excluded).

If there are no additions and no removals, skip to step 6 (no MR needed) — but still report unresolved IDs if any.

### 5. Generate the updated schedule and open an MR

Clone the app-interface repo (`https://gitlab.cee.redhat.com/service/app-interface`), create a branch named `rosa-ci-watcher-{YYYY-MM}`, and update the schedule file:

- Remove entries for people no longer in the pool. If their scheduled week is in the future, remove the entry; if it's already past, leave it.
- Append new entries for added people at the end of the schedule, each one week after the last existing entry, continuing the Monday 09:00 UTC cadence.
- Update the comment on line 13 to reflect the new IC count.

Commit with message: `rosa-ci-watcher: monthly rotation pool update ({MONTH} {YEAR})`

Open an MR against `master` targeting `service/app-interface` with title:
`rosa-ci-watcher: monthly rotation pool update ({MONTH} {YEAR})`

### 6. Report in Slack

Add a line to the handover message (or post a threaded reply if the main message is near the character limit):

- If an MR was opened: `:calendar: *Monthly schedule update:* Added {N}, removed {M} — <{mr_url}|MR !{number}>. Unresolved (no app-interface file): {list or "none"}.`
- If no changes: `:calendar: *Monthly schedule check:* pool unchanged, no MR needed. Unresolved (no app-interface file): {list or "none"}.`
- If this is not the first week of the month: omit entirely.
