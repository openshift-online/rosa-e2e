#!/bin/bash
# Deprovision a ROSA HCP e2e test cluster and clean up AWS resources.
#
# Usage:
#   source /tmp/rosa-e2e-cluster.env
#   ./scripts/deprovision-e2e-cluster.sh

set -euo pipefail

ENV_FILE="${SHARED_DIR:-/tmp}/rosa-e2e-cluster.env"
if [[ -f "${ENV_FILE}" ]]; then
  source "${ENV_FILE}"
fi

CLUSTER_NAME="${CLUSTER_NAME:-}"
CLUSTER_ID="${CLUSTER_ID:-}"
VPC_ID="${VPC_ID:-}"
VPC_STACK_NAME="${VPC_STACK_NAME:-}"
REGION="${AWS_REGION:-us-east-2}"
OIDC_CONFIG_ID="${OIDC_CONFIG_ID:-}"

if [[ -z "${CLUSTER_NAME}" && -z "${CLUSTER_ID}" ]]; then
  echo "ERROR: Set CLUSTER_NAME or CLUSTER_ID, or source the cluster env file."
  exit 1
fi

echo "=== ROSA E2E Cluster Deprovisioning ==="
echo "Cluster: ${CLUSTER_NAME:-${CLUSTER_ID}}"

# Resolve the operator role prefix before deleting the cluster: once the OCM
# record is gone, `rosa delete operator-roles -c <id>` no longer finds the roles.
# Operator role names follow <cluster-name>-<random4>-<namespace>-<operator>.
OPERATOR_ROLE_PREFIX="${OPERATOR_ROLE_PREFIX:-}"
if [[ -z "${OPERATOR_ROLE_PREFIX}" && -n "${CLUSTER_NAME:-${CLUSTER_ID}}" ]]; then
  CLUSTER_DESC=$(rosa describe cluster -c "${CLUSTER_NAME:-${CLUSTER_ID}}" -o json 2>/dev/null || true)
  if [[ -n "${CLUSTER_DESC}" ]]; then
    FIRST_OPERATOR_ROLE=$(jq -r '[.aws.sts.operator_iam_roles[].role_arn][0] // empty' <<<"${CLUSTER_DESC}" | sed 's|.*/||')
    NAME_FROM_DESC=$(jq -r '.name // empty' <<<"${CLUSTER_DESC}")
    if [[ -n "${FIRST_OPERATOR_ROLE}" && -n "${NAME_FROM_DESC}" ]]; then
      OPERATOR_ROLE_PREFIX=$(sed "s/^${NAME_FROM_DESC}-\([^-]*\)-.*/${NAME_FROM_DESC}-\1-/" <<<"${FIRST_OPERATOR_ROLE}")
      # If the role name did not match the expected pattern, do not guess.
      [[ "${OPERATOR_ROLE_PREFIX}" == "${FIRST_OPERATOR_ROLE}" ]] && OPERATOR_ROLE_PREFIX=""
    fi
  fi
fi
if [[ -n "${OPERATOR_ROLE_PREFIX}" ]]; then
  echo "Operator role prefix: ${OPERATOR_ROLE_PREFIX}"
fi

# Step 1: Delete the cluster
echo ""
echo "--- Deleting cluster ---"
rosa delete cluster -c "${CLUSTER_NAME:-${CLUSTER_ID}}" -y

echo "Waiting for cluster deletion..."
rosa logs uninstall -c "${CLUSTER_NAME:-${CLUSTER_ID}}" --watch 2>/dev/null || true

# Step 2: Clean up operator roles and OIDC provider
echo ""
echo "--- Cleaning up operator roles ---"
if [[ -n "${OPERATOR_ROLE_PREFIX}" ]]; then
  rosa delete operator-roles --prefix "${OPERATOR_ROLE_PREFIX}" --mode auto --yes || true
elif [[ -n "${CLUSTER_ID}" ]]; then
  rosa delete operator-roles -c "${CLUSTER_ID}" --mode auto --yes 2>/dev/null || true
fi
if [[ -n "${CLUSTER_ID}" ]]; then
  rosa delete oidc-provider -c "${CLUSTER_ID}" --mode auto --yes 2>/dev/null || true
fi

