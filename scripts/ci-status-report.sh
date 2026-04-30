#!/bin/bash
# ci-status-report.sh -- Check ROSA CI job health and post summary to Slack.
#
# Queries Prow GCS artifacts for the latest run of each tracked job.
# Posts a formatted message directly to Slack with per-category pass rates.
# Also prints a detailed report to stdout (captured as the Prow build log).
#
# Requirements: curl, python3
# Environment: SLACK_WEBHOOK_URL (from mounted secret, optional for local testing)

set -euo pipefail

readonly GCS_BASE="https://storage.googleapis.com/test-platform-results"
readonly PROW_BASE="https://prow.ci.openshift.org/view/gs/test-platform-results"
readonly SIPPY_URL="https://sippy.dptools.openshift.org/sippy-ng/release/rosa-stage"
readonly PROW_ROSA_E2E="https://prow.ci.openshift.org/?type=periodic&job=*rosa-hcp-e2e-nightly*"
readonly PROW_OCM_FVT="https://prow.ci.openshift.org/?type=periodic&job=*ocm-fvt*rosa*"
readonly PROW_HCP_CONFORMANCE="https://prow.ci.openshift.org/?type=periodic&job=*main-nightly-*e2e-rosa-hcp-ovn"
readonly PROW_STS_CONFORMANCE="https://prow.ci.openshift.org/?type=periodic&job=*main-nightly-*e2e-rosa-sts-ovn"

# ---------------------------------------------------------------------------
# Job definitions
# Each entry: "category|short_name|full_prow_job_name"
# ---------------------------------------------------------------------------
JOBS=(
  "rosa-e2e|rosa-e2e 4.20|periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-nightly-4-20"
  "rosa-e2e|rosa-e2e 4.21|periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-nightly-4-21"
  "rosa-e2e|rosa-e2e 4.22|periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-nightly-4-22"
  "ocm-fvt|OCM FVT (CS)|periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-periodic-cs-rosa-hcp-ad-staging-main"
  "hcp|HCP 4.19|periodic-ci-openshift-release-main-nightly-4.19-e2e-rosa-hcp-ovn"
  "hcp|HCP 4.20|periodic-ci-openshift-release-main-nightly-4.20-e2e-rosa-hcp-ovn"
  "hcp|HCP 4.21|periodic-ci-openshift-release-main-nightly-4.21-e2e-rosa-hcp-ovn"
  "hcp|HCP 4.22|periodic-ci-openshift-release-main-nightly-4.22-e2e-rosa-hcp-ovn"
  "classic|STS 4.19|periodic-ci-openshift-release-main-nightly-4.19-e2e-rosa-sts-ovn"
  "classic|STS 4.20|periodic-ci-openshift-release-main-nightly-4.20-e2e-rosa-sts-ovn"
  "classic|STS 4.21|periodic-ci-openshift-release-main-nightly-4.21-e2e-rosa-sts-ovn"
  "classic|STS 4.22|periodic-ci-openshift-release-main-nightly-4.22-e2e-rosa-sts-ovn"
)

CATEGORY_ORDER=(rosa-e2e ocm-fvt hcp classic)
declare -A CATEGORY_NAMES=(
  [rosa-e2e]="ROSA E2E"
  [ocm-fvt]="OCM FVT"
  [hcp]="HCP Conformance"
  [classic]="Classic STS Conformance"
)
declare -A CATEGORY_PROW_LINKS=(
  [rosa-e2e]="${PROW_ROSA_E2E}"
  [ocm-fvt]="${PROW_OCM_FVT}"
  [hcp]="${PROW_HCP_CONFORMANCE}"
  [classic]="${PROW_STS_CONFORMANCE}"
)

get_latest_build() {
  curl -sf --max-time 10 "${GCS_BASE}/logs/${1}/latest-build.txt" 2>/dev/null || true
}

get_build_result() {
  local json
  json=$(curl -sf --max-time 10 "${GCS_BASE}/logs/${1}/${2}/finished.json" 2>/dev/null) || true
  if [[ -n "${json}" ]]; then
    python3 -c "import json,sys; print(json.loads(sys.stdin.read()).get('result',''))" <<< "${json}" 2>/dev/null || true
  fi
}

# When latest build is still running, find the most recent completed build
get_last_completed() {
  local job_name="$1"
  python3 -c "
import json, sys, urllib.request
url = 'https://prow.ci.openshift.org/prowjobs.js?omit=annotations,labels,decoration_config,pod_spec&job=${job_name}'
try:
    with urllib.request.urlopen(url, timeout=15) as resp:
        data = json.loads(resp.read())
    completed = [i for i in data.get('items', [])
                 if i['status'].get('state') in ('success', 'failure', 'aborted', 'error')]
    completed.sort(key=lambda x: x['status'].get('startTime', ''), reverse=True)
    if completed:
        item = completed[0]
        bid = item['status'].get('build_id', '')
        state = item['status']['state']
        result = 'SUCCESS' if state == 'success' else 'FAILURE'
        print(f'{bid}|{result}')
except Exception:
    pass
" 2>/dev/null || true
}

