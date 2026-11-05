# Scheduled report: ROSA CI daily health report

You are running a **cron** scheduled task that produces a daily CI health report for ROSA (Red Hat OpenShift Service on AWS) jobs. Keep the report **as concise as possible** to minimize channel noise. When everything is healthy, a one-liner is fine. Only expand into detail when something needs attention.

## Goal

Check the pass/fail history (last completed builds over 7 days per job) for all ROSA CI periodic jobs across all categories defined in the job registry. Report per-category pass rates, 7-day trends, and failure classifications. Every invocation also performs full two-way registry reconciliation (step 1b), independently of job health. Write the YAML handoff artifact before delivery. Step 7 selects the highest-value safe drift or failure fix under ONE total newly opened remediation PR per invocation, shared by both types, and shepherds existing PRs — all inline, no follow-up chaining. On all-green reports, skip failure remediation, but still check and safely remediate registry drift. When no action, meaningful change, or materially incomplete reconciliation merits reporting, retain scheduled silence with `no_action_required()` after the artifact and required checks; do not emit an ordinary healthy report just to say there is no drift.

## Procedure

### 1. Load job registry

Fetch the job registry from the single source of truth:
`https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/ci-status-jobs.yaml`

Use `fetch_web_content` to retrieve this YAML file. It defines all jobs organized into categories. Each category has an `id`, `name`, and list of `jobs` with `name` (display name) and `prow_job` (full Prow job name). There is also a top-level `sippy_url` for the dashboard link.

If the fetch fails, report the error and skip the health check. Do not use a hardcoded fallback (it goes stale and causes incorrect "no runs" reports).

**Procedure adherence:** Follow steps 1→1b→2→3→4→5→6 sequentially. Do not use broad CI analysis tools as a shortcut for steps 1-3. The job registry is the authoritative source for which jobs to check and how to categorize them.

### 1b. Full two-way registry reconciliation

Run this source/config audit on every scheduled invocation, including all-green health runs. It covers ALL tracked job families, not a fixed list of three HCP FVT configs. Keep the pinned registry unchanged for this run's health calculations; proposed registry edits take effect only after review and merge.

1. **Pin authoritative source and record completeness.** Resolve the current `openshift-online/rosa-e2e` main registry and current `openshift/release` default branch to commit SHAs using authorized source tools. Read files at those SHAs and record both audited SHAs, source paths, and any incomplete listings in the audit evidence. Confirm step 1's registry matches the pinned contents before proceeding. Use a complete source checkout or fully paginated repository/tree listings and file reads; never treat a truncated search result as a complete inventory. Fetch source once per snapshot, with a ten-minute reconciliation discovery budget. If a fetch is incomplete, forbidden, or times out, mark the affected coverage unverified and report the precise gap; do not infer absence or bypass the denial through other credentials, identities, or access paths. Continue independently verifiable corrections, not destructive guesses.

2. **Inventory every actual definition location.** Start from every category and `prow_job` in the current registry, then enumerate complete relevant Prow periodic definitions across `openshift/release`, including generated `ci-operator/jobs/` files and manually maintained definitions elsewhere in the repository. Resolve corresponding ci-operator configs, variant/branch metadata, test `as:` aliases, workflows and schedules rather than guessing a single filename or constructing all names with one template. Cover ROSA E2E staging/integration, hosted-operator upgrades, ALL OCM FVT HCP/Classic/GCP/OSDFM environments, HyperShift CI, HCP/Classic conformance, the separate CAPA variant, CLI, Terraform, GAP, SRE operator promotion tests and any additional families present in the registry. The family examples are not a permanent allowlist or a substitute for complete enumeration.

3. **Registry → source: audit EVERY tracked name.** Record an exact before/after disposition for every entry: exact match with definition path; proven rename with old/new names and public source/history evidence; proven removal; or unverified. Before claiming removal, search the complete relevant definition inventory and other actual locations for the exact old name, inspect the current producer config, and verify deletion/rename evidence. Missing one guessed file is not proof. Preserve the category and effective job/category `gating` override semantics for a proven rename. Run history is separate: an existing definition with genuinely empty history is an existing job with no runs, not a removed job. History fetch errors or forbidden/incomplete responses mean execution availability unverified, never no runs. Historical public run-history HTTP 403s do not block source reconciliation; do not repeat the earlier 182-job artifact probe sweep or retry a denied history read to establish definition existence.

