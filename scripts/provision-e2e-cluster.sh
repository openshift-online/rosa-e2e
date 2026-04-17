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
COMPUTE_MACHINE_TYPE="${COMPUTE_MACHINE_TYPE:-m5.xlarge}"
OIDC_CONFIG_ID="${OIDC_CONFIG_ID:-}"
CLUSTER_SECTOR="${CLUSTER_SECTOR:-}"

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
if [[ -z "${OIDC_CONFIG_ID}" ]]; then
  echo ""
  echo "--- Getting OIDC config ---"
  OIDC_CONFIG_ID=$(rosa list oidc-config -o json 2>/dev/null | jq -r '.[0].id // empty')
  if [[ -z "${OIDC_CONFIG_ID}" ]]; then
    echo "Creating OIDC config..."
    rosa create oidc-config --mode auto --yes
    OIDC_CONFIG_ID=$(rosa list oidc-config -o json | jq -r '.[0].id')
  fi
fi
echo "OIDC Config: ${OIDC_CONFIG_ID}"

# Step 3: Create VPC using ROSA network template
echo ""
echo "--- Creating VPC ---"
STACK_NAME="${CLUSTER_NAME}-vpc"

rosa create network rosa-quickstart-default-vpc \
  --mode auto \
  --param Region="${REGION}" \
  --param Name="${STACK_NAME}" \
  --param AvailabilityZoneCount=2 \
  --param VpcCidr=10.0.0.0/16 \
  --yes

# Wait for stack creation
echo "Waiting for VPC stack creation..."
aws cloudformation wait stack-create-complete \
  --stack-name "${STACK_NAME}" \
  --region "${REGION}"

# Get VPC ID and subnet IDs from stack outputs
VPC_ID=$(aws cloudformation describe-stacks \
  --stack-name "${STACK_NAME}" \
  --region "${REGION}" \
  --query 'Stacks[0].Outputs[?OutputKey==`VpcId`].OutputValue' \
  --output text)

# Get subnet IDs (public and private)
PUBLIC_SUBNET_IDS=$(aws cloudformation describe-stacks \
  --stack-name "${STACK_NAME}" \
  --region "${REGION}" \
  --query 'Stacks[0].Outputs[?OutputKey==`PublicSubnetIds`].OutputValue' \
  --output text)

PRIVATE_SUBNET_IDS=$(aws cloudformation describe-stacks \
  --stack-name "${STACK_NAME}" \
  --region "${REGION}" \
  --query 'Stacks[0].Outputs[?OutputKey==`PrivateSubnetIds`].OutputValue' \
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

# Step 5: Create cluster
echo ""
echo "--- Creating ROSA HCP cluster ---"
rosa create cluster \
  --cluster-name "${CLUSTER_NAME}" \
  --hosted-cp \
  --sts \
  --mode auto \
  --region "${REGION}" \
  --replicas "${COMPUTE_NODES}" \
  --compute-machine-type "${COMPUTE_MACHINE_TYPE}" \
  --billing-account "${BILLING_ACCOUNT}" \
  --oidc-config-id "${OIDC_CONFIG_ID}" \
  --subnet-ids "${SUBNET_IDS}" \
  ${SECTOR_ARGS} \
  --yes

CLUSTER_ID=$(rosa describe cluster -c "${CLUSTER_NAME}" -o json | jq -r '.id')

# Save cluster info
cat > "${SHARED_DIR:-/tmp}/rosa-e2e-cluster.env" << EOF
export CLUSTER_ID=${CLUSTER_ID}
export CLUSTER_NAME=${CLUSTER_NAME}
export OCM_ENV=${OCM_ENV}
export AWS_REGION=${REGION}
export VPC_ID=${VPC_ID}
export VPC_STACK_NAME=${STACK_NAME}
export OIDC_CONFIG_ID=${OIDC_CONFIG_ID}
export AWS_ACCOUNT_ID=${AWS_ACCOUNT_ID}
export BILLING_ACCOUNT=${BILLING_ACCOUNT}
export CLUSTER_SECTOR=${CLUSTER_SECTOR}
EOF

echo ""
echo "=== Cluster provisioning initiated ==="
echo "Cluster ID: ${CLUSTER_ID}"
echo "Env file: ${SHARED_DIR:-/tmp}/rosa-e2e-cluster.env"
echo ""
echo "Monitor: rosa logs install -c ${CLUSTER_NAME} --watch"
echo ""
echo "Run tests after ready:"
echo "  source /tmp/rosa-e2e-cluster.env"
echo "  OCM_TOKEN=\$(ocm token) make test"
echo ""
echo "Clean up:"
echo "  source /tmp/rosa-e2e-cluster.env"
echo "  ./scripts/deprovision-e2e-cluster.sh"
