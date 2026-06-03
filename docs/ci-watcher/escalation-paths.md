# CI Failure Escalation Paths

Jira: [ROSAENG-1437](https://redhat.atlassian.net/browse/ROSAENG-1437)

## Core Principle: No Flakiness

Every failure has a root cause. The CI Watcher must investigate and classify, never dismiss a failure as "flaky" and retrigger.

Timeouts in particular are not infra noise. A timeout on cluster install is, in 99% of cases, hiding an issue in OSDFM, the Management Cluster, certificate rotation, or AWS quota exhaustion. Increasing the timeout only masks the problem and degrades the pipeline further.

## Classification Matrix

| Category | Indicators | Action | Route To |
|----------|-----------|--------|----------|
| Infrastructure issue | AWS timeout, GCS error, lease failure | Investigate root cause. Capture MC health (HC count, error ratio, node placement). File Jira with diagnostics | OCM / OSDFM team |
| Timeout | Cluster install/ready timeout, test timeout | Do NOT increase timeout. Capture MC diagnostics, check error cluster ratio, check certificate status. File Jira | OSDFM / HyperShift team |
| Test bug | Assertion failure, wrong expected value | File Jira, assign to test author | Test author |
| OCP payload | Conformance regression in new version | File upstream bug (OCPBUGS-\*), add skip only with tracked bug | TRT / OCP team |
| Config drift | Step registry mismatch, wrong env var | Fix in openshift/release | CI watcher (self-serve) |
| STS policy gap | IAM policy version mismatch | Check fallback logic, file Jira | IAM team |
| Untested profile | Private, external auth, or other profiles consistently failing or never running | Investigate feasibility. Create or fix the test. Do not leave in "no test" state | Test author / CI watcher |

## MC/SC Health Check

Before triaging individual test results, check the health of the CI management clusters and service clusters. Degraded infrastructure produces cascading failures that look like individual test bugs but are actually systemic.

Check for:
- **Error cluster ratio**: High proportion of HCPs in error state on the CI MC
- **Stuck deletions**: Clusters with deletionTimestamp set but still present (orphaned resources)
- **Developer HCPs on CI MCs**: Personal test clusters consuming capacity on CI infrastructure
- **Certificate issues**: STS AssumeRole failures, OIDC validation failures

If a MC is degraded (high error ratio, many stuck clusters), flag it immediately in `#wg-rosa-ci-enhancement`. Individual test failures on a degraded MC should be attributed to the infrastructure issue, not the tests.

Related: [OCM-23872](https://redhat.atlassian.net/browse/OCM-23872) tracks making dev SCs/MCs healthy to reduce CI noise.

## OCP Conformance Failures (TRT Interface)

ROSA HCP and Classic STS conformance jobs run openshift-tests against ROSA clusters on each OCP nightly. These jobs are the interface between OCP releases and ROSA, and TRT (Technical Release Team) monitors them to decide whether an OCP nightly is healthy enough to promote. A red ROSA conformance job can block an OCP release.

Conformance failures get a **faster response target** than other CI jobs.

### Response Targets

| Target | SLA | Notes |
|--------|-----|-------|
| Acknowledge | 4 business hours | Watcher confirms they've seen the failure and are investigating |
| Root cause identified | 24 business hours | Classified and routed to the owning team |
| Fix merged or skip-listed with tracked bug | 48 business hours | Job is green again, even if via temporary skip with OCPBUGS-\* |

### Inbound from TRT

TRT reports ROSA conformance failures by pinging `@rosa-ci-watcher` in `#wg-hcm-ocp-release-enablement`. This reaches the current watcher directly, with no routing through SRE on-call.

| Channel | Priority | Notes |
|---------|----------|-------|
| `@rosa-ci-watcher` in `#wg-hcm-ocp-release-enablement` | Primary | Direct to the current watcher. Fastest response |
| `@rosa-ci-watcher` in `#wg-rosa-ci-enhancement` | Secondary | If TRT is already in this channel |
| OHSS incident ([red.ht/ohss-incident](https://red.ht/ohss-incident)) | Fallback only | Goes to SRE on-call, not CI watcher. Use only if Slack is down or no response after 4 hours |

### Conformance Failure Routing

| Failure area | Route to |
|-------------|----------|
| HyperShift operator, HostedCluster lifecycle | HyperShift team |
| Hive, ClusterDeployment, ClusterImageSet | Hive team |
| Cluster provisioning, OSDFM | OCM / OSDFM team |
| OCP networking, ingress, DNS | OCP Networking team |
| OCP installer, bootstrap | OCP Installer team |
| STS roles, IAM policy | SRE IAM team |
| Unknown or cross-cutting | Create WebRCA, bring in all relevant teams |

## Escalation Triggers

| Situation | Action |
|-----------|--------|
| OCP conformance failure reported by TRT | Ack within 4 hours, classify and route within 24 hours |
| Multiple categories failing simultaneously | Post in `#wg-rosa-ci-enhancement`, tag `@rosa-ci-architect` |
| All conformance red for 48+ hours | Escalate to ROSA leadership as blocking for component readiness |
| Persistent failure (2+ consecutive red runs) | Promote to tracked failure, file Jira with root cause analysis |
| MC/SC degraded (high error cluster ratio) | Flag immediately, circuit-break further test triage until MC is healthy |
| Conformance failure unresolved after 24 hours | Create WebRCA incident, bring in relevant teams |

## Jira Conventions

- **Project**: File failure stories under [ROSAENG-391](https://redhat.atlassian.net/browse/ROSAENG-391) (E2E Suite Reliability)
- **Watcher rotation issues**: [ROSAENG-394](https://redhat.atlassian.net/browse/ROSAENG-394) (CI Watcher Role)
- Include the job name, build ID, Prow link, and failure classification in every Jira story
- Assign to the owning team based on the classification matrix above
