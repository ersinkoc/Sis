#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
work_dir="${2:-dist/${version:-release}}"

if [[ -z "${version}" || ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "usage: ./scripts/install-release-linux.sh vMAJOR.MINOR.PATCH[-prerelease] [work-dir]" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64 | amd64)
    linux_bin="sis_linux_amd64"
    ;;
  aarch64 | arm64)
    linux_bin="sis_linux_arm64"
    ;;
  *)
    echo "install-release-linux: unsupported Linux architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

./scripts/download-release.sh "${version}" "${work_dir}"

install_bin="${work_dir}/${linux_bin}"
if [[ ! -x "${install_bin}" ]]; then
  echo "install-release-linux: downloaded binary is not executable: ${install_bin}" >&2
  exit 1
fi

SIS_INSTALL_BIN="${install_bin}" ./scripts/install-linux-service.sh

if [[ -z "${SIS_INSTALL_ROOT:-}" && "${SIS_INSTALL_RELEASE_ENABLE_SYSTEMD:-1}" == "1" ]]; then
  systemctl enable --now "${SIS_INSTALL_SERVICE:-sis}"
fi

if [[ -n "${SIS_INSTALL_ROOT:-}" ]]; then
  SIS_VERIFY_BIN="${SIS_INSTALL_ROOT}/usr/local/bin/sis" \
    SIS_VERIFY_CONFIG="${SIS_INSTALL_ROOT}/etc/sis/sis.yaml" \
    SIS_VERIFY_SKIP_SYSTEMD=1 \
    SIS_VERIFY_SKIP_HTTP=1 \
    SIS_VERIFY_SKIP_DNS=1 \
    ./scripts/verify-linux-service.sh
else
  ./scripts/verify-linux-service.sh
fi

echo "install-release-linux: ${version} installed from ${install_bin}"