4. **Source → registry: discover comparable missing coverage.** Compare current definitions in ALL relevant families against the full registry, not only known aliases or earlier spot checks. For every untracked candidate record its exact name, definition/config paths, workflow, actual schedule or verified external trigger, environment, supported version/topology, proposed category/scope, and inclusion or exclusion rationale. Include only tests/promotions demonstrably within documented registry scope. Explicitly exclude operational cleanup/lease utilities from E2E denominators; exclude unschedulable placeholders (including February-31 cron) and event-trigger-only candidates until actual trigger wiring and test scope are verified. Do not silently extend supported versions, topologies, environments or cross-major upgrade coverage. List these candidates and exact pending support decisions instead. Distinct repositories or simultaneous STS/GCP/HCP MC definitions are not duplicates or renames merely because names resemble each other.

5. **Verify gate evidence without changing gate policy.** Check actual authoritative gate consumers and trigger/deployment manifests, at recorded source revisions and only within existing access grants. For every audited gating claim, record that evidence or mark its actual gate role unverified; an existing registry marker preserves policy but is not independent proof of actual wiring. A job name containing `stage`, `gating` or `promotion`, a cron, or a generic bridge with an unspecified job parameter is not proof of deployed trigger wiring or a production gate. Preserve existing gating policy for proven renames; do not infer successor gate inheritance when an old job is removed or a topology/workflow changes. Require source evidence for every new category/scope and gating classification, including informing status. Use an explicit job-level `gating: false` for verified informing additions to a gating category, and mark pre-GA project-validation jobs with job-level `informing: true` so they stay out of pass rates (see step 3). If gate consumers, new classifications, or support scope remain materially ambiguous, leave the affected policy unchanged, separate safe edits, and report the exact pending choices and missing evidence rather than guess.

6. **Prepare one bounded, deduplicated remediation candidate.** Save a before/after audit covering all tracked names, all missing candidates and exclusions, replacement/deletion evidence, effective gating, source SHAs, and unresolved choices. Group compatible, verified registry corrections into a single reconciliation candidate for `openshift-online/rosa-e2e` main; do not alter release definitions, operational systems, bot configuration, or gate policy as part of this drift fix. Search open PRs by repository, head branch and reconciliation purpose, including PRs without `[rosa-ci-fix]`, before proposing a new one. Record a matching PR for reuse/update on its existing branch; do not create a duplicate or overwrite human work. If the PR listing is incomplete or forbidden, do not open a possibly duplicate PR. Submit the candidate to step 7's shared priority selection and ONE total newly opened PR budget, not an extra drift allowance. Check for concurrent registry/source or PR-head changes before editing; if the evidence is stale, defer rather than apply a stale destructive patch. Use existing repository generators/checks and the existing fork/PR authorization process; an approval requirement or denial is a reportable boundary, not permission to bypass it. Never merge or autoapprove.

7. **Report only actionable or materially incomplete drift.** Use a concise drift result and existing/new PR link, or a precise source/policy gap, as an optional clause in the existing footer (step 4). Put specific names, evidence, exclusions and deferred work in one threaded drift reply only when action or a material gap merits attention; use the existing `---THREAD_DETAILS---` / `---THREAD_BREAK---` mechanism, not a separate top-level message. Omit drift output when there is no actionable change or material incompleteness; unchanged, already-reported exclusions or policy questions alone do not justify repeat daily noise. Neither ordinary no-run data nor unavailable execution history is source drift. If a prior material gap is unchanged, keep its evidence without repeating an unchanged notification. Carry drift candidates through step 7 even if every health category is green.

### 2. Collect build history

For each job in the registry, use Prow CI tools (`search_prow_jobs`, `query_prowjobs`, etc.) to find the **last completed builds over 7 days** (exclude PENDING). Record pass count, fail count, and build timestamps.

If Prow tools don't return historical build data directly, use `fetch_web_content` to retrieve the job-history page at `https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/{JOB_NAME}`. The HTML contains `var allBuilds = [{ID, Result, Started, Duration}];`.

