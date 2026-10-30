# Minimum OCP Version Coverage

## Overview

**Why it matters:** Regressions on older OCP versions reach customers first. Clusters running minimum supported versions in production are exposed to bugs that may not surface in testing against latest versions. This test suite provides continuous validation that the oldest OCP versions we support remain healthy and functional.

The min-version test suite automatically discovers the minimum OCP versions deployed in the `prod:canary` sector and runs health checks against clusters running those versions. This ensures we catch version-specific regressions before they impact customers.

## Architecture

### Version Discovery

Min-version tests use the `GetMinimumOCPVersions()` function from `pkg/framework/version.go` to query the OCM API:

1. **Sector query**: Lists all clusters in the `prod:canary` sector
2. **Topology filtering**: Filters by cluster type (HCP, Classic, OSD-GCP)
3. **Version extraction**: Extracts unique OCP versions from matching clusters
4. **Validation**: Validates version format and deduplicates
5. **Graceful empty results**: Returns empty list (not error) if no clusters found

**Note:** The sector is currently hardcoded to `prod:canary` in the test suite. To target a different sector, modify the test code in `test/e2e/min_version_test.go`.

### Parameterized Tests

Tests are parameterized over discovered versions:

- **Discovery test** (`min_version_test.go`): Validates that at least one version is discovered in prod:canary (fails if empty)
- **Health tests** (`min_version_test.go`): Verify worker nodes and cluster operators are ready on min-version clusters (skip if no versions found)

The discovery test expects non-empty results to ensure the prod:canary sector is properly configured. The health tests skip gracefully if no versions are discovered.

### Results and Reports

- **JUnit reports**: 
  - `junit-min-version-report.xml` - All min-version tests (all topologies)
  - `junit-min-version-hcp-report.xml` - HCP-only results
  - `junit-min-version-classic-report.xml` - Classic-only results
- **Console output**: Ginkgo's `-v` flag prints discovered versions to stdout
- **Failures are distinguishable**: Separate JUnit files prevent min-version failures from being mixed with regular e2e test results

## Running Locally

### Prerequisites

- Go 1.26+
- `ocm` CLI logged into production: `ocm login --url production`
- OCM token with read access to cluster metadata
- For hosted cluster access: Kubeconfig via `KUBECONFIG` env var (or OCM credentials API with `CLUSTER_ID`)
- For HCP management cluster tests: MC kubeconfig via `MC_KUBECONFIG` env var (or OCM credentials API with `MANAGEMENT_CLUSTER_ID`)

### Run All Min-Version Tests

```bash
OCM_TOKEN=$(ocm token) OCM_ENV=production make min-version-test
```

This runs tests labeled `Area:Upgrade` or `Area:ManagedService`. Note that this includes all upgrade and managed-service tests across the suite (not just the min-version-specific tests in `min_version_test.go`), so results will include cluster operator checks, infrastructure health, and upgrade verification tests.

**Environment variables:**
- `OCM_TOKEN`: Required. Your OCM API token.
- `OCM_ENV`: Required. Must be `production` to target prod:canary sector.
- `KUBECONFIG`: Optional. Path to hosted cluster kubeconfig for data plane tests.
- `CLUSTER_ID`: Optional. Cluster ID for OCM credentials API (if `KUBECONFIG` not provided).
- `MC_KUBECONFIG`: Optional. Path to management cluster kubeconfig for HCP infrastructure tests. If unset, uses `MANAGEMENT_CLUSTER_ID` to fetch credentials via OCM.

### Run for Specific Topology

**HCP only:**
```bash
OCM_TOKEN=$(ocm token) OCM_ENV=production make min-version-test-hcp
```

**Classic only:**
```bash
OCM_TOKEN=$(ocm token) OCM_ENV=production make min-version-test-classic
```

These topology-specific targets filter tests by `Platform:HCP` or `Platform:Classic` in addition to the `Area:Upgrade || Area:ManagedService` filter.

### Dry-Run (List Tests Without Executing)

```bash
make dry-run
```

Or with label filter:
```bash
LABEL_FILTER="Area:ManagedService" make dry-run
```

## Interpreting Results

### Successful Run

```
Discovering minimum OCP versions in prod:canary
Min OCP versions in prod:canary (HCP): [4.14.0 4.15.1]
Discovered 2 minimum version(s): [4.14.0 4.15.1]
✓ verifies worker nodes are Ready on minimum version clusters
✓ verifies cluster operators are ready on minimum version clusters

Ran 3 of 45 Specs in 12.345 seconds
SUCCESS! -- 3 Passed | 0 Failed | 0 Pending | 42 Skipped
```

