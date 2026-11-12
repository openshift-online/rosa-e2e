#!/bin/bash
# Provision a ROSA HCP cluster for e2e testing.
#
# Prerequisites:
#   - AWS credentials set (via osdctl or rh-aws-saml-login)
#   - OCM logged into staging: ocm login --use-auth-code --url stage
#   - rosa CLI installed
#
# Usage:
#   # Get AWS credentials via osdctl:
#   eval $(echo "y" | osdctl account cli -i <ACCOUNT_ID> -p osd-staging-2 -r <REGION> -oenv 2>/dev/null | tr '\n' ' ' | sed 's/.*AWS_ACCESS/AWS_ACCESS/')
#
#   # Or via rh-aws-saml-login (spawns subshell):
#   rh-aws-saml-login osd-staging-2 --region <REGION> --assume-uid <ACCOUNT_ID>
#
#   # Then run:
#   ./scripts/provision-e2e-cluster.sh

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-rosa-e2e-$(date +%m%d)}"
OCM_ENV="${OCM_ENV:-staging}"
REGION="${REGION:-${AWS_REGION:-us-east-2}}"
COMPUTE_NODES="${COMPUTE_NODES:-2}"
# Default node pool autoscaling (day-1 feature): set ENABLE_AUTOSCALING=true to
# create the cluster with the default pool autoscaling between MIN_REPLICAS and
# MAX_REPLICAS instead of a fixed COMPUTE_NODES count. Required by the
# Feature:Autoscaling spec, which validates scale-up/down on the default pool.
ENABLE_AUTOSCALING="${ENABLE_AUTOSCALING:-false}"
MIN_REPLICAS="${MIN_REPLICAS:-2}"
MAX_REPLICAS="${MAX_REPLICAS:-4}"
COMPUTE_MACHINE_TYPE="${COMPUTE_MACHINE_TYPE:-m5.xlarge}"
OIDC_CONFIG_ID="${OIDC_CONFIG_ID:-}"
CLUSTER_SECTOR="${CLUSTER_SECTOR:-}"
# External authentication (external OIDC / BYO identity provider) is a day-1, immutable HCP feature:
# it can only be enabled at cluster creation. Set EXTERNAL_AUTH_ENABLED=true to create a cluster that
# can later host external auth providers via ./scripts/create-external-auth-provider.sh.
EXTERNAL_AUTH_ENABLED="${EXTERNAL_AUTH_ENABLED:-false}"
# KMS encryption with a customer-managed key is a day-1 feature: EBS storage encryption can only be
# enabled at cluster creation. Set KMS_ENABLED=true to create a KMS key scoped to the cluster's
# account and operator roles (deleted by deprovision-e2e-cluster.sh), or pass KMS_KEY_ARN to reuse
# an existing key. Enables both EBS storage encryption and etcd encryption.
KMS_ENABLED="${KMS_ENABLED:-false}"
KMS_KEY_ARN="${KMS_KEY_ARN:-}"
# Shared VPC is a day-1 feature: the cluster's DNS records are served from a private
# hosted zone reachable through a dedicated IAM role (--route53-role-arn). Set
# SHARED_VPC_ENABLED=true to create the PHZ, shared role and policy, pre-create the
# operator roles (so the ingress-operator role can be granted the shared role), and
# create the cluster with the shared VPC flags. Required by the Feature:SharedVPC
# specs. Locally everything is created in your own AWS account (CI uses a second
# account); pass SHARED_VPC_BASE_DOMAIN to reuse an existing rosa dnsdomain.
SHARED_VPC_ENABLED="${SHARED_VPC_ENABLED:-false}"
SHARED_VPC_BASE_DOMAIN="${SHARED_VPC_BASE_DOMAIN:-}"

# AWS payer/billing account per OCM environment
# integration/staging: 277304166082 (osd-staging-1) or 811685182089 (osd-staging-2)
# production: 922711891673 (rhcontrol)
case "${OCM_ENV}" in
  production|prod) BILLING_ACCOUNT="${BILLING_ACCOUNT:-922711891673}" ;;
  *)               BILLING_ACCOUNT="${BILLING_ACCOUNT:-811685182089}" ;;
esac

echo "=== ROSA E2E Cluster Provisioning ==="
echo "Cluster name: ${CLUSTER_NAME}"
echo "Region: ${REGION}"
echo "Compute: ${COMPUTE_NODES}x ${COMPUTE_MACHINE_TYPE}"
echo "External auth: ${EXTERNAL_AUTH_ENABLED}"
echo "KMS encryption: ${KMS_ENABLED}"
echo "Shared VPC: ${SHARED_VPC_ENABLED}"
echo ""

# Verify prerequisites
echo "--- Checking prerequisites ---"

if ! aws sts get-caller-identity &>/dev/null; then
  echo "ERROR: AWS credentials not set."
  echo "Use: eval \$(echo 'y' | osdctl account cli -i <ACCOUNT_ID> -p osd-staging-2 -r ${REGION} -oenv 2>/dev/null | tr '\\n' ' ' | sed 's/.*AWS_ACCESS/AWS_ACCESS/')"
  exit 1