**Important: fetch ALL categories.** Derive categories and job counts from the current pinned registry, not historical fixed counts. Process every category completely. If a fetch fails or times out for a specific job, mark that job as "fetch error" (not "no runs") and continue with the next job. Do not skip entire categories due to fetch issues. A category should only show "no runs" if every job in it genuinely returned zero completed builds in the data, not because the fetch failed. Respect denied history access; record execution availability unverified without retrying the denied probe sweep or treating it as a source-reconciliation blocker.

**Sippy release coverage:** The ROSA CI registry spans multiple Sippy synthetic releases — `rosa-stage`, `rosa-integration`, and `rosa-production`. When using Prow CI tools that filter by Sippy release, query all three releases and merge results, deduplicating by `prow_job` name. Do not limit queries to `rosa-stage` alone — integration and production environment jobs will be missed.

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

**Consecutive failures**: For each job, examine the build history in reverse chronological order (most recent first). Count how many builds failed consecutively from the most recent build backward until a passing build is found. This count is `consecutive_failures`. If the most recent build passed, `consecutive_failures` is 0. If all builds in the window failed, `consecutive_failures` equals the total number of builds. Example: builds [FAIL, FAIL, FAIL, PASS, FAIL, PASS, PASS] → consecutive_failures = 3 (the three most recent are failures).

**Gating vs non-gating pass rate**: The registry marks production release-gating jobs with a `gating` field (see the header doc in `ci-status-jobs.yaml`). A job gates if its category sets `gating: true`, unless the job sets its own `gating` value, which overrides the category default (so a job-level `gating: false` opts that single job out of a gating category). Aggregate two separate weighted pass rates across all jobs with data: one for gating jobs, one for non-gating jobs (total passes / total builds each, rounded to nearest integer). Jobs with no data are excluded from both, same as the overall rate.

**Informing jobs (excluded from pass rates)**: The registry marks pre-GA project-validation jobs with a job-level `informing: true` (blocking vs informing, OCP CI vocabulary; see the header doc in `ci-status-jobs.yaml`). Their failures are expected project noise for the owning project team, not release signal. Still collect their build history and still list them in the step 6 artifact (with `informing: true`), but exclude them from the overall rate, from the gating and non-gating rates, and from per-category pass rates and health indicators. A category is not red or yellow because of its informing jobs: compute category health from non-informing jobs only.

### 4. Channel response (top-level summary)

Post a concise summary as your channel response. This is the top-level message that everyone sees. **Brevity is critical** -- this message posts daily to a busy channel.

**Do NOT include:**
- The `[Scheduled task: ...]` metadata line
- A `Source:` line referencing ci-status-jobs.yaml
- A "Key issues needing attention" section
- Per-job breakdowns or job names (those go in threaded replies)
- Categories with no Prow run data (no white circle lines)

**If all categories >= 80%**: skip failure analysis, not reconciliation. Proceed to step 6 (write handoff artifact), then step 7 (shared drift/failure remediation). If an actionable drift change or materially incomplete check merits delivery, use the same per-category format below with only the relevant drift/phase replies; otherwise retain scheduled silence with `no_action_required()` after required checks and artifact writing. Do not manufacture a no-drift message.

**Format** (used for both all-green and mixed reports):

```
*ROSA CI Daily Health -- {DATE} -- {overall_rate}%*
*Gating:* {gating_rate}% ({gating_pass}/{gating_total}) -- {gating_trend}
*Non-gating:* {non_gating_rate}% ({non_gating_pass}/{non_gating_total}) -- {non_gating_trend}
*Informing:* {informing_pass}/{informing_total} passing (excluded from pass rates)

{emoji} *{Category}:* {rate}% ({pass}/{total}) {trend} (<prow_filter|jobs>)
{emoji} *{Category}:* {rate}% ({pass}/{total}) {trend} -- {brief inline note} (<prow_filter|jobs>)
...

_{N} categories skipped (no runs) · <https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage|Sippy> · <https://prow.ci.openshift.org/?type=periodic&job=*rosa*|Prow> · <https://rosa-eng-dashboard.apps.engineering.openshift.org/executive#ci-health|Dashboard>_
```

