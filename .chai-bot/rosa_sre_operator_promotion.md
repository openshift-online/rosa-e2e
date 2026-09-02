# Weekly SRE Operator Promotion — Scheduled Task Instructions

## Goal

Every week, check all SRE operators defined in `component-deployments.yaml` for new commits that haven't been promoted to production canary (wave 1). For each operator with new changes:

1. Compare the current production canary `ref` with the stage target `ref` (the latest pipeline-validated sha)
2. Analyze code changes for risk (risky patterns, missing e2e coverage) — informational for reviewers
3. If the stage ref differs from the prod-canary ref, create an MR in `gitlab.cee.redhat.com/service/app-interface` updating the prod-canary targets
4. Post a summary to `#sre-operators` with threaded replies per operator

## The Promotion Pipeline Chain

Each saas file defines a promotion pipeline chain:

```
Integration (ref: master, auto) → promotion-int e2e → Stage (auto) → promotion-stage e2e → Prod-canary (MANUAL) → Prod wave-2 (auto, soakDays)
```

The stage target's `ref` represents the latest sha that has passed through the entire pipeline (integration deployment → promotion-int e2e → stage deployment → promotion-stage e2e). If a sha failed any e2e job, it would NOT appear in the stage target. There is no need to independently check Prow CI health — the pipeline already gates everything.

## Important Rules

- **ALWAYS produce a Slack report** — even if no operators need promotion, post a summary message.
- **One MR per operator** — each operator gets its own MR (not batched). This allows independent review and rollback.
- **Only modify the `ref` field** in prod-canary targets. Never change namespace refs, parameters, promotion subscriptions, or any other field.
- **Only promote a sha that has progressed through all pipeline stages** (integration → stage). The stage target ref in the saas file represents the latest validated sha.
- **Do not compare against GitHub HEAD** — the pipeline determines what is ready for promotion. Use the stage target ref as the source of truth.
- **Do not create an MR if there are no new pipeline-validated changes** — if stage ref == prod-canary ref, skip.
- **Discover operators dynamically** — read `configs/component-deployments.yaml` at runtime rather than using a hardcoded list. This ensures new operators are automatically included.

## Operator Discovery

The authoritative list of operators is defined in `configs/component-deployments.yaml` in the `openshift-online/rosa-e2e` GitHub repository. Read this file at runtime to discover the operators to process.

### Discovery procedure

1. **Read `configs/component-deployments.yaml`** from `openshift-online/rosa-e2e` on GitHub
2. **Filter for promotable components**: Select all entries with `strategy: hive` (these are SRE operators deployed via the Hive pipeline). Also include `managed-cluster-config` explicitly.
3. **Exclude non-operator components**: After filtering for `strategy: hive`, remove these keys — they are not SRE operators:
   - `hive` (the Hive deployment itself)
   - `osd-rhobs-rules-and-dashboards` (observability rules/dashboards, not a deployable operator)
4. **Include all remaining hive-strategy components**: Some operators have not yet migrated to Prow-based e2e testing (they lack an `e2e_saas` field in `component-deployments.yaml`). These are still included — they have stage and prod-canary targets in their saas files and follow the same promotion pipeline pattern. Do not skip operators just because they lack `e2e_saas`. Operators currently without Prow migration include:
   - `pagerduty-operator`
   - `deadmanssnitch-operator`
   - `gcp-project-operator`
   - `managed-cluster-config`
   - `managed-velero-operator`
   - `managed-cluster-validating-webhooks`
   - `managed-node-metadata-operator`
5. **For each component, extract**:
   - `saas_name` — the saas resource name in app-interface (e.g. `saas-route-monitor-operator-pko`, `saas-muo`). This is the `name:` field inside the saas YAML file in app-interface, NOT the file path.
   - `repo` — the GitHub repository (e.g. `openshift/route-monitor-operator`)
6. **Find the saas file in app-interface**: Search `data/services/osd-operators/cicd/saas/` in `gitlab.cee.redhat.com/service/app-interface` for a YAML file whose top-level `name:` field matches the `saas_name`. Note that the file name on disk may differ from the `saas_name` (e.g. `saas_name: saas-muo` → file is `saas-managed-upgrade-operator.yaml`).

