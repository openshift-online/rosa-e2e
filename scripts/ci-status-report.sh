#!/bin/bash
# ci-status-report.sh -- Post a daily ROSA CI status summary to Slack.
#
# Queries Prow GCS artifacts for the latest run of each tracked job,
# checks pass/fail status, and posts a formatted summary to Slack via
# incoming webhook.
#
# Requirements: curl, jq, date (GNU or BSD)
#
# Environment variables:
#   SLACK_BOT_TOKEN    - Slack bot token (xoxb-...) for chat.postMessage API
#   SLACK_CHANNEL      - Slack channel ID to post to (default: C0ADGRNAT8U)
#   DRY_RUN            - if "true", print the message instead of posting
#
# The bot token can also be read from a file:
#   $CLUSTER_PROFILE_DIR/slack-bot-token
#   /tmp/secrets/slack-bot-token
#
# Usage:
#   SLACK_BOT_TOKEN="xoxb-..." ./ci-status-report.sh
#   DRY_RUN=true ./ci-status-report.sh

set -euo pipefail

readonly GCS_BASE="https://storage.googleapis.com/test-platform-results"
readonly PROW_BASE="https://prow.ci.openshift.org/view/gs/test-platform-results"

# ---------------------------------------------------------------------------
# Job definitions
# Each entry: "short_name|full_prow_job_name"
# ---------------------------------------------------------------------------
JOBS=(
  # rosa-e2e versioned nightly jobs
  "rosa-e2e 4.20|periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-nightly-4-20"
  "rosa-e2e 4.21|periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-nightly-4-21"
  "rosa-e2e 4.22|periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-nightly-4-22"
  # OCM FVT (clusters-service API contract tests, separate from rosa-e2e)
  "OCM FVT (CS)|periodic-ci-openshift-online-rosa-e2e-main-periodics-ocm-fvt-periodic-cs-rosa-hcp-ad-staging-main"
  # HCP conformance
  "HCP conformance 4.19|periodic-ci-openshift-release-main-nightly-4.19-e2e-rosa-hcp-ovn"
  "HCP conformance 4.20|periodic-ci-openshift-release-main-nightly-4.20-e2e-rosa-hcp-ovn"
  "HCP conformance 4.21|periodic-ci-openshift-release-main-nightly-4.21-e2e-rosa-hcp-ovn"
  # Classic STS conformance
  "Classic STS 4.19|periodic-ci-openshift-release-main-nightly-4.19-e2e-rosa-sts-ovn"
  "Classic STS 4.20|periodic-ci-openshift-release-main-nightly-4.20-e2e-rosa-sts-ovn"
  "Classic STS 4.21|periodic-ci-openshift-release-main-nightly-4.21-e2e-rosa-sts-ovn"
)

# ---------------------------------------------------------------------------
# Resolve Slack bot token and channel
# ---------------------------------------------------------------------------
SLACK_CHANNEL="${SLACK_CHANNEL:-C0ADGRNAT8U}"

