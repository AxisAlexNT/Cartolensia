#!/usr/bin/env bash
set -euo pipefail

root="/mnt/Models/rclone"

if [ "${CARTOLENSIA_ALLOW_RCLONE_SCAN:-0}" != "1" ]; then
  cat <<'MSG'
Refusing to scan /mnt/Models/rclone by default.
Set CARTOLENSIA_ALLOW_RCLONE_SCAN=1 to perform a read-only listing smoke check.
This script never writes, deletes, chmods, transcodes, or creates cache files under /mnt/Models/rclone.
MSG
  exit 0
fi

if [ ! -d "${root}" ]; then
  printf 'Path does not exist or is not a directory: %s\n' "${root}" >&2
  exit 1
fi

find "${root}" -maxdepth "${CARTOLENSIA_RCLONE_SCAN_DEPTH:-2}" -type f -printf '%p\t%s bytes\n' | sed -n '1,100p'
