# ROSA CI Credential Rotation Runbook

## Purpose

This runbook documents how to rotate the OCM service account, AWS access keys,
and GCP service-account key used by ROSA CI. It covers credential creation,
authorization, replication to Google Secret Manager (GSM) and Bitwarden,
validation, a 24-hour soak, and removal of the old credentials.

Do not store secret values, private keys, or credential files in this repository,
Jira, chat, shell history, or CI logs.

## Rotation principles

1. Create the replacement credential while the old credential remains valid.
2. Validate the replacement directly against its source system before updating
   any CI secret.
3. Update every GSM copy of the credential, then update its Bitwarden copy for
   future recovery and rotation.
4. Allow one to two hours for synchronized GSM secrets to reach CI clusters.
5. Run representative CI canaries and confirm that cloud resources are cleaned
   up successfully.
6. Keep the old credential during a 24-hour soak. Delete it only after confirming
   that it was not used during the entire soak period.
7. Record credential identifiers, timestamps, and validation URLs as evidence,
   but never record credential values.

## Generic GSM and Bitwarden procedure

Use this procedure for every credential described below.

### Prepare the values

1. Put each new secret value in a separate protected file outside the repository.
2. Restrict file permissions to the current user.
3. Use descriptive filenames that do not contain the secret value.
4. Ask a second person to verify the GSM collection, group, and field before the
   update.

### Update GSM

Run the secret-manager commands from the `openshift/release` repository. Use
`--from-file` so the value is not retained in shell history.

For multiline secrets such as `.awscred`, always use a file containing the
exact expected format and real newline characters. Do not use `--from-literal`
with escaped line breaks such as `\n`; the escapes may be stored literally or
otherwise fail to produce the line structure expected by the consumer.

```bash
alias sm='./hack/secret-manager.sh'

sm login
sm list --rover-group hp-sre-rosa-ci
sm describe -c '<collection>' '<group>/<field>'
sm update -c '<collection>' '<group>/<field>' \
  --from-file '/protected/path/to/new-value'
```

Record the GSM update time and the identifier of the replacement credential.
Do not paste the value into the rotation record.

### Update Bitwarden

Immediately after the GSM update:

1. Open the corresponding Bitwarden collection and item.
2. Replace the stored value with the same replacement credential written to GSM.
3. Preserve useful non-secret metadata, including the account or project,
   principal name, credential ID, creation time, and next rotation date.
4. Have a second person verify that GSM and Bitwarden refer to the same
   replacement credential.

GSM is the source used by CI. Bitwarden is the recoverable copy for authorized
operators; updating only one of them leaves the inventory inconsistent.

### Confirm propagation

GSM updates can take one to two hours to reach CI clusters. A successful
`sm update` only confirms that GSM accepted the new version; it does not prove
that a CI job has received it. Keep the old credential active until a job using
the synchronized secret succeeds.

## OCM service-account rotation

### 1. Create the replacement service account

