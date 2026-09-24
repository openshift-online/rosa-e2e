# Zero Egress Cluster E2E Test Spec

Test plan for validating zero egress cluster functionality in ROSA HCP.

Jira: [ROSAENG-308](https://redhat.atlassian.net/browse/ROSAENG-308)

## Background

Zero egress clusters are ROSA HCP clusters designed to operate without requiring public internet egress. The documented installation flow can use the standard ROSA VPC template, which includes a NAT gateway and private default route, so zero egress does not by itself prove hard network isolation. OpenShift release images are pulled from AWS ECR mirrors instead of quay.io. The feature is day-1 only and immutable.

### Existing Infrastructure

- **Test profile**: `tests/ci/data/profiles/external.yaml` — profile `ocpe2e-rosa-hcp-zero-egress` (`hcp: true`, `sts: true`, `byo_vpc: true`, `private: true`, `zero_egress: true`)
- **`PrepareZeroEgressResources()`** (`tests/utils/handler/resources_handler_prepare.go:456`) — must provision the minimum endpoint contract defined below; helpers that create only `ecr.dkr` and `s3` are incomplete
- **`ClusterDescription.ZeroEgress`** — parsed from `rosa describe cluster` output
- **OCM SDK** — use the first-class AWS zero-egress field when creating clusters; CLI property behavior can be covered separately as a compatibility test

### Endpoint Contract

The E2E suite owns a capability contract, not the endpoint list implemented by a particular CLI verifier or infrastructure template. A valid zero-egress VPC must provide private AWS paths for the following services:

| Service | Required VPC path | Primary assertion |
|---|---|---|
| STS | Interface endpoint with private DNS | `sts.<region>.amazonaws.com` resolves to endpoint ENI private IPs and accepts TLS connections |
| ECR API | Interface endpoint with private DNS | `api.ecr.<region>.amazonaws.com` resolves to endpoint ENI private IPs and accepts TLS connections |
| ECR registry | Interface endpoint with private DNS | `<account>.dkr.ecr.<region>.amazonaws.com` resolves to endpoint ENI private IPs and image pulls succeed |
| S3 | Gateway endpoint associated with the private subnet route tables, or an equivalent private endpoint path | Endpoint and route-table configuration is valid and ECR image layers can be downloaded without public egress |

EC2, KMS, and other endpoints can be validated when they are provisioned by the selected infrastructure template, but they are template-specific checks rather than part of the initial portable runtime contract. If cluster provisioning proves that one is required across all supported zero-egress configurations, promote it into the table above.

`rosa verify network` is a useful compatibility signal, but its internal endpoint list is not the source of truth for this suite. The authoritative assertions are the VPC endpoint configuration, private DNS behavior where applicable, and successful mirrored image pulls.

## 1. Cluster Creation Validation

### 1.1 Happy path creation

- Create cluster with `--hosted-cp --sts --private --default-ingress-private --subnet-ids <ids> --properties zero_egress:true`
- Verify cluster reaches `ready` state
- Verify `rosa describe cluster` shows `Zero Egress: Enabled`

### 1.2 Negative: Reject on Classic

- Attempt `--properties zero_egress:true` without `--hosted-cp`
- Expect creation to fail with a clear error

### 1.3 Negative: Reject without private

- Attempt `--properties zero_egress:true` without `--private`
- Expect creation to fail

### 1.4 Negative: Reject invalid channel group

- Attempt zero egress with `--channel-group candidate` or `--channel-group nightly`
- Expect failure — only `stable`, `eus`, and `fast` are allowed

### 1.5 Negative: Reject without BYO VPC

- Attempt `--properties zero_egress:true` without `--subnet-ids`
- Expect creation to fail

## 2. Immutability / Day-2 Restrictions

### 2.1 Cannot disable zero egress

- On a running zero egress cluster, attempt `rosa edit cluster --properties zero_egress:false`
- Expect error: "Updating zero egress enabled for a cluster is not supported"

### 2.2 Cannot remove zero egress property

- Attempt to delete the `zero_egress` property via API
- Expect rejection

### 2.3 Describe output correctness

- `rosa describe cluster` on a zero egress cluster shows `Zero Egress: Enabled`
- `rosa describe cluster` on a non-zero-egress cluster shows `Zero Egress: Disabled` or omits the field

## 3. Endpoint Contract Validation

### 3.1 Required endpoint inventory

- Query the AWS VPC endpoint inventory for the cluster VPC
- Require available endpoint paths for `sts`, `ecr.api`, `ecr.dkr`, and `s3`
- Verify interface endpoints have private DNS enabled and are attached to the expected VPC and subnets
- Verify the S3 gateway endpoint is associated with the route tables used by the worker subnets, or validate the equivalent configuration when an S3 interface endpoint is used
- Verify security groups allow HTTPS from the worker subnet or cluster security group to each interface endpoint

### 3.2 Runtime connectivity

- From a cluster workload, resolve and connect to STS, ECR API, and an account-scoped ECR registry hostname
- Verify resolved interface-endpoint addresses match the endpoint ENIs and are private addresses
- Pull a digest-pinned mirrored image to exercise both ECR and the S3-backed image layer path
- Do not require the regional S3 hostname to resolve to an RFC1918 address when a gateway endpoint is used; validate its endpoint route instead

### 3.3 Missing VPC endpoint detection

- Prefer creating an intentionally incomplete test VPC over mutating endpoints used by a running shared cluster
- Omit or misconfigure one required endpoint at a time during dedicated negative cluster-creation tests
- Expect provisioning or readiness to fail with evidence identifying the unavailable service

### 3.4 ROSA CLI verifier compatibility

- Run `rosa verify network` when the test environment provides the ROSA CLI and its prerequisites
- Expect the command to pass for a correctly configured zero-egress cluster and avoid requiring public registries
- Record the command output for diagnostics, but do not assert its exact endpoint count or use it as the endpoint contract

### 3.5 Suggested rosa-e2e implementation

- Add a `ZeroEgress` feature label and keep these tests HCP-only
- Detect zero-egress capability from the OCM cluster model; skip endpoint-contract tests when the target cluster is not zero egress
- Implement an AWS-side verifier for VPC endpoints, endpoint ENIs, security groups, and worker route tables
- Implement a workload-side verifier for DNS, TLS connectivity, and mirrored image pulls
- Compare DNS answers with the endpoint ENI addresses returned by AWS instead of checking only RFC1918 ranges
- Reuse one diagnostic workload per spec and emit DNS answers, connection results, VPC endpoint state, and relevant route-table entries on failure
- Keep incomplete-endpoint scenarios in dedicated cluster-creation jobs so regular conformance runs remain non-destructive
- Resolve the probe image through the cluster's configured mirror policy; do not use the suite's current public `registry.access.redhat.com` images for zero-egress tests

## 4. Image Pull / ECR Integration

### 4.1 Workloads pull images from ECR

- Deploy a pod using a digest-pinned OCP release component image covered by the configured mirror policy
- Verify it pulls successfully through the ECR mirror without direct access to quay.io
- Check `oc get pods -n openshift-apiserver -o jsonpath='{.items[0].status.containerStatuses[0].imageID}'` references ECR

### 4.2 ImageContentSources / ImageDigestMirrorSet configured

- Verify `oc get imagedigestmirrorset -o json | jq -r '.items[].spec.imageDigestMirrors[].mirrors[]'` returns ECR URLs
- All mirror URLs should match `<account>.dkr.ecr.<region>.amazonaws.com/*`

### 4.3 Protected registry mirrors blocked

- Attempt to add image mirrors with source `quay.io/openshift-release-dev/ocp-v4.0-art-dev` or `quay.io/app-sre`
- Expect rejection — these are protected registries managed by the ECR mirroring system

### 4.4 Insights operator disabled

- Verify pull secret does not contain `cloud.openshift.com` entry:
  ```
  oc get secret pull-secret -n openshift-config -o json \
    | jq '.data[".dockerconfigjson"]' -r | base64 -d \
    | jq '.auths | has("cloud.openshift.com")'
  ```
- Expected: `false`

## 5. Proxy Behavior

### 5.1 Auto-appended no-proxy domains

- Patch proxy config on the cluster
- Verify ECR, S3, and STS regional endpoints are automatically added to the `noProxy` list:
  ```
  oc get proxy cluster -o yaml
  ```

### 5.2 Optional hard-isolation profile

- Do not require blocked public traffic or the absence of NAT routes in the portable zero-egress conformance suite
- In a dedicated restricted-network job, use a VPC intentionally created without public egress and verify public HTTPS connections fail while required AWS private paths remain reachable

## 6. DNS Resolution

### 6.1 AWS service endpoints resolve to private IPs

- From a node debug shell or pod, resolve:
  - `sts.<region>.amazonaws.com` — should return the STS endpoint ENI private IPs
  - `<account>.dkr.ecr.<region>.amazonaws.com` — should return the ECR DKR endpoint ENI private IPs
  - `api.ecr.<region>.amazonaws.com` — should return the ECR API endpoint ENI private IPs
- For S3 gateway endpoints, validate the S3 prefix-list route on each worker route table instead of requiring private DNS results

## 7. Node Pool Operations

### 7.1 Create node pool

- Add a new node pool to the zero egress cluster
- Verify new nodes join successfully and pull images from ECR

### 7.2 Scale node pool

- Scale an existing node pool up
- Verify new nodes bootstrap without internet access

### 7.3 Negative: Worker IAM missing ECR policy

- Create a node pool with a worker IAM role lacking `AmazonEC2ContainerRegistryReadOnly`
- Expect image pull failures on the new nodes

## 8. Cluster Lifecycle

### 8.1 Upgrade

- Upgrade the cluster to a new z-stream release
- Verify new release images are available in ECR and the upgrade completes

### 8.2 Delete

- `rosa delete cluster` cleans up properly
- VPC endpoints and security groups created by the test helper can be removed

## Smoke Test Summary

| Check | Command | Expected |
|---|---|---|
| Zero egress enabled | `rosa describe cluster` | "Zero Egress: Enabled" |
| Pods healthy | `oc get pods -A` | All Running |
| Interface endpoint DNS is private | Resolve STS and ECR endpoints from a pod | Addresses match endpoint ENIs |
| S3 endpoint route exists | Inspect worker subnet route tables | S3 prefix-list route targets the gateway endpoint |
| ROSA verifier compatibility | `rosa verify network` | Pass when available |
| Immutable | `rosa edit cluster --properties zero_egress:false` | Error |
| Insights disabled | Pull secret missing `cloud.openshift.com` | true |
| Images from ECR | Check pod imageID | `dkr.ecr.<region>.amazonaws.com` |

## Priority

**Highest value (no running cluster needed):**
- Section 1: Creation validation negatives
- Section 2: Immutability checks

**High value (requires provisioned cluster):**
- Section 3: Endpoint contract validation
- Section 4: Image pull / ECR integration
- Section 5-6: Proxy and DNS validation

**Medium value (longer-running):**
- Section 7: Node pool operations
- Section 8: Cluster lifecycle (upgrade, delete)
