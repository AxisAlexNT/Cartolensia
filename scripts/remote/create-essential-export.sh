#!/usr/bin/env bash
set -euo pipefail

umask 077

EXPORT_ROOT="${CARTOLENSIA_EXPORT_ROOT:-/var/lib/cartolensia/exports}"
CONFIG_FILE="${CARTOLENSIA_CONFIG_FILE:-/opt/cartolensia/current/config/production-bundle.yaml}"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
STAGE="${EXPORT_ROOT}/essential-${TIMESTAMP}"
ARCHIVE="${EXPORT_ROOT}/cartolensia-essential-${TIMESTAMP}.7z"

mkdir -p "${STAGE}"
cleanup() {
  rm -rf "${STAGE}"
}
trap cleanup EXIT

db_url="$(sed -n '/^database:/,/^[^ ]/ s/^[[:space:]]*url:[[:space:]]*"\(.*\)"/\1/p' "${CONFIG_FILE}" | head -n1)"
if [[ -z "${db_url}" ]]; then
  echo "database url not found in ${CONFIG_FILE}" >&2
  exit 1
fi

pg_dump "${db_url}" -Fc -f "${STAGE}/cartolensia.pg_dump"

STAGE="${STAGE}" CONFIG_FILE="${CONFIG_FILE}" python3 - <<'PY'
from __future__ import annotations

from datetime import UTC, datetime
import json
import os
from pathlib import Path

stage = Path(os.environ["STAGE"])
config_path = Path(os.environ["CONFIG_FILE"])
config = config_path.read_text(encoding="utf-8")

redacted_lines: list[str] = []
for line in config.splitlines():
    stripped = line.strip()
    if stripped.startswith("url:") and "postgres://" in stripped:
        indent = line[: len(line) - len(line.lstrip())]
        redacted_lines.append(indent + 'url: "<redacted: set production database url on restore>"')
    elif "admin_password" in stripped or "password_file" in stripped:
        key = line.split(":", 1)[0]
        redacted_lines.append(key + ': "<redacted>"')
    else:
        redacted_lines.append(line)

(stage / "production-bundle.redacted.yaml").write_text("\n".join(redacted_lines) + "\n", encoding="utf-8")

storages: list[dict[str, str]] = []
in_storage = False
current: dict[str, str] | None = None
for raw in config.splitlines():
    line = raw.rstrip()
    stripped = line.strip()
    if stripped == "storages:":
        in_storage = True
        continue
    if in_storage and line and not line.startswith(" "):
        if current:
            storages.append(current)
        break
    if not in_storage:
        continue
    if stripped.startswith("- name:"):
        if current:
            storages.append(current)
        current = {"name": stripped.split(":", 1)[1].strip().strip('"')}
    elif current and ":" in stripped:
        key, value = stripped.split(":", 1)
        current[key.strip()] = value.strip().strip('"')
else:
    if current:
        storages.append(current)

manifest = {
    "created_at_utc": datetime.now(UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    "contents": [
        "cartolensia.pg_dump",
        "production-bundle.redacted.yaml",
        "storage-manifest.json",
        "restore-notes.txt",
    ],
    "excludes": [
        "original media",
        "preview/cache thumbnails",
        "component/model caches",
        "local secret files",
    ],
    "storages": storages,
    "safety": "Original media paths are represented as metadata only; this archive contains no originals and no cache thumbnails.",
}
(stage / "storage-manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
(stage / "restore-notes.txt").write_text(
    "Restore with: pg_restore --clean --if-exists --dbname <cartolensia database> cartolensia.pg_dump.\n"
    "Recreate production config from production-bundle.redacted.yaml and local secrets.\n"
    "Originals remain external read-only mounts and are not included in this archive.\n",
    encoding="utf-8",
)
PY

7z a -t7z -m0=lzma2 -mx=7 "${ARCHIVE}" "${STAGE}" >/dev/null
chmod 0600 "${ARCHIVE}"
ls -lh "${ARCHIVE}"
printf '%s\n' "${ARCHIVE}"
