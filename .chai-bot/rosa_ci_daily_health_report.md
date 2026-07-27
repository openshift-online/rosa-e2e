# Scheduled report: ROSA CI daily health report

You are running a **cron** scheduled task that produces a daily CI health report for ROSA (Red Hat OpenShift Service on AWS) jobs. Keep the report **as concise as possible** to minimize channel noise. When everything is healthy, a one-liner is fine. Only expand into detail when something needs attention.

## Goal

Check the pass/fail history (last completed builds over 7 days per job) for all ROSA CI periodic jobs across all categories defined in the job registry. Report per-category pass rates, 7-day trends, and failure classifications. If all categories are >= 80%, respond with a brief summary and `no_action_required()`. After reporting, write a YAML handoff artifact to the bot's fork for the remediation task.

## Procedure

### 1. Load job registry

Fetch the job registry from the single source of truth:
`https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/ci-status-jobs.yaml`

Use `fetch_web_content` to retrieve this YAML file. It defines all jobs organized into categories. Each category has an `id`, `name`, and list of `jobs` with `name` (display name) and `prow_job` (full Prow job name). There is also a top-level `sippy_url` for the dashboard link.

If the fetch fails, report the error and skip the health check. Do not use a hardcoded fallback (it goes stale and causes incorrect "no runs" reports).

**Procedure adherence:** Follow steps 1→2→3→4→5→6 sequentially. Do not use broad CI analysis tools as a shortcut for steps 1-3. The job registry is the authoritative source for which jobs to check and how to categorize them.

### 2. Collect build history

For each job in the registry, use Prow CI tools (`search_prow_jobs`, `query_prowjobs`, etc.) to find the **last completed builds over 7 days** (exclude PENDING). Record pass count, fail count, and build timestamps.

If Prow tools don't return historical build data directly, use `fetch_web_content` to retrieve the job-history page at `https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/{JOB_NAME}`. The HTML contains `var allBuilds = [{ID, Result, Started, Duration}];`.

**Important: fetch ALL categories.** There are 13+ categories with ~139 total jobs. Process every category completely. If a fetch fails or times out for a specific job, mark that job as "fetch error" (not "no runs") and continue with the next job. Do not skip entire categories due to fetch issues. A category should only show "no runs" if every job in it genuinely returned zero completed builds in the data, not because the fetch failed.

### 3. Compute pass rates and trends

**Per-category pass rate**: aggregate pass/fail across all jobs in each category.

**Health indicators** (order by green first, then yellow, then red):
- :large_green_circle: pass rate >= 80%
- :large_yellow_circle: pass rate >= 40% and < 80%
- :red_circle: pass rate < 40%
- :white_circle: no data

**7-day trend**: compare pass rate for builds in the last 7 days vs the previous 7 days:
- :chart_with_upwards_trend: improving (10+ percentage points higher)
- :chart_with_downwards_trend: degrading (10+ percentage points lower)
- :left_right_arrow: stable

### 4. Channel response (top-level summary)

Post a concise summary as your channel response. This is the top-level message that everyone sees. **Brevity is critical** -- this message posts daily to a busy channel.

**Do NOT include:**
- The `[Scheduled task: ...]` metadata line
- A `Source:` line referencing ci-status-jobs.yaml
- A "Key issues needing attention" section
- Per-job breakdowns or job names (those go in threaded replies)
- Categories with no Prow run data (no white circle lines)

**If all categories >= 80%**: respond with a single line like:
`:large_green_circle: *ROSA CI Daily Health -- {DATE}:* all {N} categories passing (overall {rate}%)`
Proceed to step 6 (write handoff artifact), then step 7 (deliver response and schedule thread_ts update).

**If any category < 80%**: use this format:

```
*ROSA CI Daily Health -- {DATE} -- {overall_rate}%*

{emoji} *{Category}:* {rate}% ({pass}/{total}) {trend} (<prow_filter|jobs>)
{emoji} *{Category}:* {rate}% ({pass}/{total}) {trend} -- {brief inline note} (<prow_filter|jobs>)
...

_{N} categories skipped (no runs) · <https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage|Sippy> · <https://prow.ci.openshift.org/?type=periodic&job=*rosa*|Prow>_
```

