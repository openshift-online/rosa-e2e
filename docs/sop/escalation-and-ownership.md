# Escalation and Ownership

Who owns what, for the cases that fall outside the CI Watcher's own remit (see [`../ci-watcher/escalation-paths.md`](../ci-watcher/escalation-paths.md) for the CI-failure classification matrix - this doc covers component/infra ownership, not failure triage).

## CI job category ownership

When a CI failure needs to be routed to a person rather than a team-wide Slack ping, use the job's `ci-status-jobs.yaml` category:

| Job Category | Owner |
|---|---|
| ROSA CLI E2E / ROSA TF E2E | Amanda Katz (amakatz@redhat.com) |
| CAPA E2E | Mohamed ElSerngawy (melserng@redhat.com) |
| OCM FVT (HCP/Classic/GCP) | Jeff Frazier (jfrazier@redhat.com) |
| ROSA E2E STG / Conformance | Bo Meng (bmeng@redhat.com) |
| GAP E2E | Rohit Bhilare (rbhilare@redhat.com) |
| SRE Operator E2E | Dustin Row (drow@redhat.com) |

## Build-farm / Prow infrastructure issues -> DPTP

Not every CI failure is a ROSA-side problem. Build-farm/Prow infrastructure issues - `initializing_namespace` races, build0N cluster auth outages, DNS/proxy failures reaching backplane, ci-operator namespace/lease contention - are owned by DPTP (Developer Productivity Test Platform), not any ROSA team.

**Don't just write these off as "transient, no action."** They recur, and without a tracked DPTP issue there's no owner and no way to distinguish a one-off blip from a systemic build-farm regression. When you hit one:

1. Ask `@chai-bot` in the relevant thread to search for an existing DPTP issue for the same failure signature, and open one if none exists.
2. If it's urgent (broad job impact, not just one PR), escalate directly in `#forum-ocp-testplatform` and ping `@dptp-triage`.
3. Note the DPTP ticket in whatever Jira you filed on the ROSA side so the two are cross-linked.

**Worked example (2026-09-25):** a stale Infoblox DNS entry for `squid.corp.redhat.com` was causing backplane-login timeouts across multiple build clusters, hitting `hive-e2e`, `pagerduty-operator`, and `ocm-fvt` jobs. chai-bot, when asked in the failure thread, immediately found two open ROSA-side Jiras (ROSAENG-67737/67738) and the broader DPTP tracker (DPTP-5138), and confirmed the failing test's exact proxy configuration from the step registry source. That's the pattern to follow: ask chai-bot first, it usually already has (or can build) the context before you escalate a human. The escalation to `#forum-ocp-testplatform` got only an automated "an engineer will respond in several hours" ack - DPTP response times for helpdesk pings are not fast, so file/link the Jira regardless of whether a human has replied yet.