1. Open the [Red Hat Hybrid Cloud Console service-account
   page](https://console.redhat.com/iam/service-accounts/).
2. Create a service account with a meaningful name that identifies ROSA CI and
   its purpose. Avoid reusing the old account's name without a rotation suffix or
   creation date.
3. Save the new client ID and client secret in protected local files.
4. Record the new service-account name and client ID in the rotation ticket. The
   client secret must not be recorded there.

### 2. Validate every OCM environment

Before creating the `ocm-resources` merge request, use the replacement service
account to authenticate to every OCM environment used by ROSA CI, including
integration, staging, and production. For each environment, perform a minimal
read-only API request in addition to login.

Do not proceed if authentication or required authorization fails in any
environment. Correct the service-account permissions first.

### 3. Update `ocm-resources`

Create an MR in `ocm-resources` that replaces the existing ROSA CI service
account ID:

```text
6cb51d52-ca24-41fb-b0fc-5acb21be09e4
```

with the replacement service-account ID. Include the environment login results
in the MR without including any secret. Wait for the MR to merge and for the
authorization change to take effect before changing CI secrets.

Replacing the old ID before GSM propagation can create a short authentication
gap. If `ocm-resources` supports overlapping entries, prefer authorizing both
service accounts during the rotation and remove the old ID in a follow-up MR
after the soak. Otherwise, perform the replacement in a coordinated maintenance
window and update GSM immediately after the MR takes effect.

### 4. Update every GSM copy

Update both `sso-client-id` and `sso-client-secret` in all of these groups in the
`rosa-e2e` collection:

| GSM group | Fields |
|---|---|
| `rosa-e2e-prow-ci-01` | `sso-client-id`, `sso-client-secret` |
| `rosa-e2e-prow-ci-02` | `sso-client-id`, `sso-client-secret` |
| `rosa-e2e-prow-ci-03` | `sso-client-id`, `sso-client-secret` |
| `rosa-e2e-prow-ci-gcp` | `sso-client-id`, `sso-client-secret` |
| `gap-analysis` | `sso-client-id`, `sso-client-secret` |

Use the same new client ID and secret in every group. Update the corresponding
Bitwarden records after the GSM updates.

### 5. Validate OCM consumers

After propagation:

1. Run a small lifecycle or authentication canary for each ROSA E2E profile.
2. Run or observe a gap-analysis job.
3. Observe at least one normal scheduled execution that uses the OCM client.
4. Check for login failures, authorization failures, and environment-specific
   permission errors.

## AWS credential rotation

### 1. Rotate the three `ci-runner` IAM users

For each of the three ROSA CI AWS accounts:

1. Confirm the AWS account ID and the `ci-runner` IAM user.
2. Create a new access key while retaining the old access key.
3. Validate the new key with `sts:GetCallerIdentity` and confirm that the
   returned account ID and ARN are correct.
4. Build the profile's `.awscred` file from the new key:

   ```ini
   [default]
   aws_access_key_id = <AWS_ACCESS_KEY_ID>
   aws_secret_access_key = <AWS_SECRET_ACCESS_KEY>
   aws_account_id = <AWS_ACCOUNT_ID>
   ```

5. Update the following fields for the corresponding profile group:

The profile groups below are in the `rosa-e2e` GSM collection.

| GSM group | Credential fields to update together |
|---|---|
| `rosa-e2e-prow-ci-01` | `--dot--awscred`, `aws_access_key_id`, `aws_secret_access_key` |
| `rosa-e2e-prow-ci-02` | `--dot--awscred`, `aws_access_key_id`, `aws_secret_access_key` |
| `rosa-e2e-prow-ci-03` | `--dot--awscred`, `aws_access_key_id`, `aws_secret_access_key` |

`--dot--awscred` is synchronized to CI as `.awscred`. Generate the file and the
component fields from the same access-key pair. Do not change `aws_account_id`
or `aws_billing_account` unless the underlying account or billing relationship
has changed.

Update the matching Bitwarden entries after each account's GSM update. Rotate
and validate one account at a time to limit the blast radius.

### 2. Rotate `osdCcsAdmin`

Create a new access key for the `osdCcsAdmin` IAM user and validate its identity
and account before updating CI. Update both CI representations from the same key:

Both groups below are in the `rosa-e2e` GSM collection.

| GSM group | Field |
|---|---|
| `rosa-e2e-prow-ci-01` | `osd-ccs-aws-credentials` |
| `rosa-e2e-osd-aws-ccs` | `credentials` |

Update the corresponding Bitwarden record as part of the same change. Do not
revoke the old key until both consumers have passed validation.

### 3. Update marketplace generator copies

Update every AWS credential copy associated with the
`rosa-marketplace-release-generator` collection. Inventory and verify the exact
GSM group and field names before changing them, because marketplace credentials
may be stored separately from the ROSA E2E cluster-profile bundles. Update the
matching Bitwarden collection entries with the same new credential.

Validate the marketplace consumer using a dry-run or read-only operation. Do
not publish or modify a live marketplace release merely to test a rotated key.

### 4. Validate AWS consumers

After each profile has propagated:

1. Run `sts:GetCallerIdentity` using the source credential file and confirm the
   expected account.
2. Trigger the smallest representative Prow lifecycle job for that profile.
3. Require successful provision, verification, deprovision, and cleanup.
4. Observe scheduled consumers and check for `InvalidClientTokenId`,
   `SignatureDoesNotMatch`, `AccessDenied`, or shared-credentials parsing errors.

Do not use a destructive cloud-cleanup job solely as a credential canary.

## GCP credential rotation

### 1. Create the replacement key

1. Log in to the Google Cloud console and select the `rosa-ci` project.
2. Open **IAM & Admin > Service Accounts**.
3. Select:

   ```text
   osd-ccs-admin@rosa-ci.iam.gserviceaccount.com
   ```

4. Create a new JSON key for this service account while keeping the old key.
5. Store the downloaded JSON file in a protected location and record only its
   key ID and creation time in the rotation ticket.
6. Validate that the JSON identifies the `rosa-ci` project and the expected
   service-account email, then perform a minimal read-only authenticated request.

### 2. Update every CI copy

Update `osd-ccs-gcp--dot--json` in both GSM groups:

Both groups below are in the `rosa-e2e` GSM collection.

| GSM group | Field |
|---|---|
| `rosa-e2e-prow-ci-03` | `osd-ccs-gcp--dot--json` |
| `rosa-e2e-prow-ci-gcp` | `osd-ccs-gcp--dot--json` |

The field is synchronized to CI as `osd-ccs-gcp.json`. Update the corresponding
Bitwarden entry with the same JSON key.

### 3. Validate GCP consumers

After propagation, validate both profile 03 and the dedicated GCP profile.
Exercise representative lease, lifecycle, deprovision, and cleanup paths. Check
for invalid-key, disabled-key, permission, quota-project, and wrong-project
errors.

## Twenty-four-hour soak and retirement

Start the soak only after all GSM copies have propagated and the required
canaries have succeeded. Record the soak start and end times explicitly.

During the next 24 hours:

1. Observe scheduled OCM, AWS, GCP, gap-analysis, OSD CCS, and marketplace
   consumers.
2. Check authentication and authorization failures against the rotation
   timeline.
3. Verify through the provider's audit or last-used information that none of the
   old credential IDs was used during the complete 24-hour window.
4. Account for logging and last-used-metadata delays. If the evidence is
   incomplete, extend the soak rather than assuming the key is unused.

After a clean soak:

1. Reconfirm the exact old service-account or key ID with a second person.
2. Delete or revoke the old OCM service account after confirming the replacement
   authorization is active.
3. Delete the old access key from each `ci-runner` and `osdCcsAdmin` IAM user.
4. Delete the old GCP service-account key.
5. Confirm that the replacement credentials still work and that scheduled jobs
   remain healthy.
6. Record deletion times and evidence in the rotation ticket.
7. Remove protected temporary files according to workstation policy.

## Rollback

If a replacement fails before the old credential is deleted:

1. Stop launching new jobs for the affected credential.
2. Keep or restore the old credential in GSM and Bitwarden.
3. Wait for GSM propagation.
4. Rerun the same canary and clean up resources created by failed jobs.
5. Correct permissions, secret format, or missing replicas before retrying.

If the old credential has already been deleted, create another replacement
credential rather than attempting to recover the deleted secret.

## Related documentation

- [`prow-ci-cluster-profile.md`](../prow-ci-cluster-profile.md) describes GSM access,
  synchronized cluster-profile bundles, AWS credential file format, and secret
  propagation.
- [`trigger-prow-ci-manually.md`](../trigger-prow-ci-manually.md) describes how to
  trigger CI jobs through the Gangway API.
