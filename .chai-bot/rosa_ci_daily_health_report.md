# Scheduled report: ROSA CI daily health report

You are running a **cron** scheduled task that produces a daily CI health report for ROSA (Red Hat OpenShift Service on AWS) jobs. Keep the report **as concise as possible** to minimize channel noise. The choice on each run is between staying silent and posting the required summary — never between the required summary and an abbreviated one. When a report is delivered, the header line plus the `*Gating:*` and `*Non-gating:*` rollup lines are the floor it may shrink to (see step 4); brevity drops optional detail, never those lines. Brevity also never costs a category its figures: every category with figures keeps its own row in the top-level summary, and brevity is paid for out of failure detail, which lives in threaded replies (see step 4 and step 5). Only expand into detail when something needs attention. When nothing merits a report at all, retain scheduled silence (see step 7 and Constraints) rather than shrinking the report below that floor.

## Goal

Check the pass/fail history (last completed builds over 7 days per job) for all ROSA CI periodic jobs across all categories defined in the job registry. Report per-category pass rates, 7-day trends, and failure classifications. Every invocation also performs full two-way registry reconciliation (step 1b), independently of job health. Write the YAML handoff artifact before delivery. Step 7 selects the highest-value safe drift or failure fix under ONE total newly opened remediation PR per invocation, shared by both types, and shepherds existing PRs — all inline, no follow-up chaining. On all-green reports, skip failure remediation, but still check and safely remediate registry drift. When no action, meaningful change, or materially incomplete reconciliation merits reporting, retain scheduled silence with `no_action_required()` after the artifact and required checks; do not emit an ordinary healthy report just to say there is no drift.

## Procedure

### 1. Load job registry

Fetch the job registry (generated from the lean source plus the org team snapshot):
`https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/generated/ci-status-jobs.yaml`

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

For each job in the registry, use Prow CI tools (`search_prow_jobs`, `query_prowjobs`, etc.) to find the **last completed builds over 7 days** (exclude PENDING). Record pass count, fail count, and build timestamps. Retrieve enough history to cover the **previous** 7-day window as well (14 days in total, paging the history source as needed): step 3's trend needs both windows. Record, per job and per window, whether the retrieval was exhaustive — see "Window completeness" in step 3.

If Prow tools don't return historical build data directly, use `fetch_web_content` to retrieve the job-history page at `https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/{JOB_NAME}`. The HTML contains `var allBuilds = [{ID, Result, Started, Duration}];`.

**Important: fetch ALL categories.** Derive categories and job counts from the current pinned registry, not historical fixed counts. Process every category completely. If a fetch fails or times out for a specific job, mark that job as "fetch error" (not "no runs") and continue with the next job. Do not skip entire categories due to fetch issues. A category should only show "no runs" if every job in it genuinely returned zero completed builds in the data, not because the fetch failed. Respect denied history access; record execution availability unverified without retrying the denied probe sweep or treating it as a source-reconciliation blocker.

**Sippy release coverage:** The ROSA CI registry spans multiple Sippy synthetic releases — `rosa-stage`, `rosa-integration`, and `rosa-production`. When using Prow CI tools that filter by Sippy release, query all three releases and merge results, deduplicating by `prow_job` name. Do not limit queries to `rosa-stage` alone — integration and production environment jobs will be missed.

### 3. Compute pass rates and trends

**Rate population**: the set of jobs a line's rate, counts and trend are measured over. For the overall, gating and non-gating rollups it is the non-informing jobs alone, always. For a category it is the category's non-informing jobs — except an **informing-only** category, which has none, and whose population is therefore its informing jobs, yielding figures that are informational and never aggregated (see "Informing figures" below). Every availability rule below is judged over the line's own rate population.

**Per-category pass rate**: aggregate pass/fail across the category's rate population.

**Health indicators** (group green first, then yellow, then red, then informing-only):
- :large_green_circle: pass rate >= 80%
- :large_yellow_circle: pass rate >= 40% and < 80%
- :red_circle: pass rate < 40%
- :white_circle: informing-only, or no data

The thresholds are unchanged: they are measured over the category's non-informing jobs, and an informing-only category is never assigned a green, yellow or red indicator however its informational rate reads.

**7-day trend**: compare pass rate for builds in the last 7 days vs the previous 7 days:
- :chart_with_upwards_trend: improving (10+ percentage points higher)
- :chart_with_downwards_trend: degrading (10+ percentage points lower)
- :left_right_arrow: stable

**Window completeness**: a job's window (current or previous) is **complete** when the retrieval demonstrably reached past its start — the oldest returned build is older than the window start, pagination ran out, or the source explicitly bounds its coverage. A quiet job with zero builds over an exhausted source therefore has a complete window; returning fewer or older builds than 14 days is not by itself incomplete. A window is **incomplete** when the response was capped, truncated, denied or errored with none of those proofs. Record completeness per job and per window.

**Trend availability**: emit a trend emoji only when **both** windows are complete for **every** registry job in the line's rate population, **and** each window holds at least one completed build — two rates are needed to compare. Otherwise emit the literal text `trend unavailable` where the emoji would go, and leave the rest of the line unchanged. Never substitute :left_right_arrow: (or any other emoji) for an unmeasured trend: "stable" is a measured result.

**Rate availability**: a rate and its counts need a complete **current** window for every registry job in the line's rate population. Print, by what is actually known:
- complete current window with at least one completed build → the percentage and its `({pass}/{total})` counts;
- complete current window with no completed build anywhere in the population → `N/A (0/0)` — never `0%`, which asserts measured and all-failing;
- incomplete current window → the figures cover the retrieved subset only and the run is flagged as partial coverage (step 4). Print the qualified form `{rate}% ({pass}/{total}, incomplete)` when that subset has builds, and `N/A (data unavailable)` when it does not. Never print `0/0` for an incomplete window: zero builds *retrieved* is not zero builds *run*.

