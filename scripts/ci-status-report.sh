#!/bin/bash
# ci-status-report.sh -- Check ROSA CI job health and post summary to Slack.
#
# Reads job definitions from configs/ci-status-jobs.yaml, queries Prow GCS
# for the latest result of each job, and posts a formatted Slack message
# with per-category pass rates. Also prints a detailed build log to stdout.
#
# Requirements: curl, python3
# Environment: SLACK_WEBHOOK_URL (from mounted secret, optional for local testing)

set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly JOBS_CONFIG="${REPO_ROOT}/configs/ci-status-jobs.yaml"
readonly GCS_BASE="https://storage.googleapis.com/test-platform-results"
readonly PROW_BASE="https://prow.ci.openshift.org/view/gs/test-platform-results"

if [[ ! -f "${JOBS_CONFIG}" ]]; then
  echo "ERROR: Job config not found: ${JOBS_CONFIG}"
  exit 1
fi

# Parse the YAML config into shell-friendly format using python3.
# Outputs lines: category_id|category_name|prow_filter|job_display_name|prow_job_name
# followed by a metadata line: META|sippy_url|<url>
parse_config() {
  python3 -c "
import sys, re

config_text = sys.stdin.read()

# Minimal YAML parser for our specific flat structure
sippy_url = ''
categories = []
current_cat = None
current_job = None

for line in config_text.split('\n'):
    stripped = line.strip()
    if not stripped or stripped.startswith('#'):
        continue

    m = re.match(r'sippy_url:\s*(.+)', stripped)
    if m:
        sippy_url = m.group(1).strip()
        continue

    if stripped == 'categories:':
        continue

    # Category-level fields (indented under categories list)
    m = re.match(r'-\s+id:\s*(.+)', stripped)
    if m:
        current_cat = {'id': m.group(1).strip(), 'name': '', 'prow_filter': '', 'jobs': []}
        categories.append(current_cat)
        current_job = None
        continue

    if current_cat is not None:
        m = re.match(r'name:\s*(.+)', stripped)
        if m and current_job is None:
            current_cat['name'] = m.group(1).strip()
            continue

        m = re.match(r'prow_filter:\s*[\"\\']?(.+?)[\"\\']?\s*$', stripped)
        if m:
            current_cat['prow_filter'] = m.group(1).strip()
            continue

        if stripped == 'jobs:':
            continue

        # Job-level fields
        m = re.match(r'-\s+name:\s*(.+)', stripped)
        if m:
            current_job = {'name': m.group(1).strip(), 'prow_job': ''}
            current_cat['jobs'].append(current_job)
            continue

        if current_job is not None:
            m = re.match(r'prow_job:\s*(.+)', stripped)
            if m:
                current_job['prow_job'] = m.group(1).strip()
                continue

for cat in categories:
    for job in cat['jobs']:
        print(f\"{cat['id']}|{cat['name']}|{cat['prow_filter']}|{job['name']}|{job['prow_job']}\")

print(f'META|sippy_url|{sippy_url}')
" < "${JOBS_CONFIG}"
}

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

  # Parse config
  local config_lines sippy_url
  config_lines=$(parse_config)
  sippy_url=$(echo "${config_lines}" | grep "^META|sippy_url|" | cut -d'|' -f3)

  # Build category order and metadata from config (preserves YAML ordering)
  local -a category_order=()
  local -A category_names=()
  local -A category_prow_links=()
  while IFS='|' read -r cat_id cat_name prow_filter _ _; do
    if [[ "${cat_id}" == "META" ]]; then continue; fi
    if [[ -z "${category_names[${cat_id}]+x}" ]]; then
      category_order+=("${cat_id}")
      category_names[${cat_id}]="${cat_name}"
      category_prow_links[${cat_id}]="${prow_filter}"
    fi
  done <<< "${config_lines}"

  # Check all Prow jobs in parallel
  local tmpdir
  tmpdir=$(mktemp -d)
  trap "rm -rf '${tmpdir}'" EXIT

  local idx=0
  while IFS='|' read -r cat_id _ _ display_name prow_job; do
    if [[ "${cat_id}" == "META" ]]; then continue; fi
    (check_job "${cat_id}" "${display_name}" "${prow_job}" > "${tmpdir}/${idx}.txt" 2>"${tmpdir}/${idx}.err") &
    idx=$((idx + 1))
  done <<< "${config_lines}"
  wait

  # Collect results and per-category stats
  local fail_count=0 pass_count=0 other_count=0
  local -A cat_pass cat_fail cat_other cat_fail_names cat_fail_urls
  for cat in "${category_order[@]}"; do
    cat_pass[${cat}]=0
    cat_fail[${cat}]=0
    cat_other[${cat}]=0
    cat_fail_names[${cat}]=""
    cat_fail_urls[${cat}]=""
  done

  local -a result_lines=()

  for i in $(seq 0 $((idx - 1))); do
    if [[ -f "${tmpdir}/${i}.txt" ]]; then
      local line category name result url
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
        FAILURE|ABORTED)
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

  if [[ "${definitive}" -lt $((total / 2)) ]]; then
    echo ""
    echo "ERROR: Only ${definitive}/${total} jobs returned results. Possible GCS connectivity issue."
    exit 1
  fi

  # --- Slack message ---
  local overall_emoji
  overall_emoji=$(slack_emoji "${pass_count}" "${total}")
  local overall_pct
  if [[ "${total}" -gt 0 ]]; then
    overall_pct="$(( (pass_count * 100) / total ))%"
  else
    overall_pct="N/A"
  fi

  local slack_lines="${overall_emoji} *ROSA CI Daily Status:* ${pass_count}/${total} (${overall_pct})"
  slack_lines+="\n"

  for cat in "${category_order[@]}"; do
    local cp=${cat_pass[${cat}]}
    local cf=${cat_fail[${cat}]}
    local co=${cat_other[${cat}]}
    local ct=$((cp + cf + co))
    local emoji
    emoji=$(slack_emoji "${cp}" "${ct}")

    local pct_str
    if [[ "${ct}" -gt 0 ]]; then
      pct_str="$(( (cp * 100) / ct ))%"
    else
      pct_str="N/A"
    fi

    local cat_line="${emoji} *<${category_prow_links[${cat}]}|${category_names[${cat}]}>:* ${cp}/${ct} (${pct_str})"

    if [[ -n "${cat_fail_urls[${cat}]}" ]]; then
      cat_line+="  -  ${cat_fail_urls[${cat}]%, }"
    fi

    slack_lines+="\n${cat_line}"
  done

  slack_lines+="\n\n:bar_chart: <${sippy_url}|Sippy>"

  # --- Build log ---
  echo "--- Category Summary ---"
  echo ""
  for cat in "${category_order[@]}"; do
    local cp=${cat_pass[${cat}]}
    local cf=${cat_fail[${cat}]}
    local co=${cat_other[${cat}]}
    local ct=$((cp + cf + co))
    local indicator
    indicator=$(log_indicator "${cp}" "${ct}")

    local pct_str
    if [[ "${ct}" -gt 0 ]]; then
      pct_str="$(( (cp * 100) / ct ))%"
    else
      pct_str="N/A"
    fi

    local fail_detail=""
    if [[ -n "${cat_fail_names[${cat}]}" ]]; then
      fail_detail="  -- ${cat_fail_names[${cat}]%, }"
    fi

    printf "  %s %-22s %d/%d (%s)%s\n" \
      "${indicator}" "${category_names[${cat}]}:" "${cp}" "${ct}" "${pct_str}" "${fail_detail}"
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
    webhook_url=$(tr -d '\r\n' < /usr/local/rosa-ci-secrets/slack-webhook-url)
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