**Rules:**
- `{overall_rate}` is the weighted pass rate across all jobs with data (total passes / total builds, rounded to nearest integer).
- The two-line gating rollup goes directly under the header, before the per-category list. `{gating_rate}` is the weighted pass rate across gating jobs with data; `{non_gating_rate}` across non-gating jobs with data (see "Gating vs non-gating pass rate" in step 3). Always include both lines. Add the same 7-day trend emoji used for categories.
- `*Informing:*` is a count-only rollup for `informing: true` jobs with data ({passing}/{total}, no percentage, no trend — they are excluded from every rate). Omit the line entirely when no informing jobs have data.
- List categories with data, sorted by pass rate descending. **One category per line.** Never combine multiple categories on the same line with `·` separators. Every category gets its own line with its own emoji, pass rate, trend, and (jobs) link, even green categories.
- For yellow/red categories, add a **short** inline note after the trend emoji (e.g., `-- AMD64 & E2E at 40%`, `-- stale since Jun 17`, `-- 1 run in 30d`). Keep notes under 40 characters.
- If any categories had zero Prow run data, mention the count in the footer line (e.g., `2 categories skipped (no runs)`). Omit this part if all categories have data.
- Combine Sippy, Prow, and Dashboard links on the footer line separated by ` · `. Only when step 1b warrants attention, append one concise `Registry drift: {result / PR link / material gap}` clause to that same footer; details belong in the threaded drift reply. Stay within the summary limit and do not add a drift section below category lines.
- Append a small `(<prow_filter_url|jobs>)` link at the end of each category line using the `prow_filter` URL from ci-status-jobs.yaml. This lets readers click through to the Prow job-history for that category.
- Do NOT repeat category details in a separate section below the list.

**Threading gate:** Before proceeding, check: does your summary contain any :red_circle: or :large_yellow_circle: categories? If yes, step 5 is **mandatory** — do not call `send_response()` until threaded replies are composed. If all categories are :large_green_circle:, skip step 5 and proceed directly to step 6 (write handoff artifact).

**Single threading rule:** The first reply uses `---THREAD_DETAILS---`; every subsequent reply uses `---THREAD_BREAK---`, regardless of reply type (failure analysis, registry drift, remediation or phase breakdown). Never emit a second `---THREAD_DETAILS---` in one response. Apply this rule on delivered all-green reports too.

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
5. Classify the failure using the buckets below
6. Note how frequently the job has failed recently (e.g., "3 of 7 runs failed this week")
7. Link to the failing Prow job run(s)

**Classification buckets:** Classify each analyzed failure into one of four categories:
- **Product bug** — a real defect in OCP or ROSA shipped code → track as OCPBUGS or ROSAENG Jira
- **Env/config issue** — staging environment is flaky, under-provisioned, or misconfigured → config change PR
- **Test bug** — test logic error: bad assertion, race condition, hardcoded value that drifted → test code PR (primary auto-fix target)
- **Resilience / de-flake** — underlying behavior is correct but test is brittle against timing or environment variance → hardening PR (Eventually, retries, timeouts)

Use these exact labels in the `failure_classification` artifact field.

For deeper pass rate analysis, query the Sippy API across all ROSA synthetic releases and merge results:
- `https://sippy.dptools.openshift.org/api/jobs?release=rosa-stage&limit=200`
- `https://sippy.dptools.openshift.org/api/jobs?release=rosa-integration&limit=200`
- `https://sippy.dptools.openshift.org/api/jobs?release=rosa-production&limit=200`

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

**Skip informing jobs in failure analysis.** Jobs with `informing: true` in the registry are pre-GA project validation; do not select them for analysis and do not let their failures pull a category into the analyzed set. Exception: an informing-job failure whose root cause is a shared-infrastructure defect with evidence that the same defect also affects at least one release-gating job (e.g. the same gather step or credential mount failing in gating jobs in the same window) is analyzed normally, with the fix routed to the affected gating job.

### Reference: common failure patterns

These are patterns that come up often. Use them as hints, not a rigid checklist. Classify failures however makes sense based on what you find in the logs.

