#!/usr/bin/env bash
set -euo pipefail

export GOCACHE="${GOCACHE:-/tmp/cartolensia-go-build}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

go test ./...

if [ -d webui/node_modules ]; then
  npm --prefix webui run build
else
  printf 'Skipping WebUI build because webui/node_modules is not installed.\n'
fi

addr="${CARTOLENSIA_SMOKE_ADDR:-127.0.0.1:18080}"
log_file="${TMPDIR:-/tmp}/cartolensia-smoke.log"
CARTOLENSIA_HTTP_ADDR="${addr}" go run ./cmd/cartolensia >"${log_file}" 2>&1 &
pid="$!"
trap 'kill "${pid}" >/dev/null 2>&1 || true' EXIT

for _ in $(seq 1 30); do
  if curl -fsS "http://${addr}/api/v1/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

curl -fsS "http://${addr}/api/v1/health" >/dev/null
curl -fsS -X POST "http://${addr}/api/v1/discovery/start" >/dev/null
curl -fsS -X POST "http://${addr}/api/v1/hash/start" >/dev/null
curl -fsS "http://${addr}/api/v1/stats" | grep -q '"assets":4'
