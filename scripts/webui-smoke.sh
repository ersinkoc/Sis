#!/usr/bin/env bash
set -euo pipefail

bin="${SIS_WEBUI_SMOKE_BIN:-bin/sis}"
dns_addr="${SIS_WEBUI_SMOKE_DNS_ADDR:-127.0.0.1:15354}"
http_addr="${SIS_WEBUI_SMOKE_HTTP_ADDR:-127.0.0.1:18081}"

if [[ ! -x "${bin}" ]]; then
  echo "webui-smoke: binary not found or not executable: ${bin}" >&2
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
    blocked_out=""
    for _ in $(seq 1 20); do
      blocked_out="$("${bin}" query -server "${dns_addr}" test blocked.example.com A)"
      if [[ "${blocked_out}" == *"0.0.0.0"* ]]; then
        break
      fi
      sleep 0.1
    done
    if [[ "${blocked_out}" != *"0.0.0.0"* ]]; then
      echo "webui-smoke: blocklist DNS policy failed" >&2
      echo "${blocked_out}" >&2
      exit 1
    fi

    (
      cd webui
      SIS_WEBUI_BASE_URL="http://${http_addr}" npx playwright test
    )
    echo "webui-smoke: first-run dashboard flow passed"
    exit 0
  fi
  sleep 0.1
done

echo "webui-smoke: serve health check failed" >&2
cat "${tmp}/serve.log" >&2
exit 1