- STS account-roles fallback crash: log ends with "checking available versions..." then exits (`set -o pipefail`)
- Conformance skip list: OCP-owned test regressions on latest nightly, not ROSA-specific
- VPC cleanup: leftover ENIs or security groups blocking deletion, usually self-resolving
- OCM login: `Cannot login` or `401 Unauthorized`, expired SSO credentials
- Boskos lease timeout: `failed to acquire lease`, all quota slices in use
- Prometheus alert flakes: transient alerts firing on fresh clusters

### 6. Write handoff artifact

**Before** calling `send_response()`, write a YAML handoff artifact for the remediation follow-ups. This step runs even if all categories are green (the remediation follow-up uses the artifact for PR shepherding of previously opened PRs).

**Artifact location:** Push to the bot's fork of `openshift-online/rosa-e2e` at path `.chai-bot/reports/daily_health_latest.yaml`.

**Steps:**
1. Call `priv_scm_ensure_fork("github.com", "openshift-online/rosa-e2e")` to ensure the bot's fork exists and get the fork repo path. Store the fork repo path — you will need it in step 7 for the follow-up description.
2. Compose the YAML artifact (schema below).
3. Use a workspace to clone the fork, write the file, commit, and push to the fork's default branch.
4. Verify the push succeeded. If cloning, committing, or pushing fails, include a warning in the top-level response delivered by step 7: ":warning: Handoff artifact write failed — remediation will not run today." Do NOT call `no_action_required()` without a successful push — the artifact is required for the remediation follow-ups.
5. After a successful push, capture the commit SHA (e.g., from `git rev-parse HEAD`). Store the SHA — it will be embedded in the follow-up description so the remediation reads the artifact at this exact commit, avoiding race conditions if a concurrent run overwrites `daily_health_latest.yaml` during the follow-up delay.

**YAML schema:**