fi
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
echo "AWS Account: ${AWS_ACCOUNT_ID}"

if ! ocm whoami &>/dev/null; then
  echo "ERROR: OCM not logged in. Run: ocm login --use-auth-code --url ${OCM_ENV}"
  exit 1
fi
CURRENT_OCM=$(ocm config get url)
echo "OCM: ${CURRENT_OCM}"

# Step 1: Create account roles if needed
echo ""
echo "--- Checking HCP account roles ---"
if ! rosa list account-roles 2>/dev/null | grep -q 'HCP-ROSA'; then
  echo "Creating HCP account roles..."
  rosa create account-roles --hosted-cp --mode auto --yes
else
  echo "HCP account roles exist"
fi

# Step 2: Get or create OIDC config
# Prefer a managed OIDC config: unmanaged configs store their private key in AWS
# Secrets Manager, and a deleted/inaccessible secret leaves a dangling config that
# breaks cluster creation with "Failed to get secret value from '<arn>'".
# When operator roles for this cluster already exist (shared VPC re-runs
# pre-create them), pick the config whose issuer they trust instead of the
# first managed config: rosa create cluster rejects operator roles whose trust
# policy names a different issuer.
OIDC_CONFIG_CREATED="false"
if [[ -z "${OIDC_CONFIG_ID}" ]]; then
  echo ""
  echo "--- Getting OIDC config ---"
  EXISTING_INGRESS_ROLE_NAME=$(aws iam list-roles \
    --query "Roles[?starts_with(RoleName,'${CLUSTER_NAME}-openshift-ingress-operator')].RoleName" \
    --output text | awk '{print $1}')
  if [[ -n "${EXISTING_INGRESS_ROLE_NAME}" ]]; then
    TRUSTED_OIDC_CONFIG_ID=$(aws iam get-role --role-name "${EXISTING_INGRESS_ROLE_NAME}" \
      --query 'Role.AssumeRolePolicyDocument.Statement[0].Principal.Federated' \
      --output text 2>/dev/null | grep -oE '[A-Za-z0-9]+$' || true)
    if [[ -n "${TRUSTED_OIDC_CONFIG_ID}" ]]; then
      OIDC_CONFIG_ID=$(rosa list oidc-config -o json 2>/dev/null |
        jq -r --arg id "${TRUSTED_OIDC_CONFIG_ID}" '[.[] | select(.id == $id)][0].id // empty')
      if [[ -n "${OIDC_CONFIG_ID}" ]]; then
        echo "Using OIDC config ${OIDC_CONFIG_ID} trusted by the existing operator roles"
      fi
    fi
  fi
  if [[ -z "${OIDC_CONFIG_ID}" ]]; then
    OIDC_CONFIG_ID=$(rosa list oidc-config -o json 2>/dev/null | jq -r '[.[] | select(.managed==true)][0].id // empty')
  fi
  if [[ -z "${OIDC_CONFIG_ID}" ]]; then
    echo "Creating managed OIDC config..."
    OIDC_CONFIG_ID=$(rosa create oidc-config --managed --mode auto --yes -o json | jq -r '.id')
    OIDC_CONFIG_CREATED="true"
  fi
fi
echo "OIDC Config: ${OIDC_CONFIG_ID}"

# Step 3: Create VPC using ROSA network template (adopt an existing stack on re-run)
echo ""
echo "--- Creating VPC ---"
STACK_NAME="${CLUSTER_NAME}-vpc"

if ! aws cloudformation describe-stacks --stack-name "${STACK_NAME}" --region "${REGION}" &>/dev/null; then
  rosa create network rosa-quickstart-default-vpc \
    --mode auto \
    --param Region="${REGION}" \
    --param Name="${STACK_NAME}" \
    --param AvailabilityZoneCount=2 \
    --param VpcCidr=10.0.0.0/16 \
    --yes
else
  echo "VPC stack ${STACK_NAME} already exists, adopting it"
fi

# Wait for stack creation
echo "Waiting for VPC stack creation..."
aws cloudformation wait stack-create-complete \
  --stack-name "${STACK_NAME}" \
  --region "${REGION}"

# Get VPC ID and subnet IDs from stack outputs
VPC_ID=$(aws cloudformation describe-stacks \
  --stack-name "${STACK_NAME}" \
  --region "${REGION}" \
  --query 'Stacks[0].Outputs[?OutputKey==`VPCId`].OutputValue' \
  --output text)

# Get subnet IDs (public and private)
PUBLIC_SUBNET_IDS=$(aws cloudformation describe-stacks \
  --stack-name "${STACK_NAME}" \
  --region "${REGION}" \
  --query 'Stacks[0].Outputs[?OutputKey==`PublicSubnets`].OutputValue' \
  --output text)

PRIVATE_SUBNET_IDS=$(aws cloudformation describe-stacks \
  --stack-name "${STACK_NAME}" \
  --region "${REGION}" \
  --query 'Stacks[0].Outputs[?OutputKey==`PrivateSubnets`].OutputValue' \
  --output text)