The header carries no counts, so when the overall current window is incomplete it is the footer's partial-data clause that qualifies it. Trend and rate availability are decided separately: an incomplete previous window degrades only the trend field. All four forms — `trend unavailable`, `N/A (0/0)`, `N/A (data unavailable)` and the `, incomplete` qualifier — apply wherever a trend or a rate is printed, in the summary and in the threaded replies alike.

**Consecutive failures**: For each job, examine the build history in reverse chronological order (most recent first). Count how many builds failed consecutively from the most recent build backward until a passing build is found. This count is `consecutive_failures`. If the most recent build passed, `consecutive_failures` is 0. If all builds in the window failed, `consecutive_failures` equals the total number of builds. Example: builds [FAIL, FAIL, FAIL, PASS, FAIL, PASS, PASS] → consecutive_failures = 3 (the three most recent are failures).

**Gating vs non-gating pass rate**: The registry marks production release-gating jobs with a `gating` field (see the header doc in `ci-status-jobs.yaml`). A job gates if its category sets `gating: true`, unless the job sets its own `gating` value, which overrides the category default (so a job-level `gating: false` opts that single job out of a gating category). Aggregate two separate weighted pass rates across all **non-informing** jobs with data: one for gating jobs, one for non-gating jobs (total passes / total builds each, rounded to nearest integer). An informing job never enters either rate, whatever its category's `gating` value and whether or not it carries a job-level `gating` override. Jobs with no data are excluded from both, same as the overall rate; see "Rate availability" above for when that exclusion must be qualified as incomplete rather than reported as a confirmed count.

**Informing jobs (excluded from pass rates)**: The registry marks pre-GA project-validation jobs with a job-level `informing: true` (blocking vs informing, OCP CI vocabulary; see the header doc in `ci-status-jobs.yaml`). Their failures are expected project noise for the owning project team, not release signal. Still collect their build history and still list them in the step 6 artifact (with `informing: true`), but exclude them from the overall rate, from the gating and non-gating rates, and from per-category pass rates and health indicators. A category is not red or yellow because of its informing jobs: compute category health from non-informing jobs only. Exclude them from **both** comparison windows of every trend as well (current and previous), so no trend measures a population different from the rate it annotates.

**Informing figures (informational only, never aggregated)**: informing jobs are excluded from every aggregate above, and that exclusion is absolute — nothing in this subsection feeds the overall rate, the gating rate or the non-gating rate. Two display-only figures are computed from them, in addition to the count-only `*Informing:*` rollup:

- **Mixed category** (at least one non-informing job and at least one informing job): its rate, counts, trend and health indicator stay exactly as computed above, over its non-informing jobs only. Alongside them, compute the count-only tally `informing-only {passing}/{total}` over the category's informing jobs with data. The tally is a separate figure printed beside the rate; it is never folded into the rate, the counts, the trend or the health indicator.
- **Informing-only category** (no non-informing job at all): there is no release-signal population to measure, so the category takes the `:white_circle:` indicator and the `informing-only` marker. Compute its rate, `({pass}/{total})` counts and trend over its informing jobs, under the same window-completeness, rate-availability and trend-availability rules used everywhere else, and print them as **informational** figures. They describe that category alone and are excluded from the overall, gating and non-gating rollups exactly as every other informing figure is.

Informing figures never change which categories are selected for failure analysis in step 5.

### 4. Channel response (top-level summary)

Post a concise summary as your channel response. This is the top-level message that everyone sees. **Brevity is critical** -- this message posts daily to a busy channel.

**Do NOT include:**
- The `[Scheduled task: ...]` metadata line
- A `Source:` line referencing ci-status-jobs.yaml
- A "Key issues needing attention" section
- Per-job breakdowns or job names (those go in threaded replies)
- Categories with no Prow run data at all — they appear in the complete category breakdown reply instead, marked `no runs`, `unavailable` or `partial` per that section's state markers. This exclusion is about absent data, not about health or length: a category that has figures of any kind — a release-signal rate, or the informational figures of an `informing-only` category — belongs in the summary. Only a category whose every job returned nothing is left to the thread

**If no category is :large_yellow_circle: or :red_circle:** (equivalently, every category with a release-signal rate is >= 80%): skip failure analysis, not reconciliation. An informing-only category never makes this test fail: its informational rate is not release-signal health, so it is :white_circle: whatever that rate reads. Proceed to step 6 (write handoff artifact), then step 7 (shared drift/failure remediation). If an actionable drift change or materially incomplete check merits delivery, use the same per-category format below with only the relevant drift, complete-category-breakdown and phase replies; otherwise retain scheduled silence with `no_action_required()` after required checks and artifact writing. Do not manufacture a no-drift message.

**Format** (used for both all-green and mixed reports):

```
*ROSA CI Daily Health -- {DATE} -- {overall_rate}%*
*Gating:* {gating_rate}% ({gating_pass}/{gating_total}) -- {gating_trend}
*Non-gating:* {non_gating_rate}% ({non_gating_pass}/{non_gating_total}) -- {non_gating_trend}
*Informing:* {informing_pass}/{informing_total} passing (excluded from pass rates)

:large_green_circle: *{Category}:* {rate}% ({pass}/{total}) {trend} (<prow_filter|jobs>)
:large_green_circle: *{Category}:* {rate}% ({pass}/{total}) {trend} · informing-only {ipass}/{itotal} (<prow_filter|jobs>)
:large_yellow_circle: *{Category}:* {rate}% ({pass}/{total}) {trend} -- details withheld (<prow_filter|jobs>)
:red_circle: *{Category}:* {rate}% ({pass}/{total}) {trend} -- details withheld (<prow_filter|jobs>)
:white_circle: *{Category}:* {rate}% ({pass}/{total}) {trend} -- informing-only (<prow_filter|jobs>)
...

_{N} categories skipped (no runs) · full breakdown in thread ({total} categories) · <https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage|Sippy> · <https://prow.ci.openshift.org/?type=periodic&job=*rosa*|Prow> · <https://rosa-eng-dashboard.apps.engineering.openshift.org/executive#ci-health|Dashboard>_
```

