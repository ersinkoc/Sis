#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "${repo_root}"

patterns=(
  'AKIA[0-9A-Z]{16}'
  'ASIA[0-9A-Z]{16}'
  '-----BEGIN (RSA |DSA |EC |OPENSSH |PGP )?PRIVATE KEY-----'
  'gh[pousr]_[A-Za-z0-9_]{36,}'
  'github_pat_[A-Za-z0-9_]{22,}'
  'xox[baprs]-[A-Za-z0-9-]{10,}'
  'sk_live_[A-Za-z0-9]{16,}'
  'SG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}'
  'AIza[0-9A-Za-z_-]{35}'
)

exclude_paths=(
  ':!dist/*'
  ':!webui/dist/*'
  ':!webui/node_modules/*'
)

found=0
for pattern in "${patterns[@]}"; do
  if matches="$(git grep -nI -E -- "${pattern}" -- . "${exclude_paths[@]}")"; then
    echo "secret-scan: working tree matched pattern: ${pattern}" >&2
    echo "${matches}" >&2
    found=1
  fi

  if history_matches="$(git log --all --regexp-ignore-case -G "${pattern}" --format='%h %ad %s' --date=short -- . "${exclude_paths[@]}")"; then
    if [[ -n "${history_matches}" ]]; then
      echo "secret-scan: git history matched pattern: ${pattern}" >&2
      echo "${history_matches}" >&2
      found=1
    fi
  fi
done

if [[ "${found}" -ne 0 ]]; then
  exit 1
fi

echo "secret-scan: no high-confidence secret patterns found in working tree or git history"
