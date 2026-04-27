#!/usr/bin/env bash
set -euo pipefail

dist_dir="${SIS_RELEASE_DIST:-dist}"
checksums="${dist_dir}/SHA256SUMS"
signature="${dist_dir}/SHA256SUMS.asc"

if [[ ! -f "${checksums}" ]]; then
  echo "sign-release: missing ${checksums}" >&2
  exit 1
fi

if [[ -n "${RELEASE_GPG_PRIVATE_KEY_B64:-}" ]]; then
  tmp="$(mktemp -d)"
  cleanup() {
    rm -rf "${tmp}"
  }
  trap cleanup EXIT
  export GNUPGHOME="${tmp}/gnupg"
  mkdir -p "${GNUPGHOME}"
  chmod 700 "${GNUPGHOME}"
  printf '%s' "${RELEASE_GPG_PRIVATE_KEY_B64}" | base64 -d | gpg --batch --import

  passphrase_args=()
  if [[ -n "${RELEASE_GPG_PASSPHRASE:-}" ]]; then
    passphrase_args=(--pinentry-mode loopback --passphrase "${RELEASE_GPG_PASSPHRASE}")
  fi

  gpg --batch --yes "${passphrase_args[@]}" --armor --detach-sign --output "${signature}" "${checksums}"
  gpg --batch --armor --export > "${dist_dir}/release-signing-public-key.asc"
  echo "sign-release: wrote ${signature}"
  exit 0
fi

if [[ -n "${RELEASE_GPG_KEY_ID:-}" ]]; then
  gpg --batch --yes --armor --local-user "${RELEASE_GPG_KEY_ID}" --detach-sign --output "${signature}" "${checksums}"
  gpg --batch --armor --export "${RELEASE_GPG_KEY_ID}" > "${dist_dir}/release-signing-public-key.asc"
  echo "sign-release: wrote ${signature}"
  exit 0
fi

echo "sign-release: no signing key configured; set RELEASE_GPG_PRIVATE_KEY_B64 or RELEASE_GPG_KEY_ID to sign release checksums"
