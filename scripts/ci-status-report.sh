#!/bin/bash
# ci-status-report.sh -- Post a daily ROSA CI status summary to Slack.
#
# Primary: Queries Prow GCS artifacts for the latest run of each tracked job.
# Secondary: Queries Sippy API for historical pass rates when available.
# Posts a combined summary to Slack via chat.postMessage.
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
readonly SIPPY_API="https://sippy.dptools.openshift.org/api/jobs"
readonly SIPPY_RELEASE="rosa-stage"
readonly SIPPY_URL="https://sippy.dptools.openshift.org/sippy-ng/release/${SIPPY_RELEASE}"

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
  "HCP conformance 4.22|periodic-ci-openshift-release-main-nightly-4.22-e2e-rosa-hcp-ovn"
  # Classic STS conformance
  "Classic STS 4.19|periodic-ci-openshift-release-main-nightly-4.19-e2e-rosa-sts-ovn"
  "Classic STS 4.20|periodic-ci-openshift-release-main-nightly-4.20-e2e-rosa-sts-ovn"
  "Classic STS 4.21|periodic-ci-openshift-release-main-nightly-4.21-e2e-rosa-sts-ovn"
  "Classic STS 4.22|periodic-ci-openshift-release-main-nightly-4.22-e2e-rosa-sts-ovn"
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
# ---------------------------------------------------------------------------
get_latest_build() {
  local job_name="$1"
  local url="${GCS_BASE}/logs/${job_name}/latest-build.txt"
  curl -sf --max-time 10 "${url}" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Get the finished.json result for a specific build.
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
# Outputs: "display_name|icon_text|prow_link|result"
# ---------------------------------------------------------------------------
format_job_line() {
  local display_name="$1"
  local job_name="$2"

  local build_id
  build_id=$(get_latest_build "${job_name}")

  if [[ -z "${build_id}" ]]; then
    echo "${display_name}|:warning: NO DATA||NO_DATA"
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

  echo "${display_name}|${icon}|${link}|${result}"
}

# ---------------------------------------------------------------------------
# Query Sippy API for rosa-stage pass rates. Returns JSON array or empty.
# ---------------------------------------------------------------------------
query_sippy() {
  curl -sf --max-time 15 "${SIPPY_API}?release=${SIPPY_RELEASE}&limit=100" 2>/dev/null || echo "[]"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  resolve_token

  local today
  today=$(date -u +"%Y-%m-%d")

  # Collect Prow job statuses in parallel
  local tmpdir
  tmpdir=$(mktemp -d)
  trap "rm -rf '${tmpdir}'" EXIT

  local idx=0
  for entry in "${JOBS[@]}"; do
    local display_name="${entry%%|*}"
    local job_name="${entry##*|}"
    (format_job_line "${display_name}" "${job_name}" > "${tmpdir}/${idx}.txt") &
    idx=$((idx + 1))
  done

  # Query Sippy in parallel with Prow checks
  query_sippy > "${tmpdir}/sippy.json" &

  wait

  # Parse Prow results
  local prow_lines=""
  local fail_count=0
  local pass_count=0
  local total_count=0
  for i in $(seq 0 $((idx - 1))); do
    if [[ -f "${tmpdir}/${i}.txt" ]]; then
      local line
      line=$(cat "${tmpdir}/${i}.txt")
      local name icon link result
      name=$(echo "${line}" | cut -d'|' -f1)
      icon=$(echo "${line}" | cut -d'|' -f2)
      link=$(echo "${line}" | cut -d'|' -f3)
      result=$(echo "${line}" | cut -d'|' -f4)

      if [[ -n "${link}" ]]; then
        prow_lines+="${name}:  ${icon}  (<${link}|view>)"$'\n'
      else
        prow_lines+="${name}:  ${icon}"$'\n'
      fi

      total_count=$((total_count + 1))
      if [[ "${result}" == "SUCCESS" ]]; then
        pass_count=$((pass_count + 1))
      elif [[ "${result}" == "FAILURE" || "${result}" == "ABORTED" ]]; then
        fail_count=$((fail_count + 1))
      fi
    fi
  done

  # Parse Sippy results
  local sippy_section=""
  local sippy_data
  sippy_data=$(cat "${tmpdir}/sippy.json")
  local sippy_count
  sippy_count=$(echo "${sippy_data}" | jq 'length' 2>/dev/null || echo "0")

  if [[ "${sippy_count}" -gt 0 ]]; then
    local sippy_healthy
    sippy_healthy=$(echo "${sippy_data}" | jq '[.[] | select(.current_pass_percentage >= 80)] | length' 2>/dev/null || echo "0")
    sippy_section=$'\n'"*Sippy (${SIPPY_RELEASE}):* ${sippy_healthy}/${sippy_count} jobs >= 80% pass rate"
    sippy_section+=$'\n'"$(echo "${sippy_data}" | jq -r '.[] | "\(.name | ltrimstr("periodic-ci-openshift-") | ltrimstr("online-") | ltrimstr("osde2e-main-") | ltrimstr("release-main-")): \(.current_pass_percentage | floor)%"' 2>/dev/null | sort)"
  else
    sippy_section=$'\n'"_Sippy: no data available for ${SIPPY_RELEASE}_"
  fi

  # Build summary line
  local summary_icon
  if [[ "${fail_count}" -eq 0 ]]; then
    summary_icon=":white_check_mark:"
  else
    summary_icon=":x:"
  fi

  local message
  message="${summary_icon} *ROSA CI Daily Status (${today})*  --  ${pass_count}/${total_count} passing

${prow_lines}${sippy_section}

_<https://prow.ci.openshift.org/?type=periodic&job=*rosa*|All ROSA periodic jobs>_ | _<${SIPPY_URL}|Sippy>_"

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

  # Exit non-zero if any Prow jobs failed (triggers Prow slack_reporter)
  if [[ "${fail_count}" -gt 0 ]]; then
    echo "${fail_count} job(s) failing"
    exit 1
  fi
}

main "$@"