check_job() {
  local category="$1" display_name="$2" job_name="$3"
  local build_id result

  build_id=$(get_latest_build "${job_name}")
  if [[ -z "${build_id}" ]]; then
    echo >&2 "WARN: No latest-build.txt for ${job_name}"
    echo "${category}|${display_name}|NO_DATA||"
    return
  fi

  result=$(get_build_result "${job_name}" "${build_id}")

  # If latest build is still running, fall back to most recent completed build
  if [[ -z "${result}" ]]; then
    local fallback
    fallback=$(get_last_completed "${job_name}")
    if [[ -n "${fallback}" ]]; then
      build_id="${fallback%%|*}"
      result="${fallback##*|}"
      echo >&2 "INFO: ${display_name} latest build running, using previous: ${result}"
    fi
  fi

  echo "${category}|${display_name}|${result:-RUNNING}|${PROW_BASE}/logs/${job_name}/${build_id}|${job_name}"
}

# Slack emoji based on pass percentage
# 100% = green, >= 50% = orange, < 50% = red
slack_emoji() {
  local pass="$1" total="$2"
  if [[ "${total}" -eq 0 ]]; then echo ":white_circle:"; return; fi
  local pct=$(( (pass * 100) / total ))
  if [[ "${pct}" -eq 100 ]]; then
    echo ":large_green_circle:"
  elif [[ "${pct}" -ge 50 ]]; then
    echo ":large_orange_circle:"
  else
    echo ":red_circle:"
  fi
}

# Plain text indicator for build log
log_indicator() {
  local pass="$1" total="$2"
  if [[ "${total}" -eq 0 ]]; then echo "[----]"; return; fi
  local pct=$(( (pass * 100) / total ))
  if [[ "${pct}" -eq 100 ]]; then
    echo "[PASS]"
  elif [[ "${pct}" -ge 50 ]]; then
    echo "[WARN]"
  else
    echo "[FAIL]"
  fi
}

