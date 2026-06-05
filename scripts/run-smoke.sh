#!/usr/bin/env bash
set -euo pipefail

export GOCACHE="${GOCACHE:-/tmp/cartolensia-go-build}"

go test ./...

if [ -d webui/node_modules ]; then
  npm --prefix webui run build
else
  printf 'Skipping WebUI build because webui/node_modules is not installed.\n'
  printf 'Run `make webui-install` when network/dependency access is available.\n'
fi
