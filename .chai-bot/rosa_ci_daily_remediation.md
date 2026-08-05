# ROSA CI daily remediation — reference instructions

> **This file is NOT a standalone cron task.** It is triggered via `schedule_followup` chaining from the daily health report (`rosa_ci_daily_health_report.md`). The health report posts its summary, then schedules a PR remediation follow-up, which in turn schedules a Jira remediation follow-up. All follow-ups fire in the same thread as the health report — threading is automatic.
>
> The follow-up description tells you which section to execute and how to read the handoff artifact. You do not need to resolve threading or read the artifact independently — that context is provided in the follow-up prompt.

## PR Remediation

This section covers automated PR fixes for pattern-matched failures and shepherding of existing `[ci-fix]` PRs.

### Auto-fix PRs (for pattern-matched failures)

Scan the artifact for failures that match fixable patterns. Prioritize by severity (lowest pass rate first, highest consecutive failures first).

**Skip fetch_error jobs:** Jobs with `failure_classification` of `"fetch_error"` represent data retrieval failures, not real test failures. Skip these entirely — do not open PRs or count them as failures.

**Conformance skip list pattern:**

If a conformance test (HCP or Classic STS) is failing persistently (3+ consecutive failures) and the failing test is in an OCP-owned sig (sig-apps, sig-auth, sig-network, sig-storage), AND the same test is NOT failing in rosa-e2e HCP/STS jobs (confirming it's upstream, not ROSA-specific):

1. Search for existing open PRs in `openshift/release` with `[ci-fix]` in the title targeting the same test. If found, skip and note the existing PR — it will be shepherded in the next sub-step.
2. Clone `openshift/release` via workspace tools
3. Add the test name to the `TEST_SKIPS` env var in the appropriate workflow YAML:
   - HCP: `ci-operator/step-registry/rosa/aws/hcp/conformance/rosa-aws-hcp-conformance-workflow.yaml`
   - Classic STS: `ci-operator/step-registry/rosa/aws/sts/conformance/rosa-aws-sts-conformance-workflow.yaml`
4. Run `make jobs` to regenerate Prow job configs
5. Scan the diff for sensitive content (credentials, IP addresses, account IDs) before pushing
6. Open a PR with title `[ci-fix] Skip <test-name> in <workflow> (upstream OCP regression)`
7. PR description must link to the failing Prow job run(s) and reference the upstream OCP bug if identifiable

**Constraints:**
- Maximum **5** auto-fix PRs per scheduled run
- Allowed repos for fixes:
  - `openshift/release` (step registry, workflow YAMLs)
  - `openshift-online/rosa-e2e` (test code)
  - `service/ocm-backend-tests` (FVT test code on GitLab)
  - `openshift/origin` (conformance test fixes)
  - `openshift/rosa` (ROSA CLI)
  - Any SRE operator repo referenced in the `components` field of `ci-status-jobs.yaml` (e.g., `openshift/route-monitor-operator`, `openshift/configure-alertmanager-operator`, `openshift/pagerduty-operator`, `openshift/deadmanssnitch-operator`, `openshift/certman-operator`, `openshift/managed-upgrade-operator`, `openshift/must-gather-operator`, `openshift/managed-velero-operator`, `openshift/dedicated-admin-operator`, `openshift/rbac-permissions-operator`, `openshift/cloud-ingress-operator`, `openshift/aws-account-operator`)
- Never modify production configs (app-interface, managed-cluster-config)
- PRs require human `/lgtm` and `/approve` before merge (no auto-merge)

### PR shepherding

After handling new auto-fixes, shepherd open `[ci-fix]` PRs — both newly created and previously opened ones. Search for open PRs with `[ci-fix]` in the title across the allowed repos. Process at most **10 PRs per run** (prioritize newly created PRs first, then oldest existing PRs). If more than 10 open `[ci-fix]` PRs exist, note the overflow count in the summary reply.

**Stale PR cleanup (first):**
Before shepherding, check for any open `[ci-fix]` PRs older than 7 days. Only auto-close a PR if ALL of: (a) the PR was opened by the bot, (b) there has been no human comment, review, or CI activity in the last 3 days, and (c) the PR does not have a pending `/lgtm` or `/approve`. Close qualifying stale PRs with a comment explaining they were not reviewed in time. Preserve PRs that have recent human engagement — they may be awaiting review.

**CI status checks:**
1. Check each PR's CI status.
2. If checks are still running, note it and move on.
3. If CI failed, investigate the failure:
   - For `ci/prow/lint` or `ci/prow/images`: check if the failure is related to the fix or pre-existing on main
   - For rehearsals: wait for `[REHEARSALNOTIFIER]` comment, then run representative rehearsals via `/pj-rehearse <job-name>` (job names come from the rehearsal-notifier comment). Only `/pj-rehearse ack` after rehearsals pass. Never `auto-ack` or `skip`.
   - If the CI failure is caused by the fix itself, attempt to correct it, push an update, and note in the thread.
   - If the CI failure is pre-existing and unrelated, note it in the thread and proceed.

**CodeRabbitAI review handling:**
4. Check for comments from `coderabbitai` (CodeRabbit) on each PR.
5. Read each CodeRabbitAI comment carefully — they may flag code quality issues, suggest improvements, or ask clarifying questions.
6. For each CodeRabbitAI comment:
   - If the suggestion is valid and improves the code: implement the fix, push an update, and reply to the comment confirming the change was made.
   - If the suggestion is not applicable or the current approach is correct: reply to the specific comment explaining why the current approach is preferred (e.g., "This is a temporary skip list entry for an upstream regression — the test name must match the exact OCP test output").
   - If the comment asks a question: reply with a clear answer.
7. **Reply to every CodeRabbitAI comment individually** — do not leave any unaddressed. Use inline review replies, not PR-level comments.

**Human review handling:**
8. Check for comments from human reviewers. If there are unresolved comments, read them and attempt to address them (push code fixes, respond to questions, or explain the rationale for the change).

**Ready state:**
9. If all CI checks pass and no unresolved review comments remain (both CodeRabbitAI and human), post a threaded reply: "CI is green, all review comments addressed — ready for `/lgtm` and `/approve`"
10. For `openshift/release` PRs: remind that `/retest <job>` omits the `ci/prow/` prefix

The goal is that by the time a human looks at the PR, the only action needed is `/lgtm` and `/approve`.

### PR Remediation summary

After completing auto-fix PRs and PR shepherding, compose a summary of all PR actions taken:

```
:wrench: *PR Remediation summary — {DATE}*

*Auto-fix PRs:*
- Opened: {N} new PRs ({list with links})
- Shepherded: {N} existing PRs ({status of each})
- Closed stale: {N} PRs

*CodeRabbitAI:*
- Addressed: {N} comments across {M} PRs
```

**Action branch** (PRs were opened, shepherded, or closed): Post this summary using `send_response()`, then schedule the Jira follow-up via `schedule_followup`.

**No-action branch** (no fixable failures, no open PRs to shepherd): Do NOT post an empty summary. Schedule the Jira follow-up directly via `schedule_followup`, then call `no_action_required()`.

---

## Jira Remediation

This section covers Jira ticket creation for persistent non-fixable failures.

### Jira ticket creation (for non-fixable failures)

For persistent failures (3+ consecutive) where auto-fix PRs were not opened (the failure requires deeper investigation or a fix outside the allowed repos), create a Jira ticket so the owning team can investigate.

**Skip fetch_error jobs:** Jobs with `failure_classification` of `"fetch_error"` represent data retrieval failures, not real test failures. Skip these entirely — do not create Jira tickets for them.

Before creating a ticket, search Jira for existing open issues that already cover the same failure (search by job name or test name in ROSAENG and SREP projects). If found, skip and note the existing ticket.

**Team and label classification:**

Use the `team` and `labels` fields from the handoff artifact (originally sourced from `ci-status-jobs.yaml`):
- `team.id` maps to the Jira Team field (`customfield_10001`)
- `team.name` is for display only
- `team.slack_channel` is the team's Slack channel for notifications
- `team.slack_alias` is the team's Slack user group handle (e.g., `@sd-srep-team-hulk`)
- `labels` is the list of Jira labels to apply
- Job-level `team` and `labels` override category-level when present

If a category or job has no `team` field, fall back to ROSA CI (`97412673-7d28-430b-bdee-ce3d1eb702b2`) with label `ci-failure`.

**Team notifications:** When creating a Jira ticket, also post a notification to the team's `slack_channel` (if defined) mentioning the `slack_alias` (if defined). Keep the notification brief: link to the Jira ticket and a one-line summary of the failure.

For OCM FVT failures, also check cs-telemetry data (from the artifact's failure classification) to determine if the failure is CS-side (API errors, timeouts) vs test-side (assertion errors, framework issues). If test-side, use ROSA CI team instead of the category's team.

**Ticket format:**
- Type: Bug
- Summary: `[ci-failure] <Job display name>: <brief failure description>`
- Priority: Major (persistent) or Minor (intermittent)
- Parent epic: choose the most relevant open epic under these ROSA initiatives based on the failure type:
  - [ROSA-727](https://redhat.atlassian.net/browse/ROSA-727) (Canonical E2E Test Suite and Signals):
    - Search child epics of ROSA-727 for the best match based on the failure type, category, and component
  - [ROSA-714](https://redhat.atlassian.net/browse/ROSA-714) (SRE Operator Production Compliance):
    - SRE operator failures: search for an open epic matching the operator
  - [ROSA-798](https://redhat.atlassian.net/browse/ROSA-798) (OCM UI, ROSA CLI, and Terraform CI):
    - ROSA CLI E2E failures (rosa-cli-jobs): search for an open epic under ROSA-798
    - Terraform provider failures (terraform-provider-rhcs-jobs): search for an open epic under ROSA-798
  - If unsure, use ROSAENG-391 as the fallback
- Labels: from the `labels` field in the artifact
- Description: include the diagnosis from the health report threaded reply, links to failing Prow runs, and any cs-telemetry findings
- Security Level: Red Hat Employee (id: 10034)

**Constraints:**
- Maximum **4** Jira tickets per scheduled run
- Only create tickets for persistent failures (3+ consecutive), not intermittent flakes
- Always search for existing open tickets first to avoid duplicates

### Jira Remediation summary

After completing Jira ticket creation, compose a summary of all Jira actions taken:

```
:ticket: *Jira Remediation summary — {DATE}*

*Jira tickets:*
- Created: {N} tickets ({list with links})
- Skipped: {N} (existing tickets found)
```

Post this summary using `send_response()`. If no Jira actions were taken (no persistent failures needing tickets, or existing tickets already cover all failures), call `no_action_required()` instead.

---

## Constraints

- Maximum 5 auto-fix PRs and 4 Jira tickets per run.
- Never modify production configs (`app-interface`, `managed-cluster-config`).
- PRs require human `/lgtm` and `/approve` before merge.
- If the handoff artifact is missing or stale (not today's date), call `no_action_required()`.
- Each follow-up gets its own token budget (fresh context). Re-read the artifact and these instructions from the repo at the start of each follow-up.
- Only ONE pending follow-up per thread at a time. The PR remediation follow-up schedules the Jira follow-up; do not schedule both from the same turn.
- `send_response()` ends the turn immediately — no tool calls after it except `schedule_followup`.