**Key indicators:**
- Version list is non-empty
- All health checks pass
- JUnit report written to `junit-min-version-report.xml`

### Common Failures

#### No Versions Discovered

```
No clusters found in prod:canary for HCP topology
```

**Diagnosis:**
- Check if prod:canary sector has active clusters:
  ```bash
  ocm list clusters --parameter search="sector_name = 'prod:canary' AND hypershift.enabled = 't'"
  ```
- Verify OCM token has correct permissions
- Confirm you're targeting the right environment (`OCM_ENV=production`)

**Resolution:** This message indicates the health tests skipped gracefully. However, the discovery test will fail if no versions are found, as it expects the prod:canary sector to have at least one cluster. If clusters should exist, verify sector configuration with cluster-service team.

#### Worker Nodes Not Ready

```
nodes not ready:
  ip-10-0-1-234 (Ready=False)
```

**Diagnosis:**
- Node is stuck provisioning or has failed to join the cluster
- Networking or storage issues preventing node startup
- Version-specific regression in node bootstrap

**Resolution:**
1. Check cluster health in OCM UI
2. Review node events: `oc describe node <node-name>`
3. Check machine/machineset status (Classic) or NodePool status (HCP)
4. File Jira with cluster ID, node name, and OCP version

#### Cluster Operators Not Ready

```
unhealthy clusteroperators found:
  authentication: Available=False, Degraded=True, Progressing=False
```

**Diagnosis:**
- Operator reconciliation in progress (may self-heal)
- Configuration issue specific to min version
- Upstream OCP bug affecting older versions

**Resolution:**
1. Check if operator is progressing: `oc get co/<operator-name> -o yaml`
2. Review operator logs: `oc logs -n openshift-<operator-namespace> deployment/<operator-name>`
3. Check OCM service logs for managed operators
4. If persistent, file Jira with cluster ID, operator name, and OCP version

## Adding New Min-Version Tests

### When to Add a Min-Version Test

Add a min-version test when:
- **Data plane**: Testing workload deployment, networking, storage
- **Cluster operators**: Verifying core OpenShift components (authentication, console, ingress, etc.)
- **Infrastructure**: Testing HyperShift components, nodepool health, control plane stability
- **Version-specific regressions**: Known issues that require validation on older versions

**Do NOT add min-version tests for:**
- Cluster lifecycle (creation/deletion) - these are topology-specific full lifecycle tests
- Features that are version-gated (only available in newer versions)
- AWS IAM/billing (not version-dependent)

### Steps to Add a Test

1. **Create or extend test file** in `test/e2e/`:
   ```go
   //go:build E2Etests

   package e2e

   var _ = Describe("Feature Name", labels.Critical, labels.Positive, labels.ManagedService, func() {
       Context("When running against prod:canary min versions", func() {
           var minVersions []string

           BeforeEach(func(ctx context.Context) {
               By("Discovering minimum OCP versions in prod:canary")
               var err error
               minVersions, err = framework.GetMinimumOCPVersions(ctx, conn, "prod:canary", "hcp")
               Expect(err).NotTo(HaveOccurred())

               if len(minVersions) == 0 {
                   Skip("No clusters found in prod:canary for HCP topology")
               }

               GinkgoWriter.Printf("Discovered %d minimum version(s): %v\n", len(minVersions), minVersions)
           })

           It("your test name", labels.HCP, func(ctx context.Context) {
               tc := framework.NewTestContext(cfg, conn)
               // Initialize required clients
               err := tc.InitHCClients()
               if err != nil {
                   Skip("HC access not configured")
               }

               // Run health checks FIRST to ensure cluster is ready
               err = verifiers.VerifyWorkerNodesReady(ctx, tc.HCDynamicClient())
               Expect(err).NotTo(HaveOccurred())

               // Your test logic here
           })
       })
   })
   ```

2. **Apply labels for all applicable topologies**:
   - Include `labels.HCP`, `labels.Classic`, `labels.OSDGCP` if the test works across topologies
   - Omit labels for topologies that don't apply (e.g., AWS-specific tests should not include `labels.OSDGCP`)
   - Always include area label: `labels.ManagedService` or `labels.Upgrade`

