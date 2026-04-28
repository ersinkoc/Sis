#!/usr/bin/env bash
set -euo pipefail

http_url="${SIS_CLIENT_VALIDATE_HTTP_URL:-http://127.0.0.1:8080}"
cookie="${SIS_CLIENT_VALIDATE_COOKIE:-}"
client="${SIS_CLIENT_VALIDATE_CLIENT:-}"
qname="${SIS_CLIENT_VALIDATE_QNAME:-}"
limit="${SIS_CLIENT_VALIDATE_LIMIT:-100}"

if [[ -z "${cookie}" ]]; then
  echo "validate-real-client: set SIS_CLIENT_VALIDATE_COOKIE to an authenticated sis_session cookie" >&2
  exit 2
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "validate-real-client: curl not found" >&2
  exit 1
fi

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

query="/api/v1/logs/query?limit=${limit}"
if [[ -n "${client}" ]]; then
  query="${query}&client=${client}"
fi
if [[ -n "${qname}" ]]; then
  query="${query}&qname=${qname}"
fi

logs="${tmp}/logs.json"
clients="${tmp}/clients.json"
curl -fsS --cookie "${cookie}" "${http_url}${query}" > "${logs}"
curl -fsS --cookie "${cookie}" "${http_url}/api/v1/clients" > "${clients}"

if [[ -n "${client}" ]]; then
  if grep -Fq "\"client_key\":\"${client}\"" "${logs}" || \
    grep -Fq "\"client_ip\":\"${client}\"" "${logs}" || \
    grep -Fq "\"key\":\"${client}\"" "${clients}"; then
    echo "validate-real-client: observed client ${client} through logs or client inventory"
    exit 0
  fi
  echo "validate-real-client: client ${client} was not observed in query logs or client inventory" >&2
  exit 1
fi

if grep -Fq '"ts":' "${logs}"; then
  echo "validate-real-client: observed at least one query log entry"
  exit 0
fi

echo "validate-real-client: no query log entries observed; set SIS_CLIENT_VALIDATE_CLIENT for inventory-based validation" >&2
exit 1
