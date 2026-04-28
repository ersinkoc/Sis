#!/usr/bin/env bash
set -euo pipefail

report="${1:-}"
record="${SIS_PROD_VALIDATE_RECORD:-docs/PRODUCTION_VALIDATION.md}"

if [[ -z "${report}" ]]; then
  echo "usage: $0 path/to/production-validation-report.md" >&2
  exit 2
fi

if [[ ! -f "${report}" ]]; then
  echo "update-production-validation-record: report not found: ${report}" >&2
  exit 1
fi

if [[ ! -f "${record}" ]]; then
  echo "update-production-validation-record: record not found: ${record}" >&2
  exit 1
fi

summary="$(awk '
  /^## Summary$/ {in_summary=1; next}
  /^## / && in_summary {exit}
  in_summary {print}
' "${report}" | sed '/^[[:space:]]*$/d')"

if [[ -z "${summary}" ]]; then
  echo "update-production-validation-record: report does not contain a Summary section" >&2
  exit 1
fi

generated="$(sed -n 's/^- Generated: //p' "${report}" | head -n 1)"
binary="$(sed -n 's/^- Binary: //p' "${report}" | head -n 1)"
config="$(sed -n 's/^- Config: //p' "${report}" | head -n 1)"
dns_server="$(sed -n 's/^- LAN DNS server: //p' "${report}" | head -n 1)"
api_url="$(sed -n 's/^- API URL: //p' "${report}" | head -n 1)"

status="Validation report recorded"
if grep -q '^- FAIL:' <<<"${summary}"; then
  status="Validation report recorded with failures"
fi

tmp="$(mktemp)"
cleanup() {
  rm -f "${tmp}"
}
trap cleanup EXIT

awk -v status="${status}" \
  -v generated="${generated}" \
  -v binary="${binary}" \
  -v config="${config}" \
  -v dns_server="${dns_server}" \
  -v api_url="${api_url}" \
  -v summary="${summary}" '
  function print_evidence() {
    print "<!-- sis-validation-summary:start -->"
    print "```text"
    print summary
    print "```"
    print "<!-- sis-validation-summary:end -->"
  }

  function result_for(name, lower_summary) {
    lname = tolower(name)
    if (index(lower_summary, "- pass: " lname) > 0) {
      return "Pass"
    }
    if (index(lower_summary, "- fail: " lname) > 0) {
      return "Fail"
    }
    return "Pending"
  }

  function print_results() {
    lower_summary = tolower(summary)
    print "| Check | Result | Notes |"
    print "| --- | --- | --- |"
    print "| Service verification | " result_for("service verification", lower_summary) " | |"
    print "| SQLite migration dry-run | " result_for("sqlite migration dry-run", lower_summary) " | |"
    print "| LAN UDP DNS | " result_for("lan dns validation", lower_summary) " | Covered by LAN DNS validation report when enabled. |"
    print "| LAN TCP DNS | " result_for("lan dns validation", lower_summary) " | Covered by LAN DNS validation report when enabled. |"
    print "| Blocked-domain policy | " result_for("lan dns validation", lower_summary) " | Covered when SIS_PROD_VALIDATE_BLOCKED_DOMAIN is set. |"
    print "| HTTP health/readiness | " result_for("service verification", lower_summary) " | Also covered by LAN DNS validation when HTTP is enabled. |"
    print "| Authenticated API store verification | " result_for("authenticated api store verification", lower_summary) " | |"
    print "| Real client query observed | " result_for("real client observation", lower_summary) " | Use SIS_PROD_VALIDATE_REAL_CLIENT=1 during live validation. |"
    print "| Diagnostics bundle generated | " result_for("diagnostics bundle", lower_summary) " | |"
  }

  /^- Status:/ {
    print "- Status: " status
    next
  }
  /^- Last production validation report:/ {
    print "- Last production validation report: " generated
    next
  }
  /^- Validation binary:/ {
    print "- Validation binary: " binary
    next
  }
  /^- Validation config:/ {
    print "- Validation config: " config
    next
  }
  /^- Validation LAN DNS server:/ {
    print "- Validation LAN DNS server: " dns_server
    next
  }
  /^- Validation API URL:/ {
    print "- Validation API URL: " api_url
    next
  }
  /^<!-- sis-validation-summary:start -->$/ {
    print_evidence()
    skip=1
    next
  }
  /^<!-- sis-validation-summary:end -->$/ {
    skip=0
    next
  }
  skip {
    next
  }
  index($0, "| Service verification |") == 1 {
    print_results()
    table=1
    next
  }
  table && /^\|/ {
    next
  }
  table {
    table=0
  }
  {print}
' "${record}" > "${tmp}"

mv "${tmp}" "${record}"
echo "update-production-validation-record: updated ${record} from ${report}"