Every footer clause other than the three links is conditional — the ones before them and the `Registry drift:` clause after them alike: emit each when its rule below says to, drop it otherwise. The three links are always present.

**Rules:**
- **Mandatory headline block — never omit.** Every delivered report opens with exactly these three lines, in this order and adjacent: the `*ROSA CI Daily Health -- {DATE} -- {overall_rate}%*` header, then `*Gating:*`, then `*Non-gating:*`. Emit all three on every delivered report — all-green, mixed, drift-only, zero-denominator and trend-unavailable runs alike. Their presence never depends on trend availability, on how many categories have data, or on how healthy the run is. When a field cannot be filled, degrade the **field** (`N/A (0/0)`, `N/A (data unavailable)`, `trend unavailable`) — never the line. The only alternative to this block is scheduled silence, not a shorter report. Exactly one optional `:warning:` line may precede the header — the Prow/GCS no-data warning (Constraints) and the step 6 artifact-failure warning share that single line when both apply. Nothing else may appear above the header.
- `{overall_rate}` is the weighted pass rate across all **non-informing** jobs with data (total passes / total builds, rounded to nearest integer); informing jobs are excluded from it exactly as they are from the two rollups. Its `N/A (0/0)` and `N/A (data unavailable)` forms replace the whole `{overall_rate}%` token including the percent sign — never render `N/A (0/0)%`.
- The two-line gating rollup goes directly under the header, before the per-category list. `{gating_rate}` is the weighted pass rate across gating jobs with data; `{non_gating_rate}` across non-gating jobs with data (see "Gating vs non-gating pass rate" in step 3). Each line keeps its rate and its `({pass}/{total})` counts, qualified `, incomplete` when its current window is; an `N/A` form replaces that whole span, percent sign included, rather than only the percentage.
- `{gating_trend}` / `{non_gating_trend}` use the same 7-day trend emoji as categories, or the literal `trend unavailable` when either of that population's windows is incomplete (see "Trend availability" in step 3). The same applies to the per-category `{trend}` field.
- **Partial coverage.** If the current window of any registry job that belongs to some line's rate population is incomplete in step 2 — fetch error, denial, truncation, or a cap with no proof of exhaustion — the printed rates cover only the retrieved subset: append a short `partial data ({N} jobs unavailable)` clause to the footer line, separated by ` · `, where N counts the jobs whose current window is incomplete. This is distinct from the no-run category count, and distinct from the more-than-half-missing warning in Constraints. Canonical footer clause order: skipped-category count · partial data · complete-breakdown pointer · Sippy · Prow · Dashboard · `Registry drift:`.
- **Complete-breakdown pointer.** Whenever the summary's category lines do not carry exactly one line per configured category of the pinned registry — which happens only when a category has no run data at all and is therefore kept out of the summary — append `full breakdown in thread ({total} categories)` to the footer in its canonical position, where `{total}` is the number of categories in the pinned registry, and compose the complete category breakdown reply (see "Additional complete category breakdown"). This clause replaces the former `+{N} more categories` clause: a count of what is missing is not a route to it. Omit the clause only when the inline list already is the complete view.
- `*Informing:*` is a count-only rollup for `informing: true` jobs with data ({passing}/{total}, no percentage, no trend — they are excluded from every rate). Omit the line entirely when no informing jobs have data.
- **The complete category list stays in the summary.** Every category with figures gets its own line in the top-level message, with its own emoji, rate, counts, trend field and `(jobs)` link — green categories included. The list is never moved into the thread, never truncated to the categories that fit, and never reduced to a count: the summary carries every one of these lines or the report is not delivered at all.
- **Group order: green, then yellow, then red, then informing-only.** Order the lines by health indicator in that sequence, and within each group by pass rate descending. The emoji sequence *is* the grouping: emit no group heading, no blank line between groups and no other separator. **One category per line.** Never combine multiple categories on the same line with `·` separators.
- For yellow and red categories, append the exact suffix `-- details withheld` after the trend field. This is a fixed literal, not a summary: never put an error message, a failing job or test name, a date, a count or any other failure analysis on a summary line. The investigation belongs in the step 5 threaded replies.
- For a **mixed** category, print the non-informing rate, counts and trend unchanged, then append the count-only tally `· informing-only {ipass}/{itotal}` (see "Informing figures" in step 3) — after the trend field, after the `-- details withheld` suffix when the line carries one, and immediately before the `(jobs)` link. It is a separate figure beside the rate, never folded into it.
- The final `:white_circle:` group holds every category with figures but no release-signal rate. An **informing-only** category is the usual case: print its informational rate, counts and trend (step 3) with the `:white_circle:` emoji and append the exact suffix `-- informing-only` after the trend field. A **mixed** category whose non-informing jobs returned no data also lands here — it is not informing-only, so it takes no suffix: print the `N/A` rate form step 3 gives it, followed by its informing tally. Inside the group, rated rows sort by rate descending and `N/A` rows follow them. Those figures describe that category alone; they are excluded from the header, `*Gating:*` and `*Non-gating:*` rollups like every other informing figure.
- If any categories had zero Prow run data over a complete window, mention the count in the footer line (e.g., `2 categories skipped (no runs)`). A category whose current window is merely incomplete is not "no runs": it counts toward partial data instead. Omit this part if all categories have data.
- Combine Sippy, Prow, and Dashboard links on the footer line separated by ` · `. Only when step 1b warrants attention, append one concise `Registry drift: {result / PR link / material gap}` clause to that same footer; details belong in the threaded drift reply. Keep it to that one clause and do not add a drift section below category lines.
- Append a small `(<prow_filter_url|jobs>)` link at the end of each category line using the `prow_filter` URL from ci-status-jobs.yaml. This lets readers click through to the Prow job-history for that category.
- Do NOT repeat category details in a separate section below the list.

