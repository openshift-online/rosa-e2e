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
REGION="${REGION:-us-east-2}"
COMPUTE_NODES="${COMPUTE_NODES:-2}"
COMPUTE_MACHINE_TYPE="${COMPUTE_MACHINE_TYPE:-m5.xlarge}"
OIDC_CONFIG_ID="${OIDC_CONFIG_ID:-}"
# For staging, billing account is osd-staging-2 payer account
BILLING_ACCOUNT="${BILLING_ACCOUNT:-811685182089}"

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
  echo "ERROR: OCM not logged in. Run: ocm login --use-auth-code --url stage"
  exit 1
fi
echo "OCM: $(ocm config get url)"

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

# Step 3: Create VPC with HCP-compatible CIDR (10.0.0.0/16)
echo ""
echo "--- Creating VPC ---"
VPC_ID=$(aws ec2 create-vpc --cidr-block 10.0.0.0/16 --query 'Vpc.VpcId' --output text --region ${REGION})
aws ec2 create-tags --resources ${VPC_ID} --tags Key=Name,Value=${CLUSTER_NAME}-vpc --region ${REGION}
aws ec2 modify-vpc-attribute --vpc-id ${VPC_ID} --enable-dns-hostnames --region ${REGION}
aws ec2 modify-vpc-attribute --vpc-id ${VPC_ID} --enable-dns-support --region ${REGION}

# Internet gateway
IGW_ID=$(aws ec2 create-internet-gateway --query 'InternetGateway.InternetGatewayId' --output text --region ${REGION})
aws ec2 attach-internet-gateway --internet-gateway-id ${IGW_ID} --vpc-id ${VPC_ID} --region ${REGION}

# AZs
AZ1=$(aws ec2 describe-availability-zones --region ${REGION} --query 'AvailabilityZones[0].ZoneName' --output text)
AZ2=$(aws ec2 describe-availability-zones --region ${REGION} --query 'AvailabilityZones[1].ZoneName' --output text)

# Subnets
PUB1=$(aws ec2 create-subnet --vpc-id ${VPC_ID} --cidr-block 10.0.0.0/24 --availability-zone ${AZ1} --query 'Subnet.SubnetId' --output text --region ${REGION})
PUB2=$(aws ec2 create-subnet --vpc-id ${VPC_ID} --cidr-block 10.0.1.0/24 --availability-zone ${AZ2} --query 'Subnet.SubnetId' --output text --region ${REGION})
PRIV1=$(aws ec2 create-subnet --vpc-id ${VPC_ID} --cidr-block 10.0.2.0/24 --availability-zone ${AZ1} --query 'Subnet.SubnetId' --output text --region ${REGION})
PRIV2=$(aws ec2 create-subnet --vpc-id ${VPC_ID} --cidr-block 10.0.3.0/24 --availability-zone ${AZ2} --query 'Subnet.SubnetId' --output text --region ${REGION})

# NAT gateway
EIP=$(aws ec2 allocate-address --domain vpc --query 'AllocationId' --output text --region ${REGION})
NAT=$(aws ec2 create-nat-gateway --subnet-id ${PUB1} --allocation-id ${EIP} --query 'NatGateway.NatGatewayId' --output text --region ${REGION})
echo "Waiting for NAT gateway..."
aws ec2 wait nat-gateway-available --nat-gateway-ids ${NAT} --region ${REGION}

# Route tables
PUB_RT=$(aws ec2 create-route-table --vpc-id ${VPC_ID} --query 'RouteTable.RouteTableId' --output text --region ${REGION})
aws ec2 create-route --route-table-id ${PUB_RT} --destination-cidr-block 0.0.0.0/0 --gateway-id ${IGW_ID} --region ${REGION} > /dev/null
aws ec2 associate-route-table --route-table-id ${PUB_RT} --subnet-id ${PUB1} --region ${REGION} > /dev/null
aws ec2 associate-route-table --route-table-id ${PUB_RT} --subnet-id ${PUB2} --region ${REGION} > /dev/null

PRIV_RT=$(aws ec2 create-route-table --vpc-id ${VPC_ID} --query 'RouteTable.RouteTableId' --output text --region ${REGION})
aws ec2 create-route --route-table-id ${PRIV_RT} --destination-cidr-block 0.0.0.0/0 --nat-gateway-id ${NAT} --region ${REGION} > /dev/null
aws ec2 associate-route-table --route-table-id ${PRIV_RT} --subnet-id ${PRIV1} --region ${REGION} > /dev/null
aws ec2 associate-route-table --route-table-id ${PRIV_RT} --subnet-id ${PRIV2} --region ${REGION} > /dev/null

echo "VPC: ${VPC_ID}"
SUBNET_IDS="${PUB1},${PUB2},${PRIV1},${PRIV2}"

# Step 4: Create cluster
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
  --yes

CLUSTER_ID=$(rosa describe cluster -c "${CLUSTER_NAME}" -o json | jq -r '.id')

# Save cluster info
cat > "${SHARED_DIR:-/tmp}/rosa-e2e-cluster.env" << EOF
export CLUSTER_ID=${CLUSTER_ID}
export CLUSTER_NAME=${CLUSTER_NAME}
export OCM_ENV=staging
export AWS_REGION=${REGION}
export VPC_ID=${VPC_ID}
export OIDC_CONFIG_ID=${OIDC_CONFIG_ID}
export AWS_ACCOUNT_ID=${AWS_ACCOUNT_ID}
export BILLING_ACCOUNT=${BILLING_ACCOUNT}
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
