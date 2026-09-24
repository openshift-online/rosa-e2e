#!/bin/bash
# ci-status-report.sh -- Check ROSA CI job health and output a summary.
#
# Reads job definitions from configs/ci-status-jobs.yaml, queries Prow
# job-history for the latest result of each job, and prints a report with per-category
# pass rates to stdout (captured as the Prow build log). Prow's
# slack_reporter posts to Slack based on exit code; the "Full report"
# link in Slack points to this build log.
#
# Requirements: curl, python3

set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly JOBS_CONFIG="${REPO_ROOT}/configs/ci-status-jobs.yaml"
readonly PROW_BASE="https://prow.ci.openshift.org/view/gs/test-platform-results"

if [[ ! -f "${JOBS_CONFIG}" ]]; then
  echo "ERROR: Job config not found: ${JOBS_CONFIG}"
  exit 1
fi

# Parse the YAML config into shell-friendly format using python3.
# Outputs lines: category_id|category_name|prow_filter|job_display_name|prow_job_name|gating
# (gating is "true" when the job's category gates or the job sets gating: true)
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
        current_cat = {'id': m.group(1).strip(), 'name': '', 'prow_filter': '', 'gating': False, 'jobs': []}
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

        # gating flag (category- or job-level); a job gates if its category
        # gates or the job itself sets gating: true
        m = re.match(r'gating:\s*(.+)', stripped)
        if m:
            val = m.group(1).strip().lower() in ('true', 'yes', '1')
            if current_job is not None:
                current_job['gating'] = val
            else:
                current_cat['gating'] = val
            continue

        # Job-level fields
        m = re.match(r'-\s+name:\s*(.+)', stripped)
        if m:
            current_job = {'name': m.group(1).strip(), 'prow_job': '', 'gating': None}
            current_cat['jobs'].append(current_job)
            continue

        if current_job is not None:
            m = re.match(r'prow_job:\s*(.+)', stripped)
            if m:
                current_job['prow_job'] = m.group(1).strip()
                continue

for cat in categories:
    cat_g = cat.get('gating', False)
    for job in cat['jobs']:
        jg = job.get('gating')
        eff = cat_g if jg is None else jg
        g = 'true' if eff else 'false'
        print(f\"{cat['id']}|{cat['name']}|{cat['prow_filter']}|{job['name']}|{job['prow_job']}|{g}\")

print(f'META|sippy_url|{sippy_url}')
" < "${JOBS_CONFIG}"
}