resolve_token() {
  if [[ -n "${SLACK_BOT_TOKEN:-}" ]]; then
    return
  fi

  local candidate
  for candidate in \
    "${CLUSTER_PROFILE_DIR:-}/slack-bot-token" \
    "/tmp/secrets/slack-bot-token"; do
    if [[ -f "${candidate}" ]]; then
      SLACK_BOT_TOKEN=$(cat "${candidate}")
      return
    fi
  done

  if [[ "${DRY_RUN:-}" != "true" ]]; then
    echo "ERROR: No Slack bot token found. Set SLACK_BOT_TOKEN, provide" >&2
    echo "  \$CLUSTER_PROFILE_DIR/slack-bot-token, or use DRY_RUN=true." >&2
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Get the latest build number for a Prow periodic job.
# Prow stores periodic results in GCS at:
#   logs/<job-name>/latest-build.txt
# ---------------------------------------------------------------------------
get_latest_build() {
  local job_name="$1"
  local url="${GCS_BASE}/logs/${job_name}/latest-build.txt"
  curl -sf --max-time 10 "${url}" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Get the finished.json result for a specific build.
# Returns: SUCCESS, FAILURE, ABORTED, or empty if still running.
# ---------------------------------------------------------------------------
get_build_result() {
  local job_name="$1"
  local build_id="$2"
  local url="${GCS_BASE}/logs/${job_name}/${build_id}/finished.json"
  curl -sf --max-time 10 "${url}" 2>/dev/null \
    | jq -r '.result // empty' 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Build the Prow job view URL.
# ---------------------------------------------------------------------------
prow_url() {
  local job_name="$1"
  local build_id="$2"
  echo "${PROW_BASE}/logs/${job_name}/${build_id}"
}

# ---------------------------------------------------------------------------
# Format a single job status line for Slack mrkdwn.
# ---------------------------------------------------------------------------
format_job_line() {
  local display_name="$1"
  local job_name="$2"

  local build_id
  build_id=$(get_latest_build "${job_name}")

  if [[ -z "${build_id}" ]]; then
    printf "%s:  :warning: NO DATA\n" "${display_name}"
    return
  fi

  local result
  result=$(get_build_result "${job_name}" "${build_id}")
  local link
  link=$(prow_url "${job_name}" "${build_id}")

  local icon
  case "${result}" in
    SUCCESS)  icon=":white_check_mark: PASS" ;;
    FAILURE)  icon=":x: FAIL" ;;
    ABORTED)  icon=":no_entry_sign: ABORTED" ;;
    "")       icon=":hourglass: RUNNING" ;;
    *)        icon=":question: ${result}" ;;
  esac

  printf "%s:  %s  (<%s|view>)\n" "${display_name}" "${icon}" "${link}"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  resolve_token

  local today
  today=$(date -u +"%Y-%m-%d")

  # Collect all job statuses in parallel for speed
  local tmpdir
  tmpdir=$(mktemp -d)
  # Use double-quotes so tmpdir is expanded at trap-set time
  trap "rm -rf '${tmpdir}'" EXIT

  local idx=0
  for entry in "${JOBS[@]}"; do
    local display_name="${entry%%|*}"
    local job_name="${entry##*|}"
    (format_job_line "${display_name}" "${job_name}" > "${tmpdir}/${idx}.txt") &
    idx=$((idx + 1))
  done
  wait

  # Reassemble lines in order
  local body=""
  for i in $(seq 0 $((idx - 1))); do
    if [[ -f "${tmpdir}/${i}.txt" ]]; then
      body+="$(cat "${tmpdir}/${i}.txt")"$'\n'
    fi
  done

  # Build the Slack message using mrkdwn (no code blocks -- links don't render there)
  local message
  message="*ROSA CI Daily Status (${today})*

${body}
_<https://prow.ci.openshift.org/?type=periodic&job=*rosa*|All ROSA periodic jobs>_ | _<https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage|Sippy>_"

  # Build JSON payload
  local payload
  payload=$(jq -n --arg text "${message}" '{
    "text": $text,
    "unfurl_links": false,
    "unfurl_media": false
  }')

  if [[ "${DRY_RUN:-}" == "true" ]]; then
    echo "--- DRY RUN: would post to Slack ---"
    echo "${message}"
    echo ""
    echo "--- JSON payload ---"
    echo "${payload}"
    return
  fi

  # Post to Slack via chat.postMessage API
  local response
  response=$(curl -sf -X POST \
    -H "Authorization: Bearer ${SLACK_BOT_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg channel "${SLACK_CHANNEL}" --arg text "${message}" '{
      channel: $channel,
      text: $text,
      unfurl_links: false,
      unfurl_media: false
    }')" \
    "https://slack.com/api/chat.postMessage")

  if echo "${response}" | jq -e '.ok == true' > /dev/null 2>&1; then
    echo "Posted ROSA CI status to Slack successfully."
  else
    local err
    err=$(echo "${response}" | jq -r '.error // "unknown"')
    echo "ERROR: Slack API returned error: ${err}" >&2
    exit 1
  fi
}

main "$@"
