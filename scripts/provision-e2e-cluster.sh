#!/bin/bash
# Provision a ROSA HCP cluster for e2e testing.
#
# Prerequisites:
#   - AWS credentials set (via rh-aws-saml-login or osdctl)
#   - OCM logged into staging: ocm login --use-auth-code --url stage
#   - rosa CLI installed
#
# Usage:
#   # First, get AWS credentials for your dev account:
#   rh-aws-saml-login osd-staging-2 --region us-east-1 --assume-uid <YOUR_ACCOUNT_ID>
#
#   # Then in the spawned shell:
#   ./scripts/provision-e2e-cluster.sh
#
#   # Or with custom settings:
#   CLUSTER_NAME=my-e2e REGION=us-west-2 ./scripts/provision-e2e-cluster.sh

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-rosa-e2e-$(date +%m%d)}"
REGION="${REGION:-us-east-1}"
COMPUTE_NODES="${COMPUTE_NODES:-2}"
COMPUTE_MACHINE_TYPE="${COMPUTE_MACHINE_TYPE:-m5.xlarge}"
VERSION="${VERSION:-}"
CHANNEL_GROUP="${CHANNEL_GROUP:-stable}"

echo "=== ROSA E2E Cluster Provisioning ==="
echo "Cluster name: ${CLUSTER_NAME}"
echo "Region: ${REGION}"
echo "Compute: ${COMPUTE_NODES}x ${COMPUTE_MACHINE_TYPE}"
echo ""

# Verify prerequisites
echo "--- Checking prerequisites ---"

if ! aws sts get-caller-identity &>/dev/null; then
  echo "ERROR: AWS credentials not set. Run rh-aws-saml-login first."
  exit 1
fi
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
echo "AWS Account: ${AWS_ACCOUNT_ID}"

if ! ocm config get url | grep -q 'stage\|integration'; then
  echo "ERROR: OCM not logged into staging/integration."
  exit 1
fi
OCM_ENV=$(ocm config get url)
echo "OCM: ${OCM_ENV}"

if ! command -v rosa &>/dev/null; then
  echo "ERROR: rosa CLI not found."
  exit 1
fi

# Step 1: Create account roles (idempotent)
echo ""
echo "--- Creating HCP account roles ---"
rosa create account-roles --hosted-cp --mode auto --yes 2>&1 || true

# Step 2: Create OIDC config
echo ""
echo "--- Creating OIDC config ---"
OIDC_OUTPUT=$(rosa create oidc-config --mode auto --yes 2>&1)
echo "${OIDC_OUTPUT}"
OIDC_CONFIG_ID=$(echo "${OIDC_OUTPUT}" | grep -oE '[a-z0-9]{32}' | head -1)
if [[ -z "${OIDC_CONFIG_ID}" ]]; then
  # Try to get from existing configs
  OIDC_CONFIG_ID=$(rosa list oidc-config -o json | jq -r '.[0].id // empty')
fi
echo "OIDC Config ID: ${OIDC_CONFIG_ID}"

# Step 3: Create VPC and subnets
echo ""
echo "--- Creating VPC ---"
VPC_CIDR="10.0.0.0/16"
VPC_ID=$(aws ec2 create-vpc --cidr-block ${VPC_CIDR} --query 'Vpc.VpcId' --output text --region ${REGION})
aws ec2 create-tags --resources ${VPC_ID} --tags Key=Name,Value=${CLUSTER_NAME}-vpc --region ${REGION}
aws ec2 modify-vpc-attribute --vpc-id ${VPC_ID} --enable-dns-hostnames --region ${REGION}
aws ec2 modify-vpc-attribute --vpc-id ${VPC_ID} --enable-dns-support --region ${REGION}
echo "VPC: ${VPC_ID}"

# Create internet gateway
IGW_ID=$(aws ec2 create-internet-gateway --query 'InternetGateway.InternetGatewayId' --output text --region ${REGION})
aws ec2 attach-internet-gateway --internet-gateway-id ${IGW_ID} --vpc-id ${VPC_ID} --region ${REGION}
aws ec2 create-tags --resources ${IGW_ID} --tags Key=Name,Value=${CLUSTER_NAME}-igw --region ${REGION}

# Get AZs
AZ1=$(aws ec2 describe-availability-zones --region ${REGION} --query 'AvailabilityZones[0].ZoneName' --output text)
AZ2=$(aws ec2 describe-availability-zones --region ${REGION} --query 'AvailabilityZones[1].ZoneName' --output text)

# Create public subnets
PUB_SUBNET_1=$(aws ec2 create-subnet --vpc-id ${VPC_ID} --cidr-block 10.0.0.0/24 --availability-zone ${AZ1} --query 'Subnet.SubnetId' --output text --region ${REGION})
PUB_SUBNET_2=$(aws ec2 create-subnet --vpc-id ${VPC_ID} --cidr-block 10.0.1.0/24 --availability-zone ${AZ2} --query 'Subnet.SubnetId' --output text --region ${REGION})
aws ec2 create-tags --resources ${PUB_SUBNET_1} --tags Key=Name,Value=${CLUSTER_NAME}-public-1 --region ${REGION}
aws ec2 create-tags --resources ${PUB_SUBNET_2} --tags Key=Name,Value=${CLUSTER_NAME}-public-2 --region ${REGION}

