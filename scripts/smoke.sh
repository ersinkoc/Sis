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
    ready_out="$(curl -fsS "http://${http_addr}/readyz")"
    if [[ "${ready_out}" != *'"ready":true'* ]]; then
      echo "smoke: readiness check failed" >&2
      echo "${ready_out}" >&2
      exit 1
    fi
    echo "smoke: readiness check passed"

    dns_out="$("${bin}" query -server "${dns_addr}" test localhost A)"
    if [[ "${dns_out}" != *"rcode=NOERROR"* ]]; then
      echo "smoke: DNS query failed" >&2
      echo "${dns_out}" >&2
      exit 1
    fi
    echo "smoke: DNS query passed"

    blocked_out=""
    for _ in $(seq 1 20); do
      blocked_out="$("${bin}" query -server "${dns_addr}" test blocked.example.com A)"
      if [[ "${blocked_out}" == *"rcode=NOERROR"* && "${blocked_out}" == *"answers=1"* && "${blocked_out}" == *"0.0.0.0"* ]]; then
        echo "smoke: blocklist DNS policy passed"
        break
      fi
      sleep 0.1
    done
    if [[ "${blocked_out}" != *"0.0.0.0"* ]]; then
      echo "smoke: blocklist DNS policy failed" >&2
      echo "${blocked_out}" >&2
      exit 1
    fi

    curl -fsS -c "${tmp}/cookies.txt" \
      -H 'content-type: application/json' \
      -d '{"username":"admin","password":"change-me-now"}' \
      "http://${http_addr}/api/v1/auth/setup" >/dev/null
    curl -fsS -b "${tmp}/cookies.txt" "http://${http_addr}/api/v1/stats/summary" >/dev/null
    api_query_out="$(curl -fsS -b "${tmp}/cookies.txt" \
      -H 'content-type: application/json' \
      -d '{"domain":"blocked.example.com","type":"A"}' \
      "http://${http_addr}/api/v1/query/test")"
    if [[ "${api_query_out}" != *'"source":"synthetic"'* || "${api_query_out}" != *"0.0.0.0"* ]]; then
      echo "smoke: API query policy failed" >&2
      echo "${api_query_out}" >&2
      exit 1
    fi
    logs_out=""
    for _ in $(seq 1 20); do
      logs_out="$(curl -fsS -b "${tmp}/cookies.txt" \
        "http://${http_addr}/api/v1/logs/query?blocked=true&qname=blocked.example.com&limit=10")"
      if [[ "${logs_out}" == *"blocked.example.com."* && "${logs_out}" == *'"blocked":true'* ]]; then
        echo "smoke: query log API passed"
        break
      fi
      sleep 0.1
    done
    if [[ "${logs_out}" != *'"blocked":true'* ]]; then
      echo "smoke: query log API failed" >&2
      echo "${logs_out}" >&2
      exit 1
    fi
    stats_out="$(curl -fsS -b "${tmp}/cookies.txt" "http://${http_addr}/api/v1/stats/summary")"
    if [[ "${stats_out}" != *'"blocked_total":'* || "${stats_out}" == *'"blocked_total":0'* ]]; then
      echo "smoke: stats summary did not record blocked query" >&2
      echo "${stats_out}" >&2
      exit 1
    fi
    top_out="$(curl -fsS -b "${tmp}/cookies.txt" "http://${http_addr}/api/v1/stats/top-domains?blocked=true&limit=5")"
    if [[ "${top_out}" != *"blocked.example.com."* ]]; then
      echo "smoke: blocked top-domains did not include blocked.example.com" >&2
      echo "${top_out}" >&2
      exit 1
    fi
    echo "smoke: stats API passed"
    cache_out="$(curl -fsS -b "${tmp}/cookies.txt" -X POST "http://${http_addr}/api/v1/system/cache/flush")"
    if [[ "${cache_out}" != *'"flushed":true'* || "${cache_out}" != *'"entries":'* ]]; then
      echo "smoke: cache flush failed" >&2
      echo "${cache_out}" >&2
      exit 1
    fi
    echo "smoke: cache flush passed"

    reload_out="$(curl -fsS -b "${tmp}/cookies.txt" -X POST "http://${http_addr}/api/v1/system/config/reload")"
    if [[ "${reload_out}" != *'"reloaded":true'* ]]; then
      echo "smoke: config reload failed" >&2
      echo "${reload_out}" >&2
      exit 1
    fi
    history_out="$(curl -fsS -b "${tmp}/cookies.txt" "http://${http_addr}/api/v1/system/config/history?limit=1")"
    if [[ "${history_out}" != *'"snapshots":['* || "${history_out}" != *"server:"* ]]; then
      echo "smoke: config history missing reload snapshot" >&2
      echo "${history_out}" >&2
      exit 1
    fi
    echo "smoke: config reload and history passed"
    echo "smoke: auth setup, API summary, and API query policy passed"
    exit 0
  fi
  sleep 0.1
done

echo "smoke: serve health check failed" >&2
cat "${tmp}/serve.log" >&2
exit 1
