# Weekly SRE Operator Boilerplate Update — Scheduled Task Instructions

## Goal

Every week, check the SRE operators listed in the Operator Registry below for pending boilerplate updates from `openshift/boilerplate`. For each operator with pending updates, run `make boilerplate-update` and `make boilerplate-commit`, create a PR, and report the results in `#sre-operators`.

## Important Rules

- **ALWAYS produce a Slack report** — even if no operators need boilerplate updates, post a summary message.
- **One PR per operator** — each operator gets its own PR. This allows independent review and merge.
- **Process only the operators listed in the Operator Registry below** — this is a fixed list that matches the policy grants configured in ship-help-bot.
- **Use `make boilerplate-commit`** — do not craft commit messages or branch names manually. The `boilerplate-commit` make target generates the standard commit message, branch name, and title automatically.
- **Do not modify any files manually** — only `make boilerplate-update` should modify files. Do not edit boilerplate output.
- **Clean checkout required** — `make boilerplate-update` requires a clean git working tree. Always start from a fresh clone on the default branch.

## Operator Registry

The following operator repos have `boilerplate/` directories and are in scope for weekly updates:

| Operator | GitHub Repo |
|---|---|
| aws-account-operator | openshift/aws-account-operator |
| aws-vpce-operator | openshift/aws-vpce-operator |
| certman-operator | openshift/certman-operator |
| cloud-ingress-operator | openshift/cloud-ingress-operator |
| configuration-anomaly-detection | openshift/configuration-anomaly-detection |
| configure-alertmanager-operator | openshift/configure-alertmanager-operator |
| deadmanssnitch-operator | openshift/deadmanssnitch-operator |
| gcp-project-operator | openshift/gcp-project-operator |
| managed-cluster-validating-webhooks | openshift/managed-cluster-validating-webhooks |
| managed-node-metadata-operator | openshift/managed-node-metadata-operator |
| managed-upgrade-operator | openshift/managed-upgrade-operator |
| managed-velero-operator | openshift/managed-velero-operator |
| ocm-agent | openshift/ocm-agent |
| ocm-agent-operator | openshift/ocm-agent-operator |
| osd-metrics-exporter | openshift/osd-metrics-exporter |
| pagerduty-operator | openshift/pagerduty-operator |
| rbac-permissions-operator | openshift/rbac-permissions-operator |
| route-monitor-operator | openshift/route-monitor-operator |
| splunk-forwarder-operator | openshift/splunk-forwarder-operator |

Process each repo in the list above. All repos have been verified to contain `boilerplate/update.cfg`.

## Procedure

### 1. For each operator — Run boilerplate update

For each operator in the Operator Registry:

1. **Clone the operator repo**: Clone from GitHub using the repo from the Operator Registry (e.g. `https://github.com/openshift/route-monitor-operator`). Ensure a clean checkout on the default branch (typically `master` or `main`).
   After cloning, add the bot's fork as the push remote. Use `priv_scm_ensure_fork` to create/discover the fork, then `git remote add fork <fork-url>`.

2. **Run `make boilerplate-update`**: This pulls the latest conventions from `openshift/boilerplate` into the repo. The script:
   - Clones `openshift/boilerplate.git` into a temporary directory
   - Reads `boilerplate/update.cfg` for subscribed conventions
   - Copies updated convention files into the repo
   - Requires a clean git checkout (will fail if the working tree is dirty)

3. **Check for changes**: Run `git status` to see if any files were modified.
   - If **no changes** → boilerplate is already up to date. Record the result:
     > ⏭️ **<operator-name>** — Boilerplate already up to date.
   - If `make boilerplate-update` **failed** → record the failure:
     > ❌ **<operator-name>** — `make boilerplate-update` failed: <error summary>
   - If **changes exist** → continue to step 4.

4. **Check for existing boilerplate PR**: Before committing, search for open PRs on the upstream repo with `boilerplate-update` in the branch name.
   - If an open PR exists **and was created within the last 7 days** → skip this operator. Record the result:
     > ⏭️ **<operator-name>** — Recent boilerplate update PR already open (<7 days): <PR link>
   - If an open PR exists **but is older than 7 days** → it is likely stale. Proceed to create a new update (the new PR supersedes the old one).
   - If no open PR exists → proceed normally.

