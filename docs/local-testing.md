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

### Step 2: Login via backplane

```bash
# Login to the HC's MC (--manager flag)
ocm backplane login <cluster-id> --manager

# This prints the HCP namespace info:
#   export HC_NAMESPACE=ocm-staging-<cluster-id>
#   export HCP_NAMESPACE=ocm-staging-<cluster-id>-<cluster-name>
```

### Step 3: Note the namespace format

On staging, HCP namespaces follow this pattern:
- HostedCluster CR namespace: `ocm-staging-<cluster-id>`
- HCP deployments namespace: `ocm-staging-<cluster-id>-<cluster-name>`

The tests currently use `CLUSTER_ID` as the HCP namespace, which is correct for production but not staging. This is a known issue being addressed.

### Step 4: Run MC tests manually

Until the namespace resolution is automated, you can verify MC health directly:

```bash
# Check HCP deployments
oc get deployments -n ocm-staging-<cluster-id>-<cluster-name> --no-headers | wc -l

# Check HostedCluster CR
oc get hostedclusters -n ocm-staging-<cluster-id> -o wide

# Check NodePool CRs
oc get nodepools -n ocm-staging-<cluster-id> -o wide
```

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
| HCP Namespace Health | MANAGEMENT_CLUSTER_ID | MC access |
| HostedCluster CR | MANAGEMENT_CLUSTER_ID | MC access |
| NodePool CR | MANAGEMENT_CLUSTER_ID | MC access |
| RMO RouteMonitors | MANAGEMENT_CLUSTER_ID | MC access |
| AVO VpcEndpoints | MANAGEMENT_CLUSTER_ID | MC access |
| Audit Webhook | MANAGEMENT_CLUSTER_ID | MC access |
| Cluster Lifecycle | Full AWS infra | CLUSTER_ID (uses existing) |

## Known Issues

- **HCP namespace format**: Staging uses `ocm-staging-<cluster-id>-<cluster-name>` for the HCP namespace, not just the cluster ID. The tests need to be updated to resolve this dynamically.
- **MC credentials**: OCM's credentials API doesn't work for management clusters. Tests need backplane integration or a pre-created kubeconfig.
- **AWS credential validation**: The AWS SDK loads credentials lazily. If `InitAWSClients` succeeds but creds are invalid, the test fails at the API call rather than skipping. Fixed by eagerly validating credentials with `Retrieve()`.
