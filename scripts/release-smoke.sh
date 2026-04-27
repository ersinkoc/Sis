#!/usr/bin/env bash
set -euo pipefail

dist_dir="${SIS_RELEASE_DIST:-dist}"
linux_bin="${dist_dir}/sis_linux_amd64"
required=(
  "sis_linux_amd64"
  "sis_linux_arm64"
  "sis_darwin_amd64"
  "sis_darwin_arm64"
  "sis.spdx.json"
  "SHA256SUMS"
)

for name in "${required[@]}"; do
  if [[ ! -f "${dist_dir}/${name}" ]]; then
    echo "release-smoke: missing ${dist_dir}/${name}" >&2
    exit 1
  fi
done

if [[ ! -x "${linux_bin}" ]]; then
  echo "release-smoke: ${linux_bin} is not executable" >&2
  exit 1
fi

SIS_RELEASE_DIST="${dist_dir}" ./scripts/verify-release-artifacts.sh

"${linux_bin}" version
"${linux_bin}" config check -config examples/sis.yaml

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

mkdir -p "${tmp}/data"
printf '{}\n' > "${tmp}/data/sis.db.json"
SIS_DATA_DIR="${tmp}/data" "${linux_bin}" backup create -config examples/sis.yaml -out "${tmp}/sis-backup.tar.gz"
"${linux_bin}" backup verify -in "${tmp}/sis-backup.tar.gz"
"${linux_bin}" backup restore -in "${tmp}/sis-backup.tar.gz" -config "${tmp}/restore/sis.yaml" -data-dir "${tmp}/restore/data"
if [[ ! -s "${tmp}/restore/sis.yaml" || ! -s "${tmp}/restore/data/sis.db.json" ]]; then
  echo "release-smoke: backup restore did not write expected files" >&2
  exit 1
fi
SIS_SQLITE_VALIDATE_BIN="${linux_bin}" \
  SIS_SQLITE_VALIDATE_CONFIG="examples/sis.yaml" \
  SIS_SQLITE_VALIDATE_DATA_DIR="${tmp}/data" \
  ./scripts/validate-sqlite-migration.sh >/dev/null

SIS_INSTALL_ROOT="${tmp}" SIS_INSTALL_BIN="${linux_bin}" ./scripts/install-linux-service.sh
"${tmp}/usr/local/bin/sis" version
SIS_VERIFY_BIN="${tmp}/usr/local/bin/sis" \
  SIS_VERIFY_CONFIG="${tmp}/etc/sis/sis.yaml" \
  SIS_VERIFY_SKIP_SYSTEMD=1 \
  SIS_VERIFY_SKIP_HTTP=1 \
  SIS_VERIFY_SKIP_DNS=1 \
  SIS_VERIFY_SKIP_STORE=1 \
  ./scripts/verify-linux-service.sh
bash -n ./scripts/validate-lan-dns.sh
bash -n ./scripts/run-production-validation.sh

for path in \
  "${tmp}/etc/sis/sis.yaml" \
  "${tmp}/etc/sis/sis.env" \
  "${tmp}/etc/systemd/system/sis.service" \
  "${tmp}/usr/local/bin/sis"; do
  if [[ ! -e "${path}" ]]; then
    echo "release-smoke: staged install missing ${path}" >&2
    exit 1
  fi
done

for directive in \
  "NoNewPrivileges=true" \
  "ProtectSystem=strict" \
  "PrivateDevices=true" \
  "RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX" \
  "MemoryDenyWriteExecute=true"; do
  if ! grep -q "^${directive}$" "${tmp}/etc/systemd/system/sis.service"; then
    echo "release-smoke: staged service missing hardening directive ${directive}" >&2
    exit 1
  fi
done

echo "release-smoke: release artifacts and staged Linux install passed"
