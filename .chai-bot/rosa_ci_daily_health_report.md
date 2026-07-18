# Scheduled report: ROSA CI daily health report

You are running a **cron** scheduled task that produces a daily CI health report for ROSA (Red Hat OpenShift Service on AWS) jobs. Keep the report **as concise as possible** to minimize channel noise. When everything is healthy, a one-liner is fine. Only expand into detail when something needs attention.

## Goal

Check the pass/fail history (last 10 completed builds per job) for 44 ROSA CI periodic jobs across 8 categories. Report per-category pass rates, 7-day trends, and failure classifications. If all categories are >= 80%, respond with a brief summary and `no_action_required()`.

## Procedure

### 1. Load job registry

Fetch the job registry from the single source of truth:
`https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/ci-status-jobs.yaml`

Use `fetch_web_content` to retrieve this YAML file. It defines all jobs organized into categories. Each category has an `id`, `name`, and list of `jobs` with `name` (display name) and `prow_job` (full Prow job name). There is also a top-level `sippy_url` for the dashboard link.

If the fetch fails, fall back to the Job Registry at the bottom of this document.

### 2. Collect build history

For each job in the registry, use Prow CI tools (`search_prow_jobs`, `query_prowjobs`, etc.) to find the **last 10 completed builds** (exclude PENDING). Record pass count, fail count, and build timestamps.

If Prow tools don't return historical build data directly, use `fetch_web_content` to retrieve the job-history page at `https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/{JOB_NAME}`. The HTML contains `var allBuilds = [{ID, Result, Started, Duration}];`.

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
Then call `no_action_required()`.

**If any category < 80%**: use this format:

```
*ROSA CI Daily Health -- {DATE}*

{emoji} *{Category}:* {rate}% ({pass}/{total}) {trend}
{emoji} *{Category}:* {rate}% ({pass}/{total}) {trend} -- {brief inline note}
...

_{N} categories skipped (no runs) · <https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage|Sippy> · <https://prow.ci.openshift.org/?type=periodic&job=*rosa*|Prow>_
```

**Rules:**
- List categories with data, sorted by pass rate descending.
- For yellow/red categories, add a **short** inline note after the trend emoji (e.g., `-- AMD64 & E2E at 40%`, `-- stale since Jun 17`, `-- 1 run in 30d`). Keep notes under 40 characters.
- If any categories had zero Prow run data, mention the count in the footer line (e.g., `2 categories skipped (no runs)`). Omit this part if all categories have data.
- Combine Sippy and Prow links on the footer line separated by ` · `.
- Do NOT repeat category details in a separate section below the list.

### 5. Failure analysis (threaded replies)

After posting the top-level summary, use `post_thread_update` to post **separate threaded replies** for each category below 80%. One reply per failing category. Each call to `post_thread_update` creates a new message in the thread under your top-level summary.

For each failing job in the category:
1. Fetch the build log from the most recent failure using Prow CI tools or `fetch_web_content` on the artifacts URL
2. Identify the specific failure: key error messages, failing test names, failing step
3. For OCM FVT jobs, also check the `cs-telemetry` logs in the Prow artifacts. These contain Clusters Service-side request/response data that can reveal CS errors, timeouts, or API failures that caused the test to fail. Look in the artifacts directory for files matching `cs-telemetry*` or `cs_telemetry*`.
4. Perform root cause analysis using Sippy, Prow CI tools, or other available tools
5. Classify the failure based on what you find in the logs
6. Note how frequently the job has failed recently (e.g., "3 of last 10 runs failed")
7. Link to the failing Prow job run(s)

For deeper pass rate analysis, query the Sippy API:
`https://sippy.dptools.openshift.org/api/jobs?release=rosa-stage&limit=100`

Format each threaded reply like:

```
{emoji} *{Category} -- {rate}% pass rate* {trend}

*{Job Name}* -- {pass}/10 (<job-history link>)
{Short summary of failure: key error, failing test/step}
Failing since {date}. {Root cause analysis.}

*{Job Name}* -- {pass}/10 (<job-history link>)
{Short summary and analysis}
```

### Reference: common failure patterns

These are patterns that come up often. Use them as hints, not a rigid checklist. Classify failures however makes sense based on what you find in the logs.

- STS account-roles fallback crash: log ends with "checking available versions..." then exits (`set -o pipefail`)
- Conformance skip list: OCP-owned test regressions on latest nightly, not ROSA-specific
- VPC cleanup: leftover ENIs or security groups blocking deletion, usually self-resolving
- OCM login: `Cannot login` or `401 Unauthorized`, expired SSO credentials
- Boskos lease timeout: `failed to acquire lease`, all quota slices in use
- Prometheus alert flakes: transient alerts firing on fresh clusters

### 6. Auto-fix (for pattern-matched failures)

