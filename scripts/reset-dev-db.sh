#!/usr/bin/env bash
set -euo pipefail

export CARTOLENSIA_POSTGRES_PORT="${CARTOLENSIA_POSTGRES_PORT:-55432}"
docker compose -f docker-compose.yml -f docker-compose.dev.yml down
docker volume rm cartolensia_cartolensia_pgdata >/dev/null 2>&1 || true
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres
docker compose -f docker-compose.yml -f docker-compose.dev.yml ps postgres
