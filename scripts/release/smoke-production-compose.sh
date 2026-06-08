#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v docker >/dev/null 2>&1; then
  printf 'docker is not available; skipping production compose smoke.\n' >&2
  exit 0
fi

docker compose -f docker-compose.production.yml config >/dev/null

if [ "${CARTOLENSIA_SMOKE_PRODUCTION_RUN:-0}" != "1" ]; then
  printf 'Production compose config is valid.\n'
  exit 0
fi

fixture="${CARTOLENSIA_SMOKE_PRODUCTION_FIXTURE:-}"
if [ -z "${fixture}" ] || [ ! -d "${fixture}" ]; then
  printf 'CARTOLENSIA_SMOKE_PRODUCTION_FIXTURE must point to a readable fixture directory for an end-to-end smoke run.\n' >&2
  exit 1
fi

override="$(mktemp /tmp/cartolensia-production-smoke.XXXXXX.yml)"
cleanup() {
  docker compose -f docker-compose.production.yml -f "${override}" down -v >/dev/null 2>&1 || true
  rm -f "${override}"
}
trap cleanup EXIT

cat > "${override}" <<EOF
services:
  cartolensia:
    volumes:
      - type: bind
        source: ${fixture}
        target: /originals
        read_only: true
EOF

docker compose -f docker-compose.production.yml -f "${override}" up -d --build
sleep 10
docker compose -f docker-compose.production.yml -f "${override}" ps
docker compose -f docker-compose.production.yml -f "${override}" logs --no-color --tail=100

printf 'Production compose smoke completed.\n'
