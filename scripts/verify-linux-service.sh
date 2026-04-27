#!/usr/bin/env bash
set -euo pipefail

bin="${SIS_VERIFY_BIN:-/usr/local/bin/sis}"
config="${SIS_VERIFY_CONFIG:-/etc/sis/sis.yaml}"
service="${SIS_VERIFY_SERVICE:-sis}"
http_url="${SIS_VERIFY_HTTP_URL:-http://127.0.0.1:8080}"
dns_server="${SIS_VERIFY_DNS_SERVER:-127.0.0.1:53}"
dns_domain="${SIS_VERIFY_DNS_DOMAIN:-example.com}"
skip_systemd="${SIS_VERIFY_SKIP_SYSTEMD:-0}"
skip_http="${SIS_VERIFY_SKIP_HTTP:-0}"
skip_dns="${SIS_VERIFY_SKIP_DNS:-0}"

if [[ ! -x "${bin}" ]]; then
  echo "verify-linux-service: binary not found or not executable: ${bin}" >&2
  exit 1
fi

"${bin}" version
"${bin}" config check -config "${config}"

if [[ "${skip_systemd}" != "1" ]]; then
  if ! command -v systemctl >/dev/null 2>&1; then
    echo "verify-linux-service: systemctl not found; set SIS_VERIFY_SKIP_SYSTEMD=1 to skip" >&2
    exit 1
  fi
  systemctl is-enabled "${service}" >/dev/null
  systemctl is-active --quiet "${service}"
fi

if [[ "${skip_http}" != "1" ]]; then
  if ! command -v curl >/dev/null 2>&1; then
    echo "verify-linux-service: curl not found; set SIS_VERIFY_SKIP_HTTP=1 to skip" >&2
    exit 1
  fi
  curl -fsS "${http_url}/healthz" >/dev/null
  curl -fsS "${http_url}/readyz" >/dev/null
fi

if [[ "${skip_dns}" != "1" ]]; then
  "${bin}" query -server "${dns_server}" test "${dns_domain}" A >/dev/null
fi

echo "verify-linux-service: service checks passed"
