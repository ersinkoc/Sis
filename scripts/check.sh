#!/usr/bin/env bash
set -euo pipefail

webui_pm="${WEBUI_PM:-npm}"
webui_install="${WEBUI_INSTALL:-install}"

gofmt -w $(find . -name '*.go' -not -path './dist/*' -not -path './webui/node_modules/*')
git diff --exit-code

(
  cd webui
  "${webui_pm}" "${webui_install}"
  "${webui_pm}" run build
  "${webui_pm}" run lint
)

./scripts/coverage.sh
CGO_ENABLED=0 go build -trimpath -o bin/sis ./cmd/sis