**Pre-delivery structural checklist (summary):** run this immediately before composing the response element. It applies only to a *delivered* report; a run that qualifies for scheduled silence (step 7 / Constraints) skips it, because silence is not a report.

1. Line 1 is the `*ROSA CI Daily Health -- {DATE} -- ...*` header, carrying the date and the overall current-period rate (or its `N/A` form), optionally preceded by the single `:warning:` line.
2. Line 2 is `*Gating:*` and line 3 is `*Non-gating:*`, in that order, directly under the header, with no other line between them.
3. Each of those two lines carries a rate with its `({pass}/{total})` counts — qualified `, incomplete` for an incomplete current window — or `N/A (0/0)` / `N/A (data unavailable)`, plus either a trend emoji or the literal `trend unavailable`.
4. `*Informing:*` is present immediately after `*Non-gating:*` if and only if at least one informing job has data; it is count-only, with no percentage and no trend field.
5. No rollup percentage — header, `*Gating:*` or `*Non-gating:*` — includes informing jobs, no mixed category's rate or trend includes them, and every such trend excludes them from both comparison windows. The informational figures on an `informing-only` line are the one place informing builds are measured, and they reach no rollup.
6. No `0%` stands in for a zero denominator, no `0/0` stands in for an incomplete window, and no trend emoji is printed unless both windows are complete.
7. Every category with figures has its own line with its own emoji, rate, counts, trend field and `(jobs)` link; the footer is the last line.
8. The category lines are grouped green, then yellow, then red, then informing-only, with pass rate descending inside each group, and no group heading or blank line between groups.
9. Every yellow and red line ends its trend field with the exact suffix `-- details withheld` and carries no other failure detail; every informing-only line carries the `-- informing-only` suffix; and every mixed category whose informing jobs have data carries its `· informing-only {ipass}/{itotal}` tally. A mixed category whose informing jobs returned nothing carries no tally — never a `0/0` one.
10. Either the inline list carries exactly one line per configured category of the pinned registry, or the only categories missing from it have no run data at all and the footer carries the complete-breakdown pointer with the complete category breakdown reply composed. No configured category is absent from both the summary and the thread.

Never satisfy a failing item by deleting a required line. **The category list is never traded against length.** There is no overflow rule: a long summary is delivered long. The per-category list is not dropped, not truncated to the lines that fit, not thinned by pass rate, health emoji, trend or any other severity ranking, and not relocated into the thread — which categories a reader sees never depends on how healthy they are or on how many there are. The headline block is never dropped, shortened, merged onto one line, or traded against length either. When a delivered summary runs long, shorten the footer by dropping its optional clauses in this order — the `Registry drift:` clause first (compose the threaded drift reply if it is not already present, so the drift result is still delivered), then the skipped-category count — and deliver it at whatever length the complete list requires rather than dropping a required line. Never drop the complete-breakdown pointer: without it the thread that holds the no-run categories is unreachable.

A registry of many categories, or long `prow_filter` URLs, makes for a long summary. That is the accepted cost of keeping the whole scoreboard in one message: the length is paid in the summary, and the savings come from step 5 keeping every investigation in the thread.

**Scenario examples** (illustrative values only, from a smaller registry than the live one). Except where a footer is shown, these are headline fragments: the category lines and the footer are omitted for brevity, and a delivered report always ends with the footer. A footer-bearing example may still elide category lines with a `...` line.

*Trends available, informing jobs with data:*

```
*ROSA CI Daily Health -- 2026-07-25 -- 85%*
*Gating:* 94% (47/50) -- :left_right_arrow:
*Non-gating:* 71% (22/31) -- :chart_with_downwards_trend:
*Informing:* 3/9 passing (excluded from pass rates)
```

*Comparison history capped, truncated or missing:*

```
*ROSA CI Daily Health -- 2026-07-25 -- 85%*
*Gating:* 94% (47/50) -- trend unavailable
*Non-gating:* 71% (22/31) -- trend unavailable
```

Here the current window is complete for every job and only the previous window was capped, so the rates and counts stand and only the trend field degrades. The overall rate is the two rollups combined: (47+22)/(50+31) = 69/81 = 85%.

*No informing job has data:* omit only the `*Informing:*` line. The header and both rollup lines are still emitted.

*Zero denominator in one population:*

```
*ROSA CI Daily Health -- 2026-07-25 -- 71%*
*Gating:* N/A (0/0) -- trend unavailable
*Non-gating:* 71% (22/31) -- :left_right_arrow:
```

The gating population's current window is complete and genuinely held no completed build, so `N/A (0/0)` is honest and `0%` would not be; its trend is unavailable by definition. The overall rate is the non-gating population alone: 22/31 = 71%.

*Partial current-period data (incomplete current history for some jobs):*

```
*ROSA CI Daily Health -- 2026-07-25 -- 85%*
*Gating:* 94% (47/50) -- :left_right_arrow:
*Non-gating:* 71% (22/31, incomplete) -- trend unavailable

:large_green_circle: *ROSA HCP E2E:* 92% (23/25) :left_right_arrow: (<prow_filter|jobs>)
...

_1 category skipped (no runs) · partial data (4 jobs unavailable) · full breakdown in thread (19 categories) · <https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage|Sippy> · <https://prow.ci.openshift.org/?type=periodic&job=*rosa*|Prow> · <https://rosa-eng-dashboard.apps.engineering.openshift.org/executive#ci-health|Dashboard>_
```