**Rules:**
- `{overall_rate}` is the weighted pass rate across all jobs with data (total passes / total builds, rounded to nearest integer).
- List categories with data, sorted by pass rate descending.
- For yellow/red categories, add a **short** inline note after the trend emoji (e.g., `-- AMD64 & E2E at 40%`, `-- stale since Jun 17`, `-- 1 run in 30d`). Keep notes under 40 characters.
- If any categories had zero Prow run data, mention the count in the footer line (e.g., `2 categories skipped (no runs)`). Omit this part if all categories have data.
- Combine Sippy and Prow links on the footer line separated by ` · `.
- Append a small `(<prow_filter_url|jobs>)` link at the end of each category line using the `prow_filter` URL from ci-status-jobs.yaml. This lets readers click through to the Prow job-history for that category.
- Do NOT repeat category details in a separate section below the list.

**Threading gate:** Before proceeding, check: does your summary contain any :red_circle: or :large_yellow_circle: categories? If yes, step 5 is **mandatory** — do not call `send_response()` until threaded replies are composed. If all categories are :large_green_circle:, call `send_response()` then skip to step 6.

### 5. Failure analysis (threaded replies)

After the top-level summary, include **separate threaded replies** for the selected failing categories (see scope cap below) using the delimiter-based threading system. Put `---THREAD_DETAILS---` after your main summary, then each threaded reply separated by `---THREAD_BREAK---`. One reply per analyzed category.

**Response delivery:** Compose your entire output — summary + all threaded replies — as a single `set_response_element` call. The `---THREAD_DETAILS---` delimiter separates the top-level message from threaded content. Do NOT use separate `set_response_element` calls for the summary and threads — the threading system splits on delimiters within a single element.

Example structure:
```
{top-level summary content}

---THREAD_DETAILS---

{first category failure analysis}

---THREAD_BREAK---

{second category failure analysis}
```

For each selected failing job in the category (up to the scope cap):
1. Fetch the build log from the most recent failure using Prow CI tools or `fetch_web_content` on the artifacts URL
2. Identify the specific failure: key error messages, failing test names, failing step
3. For OCM FVT jobs, also check the `cs-telemetry` logs in the Prow artifacts. These contain Clusters Service-side request/response data that can reveal CS errors, timeouts, or API failures that caused the test to fail. Look in the artifacts directory for files matching `cs-telemetry*` or `cs_telemetry*`.
4. Perform root cause analysis using Sippy, Prow CI tools, or other available tools
5. Classify the failure based on what you find in the logs
6. Note how frequently the job has failed recently (e.g., "3 of 7 runs failed this week")
7. Link to the failing Prow job run(s)

For deeper pass rate analysis, query the Sippy API:
`https://sippy.dptools.openshift.org/api/jobs?release=rosa-stage&limit=100`

Format each threaded reply like:

```
{emoji} *{Category} -- {rate}% pass rate* {trend}

*{Job Name}* -- {pass}/{total} (<job-history link>)
{Short summary of failure: key error, failing test/step}
Failing since {date}. {Root cause analysis.}

*{Job Name}* -- {pass}/{total} (<job-history link>)
{Short summary and analysis}
```

**Scope cap:** Analyze the 5 worst categories first (lowest pass rate). For each, analyze at most 5 failing jobs (worst pass rate first). This keeps the report actionable without requiring excessive tool calls. If more categories are failing, note the count in the last threaded reply (e.g., "2 additional categories below 80% — see Prow dashboard for details").

### Reference: common failure patterns

These are patterns that come up often. Use them as hints, not a rigid checklist. Classify failures however makes sense based on what you find in the logs.

- STS account-roles fallback crash: log ends with "checking available versions..." then exits (`set -o pipefail`)
- Conformance skip list: OCP-owned test regressions on latest nightly, not ROSA-specific
- VPC cleanup: leftover ENIs or security groups blocking deletion, usually self-resolving
- OCM login: `Cannot login` or `401 Unauthorized`, expired SSO credentials
- Boskos lease timeout: `failed to acquire lease`, all quota slices in use
- Prometheus alert flakes: transient alerts firing on fresh clusters

### 6. Write handoff artifact

**Before** calling `send_response()`, write a YAML handoff artifact for the remediation task. This step runs even if all categories are green (the remediation task uses the artifact for PR shepherding of previously opened PRs).

**Artifact location:** Push to the bot's fork of `openshift-online/rosa-e2e` at path `.chai-bot/reports/daily_health_latest.yaml`.

**Steps:**
1. Call `priv_scm_ensure_fork("github.com", "openshift-online/rosa-e2e")` to ensure the bot's fork exists and get the fork repo path.
2. Set `thread_reference.thread_ts` to `"pending"` — the actual thread_ts is not available until after `send_response()` creates the top-level message. It will be updated in step 7.
3. Compose the YAML artifact (schema below).
4. Use a workspace to clone the fork, write the file, commit, and push to the fork's default branch.
5. Verify the push succeeded. If cloning, committing, or pushing fails, post a warning in the thread: ":warning: Handoff artifact write failed — remediation task will not run today." Do NOT call `no_action_required()` without a successful push — the artifact is required for the remediation task.

