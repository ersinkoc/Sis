#!/usr/bin/env bash
set -euo pipefail

dist_dir="${SIS_RELEASE_DIST:-dist}"
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
    echo "verify-release-artifacts: missing ${dist_dir}/${name}" >&2
    exit 1
  fi
done

(
  cd "${dist_dir}"
  sha256sum -c SHA256SUMS

  if [[ -f SHA256SUMS.asc ]]; then
    if [[ ! -f release-signing-public-key.asc ]]; then
      echo "verify-release-artifacts: SHA256SUMS.asc exists but release-signing-public-key.asc is missing" >&2
      exit 1
    fi
    verify_home="$(mktemp -d)"
    cleanup_verify_home() {
      rm -rf "${verify_home}"
    }
    trap cleanup_verify_home EXIT
    export GNUPGHOME="${verify_home}"
    chmod 700 "${GNUPGHOME}"
    gpg --batch --import release-signing-public-key.asc
    gpg --batch --verify SHA256SUMS.asc SHA256SUMS
  fi

  grep -q '"spdxVersion": "SPDX-2.3"' sis.spdx.json
  grep -q '"name": "sis"' sis.spdx.json
  grep -q 'pkg:github/ersinkoc/Sis' sis.spdx.json
)

echo "verify-release-artifacts: release artifacts verified"