# Step 3: Clean up VPC CloudFormation stack
if [[ -n "${VPC_STACK_NAME}" ]]; then
  echo ""
  echo "--- Deleting VPC CloudFormation stack ${VPC_STACK_NAME} ---"

  # Workaround for OCPBUGS-74960: Delete leftover vpce-private-router security groups
  # before deleting the stack to prevent stack deletion failure
  VPC_ID_FROM_STACK=$(aws cloudformation describe-stacks \
    --stack-name "${VPC_STACK_NAME}" \
    --region "${REGION}" \
    --query 'Stacks[0].Outputs[?OutputKey==`VPCId`].OutputValue' \
    --output text 2>/dev/null || echo "")

  if [[ -n "${VPC_ID_FROM_STACK}" ]]; then
    echo "Cleaning up vpce-private-router security groups in VPC ${VPC_ID_FROM_STACK}..."
    for sg in $(aws ec2 describe-security-groups \
      --filters "Name=vpc-id,Values=${VPC_ID_FROM_STACK}" "Name=group-name,Values=*vpce-private-router*" \
      --query 'SecurityGroups[].GroupId' \
      --output text \
      --region ${REGION} 2>/dev/null); do
      echo "Deleting security group: ${sg}"
      aws ec2 delete-security-group --group-id ${sg} --region ${REGION} 2>/dev/null || true
    done
  fi

  aws cloudformation delete-stack --stack-name "${VPC_STACK_NAME}" --region ${REGION} 2>/dev/null || true

  echo "Waiting for VPC stack deletion..."
  aws cloudformation wait stack-delete-complete --stack-name "${VPC_STACK_NAME}" --region ${REGION} 2>/dev/null || true
  STACK_STATUS=$(aws cloudformation describe-stacks --stack-name "${VPC_STACK_NAME}" --region "${REGION}" \
    --query 'Stacks[0].StackStatus' --output text 2>/dev/null || echo "")
  if [[ -n "${STACK_STATUS}" && "${STACK_STATUS}" != "DELETE_COMPLETE" ]]; then
    echo "WARNING: VPC stack deletion ended in ${STACK_STATUS}; run 'aws cloudformation describe-stack-resources --stack-name ${VPC_STACK_NAME} --region ${REGION}' to find the resource blocking deletion."
  fi
elif [[ -n "${VPC_ID}" ]]; then
  echo ""
  echo "--- Cleaning up VPC ${VPC_ID} (legacy manual cleanup) ---"

  # Delete NAT gateways
  for nat in $(aws ec2 describe-nat-gateways --filter Name=vpc-id,Values=${VPC_ID} --query 'NatGateways[].NatGatewayId' --output text --region ${REGION} 2>/dev/null); do
    echo "Deleting NAT gateway: ${nat}"
    aws ec2 delete-nat-gateway --nat-gateway-id ${nat} --region ${REGION} 2>/dev/null || true
  done
  echo "Waiting for NAT gateways to delete..."
  sleep 30

  # Release EIPs
  for eip in $(aws ec2 describe-addresses --filter Name=domain,Values=vpc --query 'Addresses[?AssociationId==null].AllocationId' --output text --region ${REGION} 2>/dev/null); do
    echo "Releasing EIP: ${eip}"
    aws ec2 release-address --allocation-id ${eip} --region ${REGION} 2>/dev/null || true
  done

  # Detach and delete internet gateway
  for igw in $(aws ec2 describe-internet-gateways --filters Name=attachment.vpc-id,Values=${VPC_ID} --query 'InternetGateways[].InternetGatewayId' --output text --region ${REGION} 2>/dev/null); do
    echo "Detaching IGW: ${igw}"
    aws ec2 detach-internet-gateway --internet-gateway-id ${igw} --vpc-id ${VPC_ID} --region ${REGION} 2>/dev/null || true
    aws ec2 delete-internet-gateway --internet-gateway-id ${igw} --region ${REGION} 2>/dev/null || true
  done

  # Delete subnets
  for subnet in $(aws ec2 describe-subnets --filters Name=vpc-id,Values=${VPC_ID} --query 'Subnets[].SubnetId' --output text --region ${REGION} 2>/dev/null); do
    echo "Deleting subnet: ${subnet}"
    aws ec2 delete-subnet --subnet-id ${subnet} --region ${REGION} 2>/dev/null || true
  done

  # Delete route tables (non-main)
  for rt in $(aws ec2 describe-route-tables --filters Name=vpc-id,Values=${VPC_ID} --query 'RouteTables[?Associations[0].Main!=`true`].RouteTableId' --output text --region ${REGION} 2>/dev/null); do
    echo "Deleting route table: ${rt}"
    aws ec2 delete-route-table --route-table-id ${rt} --region ${REGION} 2>/dev/null || true
  done

  # Delete security groups (non-default)
  for sg in $(aws ec2 describe-security-groups --filters Name=vpc-id,Values=${VPC_ID} --query 'SecurityGroups[?GroupName!=`default`].GroupId' --output text --region ${REGION} 2>/dev/null); do
    echo "Deleting security group: ${sg}"
    aws ec2 delete-security-group --group-id ${sg} --region ${REGION} 2>/dev/null || true
  done

  # Delete VPC
  echo "Deleting VPC: ${VPC_ID}"
  aws ec2 delete-vpc --vpc-id ${VPC_ID} --region ${REGION} 2>/dev/null || true
