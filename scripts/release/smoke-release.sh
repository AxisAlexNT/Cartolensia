#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

bash scripts/release/check-licenses.sh

if command -v docker >/dev/null 2>&1; then
  docker compose -f docker-compose.production.yml config >/dev/null
fi

if [ -d dist ]; then
  archive="$(ls -1t dist/*.7z 2>/dev/null | head -n 1 || true)"
  if [ -n "${archive}" ] && command -v 7z >/dev/null 2>&1; then
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "${tmpdir}"' EXIT
    7z x -y -o"${tmpdir}" "${archive}" >/dev/null
    if ! find "${tmpdir}" -name start-cartolensia.sh -type f | grep -q .; then
      printf 'Release archive is missing start-cartolensia.sh.\n' >&2
      exit 1
    fi
  fi
fi

printf 'Release smoke checks passed.\n'
