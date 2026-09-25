# SAPM and Operator Promotion

How SRE operator deploys get promoted through environments, and how that connects to this repo's CI.

## What SAPM does

SAPM (SaaS Auto Promotions Manager, part of app-interface/AppSRE tooling) progressively promotes operator SaaS files through wave-gated environments (e.g. int -> stage -> prod canary -> prod). It only advances a wave once the previous one has "soaked" successfully for its configured `soakDays`.

For SRE operators onboarded to Prow e2e (all of them, as of [ROSAENG-60066](https://redhat.atlassian.net/browse/ROSAENG-60066) closing 2026-09-25), SAPM promotion is gated on a green Prow on-demand job via a **gangway-bridge**: each operator's saas file (except MCC) has a bridge that triggers the Prow e2e job and blocks the deploy pipeline on pass/fail, so a broken e2e run stops the promotion rather than just being observed after the fact.

## Never bypass the wave structure

Don't promote all targets in a saas file to the same SHA at once to "unstick" something. Only promote wave-1 (canary) targets and let SAPM cascade through the remaining waves on its own schedule. Promoting everything at once defeats the entire point of progressive delivery - if the change is bad, it hits every environment simultaneously instead of being caught at canary.

## When a SAPM pipeline fails

Two different failure modes, don't conflate them:

1. **The gated Prow e2e job actually failed** (real regression or flaky test) - this is a CI triage problem, not a SAPM problem. Follow the normal CI Watcher triage process in [`../ci-watcher/`](../ci-watcher/README.md).
2. **The SAPM PipelineRun itself failed for an infra reason** unrelated to the operator/test code (network blip, transient API failure, or the gangway-bridge race condition below) - this needs a pipeline retrigger.

### Gangway-bridge race condition

When two SAPM pipeline runs trigger concurrently for the same saas file (common right after several PRs merge close together), the second run's reconcile can delete the first run's gangway-bridge Job (different random Job-ID suffix looks like "current but not desired" in the 3-way diff). The first run then gets a 404 on `validate_realized_data`, which raises an exception type the retry logic doesn't catch, so it fails hard instead of retrying. This is an AppSRE `qontract-reconcile` bug, not something fixable from the ROSA side - if you hit it, just retrigger.

### Retriggering a failed SAPM PipelineRun

**Use `osdctl ci retrigger-pipeline`.** Do not use an older manual backplane+kubectl script for this if you come across one referenced in a design/reference doc elsewhere - it predates the osdctl command, is more fragile, and is deprecated.

```bash
osdctl ci retrigger-pipeline \
  -n <operator>-pipelines \
  -r <failed-pipelinerun-name> \
  --reason <JIRA-KEY-or-URL>
```

Find the failed run first via `ocm backplane login --multi 29nmp5rhf8rgclg4a02lju4eld79js9e` then `kubectl get pipelineruns -n <operator>-pipelines --sort-by=.metadata.creationTimestamp`. Common namespaces: `certman-operator-pipelines`, `cloud-ingress-operator-pipelines`, `rbac-permissions-operator-pipelines`, `ocm-agent-operator-pipelines`.

If `osdctl ci retrigger-pipeline` isn't recognized, upgrade osdctl rather than falling back to a manual script.

Full step-by-step SOP with prerequisites and monitoring commands lives in the `hcm-design` design-docs repo at `cicd/docs/sapm-pipeline-retrigger.md` (a separate git repo from this one, so a direct link here won't resolve). The broader promotion workflow (osdctl promote saas, progressive delivery phases, monitoring links) is documented alongside it at `cicd/docs/operator-promotion.md` in that same repo.

## Operator abbreviations (easy to mix up)

**CAMO = configure-alertmanager-operator** (not managed-node-metadata). MNMO = managed-node-metadata-operator.