# Create private subnets
PRIV_SUBNET_1=$(aws ec2 create-subnet --vpc-id ${VPC_ID} --cidr-block 10.0.2.0/24 --availability-zone ${AZ1} --query 'Subnet.SubnetId' --output text --region ${REGION})
PRIV_SUBNET_2=$(aws ec2 create-subnet --vpc-id ${VPC_ID} --cidr-block 10.0.3.0/24 --availability-zone ${AZ2} --query 'Subnet.SubnetId' --output text --region ${REGION})
aws ec2 create-tags --resources ${PRIV_SUBNET_1} --tags Key=Name,Value=${CLUSTER_NAME}-private-1 --region ${REGION}
aws ec2 create-tags --resources ${PRIV_SUBNET_2} --tags Key=Name,Value=${CLUSTER_NAME}-private-2 --region ${REGION}

# Create NAT gateway (for private subnets)
EIP_ALLOC=$(aws ec2 allocate-address --domain vpc --query 'AllocationId' --output text --region ${REGION})
NAT_GW=$(aws ec2 create-nat-gateway --subnet-id ${PUB_SUBNET_1} --allocation-id ${EIP_ALLOC} --query 'NatGateway.NatGatewayId' --output text --region ${REGION})
aws ec2 create-tags --resources ${NAT_GW} --tags Key=Name,Value=${CLUSTER_NAME}-nat --region ${REGION}
echo "Waiting for NAT gateway..."
aws ec2 wait nat-gateway-available --nat-gateway-ids ${NAT_GW} --region ${REGION}

# Route tables
PUB_RT=$(aws ec2 create-route-table --vpc-id ${VPC_ID} --query 'RouteTable.RouteTableId' --output text --region ${REGION})
aws ec2 create-route --route-table-id ${PUB_RT} --destination-cidr-block 0.0.0.0/0 --gateway-id ${IGW_ID} --region ${REGION}
aws ec2 associate-route-table --route-table-id ${PUB_RT} --subnet-id ${PUB_SUBNET_1} --region ${REGION}
aws ec2 associate-route-table --route-table-id ${PUB_RT} --subnet-id ${PUB_SUBNET_2} --region ${REGION}

PRIV_RT=$(aws ec2 create-route-table --vpc-id ${VPC_ID} --query 'RouteTable.RouteTableId' --output text --region ${REGION})
aws ec2 create-route --route-table-id ${PRIV_RT} --destination-cidr-block 0.0.0.0/0 --nat-gateway-id ${NAT_GW} --region ${REGION}
aws ec2 associate-route-table --route-table-id ${PRIV_RT} --subnet-id ${PRIV_SUBNET_1} --region ${REGION}
aws ec2 associate-route-table --route-table-id ${PRIV_RT} --subnet-id ${PRIV_SUBNET_2} --region ${REGION}

echo "Public subnets: ${PUB_SUBNET_1}, ${PUB_SUBNET_2}"
echo "Private subnets: ${PRIV_SUBNET_1}, ${PRIV_SUBNET_2}"

# Step 4: Determine version
if [[ -z "${VERSION}" ]]; then
  VERSION=$(rosa list versions --hosted-cp --channel-group ${CHANNEL_GROUP} -o json | jq -r '.[0].raw_id')
fi
echo ""
echo "--- Using version: ${VERSION} ---"

# Step 5: Create the cluster
echo ""
echo "--- Creating ROSA HCP cluster ---"
SUBNET_IDS="${PUB_SUBNET_1},${PUB_SUBNET_2},${PRIV_SUBNET_1},${PRIV_SUBNET_2}"

rosa create cluster \
  --cluster-name "${CLUSTER_NAME}" \
  --hosted-cp \
  --sts \
  --mode auto \
  --region "${REGION}" \
  --version "${VERSION}" \
  --channel-group "${CHANNEL_GROUP}" \
  --compute-machine-type "${COMPUTE_MACHINE_TYPE}" \
  --replicas "${COMPUTE_NODES}" \
  --billing-account "${AWS_ACCOUNT_ID}" \
  --oidc-config-id "${OIDC_CONFIG_ID}" \
  --subnet-ids "${SUBNET_IDS}" \
  --yes

# Get cluster ID
CLUSTER_ID=$(rosa describe cluster -c "${CLUSTER_NAME}" -o json | jq -r '.id')
echo ""
echo "=== Cluster created ==="
echo "Cluster ID: ${CLUSTER_ID}"
echo "Cluster name: ${CLUSTER_NAME}"
echo "Region: ${REGION}"
echo ""

# Save cluster info for the test suite
cat > "${SHARED_DIR:-/tmp}/rosa-e2e-cluster.env" << EOF
export CLUSTER_ID=${CLUSTER_ID}
export CLUSTER_NAME=${CLUSTER_NAME}
export OCM_ENV=staging
export AWS_REGION=${REGION}
export VPC_ID=${VPC_ID}
export OIDC_CONFIG_ID=${OIDC_CONFIG_ID}
EOF

echo "Cluster info saved to ${SHARED_DIR:-/tmp}/rosa-e2e-cluster.env"
echo ""
echo "Monitor installation:"
echo "  rosa logs install -c ${CLUSTER_NAME} --watch"
echo ""
echo "Run tests after cluster is ready:"
echo "  source /tmp/rosa-e2e-cluster.env"
echo "  OCM_TOKEN=\$(ocm token) make test"
echo ""
echo "Delete cluster when done:"
echo "  ./scripts/deprovision-e2e-cluster.sh"
