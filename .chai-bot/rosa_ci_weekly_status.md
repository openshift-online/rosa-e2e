# Scheduled report: ROSA CI weekly status

**Always produce a report.** **Never** call `no_action_required()`.

## Goal

Weekly snapshot of ROSA CI initiative progress for #wg-rosa-ci-enhancement. Epic scorecards + key PRs. Scannable in 15 seconds.

## Procedure

### 1. Query Jira epic progress

Query all child epics under [ROSA-727](https://redhat.atlassian.net/browse/ROSA-727) dynamically (do not hardcode epic lists). Do NOT include ROSA-714 (separate initiative, different owner).

**IMPORTANT**: Count ALL child issue types (Stories, Tasks, Bugs, Sub-tasks). Use `parent = <EPIC-KEY>` which returns everything. Do NOT filter by issue type.

For each epic, query:
- Total: `parent = <EPIC-KEY>` (count)
- Closed: `parent = <EPIC-KEY> AND status = Closed` (count)
- In Progress: `parent = <EPIC-KEY> AND status = "In Progress"` (count)

### 2. Find key PRs/MRs (last 7 days)

Search merged or notable open PRs across: `openshift-online/rosa-e2e`, `openshift/release` (ROSA-related), SRE operator repos. Keep to 3-5 most impactful. Skip dependency bumps.

### 3. Post the report

```
:fyi: *ROSA CI Weekly Status ({MM/DD})*

*<url|ROSA-727> — E2E Test Suite and Signals*
_N closed:_ KEY (x/y), KEY (x/y), ...
• KEY Summary: x/y closed, z IP :sparkle:
• KEY Summary: x/y closed
• KEY Summary: x/y — planned
{top 5 open epics only, plus count of remaining}

*PRs:*
- Description (repo#N) -- merged
- Description (repo#N) -- open

*CI trend:* {overall direction} — {one sentence detail, e.g. ":large_green_circle: Improving — HCP conformance recovered to 100%, Classic STS up to 71%"}
```

### Rules

- Group closed epics in one italic line (don't list separately)
- Open epics: bullet per epic, sorted by completion % descending, **show top 5 only**. If more than 5, add "_+ N more in planning_" at the end.
- `:sparkle:` = activity this week (stories closed or moved to IP since last Monday)
- Epics with 0 stories: "no stories yet"
- Link epics: `<url|*KEY*>`, link PRs: `(<url|repo#N>)`
- CI trend: Always start with overall direction (:large_green_circle: Improving / :large_yellow_circle: Stable / :red_circle: Degrading), then ONE sentence of detail. Don't reproduce per-job data.
- ONE message, no threads, Slack mrkdwn
- If no activity: "No epic activity this week." / "No PRs this week."
- Always produce all sections even if empty

## Constraints

- Always produce a report.
- Verify PR merge status before claiming "merged."
- Count ALL child issue types, not just Stories.
