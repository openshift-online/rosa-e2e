# ROSA CI Watcher

The CI Watcher is a weekly rotating role where one person monitors ROSA CI job health, triages failures, and routes fixes to the owning team. It is separate from on-call — no paging, no SLA, no after-hours obligation.

## Getting Started

1. Join Slack: `#wg-rosa-ci-enhancement` and `#wg-hcm-ocp-release-enablement`
2. Verify you appear on the [PagerDuty schedule](https://redhat.pagerduty.com/schedules/PGLVMVG) and `@rosa-ci-watcher` resolves to you during your week
3. Set up the `/ci-triage` skill in Claude Code — see the [rosa-claude-plugins](https://github.com/bmeng/rosa-claude-plugins) repo
4. Read the previous handover in `#wg-rosa-ci-enhancement`

## Key Tools

- [Sippy rosa-stage](https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage) — job pass rates and trends
- [Sippy component readiness](https://sippy-auth.dptools.openshift.org) — ROSA component readiness views
- [Prow CI](https://prow.ci.openshift.org/) — job results and build logs

## Key Contacts

| Role | Contact |
|------|---------|
| All rotation members | `@rosa-ci-team` in Slack |
| Current watcher | `@rosa-ci-watcher` in Slack |
| Escalation channel | `#wg-rosa-ci-enhancement` |

## Documentation

- [Role and Responsibilities](role-and-responsibilities.md) — what the watcher does (and doesn't do), key principles, anti-patterns
- [Runbook](runbook.md) — step-by-step daily triage procedure, full job list, handover template, common scenarios
- [Escalation Paths](escalation-paths.md) — failure classification matrix, conformance SLAs, TRT interface, routing tables
- [Rotation Schedule](rotation-schedule.md) — PagerDuty schedule, rotation members, PTO/swap process
