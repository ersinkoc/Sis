#!/usr/bin/env bash
set -euo pipefail

tag="v0.0.0-rc.1"
tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

record="${tmp}/production-validation.md"
report="${tmp}/production-validation-report.md"
update_out="${tmp}/update.out"
gate_out="${tmp}/gate.out"

cat > "${record}" <<'EOF'
# Production Validation Record

## Current Status

- Status: Pending live host validation
- Last repository release gate: passing
- Last GitHub CI gate: passing
- Last production validation report:
- Validation binary:
- Validation config:
- Validation LAN DNS server:
- Validation API URL:

<!-- sis-validation-summary:start -->
```text
Paste validation summary here.
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
| Service verification | Pending | |
| SQLite migration dry-run | Pending | |
| LAN UDP DNS | Pending | |
| LAN TCP DNS | Pending | |
| Blocked-domain policy | Pending | |
| HTTP health/readiness | Pending | |
| Authenticated API store verification | Pending | |
| Real client query observed | Pending | |
| Diagnostics bundle generated | Pending | |
EOF

cat > "${report}" <<'EOF'
# Sis Production Validation Report

- Generated: 20260428T120000Z
- Binary: /usr/local/bin/sis
- Config: /etc/sis/sis.yaml
- DNS server: 127.0.0.1:53
- LAN DNS server: 192.0.2.10:53
- API URL: http://127.0.0.1:8080

## Summary

- PASS: service verification
- PASS: SQLite migration dry-run
- PASS: LAN DNS validation
- PASS: authenticated API store verification
- PASS: real client observation
- PASS: diagnostics bundle

## Details

Fixture report for record update smoke testing.
EOF

SIS_PROD_VALIDATE_RECORD="${record}" ./scripts/update-production-validation-record.sh "${report}" >"${update_out}"

for expected in \
  "- Status: Validation report recorded" \
  "- Last production validation report: 20260428T120000Z" \
  "- Validation binary: /usr/local/bin/sis" \
  "- Validation config: /etc/sis/sis.yaml" \
  "- Validation LAN DNS server: 192.0.2.10:53" \
  "- Validation API URL: http://127.0.0.1:8080" \
  "| Service verification | Pass | |" \
  "| SQLite migration dry-run | Pass | |" \
  "| LAN UDP DNS | Pass | Covered by LAN DNS validation report when enabled. |" \
  "| LAN TCP DNS | Pass | Covered by LAN DNS validation report when enabled. |" \
  "| Blocked-domain policy | Pass | Covered when SIS_PROD_VALIDATE_BLOCKED_DOMAIN is set. |" \
  "| HTTP health/readiness | Pass | Also covered by LAN DNS validation when HTTP is enabled. |" \
  "| Authenticated API store verification | Pass | |" \
  "| Real client query observed | Pass | Use SIS_PROD_VALIDATE_REAL_CLIENT=1 during live validation. |" \
  "| Diagnostics bundle generated | Pass | |"; do
  if ! grep -Fq -- "${expected}" "${record}"; then
    echo "update-production-validation-record-smoke: missing expected record line: ${expected}" >&2
    cat "${record}" >&2
    exit 1
  fi
done

if grep -q '| .* | Pending |' "${record}"; then
  echo "update-production-validation-record-smoke: record still contains pending result rows" >&2
  cat "${record}" >&2
  exit 1
fi

SIS_RELEASE_ALLOW_DIRTY=1 SIS_RELEASE_VALIDATION_RECORD="${record}" ./scripts/release-candidate-check.sh "${tag}" >"${gate_out}"

echo "update-production-validation-record-smoke: passed"
