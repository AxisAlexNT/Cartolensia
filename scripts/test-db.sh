#!/usr/bin/env bash
set -euo pipefail

export GOCACHE="${GOCACHE:-/tmp/cartolensia-go-build}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
export CARTOLENSIA_RUN_DB_TESTS="${CARTOLENSIA_RUN_DB_TESTS:-1}"

if [ -z "${CARTOLENSIA_TEST_DATABASE_URL:-}" ]; then
  export CARTOLENSIA_TEST_DATABASE_URL="postgres://cartolensia:cartolensia_dev_password@127.0.0.1:55432/cartolensia?sslmode=disable"
fi

go test ./internal/database -run 'TestPostgresIntegrationPhase1' -count=1 -v