SUBNET_IDS="${PUBLIC_SUBNET_IDS},${PRIVATE_SUBNET_IDS}"

echo "VPC: ${VPC_ID}"
echo "Stack: ${STACK_NAME}"

# Step 4: Resolve sector if specified
SECTOR_ARGS=""
if [[ -n "${CLUSTER_SECTOR}" ]]; then
  echo ""
  echo "--- Resolving provision shard for sector: ${CLUSTER_SECTOR} ---"
  PS_ID=$(ocm get /api/osd_fleet_mgmt/v1/service_clusters \
    --parameter search="sector is '${CLUSTER_SECTOR}' and region is '${REGION}' and status in ('ready')" \
    | jq -r '.items[].provision_shard_reference.id' | head -1)
  if [[ -z "${PS_ID}" ]]; then
    echo "ERROR: No provision shard found for sector ${CLUSTER_SECTOR} in ${REGION}"
    exit 1
  fi
  echo "Provision shard: ${PS_ID}"
  SECTOR_ARGS="--properties provision_shard_id:${PS_ID}"
fi

# Step 4b: Enable external authentication if requested (day-1, immutable)
EXTERNAL_AUTH_ARGS=""
if [[ "${EXTERNAL_AUTH_ENABLED}" == "true" ]]; then
  echo ""
  echo "--- External authentication enabled (providers configured post-install) ---"
  EXTERNAL_AUTH_ARGS="--external-auth-providers-enabled"
fi

# Step 4c: Create the KMS key if requested (day-1, immutable)
# Key policy mirrors the rosa-sts-kms-key-create CI step: role ARNs as explicit principals
# (a role ARN principal in a KMS key policy also grants the role's assumed-role sessions).
# The cluster's operator roles are created by `rosa create cluster --mode auto` with the
# cluster-name prefix and are granted access after cluster creation, below.
KMS_ARGS=""
KMS_KEY_CREATED="false"
KMS_POLICY_FILE="${SHARED_DIR:-/tmp}/rosa-e2e-kms-policy.json"

write_kms_policy() {
  local principal_list
  principal_list=$(printf '"%s"\n' $1 | paste -sd, -)
  cat > "${KMS_POLICY_FILE}" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "EnableIAMPermissions",
      "Effect": "Allow",
      "Principal": { "AWS": "arn:aws:iam::${AWS_ACCOUNT_ID}:root" },
      "Action": "kms:*",
      "Resource": "*"
    },
    {
      "Sid": "AllowClusterRolesUse",
      "Effect": "Allow",
      "Principal": { "AWS": [${principal_list}] },
      "Action": [
        "kms:Encrypt", "kms:Decrypt", "kms:ReEncrypt*", "kms:GenerateDataKey*",
        "kms:DescribeKey"
      ],
      "Resource": "*"
    },
    {
      "Sid": "AllowClusterRolesGrants",
      "Effect": "Allow",
      "Principal": { "AWS": [${principal_list}] },
      "Action": ["kms:CreateGrant", "kms:ListGrants", "kms:RevokeGrant"],
      "Resource": "*",
      "Condition": { "Bool": { "kms:GrantIsForAWSResource": "true" } }
    }
  ]
}
EOF
}

if [[ "${KMS_ENABLED}" == "true" ]]; then
  if [[ -z "${KMS_KEY_ARN}" ]]; then
    echo ""
    echo "--- Creating KMS key ---"
    ACCOUNT_ROLE_ARNS=$(rosa list account-roles -o json |
      jq -r '[.[] | select(.RoleName | startswith("ManagedOpenShift-HCP-")) | .RoleARN] | join(" ")')
    if [[ -z "${ACCOUNT_ROLE_ARNS}" ]]; then
      echo "ERROR: No HCP account roles found."
      exit 1
    fi
    write_kms_policy "${ACCOUNT_ROLE_ARNS}"
    KMS_KEY_ARN=$(aws kms create-key \
      --region "${REGION}" \
      --description "rosa-e2e KMS test key for ${CLUSTER_NAME}" \
      --policy "file://${KMS_POLICY_FILE}" \
      --query 'KeyMetadata.Arn' \
      --output text)
    KMS_KEY_CREATED="true"
  fi
  echo "KMS key: ${KMS_KEY_ARN}"
  KMS_ARGS="--enable-customer-managed-key --kms-key-arn ${KMS_KEY_ARN} --etcd-encryption-kms-arn ${KMS_KEY_ARN}"
fi

# Step 4d: Create the shared VPC DNS infrastructure if requested (day-1, immutable)
# Mirrors the CI steps aws-provision-route53-private-hosted-zone (ENABLE_SHARED_PHZ)
# and rosa-conf-shared-vpc-update-trust-policy: a private hosted zone named
# <cluster>.<dnsdomain> associated with the cluster VPC, plus a policy and role the
# installer and ingress operator assume to manage records in it. Locally these live
# in the same AWS account (CI provisions them in a second account).
SHARED_VPC_ARGS=""
SHARED_VPC_PHZ_ID=""
SHARED_VPC_INTERNAL_PHZ_ID=""
SHARED_VPC_ROLE_ARN=""
SHARED_VPC_POLICY_ARN=""
SHARED_VPC_VPC_ENDPOINT_ROLE_ARN=""
SHARED_VPC_VPC_ENDPOINT_POLICY_ARN=""
SHARED_VPC_INSTALLER_ROLE_ARN=""
SHARED_VPC_OPERATOR_ROLES_CREATED="false"