### Important notes on saas_name vs file paths

- The `saas_name` is the **resource identifier** used inside app-interface (the `name:` field at the top of each saas YAML file)
- The `saas_name` may be an abbreviation that does NOT match the file name on disk (e.g. `saas-muo` vs `saas-managed-upgrade-operator.yaml`, `saas-cdo-pko` vs `saas-custom-domains-operator-pko.yaml`)
- Always use the `saas_name` to identify the correct saas resource, then locate its file by searching for a file containing `name: <saas_name>` at the top level
- Do not hardcode file paths — discover them dynamically using the `saas_name`

All saas files are under `data/services/osd-operators/cicd/saas/` in `gitlab.cee.redhat.com/service/app-interface`.

## Procedure

### 1. Post initial Slack message

Post a parent message to `#sre-operators`:

> 🚀 **Weekly SRE Operator Promotion — <today's date>**
> cc @osd-operators-saas-approver
> Checking <N> operators for prod-canary promotion…

Use `<!subteam^S0BLN6AN7EK>` to mention the @osd-operators-saas-approver group.

All subsequent operator-specific updates go as **threaded replies** to this parent message.

### 2. For each operator — Check for pipeline-validated changes

For each operator discovered from `component-deployments.yaml`:

1. **Read the saas file** from app-interface using GitLab tools:
   - Use the `saas_name` from `component-deployments.yaml` to find the matching saas resource in app-interface. Search for files in `data/services/osd-operators/cicd/saas/` whose `name:` field matches the `saas_name`.
   - Parse the YAML to find all `resourceTemplates` entries

2. **Find the stage targets**: Search for targets that have `auto: true` in their promotion block and subscribe to integration/e2e success channels. These typically have names containing `-stage-` or `-hives0` patterns. Each operator typically has two stage targets (one per hive shard).

3. **Find the prod-canary targets**: Search for target names containing `prod-canary`. Each operator typically has two:
   - `<prefix>-hivep03uw1-prod-canary`
   - `<prefix>-hivep04ew2-prod-canary`
   
   The prefix varies per operator (e.g. `rmo-`, `co-`, `oao-`, `mcc-`). Match by the `prod-canary` suffix.

4. **Extract the stage ref**: Get the `ref` (git sha) from the stage targets. This is the latest pipeline-validated sha — it has passed through integration deployment, promotion-int e2e, stage deployment, and promotion-stage e2e.

5. **Extract the prod-canary ref**: Get the `ref` (git sha) from the prod-canary targets. This is the current production sha.

6. **Validate targets agree**: Confirm that both stage targets have the same `ref`, and both prod-canary targets have the same `ref`. If either pair disagrees, flag it as a pipeline anomaly in the Slack thread. Post a threaded reply:
   > 🔍 **<operator-name>** — Stage targets disagree on ref (hivep03: <sha1>, hivep04: <sha2>). Skipping — investigate pipeline.
   
   Skip this operator and move to the next.

7. **Compare**: If `stage ref == prod-canary ref` → no new pipeline-validated changes. Post a threaded reply:
   > ⏭️ **<operator-name>** — No new pipeline-validated changes. Skipping.
   
   Move to the next operator.

### 3. For each operator with changes — Analyze code diff

If the stage ref differs from the prod-canary ref, there are new pipeline-validated changes ready for promotion.

> **Note**: This analysis is informational for reviewers — the pipeline has already validated the sha through promotion-int and promotion-stage e2e jobs.

1. **Get the commit list** between the prod-canary ref and the stage ref using GitHub tools. Count the commits and list their summaries.

2. **Categorize the changes** by reviewing the diff:
   - **Boilerplate / Low Risk**: dependency bumps (`go.mod`, `go.sum`, `vendor/`), generated code, `Makefile` updates, `Dockerfile` updates, `.github/` CI config, documentation changes
   - **Code Changes / Medium Risk**: modifications to Go source files under `pkg/`, `cmd/`, `controllers/`, `api/`
   - **High Risk Patterns**: Look specifically for:
     - RBAC changes (`role.yaml`, `clusterrole.yaml`, `*_rbac.go`)
     - CRD changes (`*_crd.yaml`, `*_types.go`, API version changes)
     - Webhook changes (`webhook*.go`, `validating*.yaml`, `mutating*.yaml`)
     - Security context changes
     - New dependencies on external services

   High-risk patterns should be **flagged with a warning** in the Slack thread and MR description, but do NOT hold the MR — still create it. The reviewer can decide whether to merge.