3. **Use graceful skip when topology doesn't apply**:
   ```go
   if tc.IsOSDGCP() {
       Skip("This test requires AWS-specific resources")
   }
   ```

4. **Call health verifiers first**:
   ```go
   err = verifiers.VerifyWorkerNodesReady(ctx, tc.HCDynamicClient())
   Expect(err).NotTo(HaveOccurred(), "all worker nodes should be Ready")

   err = verifiers.VerifyClusterOperatorsReady(ctx, tc.HCDynamicClient())
   Expect(err).NotTo(HaveOccurred(), "all cluster operators should be ready")
   ```

5. **Test locally**:
   ```bash
   OCM_TOKEN=$(ocm token) OCM_ENV=production make min-version-test
   ```

6. **Verify labels are correct**:
   ```bash
   make dry-run | grep "your test name"
   ```

### Multi-Topology Example

For a test that works on HCP, Classic, and OSD-GCP:

```go
var _ = Describe("DNS Resolution", labels.Critical, labels.Positive, labels.HCP, labels.Classic, labels.OSDGCP, labels.ManagedService, func() {
    Context("When running against prod:canary min versions", func() {
        var minVersions []string

        BeforeEach(func(ctx context.Context) {
            topology := "hcp"
            if os.Getenv("CLUSTER_TOPOLOGY") == "classic" {
                topology = "classic"
            } else if os.Getenv("CLUSTER_TOPOLOGY") == "osd-gcp" {
                topology = "osd-gcp"
            }

            var err error
            minVersions, err = framework.GetMinimumOCPVersions(ctx, conn, "prod:canary", topology)
            Expect(err).NotTo(HaveOccurred())

            if len(minVersions) == 0 {
                Skip("No clusters found in prod:canary")
            }
        })

        It("resolves cluster internal DNS", func(ctx context.Context) {
            tc := framework.NewTestContext(cfg, conn)
            err := tc.InitHCClients()
            if err != nil {
                Skip("Kubeconfig not configured")
            }

            // Test logic that works across all topologies
        })
    })
})
```

## Debugging a Failure

### Step 1: Identify the Failing Test

Review the JUnit report or console output:
```bash
cat junit-min-version-report.xml | grep '<failure'
```

### Step 2: Run the Failing Test in Isolation

Use `-focus` to run a single test:
```bash
OCM_TOKEN=$(ocm token) OCM_ENV=production \
    ginkgo run --tags E2Etests \
    --focus "verifies worker nodes are Ready" \
    -v ./test/e2e/
```

### Step 3: Check Actual Min Versions in Prod:Canary

Query OCM directly:
```bash
# For HCP
ocm list clusters --parameter search="sector_name = 'prod:canary' AND hypershift.enabled = 't'"

# For Classic
ocm list clusters --parameter search="sector_name = 'prod:canary' AND hypershift.enabled = 'f'"
```

Note: The OCM CLI output is JSON by default. Use `--columns id,openshift_version` for tabular output or pipe through `jq` if needed.

### Step 4: Verify Cluster Health in OCM UI

1. Navigate to https://console.redhat.com/openshift/clusters
2. Search for cluster ID from version discovery output
3. Check **Overview** tab for alerts
4. Check **Monitoring** tab for degraded operators
5. Check **Support** tab for recent service logs

### Step 5: Access the Cluster Directly

For HCP clusters, get management cluster access:
```bash
# If using OCM credentials API
export MANAGEMENT_CLUSTER_ID=<mc-id>
OCM_TOKEN=$(ocm token) OCM_ENV=production make min-version-test-hcp

# Or provide kubeconfig directly
export MC_KUBECONFIG=/path/to/mc-kubeconfig
make min-version-test-hcp
```

Then inspect resources:
```bash
# Worker nodes
oc get nodes

# Cluster operators
oc get co

# NodePools (HCP)
oc get nodepool -A

# MachineSets (Classic)
oc get machineset -n openshift-machine-api
```

### Step 6: File a Jira Issue

Include:
- **Cluster ID** from version discovery output
- **OCP version** that failed
- **Test name** and failure message
- **Topology** (HCP, Classic, OSD-GCP)
- **JUnit report** attachment
- **Timeline** (when did this start failing?)

## Version Discovery Behavior

### Sector Configuration

Min-version tests target the `prod:canary` sector. This sector contains clusters running the minimum supported OCP versions in production.

The sector is hardcoded in the test suite (`test/e2e/min_version_test.go`). To target a different sector, modify the `GetMinimumOCPVersions()` calls in the test code.

