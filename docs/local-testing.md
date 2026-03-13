# Local Testing Guide

How to run rosa-e2e tests from your laptop against staging clusters.

## Prerequisites

- Go 1.24+
- `ocm` CLI logged into staging: `ocm login --use-auth-code --url stage`
- `ocm-backplane` CLI (for management cluster access)
- AWS credentials (for CloudTrail/EBS tag tests, optional)

## Quick Start: Hosted Cluster Tests Only

The simplest path -- tests against the hosted cluster data plane.

```bash
# Find a ready HCP cluster in staging
ocm list clusters --parameter search="product.id='rosa' AND hypershift.enabled='true' AND state='ready'" --columns id,name,region.id

# Run tests
export OCM_TOKEN=$(ocm token)
export OCM_ENV=staging
export CLUSTER_ID=<cluster-id>
make test
```

This runs all tests. Tests requiring MC or AWS access will skip gracefully.

## Filtering by Test Area

```bash
# Only data plane tests (workload, storage)
LABEL_FILTER="Area:DataPlane" make test

# Only managed service health (ClusterOperators, IAM, tags, HCP health)
LABEL_FILTER="Area:ManagedService" make test

# Only critical tests
LABEL_FILTER="Importance:Critical" make test

# Dry run to see what would execute
LABEL_FILTER="Area:ManagedService" make dry-run
```

## Adding Management Cluster Access

Some tests (HCP namespace health, HostedCluster CR, managed operators) need access to the management cluster. This requires backplane.

### Step 1: Find the management cluster

```bash
# Find MCs for a region/sector
ocm get /api/osd_fleet_mgmt/v1/management_clusters \
  --parameter search="status='ready' AND region='us-west-2' AND sector='main'" \
  | jq -r '.items[] | .name + "\t" + .cluster_management_reference.cluster_id'
```

### Step 2: Login via backplane and save kubeconfig

```bash
# Login to the HC's MC (--manager flag)
ocm backplane login <cluster-id> --manager

# Copy the kubeconfig to a temporary location
cp ~/.kube/config /tmp/mc-kubeconfig.yaml
```

### Step 3: Run tests with MC access

```bash
# Export the MC kubeconfig path
export MC_KUBECONFIG=/tmp/mc-kubeconfig.yaml
export MANAGEMENT_CLUSTER_ID=<mc-cluster-id>

# Run with MC access (tests will now access MC)
OCM_TOKEN=$(ocm token) OCM_ENV=staging CLUSTER_ID=<cluster-id> make test
```

The tests will automatically discover the HCP namespace by querying the HostedCluster CR on the MC. No manual namespace configuration is needed.

## Adding AWS Access

CloudTrail IAM validation and EBS tag tests need AWS credentials for the cluster's AWS account.

### Option 1: Backplane cloud credentials

```bash
# Get AWS creds via backplane
ocm backplane cloud credentials <cluster-id>

# Export them
export AWS_ACCESS_KEY_ID=<from output>
export AWS_SECRET_ACCESS_KEY=<from output>
export AWS_SESSION_TOKEN=<from output>
export HTTPS_PROXY=http://squid.corp.redhat.com:3128

# Run with AWS access
OCM_TOKEN=$(ocm token) OCM_ENV=staging CLUSTER_ID=<id> make test
```

### Option 2: Direct AWS credentials

If you have direct access to the AWS account:

```bash
export AWS_ACCESS_KEY_ID=<key>
export AWS_SECRET_ACCESS_KEY=<secret>
export AWS_DEFAULT_REGION=us-west-2
```

## Finding Test Clusters by Sector

```bash
# List all sectors and their MCs
ocm get /api/osd_fleet_mgmt/v1/management_clusters \
  --parameter search="status='ready'" \
  | jq -r '.items[] | .name + "\t" + .region + "\t" + .sector' \
  | sort -k3

# Find HCP clusters in a specific sector's region
ocm list clusters --parameter search="product.id='rosa' AND hypershift.enabled='true' AND state='ready' AND region.id='us-west-2'" \
  --columns id,name,version.raw_id
```

## Example: Full Test Run

```bash
# Login to staging
ocm login --use-auth-code --url stage

# Pick a cluster
export CLUSTER_ID=2ou98sjcil73402c0uvols2il81dftiv  # ra-auto
export OCM_TOKEN=$(ocm token)
export OCM_ENV=staging

# Run all tests (MC/AWS tests will skip)
make test

# Run only data plane tests
LABEL_FILTER="Area:DataPlane" make test

# Build the test binary
make build

# Run directly with ginkgo flags
./test/e2e/e2e.test --ginkgo.label-filter="Area:DataPlane" --ginkgo.v
```

## What to Expect

| Test | Requires | Skips Without |
|------|----------|---------------|
| Workload Deployment | CLUSTER_ID, OCM_TOKEN | CLUSTER_ID |
| Storage PVC | CLUSTER_ID, OCM_TOKEN | CLUSTER_ID |
| ClusterOperators | CLUSTER_ID, OCM_TOKEN | CLUSTER_ID |
| IAM Validation | AWS creds | AWS creds |
| Infrastructure Tags | AWS creds | AWS creds |
| HCP Namespace Health | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| HostedCluster CR | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| NodePool CR | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| RMO RouteMonitors | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| AVO VpcEndpoints | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| Audit Webhook | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| Cluster Lifecycle | Full AWS infra | CLUSTER_ID (uses existing) |

## Known Issues

- **AWS credential validation**: The AWS SDK loads credentials lazily. If `InitAWSClients` succeeds but creds are invalid, the test fails at the API call rather than skipping. Fixed by eagerly validating credentials with `Retrieve()`.