fi

# Step 4: Delete the OIDC config, but only when this run created it: a config
# the provision script adopted (any managed config in the org) may be shared
# with other clusters and is not ours to delete.
OIDC_CONFIG_CREATED="${OIDC_CONFIG_CREATED:-false}"
if [[ -n "${OIDC_CONFIG_ID}" ]]; then
  if [[ "${OIDC_CONFIG_CREATED}" == "true" ]]; then
    echo ""
    echo "--- Deleting OIDC config ${OIDC_CONFIG_ID} ---"
    rosa delete oidc-config --oidc-config-id "${OIDC_CONFIG_ID}" --mode auto --yes 2>/dev/null || true
  else
    echo ""
    echo "--- Keeping OIDC config ${OIDC_CONFIG_ID} (not created by the provision script) ---"
  fi
fi

# Step 5: Schedule KMS key deletion if the provision script created it (the key must
# no longer be in use by the cluster, so this runs after cluster deletion)
KMS_KEY_ARN="${KMS_KEY_ARN:-}"
KMS_KEY_CREATED="${KMS_KEY_CREATED:-false}"
if [[ "${KMS_KEY_CREATED}" == "true" && -n "${KMS_KEY_ARN}" ]]; then
  echo ""
  echo "--- Scheduling KMS key deletion: ${KMS_KEY_ARN} ---"
  aws kms schedule-key-deletion \
    --key-id "${KMS_KEY_ARN}" \
    --pending-window-in-days 7 \
    --region "${REGION}" 2>/dev/null || true
fi

