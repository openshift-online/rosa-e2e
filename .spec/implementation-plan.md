# ROSA E2E Test Framework - Implementation Plan

## Architecture

### Directory Structure

```
rosa-e2e/
├── .spec/                          # Requirements and planning (this directory)
├── cmd/
│   └── rosa-e2e/
│       └── main.go                 # CLI entry point (optional, for suite runner)
├── test/
│   └── e2e/
│       ├── e2e_test.go             # Ginkgo test suite bootstrap
│       ├── setup.go               # BeforeSuite/AfterSuite setup and teardown
│       └── cluster_lifecycle_test.go  # Initial test: create and delete ROSA HCP cluster
├── pkg/
│   ├── framework/
│   │   ├── context.go             # TestContext: per-test isolated context with cleanup
│   │   ├── ocm.go                 # OCM client helpers (create cluster, wait, delete)
│   │   └── kube.go                # Kubernetes client helpers (get kubeconfig, client-go)
│   ├── config/
│   │   └── config.go             # Configuration loading (env vars, YAML, defaults)
│   ├── verifiers/
│   │   ├── cluster.go            # Cluster-level verifiers (status, health)
│   │   └── nodes.go              # Node verifiers (readiness, count)
│   └── labels/
│       └── labels.go             # Ginkgo label constants
├── configs/
│   └── rosa-hcp-default.yaml     # Default ROSA HCP cluster parameters
├── Makefile                       # Build, test, lint targets
├── Dockerfile                     # Container image for ci-operator
├── go.mod
├── go.sum
└── README.md
```

### Design Principles (from ARO-HCP)

1. **Framework/Test/Verifier separation**: `pkg/framework/` provides test infrastructure, `test/e2e/` contains test specifications, `pkg/verifiers/` provides reusable assertions.

2. **TestContext pattern**: Each test gets a `TestContext` that manages lifecycle and cleanup. Resources created through the context are automatically cleaned up via `DeferCleanup()`.

3. **Outside-in**: Test customer-facing operations (create cluster, delete cluster) before internal concerns (operator health, node labels).

4. **Label-based selection**: All tests carry labels for importance, environment, and speed to enable flexible CI job configuration.

## Implementation Steps

### Phase 1: Project Scaffolding

**Step 1.1: Initialize Go module**
```bash
cd rosa-e2e
go mod init github.com/openshift/rosa-e2e
```

Dependencies:
- `github.com/onsi/ginkgo/v2`
- `github.com/onsi/gomega`
- `github.com/openshift-online/ocm-sdk-go`
- `k8s.io/client-go`
- `k8s.io/apimachinery`

**Step 1.2: Create Makefile**

Targets:
- `build`: `ginkgo build --tags E2Etests ./test/e2e/`
- `test`: `ginkgo run --tags E2Etests ./test/e2e/`
- `lint`: `golangci-lint run`
- `image`: `podman build -t rosa-e2e .`

**Step 1.3: Create Dockerfile**

Multi-stage build:
1. Builder stage: Go build of e2e binary
2. Runtime stage: Minimal image with binary, `oc`, and `ocm` CLIs

### Phase 2: Configuration

**Step 2.1: `pkg/config/config.go`**

Configuration struct loaded from environment variables with YAML file override:

```go
type Config struct {
    OCMEnv        string // OCM_ENV: staging, production, integration
    OCMToken      string // OCM_TOKEN: offline token for authentication
    ClusterID     string // CLUSTER_ID: reuse existing cluster (skip provision)
    AWSRegion     string // AWS_REGION: default us-east-1
    ClusterConfig string // CLUSTER_CONFIG: path to YAML cluster params
}
```

**Step 2.2: `configs/rosa-hcp-default.yaml`**

Default cluster parameters for ROSA HCP:

```yaml
name_prefix: "e2e"
product: rosa
hypershift: true
cloud_provider: aws
region: us-east-1
multi_az: false
compute_nodes: 2
compute_machine_type: m5.xlarge
channel_group: stable
```

### Phase 3: Framework

**Step 3.1: `pkg/labels/labels.go`**

```go
package labels

import "github.com/onsi/ginkgo/v2"

var (
    Critical    = ginkgo.Label("Critical")
    High        = ginkgo.Label("High")
    Medium      = ginkgo.Label("Medium")
    Low         = ginkgo.Label("Low")
    Positive    = ginkgo.Label("Positive")
    Negative    = ginkgo.Label("Negative")
    Slow        = ginkgo.Label("Slow")
    Integration = ginkgo.Label("Integration")
    Stage       = ginkgo.Label("Stage")
    Production  = ginkgo.Label("Production")
)
```

**Step 3.2: `pkg/framework/context.go`**

TestContext provides per-test isolation:

```go
type TestContext struct {
    OCMConnection *sdk.Connection
    Config        *config.Config
    ClusterID     string
    cleanups      []func()
}

func NewTestContext(cfg *config.Config, conn *sdk.Connection) *TestContext
func (tc *TestContext) DeferCleanup(fn func())
func (tc *TestContext) Cleanup()
```

**Step 3.3: `pkg/framework/ocm.go`**

OCM client helpers:

```go
func NewOCMConnection(cfg *config.Config) (*sdk.Connection, error)
func CreateRosaHCPCluster(conn *sdk.Connection, params ClusterParams) (string, error)
func WaitForClusterReady(conn *sdk.Connection, clusterID string, timeout time.Duration) error
func DeleteCluster(conn *sdk.Connection, clusterID string) error
func WaitForClusterDeleted(conn *sdk.Connection, clusterID string, timeout time.Duration) error
func GetClusterKubeconfig(conn *sdk.Connection, clusterID string) (*rest.Config, error)
```

