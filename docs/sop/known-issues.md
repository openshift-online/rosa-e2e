# Known Issues and Gotchas

Bugs and workarounds worth knowing before you spend time re-diagnosing them.

## Live: squid proxy / backplane DNS timeouts

`squid.corp.redhat.com:3128` (an Infoblox DNS DTC pool) intermittently resolves to a stale/invalid IP, `10.4.209.37`, which is not on the production backplane squid whitelist (valid ranges are `209.132.x.x`, `66.187.x.x`, `91.209.x.x` - a `10.x.x.x` RFC1918 address is not a legitimate backend). Any CI step that does `ocm-backplane login` through this proxy (e.g. the `rosa-hive-backplane-login` step, which defaults `BACKPLANE_PROXY_URL` to this host) times out after ~2 minutes with:

```
proxyconnect tcp: dial tcp 10.4.209.37:3128: connect: connection timed out
```

The step has no retry logic, so a single timeout kills the whole job. This is not code/test-specific - it hit `hive-e2e`, `pagerduty-operator`, and `ocm-fvt` jobs across multiple build clusters (build03/05/06/10) within a 24-hour window. Same class of issue as [ROSAENG-47756](https://redhat.atlassian.net/browse/ROSAENG-47756) (April 2023), where half the squid pool was broken.

**Tracking**: [ROSAENG-67737](https://redhat.atlassian.net/browse/ROSAENG-67737), [ROSAENG-67738](https://redhat.atlassian.net/browse/ROSAENG-67738), broader DPTP tracker [DPTP-5138](https://redhat.atlassian.net/browse/DPTP-5138), ROSA-side retry/diagnostic work [ROSAENG-66044](https://redhat.atlassian.net/browse/ROSAENG-66044). A fix adding retry logic to the `rosa-hive-backplane-login` step is proposed in [openshift/release#85930](https://github.com/openshift/release/pull/85930) but not yet merged.

**If you hit this**: the failure signature above means it's safe to `/retest` - it's infra, not a real regression. The actual fix needs RH IT to remove the bad IP from the Infoblox pool; that's outside anything fixable from the ROSA side beyond the retry-logic PR. See [`escalation-and-ownership.md`](escalation-and-ownership.md) for how this kind of build-farm infra issue gets escalated to DPTP.

## Operator e2e Prow CI (SRE Operator E2E)

These affect the SRE operator focused e2e suites that share Prow/lease-pool infrastructure with rosa-e2e, tracked historically under [ROSAENG-60066](https://redhat.atlassian.net/browse/ROSAENG-60066) (closed 2026-09-25 - full migration to Prow complete for all SRE operators, Hive-based operators, MCVW, MCC, CAD, and ocm-agent).

**Operator-e2e install can wipe Hive-managed CRs on shared lease clusters.** The `rosa-operator-install` step deletes the production ClusterPackage to hand its CRDs to the e2e package; PKO garbage-collects those CRDs, cascade-deleting any Hive/MCC-delivered CRs of the same kind (e.g. the console `RouteMonitor`, api `ClusterUrlMonitor` delivered via SelectorSyncSet, labeled `hive.openshift.io/managed: "true"`). Symptom: an operator promotion e2e fails "has all of the required resources" with something like `routemonitors.monitoring.openshift.io "console" not found` - this is not a test bug and not caused by the operator's own code changes if you see it on a lease cluster that another job recently ran against. Root-cause fix (orphan CRDs before the ClusterPackage delete, label-driven backup/restore, a handoff gate) is planned as a single PR against `openshift/release`; check with the SRE Operator E2E owner (see [`escalation-and-ownership.md`](escalation-and-ownership.md)) if it's landed.

**certman-operator cannot run its e2e suite directly against a shared Hive cluster.** [ROSAENG-61837](https://redhat.atlassian.net/browse/ROSAENG-61837). certman is deployed via PKO directly onto Hive clusters (not onto spoke ROSA/OSD clusters like most other operators), so the lease-pool e2e workflow is the wrong shape for it. Its test suite's `AfterAll` unconditionally deletes the `certman-operator` namespace, deletes the `clusterdeployments.hive.openshift.io` CRD (cascading to every ClusterDeployment on the cluster), and deletes AWS/Let's Encrypt secrets that, on a real Hive cluster, are the production credentials. It's currently safe only because the osde2e job provisions its own disposable cluster per run - it does NOT run directly against a shared hive cluster. Do not wire certman e2e onto the lease-pool workflow or point it at a shared Hive cluster without reworking the test suite first (isolated namespace, scoped cleanup, only touch resources the test itself created).

**MCVW build OOMs / test flake (resolved).** Closed via [ROSAENG-61841](https://redhat.atlassian.net/browse/ROSAENG-61841) - the boring-crypto Go build needed 6-8Gi instead of the original 4Gi ci-operator memory limit, plus a `sre-regular-user-validation` BeforeAll fix. Noted here in case of regression on a future dependency bump.

**SAPM gangway-bridge race condition.** Concurrent SAPM pipeline runs against the same saas file (common right after a batch of merges) can delete each other's gangway-bridge Jobs, causing a hard failure that isn't retried. This is an AppSRE `qontract-reconcile` issue, not fixable on the ROSA side. See [`sapm-and-promotion.md`](sapm-and-promotion.md).
