#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
work_dir="${2:-dist/${version:-release}}"

if [[ -z "${version}" || ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "usage: ./scripts/upgrade-release-linux.sh vMAJOR.MINOR.PATCH[-prerelease] [work-dir]" >&2
  exit 1
fi

service="${SIS_UPGRADE_SERVICE:-${SIS_INSTALL_SERVICE:-sis}}"

./scripts/backup-linux-service.sh

if [[ -z "${SIS_INSTALL_ROOT:-}" && "${SIS_UPGRADE_STOP_SERVICE:-1}" == "1" ]]; then
  if command -v systemctl >/dev/null 2>&1; then
    systemctl stop "${service}"
  else
    echo "upgrade-release-linux: systemctl not found; set SIS_UPGRADE_STOP_SERVICE=0 to skip service stop" >&2
    exit 1
  fi
fi

./scripts/install-release-linux.sh "${version}" "${work_dir}"

echo "upgrade-release-linux: ${version} upgraded with a verified pre-upgrade backup"
