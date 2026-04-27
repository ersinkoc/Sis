#!/usr/bin/env bash
set -euo pipefail

bin="${SIS_LAN_VALIDATE_BIN:-/usr/local/bin/sis}"
config="${SIS_LAN_VALIDATE_CONFIG:-/etc/sis/sis.yaml}"
dns_server="${SIS_LAN_VALIDATE_DNS_SERVER:-127.0.0.1:53}"
domain="${SIS_LAN_VALIDATE_DOMAIN:-example.com}"
blocked_domain="${SIS_LAN_VALIDATE_BLOCKED_DOMAIN:-}"
http_url="${SIS_LAN_VALIDATE_HTTP_URL:-http://127.0.0.1:8080}"
skip_http="${SIS_LAN_VALIDATE_SKIP_HTTP:-0}"
skip_tcp="${SIS_LAN_VALIDATE_SKIP_TCP:-0}"

if [[ ! -x "${bin}" ]]; then
  echo "validate-lan-dns: binary not found or not executable: ${bin}" >&2
  exit 1
fi

if [[ ! -f "${config}" ]]; then
  echo "validate-lan-dns: config not found: ${config}" >&2
  exit 1
fi

"${bin}" config check -config "${config}" >/dev/null

udp_out="$("${bin}" query -server "${dns_server}" -proto udp test "${domain}" A)"
if [[ "${udp_out}" != *"rcode=NOERROR"* ]]; then
  echo "validate-lan-dns: UDP DNS query failed for ${domain} via ${dns_server}" >&2
  echo "${udp_out}" >&2
  exit 1
fi

if [[ "${skip_tcp}" != "1" ]]; then
  tcp_out="$("${bin}" query -server "${dns_server}" -proto tcp test "${domain}" A)"
  if [[ "${tcp_out}" != *"rcode=NOERROR"* ]]; then
    echo "validate-lan-dns: TCP DNS query failed for ${domain} via ${dns_server}" >&2
    echo "${tcp_out}" >&2
    exit 1
  fi
fi

if [[ -n "${blocked_domain}" ]]; then
  blocked_out="$("${bin}" query -server "${dns_server}" -proto udp test "${blocked_domain}" A)"
  if [[ "${blocked_out}" != *"rcode=NOERROR"* || "${blocked_out}" != *"0.0.0.0"* ]]; then
    echo "validate-lan-dns: blocked-domain policy check failed for ${blocked_domain}" >&2
    echo "${blocked_out}" >&2
    exit 1
  fi
fi

if [[ "${skip_http}" != "1" ]]; then
  if ! command -v curl >/dev/null 2>&1; then
    echo "validate-lan-dns: curl not found; set SIS_LAN_VALIDATE_SKIP_HTTP=1 to skip HTTP checks" >&2
    exit 1
  fi
  curl -fsS "${http_url}/healthz" >/dev/null
  curl -fsS "${http_url}/readyz" >/dev/null
fi

echo "validate-lan-dns: UDP DNS passed for ${domain} via ${dns_server}"
if [[ "${skip_tcp}" != "1" ]]; then
  echo "validate-lan-dns: TCP DNS passed for ${domain} via ${dns_server}"
fi
if [[ -n "${blocked_domain}" ]]; then
  echo "validate-lan-dns: blocked-domain policy passed for ${blocked_domain}"
fi
if [[ "${skip_http}" != "1" ]]; then
  echo "validate-lan-dns: HTTP health/readiness passed at ${http_url}"
fi
