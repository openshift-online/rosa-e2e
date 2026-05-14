# OSD GCP Testing Guide

This guide describes how to run E2E tests for OSD clusters on GCP.

## Prerequisites

1. **OCM Authentication**
   ```bash
   ocm login --url=staging  # or production
   export OCM_TOKEN=$(ocm token)
   ```

2. **GCP Infrastructure**
   - A GCP project with OSD enabled
   - Workload Identity Federation (WIF) configuration created in OCM
   - Export required environment variables:
     ```bash
     export GCP_PROJECT_ID=your-gcp-project-id
     export GCP_WIF_CONFIG=your-wif-config-id  # From OCM WIF config
     ```

3. **Configuration File**
   Use the provided `configs/osd-gcp-stage.yaml` or create your own:
   ```yaml
   ocm_env: staging
   cluster_topology: osd-gcp
   gcp_project_id: ""  # Set via GCP_PROJECT_ID env var
   gcp_region: us-central1
   gcp_wif_config: ""  # REQUIRED: Set via GCP_WIF_CONFIG env var
   cluster_name_prefix: e2e-gcp
   compute_machine_type: n2-standard-4
   compute_nodes: 3
   channel_group: stable
   ```
   
   **Note:** `CLUSTER_CONFIG` supports both absolute and relative paths. Relative paths are resolved from your current working directory.

## Running Tests

### Full Lifecycle Test (Create → Verify → Delete)

Creates a new cluster, verifies it's healthy, then deletes it:

```bash
# Set environment variables
export OCM_TOKEN=$(ocm token)
export OCM_ENV=staging
export GCP_PROJECT_ID=your-project-id
export GCP_WIF_CONFIG=your-wif-config-id

# Option 1: Relative path (recommended)
export CLUSTER_CONFIG=configs/osd-gcp-stage.yaml

# Option 2: Absolute path
# export CLUSTER_CONFIG=/path/to/rosa-e2e/configs/osd-gcp-stage.yaml

# Build test binary
make build

# Run GCP lifecycle tests
LABEL_FILTER="Platform:OSD-GCP && Area:ClusterLifecycle" make test
```

### Preserve Cluster After Test

To keep the cluster after testing (for debugging or manual inspection):

```bash
export PRESERVE_CLUSTERS=true
LABEL_FILTER="Platform:OSD-GCP && Area:ClusterLifecycle" make test
```

The test will create and verify the cluster but skip deletion. You'll see:
```
PRESERVE_CLUSTERS=true: Cluster <cluster-id> will not be deleted
To delete manually: ocm delete cluster <cluster-id>
```

### Existing Cluster Verification

Verify an existing GCP cluster is healthy:

```bash
export OCM_TOKEN=$(ocm token)
export CLUSTER_ID=your-cluster-id
export CLUSTER_TOPOLOGY=osd-gcp

# Run existing cluster health check
LABEL_FILTER="Platform:OSD-GCP && Area:ClusterLifecycle" make test
```

### Dry Run (List Tests Without Executing)

```bash
LABEL_FILTER="Platform:OSD-GCP" make dry-run
```

## Test Labels

GCP tests use the following labels:

- **Platform**: `Platform:OSD-GCP` - Filters to GCP-specific tests
- **Area**: Various areas like `Area:ClusterLifecycle`, `Area:DataPlane`, etc.
- **Importance**: `Critical`, `High`, `Medium`, `Low`
- **Speed**: `Slow` for lifecycle tests (cluster creation takes 30-45 minutes)

## Example: Filter by Multiple Criteria

```bash
# Run critical GCP tests only
LABEL_FILTER="Platform:OSD-GCP && Importance:Critical" make test

# Run all GCP tests except slow ones
LABEL_FILTER="Platform:OSD-GCP && !Speed:Slow" make test
```

## Configuration Options

### Environment Variables (Override YAML Config)

- `OCM_TOKEN` - OCM authentication token (required)
- `OCM_ENV` - OCM environment: `staging`, `production`, `integration`
- `CLUSTER_TOPOLOGY` - Force topology: `osd-gcp`, `hcp`, `classic`
- `CLUSTER_ID` - Use existing cluster instead of creating new one
- `GCP_PROJECT_ID` - GCP project ID for cluster creation
- `GCP_REGION` - GCP region (default: `us-central1`)
- `GCP_WIF_CONFIG` - **REQUIRED** for cluster creation: WIF configuration ID from OCM
- `COMPUTE_MACHINE_TYPE` - Machine type (default: `n2-standard-4`)
- `COMPUTE_NODES` - Number of compute nodes (default: 3)
- `OPENSHIFT_VERSION` - Specific OpenShift version (optional)
- `CHANNEL_GROUP` - Version channel: `stable`, `candidate`, `fast`
- `PRESERVE_CLUSTERS` - Set to `true` to skip cluster deletion after tests (default: `false`)

### GCP-Specific Settings

- **Region**: `us-central1` is the default. Other options: `us-east1`, `us-west1`, `europe-west1`
- **Machine Types**: Use GCP machine types like `n2-standard-4`, `n2-standard-8`, `n2-highmem-4`
- **Multi-AZ**: GCP clusters are created as multi-AZ by default

## Test Structure

### Current Tests

1. **Full Lifecycle** (`gcp_lifecycle_test.go`)
   - Creates OSD GCP cluster
   - Waits for cluster ready (up to 45 minutes)
   - Verifies cluster health via OCM API
   - Verifies cluster health via Kubernetes API
   - Deletes cluster
   - Verifies cluster uninstalling

2. **Existing Cluster** (`gcp_lifecycle_test.go`)
   - Verifies existing cluster is ready in OCM
   - Verifies nodes are ready via Kubernetes API

### Adding New Tests

Follow the rosa-e2e pattern:

```go
//go:build E2Etests

package e2e

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/openshift-online/rosa-e2e/pkg/framework"
    "github.com/openshift-online/rosa-e2e/pkg/labels"
)

var _ = Describe("Your Test", labels.OSDGCP, labels.YourArea, func() {
    It("should do something", func(ctx context.Context) {
        if cfg.ClusterID == "" {
            Skip("CLUSTER_ID not set")
        }
        
        tc := framework.NewTestContext(cfg, conn)
        
        // Skip if not GCP cluster
        if !tc.IsOSDGCP() {
            Skip("Not an OSD-GCP cluster")
        }
        
        // Your test logic here
    })
})
```

## Troubleshooting

### "No valid OCM token"
```bash
ocm login --url=staging
export OCM_TOKEN=$(ocm token)
```

### "GCP_WIF_CONFIG environment variable is required"
You must provide a WIF configuration ID. To get this:
1. Create a WIF config via OCM API or UI
2. Export the WIF config ID:
```bash
export GCP_WIF_CONFIG=your-wif-config-id
```

Or set it in your config YAML file:
```yaml
gcp_wif_config: "your-wif-config-id"
```

### "GCP project not found"
Verify your GCP_PROJECT_ID is correct:
```bash
echo $GCP_PROJECT_ID
```

### "Version not found for topology osd-gcp"
The test tries to find the latest available OpenShift version for GCP. If this fails, specify a version manually:
```bash
export OPENSHIFT_VERSION=4.14.0
```

### Cluster creation times out
GCP clusters can take 30-45 minutes to provision. The default timeout is 45 minutes. For slow networks, you may need to increase this in the test code.

## CI Integration

The tests are designed to run in CI with the `E2Etests` build tag:

```bash
ginkgo build --tags=E2Etests test/e2e
./e2e.test --label-filter="Platform:OSD-GCP"
```

JUnit reports are automatically generated in `test-results/` directory.
