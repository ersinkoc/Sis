#!/usr/bin/env bash
set -euo pipefail

tag="${1:-}"
record="${SIS_RELEASE_VALIDATION_RECORD:-docs/PRODUCTION_VALIDATION.md}"

if [[ -z "${tag}" || ! "${tag}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "usage: ./scripts/release-candidate-check.sh vMAJOR.MINOR.PATCH[-prerelease]" >&2
  exit 2
fi

branch="$(git branch --show-current)"
expected_branch="${SIS_RELEASE_BRANCH-main}"
if [[ "${branch}" != "${expected_branch}" ]]; then
  echo "release-candidate-check: expected branch ${expected_branch}, got ${branch}" >&2
  exit 1
fi

if [[ "${SIS_RELEASE_ALLOW_DIRTY:-0}" != "1" && -n "$(git status --porcelain)" ]]; then
  echo "release-candidate-check: working tree is dirty; commit or stash changes first" >&2
  exit 1
fi

if [[ ! -f "${record}" ]]; then
  echo "release-candidate-check: validation record not found: ${record}" >&2
  exit 1
fi

failures=()

if grep -Eiq 'Pending live host validation|not recorded|Paste validation summary here' "${record}"; then
  failures+=("production validation record still contains pending placeholders")
fi

required_metadata=(
  "Status"
  "Last production validation report"
  "Validation binary"
  "Validation config"
  "Validation LAN DNS server"
  "Validation API URL"
)

for field in "${required_metadata[@]}"; do
  if ! awk -v field="${field}" '
    $0 ~ "^- " field ":" {
      value=$0
      sub("^- " field ":[[:space:]]*", "", value)
      if (value != "") {
        found=1
      }
    }
    END {exit found ? 0 : 1}
  ' "${record}"; then
    failures+=("validation metadata is empty: ${field}")
  fi
done

if ! awk '
  /^- Status:/ {
    value=$0
    sub(/^- Status:[[:space:]]*/, "", value)
    if (value == "Validation report recorded") {
      ok=1
    }
  }
  END {exit ok ? 0 : 1}
' "${record}"; then
  failures+=("validation status is not complete")
fi

if ! awk '
  /^<!-- sis-validation-summary:start -->$/ {in_summary=1; next}
  /^<!-- sis-validation-summary:end -->$/ {in_summary=0; seen=1; next}
  in_summary && /^- PASS:/ {pass=1}
  in_summary && /^- FAIL:/ {fail=1}
  END {exit seen && pass && !fail ? 0 : 1}
' "${record}"; then
  failures+=("production validation summary is missing, failed, or has no passes")
fi

if awk -F'|' '
  /^## Results$/ {in_results=1; next}
  /^## / && in_results {in_results=0}
  !in_results {next}
  /^\|/ && $2 ~ /Check/ {next}
  /^\|/ && $2 ~ /---/ {next}
  /^\|/ {
    result=$3
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", result)
    if (tolower(result) != "pass") {
      bad=1
    }
  }
  END {exit bad ? 0 : 1}
' "${record}"; then
  failures+=("production validation results table has non-Pass checks")
fi

required_results=(
  "Service verification"
  "SQLite migration dry-run"
  "LAN UDP DNS"
  "LAN TCP DNS"
  "Blocked-domain policy"
  "HTTP health/readiness"
  "Authenticated API store verification"
  "Real client query observed"
  "Diagnostics bundle generated"
)

for check in "${required_results[@]}"; do
  if ! awk -F'|' -v check="${check}" '
    /^## Results$/ {in_results=1; next}
    /^## / && in_results {in_results=0}
    !in_results {next}
    /^\|/ {
      name=$2
      result=$3
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", name)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", result)
      if (name == check && tolower(result) == "pass") {
        found=1
      }
    }
    END {exit found ? 0 : 1}
  ' "${record}"; then
    failures+=("production validation result is missing or not Pass: ${check}")
  fi
done

required_fields=(
  "Validation date"
  "Sis version"
  "Commit or release tag"
  "Host OS/kernel"
  "Host IP"
  "DNS listen address"
  "HTTP listen address"
  "Store backend"
  "Data directory"
  "Router/DHCP DNS setting"
)

for field in "${required_fields[@]}"; do
  if ! awk -v field="${field}" '
    $0 ~ "^- " field ":" {
      value=$0
      sub("^- " field ":[[:space:]]*", "", value)
      if (value != "") {
        found=1
      }
    }
    END {exit found ? 0 : 1}
  ' "${record}"; then
    failures+=("host detail is empty: ${field}")
  fi
done

if [[ "${#failures[@]}" -gt 0 ]]; then
  echo "release-candidate-check: ${tag} is blocked" >&2
  for failure in "${failures[@]}"; do
    echo "- ${failure}" >&2
  done
  exit 1
fi

echo "release-candidate-check: ${tag} has recorded production validation evidence"
