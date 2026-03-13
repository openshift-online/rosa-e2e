# ROSA E2E Test Framework - Requirements

## Problem Statement

ROSA end-to-end tests are split between two fundamentally different systems:

1. **OCM Backend Tests** (ocm-backend-tests): Tests OCM API contracts (Clusters Service, AMS, OSDFM). Runs in Tekton/Konflux. Cannot run in Prow.
2. **osde2e**: Tests platform health (operators, networking, storage). Runs in Prow. Does not test OCM API contracts.

Neither system provides a unified view of ROSA quality. There is no single framework that tests both the OCM layer (cluster creation via API, backup/restore, upgrades) and the platform layer (cluster health, operator status, workload deployment) together.

## Goals

### G1: Unified ROSA E2E Framework
A single test framework that can validate both OCM-level operations and platform-level verification for ROSA clusters.

### G2: Prow Native
Tests must be designed to run in Prow via ci-operator, using the step registry and multi-stage test workflows defined in `openshift/release`.

### G3: ARO-HCP Methodology
Follow the ARO-HCP e2e test design patterns:
- Per-test isolation where feasible
- Outside-in testing (customer operations first)
- Reusable verifiers as composable assertions
- Label-based test selection for environment filtering
- Framework/test/verifier architectural separation
- Designed to run anywhere (local, CI, any OCM environment)

### G4: Incremental Adoption
Start with a single test case and expand. The framework must be designed for growth but only implement what is needed now.

## Non-Goals

- Replacing ocm-backend-tests or osde2e entirely
- Testing OSD (non-ROSA) cluster types initially
- Testing ROSA Classic initially (start with HCP)
- Implementing CI pipeline configuration in openshift/release (design for it, implement later)

## Functional Requirements

### FR1: Test Execution
- Tests written in Go using Ginkgo v2 / Gomega
- Tests compiled with a build tag (e.g., `E2Etests`) to separate from unit tests
- Executable as a standalone binary or via `ginkgo run`
- Support `--ginkgo.label-filter` for test selection
- Generate JUnit XML reports

### FR2: OCM Integration
- Connect to OCM API (staging, production, integration) via `ocm-sdk-go`
- Authenticate using OCM tokens or client credentials
- Create, monitor, and delete ROSA HCP clusters via OCM Clusters Service API
- Query cluster status, node pools, and other OCM resources

### FR3: Platform Integration
- Obtain kubeconfig for provisioned clusters (via OCM backplane or direct)
- Execute Kubernetes API calls against provisioned clusters
- Verify cluster health (nodes, operators, pods)

### FR4: Cloud Provider Integration
- AWS SDK integration for verifying cloud resources
- Support AWS credential configuration via environment variables or shared credentials file

### FR5: Configuration
- Environment selection via environment variable (`OCM_ENV` or similar)
- Cluster parameters configurable via YAML files and/or environment variables
- Support for reusing an existing cluster (`--cluster-id`) to skip provisioning

### FR6: Labeling
- Tests labeled by importance: Critical, High, Medium, Low
- Tests labeled by environment: Integration, Stage, Production
- Tests labeled by type: Positive, Negative
- Tests labeled by speed: Fast, Slow

## Initial Test Case

### "Create and Delete a ROSA HCP Cluster"

This is the foundational test validating the most critical user journey:

1. Create required AWS infrastructure (VPC, subnets, or reuse existing)
2. Create ROSA HCP cluster via OCM API
3. Wait for cluster to reach "ready" state
4. Verify cluster health (nodes ready, operators available)
5. Delete cluster via OCM API
6. Verify cluster reaches "uninstalling" state

This test exercises both OCM (create/delete via API) and Platform (health verification on the running cluster) concerns.

## Constraints

- Must use Go and Ginkgo v2 (consistent with all three analyzed suites)
- Must produce JUnit XML compatible with Prow artifact collection
- Must not require any custom CI tooling (ocmtest, etc.) - standard `ginkgo` binary only
- Must be buildable as a container image for ci-operator
- AWS credentials provided via standard environment variables