# Step 6: Clean up the shared VPC DNS infrastructure (PHZ, role, policy) created by
# the provision script. The PHZ must be emptied first: the cluster's records are
# removed during cluster deletion, so this runs after it (mirrors the CI orphan
# gate rosa-e2e-validate-shared-vpc-teardown).
SHARED_VPC_ENABLED="${SHARED_VPC_ENABLED:-false}"
if [[ "${SHARED_VPC_ENABLED}" == "true" ]]; then
  SHARED_VPC_HOSTED_ZONE_ID="${SHARED_VPC_HOSTED_ZONE_ID:-}"
  SHARED_VPC_INTERNAL_PHZ_ID="${SHARED_VPC_INTERNAL_PHZ_ID:-}"
  SHARED_VPC_ROLE_ARN="${SHARED_VPC_ROLE_ARN:-}"
  SHARED_VPC_POLICY_ARN="${SHARED_VPC_POLICY_ARN:-}"
  SHARED_VPC_VPC_ENDPOINT_ROLE_ARN="${SHARED_VPC_VPC_ENDPOINT_ROLE_ARN:-}"
  SHARED_VPC_VPC_ENDPOINT_POLICY_ARN="${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN:-}"

  delete_hosted_zone() {
    local zone_id="$1"
    LEFTOVER_RECORDS=$(aws route53 list-resource-record-sets \
      --hosted-zone-id "${zone_id}" \
      --query 'ResourceRecordSets[?Type!=`NS` && Type!=`SOA`]' \
      --output json --region "${REGION}" 2>/dev/null || echo "[]")
    if [[ "$(jq length <<<"${LEFTOVER_RECORDS}")" != "0" ]]; then
      echo "WARNING: hosted zone still has records, deleting them:"
      jq -r '.[].Name' <<<"${LEFTOVER_RECORDS}"
      CHANGE_FILE=$(mktemp)
      jq '{Changes: [.[] | {Action: "DELETE", ResourceRecordSet: .}]}' <<<"${LEFTOVER_RECORDS}" > "${CHANGE_FILE}"
      aws route53 change-resource-record-sets \
        --hosted-zone-id "${zone_id}" \
        --change-batch "file://${CHANGE_FILE}" --region "${REGION}" 2>/dev/null || true
      rm -f "${CHANGE_FILE}"
    fi
    aws route53 delete-hosted-zone --id "${zone_id}" --region "${REGION}" 2>/dev/null \
      || echo "WARNING: failed to delete hosted zone ${zone_id}"
  }

  if [[ -n "${SHARED_VPC_HOSTED_ZONE_ID}" ]]; then
    echo ""
    echo "--- Deleting shared VPC private hosted zone ${SHARED_VPC_HOSTED_ZONE_ID} ---"
    delete_hosted_zone "${SHARED_VPC_HOSTED_ZONE_ID}"
  fi
  if [[ -n "${SHARED_VPC_INTERNAL_PHZ_ID}" ]]; then
    echo ""
    echo "--- Deleting internal communication hosted zone ${SHARED_VPC_INTERNAL_PHZ_ID} ---"
    delete_hosted_zone "${SHARED_VPC_INTERNAL_PHZ_ID}"
  fi

  # The assume-role policies reference the shared roles, so delete them first
  for shared_role_arn in "${SHARED_VPC_ROLE_ARN}" "${SHARED_VPC_VPC_ENDPOINT_ROLE_ARN}"; do
    [[ -z "${shared_role_arn}" ]] && continue
    shared_role_name="${shared_role_arn##*/}"
    policy_arn="$(sed 's|:role/.*|:policy/'"${shared_role_name}"'-assume-role|' <<<"${shared_role_arn}")"
    if aws iam get-policy --policy-arn "${policy_arn}" &>/dev/null; then
      echo "--- Deleting assume-role policy ${policy_arn} ---"
      for attached_role in $(aws iam list-entities-for-policy --policy-arn "${policy_arn}" \
        --query 'PolicyRoles[].RoleName' --output text 2>/dev/null); do
        aws iam detach-role-policy --role-name "${attached_role}" --policy-arn "${policy_arn}" 2>/dev/null || true
      done
      aws iam delete-policy --policy-arn "${policy_arn}" 2>/dev/null \
        || echo "WARNING: failed to delete policy ${policy_arn}"
    fi
  done

  if [[ -n "${SHARED_VPC_ROLE_ARN}" ]]; then
    SHARED_ROLE_NAME="${SHARED_VPC_ROLE_ARN##*/}"
    echo "--- Deleting shared VPC role ${SHARED_ROLE_NAME} ---"
    if [[ -n "${SHARED_VPC_POLICY_ARN}" ]]; then
      aws iam detach-role-policy --role-name "${SHARED_ROLE_NAME}" --policy-arn "${SHARED_VPC_POLICY_ARN}" 2>/dev/null || true
    fi
    aws iam delete-role --role-name "${SHARED_ROLE_NAME}" 2>/dev/null \
      || echo "WARNING: failed to delete role ${SHARED_ROLE_NAME}"
  fi
  if [[ -n "${SHARED_VPC_POLICY_ARN}" ]]; then
    echo "--- Deleting shared VPC policy ${SHARED_VPC_POLICY_ARN} ---"
    aws iam delete-policy --policy-arn "${SHARED_VPC_POLICY_ARN}" 2>/dev/null \
      || echo "WARNING: failed to delete policy ${SHARED_VPC_POLICY_ARN}"
  fi
  if [[ -n "${SHARED_VPC_VPC_ENDPOINT_ROLE_ARN}" ]]; then
    VPC_ENDPOINT_ROLE_NAME="${SHARED_VPC_VPC_ENDPOINT_ROLE_ARN##*/}"
    echo "--- Deleting VPC endpoint role ${VPC_ENDPOINT_ROLE_NAME} ---"
    if [[ -n "${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN}" ]]; then
      aws iam detach-role-policy --role-name "${VPC_ENDPOINT_ROLE_NAME}" --policy-arn "${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN}" 2>/dev/null || true
    fi
    aws iam delete-role --role-name "${VPC_ENDPOINT_ROLE_NAME}" 2>/dev/null \
      || echo "WARNING: failed to delete role ${VPC_ENDPOINT_ROLE_NAME}"
  fi
  if [[ -n "${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN}" ]]; then
    echo "--- Deleting VPC endpoint policy ${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN} ---"
    aws iam delete-policy --policy-arn "${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN}" 2>/dev/null \
      || echo "WARNING: failed to delete policy ${SHARED_VPC_VPC_ENDPOINT_POLICY_ARN}"
  fi
fi

# Clean up env file
rm -f "${ENV_FILE}"

echo ""
echo "=== Cleanup complete ==="