**Step 3.4: `pkg/framework/kube.go`**

Kubernetes client helpers:

```go
func NewKubeClient(restConfig *rest.Config) (kubernetes.Interface, error)
func GetNodes(client kubernetes.Interface) ([]v1.Node, error)
```

### Phase 4: Verifiers

**Step 4.1: `pkg/verifiers/cluster.go`**

```go
func ClusterIsReady(conn *sdk.Connection, clusterID string) error
func ClusterIsDeleting(conn *sdk.Connection, clusterID string) error
```

**Step 4.2: `pkg/verifiers/nodes.go`**

```go
func AllNodesReady(client kubernetes.Interface) error
func NodeCount(client kubernetes.Interface, expected int) error
```

### Phase 5: Test Suite

**Step 5.1: `test/e2e/e2e_test.go`**

```go
//go:build E2Etests

package e2e

import (
    "testing"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestROSAE2E(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "ROSA E2E Suite")
}
```

**Step 5.2: `test/e2e/setup.go`**

Suite-level setup:

```go
var (
    cfg  *config.Config
    conn *sdk.Connection
)

var _ = BeforeSuite(func() {
    cfg = config.Load()
    var err error
    conn, err = framework.NewOCMConnection(cfg)
    Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
    if conn != nil {
        conn.Close()
    }
})
```

**Step 5.3: `test/e2e/cluster_lifecycle_test.go`**

The initial test case:

```go
var _ = Describe("ROSA HCP Cluster Lifecycle", labels.Critical, labels.Positive, labels.Slow, func() {
    var tc *framework.TestContext

    BeforeEach(func() {
        tc = framework.NewTestContext(cfg, conn)
    })

    AfterEach(func() {
        tc.Cleanup()
    })

    It("should create and delete a ROSA HCP cluster", func() {
        By("Creating a ROSA HCP cluster")
        clusterID, err := framework.CreateRosaHCPCluster(conn, clusterParams)
        Expect(err).NotTo(HaveOccurred())
        tc.DeferCleanup(func() {
            framework.DeleteCluster(conn, clusterID)
        })

        By("Waiting for cluster to be ready")
        err = framework.WaitForClusterReady(conn, clusterID, 45*time.Minute)
        Expect(err).NotTo(HaveOccurred())

        By("Verifying cluster health")
        kubeConfig, err := framework.GetClusterKubeconfig(conn, clusterID)
        Expect(err).NotTo(HaveOccurred())

        kubeClient, err := framework.NewKubeClient(kubeConfig)
        Expect(err).NotTo(HaveOccurred())

        err = verifiers.AllNodesReady(kubeClient)
        Expect(err).NotTo(HaveOccurred())

        By("Deleting the cluster")
        err = framework.DeleteCluster(conn, clusterID)
        Expect(err).NotTo(HaveOccurred())

        By("Verifying cluster is deleting")
        err = verifiers.ClusterIsDeleting(conn, clusterID)
        Expect(err).NotTo(HaveOccurred())
    })
})
```

### Phase 6: Build & CI Readiness

**Step 6.1: Makefile**

```makefile
.PHONY: build test lint image clean

build:
	ginkgo build --tags E2Etests ./test/e2e/

test:
	ginkgo run --tags E2Etests --junit-report junit-report.xml ./test/e2e/

lint:
	golangci-lint run ./...

image:
	podman build -t rosa-e2e:latest .

clean:
	rm -f test/e2e/e2e.test
```

**Step 6.2: Dockerfile**

```dockerfile
FROM registry.access.redhat.com/ubi9/go-toolset:latest AS builder
WORKDIR /app
COPY . .
RUN go install github.com/onsi/ginkgo/v2/ginkgo@latest && \
    ginkgo build --tags E2Etests ./test/e2e/

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
COPY --from=builder /app/test/e2e/e2e.test /rosa-e2e
ENTRYPOINT ["/rosa-e2e"]
```

**Step 6.3: ci-operator config (future)**

The test image can be referenced in ci-operator config as a container test:

```yaml
tests:
- as: rosa-hcp-e2e
  container:
    from: rosa-e2e
    clone: false
  commands: /rosa-e2e --ginkgo.label-filter "Critical" --ginkgo.junit-report ${ARTIFACT_DIR}/junit.xml
  secret:
    name: rosa-e2e-credentials
    mount_path: /etc/rosa-e2e
```

## Implementation Order

1. Phase 1 (Scaffolding) - Project structure, go.mod, Makefile
2. Phase 2 (Configuration) - Config loading from environment
3. Phase 3 (Framework) - Labels, TestContext, OCM helpers, Kube helpers
4. Phase 4 (Verifiers) - Cluster and node verifiers
5. Phase 5 (Test Suite) - Suite bootstrap, setup, initial test case
6. Phase 6 (Build & CI) - Dockerfile, ci-operator readiness

## Future Test Cases

After the initial framework is validated with the cluster lifecycle test, candidates for next tests include:

1. **ROSA HCP Backup/Restore** - Port from `ocm-cs-qe-fvt-rosa-hcp-backup-restore` in ocm-backend-tests
2. **Node Pool Operations** - Create, scale, delete node pools
3. **Cluster Upgrade** - Trigger and verify managed upgrade
4. **IDP Configuration** - Add identity providers to cluster
5. **Private Link** - Create and verify private cluster
