#!/usr/bin/env bash
set -euo pipefail

root="/mnt/Models/rclone"
api_base="${CARTOLENSIA_API_BASE:-http://127.0.0.1:18080}"
storage="${CARTOLENSIA_RCLONE_DRY_RUN_STORAGE:-rclone_dryrun}"
prefix="${CARTOLENSIA_RCLONE_DRY_RUN_PREFIX:-}"
max_files="${CARTOLENSIA_RCLONE_DRY_RUN_MAX_FILES:-50}"
max_bytes="${CARTOLENSIA_RCLONE_DRY_RUN_MAX_BYTES:-2147483648}"
extensions="${CARTOLENSIA_RCLONE_DRY_RUN_EXTENSIONS:-jpg,jpeg,png,gpx,mp4,mov}"

cat <<MSG
Cartolensia rclone dry-run preflight
root: ${root}
storage: ${storage}
prefix: ${prefix:-<empty>}
max_files: ${max_files}
max_bytes: ${max_bytes}
api: ${api_base}

Safety:
- This script does not scan by default.
- It refuses empty prefixes.
- It uses the /api/v1/discovery/dry-run endpoint only when explicitly enabled.
- Cartolensia must be configured with strict_read_only storage.
- Hashing, metadata enrichment, previews, and missing-file marking are not requested.
MSG

if [ "${CARTOLENSIA_ALLOW_RCLONE_DRY_RUN:-0}" != "1" ]; then
  echo "Refusing: set CARTOLENSIA_ALLOW_RCLONE_DRY_RUN=1 after reviewing the values above."
  exit 0
fi

if [ -z "${prefix}" ] || [ "${prefix}" = "/" ] || [ "${prefix}" = "." ]; then
  echo "Refusing: CARTOLENSIA_RCLONE_DRY_RUN_PREFIX must be a non-empty relative prefix." >&2
  exit 1
fi

if [ "${max_files}" -gt 50 ]; then
  echo "Refusing: max_files must be <= 50 for the default dry-run guard." >&2
  exit 1
fi

if [ "${CARTOLENSIA_EXECUTE_RCLONE_DRY_RUN:-0}" != "1" ]; then
  echo "Preflight only. Set CARTOLENSIA_EXECUTE_RCLONE_DRY_RUN=1 to call the API."
  exit 0
fi

json_extensions=$(printf '%s' "${extensions}" | awk -F, '{
  printf "["
  for (i=1; i<=NF; i++) {
    gsub(/^[ \t]+|[ \t]+$/, "", $i)
    if ($i != "") {
      if (count++ > 0) printf ","
      printf "\"%s\"", $i
    }
  }
  printf "]"
}')

curl -fsS "${api_base}/api/v1/discovery/dry-run" \
  -H "Content-Type: application/json" \
  -d "{\"storage\":\"${storage}\",\"prefixes\":[\"${prefix}\"],\"max_files\":${max_files},\"max_bytes\":${max_bytes},\"include_extensions\":${json_extensions},\"hash\":false,\"metadata\":false,\"previews\":false,\"mark_missing\":false}"