An incomplete current window reduces coverage, not the pass rate: the unretrieved jobs are excluded from the denominators and never counted as failures. Availability is decided per line: here the four unretrieved jobs are all non-gating and none belongs to the listed category, so the gating rollup and that category keep complete windows, while the non-gating rollup is qualified `, incomplete` and loses its trend. The header carries no counts, so the footer's partial-data clause is what qualifies the 85%. The remaining category lines are elided here as `...`. The skipped category is not among them, so the footer also points at the complete category breakdown reply, where every configured category appears one row each.

*Current history unavailable for the whole population:*

```
*ROSA CI Daily Health -- 2026-07-25 -- N/A (data unavailable)*
*Gating:* N/A (data unavailable) -- trend unavailable
*Non-gating:* N/A (data unavailable) -- trend unavailable
```

Nothing was retrieved, so nothing is known. `N/A (data unavailable)` says exactly that, where `N/A (0/0)` would falsely assert that every job ran nothing. Reserve `N/A (0/0)` for a current window that is complete for the whole population and genuinely held no completed build.

*Complete grouped category list (green, yellow, red, informing-only):*

```
*ROSA CI Daily Health -- 2026-07-25 -- 85%*
*Gating:* 94% (47/50) -- :left_right_arrow:
*Non-gating:* 71% (22/31) -- :chart_with_downwards_trend:
*Informing:* 6/12 passing (excluded from pass rates)

:large_green_circle: *ROSA HCP E2E:* 100% (20/20) :left_right_arrow: (<prow_filter|jobs>)
:large_green_circle: *ROSA Classic E2E:* 95% (21/22) :left_right_arrow: · informing-only 2/4 (<prow_filter|jobs>)
:large_green_circle: *OCM FVT HCP:* 93% (14/15) :chart_with_upwards_trend: (<prow_filter|jobs>)
:large_yellow_circle: *HyperShift CI:* 79% (11/14) :chart_with_downwards_trend: -- details withheld (<prow_filter|jobs>)
:red_circle: *ROSA CLI:* 30% (3/10) :chart_with_downwards_trend: -- details withheld (<prow_filter|jobs>)
:white_circle: *Seaplane Regional:* 50% (3/6) :left_right_arrow: -- informing-only (<prow_filter|jobs>)
:white_circle: *OSDFM Health:* N/A (0/0) trend unavailable · informing-only 1/2 (<prow_filter|jobs>)

_<https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage|Sippy> · <https://prow.ci.openshift.org/?type=periodic&job=*rosa*|Prow> · <https://rosa-eng-dashboard.apps.engineering.openshift.org/executive#ci-health|Dashboard>_
```

Every configured category of this (small) registry has a line, so the footer carries neither a skipped-category count nor the complete-breakdown pointer: the inline list already is the complete view. The groups run green, yellow, red, informing-only with no heading between them, and pass rate descends inside the green group (100%, 95%, 93%). The yellow and red lines carry the fixed `-- details withheld` suffix: which jobs failed and why is in their step 5 threaded replies, not here.

Informing jobs stay out of every rate. *ROSA Classic E2E* is mixed: its 95% (21/22) is its non-informing jobs alone, and its four informing jobs appear only as the `· informing-only 2/4` tally. *Seaplane Regional* is informing-only, so its 50% (3/6) is informational — it is the final group, and neither its passes nor its builds are in the 94%, the 71% or the 85%.

*OSDFM Health* is the third form the final group holds: a **mixed** category whose non-informing jobs have a complete window that held no completed build, and whose informing jobs do have data. It is not informing-only, so it takes no suffix; its release-signal rate is `N/A (0/0)` and its informing figures stay in the tally beside it. A zero denominator is a measured zero, so the row contributes `0/0` to the release-signal sums rather than being left out of them — unlike `N/A (data unavailable)`, where the totals are unknown and nothing may be assumed.

The three sets of informing jobs are what the `*Informing:*` rollup totals as 6/12: 2/4 from the mixed green line, 3/6 from the informing-only line, and 1/2 from the rate-less mixed line. The release-signal lines reconcile against the rollups instead: 20+21+14+11+3+0 = 69 passes over 20+22+15+14+10+0 = 81 builds, which is (47+22)/(50+31) = 85%.

*All categories green and the report is delivered:* the same three-line headline block is emitted, followed by every green category line and the footer. Healthy is a reason to skip failure analysis, not a reason to skip the headline block or any category line.

**Threading gate:** Before proceeding, check: did this run **compute** any :red_circle: or :large_yellow_circle: category? If yes, step 5 is **mandatory** — do not call `send_response()` until threaded replies are composed. If every computed category is :large_green_circle:, or no category had data at all, skip step 5 and proceed directly to step 6 (write handoff artifact). The complete category breakdown reply is not failure analysis: it never satisfies this gate, never substitutes for a step 5 reply, and is never a reason to enter step 5.

**Single threading rule:** The first reply uses `---THREAD_DETAILS---`; every subsequent reply uses `---THREAD_BREAK---`, regardless of reply type (failure analysis, registry drift, remediation, complete category breakdown or phase breakdown). Never emit a second `---THREAD_DETAILS---` in one response. Apply this rule on delivered all-green reports too.

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

**Scope cap:** Analyze the 5 worst :large_yellow_circle:/:red_circle: categories first (lowest pass rate). Informing-only categories are never in this ranking, whatever their informational rate. For each, analyze at most 5 failing jobs (worst pass rate first). This keeps the report actionable without requiring excessive tool calls. If more categories are failing, note the count in the last failure-analysis reply (e.g., "2 additional categories below 80% — see Prow dashboard for details").

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
job_registry_url: "https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/generated/ci-status-jobs.yaml"
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
        components:
          - "rosa-cli"