post_to_slack() {
  local webhook_url="$1" payload="$2"
  local http_code
  http_code=$(curl -sf --max-time 10 -o /dev/null -w "%{http_code}" \
    -X POST -H "Content-Type: application/json" \
    -d "${payload}" "${webhook_url}" 2>/dev/null) || true
  if [[ "${http_code}" != "200" ]]; then
    echo >&2 "WARN: Slack post returned HTTP ${http_code}"
  fi
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
    IFS='|' read -r category display_name job_name <<< "${entry}"
    (check_job "${category}" "${display_name}" "${job_name}" > "${tmpdir}/${idx}.txt" 2>"${tmpdir}/${idx}.err") &
    idx=$((idx + 1))
  done
  wait

  # Collect results and per-category stats
  local fail_count=0 pass_count=0 other_count=0
  declare -A cat_pass cat_fail cat_other cat_fail_names cat_fail_urls
  for cat in "${CATEGORY_ORDER[@]}"; do
    cat_pass[${cat}]=0
    cat_fail[${cat}]=0
    cat_other[${cat}]=0
    cat_fail_names[${cat}]=""
    cat_fail_urls[${cat}]=""
  done

  declare -a result_lines=()

  for i in $(seq 0 $((idx - 1))); do
    if [[ -f "${tmpdir}/${i}.txt" ]]; then
      local line category name result url job_name
      line=$(cat "${tmpdir}/${i}.txt")
      category=$(echo "${line}" | cut -d'|' -f1)
      name=$(echo "${line}" | cut -d'|' -f2)
      result=$(echo "${line}" | cut -d'|' -f3)
      url=$(echo "${line}" | cut -d'|' -f4)

      case "${result}" in
        SUCCESS)
          pass_count=$((pass_count + 1))
          cat_pass[${category}]=$(( ${cat_pass[${category}]} + 1 ))
          ;;
        FAILURE|ABORTED|NO_DATA)
          fail_count=$((fail_count + 1))
          cat_fail[${category}]=$(( ${cat_fail[${category}]} + 1 ))
          cat_fail_names[${category}]+="${name}, "
          if [[ -n "${url}" ]]; then
            cat_fail_urls[${category}]+="<${url}|${name}>, "
          fi
          ;;
        *)
          other_count=$((other_count + 1))
          cat_other[${category}]=$(( ${cat_other[${category}]} + 1 ))
          ;;
      esac

      local icon
      case "${result}" in
        SUCCESS) icon="PASS" ;;
        FAILURE) icon="FAIL" ;;
        ABORTED) icon="ABORTED" ;;
        NO_DATA) icon="NO DATA" ;;
        RUNNING) icon="RUNNING" ;;
        *)       icon="${result}" ;;
      esac

      if [[ -n "${url}" ]]; then
        result_lines+=("$(printf "  %-20s %-8s %s" "${name}" "${icon}" "${url}")")
      else
        result_lines+=("$(printf "  %-20s %s" "${name}" "${icon}")")
      fi
    fi

    if [[ -f "${tmpdir}/${i}.err" ]] && [[ -s "${tmpdir}/${i}.err" ]]; then
      cat "${tmpdir}/${i}.err" >&2
    fi
  done

  local total=$((pass_count + fail_count + other_count))
  local definitive=$((pass_count + fail_count))

  # Guard against GCS connectivity issues
  if [[ "${definitive}" -lt $((total / 2)) ]]; then
    echo ""
    echo "ERROR: Only ${definitive}/${total} jobs returned results. Possible GCS connectivity issue."
    exit 1
  fi

  # --- Build the Slack message ---
  local overall_emoji
  overall_emoji=$(slack_emoji "${pass_count}" "${definitive}")
  local overall_pct
  if [[ "${definitive}" -gt 0 ]]; then
    overall_pct="$(( (pass_count * 100) / definitive ))%"
  else
    overall_pct="N/A"
  fi

  local slack_lines="${overall_emoji} *ROSA CI Daily Status:* ${pass_count}/${total} (${overall_pct})"
  slack_lines+="\n"

  for cat in "${CATEGORY_ORDER[@]}"; do
    local cp=${cat_pass[${cat}]}
    local cf=${cat_fail[${cat}]}
    local co=${cat_other[${cat}]}
    local ct=$((cp + cf + co))
    local emoji
    emoji=$(slack_emoji "${cp}" "$((cp + cf))")

    local pct_str
    if [[ $((cp + cf)) -gt 0 ]]; then
      pct_str="$(( (cp * 100) / (cp + cf) ))%"
    elif [[ "${co}" -gt 0 ]]; then
      pct_str="running"
    else
      pct_str="N/A"
    fi

    local cat_line="${emoji} *<${CATEGORY_PROW_LINKS[${cat}]}|${CATEGORY_NAMES[${cat}]}>:* ${cp}/${ct} (${pct_str})"

    if [[ -n "${cat_fail_urls[${cat}]}" ]]; then
      cat_line+="  -  ${cat_fail_urls[${cat}]%, }"
    fi

    slack_lines+="\n${cat_line}"
  done

  slack_lines+="\n\n:bar_chart: <${SIPPY_URL}|Sippy>"

  # --- Print build log (stdout) ---
  echo "--- Category Summary ---"
  echo ""
  for cat in "${CATEGORY_ORDER[@]}"; do
    local cp=${cat_pass[${cat}]}
    local cf=${cat_fail[${cat}]}
    local co=${cat_other[${cat}]}
    local ct=$((cp + cf + co))
    local indicator
    indicator=$(log_indicator "${cp}" "$((cp + cf))")

    local pct_str
    if [[ $((cp + cf)) -gt 0 ]]; then
      pct_str="$(( (cp * 100) / (cp + cf) ))%"
    elif [[ "${co}" -gt 0 ]]; then
      pct_str="running"
    else
      pct_str="N/A"
    fi

    local fail_detail=""
    if [[ -n "${cat_fail_names[${cat}]}" ]]; then
      fail_detail="  -- ${cat_fail_names[${cat}]%, }"
    fi

    printf "  %s %-22s %d/%d (%s)%s\n" \
      "${indicator}" "${CATEGORY_NAMES[${cat}]}:" "${cp}" "${ct}" "${pct_str}" "${fail_detail}"
  done

  echo ""
  echo "--- Prow Job Status ---"
  echo ""
  for line in "${result_lines[@]}"; do
    echo "${line}"
  done

  echo ""
  echo "--- Overall ---"
  echo ""
  echo "  Passing: ${pass_count}/${total}"
  echo "  Failing: ${fail_count}/${total}"
  if [[ "${other_count}" -gt 0 ]]; then
    echo "  Other:   ${other_count}/${total} (running/no data)"
  fi

  # --- Post to Slack ---
  local webhook_url="${SLACK_WEBHOOK_URL:-}"
  if [[ -z "${webhook_url}" ]] && [[ -f "/usr/local/rosa-ci-secrets/slack-webhook-url" ]]; then
    webhook_url=$(cat /usr/local/rosa-ci-secrets/slack-webhook-url)
  fi

  if [[ -n "${webhook_url}" ]]; then
    local escaped_text
    escaped_text=$(echo -ne "${slack_lines}")
    local payload
    payload=$(python3 -c "
import json, sys
print(json.dumps({'text': sys.stdin.read()}))
" <<< "${escaped_text}")
    echo ""
    echo "--- Slack Message ---"
    echo ""
    echo -e "${slack_lines}"
    echo ""
    post_to_slack "${webhook_url}" "${payload}"
    echo "Slack message posted."
  else
    echo ""
    echo "--- Slack Message (not posted, no SLACK_WEBHOOK_URL) ---"
    echo ""
    echo -e "${slack_lines}"
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
