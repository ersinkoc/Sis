#!/usr/bin/env bash
set -euo pipefail

timestamp="${SIS_PROD_VALIDATE_TIMESTAMP:-$(date -u +%Y%m%dT%H%M%SZ)}"
out_dir="${SIS_PROD_VALIDATE_DIR:-sis-validation}"
out="${SIS_PROD_VALIDATE_OUT:-${out_dir}/production-validation-${timestamp}.md}"
bin="${SIS_PROD_VALIDATE_BIN:-/usr/local/bin/sis}"
config="${SIS_PROD_VALIDATE_CONFIG:-/etc/sis/sis.yaml}"
run_sqlite="${SIS_PROD_VALIDATE_SQLITE:-1}"
run_lan="${SIS_PROD_VALIDATE_LAN:-1}"
run_diagnostics="${SIS_PROD_VALIDATE_DIAGNOSTICS:-0}"
run_real_client="${SIS_PROD_VALIDATE_REAL_CLIENT:-0}"
api_url="${SIS_PROD_VALIDATE_API_URL:-${SIS_PROD_VALIDATE_HTTP_URL:-http://127.0.0.1:8080}}"
api_cookie="${SIS_PROD_VALIDATE_API_COOKIE:-}"

mkdir -p "$(dirname "${out}")"
tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

checks=()
statuses=()
logs=()

run_check() {
  local name="$1"
  shift
  local log="${tmp}/${#checks[@]}.log"
  checks+=("${name}")
  logs+=("${log}")
  set +e
  {
    printf '$'
    printf ' %q' "$@"
    printf '\n\n'
    "$@"
  } > "${log}" 2>&1
  local status=$?
  set -e
  statuses+=("${status}")
  return 0
}

run_auth_store_verify() {
  local name="authenticated API store verification"
  local log="${tmp}/${#checks[@]}.log"
  checks+=("${name}")
  logs+=("${log}")
  set +e
  {
    printf '$ %q system -api %q -cookie %q store-verify\n\n' "${bin}" "${api_url}" "redacted"
    "${bin}" system -api "${api_url}" -cookie "${api_cookie}" store-verify 2>&1
  } | sed -E 's/(cookie: [^=]+=).*/\1redacted/' > "${log}"
  local status=${PIPESTATUS[0]}
  set -e
  statuses+=("${status}")
  return 0
}

run_real_client_observation() {
  local name="real client observation"
  local log="${tmp}/${#checks[@]}.log"
  checks+=("${name}")
  logs+=("${log}")
  set +e
  {
    printf '$ env SIS_CLIENT_VALIDATE_HTTP_URL=%q SIS_CLIENT_VALIDATE_COOKIE=%q SIS_CLIENT_VALIDATE_CLIENT=%q SIS_CLIENT_VALIDATE_QNAME=%q ./scripts/validate-real-client.sh\n\n' \
      "${api_url}" \
      "redacted" \
      "${SIS_PROD_VALIDATE_REAL_CLIENT_ID:-}" \
      "${SIS_PROD_VALIDATE_REAL_CLIENT_QNAME:-}"
    SIS_CLIENT_VALIDATE_HTTP_URL="${api_url}" \
      SIS_CLIENT_VALIDATE_COOKIE="${api_cookie}" \
      SIS_CLIENT_VALIDATE_CLIENT="${SIS_PROD_VALIDATE_REAL_CLIENT_ID:-}" \
      SIS_CLIENT_VALIDATE_QNAME="${SIS_PROD_VALIDATE_REAL_CLIENT_QNAME:-}" \
      ./scripts/validate-real-client.sh 2>&1
  } | sed -E 's/(SIS_CLIENT_VALIDATE_COOKIE=)[^ ]+/\1redacted/g; s/(cookie: [^=]+=).*/\1redacted/' > "${log}"
  local status=${PIPESTATUS[0]}
  set -e
  statuses+=("${status}")
  return 0
}

run_check "service verification" env \
  SIS_VERIFY_BIN="${bin}" \
  SIS_VERIFY_CONFIG="${config}" \
  SIS_VERIFY_SERVICE="${SIS_PROD_VALIDATE_SERVICE:-sis}" \
  SIS_VERIFY_HTTP_URL="${SIS_PROD_VALIDATE_HTTP_URL:-http://127.0.0.1:8080}" \
  SIS_VERIFY_DNS_SERVER="${SIS_PROD_VALIDATE_DNS_SERVER:-127.0.0.1:53}" \
  SIS_VERIFY_DNS_DOMAIN="${SIS_PROD_VALIDATE_DNS_DOMAIN:-example.com}" \
  SIS_VERIFY_SKIP_SYSTEMD="${SIS_PROD_VALIDATE_SKIP_SYSTEMD:-0}" \
  SIS_VERIFY_SKIP_HTTP="${SIS_PROD_VALIDATE_SKIP_HTTP:-0}" \
  SIS_VERIFY_SKIP_DNS="${SIS_PROD_VALIDATE_SKIP_DNS:-0}" \
  ./scripts/verify-linux-service.sh

