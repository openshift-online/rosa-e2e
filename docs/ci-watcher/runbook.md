# CI Watcher Runbook

## Jobs Under Surveillance

All configured periodic jobs can be found in [CI Status jobs](https://github.com/openshift-online/rosa-e2e/blob/main/configs/ci-status-jobs.yaml). The jobs are organized into the following categories:

| Category | Description | Jobs |
|----------|-------------|------|
| ROSA E2E | Managed service validation (HCP, Classic STS, OSD GCP, Upgrade) | [Prow Periodic](https://prow.ci.openshift.org/?type=periodic&job=periodic-ci-openshift-online-rosa-e2e-main-periodics*), [Prow Upgrade](https://prow.ci.openshift.org/?type=periodic&job=periodic-ci-openshift-online-rosa-e2e-main-upgrade*) |
| OCM FVT | Clusters-service API contract tests (HCP, Classic, OSD GCP across staging and integration) | [Prow location](https://prow.ci.openshift.org/?type=periodic&job=periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt*) |
| OCP Conformance | openshift-tests run against ROSA HCP and Classic STS clusters on each OCP nightly | [Prow location](https://prow.ci.openshift.org/?job=periodic-ci-openshift-release-main-nightly*rosa*) |

## Daily Triage Procedure

### Step 1: Review Daily Status Report

An automated [CI status report](../../scripts/ci-status-report.sh) is posted daily to `#wg-rosa-ci-enhancement`. Use this report as your starting point to track job health across all monitored jobs. The report reads the canonical [job list](../../configs/ci-status-jobs.yaml) and queries Prow GCS for the latest results.

Review the report for:
- Jobs that have turned red since the last report
- Persistent failures that need Jira stories or escalation
- Jobs with no recent data that may need investigation

### Step 2: Run /ci-triage

```
/ci-triage
```

This spawns a team of 4 background agents:

| Agent | Role |
|-------|------|
| status-checker | Polls all tracked jobs, reports state changes |
| log-analyzer | Deep-dives into specific failures on demand |
| fix-proposer | Creates fix PRs for test bugs and config drift |
| jira-implementer | Creates Jira stories, shepherds open PRs |

Wait for agent reports to complete before proceeding.

### Step 3: Review Against Sippy

Open the [Sippy rosa-stage dashboard](https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage) and cross-reference with `/ci-triage` findings. Look for:
- Jobs showing declining pass rates over multiple days
- New failures that appeared overnight
- Jobs with no recent data (may indicate a config or infra problem)

### Step 4: Investigate Unclassified Failures

For any failures `/ci-triage` could not classify:
1. Navigate to the job in [Prow](https://prow.ci.openshift.org/)
2. Pull the build logs from GCS: `storage.googleapis.com/test-platform-results/logs/<job-name>/<build-id>/`
3. Key files to check: `finished.json`, `build-log.txt`, step artifacts
4. Classify the failure using the [classification matrix](escalation-paths.md#classification-matrix)

### Step 5: Take Action

- Review and approve Jira stories proposed by `/ci-triage`
- Review and merge fix PRs (request a reviewer — do not self-lgtm)
- File Jira under [ROSAENG-391](https://redhat.atlassian.net/browse/ROSAENG-391) for anything the AI missed

## Weekly Handover Procedure

### Friday: Write Handover

A Slack workflow will be present to handle weekly handover with the template like below:

```markdown
# ROSA CI Handover - YYYY-MM-DD

## Weekly summary

### Healthy
- list of passing jobs

### Persistent Failures (diagnosed, fixes in progress)
- job name: description, Jira link, PR link

### No Data
- jobs with no recent builds

## Open PRs Needing Review
- Grouped by repository

## Monday Action Items
1. Concrete next steps for incoming watcher
```

### Monday: Incoming Watcher

1. Read the handover document from the outgoing watcher
2. Review the things highlighted in the handover
3. Check that any "in progress" fixes from last week have merged
4. Start to follow the `Daily Triage Procedure`

## Common Scenarios

### Conformance Job Turns Red

1. Check if `/ci-triage` already caught and classified the failure
2. If it's an OCP regression (test passes on prior nightly, fails on new one), file an upstream OCPBUGS-\* bug and add a temporary skip with a link to the bug
3. If it's a ROSA-side issue (cluster provisioning, STS policy, config drift), route to the appropriate ROSA team
4. Ack within 4 business hours in the reporting channel
5. If unresolved after 24 hours, escalate via WebRCA and bring in the relevant teams

### MC/SC Appears Degraded

1. Flag immediately in `#wg-rosa-ci-enhancement`
2. Circuit-break further test triage until MC is healthy
3. Attribute individual test failures on a degraded MC to the infrastructure issue
4. Related: [OCM-23872](https://redhat.atlassian.net/browse/OCM-23872) tracks making dev SCs/MCs healthy to reduce CI noise

### Cluster not ready due to readiness check failure

1. Flag immediately in `#wg-rosa-ci-enhancement`
2. Identify the component which is blocking the cluster readiness check
3. Fix the problem to unblock the tests if issue identified
4. Find the owner team of the component as high priority if the fix is not obvious