if [[ "${SHARED_VPC_ENABLED}" == "true" ]]; then
  echo ""
  echo "--- Shared VPC enabled: creating DNS infrastructure ---"

  # Base domain: an unreserved HCP dnsdomain owned by the org (or create one)
  if [[ -z "${SHARED_VPC_BASE_DOMAIN}" ]]; then
    SHARED_VPC_BASE_DOMAIN=$(rosa list dnsdomain -o json 2>/dev/null |
      jq -r '[.[] | select(.cluster_arch=="hcp")][0].id // empty')
    if [[ -z "${SHARED_VPC_BASE_DOMAIN}" ]]; then
      echo "No unreserved HCP dnsdomain found, creating one..."
      SHARED_VPC_BASE_DOMAIN=$(rosa create dnsdomain 2>/dev/null | grep -oE "[0-9A-Za-z]{4}\.[0-9A-Za-z.]+")
    fi
  fi
  if [[ -z "${SHARED_VPC_BASE_DOMAIN}" ]]; then
    echo "ERROR: could not resolve a base domain for the shared VPC cluster"
    exit 1
  fi
  echo "Base domain: ${SHARED_VPC_BASE_DOMAIN}"

  # The installer account role (ManagedOpenShift-HCP-ROSA-Installer-Role) the
  # cluster will be created with; also determines the operator policy set.
  SHARED_VPC_INSTALLER_ROLE_ARN=$(rosa list account-roles -o json 2>/dev/null |
    jq -r '[.[] | select(.RoleName | contains("HCP-ROSA") and contains("Installer-Role"))][0].RoleARN // empty')
  if [[ -z "${SHARED_VPC_INSTALLER_ROLE_ARN}" ]]; then
    echo "ERROR: no HCP-ROSA installer account role found (run rosa create account-roles --hosted-cp first)"
    exit 1
  fi

  # Pre-create the operator roles so the ingress-operator role exists before the
  # shared role's trust policy is written (mirrors the rosa-sts-oidc-config-create
  # chain in CI; rosa create cluster reuses them via --operator-roles-prefix).
  if ! aws iam list-roles --query "Roles[?starts_with(RoleName,'${CLUSTER_NAME}-openshift-ingress-operator')].Arn" --output text | grep -q arn; then
    echo "Creating operator roles with prefix ${CLUSTER_NAME}..."
    rosa create operator-roles \
      --hosted-cp \
      --prefix "${CLUSTER_NAME}" \
      --oidc-config-id "${OIDC_CONFIG_ID}" \
      --role-arn "${SHARED_VPC_INSTALLER_ROLE_ARN}" \
      --mode auto \
      --yes
    SHARED_VPC_OPERATOR_ROLES_CREATED="true"
  else
    echo "Operator roles with prefix ${CLUSTER_NAME} exist"
  fi
  # rosa generates the ingress-operator role name from the prefix; resolve the
  # actual ARN (AWS rejects unresolvable principals in the trust policy below).
  INGRESS_OPERATOR_ROLE_ARN=$(aws iam list-roles \
    --query "Roles[?starts_with(RoleName,'${CLUSTER_NAME}-openshift-ingress-operator')].Arn" \
    --output text | awk '{print $1}')
  if [[ -z "${INGRESS_OPERATOR_ROLE_ARN}" ]]; then
    echo "ERROR: could not resolve the ingress-operator role ARN"
    exit 1
  fi
  # The control-plane operator role is the third principal in the HCP shared
  # role trust policies (see rosa's PrepareSharedRoute53RoleForHostedCP).
  CONTROL_PLANE_OPERATOR_ROLE_ARN=$(aws iam list-roles \
    --query "Roles[?starts_with(RoleName,'${CLUSTER_NAME}-kube-system-control-plane-operator')].Arn" \
    --output text | awk '{print $1}')
  if [[ -z "${CONTROL_PLANE_OPERATOR_ROLE_ARN}" ]]; then
    echo "ERROR: could not resolve the control-plane-operator role ARN"
    exit 1
  fi

  # Reuse an existing zone only when it is private and associated with the
  # cluster VPC: a name match alone could pick a public zone or a zone
  # associated with another VPC, and OCM requires the VPC association.
  find_reusable_zone() {
    local zone_name="$1" zone_id
    for zone_id in $(aws --region "${REGION}" route53 list-hosted-zones-by-name \
      --dns-name "${zone_name}" --output json |
      jq -r --arg name "${zone_name}." \
        '.HostedZones[] | select(.Name == $name and .Config.PrivateZone == true) | .Id' |
      awk -F / '{print $NF}'); do
      if aws --region "${REGION}" route53 get-hosted-zone --id "${zone_id}" --output json |
        jq -e --arg vpc "${VPC_ID}" 'any(.VPCs[]?; .VPCId == $vpc)' >/dev/null; then
        echo "${zone_id}"
        return
      fi
    done
  }

  # Private hosted zone for the cluster's records, associated with the VPC
  # (reuse an existing zone on re-run). HCP requires the 'rosa.' prefix:
  # OCM validates the zone name against 'cluster_domain_prefix.base_domain'
  # and the ingress records live under rosa.<cluster>.<base-domain>.
  PHZ_NAME="rosa.${CLUSTER_NAME}.${SHARED_VPC_BASE_DOMAIN}"
  SHARED_VPC_PHZ_ID=$(find_reusable_zone "${PHZ_NAME}")
  if [[ -z "${SHARED_VPC_PHZ_ID}" ]]; then
    HOSTED_ZONE_CREATION=$(aws --region "${REGION}" route53 create-hosted-zone \
      --name "${PHZ_NAME}" \
      --vpc "VPCRegion=${REGION},VPCId=${VPC_ID}" \
      --caller-reference "${PHZ_NAME}-$(date +%s)")
    SHARED_VPC_PHZ_ID=$(jq -r '.HostedZone.Id' <<<"${HOSTED_ZONE_CREATION}" | awk -F / '{printf $3}')
  fi
  echo "Private hosted zone: ${SHARED_VPC_PHZ_ID} (${PHZ_NAME})"

  # Shared policy (route53 + ec2:DeleteTags on the VPC, mirrors the CI step;
  # reuse an existing policy on re-run)
  SHARED_VPC_POLICY_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:policy/${CLUSTER_NAME}-shared-policy"
  if ! aws iam get-policy --policy-arn "${SHARED_VPC_POLICY_ARN}" &>/dev/null; then
    SHARED_VPC_POLICY_ARN=$(aws --region "${REGION}" iam create-policy \
      --policy-name "${CLUSTER_NAME}-shared-policy" \
      --policy-document "{
        \"Version\": \"2012-10-17\",
        \"Statement\": [
          {
            \"Effect\": \"Allow\",
            \"Action\": \"ec2:DeleteTags\",
            \"Resource\": \"arn:aws:ec2:${REGION}:${AWS_ACCOUNT_ID}:vpc/${VPC_ID}\"
          },
          {
            \"Effect\": \"Allow\",
            \"Action\": [
              \"route53:ChangeResourceRecordSets\", \"route53:ListHostedZones\",
              \"route53:ListHostedZonesByName\", \"route53:ListResourceRecordSets\",
              \"route53:ChangeTagsForResource\", \"route53:GetAccountLimit\",
              \"route53:GetChange\", \"route53:GetHostedZone\",
              \"route53:ListTagsForResource\", \"route53:UpdateHostedZoneComment\",
              \"tag:GetResources\", \"tag:UntagResources\"
            ],
            \"Resource\": \"*\"
          }
        ]
      }" --query 'Policy.Arn' --output text)
  fi
  echo "Shared policy: ${SHARED_VPC_POLICY_ARN}"

  # Shared (route53) role trusting the installer, ingress-operator and
  # control-plane-operator roles (reuse and re-trust an existing role on re-run)
  if ! aws iam get-role --role-name "${CLUSTER_NAME}-shared-role" &>/dev/null; then
    SHARED_VPC_ROLE_ARN=$(aws --region "${REGION}" iam create-role \
      --role-name "${CLUSTER_NAME}-shared-role" \
      --assume-role-policy-document "{
        \"Version\": \"2012-10-17\",
        \"Statement\": [
          {
            \"Effect\": \"Allow\",
            \"Principal\": {\"AWS\": [\"${SHARED_VPC_INSTALLER_ROLE_ARN}\", \"${INGRESS_OPERATOR_ROLE_ARN}\", \"${CONTROL_PLANE_OPERATOR_ROLE_ARN}\"]},
            \"Action\": \"sts:AssumeRole\",
            \"Condition\": {}
          }
        ]
      }" --query 'Role.Arn' --output text)
  else
    SHARED_VPC_ROLE_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:role/${CLUSTER_NAME}-shared-role"
  fi
  aws --region "${REGION}" iam update-assume-role-policy \
    --role-name "${CLUSTER_NAME}-shared-role" \
    --policy-document "{
      \"Version\": \"2012-10-17\",
      \"Statement\": [
        {
          \"Effect\": \"Allow\",
          \"Principal\": {\"AWS\": [\"${SHARED_VPC_INSTALLER_ROLE_ARN}\", \"${INGRESS_OPERATOR_ROLE_ARN}\", \"${CONTROL_PLANE_OPERATOR_ROLE_ARN}\"]},
          \"Action\": \"sts:AssumeRole\",
          \"Condition\": {}
        }
      ]
    }"
  aws --region "${REGION}" iam attach-role-policy \
    --role-name "${CLUSTER_NAME}-shared-role" \
    --policy-arn "${SHARED_VPC_POLICY_ARN}"
  echo "Shared role: ${SHARED_VPC_ROLE_ARN}"

  # Internal-communication PHZ (<cluster>.hypershift.local) associated with the
  # VPC, required by --hcp-internal-communication-hosted-zone-id (reuse on re-run)
  INTERNAL_PHZ_NAME="${CLUSTER_NAME}.hypershift.local"
  SHARED_VPC_INTERNAL_PHZ_ID=$(find_reusable_zone "${INTERNAL_PHZ_NAME}")
  if [[ -z "${SHARED_VPC_INTERNAL_PHZ_ID}" ]]; then
    INTERNAL_ZONE_CREATION=$(aws --region "${REGION}" route53 create-hosted-zone \
      --name "${INTERNAL_PHZ_NAME}" \
      --vpc "VPCRegion=${REGION},VPCId=${VPC_ID}" \
      --caller-reference "${INTERNAL_PHZ_NAME}-$(date +%s)")
    SHARED_VPC_INTERNAL_PHZ_ID=$(jq -r '.HostedZone.Id' <<<"${INTERNAL_ZONE_CREATION}" | awk -F / '{printf $3}')
  fi
  echo "Internal communication hosted zone: ${SHARED_VPC_INTERNAL_PHZ_ID} (${INTERNAL_PHZ_NAME})"

  # VPC endpoint policy + role: grants the ec2 VPC-endpoint / security-group and
  # route53 record permissions the control plane needs to wire up the
  # cross-account VPC (mirrors rosa's PrepareSharedVPCEndPointRoleForHostedCP)
  SHARED_VPC_VPC_ENDPOINT_POLICY_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:policy/${CLUSTER_NAME}-sharedvpc-vpc-endpoint-policy"
  if ! aws iam get-policy --policy-arn "${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN}" &>/dev/null; then
    SHARED_VPC_VPC_ENDPOINT_POLICY_ARN=$(aws --region "${REGION}" iam create-policy \
      --policy-name "${CLUSTER_NAME}-sharedvpc-vpc-endpoint-policy" \
      --policy-document "{
        \"Version\": \"2012-10-17\",
        \"Statement\": [
          {
            \"Effect\": \"Allow\",
            \"Action\": [
              \"ec2:CreateVpcEndpoint\", \"ec2:DescribeVpcEndpoints\", \"ec2:ModifyVpcEndpoint\",
              \"ec2:DeleteVpcEndpoints\", \"ec2:CreateTags\", \"ec2:CreateSecurityGroup\",
              \"ec2:AuthorizeSecurityGroupIngress\", \"ec2:AuthorizeSecurityGroupEgress\",
              \"ec2:DeleteSecurityGroup\", \"ec2:RevokeSecurityGroupIngress\",
              \"ec2:RevokeSecurityGroupEgress\", \"ec2:DescribeSecurityGroups\",
              \"ec2:DescribeVpcs\", \"route53:ListHostedZones\",
              \"route53:ChangeResourceRecordSets\", \"route53:ListResourceRecordSets\"
            ],
            \"Resource\": \"*\"
          }
        ]
      }" --query 'Policy.Arn' --output text)
  fi
  echo "VPC endpoint policy: ${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN}"

  if ! aws iam get-role --role-name "${CLUSTER_NAME}-shared-vpcendpoint-role" &>/dev/null; then
    SHARED_VPC_VPC_ENDPOINT_ROLE_ARN=$(aws --region "${REGION}" iam create-role \
      --role-name "${CLUSTER_NAME}-shared-vpcendpoint-role" \
      --assume-role-policy-document "{
        \"Version\": \"2012-10-17\",
        \"Statement\": [
          {
            \"Effect\": \"Allow\",
            \"Principal\": {\"AWS\": [\"${SHARED_VPC_INSTALLER_ROLE_ARN}\", \"${CONTROL_PLANE_OPERATOR_ROLE_ARN}\"]},
            \"Action\": \"sts:AssumeRole\",
            \"Condition\": {}
          }
        ]
      }" --query 'Role.Arn' --output text)
  else
    SHARED_VPC_VPC_ENDPOINT_ROLE_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:role/${CLUSTER_NAME}-shared-vpcendpoint-role"
  fi
  aws --region "${REGION}" iam update-assume-role-policy \
    --role-name "${CLUSTER_NAME}-shared-vpcendpoint-role" \
    --policy-document "{
      \"Version\": \"2012-10-17\",
      \"Statement\": [
        {
          \"Effect\": \"Allow\",
          \"Principal\": {\"AWS\": [\"${SHARED_VPC_INSTALLER_ROLE_ARN}\", \"${CONTROL_PLANE_OPERATOR_ROLE_ARN}\"]},
          \"Action\": \"sts:AssumeRole\",
          \"Condition\": {}
        }
      ]
    }"
  aws --region "${REGION}" iam attach-role-policy \
    --role-name "${CLUSTER_NAME}-shared-vpcendpoint-role" \
    --policy-arn "${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN}"
  echo "VPC endpoint role: ${SHARED_VPC_VPC_ENDPOINT_ROLE_ARN}"

  # Assume-role permission policies: IAM needs both the trust on the shared
  # roles and an sts:AssumeRole grant on the assuming side. rosa create
  # account-roles/operator-roles attaches these when run with the shared VPC
  # flags; with pre-existing org account roles we attach them directly
  # (naming mirrors rosa's AssumeRolePolicyPrefix: <role-name>-assume-role).
  ensure_assume_role_policy() {
    local target_role_arn="$1" shared_role_arn="$2"
    local target_role_name="${target_role_arn##*/}"
    local shared_role_name="${shared_role_arn##*/}"
    local policy_arn="arn:aws:iam::${AWS_ACCOUNT_ID}:policy/${shared_role_name}-assume-role"
    if ! aws iam get-policy --policy-arn "${policy_arn}" &>/dev/null; then
      policy_arn=$(aws --region "${REGION}" iam create-policy \
        --policy-name "${shared_role_name}-assume-role" \
        --policy-document "{
          \"Version\": \"2012-10-17\",
          \"Statement\": {
            \"Effect\": \"Allow\",
            \"Action\": \"sts:AssumeRole\",
            \"Resource\": \"${shared_role_arn}\"
          }
        }" --query 'Policy.Arn' --output text)
    fi
    aws --region "${REGION}" iam attach-role-policy \
      --role-name "${target_role_name}" \
      --policy-arn "${policy_arn}"
  }
  for principal in "${SHARED_VPC_INSTALLER_ROLE_ARN}" "${INGRESS_OPERATOR_ROLE_ARN}" "${CONTROL_PLANE_OPERATOR_ROLE_ARN}"; do
    ensure_assume_role_policy "${principal}" "${SHARED_VPC_ROLE_ARN}"
  done
  for principal in "${SHARED_VPC_INSTALLER_ROLE_ARN}" "${CONTROL_PLANE_OPERATOR_ROLE_ARN}"; do
    ensure_assume_role_policy "${principal}" "${SHARED_VPC_VPC_ENDPOINT_ROLE_ARN}"
  done

  SHARED_VPC_ARGS="--base-domain ${SHARED_VPC_BASE_DOMAIN} --ingress-private-hosted-zone-id ${SHARED_VPC_PHZ_ID} --hcp-internal-communication-hosted-zone-id ${SHARED_VPC_INTERNAL_PHZ_ID} --route53-role-arn ${SHARED_VPC_ROLE_ARN} --vpc-endpoint-role-arn ${SHARED_VPC_VPC_ENDPOINT_ROLE_ARN} --additional-allowed-principals ${SHARED_VPC_ROLE_ARN},${SHARED_VPC_VPC_ENDPOINT_ROLE_ARN} --operator-roles-prefix ${CLUSTER_NAME} --role-arn ${SHARED_VPC_INSTALLER_ROLE_ARN}"
  # Passing --role-arn explicitly makes rosa ask for the full account-role set;
  # resolve the matching support and worker roles so creation stays non-interactive.
  SHARED_VPC_SUPPORT_ROLE_ARN=$(rosa list account-roles -o json 2>/dev/null |
    jq -r '[.[] | select(.RoleName | contains("HCP-ROSA") and contains("Support-Role"))][0].RoleARN // empty')
  SHARED_VPC_WORKER_ROLE_ARN=$(rosa list account-roles -o json 2>/dev/null |
    jq -r '[.[] | select(.RoleName | contains("HCP-ROSA") and contains("Worker-Role"))][0].RoleARN // empty')
  if [[ -n "${SHARED_VPC_SUPPORT_ROLE_ARN}" ]]; then
    SHARED_VPC_ARGS+=" --support-role-arn ${SHARED_VPC_SUPPORT_ROLE_ARN}"
  fi
  if [[ -n "${SHARED_VPC_WORKER_ROLE_ARN}" ]]; then
    SHARED_VPC_ARGS+=" --worker-iam-role-arn ${SHARED_VPC_WORKER_ROLE_ARN}"
  fi
