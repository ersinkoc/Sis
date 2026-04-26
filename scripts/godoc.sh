#!/usr/bin/env bash
set -euo pipefail

tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

while IFS= read -r file; do
  awk '
    function exported_name(name) {
      return name ~ /^[A-Z]/
    }
    function receiver_exported(line, receiver) {
      receiver = line
      sub(/^func \(/, "", receiver)
      sub(/\).*/, "", receiver)
      gsub(/[*]/, "", receiver)
      sub(/^.* /, "", receiver)
      return exported_name(receiver)
    }
    function exported(line) {
      if (line ~ /^(type|func) [A-Z]/ || line ~ /^var Err/) {
        return 1
      }
      if (line ~ /^func \([^)]+\) [A-Z]/) {
        return receiver_exported(line)
      }
      return 0
    }
    exported($0) && prev !~ /^\/\// {
      printf("%s:%d: exported declaration is missing GoDoc: %s\n", FILENAME, NR, $0)
    }
    { prev = $0 }
  ' "${file}" >> "${tmp}"
done < <(find internal pkg -name '*.go' -not -name '*_test.go' -not -path './internal/webui/dist/*' | sort)

if [[ -s "${tmp}" ]]; then
  cat "${tmp}" >&2
  exit 1
fi

echo "godoc: exported declarations documented"
