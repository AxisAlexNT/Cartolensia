#!/usr/bin/env bash
set -euo pipefail

export GOCACHE="${GOCACHE:-/tmp/cartolensia-go-build}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

echo "Running bounded worker/job stress checks"
go test ./internal/jobs ./internal/workers ./internal/catalog

if [ "${CARTOLENSIA_RUN_DB_TESTS:-0}" = "1" ] && [ -n "${CARTOLENSIA_TEST_DATABASE_URL:-}" ]; then
  echo "Running DB-backed worker/job tests"
  go test ./internal/database ./internal/workers -run 'Test.*(Job|Lease|Worker|Cancel|Retry|Heartbeat)'
else
  echo "Skipping DB-backed worker stress: set CARTOLENSIA_RUN_DB_TESTS=1 and CARTOLENSIA_TEST_DATABASE_URL to enable."
fi