if [[ "${run_sqlite}" == "1" ]]; then
  run_check "sqlite migration dry-run" env \
    SIS_SQLITE_VALIDATE_BIN="${bin}" \
    SIS_SQLITE_VALIDATE_CONFIG="${config}" \
    SIS_SQLITE_VALIDATE_DATA_DIR="${SIS_PROD_VALIDATE_DATA_DIR:-}" \
    ./scripts/validate-sqlite-migration.sh
fi

if [[ "${run_lan}" == "1" ]]; then
  run_check "lan dns validation" env \
    SIS_LAN_VALIDATE_BIN="${bin}" \
    SIS_LAN_VALIDATE_CONFIG="${config}" \
    SIS_LAN_VALIDATE_DNS_SERVER="${SIS_PROD_VALIDATE_LAN_DNS_SERVER:-${SIS_PROD_VALIDATE_DNS_SERVER:-127.0.0.1:53}}" \
    SIS_LAN_VALIDATE_DOMAIN="${SIS_PROD_VALIDATE_LAN_DOMAIN:-example.com}" \
    SIS_LAN_VALIDATE_BLOCKED_DOMAIN="${SIS_PROD_VALIDATE_BLOCKED_DOMAIN:-}" \
    SIS_LAN_VALIDATE_HTTP_URL="${SIS_PROD_VALIDATE_HTTP_URL:-http://127.0.0.1:8080}" \
    SIS_LAN_VALIDATE_SKIP_HTTP="${SIS_PROD_VALIDATE_LAN_SKIP_HTTP:-${SIS_PROD_VALIDATE_SKIP_HTTP:-0}}" \
    SIS_LAN_VALIDATE_SKIP_TCP="${SIS_PROD_VALIDATE_LAN_SKIP_TCP:-0}" \
    ./scripts/validate-lan-dns.sh
fi

if [[ -n "${api_cookie}" ]]; then
  run_auth_store_verify
fi

if [[ "${run_real_client}" == "1" ]]; then
  run_real_client_observation
fi

if [[ "${run_diagnostics}" == "1" ]]; then
  run_check "diagnostics bundle" env \
    SIS_DIAG_BIN="${bin}" \
    SIS_DIAG_CONFIG="${config}" \
    SIS_DIAG_SERVICE="${SIS_PROD_VALIDATE_SERVICE:-sis}" \
    SIS_DIAG_DIR="${SIS_PROD_VALIDATE_DIAG_DIR:-sis-diagnostics}" \
    ./scripts/collect-linux-diagnostics.sh
fi

{
  echo "# Sis Production Validation"
  echo
  echo "- Generated: ${timestamp}"
  echo "- Binary: ${bin}"
  echo "- Config: ${config}"
  echo "- Service: ${SIS_PROD_VALIDATE_SERVICE:-sis}"
  echo "- DNS server: ${SIS_PROD_VALIDATE_DNS_SERVER:-127.0.0.1:53}"
  echo "- LAN DNS server: ${SIS_PROD_VALIDATE_LAN_DNS_SERVER:-${SIS_PROD_VALIDATE_DNS_SERVER:-127.0.0.1:53}}"
  echo "- API URL: ${api_url}"
  if [[ -n "${api_cookie}" ]]; then
    echo "- Authenticated API checks: enabled"
  else
    echo "- Authenticated API checks: skipped (set SIS_PROD_VALIDATE_API_COOKIE)"
  fi
  if [[ "${run_real_client}" == "1" ]]; then
    echo "- Real client observation: enabled"
  else
    echo "- Real client observation: skipped (set SIS_PROD_VALIDATE_REAL_CLIENT=1)"
  fi
  echo
  echo "## Summary"
  echo
  for i in "${!checks[@]}"; do
    if [[ "${statuses[$i]}" == "0" ]]; then
      echo "- PASS: ${checks[$i]}"
    else
      echo "- FAIL: ${checks[$i]} (exit ${statuses[$i]})"
    fi
  done
  echo
  echo "## Logs"
  for i in "${!checks[@]}"; do
    echo
    echo "### ${checks[$i]}"
    echo
    echo '```text'
    cat "${logs[$i]}"
    echo '```'
  done
} > "${out}"

chmod 600 "${out}"

failed=0
for status in "${statuses[@]}"; do
  if [[ "${status}" != "0" ]]; then
    failed=1
    break
  fi
done

echo "run-production-validation: wrote ${out}"
if [[ "${failed}" != "0" ]]; then
  echo "run-production-validation: one or more checks failed" >&2
  exit 1
fi
echo "run-production-validation: all checks passed"
