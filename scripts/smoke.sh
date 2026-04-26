#!/usr/bin/env bash
set -euo pipefail

bin="${SIS_SMOKE_BIN:-bin/sis}"
dns_addr="${SIS_SMOKE_DNS_ADDR:-127.0.0.1:15353}"
http_addr="${SIS_SMOKE_HTTP_ADDR:-127.0.0.1:18080}"

if [[ ! -x "${bin}" ]]; then
  echo "smoke: binary not found or not executable: ${bin}" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "smoke: missing required tool: curl" >&2
  exit 1
fi

tmp="$(mktemp -d)"
pid=""

cleanup() {
  if [[ -n "${pid}" ]]; then
    kill -TERM "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  fi
  rm -rf "${tmp}"
}
trap cleanup EXIT

cat > "${tmp}/ads.txt" <<'EOF'
blocked.example.com
EOF

sed \
  -e "s#./data#${tmp}/data#g" \
  -e "s#127.0.0.1:5353#${dns_addr}#g" \
  -e "s#127.0.0.1:8080#${http_addr}#g" \
  -e "s#https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts#file://${tmp}/ads.txt#g" \
  examples/sis.yaml > "${tmp}/sis.yaml"

"${bin}" config check -config "${tmp}/sis.yaml" >/dev/null
"${bin}" serve -config "${tmp}/sis.yaml" >"${tmp}/serve.log" 2>&1 &
pid="$!"

for _ in $(seq 1 50); do
  if curl -fsS "http://${http_addr}/healthz" >/dev/null 2>&1; then
    echo "smoke: serve health check passed"
    dns_out="$("${bin}" query -server "${dns_addr}" test localhost A)"
    if [[ "${dns_out}" != *"rcode=NOERROR"* ]]; then
      echo "smoke: DNS query failed" >&2
      echo "${dns_out}" >&2
      exit 1
    fi
    echo "smoke: DNS query passed"

    curl -fsS -c "${tmp}/cookies.txt" \
      -H 'content-type: application/json' \
      -d '{"username":"admin","password":"change-me-now"}' \
      "http://${http_addr}/api/v1/auth/setup" >/dev/null
    curl -fsS -b "${tmp}/cookies.txt" "http://${http_addr}/api/v1/stats/summary" >/dev/null
    echo "smoke: auth setup and API summary passed"
    exit 0
  fi
  sleep 0.1
done

echo "smoke: serve health check failed" >&2
cat "${tmp}/serve.log" >&2
exit 1
