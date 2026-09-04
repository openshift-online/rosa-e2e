# ROSA CI Job Registry Sync

## Purpose
Detect drift between active Prow periodic jobs and the job registry
(configs/ci-status-jobs.yaml). Auto-categorize new jobs where possible,
flag uncategorizable jobs for human review, and open a PR with changes.

## Discovery Patterns
Query BigQuery for all periodic jobs that ran in the last 14 days
matching ANY of these patterns:

1. periodic-ci-openshift-online-rosa-e2e-main-*
2. periodic-ci-openshift-online-rosa-gap-analysis-main-*
3. periodic-ci-openshift-release-main-nightly-*-e2e-rosa-hcp-ovn
4. periodic-ci-openshift-release-main-nightly-*-e2e-rosa-sts-ovn
5. periodic-ci-openshift-rosa-master-e2e-*
6. periodic-ci-openshift-*-rosa-sts-e2e-promotion-*
7. periodic-ci-terraform-redhat-terraform-provider-rhcs-main-e2e-rosa-*
8. periodic-ci-openshift-*-osd-gcp-e2e-promotion-*
9. periodic-ci-openshift-*-hive-e2e-promotion-*
10. periodic-ci-openshift-*-rosa-osd-aws-e2e-promotion-*

Exclude: periodic-ci-openshift-online-rosa-hyperfleet-*

## Procedure

### Step 1 — Discover active jobs
Query BigQuery for distinct periodic job names matching the patterns
above that have run in the last 14 days.

### Step 2 — Fetch current registry
Fetch `ci-status-jobs.yaml` from:
https://raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/ci-status-jobs.yaml

Extract all prow_job values from every category.

### Step 3 — Diff
Compare the two lists:
- **New jobs**: active in BigQuery but not in the registry
- **Stale jobs**: in the registry but not active in BigQuery (14+ days)

If no new or stale jobs are found, report "no drift detected" and stop.

### Step 4 — Auto-categorize new jobs
For each new job, attempt to match it to an existing category using the
category's prow_filter URL pattern or naming convention. Place the job
in the matching category with an appropriate display name derived from
the job name.

Jobs that don't match any category → flag as "uncategorized" for
human placement.

### Step 5 — Open a PR
Clone openshift-online/rosa-e2e, update ci-status-jobs.yaml with:
- New auto-categorized jobs added to their matched categories
- PR description listing:
  - Auto-placed jobs (with category)
  - Uncategorized jobs needing human placement
  - Stale jobs for potential cleanup (do NOT remove them automatically)

### Step 6 — Report
Post a summary to the channel with:
- Count of new, stale, and uncategorized jobs
- Link to the PR
- If no drift: call no_action_required (silent)