get_job_status() {
  local job_name="$1"
  local raw
  raw=$(curl -sf --max-time 15 \
    "https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/${job_name}" 2>/dev/null) || true

  if [[ -z "${raw}" ]]; then
    echo "|NO_DATA"
    return
  fi

  python3 -c "
import sys, re, json
html = sys.stdin.read()
match = re.search(r'var allBuilds = (\[.+?\]);', html, re.DOTALL)
if not match:
    print('|NO_DATA')
    sys.exit(0)
builds = json.loads(match.group(1))
if not builds:
    print('|NO_DATA')
    sys.exit(0)

# Check if latest is still running
latest = builds[0]
if latest.get('Result') == 'PENDING':
    # Find most recent completed build for result
    completed = [b for b in builds if b.get('Result','') != 'PENDING']
    if completed:
        b = completed[0]
        result = 'SUCCESS' if b.get('Result') == 'SUCCESS' else 'FAILURE'
        print(f\"{b.get('ID', '')}|{result}\")
    else:
        print(f\"{latest.get('ID', '')}|RUNNING\")
else:
    result = 'SUCCESS' if latest.get('Result') == 'SUCCESS' else 'FAILURE'
    print(f\"{latest.get('ID', '')}|{result}\")
" <<< "${raw}" 2>/dev/null || echo "|NO_DATA"
}

check_job() {
  local category="$1" display_name="$2" job_name="$3" gating="$4"
  local status_line build_id result

  status_line=$(get_job_status "${job_name}")
  build_id="${status_line%%|*}"
  result="${status_line##*|}"

  if [[ "${result}" == "NO_DATA" ]] || [[ -z "${result}" ]]; then
    echo "${category}|${display_name}|NO_DATA|||${gating}"
    return
  fi

  if [[ -n "${build_id}" ]]; then
    echo "${category}|${display_name}|${result}|${PROW_BASE}/logs/${job_name}/${build_id}|${job_name}|${gating}"
  else
    echo "${category}|${display_name}|${result}||${job_name}|${gating}"
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
  while IFS='|' read -r cat_id cat_name prow_filter _ _ _; do
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
  while IFS='|' read -r cat_id _ _ display_name prow_job gating; do
    if [[ "${cat_id}" == "META" ]]; then continue; fi
    (check_job "${cat_id}" "${display_name}" "${prow_job}" "${gating}" > "${tmpdir}/${idx}.txt" 2>"${tmpdir}/${idx}.err") &
    idx=$((idx + 1))
  done <<< "${config_lines}"
  wait

  # Collect results and per-category stats
  local fail_count=0 pass_count=0 other_count=0
  local g_pass=0 g_fail=0 g_other=0 ng_pass=0 ng_fail=0 ng_other=0
  local -A cat_pass cat_fail cat_other cat_fail_names
  for cat in "${category_order[@]}"; do
    cat_pass[${cat}]=0
    cat_fail[${cat}]=0
    cat_other[${cat}]=0
    cat_fail_names[${cat}]=""
  done

  local -a result_lines=()

  for i in $(seq 0 $((idx - 1))); do
    if [[ -f "${tmpdir}/${i}.txt" ]]; then
      local line category name result url gating
      line=$(cat "${tmpdir}/${i}.txt")
      category=$(echo "${line}" | cut -d'|' -f1)
      name=$(echo "${line}" | cut -d'|' -f2)
      result=$(echo "${line}" | cut -d'|' -f3)
      url=$(echo "${line}" | cut -d'|' -f4)
      gating=$(echo "${line}" | cut -d'|' -f6)

      case "${result}" in
        SUCCESS)
          pass_count=$((pass_count + 1))
          cat_pass[${category}]=$(( ${cat_pass[${category}]} + 1 ))
          if [[ "${gating}" == "true" ]]; then g_pass=$((g_pass + 1)); else ng_pass=$((ng_pass + 1)); fi
          ;;
        FAILURE|ABORTED)
          fail_count=$((fail_count + 1))
          cat_fail[${category}]=$(( ${cat_fail[${category}]} + 1 ))
          cat_fail_names[${category}]+="${name}, "
          if [[ "${gating}" == "true" ]]; then g_fail=$((g_fail + 1)); else ng_fail=$((ng_fail + 1)); fi
          ;;
        *)
          other_count=$((other_count + 1))
          cat_other[${category}]=$(( ${cat_other[${category}]} + 1 ))
          if [[ "${gating}" == "true" ]]; then g_other=$((g_other + 1)); else ng_other=$((ng_other + 1)); fi
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
  # --- Build log (linked from Prow slack reporter as "Full report") ---
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

  # Gating (production release gate) vs non-gating pass rates. Pass rate is
  # computed over definitive results (pass + fail); no-data/running are shown
  # separately so they don't skew the gate signal. Target: 95% (ROSAENG-62472).
  echo ""
  echo "--- Gating vs Non-Gating (production release gate) ---"
  echo ""
  local g_def=$((g_pass + g_fail))
  local ng_def=$((ng_pass + ng_fail))
  local g_pct ng_pct
  if [[ "${g_def}" -gt 0 ]]; then g_pct="$(( (g_pass * 100) / g_def ))%"; else g_pct="N/A"; fi
  if [[ "${ng_def}" -gt 0 ]]; then ng_pct="$(( (ng_pass * 100) / ng_def ))%"; else ng_pct="N/A"; fi
  printf "  %s %-12s %d/%d passing (%s)" "$(log_indicator "${g_pass}" "${g_def}")" "Gating:" "${g_pass}" "${g_def}" "${g_pct}"
  if [[ "${g_other}" -gt 0 ]]; then printf "  [%d no-data/running]" "${g_other}"; fi
  echo ""
  printf "  %s %-12s %d/%d passing (%s)" "$(log_indicator "${ng_pass}" "${ng_def}")" "Non-gating:" "${ng_pass}" "${ng_def}" "${ng_pct}"
  if [[ "${ng_other}" -gt 0 ]]; then printf "  [%d no-data/running]" "${ng_other}"; fi
  echo ""
  echo "  (Gating target: 95% -- tracked in ROSAENG-62472)"

  echo ""
  echo "========================================="
  echo "Result: ${pass_count}/${total} passing, ${fail_count} failing"
  echo "========================================="

  if [[ "${fail_count}" -gt 0 ]]; then
    exit 1
  fi
}

main "$@"
