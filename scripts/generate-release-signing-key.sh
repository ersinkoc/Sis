#!/usr/bin/env bash
set -euo pipefail

name="${SIS_RELEASE_GPG_NAME:-Sis Release Signing}"
email="${SIS_RELEASE_GPG_EMAIL:-release@example.invalid}"
expire="${SIS_RELEASE_GPG_EXPIRE:-1y}"
out_dir="${SIS_RELEASE_GPG_OUT:-release-signing-key}"

if ! command -v gpg >/dev/null 2>&1; then
  echo "generate-release-signing-key: gpg is required" >&2
  exit 1
fi

if [[ -e "${out_dir}" ]]; then
  echo "generate-release-signing-key: refusing to overwrite ${out_dir}" >&2
  exit 1
fi

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

export GNUPGHOME="${tmp}/gnupg"
mkdir -p "${GNUPGHOME}" "${out_dir}"
chmod 700 "${GNUPGHOME}"

cat > "${tmp}/key.batch" <<EOF
%no-protection
Key-Type: eddsa
Key-Curve: ed25519
Key-Usage: sign
Name-Real: ${name}
Name-Email: ${email}
Expire-Date: ${expire}
%commit
EOF

gpg --batch --generate-key "${tmp}/key.batch"
fingerprint="$(gpg --batch --with-colons --list-secret-keys "${email}" | awk -F: '/^fpr:/ { print $10; exit }')"

if [[ -z "${fingerprint}" ]]; then
  echo "generate-release-signing-key: failed to resolve generated key fingerprint" >&2
  exit 1
fi

private_key="${out_dir}/release-signing-private-key.asc"
public_key="${out_dir}/release-signing-public-key.asc"
secret_value="${out_dir}/RELEASE_GPG_PRIVATE_KEY_B64.txt"

gpg --batch --armor --export-secret-keys "${fingerprint}" > "${private_key}"
gpg --batch --armor --export "${fingerprint}" > "${public_key}"
base64 -w0 "${private_key}" > "${secret_value}"
printf '\n' >> "${secret_value}"
chmod 600 "${private_key}" "${secret_value}"

cat <<EOF
generate-release-signing-key: wrote ${out_dir}

Fingerprint:
${fingerprint}

GitHub secret value:
  Add the single-line contents of ${secret_value} as RELEASE_GPG_PRIVATE_KEY_B64.

Store ${private_key} outside the repository and delete ${out_dir} after adding the GitHub secret.
EOF
