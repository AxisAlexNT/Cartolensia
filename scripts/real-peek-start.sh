#!/usr/bin/env bash
set -euo pipefail

ROOT="/mnt/Models/rclone"
PROJECT="cartolensia_realpeek"
PREFIX_RAW="${CARTOLENSIA_REAL_PEEK_PREFIX:-}"
MAX_FILES="${CARTOLENSIA_REAL_PEEK_MAX_FILES:-50}"
MAX_BYTES="${CARTOLENSIA_REAL_PEEK_MAX_BYTES:-2147483648}"
EXECUTE="${CARTOLENSIA_REAL_PEEK_EXECUTE:-0}"
HASH="${CARTOLENSIA_REAL_PEEK_HASH:-1}"
METADATA="${CARTOLENSIA_REAL_PEEK_METADATA:-0}"
PREVIEWS="${CARTOLENSIA_REAL_PEEK_PREVIEWS:-0}"
RUNTIME_DIR=".cartolensia/runtime"
CACHE_DIR=".cartolensia/realpeek-cache"
CONFIG_PATH="${RUNTIME_DIR}/realpeek.yaml"
LOG_PATH="${RUNTIME_DIR}/realpeek.log"
PID_PATH="${RUNTIME_DIR}/realpeek.pid"
STATUS_PATH="${RUNTIME_DIR}/REAL_PEEK_STATUS.md"

fail() {
  printf 'real-peek-start: %s\n' "$*" >&2
  exit 1
}

normalize_prefix() {
  local raw="$1"
  raw="${raw#"${raw%%[![:space:]]*}"}"
  raw="${raw%"${raw##*[![:space:]]}"}"
  case "$raw" in
    ""|"."|".."|"/"|"$ROOT"|"$ROOT/") fail "prefix must be a non-empty subpath under $ROOT" ;;
    "$ROOT"/*) raw="${raw#"$ROOT"/}" ;;
    /*) fail "prefix must be adapter-relative or safely under $ROOT" ;;
  esac
  raw="${raw#/}"
  raw="${raw%/}"
  case "$raw" in
    ""|"."|".."|../*|*/../*) fail "unsafe prefix rejected: $raw" ;;
  esac
  printf '%s\n' "$raw"
}

json_field() {
  local key="$1"
  sed -n "s/.*\"$key\":\"\\([^\"]*\\)\".*/\\1/p" | head -n 1
}

wait_job() {
  local job_id="$1"
  local status=""
  for _ in $(seq 1 180); do
    status="$(curl -fsS "http://127.0.0.1:18080/api/v1/jobs/${job_id}" | json_field status)"
    case "$status" in
      succeeded|failed|canceled|cancelled) printf '%s\n' "$status"; return 0 ;;
    esac
    sleep 1
  done
  printf '%s\n' "$status"
  return 1
}

PREFIX="$(normalize_prefix "$PREFIX_RAW")"
case "$MAX_FILES" in
  ''|*[!0-9]*) fail "CARTOLENSIA_REAL_PEEK_MAX_FILES must be an integer" ;;
esac
case "$MAX_BYTES" in
  ''|*[!0-9]*) fail "CARTOLENSIA_REAL_PEEK_MAX_BYTES must be an integer" ;;
esac
if [ "$MAX_FILES" -lt 1 ] || [ "$MAX_FILES" -gt 50 ]; then
  fail "max files must be between 1 and 50 for real-peek"
fi
if [ "$MAX_BYTES" -lt 1 ]; then
  fail "max bytes must be positive"
fi
if [ ! -e "$ROOT/$PREFIX" ]; then
  fail "approved prefix does not exist: $ROOT/$PREFIX"
fi

mkdir -p "$RUNTIME_DIR" "$CACHE_DIR"
docker compose -p "$PROJECT" -f docker-compose.yml -f docker-compose.dev.yml up -d postgres
for _ in $(seq 1 80); do
  if [ "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "${PROJECT}-postgres-1" 2>/dev/null || true)" = "healthy" ]; then
    break
  fi
  sleep 0.5
done
if [ "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "${PROJECT}-postgres-1" 2>/dev/null || true)" != "healthy" ]; then
  fail "temporary PostgreSQL did not become healthy"
fi
for _ in $(seq 1 20); do
  if docker compose -p "$PROJECT" -f docker-compose.yml -f docker-compose.dev.yml exec -T postgres \
    pg_isready -U cartolensia -d cartolensia >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
docker compose -p "$PROJECT" -f docker-compose.yml -f docker-compose.dev.yml exec -T postgres \
  pg_isready -U cartolensia -d cartolensia >/dev/null

