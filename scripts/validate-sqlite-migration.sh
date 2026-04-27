#!/usr/bin/env bash
set -euo pipefail

bin="${SIS_SQLITE_VALIDATE_BIN:-/usr/local/bin/sis}"
config="${SIS_SQLITE_VALIDATE_CONFIG:-/etc/sis/sis.yaml}"
backup_out="${SIS_SQLITE_VALIDATE_BACKUP:-}"
data_dir="${SIS_SQLITE_VALIDATE_DATA_DIR:-}"
keep_tmp="${SIS_SQLITE_VALIDATE_KEEP_TMP:-0}"

if [[ ! -x "${bin}" ]]; then
  echo "validate-sqlite-migration: binary not found or not executable: ${bin}" >&2
  exit 1
fi

if [[ ! -f "${config}" ]]; then
  echo "validate-sqlite-migration: config not found: ${config}" >&2
  exit 1
fi

tmp="$(mktemp -d)"
cleanup() {
  if [[ "${keep_tmp}" == "1" ]]; then
    echo "validate-sqlite-migration: kept temp dir ${tmp}"
  else
    rm -rf "${tmp}"
  fi
}
trap cleanup EXIT

backup="${backup_out:-${tmp}/sis-pre-sqlite.tar.gz}"
restore_config="${tmp}/restore/sis.yaml"
restore_data="${tmp}/restore/data"
export_json="${tmp}/sqlite-export.json"

if [[ -n "${data_dir}" ]]; then
  SIS_DATA_DIR="${data_dir}" "${bin}" config check -config "${config}" >/dev/null
  SIS_DATA_DIR="${data_dir}" "${bin}" backup create -config "${config}" -out "${backup}" >/dev/null
else
  "${bin}" config check -config "${config}" >/dev/null
  "${bin}" backup create -config "${config}" -out "${backup}" >/dev/null
fi
"${bin}" backup verify -in "${backup}" >/dev/null
"${bin}" backup restore -in "${backup}" -config "${restore_config}" -data-dir "${restore_data}" >/dev/null
"${bin}" store migrate-json-to-sqlite -data-dir "${restore_data}" >/dev/null
"${bin}" store export-sqlite-json -data-dir "${restore_data}" -out "${export_json}" >/dev/null
SIS_DATA_DIR="${restore_data}" SIS_STORE_BACKEND=sqlite "${bin}" config check -config "${restore_config}" >/dev/null

if [[ ! -s "${restore_data}/sis.db" ]]; then
  echo "validate-sqlite-migration: migrated SQLite store missing or empty" >&2
  exit 1
fi

if [[ ! -s "${export_json}" ]]; then
  echo "validate-sqlite-migration: SQLite JSON export missing or empty" >&2
  exit 1
fi

echo "validate-sqlite-migration: SQLite dry-run migration passed"
echo "validate-sqlite-migration: backup ${backup}"
