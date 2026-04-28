#!/usr/bin/env bash
set -euo pipefail

webui_pm="${WEBUI_PM:-npm}"
webui_install="${WEBUI_INSTALL:-install}"
go_packages="${GO_PACKAGES:-$(go list ./... | grep -v '/webui/node_modules/')}"

unformatted="$(find . -name '*.go' -not -path './dist/*' -not -path './webui/node_modules/*' -print0 | xargs -0 gofmt -l)"
if [[ -n "${unformatted}" ]]; then
  echo "gofmt required for:" >&2
  echo "${unformatted}" >&2
  exit 1
fi
git diff --exit-code
./scripts/godoc.sh
./scripts/release-candidate-check-smoke.sh
./scripts/production-validation-preflight-smoke.sh
./scripts/update-production-validation-record-smoke.sh

(
  cd webui
  "${webui_pm}" "${webui_install}"
  "${webui_pm}" run build
  "${webui_pm}" run lint
)
./scripts/webui-embed.sh
git diff --exit-code -- internal/webui/dist

./scripts/coverage.sh
go vet ${go_packages}
CGO_ENABLED=0 go build -trimpath -o bin/sis ./cmd/sis
./scripts/smoke.sh
