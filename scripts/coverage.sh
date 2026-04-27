#!/usr/bin/env bash
set -euo pipefail

threshold="${COVERAGE_THRESHOLD:-60.0}"
profile="${COVERAGE_PROFILE:-coverage.out}"
packages="${GO_PACKAGES:-$(go list ./... | grep -v '/webui/node_modules/')}"

go test -count=1 -covermode=atomic -coverpkg="${packages//$'\n'/,}" -coverprofile="${profile}" ${packages}

total="$(go tool cover -func="${profile}" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
if [[ -z "${total}" ]]; then
  echo "coverage: unable to read total coverage" >&2
  exit 1
fi

awk -v total="${total}" -v threshold="${threshold}" 'BEGIN {
  if (total + 0 < threshold + 0) {
    printf("coverage %.1f%% is below required %.1f%%\n", total, threshold) > "/dev/stderr"
    exit 1
  }
  printf("coverage %.1f%% meets required %.1f%%\n", total, threshold)
}'
