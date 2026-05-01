#!/usr/bin/env bash
set -euo pipefail

bin="${SIS_LOAD_BIN:-bin/sis}"
dns_addr="${SIS_LOAD_DNS_ADDR:-127.0.0.1:15355}"
http_addr="${SIS_LOAD_HTTP_ADDR:-127.0.0.1:18082}"
duration="${SIS_LOAD_DURATION_SECONDS:-10}"
dns_workers="${SIS_LOAD_DNS_WORKERS:-4}"
api_workers="${SIS_LOAD_API_WORKERS:-2}"
domain="${SIS_LOAD_DOMAIN:-localhost}"
blocked_domain="${SIS_LOAD_BLOCKED_DOMAIN:-blocked.example.com}"
disable_rate_limit="${SIS_LOAD_DISABLE_RATE_LIMIT:-1}"

if [[ ! "${duration}" =~ ^[0-9]+$ || ! "${dns_workers}" =~ ^[0-9]+$ || ! "${api_workers}" =~ ^[0-9]+$ ]]; then
  echo "local-load: duration and worker counts must be non-negative integers" >&2
  exit 1
fi
duration=$((10#${duration}))
dns_workers=$((10#${dns_workers}))
api_workers=$((10#${api_workers}))

if [[ ! -x "${bin}" ]]; then
  echo "local-load: binary not found or not executable: ${bin}" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "local-load: missing required tool: curl" >&2
  exit 1
fi

if (( duration <= 0 || dns_workers + api_workers == 0 )); then
  echo "local-load: duration must be > 0 and at least one worker must be enabled" >&2
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

cat >"${tmp}/ads.txt" <<EOF
${blocked_domain}
EOF

sed \
  -e "s#./data#${tmp}/data#g" \
  -e "s#127.0.0.1:5353#${dns_addr}#g" \
  -e "s#127.0.0.1:8080#${http_addr}#g" \
  -e "s#https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts#file://${tmp}/ads.txt#g" \
  examples/sis.yaml >"${tmp}/sis.yaml"

"${bin}" config check -config "${tmp}/sis.yaml" >/dev/null
if [[ "${disable_rate_limit}" == "1" ]]; then
  SIS_DNS_RATE_LIMIT_QPS=0 SIS_HTTP_RATE_LIMIT_PER_MINUTE=0 \
    "${bin}" serve -config "${tmp}/sis.yaml" >"${tmp}/serve.log" 2>&1 &
else
  "${bin}" serve -config "${tmp}/sis.yaml" >"${tmp}/serve.log" 2>&1 &
fi
pid="$!"

ready="0"
for _ in $(seq 1 50); do
  if curl -fsS "http://${http_addr}/healthz" >/dev/null 2>&1; then
    ready_out="$(curl -fsS "http://${http_addr}/readyz")"
    if [[ "${ready_out}" == *'"ready":true'* ]]; then
      ready="1"
      break
    fi
  fi
  sleep 0.1
done

if [[ "${ready}" != "1" ]]; then
  echo "local-load: service did not become ready" >&2
  cat "${tmp}/serve.log" >&2
  exit 1
fi

curl -fsS -c "${tmp}/cookies.txt" \
  -H 'content-type: application/json' \
  -d '{"username":"admin","password":"change-me-now"}' \
  "http://${http_addr}/api/v1/auth/setup" >/dev/null

deadline=$(( $(date +%s) + duration ))

run_dns_worker() {
  local id="$1"
  local ok=0
  local err=0
  while (( $(date +%s) < deadline )); do
    if "${bin}" query -server "${dns_addr}" test "${domain}" A >/dev/null 2>&1; then
      ok=$((ok + 1))
    else
      err=$((err + 1))
    fi
    if "${bin}" query -server "${dns_addr}" test "${blocked_domain}" A >/dev/null 2>&1; then
      ok=$((ok + 1))
    else
      err=$((err + 1))
    fi
  done
  printf '%s %s\n' "${ok}" "${err}" >"${tmp}/dns-${id}.count"
}

run_api_worker() {
  local id="$1"
  local ok=0
  local err=0
  while (( $(date +%s) < deadline )); do
    if curl -fsS -b "${tmp}/cookies.txt" "http://${http_addr}/api/v1/stats/summary" >/dev/null 2>&1; then
      ok=$((ok + 1))
    else
      err=$((err + 1))
    fi
    if curl -fsS -b "${tmp}/cookies.txt" \
      -H 'content-type: application/json' \
      -d "{\"domain\":\"${blocked_domain}\",\"type\":\"A\"}" \
      "http://${http_addr}/api/v1/query/test" >/dev/null 2>&1; then
      ok=$((ok + 1))
    else
      err=$((err + 1))
    fi
  done
  printf '%s %s\n' "${ok}" "${err}" >"${tmp}/api-${id}.count"
}

start_ns="$(date +%s%N)"
worker_pids=()
for ((i = 1; i <= dns_workers; i++)); do
  run_dns_worker "${i}" &
  worker_pids+=("$!")
done
for ((i = 1; i <= api_workers; i++)); do
  run_api_worker "${i}" &
  worker_pids+=("$!")
done
for worker_pid in "${worker_pids[@]}"; do
  wait "${worker_pid}"
done
end_ns="$(date +%s%N)"

sum_counts() {
  local pattern="$1"
  local ok=0
  local err=0
  local file
  for file in ${pattern}; do
    [[ -e "${file}" ]] || continue
    read -r file_ok file_err <"${file}"
    ok=$((ok + file_ok))
    err=$((err + file_err))
  done
  printf '%s %s\n' "${ok}" "${err}"
}

read -r dns_ok dns_err < <(sum_counts "${tmp}/dns-"*.count)
read -r api_ok api_err < <(sum_counts "${tmp}/api-"*.count)
total_ok=$((dns_ok + api_ok))
total_err=$((dns_err + api_err))
elapsed="$(awk -v start="${start_ns}" -v end="${end_ns}" 'BEGIN { printf "%.3f", (end - start) / 1000000000 }')"
rate="$(awk -v total="${total_ok}" -v seconds="${elapsed}" 'BEGIN { if (seconds > 0) printf "%.1f", total / seconds; else printf "0.0" }')"

summary="$(curl -fsS -b "${tmp}/cookies.txt" "http://${http_addr}/api/v1/stats/summary")"

cat <<EOF
local-load: completed
- duration_seconds: ${elapsed}
- dns_workers: ${dns_workers}
- api_workers: ${api_workers}
- rate_limit_disabled: ${disable_rate_limit}
- dns_ok: ${dns_ok}
- dns_errors: ${dns_err}
- api_ok: ${api_ok}
- api_errors: ${api_err}
- total_ok: ${total_ok}
- total_errors: ${total_err}
- approx_successes_per_second: ${rate}
- stats_summary: ${summary}
EOF

if (( total_ok == 0 )); then
  echo "local-load: completed without any successful operations" >&2
  exit 1
fi
if (( total_err != 0 )); then
  echo "local-load: completed with errors" >&2
  exit 1
fi