5. **Run `make boilerplate-commit`**: This automatically:
   - Creates a branch named `boilerplate-update-<N>-<hash>` (where N = number of conventions, hash = boilerplate commit)
   - Stages all changes (`git add -A`)
   - Generates a commit message with:
     - Title: `Boilerplate: Update to <hash>`
     - Body: convention statuses (Subscribe/Update/No change per convention), compare URL, upstream commit log
   - Commits the changes

6. **Push the branch**: Push to the `fork` remote (the bot's fork). If the push fails, record the failure, increment the Failed count, and skip to the next operator — do not create the PR.

7. **Create a PR**: Create a pull request on the operator's upstream GitHub repo, with `head` set to the bot fork's branch:
   - **Title**: Use the commit title generated by `make boilerplate-commit` (e.g. `Boilerplate: Update to abc1234`)
   - **Target branch**: The repo's default branch
   - **Description**: Use the commit message body generated by `make boilerplate-commit`. It already contains the convention statuses and compare URL.

8. **Record the result**:
   > ✅ **<operator-name>** — Boilerplate updated. PR: <GitHub PR link>
   If this PR supersedes a stale PR (from step 4), also note:
   > ✅ **<operator-name>** — Boilerplate updated. PR: <GitHub PR link> (supersedes stale PR: <old PR link>)

### 2. Post report

After processing all operators, post the results to `#sre-operators` as a concise parent message with a detailed threaded reply.

#### Parent message (channel post)

Post a summary message to `#sre-operators`:

> 🔧 **Weekly SRE Operator Boilerplate Update — <today's date>**
> cc @osd-operators-saas-approver
>
> Boilerplate commit: `<hash>` ([compare](<compare URL>))
> PRs are ready for `/lgtm` + `/approve`.
>
> **Summary:**
> - ✅ Updated: <N> operators
> - ⏭️ Already up-to-date: <N> operators
> - ⏭️ Skipped (recent PR open): <N> operators
> - ❌ Failed: <N> operators
> - Total PRs created: <N>
> - Stale PRs superseded: <N>

Use `<!subteam^S0BLN6AN7EK>` to mention the @osd-operators-saas-approver group.

Include the "Stale PRs superseded" line only if the count is greater than 0.

#### Threaded reply (per-operator details)

Post a **single** threaded reply to the parent message containing all per-operator results collected during processing:

> ✅ **operator-a** — Boilerplate updated. PR: <link>
> ✅ **operator-b** — Boilerplate updated. PR: <link> (supersedes stale PR: <old link>)
> ⏭️ **operator-c** — Boilerplate already up to date.
> ⏭️ **operator-d** — Recent boilerplate update PR already open (<7 days): <link>
> ❌ **operator-e** — `make boilerplate-update` failed: <error summary>

List operators in the order they were processed.

## Error Handling

- If you cannot clone a repo → log the error in the thread and continue with the next operator
- If `make boilerplate-update` fails → log the error in the thread (include the error output) and continue
- If `make boilerplate-commit` fails → log the error in the thread and continue
- If PR creation fails → log the error in the thread with the specific failure reason and continue
- If the fork does not exist → use `priv_scm_ensure_fork` to create it, then retry the push
- **Never skip the Slack report** — always post the parent message and at least a summary, even if everything fails

## Technical Notes

- The `boilerplate/update` script clones `openshift/boilerplate.git` from GitHub. Ensure the workspace has network access to GitHub.
- The `boilerplate/update.cfg` file lists which conventions the repo subscribes to. The update script only processes subscribed conventions.
- `make boilerplate-commit` will exit with an error if there are no boilerplate-related changes. This is expected — treat it as "already up to date" rather than a failure.
- The branch name format `boilerplate-update-<N>-<hash>` includes the boilerplate commit hash, so each week's update gets a unique branch name (assuming boilerplate has new commits).
- This task creates PRs on GitHub repos under `openshift/*`. Each PR still requires human review and `/lgtm` + `/approve` to merge.