### Empty Results Behavior

If the sector has no clusters matching the topology, `GetMinimumOCPVersions()` returns an empty slice (not an error).

**Discovery test**: Fails if no versions are found (expects non-empty to validate prod:canary is configured).  
**Health tests**: Skip gracefully if no versions are found.

This behavior ensures the prod:canary sector is properly populated while allowing health tests to skip when no clusters are available (e.g., during maintenance windows).

### Unhealthy Clusters Are Included

If a cluster in the sector is unhealthy (degraded operators, nodes not ready), it's still included in the version list. This is intentional:
- **Goal**: Catch issues on specific OCP versions
- **Trade-off**: Health tests may fail due to pre-existing cluster issues
- **Mitigation**: Health tests tolerate `Progressing=True` on cluster operators (operators may be converging) and provide detailed error messages to distinguish transient vs. persistent failures

If a min-version test fails consistently, verify the cluster is healthy in OCM UI before investigating code-level issues.

### Version Validation

Discovered versions are validated against a relaxed semver regex (`^\d+\.\d+(\.\d+)?`). Invalid versions are logged and skipped:

```
WARNING: cluster abc123 has malformed version "unknown", skipping
```

### Topology Detection vs. Version Discovery Filtering

**For version discovery** (`GetMinimumOCPVersions`): The topology parameter filters which clusters to query from OCM:
- `"hcp"`: Queries clusters with `hypershift.enabled = true` AND cloud provider = AWS
- `"classic"`: Queries clusters with `hypershift.enabled = false` AND cloud provider = AWS
- `"osd-gcp"`: Queries clusters with `hypershift.enabled = false` AND cloud provider = GCP

**For test execution** (`TestContext.IsHCP()`, etc.): The framework auto-detects topology from the test cluster's OCM metadata or via `CLUSTER_TOPOLOGY` env var (values: `hcp`, `classic`, `osd-gcp`).

These are separate concepts: version discovery filters the prod:canary sector, while topology detection identifies the cluster under test.

## CI Integration

This test suite is designed to run in CI pipelines on a recurring schedule. Integration depends on your CI system:

### Prow (OpenShift CI)

Example periodic job configuration (not directly usable in this repo; adapt for your Prow setup):

```yaml
periodics:
- name: rosa-e2e-min-version-coverage
  interval: 24h
  decorate: true
  spec:
    containers:
    - image: quay.io/openshift/origin-cli:latest
      command:
      - /bin/bash
      args:
      - -c
      - |
        make min-version-test
      env:
      - name: OCM_TOKEN
        valueFrom:
          secretKeyRef:
            name: ocm-token
            key: token
      - name: OCM_ENV
        value: "production"
```

See [docs/ci-setup.md](./ci-setup.md) for actual Prow configuration patterns used in this repo.

### GitHub Actions

Create `.github/workflows/min-version-tests.yaml`:

```yaml
name: Minimum OCP Version Tests

on:
  schedule:
    - cron: '0 4 * * *'  # Daily 4 AM UTC
  workflow_dispatch:

jobs:
  min-version-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.26'
      - name: Run min-version tests
        env:
          OCM_TOKEN: ${{ secrets.OCM_TOKEN }}
        run: |
          OCM_ENV=production make min-version-test
      - name: Upload test results
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: min-version-junit
          path: junit-min-version-report.xml
```

### Jenkins

Add a pipeline job:

```groovy
pipeline {
    agent any
    triggers {
        cron('0 4 * * *')
    }
    environment {
        OCM_TOKEN = credentials('ocm-token')
        OCM_ENV = 'production'
    }
    stages {
        stage('Run Min-Version Tests') {
            steps {
                sh 'make min-version-test'
            }
        }
    }
    post {
        always {
            junit 'junit-min-version-report.xml'
        }
    }
}
```

## Related Documentation

- **Local Testing**: [docs/local-testing.md](./local-testing.md) - Running e2e tests from your laptop
- **CI Setup**: [docs/ci-setup.md](./ci-setup.md) - Prow integration and cluster profiles
- **Architecture**: [CLAUDE.md](../CLAUDE.md) - Codebase structure and patterns
- **Version Discovery**: [pkg/framework/version.go](../pkg/framework/version.go) - Implementation details
- **Verifiers**: [pkg/verifiers/hostedcluster.go](../pkg/verifiers/hostedcluster.go) - Health check functions