fi

# Step 5: Create cluster
echo ""
echo "--- Creating ROSA HCP cluster ---"
COMPUTE_NODES_ARGS="--replicas ${COMPUTE_NODES}"
if [[ "${ENABLE_AUTOSCALING}" == "true" ]]; then
  COMPUTE_NODES_ARGS="--enable-autoscaling --min-replicas ${MIN_REPLICAS} --max-replicas ${MAX_REPLICAS}"
fi
rosa create cluster \
  --cluster-name "${CLUSTER_NAME}" \
  --hosted-cp \
  --sts \
  --mode auto \
  --region "${REGION}" \
  ${COMPUTE_NODES_ARGS} \
  --compute-machine-type "${COMPUTE_MACHINE_TYPE}" \
  --billing-account "${BILLING_ACCOUNT}" \
  --oidc-config-id "${OIDC_CONFIG_ID}" \
  --subnet-ids "${SUBNET_IDS}" \
  ${SECTOR_ARGS} \
  ${EXTERNAL_AUTH_ARGS} \
  ${KMS_ARGS} \
  ${SHARED_VPC_ARGS} \
  --yes

CLUSTER_ID=$(rosa describe cluster -c "${CLUSTER_NAME}" -o json | jq -r '.id')

# Grant the cluster's operator roles access to the KMS key: `rosa create cluster --mode auto`
# creates them during cluster creation. Use the key ID rather than the ARN: older AWS CLI
# builds reject KMS key ARNs here.
if [[ "${KMS_ENABLED}" == "true" && "${KMS_KEY_CREATED}" == "true" ]]; then
  OPERATOR_ROLE_ARNS=$(rosa describe cluster -c "${CLUSTER_NAME}" -o json |
    jq -r '[.aws.sts.operator_iam_roles[].role_arn] | join(" ")')
  if [[ -n "${OPERATOR_ROLE_ARNS}" ]]; then
    echo ""
    echo "--- Granting KMS access to operator roles ---"
    write_kms_policy "${ACCOUNT_ROLE_ARNS} ${OPERATOR_ROLE_ARNS}"
    aws kms put-key-policy \
      --region "${REGION}" \
      --key-id "${KMS_KEY_ARN##*/}" \
      --policy-name default \
      --policy "file://${KMS_POLICY_FILE}"
  fi
