#!/usr/bin/env bash
set -euo pipefail

bin="${SIS_BACKUP_BIN:-/usr/local/bin/sis}"
config="${SIS_BACKUP_CONFIG:-/etc/sis/sis.yaml}"
out_dir="${SIS_BACKUP_DIR:-/var/backups/sis}"
data_dir="${SIS_BACKUP_DATA_DIR:-}"
timestamp="${SIS_BACKUP_TIMESTAMP:-$(date -u +%Y%m%dT%H%M%SZ)}"
out="${SIS_BACKUP_OUT:-${out_dir}/sis-${timestamp}.tar.gz}"

if [[ ! -x "${bin}" ]]; then
  echo "backup-linux-service: binary not found or not executable: ${bin}" >&2
  exit 1
fi

if [[ ! -f "${config}" ]]; then
  echo "backup-linux-service: config not found: ${config}" >&2
  exit 1
fi

umask 077
mkdir -p "$(dirname "${out}")"

backup_args=(backup create -config "${config}" -out "${out}")
if [[ -n "${data_dir}" ]]; then
  SIS_DATA_DIR="${data_dir}" "${bin}" "${backup_args[@]}"
else
  "${bin}" "${backup_args[@]}"
fi

"${bin}" backup verify -in "${out}"

echo "backup-linux-service: wrote verified backup ${out}"
