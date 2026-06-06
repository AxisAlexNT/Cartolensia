#!/usr/bin/env bash
set -euo pipefail

PROJECT="cartolensia_realpeek"
PID_PATH=".cartolensia/runtime/realpeek.pid"

if [ -f "$PID_PATH" ]; then
  PID="$(cat "$PID_PATH")"
  if kill -0 "$PID" 2>/dev/null; then
    kill "$PID" || true
    for _ in $(seq 1 20); do
      if ! kill -0 "$PID" 2>/dev/null; then
        break
      fi
      sleep 0.25
    done
  fi
  rm -f "$PID_PATH"
fi

if command -v fuser >/dev/null 2>&1; then
  fuser -k 18080/tcp >/dev/null 2>&1 || true
fi

docker compose -p "$PROJECT" -f docker-compose.yml -f docker-compose.dev.yml down -v
rm -rf .cartolensia/runtime .cartolensia/realpeek-cache

printf 'Removed temporary Cartolensia real-peek app runtime, cache, and PostgreSQL volume.\n'
printf 'No command in this script writes to /mnt/Models/rclone.\n'
