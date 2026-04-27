#!/usr/bin/env bash
set -euo pipefail

tag="${1:-}"
if [[ -z "${tag}" || ! "${tag}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "usage: ./scripts/release-readiness.sh vMAJOR.MINOR.PATCH[-prerelease]" >&2
  exit 1
fi

branch="$(git branch --show-current)"
if [[ "${branch}" != "${SIS_RELEASE_BRANCH:-main}" ]]; then
  echo "release-readiness: expected branch ${SIS_RELEASE_BRANCH:-main}, got ${branch}" >&2
  exit 1
fi

if [[ "${SIS_RELEASE_ALLOW_DIRTY:-0}" != "1" && -n "$(git status --porcelain)" ]]; then
  echo "release-readiness: working tree is dirty; commit or stash changes first" >&2
  exit 1
fi

git fetch --tags --quiet
if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  echo "release-readiness: local tag ${tag} already exists" >&2
  exit 1
fi
if git ls-remote --exit-code --tags origin "refs/tags/${tag}" >/dev/null 2>&1; then
  echo "release-readiness: remote tag ${tag} already exists" >&2
  exit 1
fi

if [[ -z "${RELEASE_GPG_PRIVATE_KEY_B64:-}" && -z "${RELEASE_GPG_KEY_ID:-}" ]]; then
  echo "release-readiness: warning: no release signing key configured; checksums will be unsigned" >&2
fi

WEBUI_PM="${WEBUI_PM:-npm}" WEBUI_INSTALL="${WEBUI_INSTALL:-ci}" ./scripts/check.sh
./scripts/release-signing-key-smoke.sh
VERSION="${tag}" ./scripts/build.sh
./scripts/sign-release.sh
./scripts/release-smoke.sh

echo "release-readiness: ${tag} is ready to tag and push"