3. **Check for e2e test coverage**: Look for changes in `test/`, `e2e/`, `osde2e/` directories in the diff.
   - If there are **code changes but NO e2e changes** → flag: `⚠️ Code changes without e2e test updates`
   - If there are **only boilerplate changes** → no e2e flag needed

### 4. For each operator — Create promotion MR

If the stage ref differs from the prod-canary ref:

1. **Prepare the saas file change**: Update ONLY the `ref` field in both prod-canary targets to the stage ref sha. Do not modify any other fields.

2. **Create the MR** on `gitlab.cee.redhat.com/service/app-interface`:
   - **Title**: `Promote <operator-name> to prod-canary (<short-sha>)`
   - **Target branch**: `master`
   - **File to change**: The saas file discovered via the `saas_name` lookup (under `data/services/osd-operators/cicd/saas/`)
   - **Description** (use this template):

```
## Monitoring and Validation

- 📊 [Monitor delivery status](https://rosa-eng-dashboard.apps.engineering.openshift.org/delivery?component=<operator-name>)
- 🧪 [View Prow e2e results](https://prow.ci.openshift.org/?type=periodic&job=*<operator-name>*)

## Changes

[Compare changes on GitHub](https://github.com/<repo>/compare/<prod-canary-ref>...<stage-ref>)

## Commit Summary

<list the commits between prod-canary-ref and stage-ref, one line each>

## Risk Assessment

<brief summary of risk level and any flags>
```

3. **Post threaded reply** to the parent Slack message with the result.

### 5. Threaded reply format per operator

Use these emoji-prefixed formats for the threaded replies:

**Promoted successfully (low risk):**
> ✅ **<operator-name>** — <N> new commits, low risk (boilerplate only).
> MR: <gitlab MR link>
> Changes: <GitHub compare link>

**Promoted with flags (medium/high risk):**
> ⚠️ **<operator-name>** — <N> new commits. <risk flags>.
> MR: <gitlab MR link>
> Changes: <GitHub compare link>
> _Flags: <list flags, e.g. "Code changes without e2e tests", "RBAC modifications">_

**Pipeline anomaly (targets disagree):**
> 🔍 **<operator-name>** — Stage targets disagree on ref (hivep03: <sha1>, hivep04: <sha2>). Skipping — investigate pipeline.

**No changes:**
> ⏭️ **<operator-name>** — No new pipeline-validated changes. Skipping.

### 6. Post summary

After processing all operators, edit the parent message or post a final threaded reply with a summary:

> **Summary:**
> - ✅ Promoted: <N> operators
> - ⚠️ Promoted with flags: <N> operators
> - 🔍 Pipeline anomalies: <N> operators
> - ⏭️ Skipped (no changes): <N> operators
> - Total MRs created: <N>

## Error Handling

- If you cannot read a saas file from app-interface → log the error in the thread and continue with the next operator
- If you cannot find stage targets in the saas file → log it as an anomaly and skip that operator
- If MR creation fails → log the error in the thread with the specific failure reason
- **Never skip the Slack report** — always post the parent message and at least a summary, even if everything fails

## Technical Notes

- The saas files use `$ref` for namespace references — these are YAML references internal to app-interface, not git refs. Do not confuse them with the git `ref` field.
- Some saas files have multiple `resourceTemplates` entries (e.g. managed-cluster-config has `managed-cluster-config-integration` and `managed-cluster-config-production`). The prod-canary targets are in the production template. Only modify the production template.
- The `promotion.subscribe` blocks in the saas file define automated promotion channels — do not modify these.
- Both prod-canary targets (`hivep03uw1` and `hivep04ew2`) should always be updated to the same ref in a single MR.
- The stage targets represent shas that have passed through the full promotion pipeline (integration deployment → promotion-int e2e → stage deployment → promotion-stage e2e). There is no need to independently check Prow CI health — the pipeline already gates everything.
- Stage targets typically have `auto: true` in their promotion block and subscribe to integration/e2e success channels.
