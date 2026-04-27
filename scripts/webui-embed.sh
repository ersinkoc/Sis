#!/usr/bin/env bash
set -euo pipefail

if [[ ! -d webui/dist ]]; then
  echo "webui-embed: webui/dist not found; run the WebUI build first" >&2
  exit 1
fi

rm -rf internal/webui/dist
mkdir -p internal/webui/dist
cp -R webui/dist/. internal/webui/dist/

echo "webui-embed: embedded WebUI assets updated"
