#!/usr/bin/env bash
set -euo pipefail

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

missing_out="${tmp}/missing.out"
if SIS_PROD_VALIDATE_STRICT=1 \
  SIS_PROD_VALIDATE_PRECHECK_ONLY=1 \
  ./scripts/run-production-validation.sh >"${missing_out}" 2>&1; then
  echo "production-validation-preflight-smoke: incomplete strict env unexpectedly passed" >&2
  cat "${missing_out}" >&2
  exit 1
fi

if ! grep -q 'SIS_PROD_VALIDATE_STRICT=1 requires SIS_PROD_VALIDATE_API_COOKIE' "${missing_out}"; then
  echo "production-validation-preflight-smoke: missing API cookie was not reported" >&2
  cat "${missing_out}" >&2
  exit 1
fi

complete_out="${tmp}/complete.out"
SIS_PROD_VALIDATE_STRICT=1 \
  SIS_PROD_VALIDATE_PRECHECK_ONLY=1 \
  SIS_PROD_VALIDATE_LAN_DNS_SERVER=192.0.2.10:53 \
  SIS_PROD_VALIDATE_BLOCKED_DOMAIN=blocked.example.com \
  SIS_PROD_VALIDATE_API_COOKIE='sis_session=test' \
  SIS_PROD_VALIDATE_REAL_CLIENT=1 \
  SIS_PROD_VALIDATE_DIAGNOSTICS=1 \
  ./scripts/run-production-validation.sh >"${complete_out}" 2>&1

if ! grep -q 'run-production-validation: preflight passed' "${complete_out}"; then
  echo "production-validation-preflight-smoke: complete strict env did not pass" >&2
  cat "${complete_out}" >&2
  exit 1
fi

echo "production-validation-preflight-smoke: passed"