**YAML schema:**

```yaml
report_date: "2026-07-25"
generated_at: "2026-07-25T14:45:00Z"
thread_reference:
  channel_id: "C0ADGRNAT8U"
  thread_ts: "1784990442.709069"
overall_pass_rate: 66
job_registry_url: "https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/ci-status-jobs.yaml"
categories:
  - id: "rosa-hcp-e2e"
    name: "ROSA HCP E2E"
    pass_rate: 85
    trend: "stable"
    health: "green"
    jobs:
      - name: "HCP Day1 Validation"
        prow_job: "periodic-ci-openshift-online-rosa-e2e-master-..."
        pass_count: 6
        fail_count: 1
        total: 7
        pass_rate: 86
        last_failure_url: "https://prow.ci.openshift.org/view/gs/..."
        last_failure_date: "2026-07-23"
        failure_classification: "VPC cleanup timeout"
        consecutive_failures: 0
        failing_tests:
          - "[sig-apps] Deployment should run the lifecycle of a Deployment"
        failure_summary: "Conformance test failure in sig-apps Deployment lifecycle test"
        diagnosis: "Test fails on OCP 4.18 nightly, upstream regression. Not ROSA-specific."
        team:
          id: "97412673-7d28-430b-bdee-ce3d1eb702b2"
          name: "ROSA CI"
          slack_channel: "#rosa-ci"
          slack_alias: "@rosa-ci-team"
        labels:
          - "ci-failure"
          - "rosa-hcp"
```

**Field notes:**
- `thread_reference`: channel_id is always `C0ADGRNAT8U`. thread_ts is initially `pending` and updated ~2 minutes after the health report posts via a scheduled follow-up. The remediation task uses this to post threaded replies to the same thread.
- Include **ALL** categories and **ALL** jobs, not just failing ones. The remediation task needs the full picture.
- `consecutive_failures`: count of consecutive recent failed builds (0 if the latest passed).
- `failure_classification`: short label from your analysis (e.g., "conformance skip list", "STS account-roles crash", "Boskos lease timeout"). Empty string if the job is passing.
- `failing_tests`: list of specific test names or step names that are failing. Empty list if the job is passing. The remediation task uses these to create skip-list PRs and accurate Jira ticket descriptions.
- `failure_summary`: one-line human-readable summary of the failure diagnosis from step 5. Empty string if the job is passing.
- `diagnosis`: structured diagnostic evidence including error messages, cs-telemetry findings, or log excerpts. Empty string if the job is passing.
- `team` and `labels`: from the job registry (`ci-status-jobs.yaml`). Include them verbatim. If a job overrides the category-level team/labels, use the job-level values.
- If a job had a fetch error in step 2, set `pass_count` and `fail_count` to -1 and `failure_classification` to "fetch_error".

### 7. Deliver response and update thread reference

1. Call `send_response()` to deliver the summary (and threaded replies if applicable). **This ends your current turn.**
2. Before calling `send_response()`, also call `schedule_followup` with a 2-minute delay and this description:

   > Update the handoff artifact thread_ts. Call `get_current_thread_url()` to get the real thread_ts. Then use a workspace to clone the fork at `<fork_repo>`, update `.chai-bot/reports/daily_health_latest.yaml` — replace `thread_ts: 'pending'` with the actual thread_ts value. Commit and push. If the artifact doesn't exist or thread_ts is already set, call `no_action_required()`.

   Replace `<fork_repo>` in the description with the actual fork repo path from step 6.1.

3. Call `send_response()` last (this ends the turn). The scheduled follow-up will fire ~2 minutes later and update the artifact with the real thread_ts.

## Constraints

- Keep the top-level summary under 1200 characters. The message should be a scannable scoreboard, not a report. All detailed analysis goes in threaded replies.
- Never add sections, headers, or bullet lists below the category lines. The only thing after the last category line is the footer.
- If more than half the jobs return no data, warn about possible Prow/GCS issues at the top.
- Before sending: if any category is below 80%, verify your response content contains `---THREAD_DETAILS---` followed by at least one threaded reply section. If these delimiters are missing, your threaded replies will not be posted — go back to step 5.
- Always write the handoff artifact (step 6) before posting, even if all categories are green. Step 7 delivers the response and schedules the thread_ts update.

