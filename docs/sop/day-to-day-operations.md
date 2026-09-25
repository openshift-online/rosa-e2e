# Day-to-Day Operations

What actually fills the days of whoever's driving ROSA CI Watcher work, beyond the formal runbook steps in [`../ci-watcher/runbook.md`](../ci-watcher/runbook.md).

## The recurring channels

- **[#wg-rosa-cicd](https://redhat-internal.slack.com/archives/C0ADGRNAT8U)** - the main working channel. Chai-bot's daily health report lands here, CI Watcher handovers post here (automated weekly digest tagging the on-call usergroup with CI health by category, unresolved `#rosa-prow-info` threads needing follow-up, key merges, and open PRs needing review), and it's where you escalate infra/multi-category problems.
- **[#rosa-prow-info](https://redhat-internal.slack.com/archives/C0AT31ERJLS)** - real-time Prow job result feed. This is where individual failure investigation threads happen, often with `@chai-bot` pulled in directly to get a first-pass diagnosis before a human responds.
- **[#forum-rosa-support](https://redhat.enterprise.slack.com/archives/CCX9DB894)** - fields access/permissions questions from the wider team: adding people to the `openshift-online` GitHub org or the `rosa-org` GitHub team, confirming org/team membership for new contributors. This is routine, low-effort, but comes up often enough to expect it as part of the job, not a one-off.
- **[#forum-rosa-deployments](https://redhat.enterprise.slack.com/archives/C081W589GRG)** - bundle release coordination.
- **[#sre-operators](https://redhat.enterprise.slack.com/archives/CFJD1NZFT)** - SRE operator PR review requests.

## Triage pattern in practice

The documented flow in [`../ci-watcher/role-and-responsibilities.md`](../ci-watcher/role-and-responsibilities.md) (read logs -> check known issues -> check MC/SC health -> confirm reproducibility -> file Jira -> route) holds up in practice, with one addition worth calling out: **ask chai-bot before escalating to a human.** In a live example (squid proxy DNS incident, 2026-09-25), asking `@chai-bot` directly in the `#rosa-prow-info` failure thread got an immediate, well-sourced answer: it identified the exact failing step and its proxy configuration from the `openshift/release` source, found the two relevant open ROSA-side Jiras, and confirmed the failure was infra and safe to `/retest` - all before escalating to DPTP. Escalating to `#forum-ocp-testplatform` (DPTP helpdesk) after that only got an automated "an engineer will respond in several hours" ack, so chai-bot's first-pass answer was the actually useful information for the several hours in between. Don't skip straight to a human escalation without giving chai-bot the failure thread first.

## Recurring PR review load

Expect a steady trickle of PRs needing review/lgtm across `openshift/release` and the operator repos - conformance skip-list changes, CI debugging steps, lint/perf tweaks, backend-tests fixture updates. These show up in the weekly CI Watcher handover digest under "Open PRs needing review." Don't let them stack up past a week; an approved-but-unmerged PR (missing only an `/lgtm`) is one of the easiest things to clear quickly.
