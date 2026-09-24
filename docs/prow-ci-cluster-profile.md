# ROSA E2E Prow CI Account Credentials

## Overview

This document describes how to configure and manage credentials for ROSA E2E cluster profiles used by OpenShift Prow CI.

It intentionally excludes cloud account identifiers, payer information, employee contact details, and links to internal systems. Follow the approved internal account-provisioning process before using this guide.

## Prepare the Cloud Account

Before adding the account to Prow CI:

1. Provision a dedicated cloud account through the approved internal process.
2. Create dedicated credentials with the permissions required by the ROSA E2E jobs.
3. Apply the required service-quota increases in every test region.
4. Create the Elastic Load Balancing service-linked role if it does not already exist:

   ```bash
   aws iam create-service-linked-role \
     --aws-service-name elasticloadbalancing.amazonaws.com
   ```

5. Complete any required ROSA subscription or staging-environment enablement.

See [Prepare the Cloud Account](https://docs.ci.openshift.org/how-tos/adding-a-cluster-profile/#prepare-the-cloud-account) for the general OpenShift CI requirements.

## Request AWS Service Quotas

Request the following quotas in every Region used by the ROSA E2E profile:

| AWS service | Quota | Quota code | Requested value |
|---|---|---|---:|
| Amazon EC2 | EC2-VPC Elastic IPs | `L-0263D0A3` | 500 |
| Amazon EC2 | Running On-Demand Standard instances (vCPUs) | `L-1216C47A` | 3700 |
| Amazon VPC | VPCs per Region | `L-F678F1CE` | 250 |
| Amazon VPC | NAT gateways per Availability Zone | `L-FE5A380F` | 200 |
| Amazon VPC | Internet gateways per Region | `L-A4707A72` | 200 |
| Elastic Load Balancing | Classic Load Balancers per Region | `L-E9E9831D` | 250 |

Configure an AWS CLI profile with permission to request quota increases, then run:

```bash
ROSA_AWS_PROFILE="<aws-cli-profile>"
ROSA_AWS_REGIONS=("us-east-1" "us-west-2" "us-west-1" "us-east-2")

for ROSA_AWS_REGION in "${ROSA_AWS_REGIONS[@]}"; do
  aws service-quotas request-service-quota-increase \
    --service-code ec2 --quota-code L-0263D0A3 --desired-value 500 \
    --region "${ROSA_AWS_REGION}" --profile "${ROSA_AWS_PROFILE}"
  aws service-quotas request-service-quota-increase \
    --service-code ec2 --quota-code L-1216C47A --desired-value 3700 \
    --region "${ROSA_AWS_REGION}" --profile "${ROSA_AWS_PROFILE}"
  aws service-quotas request-service-quota-increase \
    --service-code vpc --quota-code L-F678F1CE --desired-value 250 \
    --region "${ROSA_AWS_REGION}" --profile "${ROSA_AWS_PROFILE}"
  aws service-quotas request-service-quota-increase \
    --service-code vpc --quota-code L-FE5A380F --desired-value 200 \
    --region "${ROSA_AWS_REGION}" --profile "${ROSA_AWS_PROFILE}"
  aws service-quotas request-service-quota-increase \
    --service-code vpc --quota-code L-A4707A72 --desired-value 200 \
    --region "${ROSA_AWS_REGION}" --profile "${ROSA_AWS_PROFILE}"
  aws service-quotas request-service-quota-increase \
    --service-code elasticloadbalancing --quota-code L-E9E9831D \
    --desired-value 250 --region "${ROSA_AWS_REGION}" \
    --profile "${ROSA_AWS_PROFILE}"

  # Avoid exceeding the Service Quotas request rate.
  sleep 60
done
```

Quota requests are asynchronous. Check their status for each service and Region before using the account:

```bash
ROSA_AWS_REGION="<aws-region>"

aws service-quotas list-requested-service-quota-change-history \
  --service-code ec2 --region "${ROSA_AWS_REGION}" \
  --profile "${ROSA_AWS_PROFILE}"
aws service-quotas list-requested-service-quota-change-history \
  --service-code vpc --region "${ROSA_AWS_REGION}" \
  --profile "${ROSA_AWS_PROFILE}"
aws service-quotas list-requested-service-quota-change-history \
  --service-code elasticloadbalancing --region "${ROSA_AWS_REGION}" \
  --profile "${ROSA_AWS_PROFILE}"
```

For console-based requests and approval status, see [Amazon EC2 service quotas](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-resource-limits.html).

## Configure the Cluster Profile

Follow the [OpenShift CI cluster-profile documentation](https://docs.ci.openshift.org/how-tos/adding-a-cluster-profile/) to register the profile.

The profile entry in `ci-operator/step-registry/cluster-profiles/cluster-profiles-config.yaml` should reference a Kubernetes Secret named `cluster-secrets-<profile-name>`:

```yaml
- cluster_type: aws
  lease_type: <profile-name>-quota-slice
  name: <profile-name>
  owners:
  - org: openshift-online
  secret: cluster-secrets-<profile-name>
```

Configure Boskos consistently with the profile's `lease_type`, then run `make update` in the `openshift/release` repository and include the generated changes in the pull request.

## Configure the GSM Bundle

Add the profile's bundle to `core-services/ci-secret-bootstrap/gsm-config.yaml` in the `openshift/release` repository.

Use the `rosa-e2e` collection and an account-specific GSM group. The group name does not need to match the cluster profile name; the bundle explicitly maps it to the generated Kubernetes Secret. Follow an existing ROSA E2E bundle so the standard pull-secret registries are included. The account-specific portion has this form:

```yaml
gsm_secrets:
- collection: rosa-e2e
  group: <gsm-group-name>
name: cluster-secrets-<profile-name>
sync_to_cluster: true
targets:
- cluster_groups:
  - non_app_ci
  namespace: ci
```

`non_app_ci` synchronizes the Secret to the build clusters, `core-ci`, and `vsphere02`. Use the broader `user_secrets_target_clusters` group only if the Secret must also exist on `app.ci`, `hosted-mgmt`, or `hosted-mgmt2`.

Do not create `secretsync/target-name` or `secretsync/target-namespace` fields. The GSM bundle's `name` and `targets` define the generated Kubernetes Secret and its destination.

## Manage Credentials with the Secret Manager CLI

The supported interface is the Secret Manager CLI from the `openshift/release` repository. There is no GSM self-service web UI.

### Prerequisites

- A local clone of `openshift/release`
- Membership in the [`hp-sre-rosa-ci` Rover group](https://rover.redhat.com/groups/group/hp-sre-rosa-ci)
- The `gcloud` CLI
- Podman, or Docker with `CONTAINER_ENGINE=docker`

### Log In and Verify Access

From the root of the `openshift/release` repository:

```bash
alias sm='./hack/secret-manager.sh'
sm login
sm list --rover-group hp-sre-rosa-ci
sm list -c rosa-e2e
```

The `rosa-e2e` collection should appear in the Rover group output. If it does not, verify Rover membership and refresh the cached login:

```bash
sm clean
sm login
```

### Credential Layout

Use one GSM group per CI account. `gsm-config.yaml` maps the group to the generated cluster-profile Secret:

```text
rosa-e2e/                 # collection
  <gsm-group-name>/       # account-specific GSM group
    --dot--awscred        # field synchronized as .awscred
    sso-client-id
    sso-client-secret
    ssh-privatekey
    ssh-publickey
```

GSM field names cannot contain dots. The legacy `.awscred` key is stored as `--dot--awscred` and restored as `.awscred` in the generated cluster-profile Secret.

### Create Credentials

Set the collection and group:

```bash
ROSA_GSM_COLLECTION="rosa-e2e"
ROSA_GSM_GROUP="<gsm-group-name>"
```

Prepare each credential value in a separate local file. The AWS credentials file must use this format:

```ini
[default]
aws_access_key_id = <AWS_ACCESS_KEY_ID>
aws_secret_access_key = <AWS_SECRET_ACCESS_KEY>
aws_account_id = <AWS_ACCOUNT_ID>
```

Create each GSM field:

```bash
sm create -c "${ROSA_GSM_COLLECTION}" "${ROSA_GSM_GROUP}/--dot--awscred" \
  --from-file ./aws-credentials
sm create -c "${ROSA_GSM_COLLECTION}" "${ROSA_GSM_GROUP}/sso-client-id" \
  --from-file ./sso-client-id
sm create -c "${ROSA_GSM_COLLECTION}" "${ROSA_GSM_GROUP}/sso-client-secret" \
  --from-file ./sso-client-secret
sm create -c "${ROSA_GSM_COLLECTION}" "${ROSA_GSM_GROUP}/ssh-privatekey" \
  --from-file ./ssh-privatekey
sm create -c "${ROSA_GSM_COLLECTION}" "${ROSA_GSM_GROUP}/ssh-publickey" \
  --from-file ./ssh-publickey
```

`--from-literal` is also supported, but `--from-file` prevents credential values from being retained in shell history.

### List and Inspect Credentials

List field names in the collection:

```bash
sm list -c "${ROSA_GSM_COLLECTION}"
```

Inspect non-sensitive metadata for a field:

```bash
sm describe -c "${ROSA_GSM_COLLECTION}" \
  "${ROSA_GSM_GROUP}/sso-client-id"
```

Users cannot retrieve secret values from GSM. Only CI infrastructure can read values during job execution. If a value is lost, obtain it from its source system or rotate it and update GSM.

### Update Credentials

Use the same collection, group, and field with `sm update`:

```bash
sm update -c "${ROSA_GSM_COLLECTION}" "${ROSA_GSM_GROUP}/--dot--awscred" \
  --from-file ./aws-credentials
sm update -c "${ROSA_GSM_COLLECTION}" "${ROSA_GSM_GROUP}/sso-client-id" \
  --from-file ./sso-client-id
sm update -c "${ROSA_GSM_COLLECTION}" "${ROSA_GSM_GROUP}/sso-client-secret" \
  --from-file ./sso-client-secret
sm update -c "${ROSA_GSM_COLLECTION}" "${ROSA_GSM_GROUP}/ssh-privatekey" \
  --from-file ./ssh-privatekey
sm update -c "${ROSA_GSM_COLLECTION}" "${ROSA_GSM_GROUP}/ssh-publickey" \
  --from-file ./ssh-publickey
```

Updates to synchronized cluster-profile bundles can take one to two hours to reach CI clusters.

### Delete a Credential

```bash
sm delete -c "${ROSA_GSM_COLLECTION}" \
  "${ROSA_GSM_GROUP}/<field-name>"
```

Deleting a credential used by a cluster profile will break jobs that use the profile. Rotate credentials with `sm update` whenever possible.

## Existing Profile Mappings

The following mappings are visible in the public `openshift/release` configuration:

| Cluster profile | GSM collection/group | Generated Kubernetes Secret |
|---|---|---|
| `rosa-e2e-01` | `rosa-e2e/rosa-e2e-prow-ci-01` | `cluster-secrets-rosa-e2e-01` |
| `rosa-e2e-02` | `rosa-e2e/rosa-e2e-prow-ci-02` | `cluster-secrets-rosa-e2e-02` |
| `rosa-e2e-03` | `rosa-e2e/rosa-e2e-prow-ci-03` | `cluster-secrets-rosa-e2e-03` |
| `rosa-e2e-gcp` | `rosa-e2e/rosa-e2e-prow-ci-gcp` | `cluster-secrets-rosa-e2e-gcp` |

## References

- [Adding a New Secret to CI with GSM](https://docs.ci.openshift.org/how-tos/adding-a-new-secret-to-ci-gsm/)
- [Using the Secret Manager CLI](https://docs.ci.openshift.org/architecture/cli-secret-manager/)
- [Adding a Cluster Profile](https://docs.ci.openshift.org/how-tos/adding-a-cluster-profile/)
- [ROSA E2E GSM bundles](https://github.com/openshift/release/blob/main/core-services/ci-secret-bootstrap/gsm-config.yaml)
- [ROSA E2E cluster profiles](https://github.com/openshift/release/blob/main/ci-operator/step-registry/cluster-profiles/cluster-profiles-config.yaml)
