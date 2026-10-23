# Provisioner Types and the Provision × Validate Matrix

This guide explains how to run rosa-e2e validation against a cluster provisioned by any of the
supported **provisioner types**, and how to wire a new CI job that composes a provisioner
front-end with the shared validation back-end.

Design doc: [Unified ROSA E2E Testing Architecture](https://docs.google.com/document/d/1dRynQSB2bArf5hrVLobz8mWwJdCqVf8Vaytx1UE8j98/edit)
(Jira: [ROSAENG-308](https://redhat.atlassian.net/browse/ROSAENG-308)).

## The architecture in one paragraph

Provisioning is a swappable front-end owned by each tool team; validation is a shared back-end
owned by rosa-e2e, split into a customer plane and a platform plane. Provisioners publish a
frozen [`SHARED_DIR` contract](#the-shared_dir-contract) and the validation refs consume it, so
any provisioner can feed the same validation tail:

```
  PROVISIONERS (front-end, team-owned)         VALIDATION (back-end, rosa-e2e owned)

  +---------------------------+                +--------------------------------+
  | rosa-cli-provision        |--+             | CUSTOMER PLANE                 |
  | terraform-rhcs-provision  |  |  SHARED_DIR |   Area:DataPlane,              |
  | ocm-ui-provision (planned)|  +--------+    |   Area:CustomerFeatures        |
  | rosa-e2e-ocm-provision    |--+        |    |   (from cluster kubeconfig)    |
  +---------------------------+          |    +--------------------------------+
                                         v
                            +------------------------+
                            | rosa-backplane-login   |
                            |  -> mc/sc-kubeconfig  |
                            +------------------------+
                                         |    +--------------------------------+
                                         +--> | PLATFORM PLANE                 |
                                              |   Area:ManagedService,         |
                                              |   Area:InfraTiers, MgmtPlane   |
                                              |   (from MC/SC kubeconfigs)     |
                                              +--------------------------------+
```

Composition is one chain per provisioner with an identical tail — swap the head, keep the tail:

```
<provisioner>-provision -> rosa-backplane-login ->
rosa-e2e-validate-customer -> rosa-e2e-validate-platform -> destroy
```

## The SHARED_DIR contract

The frozen interface between provisioner teams and rosa-e2e. Provisioners write it; validation
refs read it. `cluster-metadata.json` lets the validation refs self-select feature assertions
without per-job config.

| Path | Written by | Consumed by | Contents |
|---|---|---|---|
| `${SHARED_DIR}/cluster-id` | provisioner | both planes | OCM cluster ID |
| `${SHARED_DIR}/kubeconfig` | provisioner | customer plane | cluster (hosted) kubeconfig |
| `${SHARED_DIR}/bp-kubeconfig` | `rosa-backplane-login` | customer plane (fallback) | hosted-cluster kubeconfig via backplane |
| `${SHARED_DIR}/mc-kubeconfig` | `rosa-backplane-login` | platform plane | management cluster kubeconfig |
| `${SHARED_DIR}/sc-kubeconfig` | `rosa-backplane-login` | platform plane | service cluster kubeconfig |
| `${SHARED_DIR}/cluster-metadata.json` | provisioner | both planes | cluster type, region, channel, feature flags |

`cluster-metadata.json` schema (version `1.0.0`, `provisioner` identifies the head):

```json
{
  "schema_version": "1.0.0",
  "provisioner": "rosa-e2e-cli",
  "cluster_id": "...",
  "cluster_name": "...",
  "topology": "hcp",
  "region": "us-west-2",
  "channel_group": "stable",
  "openshift_version": "4.22",
  "feature_flags": {
    "privatelink": false, "private": false, "kms": false, "etcd_encryption": false,
    "fips": false, "sts": true, "byovpc": true, "sharedvpc": false,
    "zero_egress": false, "multi_az": false
  }
}
```

Every feature flag is exported to the suite as `FEATURE_<NAME>` (e.g. `FEATURE_PRIVATELINK=true`),
and `topology` as `CLUSTER_TOPOLOGY`, so specs enable/skip themselves.

## Provisioner types

All step-registry components below live in
[openshift/release](https://github.com/openshift/release/tree/master/ci-operator/step-registry/rosa/e2e).
Status as of October 2026.

### rosa-e2e-ocm — OCM Go SDK *(rosa-e2e owned)*

| | |
|---|---|
| Chain | `rosa-e2e-hcp-ocm-provision` |
| Publisher ref | `rosa-e2e-ocm-provision` |
| Workflow | `rosa-e2e-hcp-ocm` |
| Merged | [#85810](https://github.com/openshift/release/pull/85810) (ROSAENG-67580) |
| Image | `rosa-e2e` (pipeline image) |
| Status | **Blocked on `PROVISION_ENTRYPOINT`** — the rosa-e2e image has no standalone provisioner binary yet (its only entrypoint is `e2e.test`, whose full-lifecycle spec self-deletes). Tracked as a ROSAENG-308 follow-up. |

Provisions via the OCM Go SDK (`pkg/framework/ocm.go`) and publishes the whole contract itself.
Feature flags come from the job's per-feature env vars.

### rosa-cli — rosa CLI *(CLI/TF team's day1, ROSA CI wiring)*

| | |
|---|---|
| Chain (public) | `rosa-e2e-hcp-rosa-cli-provision` |
| Chain (PrivateLink) | `rosa-e2e-hcp-rosa-cli-private-provision` |
| Publisher ref | `rosa-e2e-rosa-cli-publish-contract` |
| Merged | [#86844](https://github.com/openshift/release/pull/86844) (ROSAENG-67583) |
| Image | `rosa-aws-cli` base image |
| Status | **Working.** PrivateLink pilot job: [#87094](https://github.com/openshift/release/pull/87094) (ROSAENG-67585) |

Reuses the long-standing `rosa-cluster-provision` steps; the publisher emits only the missing
`cluster-metadata.json` (`cluster-id`/`kubeconfig` are already produced by the flow). Feature
flags come from the job's env vars.

**PrivateLink caveat:** the public chain includes `rosa-conf-idp-htpasswd`, whose `oc login`
cannot reach a private API from the build farm. The `-private` variant swaps that step for
`rosa-cluster-credentials-backplane` (which also writes `proxy-conf.sh` so downstream steps
route through the corp proxy). Use the private chain for any private/privatelink cluster.

### terraform-rhcs — terraform-provider-rhcs *(CLI/TF team)*

| | |
|---|---|
| Chain | `rosa-e2e-hcp-terraform-rhcs-provision` |
| Publisher ref | `rosa-e2e-terraform-rhcs-publish-contract` |
| Merged | [#86986](https://github.com/openshift/release/pull/86986) (ROSAENG-67583) |
| Image | `rhcs-tf-e2e` pipeline image — **built only by terraform-redhat/terraform-provider-rhcs** |
| Status | **Working** (contract verified in the PR rehearsal). |

Wraps the provider's `rhcs-e2e-setup` day1 (driven by `CLUSTER_PROFILE`, a profile from
`tests/ci/profiles/tf_hcp_cluster_profiles.yml`). The TF profile builds its own account roles,
OIDC config and VPC — do **not** chain `aws-provision-vpc-shared` / `rosa-sts-oidc-config-create`
in front of it. The publisher resolves `cluster-id` from OCM by cluster name and derives feature
flags from the OCM cluster representation (the profile-driven flow has no per-feature env vars).

Two hard constraints:

1. **Teardown must use `rhcs-e2e-teardown`** (the cluster exists only in the terraform state
   saved to `${SHARED_DIR}/statefiles.tar.gz`). Never `rosa-cluster-deprovision` — the cluster
   and its AWS resources will leak.
2. **No customer kubeconfig is published** — every HCP profile sets `admin_enabled: false`.
   `rosa-backplane-login` must run before the validation tail so `rosa-e2e-validate-customer`
   can fall back to `bp-kubeconfig`.

### Planned types

- **ocm-ui** — browser-driven OCM console flow via Playwright, being built under
  [OCMUI-4682](https://issues.redhat.com/browse/OCMUI-4682). Must conform to the SHARED_DIR
  contract to plug in as a front-end.
- **rosa-backend-tests (FVT)** — wraps FVT's OCM API day1. Coordinate with CS/OCM QE.

## The validation tail

| Ref | Plane | Label filter (default) | Reads |
|---|---|---|---|
| `rosa-e2e-validate-customer` | customer | `Area:DataPlane \|\| Area:CustomerFeatures` | `kubeconfig` (falls back to `bp-kubeconfig`) |
| `rosa-e2e-validate-platform` | platform | `Area:ManagedService \|\| Area:InfraTiers \|\| Area:MgmtPlane` | `mc-kubeconfig`, `sc-kubeconfig` |

Both run `/usr/local/bin/e2e.test` from the `rosa-e2e` image (`dependencies: rosa-e2e`), load
feature flags from `cluster-metadata.json`, and source `proxy-conf.sh` when present (private
clusters). Do not set a job-level `LABEL_FILTER` — it would clobber both refs' distinct
defaults; override per-ref only if you truly need to.

`rosa-backplane-login` writes `bp-kubeconfig`, `mc-kubeconfig`, `sc-kubeconfig` (best-effort)
and `mc-cluster-id`/`mc-cluster-name`.

## Constraints and gotchas

- **The platform plane is intranet-gated.** Backplane requires corp reachability:
  `capabilities: [intranet]`, `restrict_network_access: false` on the job, and the
  `network-access-rehearsals-ok` label (added by an org member other than the PR author via
  `/pj-rehearse network-access-allowed`) for rehearsals. Run customer-plane-only where backplane
  is not reachable.
- **Image requirements in the consuming ci-operator config:**
  - rosa-cli head: `base_images: rosa-aws-cli`
  - terraform-rhcs head: builds (or imports) `rhcs-tf-e2e`
  - any validation tail: the `rosa-e2e` image (pipeline image or `base_images: rosa-e2e`)
- **Teardown rule of thumb:** rosa-cli/OCM heads tear down with `rosa-cluster-deprovision`;
  terraform head with `rhcs-e2e-teardown`. Mixing them leaks resources.
- **Backplane elevation reasons** must be trackable URLs/Jira keys (each ref has a
  `BACKPLANE_ELEVATE_REASON` default).
- **RACI:** rosa-e2e (ROSA CI) owns the validation refs, `rosa-backplane-login` and the
  composition chains; the CLI/TF/UI/FVT teams own their provision refs. The SHARED_DIR schema
  is the frozen interface between them — bump `schema_version` when it changes.

## Recipe: creating a new job

1. Pick the provisioner type and its chain (see above); for a private cluster use the
   `-private` variant where one exists.
2. In the consuming repo's ci-operator config, ensure the required images (see constraints).
3. Compose the workflow — head chain, wait-ready refs, backplane-login, then the validation
   tail; post: standard gap-analysis + teardown (best-effort).
4. Gate the job: `capabilities: [intranet]`, `restrict_network_access: false`,
   `allow_best_effort_post_steps: true`.
5. Set env: `OCM_LOGIN_ENV`, `REGION`, `CHANNEL_GROUP`, plus head-specific vars
   (e.g. `CLUSTER_PROFILE` for terraform, `CLUSTER_SECTOR` for rosa-cli HCP shards).
6. Run `make update` in openshift/release, then rehearse with
   `/pj-rehearse <job-name>` (after the network-access label).

### Worked example: PrivateLink data-plane + MC health (rosa-cli head)

```yaml
# openshift-online/rosa-e2e __periodics variant
- as: rosa-hcp-e2e-privatelink-connectivity-mc-health-staging
  capabilities:
  - intranet
  cron: 0 2 * * 3
  restrict_network_access: false
  steps:
    allow_best_effort_post_steps: true
    cluster_profile: rosa-e2e-01
    env:
      CHANNEL_GROUP: stable
      CLUSTER_SECTOR: rosa-e2e
      ENABLE_BILLING_ACCOUNT: "yes"
      HOSTED_CP: "true"
      OCM_LOGIN_ENV: staging
      REGION: us-west-2
    workflow: rosa-e2e-hcp-rosa-cli-private
```

The workflow (`rosa-e2e-hcp-rosa-cli-private`) composes
`rosa-e2e-hcp-rosa-cli-private-provision` → `rosa-cluster-wait-ready-hosted-cp`/`-nodes` →
`rosa-backplane-login` → `rosa-e2e-validate-customer` → `rosa-e2e-validate-platform`, then the
standard teardown. See [#87094](https://github.com/openshift/release/pull/87094).

## References

- Design doc: [Unified ROSA E2E Testing Architecture](https://docs.google.com/document/d/1dRynQSB2bArf5hrVLobz8mWwJdCqVf8Vaytx1UE8j98/edit) (ROSAENG-308)
- SHARED_DIR contract: [ROSAENG-67580](https://redhat.atlassian.net/browse/ROSAENG-67580) — PR [#85810](https://github.com/openshift/release/pull/85810)
- Validation split: [ROSAENG-67581](https://redhat.atlassian.net/browse/ROSAENG-67581)
- rosa-cli + terraform-rhcs heads: [ROSAENG-67583](https://redhat.atlassian.net/browse/ROSAENG-67583) — PRs [#86844](https://github.com/openshift/release/pull/86844), [#86986](https://github.com/openshift/release/pull/86986)
- PrivateLink pilot: [ROSAENG-67585](https://redhat.atlassian.net/browse/ROSAENG-67585) — PR [#87094](https://github.com/openshift/release/pull/87094)
- Step registry: [ci-operator/step-registry/rosa/e2e](https://github.com/openshift/release/tree/master/ci-operator/step-registry/rosa/e2e)
