#!/usr/bin/env bash
set -euo pipefail

dist_dir="${SIS_RELEASE_DIST:-dist}"
linux_bin="${dist_dir}/sis_linux_amd64"
required=(
  "sis_linux_amd64"
  "sis_linux_arm64"
  "sis_darwin_amd64"
  "sis_darwin_arm64"
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

(
  cd "${dist_dir}"
  sha256sum -c SHA256SUMS
)

"${linux_bin}" version
"${linux_bin}" config check -config examples/sis.yaml

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

SIS_INSTALL_ROOT="${tmp}" SIS_INSTALL_BIN="${linux_bin}" ./scripts/install-linux-service.sh
"${tmp}/usr/local/bin/sis" version

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

echo "release-smoke: release artifacts and staged Linux install passed"