cat > "$CONFIG_PATH" <<YAML
http:
  addr: "127.0.0.1:18080"
database:
  url: "postgres://cartolensia:cartolensia_dev_password@127.0.0.1:55432/cartolensia?sslmode=disable"
cache:
  dir: "$CACHE_DIR"
auth:
  mode: "dev_no_auth"
workers:
  enabled: true
  worker_id: "realpeek-local"
  poll_interval: "500ms"
  lease_duration: "30s"
  heartbeat_interval: "5s"
  max_concurrency: 2
storages:
  - name: "rclone_peek"
    kind: "fs"
    root: "$ROOT"
    mode: "strict_read_only"
YAML

if [ -f "$PID_PATH" ] && kill -0 "$(cat "$PID_PATH")" 2>/dev/null; then
  fail "Cartolensia already appears to be running with pid $(cat "$PID_PATH")"
fi
if command -v setsid >/dev/null 2>&1; then
  nohup setsid go run ./cmd/cartolensia -config "$CONFIG_PATH" > "$LOG_PATH" 2>&1 &
else
  nohup go run ./cmd/cartolensia -config "$CONFIG_PATH" > "$LOG_PATH" 2>&1 &
fi
printf '%s\n' "$!" > "$PID_PATH"

for _ in $(seq 1 80); do
  if curl -fsS http://127.0.0.1:18080/api/v1/health >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
curl -fsS http://127.0.0.1:18080/api/v1/health >/dev/null

SCAN_JOB_JSON=""
SCAN_JOB_ID=""
SCAN_JOB_STATUS=""
HASH_JOB_JSON=""
HASH_JOB_ID=""
HASH_JOB_STATUS=""
if [ "$EXECUTE" = "1" ]; then
  SCAN_JOB_JSON="$(curl -fsS -X POST http://127.0.0.1:18080/api/v1/discovery/start \
    -H 'Content-Type: application/json' \
    -d "{\"storage\":\"rclone_peek\",\"prefixes\":[\"$PREFIX\"],\"max_files\":$MAX_FILES,\"max_bytes\":$MAX_BYTES,\"mark_missing\":false,\"hash\":true,\"metadata\":$([ "$METADATA" = "1" ] && printf true || printf false),\"previews\":$([ "$PREVIEWS" = "1" ] && printf true || printf false)}")"
  SCAN_JOB_ID="$(printf '%s\n' "$SCAN_JOB_JSON" | json_field id)"
  SCAN_JOB_STATUS="$(wait_job "$SCAN_JOB_ID")"
  if [ "$SCAN_JOB_STATUS" = "succeeded" ] && [ "$HASH" = "1" ]; then
    HASH_JOB_JSON="$(curl -fsS -X POST http://127.0.0.1:18080/api/v1/hash/start \
      -H 'Content-Type: application/json' \
      -d "{\"scope\":\"prefix\",\"storage\":\"rclone_peek\",\"prefixes\":[\"$PREFIX\"],\"max_files\":$MAX_FILES}")"
    HASH_JOB_ID="$(printf '%s\n' "$HASH_JOB_JSON" | json_field id)"
    HASH_JOB_STATUS="$(wait_job "$HASH_JOB_ID")"
  fi
fi

cat > "$STATUS_PATH" <<STATUS
# Real Peek Status

- App URL: http://127.0.0.1:18080
- Config: \`$CONFIG_PATH\`
- Log: \`$LOG_PATH\`
- PID file: \`$PID_PATH\`
- Storage: \`rclone_peek\`
- Root: \`$ROOT\`
- Mode: \`strict_read_only\`
- Prefix: \`$PREFIX\`
- Max files: \`$MAX_FILES\`
- Max bytes: \`$MAX_BYTES\`
- Initial scan requested: \`$EXECUTE\`
- Initial scan job ID: \`$SCAN_JOB_ID\`
- Initial scan status: \`$SCAN_JOB_STATUS\`
- Hash after index requested: \`$HASH\`
- Hash job ID: \`$HASH_JOB_ID\`
- Hash job status: \`$HASH_JOB_STATUS\`
- Initial scan job JSON: \`$SCAN_JOB_JSON\`
- Hash job JSON: \`$HASH_JOB_JSON\`
- Metadata requested: \`$METADATA\`
- Previews requested: \`$PREVIEWS\`
- Missing marking: \`false\`
- Real data write policy: read-only; cache/work stays under \`$CACHE_DIR\`.

## Stop and Reset

\`\`\`bash
bash scripts/real-peek-reset.sh
\`\`\`
STATUS

printf 'Cartolensia real-peek is running at http://127.0.0.1:18080\n'
printf 'Prefix: %s\n' "$PREFIX"
