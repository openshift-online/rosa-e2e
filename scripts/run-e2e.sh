#!/bin/bash
# Run the rosa-e2e test suite.
#
# Works in two modes:
#   1. Prow CI: uses CLUSTER_PROFILE_DIR for AWS creds and OCM token
#   2. Local: uses environment variables directly
#
# Usage (local):
#   source /tmp/rosa-e2e-cluster.env
#   OCM_TOKEN=$(ocm token) ./scripts/run-e2e.sh
#
# Usage (Prow):
#   Automatically picks up creds from CLUSTER_PROFILE_DIR

set -euo pipefail

# In Prow, load creds from cluster profile
if [[ -n "${CLUSTER_PROFILE_DIR:-}" ]]; then
  echo "Running in Prow mode (cluster_profile: ${CLUSTER_PROFILE_DIR})"

  AWSCRED="${CLUSTER_PROFILE_DIR}/.awscred"
  if [[ -f "${AWSCRED}" ]]; then
    export AWS_SHARED_CREDENTIALS_FILE="${AWSCRED}"
  fi

  if [[ -f "${CLUSTER_PROFILE_DIR}/ocm-token" ]]; then
    export OCM_TOKEN=$(cat "${CLUSTER_PROFILE_DIR}/ocm-token")
  fi

  if [[ -f "${CLUSTER_PROFILE_DIR}/sso-client-id" ]]; then
    SSO_CLIENT_ID=$(cat "${CLUSTER_PROFILE_DIR}/sso-client-id")
    SSO_CLIENT_SECRET=$(cat "${CLUSTER_PROFILE_DIR}/sso-client-secret")
    rosa login --env "${OCM_LOGIN_ENV:-staging}" --client-id "${SSO_CLIENT_ID}" --client-secret "${SSO_CLIENT_SECRET}"
    ocm login --url "${OCM_LOGIN_ENV:-staging}" --client-id "${SSO_CLIENT_ID}" --client-secret "${SSO_CLIENT_SECRET}"
  elif [[ -n "${OCM_TOKEN:-}" ]]; then
    rosa login --env "${OCM_LOGIN_ENV:-staging}" --token "${OCM_TOKEN}"
    ocm login --url "${OCM_LOGIN_ENV:-staging}" --token "${OCM_TOKEN}"
  fi

  # Load cluster info from shared dir
  if [[ -f "${SHARED_DIR:-}/cluster-id" ]]; then
    export CLUSTER_ID=$(cat "${SHARED_DIR}/cluster-id")
  fi

  export AWS_REGION="${LEASED_RESOURCE:-${AWS_REGION:-us-east-2}}"
fi

# Verify required vars
if [[ -z "${OCM_TOKEN:-}" ]]; then
  echo "ERROR: OCM_TOKEN not set"
  exit 1
fi

echo "=== ROSA E2E Test Suite ==="
echo "Cluster: ${CLUSTER_ID:-not set}"
echo "Region: ${AWS_REGION:-not set}"
echo "MC: ${MANAGEMENT_CLUSTER_ID:-not set}"
echo ""

# If MC_KUBECONFIG not set but we're in Prow with backplane, try to get MC access
if [[ -z "${MC_KUBECONFIG:-}" ]] && [[ -n "${CLUSTER_ID:-}" ]] && command -v ocm &>/dev/null; then
  echo "Attempting MC access via backplane..."
  if ocm backplane login "${CLUSTER_ID}" --manager 2>/dev/null; then
    export MC_KUBECONFIG="${HOME}/.kube/config"
    MC_SERVER=$(oc whoami --show-server 2>/dev/null || true)
    if [[ "${MC_SERVER}" == *"backplane"* ]]; then
      MANAGEMENT_CLUSTER_ID=$(echo "${MC_SERVER}" | grep -o 'cluster/[^/]*' | cut -d/ -f2)
      export MANAGEMENT_CLUSTER_ID
      echo "MC access established: ${MANAGEMENT_CLUSTER_ID}"
    fi
  fi
fi

# Run tests
GINKGO_FLAGS="-v --tags E2Etests --junit-report ${ARTIFACT_DIR:-/tmp}/junit-report.xml"

if [[ -n "${LABEL_FILTER:-}" ]]; then
  GINKGO_FLAGS="${GINKGO_FLAGS} --label-filter='${LABEL_FILTER}'"
fi

echo "Running tests..."
go run github.com/onsi/ginkgo/v2/ginkgo run ${GINKGO_FLAGS} ./test/e2e/

echo ""
echo "Results saved to ${ARTIFACT_DIR:-/tmp}/junit-report.xml"
