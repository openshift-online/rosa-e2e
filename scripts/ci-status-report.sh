#!/bin/bash
# ci-status-report.sh -- Check ROSA CI job health and output a summary.
#
# Queries Prow GCS artifacts for the latest run of each tracked job and
# Sippy API for historical pass rates. Prints a detailed report to stdout
# (captured as the Prow build log) and exits non-zero if any jobs are failing.
#
# Prow's slack_reporter_config handles Slack notification based on exit code.
# The detailed breakdown is available in the build log linked from Slack.
#
# Requirements: curl, jq

set -euo pipefail

readonly GCS_BASE="https://storage.googleapis.com/test-platform-results"
readonly PROW_BASE="https://prow.ci.openshift.org/view/gs/test-platform-results"
readonly SIPPY_API="https://sippy.dptools.openshift.org/api/jobs"
readonly SIPPY_RELEASE="rosa-stage"

# ---------------------------------------------------------------------------
# Job definitions
# Each entry: "short_name|full_prow_job_name"
# ---------------------------------------------------------------------------
JOBS=(
  # rosa-e2e versioned nightly jobs
  "rosa-e2e 4.20|periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-nightly-4-20"
  "rosa-e2e 4.21|periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-nightly-4-21"
  "rosa-e2e 4.22|periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-nightly-4-22"
  # OCM FVT (clusters-service API contract tests)
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

get_latest_build() {
  local job_name="$1"
  curl -sf --max-time 10 "${GCS_BASE}/logs/${job_name}/latest-build.txt" 2>/dev/null || true
}

get_build_result() {
  local job_name="$1" build_id="$2"
  curl -sf --max-time 10 "${GCS_BASE}/logs/${job_name}/${build_id}/finished.json" 2>/dev/null \
    | jq -r '.result // empty' 2>/dev/null || true
}

prow_url() {
  echo "${PROW_BASE}/logs/${1}/${2}"
}

check_job() {
  local display_name="$1" job_name="$2"
  local build_id result

  build_id=$(get_latest_build "${job_name}")
  if [[ -z "${build_id}" ]]; then
    echo "${display_name}|NO_DATA|"
    return
  fi

  result=$(get_build_result "${job_name}" "${build_id}")
  echo "${display_name}|${result:-RUNNING}|$(prow_url "${job_name}" "${build_id}")"
}

query_sippy() {
  curl -sf --max-time 15 "${SIPPY_API}?release=${SIPPY_RELEASE}&limit=100" 2>/dev/null || echo "[]"
}

main() {
  local today
  today=$(date -u +"%Y-%m-%d")

  echo "========================================="
  echo "ROSA CI Daily Status - ${today}"
  echo "========================================="
  echo ""

  # Check all Prow jobs in parallel
  local tmpdir
  tmpdir=$(mktemp -d)
  trap "rm -rf '${tmpdir}'" EXIT

  local idx=0
  for entry in "${JOBS[@]}"; do
    local display_name="${entry%%|*}"
    local job_name="${entry##*|}"
    (check_job "${display_name}" "${job_name}" > "${tmpdir}/${idx}.txt") &
    idx=$((idx + 1))
  done

  query_sippy > "${tmpdir}/sippy.json" &
  wait

  # Parse results
  local fail_count=0 pass_count=0 other_count=0
  echo "--- Prow Job Status ---"
  echo ""
  for i in $(seq 0 $((idx - 1))); do
    if [[ -f "${tmpdir}/${i}.txt" ]]; then
      local line name result url
      line=$(cat "${tmpdir}/${i}.txt")
      name=$(echo "${line}" | cut -d'|' -f1)
      result=$(echo "${line}" | cut -d'|' -f2)
      url=$(echo "${line}" | cut -d'|' -f3)

      local icon
      case "${result}" in
        SUCCESS)  icon="PASS"; pass_count=$((pass_count + 1)) ;;
        FAILURE)  icon="FAIL"; fail_count=$((fail_count + 1)) ;;
        ABORTED)  icon="ABORTED"; fail_count=$((fail_count + 1)) ;;
        NO_DATA)  icon="NO DATA"; fail_count=$((fail_count + 1)) ;;
        RUNNING)  icon="RUNNING"; other_count=$((other_count + 1)) ;;
        *)        icon="${result}"; other_count=$((other_count + 1)) ;;
      esac

      if [[ -n "${url}" ]]; then
        printf "  %-25s %s  %s\n" "${name}" "${icon}" "${url}"
      else
        printf "  %-25s %s\n" "${name}" "${icon}"
      fi
    fi
  done

  local total=$((pass_count + fail_count + other_count))

  # Guard against false green: if fewer than half the jobs returned a
  # definitive result, something is wrong with GCS connectivity.
  local definitive=$((pass_count + fail_count))
  if [[ "${definitive}" -lt $((total / 2)) ]]; then
    echo ""
    echo "ERROR: Only ${definitive}/${total} jobs returned results. Possible GCS connectivity issue."
    exit 1
  fi

  echo ""
  echo "--- Summary ---"
  echo ""
  echo "  Passing: ${pass_count}/${total}"
  echo "  Failing: ${fail_count}/${total}"
  if [[ "${other_count}" -gt 0 ]]; then
    echo "  Other:   ${other_count}/${total} (running/no data)"
  fi

  # Sippy section
  echo ""
  echo "--- Sippy (${SIPPY_RELEASE}) ---"
  echo ""
  local sippy_data sippy_count
  sippy_data=$(cat "${tmpdir}/sippy.json")
  sippy_count=$(echo "${sippy_data}" | jq 'length' 2>/dev/null || echo "0")

  if [[ "${sippy_count}" -gt 0 ]]; then
    local sippy_healthy
    sippy_healthy=$(echo "${sippy_data}" | jq '[.[] | select(.current_pass_percentage >= 80)] | length' 2>/dev/null || echo "0")
    echo "  ${sippy_healthy}/${sippy_count} jobs >= 80% pass rate"
    echo ""
    echo "${sippy_data}" | jq -r '.[] | "  \(.name): \(.current_pass_percentage | floor)%"' 2>/dev/null | sort
  else
    echo "  No data available"
  fi

  echo ""
  echo "========================================="
  echo "Result: ${pass_count}/${total} passing, ${fail_count} failing"
  echo "========================================="

  if [[ "${fail_count}" -gt 0 ]]; then
    exit 1
  fi
}

main "$@"