```

**Field notes:**
- Include **ALL** categories and **ALL** jobs, not just failing ones. The remediation follow-ups need the full picture.
- `gating_pass_rate` / `non_gating_pass_rate`: the two weighted pass rates from step 3.
- `gating`: per-job effective gating value (job-level `gating` overrides the category default). Remediation should prioritize gate failures (`gating: true`) when picking auto-fix targets.
- `informing`: per-job value from the registry (`informing: true` marks pre-GA project validation, excluded from all pass rates). Failure analysis and remediation must skip these jobs — they are project noise for the owning team, not remediation targets — unless a shared-infrastructure defect is confirmed to also affect release-gating jobs, in which case route the fix to the affected gating job, not the informing job.
- `trend`: `improving`, `degrading`, `stable`, or `unavailable` when either comparison window was incomplete (step 3). Never record `stable` for a trend that was not measured. This is a value convention on the existing field, not a schema change.
- `pass_rate` (top-level, category and job): integer percentage, or `null` when a complete current window held no completed build. For an **informing-only** category record `pass_rate: null` and `health: "informing-only"`: its informational summary figure is not release-signal health, and recording it here would feed it to step 7's ranking and thresholds, which read this artifact. Never record `0` for a zero denominator — `0` asserts measured and all-failing. The existing `-1` / `fetch_error` convention below covers **any** incomplete current window, not only a fetch error, so a figure from a partly retrieved window is never recorded as a confirmed rate. No new artifact fields are introduced.
- `consecutive_failures`: count of consecutive recent failed builds (0 if the latest passed).
- `failure_classification`: short label from your analysis (e.g., "conformance skip list", "STS account-roles crash", "Boskos lease timeout"). Empty string if the job is passing.
- `failing_tests`: list of specific test names or step names that are failing. Empty list if the job is passing. The remediation follow-up uses these to create skip-list PRs and accurate Jira ticket descriptions.
- `failure_summary`: one-line human-readable summary of the failure diagnosis from step 5. Empty string if the job is passing.
- `diagnosis`: structured diagnostic evidence including error messages, cs-telemetry findings, or log excerpts. Empty string if the job is passing.
- `team` and `labels`: from the job registry (`configs/generated/ci-status-jobs.yaml`, generated from the lean source plus the org snapshot). Include them verbatim. If a job overrides the category-level team/labels, use the job-level values.
- `components`: from the job registry, verbatim (empty list for platform/upstream-scope categories). The Jira remediation follow-up uses it to set the Jira component on filed tickets.
- If a job had a fetch error in step 2, set `pass_count` and `fail_count` to -1, `total` to 0, `pass_rate` to -1, `consecutive_failures` to 0, and `failure_classification` to "fetch_error". This signals that no valid data was retrieved — remediation follow-ups must skip these jobs entirely (they are not real failures).

### 7. Remediation — shared drift/failure PR budget

After composing the health report and writing the artifact, rank the verified step 1b reconciliation candidate together with fixable step 5 failures by safety, impact and value. Select at most one remediation target per invocation; record the other candidates as deferred with their reasons. Run this selection on all-green reports too, with only registry drift eligible; never perform all-green FAILURE remediation. A verified gate-impacting issue takes priority when other factors are comparable. Avoid unrelated changes in a combined PR.

**Shared limit and duplicate prevention:** ONE total newly opened remediation PR per report invocation, shared by failure and drift fixes, not one per type. Before creating any PR, search open PRs for the same purpose and repository, not just title prefixes. Prefer reusing/updating the existing reconciliation PR on its current branch when that candidate is selected. Inspect at most five matching existing PRs; an incomplete search must not be treated as proof that none exists. Make at most one selected reconciliation PR update per invocation, batch its safe edits and validation into one push, and do not restart automated reviews with repeated pushes. Use only existing authorized fork/PR tools and grants; if access or approval is unavailable, report the exact blocker, retain the proposed patch/evidence, and do not bypass the restriction. Never merge or autoapprove.

**Failure scope:** Only when some category is :large_yellow_circle: or :red_circle: — an informing-only category never qualifies — consider failures analyzed in step 5, ranked by `consecutive_failures` and impact. Informing jobs (`informing: true`) are not eligible remediation or fallback targets unless the shared-infrastructure exception in step 5 applies; in that case treat the affected release-gating job as the target, not the informing job. Skip creating a duplicate for a job already covered by an open `[rosa-ci-fix]` PR; shepherd it instead. Verified safe registry drift remains eligible regardless of the health thresholds.

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
- After opening a remediation PR, immediately narrow its follow-up scope with `github_unwatch_pr` (`scope="addressed"`), and narrow bot-owned `[rosa-ci-fix]` PRs you shepherd the same way. CI results, pushes, and merge-state changes must not start background-update turns in this thread — reports you compose as part of a scheduled run are unaffected. Shepherding happens on each daily run. Anyone can restore full follow-up on a PR by asking chai to watch it (`github_adopt_pr` with `mode="full"`).
- Newly opened remediation PR titles must start with `[rosa-ci-fix]` for tracking; reuse an existing matching reconciliation PR even if its title differs.
- Broad duplicate lookup includes human-authored PRs, but automatic updates, retests, comments or closure are restricted to bot-owned PRs or explicitly authorized work. Link and report other human-authored matching PRs without modification or a duplicate PR; defer any change requiring their owner's authorization. Apply this ownership rule to both failure and reconciliation PRs, regardless of title.
- Public commits, PR text and reports must use public-source evidence only; do not copy secrets, private/internal source content, internal discussion, personal names/handles or attribution into them. Unavailable authoritative evidence is a pending choice, not permission to expose confidential material or weaken policy.
- Before proposing a new Jira ticket, search for existing open ROSAENG issues for the same job or error pattern — skip duplicates

**Shepherd existing `[rosa-ci-fix]` PRs:**
On runs with health failures, after the selected remediation attempt, check existing `[rosa-ci-fix]` PRs and any matching reconciliation PR. On all-green runs, do not run failure remediation, but still check the CI and review state of open bot-owned `[rosa-ci-fix]` PRs — with their follow-up scope narrowed, this daily check is what surfaces blocked fixes. Within the existing grants, for each:
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

- Keep the top-level summary to the headline block, one line per category with figures, and the footer. The message should be a scannable scoreboard, not a report: its length is set by how many categories the registry configures, not by how much there is to say about them. All detailed analysis goes in threaded replies. Nothing may cost the summary a category line — brevity comes from what each line omits, never from how many lines are emitted.
- Every delivered summary contains the mandatory headline block: the header line plus `*Gating:*` and `*Non-gating:*` directly beneath it. Brevity, an all-green run, a zero denominator and missing comparison history are all reasons to shorten *fields*, never reasons to omit those lines. Run the pre-delivery structural checklist in step 4 before composing the response.
- A delivered report never leaves a configured category out of both the summary and the thread. Every category with figures has its own summary line; the only categories the summary may omit are those with no run data at all, and the complete category breakdown reply then carries every configured category with the footer pointing at it. A category line is never dropped, truncated away or selected against by health, rate, severity or length.
- Never add sections, headers, or bullet lists below the category lines. The only thing after the last category line is the footer, which may contain the one concise actionable/materially incomplete drift clause allowed by step 4; detailed drift evidence goes in the existing thread.
- If more than half the jobs return no data, warn about possible Prow/GCS issues at the top.
- Before sending: if any category is :large_yellow_circle: or :red_circle: (an informing-only :white_circle: category does not count, however its informational rate reads), verify your response content contains `---THREAD_DETAILS---` followed by at least one **failure-analysis** reply section. The complete-category-breakdown, drift, remediation and phase replies do not satisfy this check. If these delimiters are missing, your threaded replies will not be posted — go back to step 5.
- Always write the handoff artifact (step 6) before calling `send_response()`, even if all categories are green.
- `send_response()` ends the turn immediately — no tool calls after it.
- On the all-green path (no :large_yellow_circle: or :red_circle: category), skip failure analysis and FAILURE remediation, not step 1b reconciliation or step 7's safe drift remediation. After the required artifact/checks, use `no_action_required()` when no action/change or newly material incomplete result merits reporting; do not post unchanged healthy/no-drift noise.

## Additional complete category breakdown

This section is additive. It changes no pass rate, trend, window-completeness or gating definition, no top-level format rule, no failure-analysis gate, no handoff artifact field, and no remediation behaviour. It only guarantees that a delivered report shows every configured category somewhere, including the ones the summary omits because they have no run data at all.

### When it is required

On a delivered report, compose this reply whenever the summary's category lines do not already carry **exactly one line per configured category** of the pinned registry. Since the summary keeps every category that has figures, that is the case only when a category has no run data at all and step 4 keeps it out of the summary. When every configured category already has its own summary line, the inline list *is* the complete view: emit no reply and no footer pointer.

A run that qualifies for scheduled silence has no report and therefore no breakdown. Never compose this reply, or its footer pointer, to defeat the scheduled silence of `no_action_required()`.

The reply lives in the thread, so completeness costs the channel nothing: the top-level summary keeps its scoreboard shape.

### Rows

Build the rows from the same run and the same reporting window as the summary, out of the step 2 history and the step 3 figures already computed. Do not issue a second history query, re-derive a rate by another route, or print a figure that differs from the summary's.

Emit one row per configured category in the pinned registry, in registry order, carrying:

- the category display name from the registry and its step 3 health emoji — `:white_circle:` for any category with no release-signal rate, which covers an `informing-only` category as well as both the `N/A (0/0)` and the `N/A (data unavailable)` forms;
- the pass rate with its `({pass}/{total})` counts, in exactly the forms step 3 defines and nowhere else: `{rate}% ({pass}/{total})`, `{rate}% ({pass}/{total}, incomplete)`, `N/A (0/0)` or `N/A (data unavailable)`;
- the trend emoji, or the literal `trend unavailable` when step 3's trend availability is not met;
- the category's `(<prow_filter|jobs>)` link when the registry supplies a `prow_filter`, omitted when it does not;
- an explicit state marker that keeps the kinds of missing data apart. Every marker is judged over the category's **non-informing** jobs only, the same population as its rate, so a marker can never contradict the rate beside it. The one category whose rate is drawn from another population is an `informing-only` one, which has no non-informing job to judge and whose marker says so outright. The marker describes the row's *coverage*; the rate field, unchanged from step 3, reports its *figures*. They are not in one-to-one correspondence: different markers can accompany the same rate form, one marker can accompany different rate forms, and the marker never changes which form step 3 prints. At most one marker per row, first match wins in this order:
  1. `informing-only` — the category has no non-informing job at all, so it has no release-signal rate; the row instead carries the informational rate, counts and trend step 3 computes over its informing jobs;
  2. `unavailable` — no non-informing window is complete **and** no completed build was retrieved, so nothing about the category is known;
  3. `partial` — at least one non-informing window is incomplete, and either completed builds were retrieved or coverage is mixed (some windows complete, some not). A qualified measured rate `{rate}% ({pass}/{total}, incomplete)` is a `partial` row, including when every window is incomplete: step 3 measures whatever subset came back, and the marker says the coverage behind it is partial. Mixed coverage that returned no completed build is a `partial` row too, and it prints the same `N/A (data unavailable)` rate form as an `unavailable` row — step 3 gives that form to every incomplete current window whose retrieved subset is empty, whether or not some window of the population was complete. Keep the rate exactly as step 3 computes it: the marker never downgrades a measured rate to an `N/A` form, and never upgrades an `N/A` form to a measured one;
  4. `no runs` — every non-informing window is complete and none held a completed build;
  5. no marker at all — every non-informing window is complete and a rate is measured.

  A quiet job whose source was exhausted has a complete window (step 3), so a quiet category is `no runs`, never `unavailable`. Never show an incomplete window as `no runs`, and never omit the marker from a row whose rate carries the `, incomplete` qualifier. An `informing-only` row keeps that marker even when its informational rate is qualified `, incomplete`: the qualifier on the rate is what reports the partial coverage behind it, so the two appear together rather than the row losing its marker;
- the count-only tally `informing-only {passing}/{total}` when a **mixed** category has informing jobs with data, omitted when it has none and omitted on an `informing-only` row, whose own rate already reports that population. Informing jobs stay out of a mixed row's rate, its counts and its trend exactly as step 3 requires; the tally is a separate figure beside them and is never folded into the rate.

Row format:

```
{emoji} *{Category}:* {rate}% ({pass}/{total}) {trend} [-- no runs | -- partial | -- unavailable | -- informing-only] [· informing-only {p}/{t}] (<prow_filter|jobs>)
```

An `N/A (0/0)` or `N/A (data unavailable)` form, and the `, incomplete` qualifier, replace the whole `{rate}% ({pass}/{total})` span exactly as they do in the summary — never only the percentage.

An `informing-only` category has no release-signal population, so its rate, counts and trend are the informational ones step 3 computes over its informing jobs — the same figures its summary line shows. The marker is what tells a reader that they measure project validation rather than release signal, and that none of them reaches the overall, gating or non-gating rollups.

### Coverage and reconciliation check

Before composing the reply, check it against the pinned registry and the figures already computed:

1. every configured category `id` appears exactly once — no missing category, no duplicate row. Match on `id`, which the registry pairs with the display name the row prints;
2. the row count equals the number of categories in the pinned registry, and the row set matches the categories written to the step 6 artifact;
3. the rows' non-informing `{pass}` and `{total}` sum to the combined `*Gating:*` and `*Non-gating:*` counts, which is the overall rate's population. An `informing-only` row contributes nothing here: its rate is drawn from informing jobs and belongs to check 4 instead;
4. the rows' mixed-category `informing-only` tallies, plus the counts on every `informing-only` row, sum to the `*Informing:*` rollup;
5. a category that also has a summary line shows the same rate, counts, trend and emoji in both places.

These checks reconcile the existing step 3 definitions; they introduce no new calculation. A category excluded from the rates — nothing retrieved, no completed build, or informing jobs only — contributes zero to check 3 and states why in its row marker; its informing jobs still contribute to check 4. A rollup printed in an `N/A` form carries no counts and so contributes nothing to check 3: reconcile against whichever rollup does carry counts, and skip check 3 entirely when neither does. If a check fails, do not drop, merge or invent a row to make it pass: deliver the rows as computed, prepend `:warning: coverage check failed: {what did not reconcile}` above the reply's opening sentence, and leave the summary and every figure unchanged.

### Delivery

Compose this reply after any failure-analysis, registry-drift and remediation replies and before the phase-breakdown reply, in the same single `set_response_element` call, under step 4's single threading rule: `---THREAD_DETAILS---` if it is the first reply of the response, otherwise `---THREAD_BREAK---`. Never create another top-level message or a second thread.

Begin the reply with: `Complete category breakdown for the same reporting window; one row per configured category.` — below the `:warning: coverage check failed:` line when the coverage check prepended one.

If the rows do not fit a single reply, continue them across further replies in the same thread, each separated by `---THREAD_BREAK---` and each labelled `Complete category breakdown ({n} of {replies})`, splitting only between whole rows. The opening sentence above goes on the first reply only; the continuations carry their label and their rows. The existing in-thread attachment mechanism (the phase follow-up's self-contained HTML file is the supported example) may carry the same rows in addition, never instead of the labelled replies, and never as a reason to leave a row out. If attachment delivery is unavailable, the labelled replies are the delivery and nothing is lost.

## Additional phase-breakdown follow-up

This section is additive. Do not change the top-level report format, existing failure-analysis replies, handoff artifact, or remediation flow above.

When a report merits delivery, after the existing report and any remediation/drift or complete-category-breakdown reply are composed, but before `send_response()`, append exactly one phase-breakdown reply and one HTML attachment to the same report thread. On a delivered all-green report, the phase reply may accompany an actionable/materially incomplete drift reply. Do not create a phase reply or attachment to defeat the scheduled silence of `no_action_required()`.

### Reuse the report sample

During step 2, in addition to the existing aggregate counts, retain an in-memory `phase_run_sample` containing every completed **non-informing** run counted in the seven-day report. Informing runs are out of scope for this follow-up, so an informing-only category contributes no rows and does not appear in the table; this is what lets the per-category and totals reconciliation below match the report's own counts. Each record must include category ID, category display name, `prow_job`, build ID, terminal state, start/completion timestamps, and Prow URL. Deduplicate it by `(prow_job, build ID)` and pass that exact sample to the phase follow-up. Do not issue a second history query or change the report's pass/fail totals.

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

- If any reply already exists (failure analysis, registry drift, remediation or complete category breakdown), append `---THREAD_BREAK---` and then the phase reply.
- Otherwise append `---THREAD_DETAILS---` and then the phase reply; this is the first reply, not an additional thread.
- Begin the reply with: `Failure-phase breakdown for the same reporting window; see the attached summary table.`
- Attach the HTML file to that same report thread.
- Never create another top-level message or a second thread.
- If threaded attachment delivery is unavailable, put a clear warning in the phase reply and leave the original report unchanged.
