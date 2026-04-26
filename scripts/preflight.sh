#!/usr/bin/env bash
set -euo pipefail

missing=0

require_tool() {
  local tool="$1"
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "missing required tool: ${tool}" >&2
    missing=1
  fi
}

require_tool go
require_tool gofmt
require_tool npm

if (( missing != 0 )); then
  exit 1
fi

go version
gofmt --help >/dev/null
npm --version
echo "preflight: required tools found"
