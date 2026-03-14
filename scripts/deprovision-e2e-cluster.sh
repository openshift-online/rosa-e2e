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
REGION="${AWS_REGION:-us-east-1}"
OIDC_CONFIG_ID="${OIDC_CONFIG_ID:-}"

if [[ -z "${CLUSTER_NAME}" && -z "${CLUSTER_ID}" ]]; then
  echo "ERROR: Set CLUSTER_NAME or CLUSTER_ID, or source the cluster env file."
  exit 1
fi

echo "=== ROSA E2E Cluster Deprovisioning ==="
echo "Cluster: ${CLUSTER_NAME:-${CLUSTER_ID}}"

# Step 1: Delete the cluster
echo ""
echo "--- Deleting cluster ---"
rosa delete cluster -c "${CLUSTER_NAME:-${CLUSTER_ID}}" -y

echo "Waiting for cluster deletion..."
rosa logs uninstall -c "${CLUSTER_NAME:-${CLUSTER_ID}}" --watch 2>/dev/null || true

# Step 2: Clean up operator roles and OIDC provider
echo ""
echo "--- Cleaning up operator roles ---"
if [[ -n "${CLUSTER_ID}" ]]; then
  rosa delete operator-roles -c "${CLUSTER_ID}" --mode auto --yes 2>/dev/null || true
  rosa delete oidc-provider -c "${CLUSTER_ID}" --mode auto --yes 2>/dev/null || true
fi

# Step 3: Clean up VPC
if [[ -n "${VPC_ID}" ]]; then
  echo ""
  echo "--- Cleaning up VPC ${VPC_ID} ---"

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

# Step 4: Clean up OIDC config (optional, shared across clusters)
if [[ -n "${OIDC_CONFIG_ID}" ]]; then
  echo ""
  echo "--- Deleting OIDC config ${OIDC_CONFIG_ID} ---"
  rosa delete oidc-config --oidc-config-id "${OIDC_CONFIG_ID}" --mode auto --yes 2>/dev/null || true
fi

# Clean up env file
rm -f "${ENV_FILE}"

echo ""
echo "=== Cleanup complete ==="