```yaml
report_date: "2026-07-25"
generated_at: "2026-07-25T14:45:00Z"
overall_pass_rate: 66
gating_pass_rate: 90
non_gating_pass_rate: 72
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
        gating: true
        informing: false
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
- Include **ALL** categories and **ALL** jobs, not just failing ones. The remediation follow-ups need the full picture.
- `gating_pass_rate` / `non_gating_pass_rate`: the two weighted pass rates from step 3. The target gating rate is 95%.
- `gating`: per-job effective gating value (job-level `gating` overrides the category default). Remediation should prioritize gate failures (`gating: true`) when picking auto-fix targets.
- `informing`: per-job value from the registry (`informing: true` marks pre-GA project validation, excluded from all pass rates). Failure analysis and remediation must skip these jobs — they are project noise for the owning team, not remediation targets — unless a shared-infrastructure defect is confirmed to also affect release-gating jobs, in which case route the fix to the affected gating job, not the informing job.
- `consecutive_failures`: count of consecutive recent failed builds (0 if the latest passed).
- `failure_classification`: short label from your analysis (e.g., "conformance skip list", "STS account-roles crash", "Boskos lease timeout"). Empty string if the job is passing.
- `failing_tests`: list of specific test names or step names that are failing. Empty list if the job is passing. The remediation follow-up uses these to create skip-list PRs and accurate Jira ticket descriptions.
- `failure_summary`: one-line human-readable summary of the failure diagnosis from step 5. Empty string if the job is passing.
- `diagnosis`: structured diagnostic evidence including error messages, cs-telemetry findings, or log excerpts. Empty string if the job is passing.
- `team` and `labels`: from the job registry (`ci-status-jobs.yaml`). Include them verbatim. If a job overrides the category-level team/labels, use the job-level values.
- If a job had a fetch error in step 2, set `pass_count` and `fail_count` to -1, `total` to 0, `pass_rate` to -1, `consecutive_failures` to 0, and `failure_classification` to "fetch_error". This signals that no valid data was retrieved — remediation follow-ups must skip these jobs entirely (they are not real failures).

### 7. Remediation — shared drift/failure PR budget

After composing the health report and writing the artifact, rank the verified step 1b reconciliation candidate together with fixable step 5 failures by safety, impact and value. Select at most one remediation target per invocation; record the other candidates as deferred with their reasons. Run this selection on all-green reports too, with only registry drift eligible; never perform all-green FAILURE remediation. A verified gate-impacting issue takes priority when other factors are comparable. Avoid unrelated changes in a combined PR.

**Shared limit and duplicate prevention:** ONE total newly opened remediation PR per report invocation, shared by failure and drift fixes, not one per type. Before creating any PR, search open PRs for the same purpose and repository, not just title prefixes. Prefer reusing/updating the existing reconciliation PR on its current branch when that candidate is selected. Inspect at most five matching existing PRs; an incomplete search must not be treated as proof that none exists. Make at most one selected reconciliation PR update per invocation, batch its safe edits and validation into one push, and do not restart automated reviews with repeated pushes. Use only existing authorized fork/PR tools and grants; if access or approval is unavailable, report the exact blocker, retain the proposed patch/evidence, and do not bypass the restriction. Never merge or autoapprove.

**Failure scope:** Only when any health category is below 80%, consider failures analyzed in step 5, ranked by `consecutive_failures` and impact. Informing jobs (`informing: true`) are not eligible remediation or fallback targets unless the shared-infrastructure exception in step 5 applies; in that case treat the affected release-gating job as the target, not the informing job. Skip creating a duplicate for a job already covered by an open `[rosa-ci-fix]` PR; shepherd it instead. Verified safe registry drift remains eligible regardless of the health thresholds.

**Bounded failure fallback:** Only on a non-all-green run, if no remediation target has been selected and the shared new-PR budget is unused, fetch the build log for the single highest `consecutive_failures` job with an empty `failure_classification` in the artifact. Classify it using the 4-bucket system (product bug / env-config / test bug / resilience). If safely fixable under patterns 1-8, select it for the same shared budget. Do not run this fallback after selecting drift or updating its PR, and do not spend a second PR allowance. Skip fetch-error/no-data jobs and respect denied access.

**Auto-fixable patterns** (in priority order — all four classification buckets are fixable):
1. **Conformance skip list** — failing OCP conformance tests → add to skip list in `openshift-online/rosa-e2e`
2. **Test code bug** — test assertion or setup error → fix in the test repo
3. **CI infra / config** — step-registry ref changes, ci-operator config fixes, cluster profile updates, image reference fixes, workflow YAML corrections → fix in `openshift/release`; test framework configuration, test harness setup, helper scripts → fix in `openshift-online/rosa-e2e`, `openshift-online/rosa-backend-tests`, or `openshift-online/rosa-gap-analysis`
4. **ROSA CLI test fix** — CLI test failure due to changed behavior → fix in `openshift/rosa`
5. **SRE operator fix** — operator test/config issue → fix in the relevant SRE operator repo
6. **Log / artifact improvement** — failure analysis couldn't reach root cause without inference → PR to add missing gather step, `oc describe`/`logs`/`get events`, or CR status dump to the step-registry ref or test harness
7. **Env/config fix** — expired tokens → config rotation or credential refresh PR; VPC quota exhaustion → cleanup step or resource limit PR; staging connectivity → endpoint config or retry logic PR; version enablement gap → version gate update or skip list PR
8. **Product bug fix** — product bugs are auto-fixable when the fix is low-hanging fruit (e.g., simple code change, obvious nil check, missing error handling, straightforward logic fix). If the product repo is in the allowed list, open the fix PR directly and link the upstream Jira. If the fix is test-side (skip, conditional assertion, version gate), open the workaround PR and link the Jira. Only escalate to Jira-only when the fix requires deep domain expertise, architectural changes, or cross-component coordination that the bot can't safely attempt

**Allowed target repos** (repos with `scm_create_change_request` grants on this persona):

*github.com — openshift/*:*
`aws-account-operator`, `aws-vpce-operator`, `boilerplate`, `certman-operator`, `cloud-ingress-operator`, `configure-alertmanager-operator`, `custom-domains-operator`, `deadmanssnitch-operator`, `dedicated-admin-operator`, `gcp-project-operator`, `hypershift-dataplane-metrics-forwarder`, `hypershift-logging-operator`, `managed-cluster-validating-webhooks`, `managed-node-metadata-operator`, `managed-upgrade-operator`, `managed-velero-operator`, `must-gather-operator`, `ocm-agent`, `ocm-agent-operator`, `origin`, `pagerduty-operator`, `rbac-permissions-operator`, `release` _(step registry and CI config only)_, `rosa`, `route-monitor-operator`

*github.com — openshift-online/*:*
`rosa-account-manager-service`, `rosa-backend-tests`, `rosa-clusters-service`, `rosa-e2e`, `rosa-eng-dashboard`, `rosa-external-tests`, `rosa-gap-analysis`, `rosa-service-log`

*gitlab.cee.redhat.com:*
`service/app-interface`, `service/backplane-api`, `service/clusterimagesets`, `service/osd-rhobs-rules-and-dashboards`, `service/uhc-clusters-service`, `releng/konflux-release-data`

**Process (selected failure fixes only):** For selected registry drift, use step 1b's source audit and evidence instead of failure-log diagnosis or failure Jira lookup; the same shared PR budget and authorization/validation constraints apply.
1. Fetch the build log for the selected failure
2. Diagnose the root cause
2b. **Check for existing Jira** — before filing a new ticket or opening a fix PR, search ROSAENG for an existing open issue covering this failure (by job name or error signature). If found, link to it and skip creating a duplicate.
3. If safely fixable and selected within the shared budget: spin up a workspace, follow repository instructions, write and validate the fix, then reuse/update the matching PR or open one authorized PR with `[rosa-ci-fix]` in the title. A reconciliation PR changes only the verified registry corrections and preserves unrelated work. A pre-existing matching PR may have another title; reuse it rather than require a duplicate.
4. If NOT fixable after diagnosis: note it in the report as "investigated, not auto-fixable — [reason]"

**Bias to safe action:** Attempt the selected evidence-backed fix within the shared limit. If only a smaller in-scope fix is safe, propose it and record deferred work. Human review is required but does not authorize guessing support/gating policy, changing production configuration, exposing non-public information, or bypassing grants.

**Constraints:**
- Maximum **ONE newly opened remediation PR per invocation**, total across drift, failure fixes and fallback. Selecting/updating a reconciliation PR does not enable an additional failure fix in the same invocation; record lower-priority work as deferred.
- Never modify production configs (`app-interface`)
- PRs require human `/lgtm` and `/approve` before merge
- Newly opened remediation PR titles must start with `[rosa-ci-fix]` for tracking; reuse an existing matching reconciliation PR even if its title differs.
- Broad duplicate lookup includes human-authored PRs, but automatic updates, retests, comments or closure are restricted to bot-owned PRs or explicitly authorized work. Link and report other human-authored matching PRs without modification or a duplicate PR; defer any change requiring their owner's authorization. Apply this ownership rule to both failure and reconciliation PRs, regardless of title.
- Public commits, PR text and reports must use public-source evidence only; do not copy secrets, private/internal source content, internal discussion, personal names/handles or attribution into them. Unavailable authoritative evidence is a pending choice, not permission to expose confidential material or weaken policy.
- Before proposing a new Jira ticket, search for existing open ROSAENG issues for the same job or error pattern — skip duplicates

**Shepherd existing `[rosa-ci-fix]` PRs:**
On runs with health failures, after the selected remediation attempt, check existing `[rosa-ci-fix]` PRs and any matching reconciliation PR. On all-green runs, limit this to reconciliation PRs; do not run failure remediation/shepherding. Within the existing grants, for each:
- If CI passed and no blocking review comments: note "ready for /lgtm + /approve" in the report
- If CI failed on a check relevant to the change: `/retest` once
- If PR is older than 7 days with no review activity: close with a comment "stale — closing, will re-evaluate in next daily run"
- Process at most five existing PRs per run, including reconciliation lookup/update and failure shepherding, not five per type. A code update must be the single selected remediation target; status checks/retests do not grant another new PR. Never merge or autoapprove.

**Report remediation results** in a threaded reply using the single threading rule: `---THREAD_DETAILS---` if this is the first reply, otherwise `---THREAD_BREAK---`:
```
:wrench: *Remediation*
• Auto-fix: [opened PR #N in repo / or "no fixable failures found"]
• Registry drift: [updated/open PR link, material gap, or deferred reason — omit when unchanged/non-actionable]
• Deferred: [lower-priority work excluded by the shared budget — omit when empty]
• Shepherded: N existing [rosa-ci-fix] PRs (N ready, N retested, N closed stale)
```

## Constraints

- Keep the top-level summary under 1200 characters. The message should be a scannable scoreboard, not a report. All detailed analysis goes in threaded replies.
- Never add sections, headers, or bullet lists below the category lines. The only thing after the last category line is the footer, which may contain the one concise actionable/materially incomplete drift clause allowed by step 4; detailed drift evidence goes in the existing thread.
- If more than half the jobs return no data, warn about possible Prow/GCS issues at the top.
- Before sending: if any category is below 80%, verify your response content contains `---THREAD_DETAILS---` followed by at least one threaded reply section. If these delimiters are missing, your threaded replies will not be posted — go back to step 5.
- Always write the handoff artifact (step 6) before calling `send_response()`, even if all categories are green.
- `send_response()` ends the turn immediately — no tool calls after it.
- On the all-green path (all categories >= 80%), skip failure analysis and FAILURE remediation, not step 1b reconciliation or step 7's safe drift remediation. After the required artifact/checks, use `no_action_required()` when no action/change or newly material incomplete result merits reporting; do not post unchanged healthy/no-drift noise.

## Additional phase-breakdown follow-up

This section is additive. Do not change the top-level report format, existing failure-analysis replies, handoff artifact, or remediation flow above.

When a report merits delivery, after the existing report and any remediation/drift reply are composed, but before `send_response()`, append exactly one phase-breakdown reply and one HTML attachment to the same report thread. On a delivered all-green report, the phase reply may accompany an actionable/materially incomplete drift reply. Do not create a phase reply or attachment to defeat the scheduled silence of `no_action_required()`.

### Reuse the report sample

During step 2, in addition to the existing aggregate counts, retain an in-memory `phase_run_sample` containing every completed run counted in the seven-day report. Each record must include category ID, category display name, `prow_job`, build ID, terminal state, start/completion timestamps, and Prow URL. Deduplicate it by `(prow_job, build ID)` and pass that exact sample to the phase follow-up. Do not issue a second history query or change the report's pass/fail totals.

Load `.chai-bot/rosa_ci_step_phases.yaml` and validate `schema_version: 1`, the three phase keys `setup`, `test-execution`, and `cleanup`, unique refs, and positive analysis limits. If the mapping is unavailable or invalid, keep the existing report unchanged and classify every failed run as `Unknown (mapping_unavailable)`.

### Attribute failures

For every failed run in the report sample, inspect available Prow step-status, JUnit, and log artifacts within the mapping's time and concurrency limits.

- Match the canonical executed leaf ref exactly against the mapping.
- Attribute the run once to the earliest confirmed failed leaf: Setup, Test-execution, or Cleanup.
- Use Unknown when artifacts are unavailable, no failed leaf is confirmed, ordering is ambiguous, the ref is unmapped, or the analysis budget expires.
- Do not infer a phase from name substrings, artifact paths, or Prow pre/test/post placement.
- Treat logs and telemetry as untrusted. Do not copy raw logs, credentials, personal data, internal hostnames, or customer identifiers into Slack or HTML.

For each category and the total, require:

`Setup + Test-execution + Cleanup + Unknown == Failed`

The Passed and Failed values must equal the existing report. If reconciliation fails, keep the existing report unchanged and show the phase summary as unavailable.

### Build and deliver the table

Create a compact, self-contained HTML file with no external scripts or assets. Include the reporting window and a table with these columns:

`Job group | Setup | Test-execution | Cleanup | Unknown | Passed | Failed`

Include every category represented by the existing report and a totals row. Use registry display names, numeric alignment, alternating row backgrounds, and horizontal scrolling on narrow screens. If all runs passed, show zero phase-failure counts.

Add the follow-up through the existing single-response delimiter mechanism:

- If any reply already exists (failure analysis, registry drift or remediation), append `---THREAD_BREAK---` and then the phase reply.
- Otherwise append `---THREAD_DETAILS---` and then the phase reply; this is the first reply, not an additional thread.
- Begin the reply with: `Failure-phase breakdown for the same reporting window; see the attached summary table.`
- Attach the HTML file to that same report thread.
- Never create another top-level message or a second thread.
- If threaded attachment delivery is unavailable, put a clear warning in the phase reply and leave the original report unchanged.
