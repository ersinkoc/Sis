#!/usr/bin/env bash
set -euo pipefail

tag="v0.0.0-rc.1"
release_branch="$(git branch --show-current)"
tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

pending_record="${tmp}/pending.md"
complete_record="${tmp}/complete.md"
missing_result_record="${tmp}/missing-result.md"
missing_metadata_record="${tmp}/missing-metadata.md"

cat > "${pending_record}" <<'EOF'
# Production Validation Record

- Status: Pending live host validation
- Last production validation report:

<!-- sis-validation-summary:start -->
```text
Paste validation summary here.
```
<!-- sis-validation-summary:end -->

## Host Details

- Validation date:
- Sis version:
- Commit or release tag:
- Host OS/kernel:
- Host IP:
- DNS listen address:
- HTTP listen address:
- Store backend:
- Data directory:
- Router/DHCP DNS setting:

## Results

| Check | Result | Notes |
| --- | --- | --- |
| Service verification | Pending | |
EOF

pending_out="${tmp}/pending.out"
complete_out="${tmp}/complete.out"

if SIS_RELEASE_ALLOW_DIRTY=1 SIS_RELEASE_BRANCH="${release_branch}" SIS_RELEASE_VALIDATION_RECORD="${pending_record}" ./scripts/release-candidate-check.sh "${tag}" >"${pending_out}" 2>&1; then
  echo "release-candidate-check-smoke: pending record unexpectedly passed" >&2
  cat "${pending_out}" >&2
  exit 1
fi

cat > "${complete_record}" <<'EOF'
# Production Validation Record

- Status: Validation report recorded
- Last production validation report: 20260428T120000Z
- Validation binary: /usr/local/bin/sis
- Validation config: /etc/sis/sis.yaml
- Validation LAN DNS server: 192.0.2.10:53
- Validation API URL: http://127.0.0.1:8080

<!-- sis-validation-summary:start -->
```text
- PASS: service verification
- PASS: SQLite migration dry-run
- PASS: LAN DNS validation
- PASS: authenticated API store verification
- PASS: real client observation
- PASS: diagnostics bundle
```
<!-- sis-validation-summary:end -->

## Host Details

- Validation date: 2026-04-28
- Sis version: v0.0.0-rc.1
- Commit or release tag: abc123
- Host OS/kernel: Linux test
- Host IP: 192.0.2.10
- DNS listen address: 0.0.0.0:53
- HTTP listen address: 127.0.0.1:8080
- Store backend: sqlite
- Data directory: /var/lib/sis
- Router/DHCP DNS setting: 192.0.2.10

## Results

| Check | Result | Notes |
| --- | --- | --- |
| Service verification | Pass | |
| SQLite migration dry-run | Pass | |
| LAN UDP DNS | Pass | |
| LAN TCP DNS | Pass | |
| Blocked-domain policy | Pass | |
| HTTP health/readiness | Pass | |
| Authenticated API store verification | Pass | |
| Real client query observed | Pass | |
| Diagnostics bundle generated | Pass | |
EOF

SIS_RELEASE_ALLOW_DIRTY=1 SIS_RELEASE_BRANCH="${release_branch}" SIS_RELEASE_VALIDATION_RECORD="${complete_record}" ./scripts/release-candidate-check.sh "${tag}" >"${complete_out}"

sed '/| Real client query observed |/d' "${complete_record}" > "${missing_result_record}"
missing_result_out="${tmp}/missing-result.out"
if SIS_RELEASE_ALLOW_DIRTY=1 SIS_RELEASE_BRANCH="${release_branch}" SIS_RELEASE_VALIDATION_RECORD="${missing_result_record}" ./scripts/release-candidate-check.sh "${tag}" >"${missing_result_out}" 2>&1; then
  echo "release-candidate-check-smoke: missing result record unexpectedly passed" >&2
  cat "${missing_result_out}" >&2
  exit 1
fi

if ! grep -q 'production validation result is missing or not Pass: Real client query observed' "${missing_result_out}"; then
  echo "release-candidate-check-smoke: missing result was not reported" >&2
  cat "${missing_result_out}" >&2
  exit 1
fi

sed '/^- Validation API URL:/d' "${complete_record}" > "${missing_metadata_record}"
missing_metadata_out="${tmp}/missing-metadata.out"
if SIS_RELEASE_ALLOW_DIRTY=1 SIS_RELEASE_BRANCH="${release_branch}" SIS_RELEASE_VALIDATION_RECORD="${missing_metadata_record}" ./scripts/release-candidate-check.sh "${tag}" >"${missing_metadata_out}" 2>&1; then
  echo "release-candidate-check-smoke: missing metadata record unexpectedly passed" >&2
  cat "${missing_metadata_out}" >&2
  exit 1
fi

if ! grep -q 'validation metadata is empty: Validation API URL' "${missing_metadata_out}"; then
  echo "release-candidate-check-smoke: missing metadata was not reported" >&2
  cat "${missing_metadata_out}" >&2
  exit 1
fi

echo "release-candidate-check-smoke: passed"
