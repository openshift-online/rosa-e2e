#!/bin/bash
# Add an external authentication provider (external OIDC / BYO identity provider) to a ROSA HCP
# cluster that was created with external authentication enabled.
#
# The cluster MUST have been provisioned with `--external-auth-providers-enabled` (day-1 only):
#   EXTERNAL_AUTH_ENABLED=true ./scripts/provision-e2e-cluster.sh
#
# By default this points the provider at the cluster's ROSA/AWS STS OIDC issuer (derived from
# OIDC_CONFIG_ID). That issuer publishes a valid discovery document + JWKS, so it satisfies the
# config-level and issuer-reachability verifiers, but it has NO login/token endpoint and no human
# users — it cannot be used for an interactive end-to-end login. For a real login-capable provider,
# override ISSUER_URL / ISSUER_AUDIENCES / CONSOLE_CLIENT_* with a full OIDC IdP (Keycloak, Entra
# ID, etc.) and set USERNAME_CLAIM accordingly (e.g. email).
#
# Prerequisites:
#   - rosa CLI installed
#   - OCM logged in: ocm login --use-auth-code --url stage
#
# Usage:
#   source /tmp/rosa-e2e-cluster.env
#   ./scripts/create-external-auth-provider.sh
#
#   # With a real IdP:
#   ISSUER_URL=https://keycloak.example.com/realms/rosa \
#   ISSUER_AUDIENCES=rosa-hcp \
#   USERNAME_CLAIM=email \
#   CONSOLE_CLIENT_ID=... CONSOLE_CLIENT_SECRET=... \
#     ./scripts/create-external-auth-provider.sh

set -euo pipefail

ENV_FILE="${SHARED_DIR:-/tmp}/rosa-e2e-cluster.env"
if [[ -f "${ENV_FILE}" ]]; then
  source "${ENV_FILE}"
fi

CLUSTER_NAME="${CLUSTER_NAME:-}"
OIDC_CONFIG_ID="${OIDC_CONFIG_ID:-}"

# Provider parameters (override via env for a real IdP)
PROVIDER_NAME="${PROVIDER_NAME:-test-oidc}"
ISSUER_URL="${ISSUER_URL:-}"
ISSUER_AUDIENCES="${ISSUER_AUDIENCES:-openshift}"
USERNAME_CLAIM="${USERNAME_CLAIM:-sub}"
GROUPS_CLAIM="${GROUPS_CLAIM:-groups}"
CONSOLE_CLIENT_ID="${CONSOLE_CLIENT_ID:-}"
CONSOLE_CLIENT_SECRET="${CONSOLE_CLIENT_SECRET:-}"
ISSUER_CA_FILE="${ISSUER_CA_FILE:-}"

if [[ -z "${CLUSTER_NAME}" ]]; then
  echo "ERROR: Set CLUSTER_NAME or source the cluster env file (${ENV_FILE})."
  exit 1
fi

# Verify prerequisites
echo "--- Checking prerequisites ---"
if ! ocm whoami &>/dev/null; then
  echo "ERROR: OCM not logged in. Run: ocm login --use-auth-code --url stage"
  exit 1
fi

# Confirm external authentication is enabled on the cluster (day-1 setting)
EXT_AUTH_ENABLED=$(rosa describe cluster -c "${CLUSTER_NAME}" -o json 2>/dev/null \
  | jq -r '.external_auth_config.enabled // false')
if [[ "${EXT_AUTH_ENABLED}" != "true" ]]; then
  echo "ERROR: External authentication is not enabled on cluster '${CLUSTER_NAME}'."
  echo "It can only be enabled at creation. Re-provision with:"
  echo "  EXTERNAL_AUTH_ENABLED=true ./scripts/provision-e2e-cluster.sh"
  exit 1
fi

# Derive the issuer URL from the cluster's OIDC config when not explicitly provided.
if [[ -z "${ISSUER_URL}" ]]; then
  if [[ -z "${OIDC_CONFIG_ID}" ]]; then
    echo "ERROR: ISSUER_URL not set and OIDC_CONFIG_ID unavailable to derive it."
    echo "Set ISSUER_URL to your OIDC issuer, or source the cluster env file."
    exit 1
  fi
  echo "--- Deriving issuer URL from OIDC config ${OIDC_CONFIG_ID} ---"
  ISSUER_URL=$(ocm get "/api/clusters_mgmt/v1/oidc_configs/${OIDC_CONFIG_ID}" 2>/dev/null \
    | jq -r '.issuer_url // empty')
  if [[ -z "${ISSUER_URL}" ]]; then
    echo "ERROR: Could not resolve issuer_url for OIDC config ${OIDC_CONFIG_ID}."
    exit 1
  fi
fi

echo ""
echo "=== Creating external auth provider ==="
echo "Cluster:    ${CLUSTER_NAME}"
echo "Name:       ${PROVIDER_NAME}"
echo "Issuer:     ${ISSUER_URL}"
echo "Audiences:  ${ISSUER_AUDIENCES}"
echo "Username:   ${USERNAME_CLAIM}"
echo "Groups:     ${GROUPS_CLAIM}"
echo ""

# Optional args
EXTRA_ARGS=()
if [[ -n "${CONSOLE_CLIENT_ID}" ]]; then
  EXTRA_ARGS+=(--console-client-id "${CONSOLE_CLIENT_ID}")
fi
if [[ -n "${CONSOLE_CLIENT_SECRET}" ]]; then
  EXTRA_ARGS+=(--console-client-secret "${CONSOLE_CLIENT_SECRET}")
fi
if [[ -n "${ISSUER_CA_FILE}" ]]; then
  EXTRA_ARGS+=(--issuer-ca-file "${ISSUER_CA_FILE}")
fi

rosa create external-auth-provider -c "${CLUSTER_NAME}" \
  --name "${PROVIDER_NAME}" \
  --issuer-url "${ISSUER_URL}" \
  --issuer-audiences "${ISSUER_AUDIENCES}" \
  --claim-mapping-username-claim "${USERNAME_CLAIM}" \
  --claim-mapping-groups-claim "${GROUPS_CLAIM}" \
  ${EXTRA_ARGS[@]+"${EXTRA_ARGS[@]}"}

echo ""
echo "--- Configured external auth providers ---"
rosa list external-auth-provider -c "${CLUSTER_NAME}"

echo ""
echo "=== Done ==="
echo "Run the external auth verifiers:"
echo "  source ${ENV_FILE}"
echo "  OCM_TOKEN=\$(ocm token) LABEL_FILTER=\"Area:CustomerFeatures\" make test"
