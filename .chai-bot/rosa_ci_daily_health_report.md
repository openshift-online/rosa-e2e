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

For each category below 80%, post a **separate threaded reply** to the top-level message with a deep investigation. One reply per failing category.

For each failing job in the category:
1. Fetch the build log from the most recent failure using Prow CI tools or `fetch_web_content` on the artifacts URL
2. Identify the specific failure: key error messages, failing test names, failing step
3. Perform root cause analysis using Sippy, Prow CI tools, or other available tools
4. Classify the failure based on what you find in the logs
5. Note how frequently the job has failed recently (e.g., "3 of last 10 runs failed")
6. Link to the failing Prow job run(s)

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