fi

# Save cluster info
cat > "${SHARED_DIR:-/tmp}/rosa-e2e-cluster.env" << EOF
export CLUSTER_ID=${CLUSTER_ID}
export CLUSTER_NAME=${CLUSTER_NAME}
export OCM_ENV=${OCM_ENV}
export AWS_REGION=${REGION}
export VPC_ID=${VPC_ID}
export VPC_STACK_NAME=${STACK_NAME}
export OIDC_CONFIG_ID=${OIDC_CONFIG_ID}
export OIDC_CONFIG_CREATED=${OIDC_CONFIG_CREATED}
export AWS_ACCOUNT_ID=${AWS_ACCOUNT_ID}
export BILLING_ACCOUNT=${BILLING_ACCOUNT}
export CLUSTER_SECTOR=${CLUSTER_SECTOR}
export EXTERNAL_AUTH_ENABLED=${EXTERNAL_AUTH_ENABLED}
export KMS_ENABLED=${KMS_ENABLED}
export KMS_KEY_ARN=${KMS_KEY_ARN}
export KMS_KEY_CREATED=${KMS_KEY_CREATED}
export SHARED_VPC_ENABLED=${SHARED_VPC_ENABLED}
export SHARED_VPC_BASE_DOMAIN=${SHARED_VPC_BASE_DOMAIN}
export SHARED_VPC_HOSTED_ZONE_ID=${SHARED_VPC_PHZ_ID}
export SHARED_VPC_INTERNAL_PHZ_ID=${SHARED_VPC_INTERNAL_PHZ_ID}
export SHARED_VPC_ROLE_ARN=${SHARED_VPC_ROLE_ARN}
export SHARED_VPC_POLICY_ARN=${SHARED_VPC_POLICY_ARN}
export SHARED_VPC_VPC_ENDPOINT_ROLE_ARN=${SHARED_VPC_VPC_ENDPOINT_ROLE_ARN}
export SHARED_VPC_VPC_ENDPOINT_POLICY_ARN=${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN}
export SHARED_VPC_INSTALLER_ROLE_ARN=${SHARED_VPC_INSTALLER_ROLE_ARN}
export SHARED_VPC_OPERATOR_ROLES_CREATED=${SHARED_VPC_OPERATOR_ROLES_CREATED}
export SHARED_VPC_AWS_SHARED_CREDENTIALS_FILE=${SHARED_VPC_AWS_SHARED_CREDENTIALS_FILE:-${HOME}/.aws/credentials}
EOF

