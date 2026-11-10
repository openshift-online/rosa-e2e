# ROSA CI permafail ladder

Daily enforcement pass over open `[ci-failure]` Jiras (ROSAENG-75797, part of the ROSAENG-309 ownership model). The daily health report and its remediation follow-ups **file** failures and route them to owning teams; this task makes sure those Jiras do not silently rot. It never fixes, transitions, or closes anything itself: it measures clocks, escalates misses, and reports. Enforcement actions stay with the owning teams and the CI Watcher.

## Vocabulary (Jira state machine)

All state is Jira-native; no external tracking. Labels auto-create in Jira Cloud on first use.

- **Filed (T+0)**: remediation creates a Bug with summary `[ci-failure] <Job display name>: ...`, the owning team in the Team field (`customfield_10001`), and the registry labels (always includes `ci-failure`).
- **Ack**: any transition out of `New` by the owning team. A comment alone is not an ack.
- **Triage**: the team adds exactly one classification label: `ci-triaged-test-bug`, `ci-triaged-product-bug`, `ci-triaged-infra`, or `ci-triaged-upstream`.
- **Grace**: the team adds `ci-grace` plus a due date on the issue (e.g., waiting on an upstream release). Clocks pause while `ci-grace` is set and the due date is today or later. When the due date passes, the issue re-enters the ladder with its original clocks.

## Clocks

Two clock kinds, measured from the issue's `created` date:

- **Business-day clocks** (Mon-Fri; skip Sat/Sun, do not skip holidays): ack due at T+2, triage due at T+3 (gating) or T+5 (informing).
- **Calendar-day clocks**: T+7 weekly-table naming, T+14 consequence-eligible.

Business-day math: count days from `created`, excluding Saturdays and Sundays. An issue created Thursday is at T+2 business days on Monday.

Gating vs informing per issue: fetch the generated job registry (URL below), find the category whose job `name` matches the job display name in the issue summary (fall back to matching the issue's labels against category `labels`). The job is **gating** if the category sets `gating: true` and the job itself does not override `gating: false`; otherwise **informing**.

## Rungs

| Rung | Condition | Action |
|------|-----------|--------|
| T+1 business day | still `New` | Reminder posted to the team's channel |
| T+2 business days | still `New` (ack missed) | Escalation contact named in the team's channel + `[ci-ladder]` comment on the issue |
| T+3 business days (gating) / T+5 (informing) | acked but no `ci-triaged-*` label (triage missed) | Flagged in the digest for the on-shift CI Watcher to investigate |
| T+7 calendar days | still unresolved | Listed in the weekly-table section of the digest, with team names |
| T+14 calendar days | still unresolved | Labeled `ci-consequence-eligible` + `[ci-ladder]` comment on the issue (consequence mechanics are ROSAENG-75798, not this task) |

Each rung fires once per issue, not daily. Before acting on a rung, check the issue's comments for an existing `[ci-ladder] T+<N>` marker and skip if present. When a rung acts, record it as a Jira comment on the issue starting with `[ci-ladder] T+<N> <date>` so the audit trail lives on the issue.

## Step 1: Gather data

1. Query unacked issues with `query_jira` (pass `customfield_10001` in `extra_fields` so the Team field comes back):

   ```
   project = ROSAENG AND labels = "ci-failure" AND resolution = Unresolved AND status = New AND NOT (labels = "ci-grace" AND duedate >= startOfDay()) ORDER BY created ASC
   ```

2. Query acked-but-untriaged issues the same way:

   ```
   project = ROSAENG AND labels = "ci-failure" AND resolution = Unresolved AND status != New AND NOT labels IN ("ci-triaged-test-bug", "ci-triaged-product-bug", "ci-triaged-infra", "ci-triaged-upstream") AND NOT (labels = "ci-grace" AND duedate >= startOfDay()) ORDER BY created ASC
   ```

   Page through with `next_page_token` until exhausted (cap at 200 issues total; if the cap is hit, note it in the digest).

3. Fetch the generated job registry with `fetch_web_content`:
   `https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/generated/ci-status-jobs.yaml`
   (gating classification and team slack channels)

4. Fetch the org team snapshot with `fetch_web_content`:
   `https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/generated/org-teams.yaml`
   (escalation contacts, keyed by `jira_team_id`)

If either URL fails to fetch (e.g., the file is not on main yet), proceed with what is available: gating defaults to informing (T+5), and escalation pings fall back to naming the team only. Note the degraded mode in the digest.

## Step 2: Classify each issue

For every issue from both queries, compute:

- **Team**: the `customfield_10001` UUID -> the registry category whose `team.id` matches -> `team.name`, `team.slack_channel`. If no category matches, the issue is **unrouted**: list it in the digest under "Unrouted" (no team to ping); do not guess.
- **Escalation contact**: the same UUID -> the org snapshot team with matching `jira_team_id` -> `escalation.person` (a kerberos id; render as a plain name, e.g., "njean").
- **Gating**: per the clock rules above.
- **Age**: business days and calendar days since `created`.

## Step 3: Act on rungs

For each issue, determine the highest rung it has reached and not yet had fired (checking `[ci-ladder]` comment markers), then act:

- **T+1 reminder** (`send_message` to the team's `slack_channel` if set; skip silently if not): one line, issue link, "filed {N} business day(s) ago, still unacked". Do not mention individuals at this rung.
- **T+2 escalation** (same channel): name the escalation contact in bold (e.g., "*njean*") with the issue link and "ack overdue since {date}". Also post the `[ci-ladder]` Jira comment via `comment_on_jira_issue`.
- **T+3/T+5 watcher flag**: no team ping; the issue is listed in the digest under the CI Watcher section with a one-line "triage overdue, acked {date}".
- **T+14 consequence-eligible**: add the `ci-consequence-eligible` label via `manage_jira_labels` and post the `[ci-ladder]` Jira comment via `comment_on_jira_issue`. Do not change status, priority, or any other field.

Cap team-channel pings at 10 per run (T+1 and T+2 combined); if more issues qualify, list the overflow in the digest only.

Never: transition an issue's status, resolve or close issues, edit summaries/descriptions, or remove labels. This task adds labels only where a rung says so.

## Step 4: Digest

Compose ONE digest and post it with `send_response()`:

```
:ladder: *CI permafail ladder — {DATE}*

*Ack overdue (T+2):*
- {issue link} — {team name}, filed {date}, escalation pinged: {person}

*Triaged overdue (gating T+3 / informing T+5):*
- {issue link} — {team name}, acked {date}, awaiting classification (CI Watcher)

*Reminders sent (T+1):* {N}
*Consequence-eligible (T+14):* {issue links, or "none"}

*Weekly table (open 7+ calendar days):*
| Team | Issue | Age | State |
|------|-------|-----|-------|

*Unrouted:* {issue links with a note to fix the registry routing, or "none"}
```

Only include sections that have content. The weekly-table section appears only when at least one issue is 7+ calendar days old (it is the raw material for the weekly review, ROSAENG-75800).

If both queries return zero issues, call `no_action_required()` — do not post an empty digest.

## Constraints

- Read-only on Jira state except: `ci-consequence-eligible` label and `[ci-ladder]` comments, exactly where a rung specifies.
- One digest per run in this task's channel; team-channel pings via `send_message` only for T+1/T+2 rungs, capped at 10.
- `send_response()` ends the turn immediately — compose everything, write all Jira comments and labels first, then post the digest.
- If a query fails, retry once, then post a degraded digest noting which data is missing rather than failing silently.
