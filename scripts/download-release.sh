#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
out_dir="${2:-dist}"
repo="${SIS_RELEASE_REPO:-ersinkoc/Sis}"
base_url="${SIS_RELEASE_BASE_URL:-https://github.com/${repo}/releases/download}"

if [[ -z "${version}" || ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "usage: ./scripts/download-release.sh vMAJOR.MINOR.PATCH[-prerelease] [out-dir]" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "download-release: curl is required" >&2
  exit 1
fi

mkdir -p "${out_dir}"

assets=(
  "sis_linux_amd64"
  "sis_linux_arm64"
  "sis_darwin_amd64"
  "sis_darwin_arm64"
  "sis.spdx.json"
  "SHA256SUMS"
  "SHA256SUMS.asc"
  "release-signing-public-key.asc"
)

for asset in "${assets[@]}"; do
  url="${base_url}/${version}/${asset}"
  echo "download-release: ${url}"
  curl --fail --location --show-error --silent --output "${out_dir}/${asset}" "${url}"
done

chmod +x "${out_dir}/sis_linux_amd64" "${out_dir}/sis_linux_arm64"
SIS_RELEASE_DIST="${out_dir}" ./scripts/verify-release-artifacts.sh

echo "download-release: ${version} downloaded and verified in ${out_dir}"
