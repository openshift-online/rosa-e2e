# ROSA CI Watcher

The CI Watcher is a weekly rotating role where one person monitors ROSA CI job health, triages failures, and routes fixes to the owning team. It is separate from on-call — no paging, no SLA, no after-hours obligation.

## Getting Started

1. Join Slack: [#wg-rosa-cicd](https://redhat-internal.slack.com/archives/C0ADGRNAT8U), [#rosa-prow-info](https://redhat-internal.slack.com/archives/C0AT31ERJLS), and [#wg-hcm-ocp-release-enablement](https://redhat-internal.slack.com/archives/C07QEA1PDFX)
2. Verify you appear on the [PagerDuty schedule](https://redhat.pagerduty.com/schedules/PGLVMVG) and the [CI Watcher team](https://redhat.pagerduty.com/teams/PTYDD6M/users), and that `@rosa-ci-watcher` resolves to you during your week
3. Set up the `/ci-triage` skill in Claude Code — see the [rosa-claude-plugins](https://github.com/bmeng/rosa-claude-plugins) repo
4. Read the previous handover in [#wg-rosa-cicd](https://redhat-internal.slack.com/archives/C0ADGRNAT8U)

## Key Tools

- [Sippy rosa-stage](https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage) — job pass rates and trends
- [Sippy component readiness](https://sippy-auth.dptools.openshift.org) — ROSA component readiness views
- [Prow CI](https://prow.ci.openshift.org/) — job results and build logs
- [PagerDuty CI Watcher Team](https://redhat.pagerduty.com/teams/PTYDD6M/users) — rotation management
- [ROSA Engineering Dashboard - CI Health](https://rosa-eng-dashboard.apps.engineering.openshift.org/executive#ci-health) — consolidated CI job health and pass rate trends
- [ROSA Engineering Dashboard - Delivery](https://rosa-eng-dashboard.apps.engineering.openshift.org/delivery) — component deployment status across environments
- [#rosa-prow-info](https://redhat-internal.slack.com/archives/C0AT31ERJLS) — real-time Prow job result notifications (failures tag `@rosa-ci-watcher`)
- Chai-bot daily health report — posted to [#wg-rosa-cicd](https://redhat-internal.slack.com/archives/C0ADGRNAT8U) at 14:30 UTC with category pass rates and failure analysis

## Key Contacts

| Role | Contact |
|------|---------|
| All rotation members | `@rosa-ci-team` in Slack |
| Current watcher | `@rosa-ci-watcher` in Slack |
| Escalation channel | [#wg-rosa-cicd](https://redhat-internal.slack.com/archives/C0ADGRNAT8U) |

## Documentation

- [Role and Responsibilities](role-and-responsibilities.md) — what the watcher does (and doesn't do), key principles, anti-patterns
- [Runbook](runbook.md) — step-by-step daily triage procedure, full job list, handover template, common scenarios
- [Escalation Paths](escalation-paths.md) — failure classification matrix, conformance SLAs, TRT interface, routing tables
- [Rotation Schedule](rotation-schedule.md) — PagerDuty schedule, rotation members, PTO/swap process