echo ""
echo "=== Cluster provisioning initiated ==="
echo "Cluster ID: ${CLUSTER_ID}"
echo "Env file: ${SHARED_DIR:-/tmp}/rosa-e2e-cluster.env"
echo ""
echo "Monitor: rosa logs install -c ${CLUSTER_NAME} --watch"
echo ""
if [[ "${EXTERNAL_AUTH_ENABLED}" == "true" ]]; then
  echo "Add an external auth provider after ready:"
  echo "  source /tmp/rosa-e2e-cluster.env"
  echo "  ./scripts/create-external-auth-provider.sh"
  echo ""
fi
echo "Run tests after ready:"
echo "  source /tmp/rosa-e2e-cluster.env"
echo "  OCM_TOKEN=\$(ocm token) make test"
if [[ "${SHARED_VPC_ENABLED}" == "true" ]]; then
  echo ""
  echo "The env file pre-sets the SHARED_VPC_* variables the Feature:SharedVPC specs need."
  echo "Run only the shared VPC specs:"
  echo "  OCM_TOKEN=\$(ocm token) LABEL_FILTER=\"Feature:SharedVPC\" make test"
fi
echo ""
echo "Clean up:"
echo "  source /tmp/rosa-e2e-cluster.env"
echo "  ./scripts/deprovision-e2e-cluster.sh"
