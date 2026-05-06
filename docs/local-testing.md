# Local Testing Guide

How to run rosa-e2e tests from your laptop against staging clusters. The test suite supports ROSA HCP, ROSA Classic STS, and (planned) OSD GCP topologies.

## Prerequisites

- Go 1.24+
- `ocm` CLI logged into staging: `ocm login --use-auth-code --url stage`
- `rosa` CLI (for cluster provisioning)
- `osdctl` CLI (for AWS credentials)
- `ocm-backplane` CLI (for management cluster access)

## Provision Your Own Cluster

Don't use someone else's cluster. Provision a dedicated one with known configuration:

```bash
# Step 1: Get AWS credentials for your dev account
# List your accounts:
osdctl account mgmt list -u <your-username> -p osd-staging-2

# Get credentials (exports AWS env vars):
eval $(echo "y" | osdctl account cli -i <ACCOUNT_ID> -p osd-staging-2 -r us-east-2 -oenv 2>/dev/null | tr '\n' ' ' | sed 's/.*AWS_ACCESS/AWS_ACCESS/')

# Step 2: Provision (creates VPC, account roles, OIDC, and cluster)
./scripts/provision-e2e-cluster.sh

# Step 3: Wait for cluster to be ready (~15 min)
rosa logs install -c rosa-e2e-$(date +%m%d) --watch

# Step 4: Run tests
source /tmp/rosa-e2e-cluster.env
OCM_TOKEN=$(ocm token) make test

# Step 5: Clean up when done
source /tmp/rosa-e2e-cluster.env
./scripts/deprovision-e2e-cluster.sh
```

Customize with environment variables:
```bash
CLUSTER_NAME=my-test REGION=us-west-2 COMPUTE_NODES=3 ./scripts/provision-e2e-cluster.sh
```

## Quick Start: Test Against Any Existing Cluster

The simplest path. The framework auto-detects whether the cluster is HCP or Classic.

```bash
# Find a ready cluster in staging
ocm list clusters --parameter search="product.id='rosa' AND state='ready'" --columns id,name,region.id

# Run tests (topology auto-detected)
export OCM_TOKEN=$(ocm token)
export OCM_ENV=staging
export CLUSTER_ID=<cluster-id>
make test
```

This runs all tests applicable to the detected topology. Tests requiring MC, SC, or AWS access skip gracefully.

### Classic STS clusters

```bash
# Find a Classic STS cluster
ocm list clusters --parameter search="product.id='rosa' AND hypershift.enabled='false' AND state='ready'" --columns id,name,region.id

# Run Classic-only tests
export CLUSTER_ID=<classic-cluster-id>
LABEL_FILTER="Platform:Classic" make test
```

### HCP clusters

```bash
# Find an HCP cluster
ocm list clusters --parameter search="product.id='rosa' AND hypershift.enabled='true' AND state='ready'" --columns id,name,region.id

# Run HCP-only tests
export CLUSTER_ID=<hcp-cluster-id>
LABEL_FILTER="Platform:HCP" make test
```

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

### Tests that run on all topologies (HCP + Classic)

| Test | Requires | Skips Without |
|------|----------|---------------|
| Workload Deployment | CLUSTER_ID, OCM_TOKEN | CLUSTER_ID |
| Storage PVC | CLUSTER_ID, OCM_TOKEN | CLUSTER_ID |
| ClusterOperators | CLUSTER_ID, OCM_TOKEN | CLUSTER_ID |
| Networking (DNS, connectivity) | CLUSTER_ID, OCM_TOKEN | CLUSTER_ID |
| RBAC, Ingress, Log Forwarding | CLUSTER_ID, OCM_TOKEN | CLUSTER_ID |
| KMS, PrivateLink | CLUSTER_ID, OCM_TOKEN | Skips if not configured |
| IAM Validation (CloudTrail) | AWS creds | AWS creds |
| Infrastructure Tags (EBS) | AWS creds | AWS creds |
| OCM API Health | OCM_TOKEN | - |
| Cluster Service Health | CLUSTER_ID, OCM_TOKEN | CLUSTER_ID |

### HCP-only tests

| Test | Requires | Skips Without |
|------|----------|---------------|
| HCP Namespace Health | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| HostedCluster CR | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| NodePool CR | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| RMO RouteMonitors | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| AVO VpcEndpoints | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| Audit Webhook | MC_KUBECONFIG or MANAGEMENT_CLUSTER_ID | MC access |
| SC Health (ACM, Hive, MCE) | SERVICE_CLUSTER_ID | SC access |
| OSDFM Health | OCM_TOKEN with OSDFM access | 403 Forbidden |
| NodePool Upgrade | CLUSTER_ID, UPGRADE_TARGET_VERSION | Version not set |
| HCP Lifecycle (Full) | Full AWS infra + OIDC config | CLUSTER_ID (uses existing) |

### Classic-only tests

| Test | Requires | Skips Without |
|------|----------|---------------|
| MachinePool List/Verify | CLUSTER_ID (Classic topology) | Non-Classic cluster |
| Classic Upgrade | CLUSTER_ID, UPGRADE_TARGET_VERSION | Version not set |
| Classic Lifecycle (Full) | Full AWS infra (no OIDC needed) | CLUSTER_ID (uses existing) |

## Scripts Reference

| Script | Purpose |
|--------|---------|
| `scripts/provision-e2e-cluster.sh` | Create VPC, account roles, OIDC config, and ROSA HCP cluster |
| `scripts/deprovision-e2e-cluster.sh` | Delete cluster, operator roles, OIDC provider, and VPC |
| `scripts/run-e2e.sh` | Run tests (works both locally and in Prow CI) |

## Known Issues

- **AWS credential validation**: The AWS SDK loads credentials lazily. `InitAWSClients` uses eager `Retrieve()` to fail fast and skip when creds are unavailable.
- **HCP regions**: Not all regions support HCP in staging. us-east-2 and us-west-2 are reliable choices.
- **Classic provisioning time**: Classic STS clusters take longer to provision (~30-45 min) compared to HCP (~15 min). The lifecycle test uses a 60-minute timeout.
- **Topology detection**: If `CLUSTER_TOPOLOGY` is not set, the framework queries the OCM API to detect topology. This adds one API call per test context creation. Set `CLUSTER_TOPOLOGY` explicitly to avoid this overhead.
