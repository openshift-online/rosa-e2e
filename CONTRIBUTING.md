# Contributing to rosa-e2e

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork and set up remotes:
   ```bash
   git clone git@github.com:<your-username>/rosa-e2e.git
   cd rosa-e2e
   git remote add upstream git@github.com:openshift-online/rosa-e2e.git
   ```
3. Install Go 1.24+
4. Verify the build: `make build`

## Development Workflow

1. Create a branch from upstream/main:
   ```bash
   git fetch upstream
   git checkout -b my-feature upstream/main
   ```
2. Make your changes
3. Run checks: `make build && go vet --tags E2Etests ./...`
4. Test locally against a staging cluster (see [docs/local-testing.md](docs/local-testing.md))
5. Push and open a PR against `openshift-online/rosa-e2e`

## Adding Tests

### New test file

1. Create `test/e2e/<name>_test.go` with build tag and package:
   ```go
   //go:build E2Etests

   package e2e
   ```

2. Apply labels to your `Describe` block. Include all platform labels that apply:
   ```go
   var _ = Describe("My Test", labels.High, labels.Positive, labels.HCP, labels.Classic, labels.DataPlane, func() {
   ```

3. Use the suite-level `cfg` and `conn` singletons (defined in `setup.go`). Don't create your own OCM connection.

4. Create a `TestContext` per test and initialize the clients you need:
   ```go
   tc := framework.NewTestContext(cfg, conn)
   Expect(tc.InitHCClients()).To(Succeed())
   ```

5. Skip gracefully when prerequisites aren't available:
   ```go
   if !tc.HasMCAccess() {
       Skip("MANAGEMENT_CLUSTER_ID not configured")
   }
   ```

6. For topology-specific behavior, use the helpers:
   ```go
   if !tc.IsClassic() {
       Skip("Only applies to Classic clusters")
   }
   ```

7. Clean up resources with `DeferCleanup`.

### Platform labels

- Use both `labels.HCP` and `labels.Classic` for tests that work on both topologies (data plane, networking, storage, OCM API health, customer features)
- Use only `labels.HCP` for tests that require MC/SC access or HCP-specific CRs (HostedCluster, NodePool)
- Use only `labels.Classic` for tests that use MachinePool API or Classic-specific upgrade policies

### New verifier

- For kube-client checks: implement the `ClusterVerifier` interface in `pkg/verifiers/`
- For OCM/AWS checks: create a standalone `Verify*` function
- Accept `context.Context` as the first parameter and use `SendContext(ctx)` for cancellation
- Use the dynamic client for CRDs to avoid importing heavy API types

## Code Style

- No comments unless explaining a non-obvious "why"
- Use existing domain vocabulary for function/variable names
- Follow existing patterns in neighboring files
- `go vet` must pass

## PR Guidelines

- Keep PRs focused. One logical change per PR.
- Include the test area or component in the PR title
- If adding a new topology or test area, update README.md and AGENTS.md
- If adding new config vars, update the Configuration tables in README.md

## Running Tests Locally

See [docs/local-testing.md](docs/local-testing.md) for detailed instructions on running against staging clusters.

Quick version:
```bash
export OCM_TOKEN=$(ocm token)
export OCM_ENV=staging
export CLUSTER_ID=<your-cluster-id>
make test
```

## CI

PRs run unit tests and lint automatically. E2E tests run as periodic jobs against staging. See [docs/ci-setup.md](docs/ci-setup.md) for details.