After completing the failure analysis, check if any failures match fixable patterns. Use `post_thread_update` to post the results as another threaded reply.

**Conformance skip list pattern:**
If a conformance test (HCP or Classic STS) is failing persistently (3+ consecutive failures) and the failing test is in an OCP-owned sig (sig-apps, sig-auth, sig-network, sig-storage), AND the same test is NOT failing in rosa-e2e HCP/STS jobs (confirming it's upstream, not ROSA-specific):

1. Search for existing open PRs in `openshift/release` with `[ci-fix]` in the title targeting the same test. If found, skip and note the existing PR.
2. Clone `openshift/release` via workspace tools
3. Add the test name to the `TEST_SKIPS` env var in the appropriate workflow YAML:
   - HCP: `ci-operator/step-registry/rosa/aws/hcp/conformance/rosa-aws-hcp-conformance-workflow.yaml`
   - Classic STS: `ci-operator/step-registry/rosa/aws/sts/conformance/rosa-aws-sts-conformance-workflow.yaml`
4. Run `make jobs` to regenerate Prow job configs
5. Scan the diff for sensitive content (credentials, IP addresses, account IDs) before pushing
6. Open a PR with title `[ci-fix] Skip <test-name> in <workflow> (upstream OCP regression)`
7. PR description must link to the failing Prow job run(s) and reference the upstream OCP bug if identifiable
8. Use `post_thread_update` to post a threaded reply with the PR link

**Stale PR cleanup:**
Before creating new PRs, check for any open `[ci-fix]` PRs older than 7 days. Auto-close them with a comment explaining they were not reviewed in time.

**Constraints:**
- Maximum 3 auto-fix PRs per scheduled run
- Allowed repos for fixes: `openshift/release` (step registry, workflow YAMLs), `openshift-online/rosa-e2e` (test code), `service/ocm-backend-tests` (FVT test code on GitLab), `openshift/origin` (conformance test fixes)
- Never modify production configs or operator code
- PRs require human `/lgtm` and `/approve` before merge (no auto-merge)

### 7. Jira ticket creation (for non-fixable failures)

For persistent failures (3+ consecutive) where the auto-fix step did not open a PR (the failure requires deeper investigation or a fix outside the allowed repos), create a Jira ticket so the owning team can investigate.

Before creating a ticket, search Jira for existing open issues that already cover the same failure (search by job name or test name in ROSAENG and SREP projects). If found, skip and note the existing ticket.

**Team and label classification:**

The `ci-status-jobs.yaml` config includes `team` and `labels` fields per category (and optionally per job). Use these directly:
- `team.id` maps to the Jira Team field (`customfield_10001`)
- `team.name` is for display only
- `labels` is the list of Jira labels to apply
- Job-level `team` and `labels` override category-level when present

If a category or job has no `team` field, fall back to ROSA CI (`97412673-7d28-430b-bdee-ce3d1eb702b2`) with label `ci-failure`.

For OCM FVT failures, also check cs-telemetry to determine if the failure is CS-side (API errors, timeouts) vs test-side (assertion errors, framework issues). If test-side, use ROSA CI team instead of the category's team.

**Ticket format:**
- Type: Bug
- Summary: `[ci-failure] <Job display name>: <brief failure description>`
- Priority: Major (persistent) or Minor (intermittent)
- Parent epic: ROSAENG-391
- Labels: from the `labels` field in ci-status-jobs.yaml
- Description: include the diagnosis from the threaded reply, links to failing Prow runs, and any cs-telemetry findings
- Security Level: Red Hat Employee (id: 10034)

**Constraints:**
- Maximum 2 Jira tickets per scheduled run
- Only create tickets for persistent failures (3+ consecutive), not intermittent flakes
- Always search for existing open tickets first to avoid duplicates

Use `post_thread_update` to post a threaded reply noting the created ticket with a link.

## Constraints

- Keep the top-level summary under 1200 characters. The message should be a scannable scoreboard, not a report. All detailed analysis goes in threaded replies.
- Never add sections, headers, or bullet lists below the category lines. The only thing after the last category line is the footer.
- If more than half the jobs return no data, warn about possible Prow/GCS issues at the top.

## Job Registry (fallback)

> Only used if the live fetch from `https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/ci-status-jobs.yaml` fails. This list may be stale.

### ROSA E2E (9 jobs)

| Name | Prow Job |
|---|---|
| HCP 4.19 | periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-stable-4-19 |
| HCP 4.20 | periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-stable-4-20 |
| HCP 4.21 | periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-stable-4-21 |
| HCP 4.22 | periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-candidate-4-22 |
| HCP 5.0 | periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-nightly-5-0 |
| STS 4.19 | periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-classic-sts-e2e-stable-4-19 |
| STS 4.20 | periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-classic-sts-e2e-stable-4-20 |
| STS 4.21 | periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-classic-sts-e2e-stable-4-21 |
| STS 4.22 | periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-classic-sts-e2e-candidate-4-22 |

### OSD GCP E2E (1 job)

| Name | Prow Job |
|---|---|
| OSD GCP 4.22 | periodic-ci-openshift-online-rosa-e2e-main-periodics-osd-gcp-e2e-candidate-4-22 |

### OCM FVT HCP (11 jobs)

| Name | Prow Job |
|---|---|
| HCP AD | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-rosa-hcp-ad-staging-main |
| HCP Adobe | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-rosa-hcp-adobe-staging-main |
| HCP AMD64 | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-rosa-hcp-amd64-staging-main |
| HCP ARM | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-rosa-hcp-arm-staging-main |
| HCP Autonode | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-rosa-hcp-autonode-staging-main |
| HCP E2E | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-hcp-e2e-staging-main |
| HCP PL | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-rosa-hcp-pl-staging-main |
| HCP Shared VPC | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-rosa-hcp-shared-vpc-staging-main |
| HCP Y-Upgrade | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-rosa-hcp-y-upgrade-staging-main |
| HCP Zero Egress | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-rosa-hcp-zero-egress-staging-main |
| HCP Zero Egress Upgrade | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-staging-ocm-fvt-periodic-cs-rosa-hcp-zero-egress-upgrade-staging-main |

### OCM FVT HCP Integration (1 job)

| Name | Prow Job |
|---|---|
| HCP Backup Restore | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-hcp-integration-ocm-fvt-periodic-cs-rosa-hcp-backup-restore-integration-main |

### OCM FVT Classic (9 jobs)

| Name | Prow Job |
|---|---|
| ROSA AD | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-classic-staging-ocm-fvt-periodic-cs-rosa-ad-staging-main |
| STS AD (stg) | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-classic-staging-ocm-fvt-periodic-cs-rosa-sts-ad-staging-main |
| STS AD (int) | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-classic-integration-ocm-fvt-periodic-cs-rosa-sts-ad-integration-main |
| STS PL | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-classic-staging-ocm-fvt-periodic-cs-rosa-sts-pl-staging-main |
| STS Shared VPC | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-classic-staging-ocm-fvt-periodic-cs-rosa-sts-shared-vpc-staging-main |
| STS Upgrade | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-classic-staging-ocm-fvt-periodic-cs-rosa-sts-upgrade-staging-main |
| HCP Upgrade | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-classic-staging-ocm-fvt-periodic-cs-rosa-hcp-upgrade-staging-main |
| OCM Resources | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-classic-staging-ocm-fvt-periodic-cs-ocm-resources-staging-main |
| OSD RH AWS | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-rosa-classic-staging-ocm-fvt-periodic-cs-osd-rh-aws-staging-main |

### OCM FVT GCP (3 jobs)

| Name | Prow Job |
|---|---|
| GCP CCS AD | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-osd-gcp-staging-ocm-fvt-periodic-cs-osd-ccs-gcp-ad-staging-main |
| GCP Marketplace | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-osd-gcp-staging-ocm-fvt-periodic-cs-osd-ccs-gcp-marketplace-staging-main |
| GCP Non-Cross-Proj WIF | periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-osd-gcp-staging-ocm-fvt-periodic-cs-osd-gcp-non-cross-proj-wif-staging-main |

### HCP Conformance (6 jobs)

| Name | Prow Job |
|---|---|
| HCP 4.19 | periodic-ci-openshift-release-main-nightly-4.19-e2e-rosa-hcp-ovn |
| HCP 4.20 | periodic-ci-openshift-release-main-nightly-4.20-e2e-rosa-hcp-ovn |
| HCP 4.21 | periodic-ci-openshift-release-main-nightly-4.21-e2e-rosa-hcp-ovn |
| HCP 4.22 | periodic-ci-openshift-release-main-nightly-4.22-e2e-rosa-hcp-ovn |
| HCP 4.23 | periodic-ci-openshift-release-main-nightly-4.23-e2e-rosa-hcp-ovn |
| HCP 5.0 | periodic-ci-openshift-release-main-nightly-5.0-e2e-rosa-hcp-ovn |

### Classic STS Conformance (4 jobs)

| Name | Prow Job |
|---|---|
| STS 4.19 | periodic-ci-openshift-release-main-nightly-4.19-e2e-rosa-sts-ovn |
| STS 4.20 | periodic-ci-openshift-release-main-nightly-4.20-e2e-rosa-sts-ovn |
| STS 4.21 | periodic-ci-openshift-release-main-nightly-4.21-e2e-rosa-sts-ovn |
| STS 4.22 | periodic-ci-openshift-release-main-nightly-4.22-e2e-rosa-sts-ovn |
