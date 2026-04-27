#!/usr/bin/env bash
set -euo pipefail

if ! command -v gpg >/dev/null 2>&1; then
  echo "release-signing-key-smoke: gpg is required" >&2
  exit 1
fi

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

out="${tmp}/key-out"
log="${tmp}/helper.out"

SIS_RELEASE_GPG_EMAIL="release-smoke@example.invalid" \
  SIS_RELEASE_GPG_OUT="${out}" \
  ./scripts/generate-release-signing-key.sh > "${log}"

for path in \
  "${out}/release-signing-private-key.asc" \
  "${out}/release-signing-public-key.asc" \
  "${out}/RELEASE_GPG_PRIVATE_KEY_B64.txt"; do
  if [[ ! -s "${path}" ]]; then
    echo "release-signing-key-smoke: missing ${path}" >&2
    exit 1
  fi
done

if ! grep -q "Add the single-line contents" "${log}"; then
  echo "release-signing-key-smoke: helper output did not describe the secret handoff" >&2
  exit 1
fi

if grep -q "BEGIN PGP PRIVATE KEY BLOCK" "${log}"; then
  echo "release-signing-key-smoke: helper output leaked private key material" >&2
  exit 1
fi

decoded="${tmp}/decoded-private-key.asc"
base64 -d "${out}/RELEASE_GPG_PRIVATE_KEY_B64.txt" > "${decoded}"
if ! cmp -s "${out}/release-signing-private-key.asc" "${decoded}"; then
  echo "release-signing-key-smoke: base64 secret does not decode to the private key export" >&2
  exit 1
fi

echo "release-signing-key-smoke: signing key helper passed"
