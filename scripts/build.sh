#!/usr/bin/env bash
set -euo pipefail

app="sis"
module="github.com/ersinkoc/sis"
version="${VERSION:-dev}"
commit="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo none)}"
date="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
ldflags="-s -w -X ${module}/pkg/version.Version=${version} -X ${module}/pkg/version.Commit=${commit} -X ${module}/pkg/version.Date=${date}"

rm -rf dist
mkdir -p dist

targets=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
)

for target in "${targets[@]}"; do
  read -r goos goarch <<<"${target}"
  out="dist/${app}_${goos}_${goarch}"
  echo "building ${out}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" go build -trimpath -ldflags "${ldflags}" -o "${out}" ./cmd/sis
done

go run ./internal/tools/sbom -out dist/sis.spdx.json -version "${version}" -commit "${commit}" -date "${date}"

(
  cd dist
  sha256sum "${app}"_* sis.spdx.json > SHA256SUMS
)
