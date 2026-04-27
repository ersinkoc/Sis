#!/usr/bin/env bash
set -euo pipefail

bin="${SIS_DIAG_BIN:-/usr/local/bin/sis}"
config="${SIS_DIAG_CONFIG:-/etc/sis/sis.yaml}"
service="${SIS_DIAG_SERVICE:-sis}"
out_dir="${SIS_DIAG_DIR:-sis-diagnostics}"
timestamp="${SIS_DIAG_TIMESTAMP:-$(date -u +%Y%m%dT%H%M%SZ)}"
out="${SIS_DIAG_OUT:-${out_dir}/sis-diagnostics-${timestamp}.tar.gz}"
include_journal="${SIS_DIAG_INCLUDE_JOURNAL:-0}"

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

bundle="${tmp}/bundle"
mkdir -p "${bundle}" "$(dirname "${out}")"

write_cmd() {
  local name="$1"
  shift
  {
    printf '$'
    printf ' %q' "$@"
    printf '\n\n'
    "$@"
  } > "${bundle}/${name}" 2>&1 || {
    status=$?
    printf '\ncommand exited with status %s\n' "${status}" >> "${bundle}/${name}"
  }
}

{
  echo "generated_at=${timestamp}"
  echo "service=${service}"
  echo "bin=${bin}"
  echo "config=${config}"
  echo "include_journal=${include_journal}"
  uname -a
} > "${bundle}/environment.txt" 2>&1

if [[ -x "${bin}" ]]; then
  write_cmd "sis-version.txt" "${bin}" version
  write_cmd "config-check.txt" "${bin}" config check -config "${config}"
  write_cmd "store-verify.txt" "${bin}" store verify -config "${config}"
  sha256sum "${bin}" > "${bundle}/binary-sha256.txt" 2>&1 || true
else
  echo "binary not found or not executable: ${bin}" > "${bundle}/sis-version.txt"
fi

write_cmd "disk-usage.txt" df -h / /var /var/lib /var/backups

if command -v systemctl >/dev/null 2>&1; then
  write_cmd "systemctl-status.txt" systemctl status "${service}" --no-pager
  write_cmd "systemctl-is-enabled.txt" systemctl is-enabled "${service}"
  write_cmd "systemctl-is-active.txt" systemctl is-active "${service}"
fi

if [[ "${include_journal}" == "1" ]]; then
  if command -v journalctl >/dev/null 2>&1; then
    write_cmd "journal-tail.txt" journalctl -u "${service}" -n 200 --no-pager
  fi
else
  cat > "${bundle}/journal-tail.txt" <<EOF
journal collection skipped.
Set SIS_DIAG_INCLUDE_JOURNAL=1 only after removing or accepting possible domain/client data exposure.
EOF
fi

tar -C "${bundle}" -czf "${out}" .
chmod 600 "${out}"

echo "collect-linux-diagnostics: wrote ${out}"
