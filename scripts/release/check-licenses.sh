#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

find_latest_archive() {
  ls -1t dist/*.7z 2>/dev/null | head -n 1 || true
}

require_file() {
  local path="$1"
  if [ ! -f "${path}" ]; then
    printf 'Missing required file: %s\n' "${path}" >&2
    exit 1
  fi
}

check_archive() {
  local archive="$1"
  command -v 7z >/dev/null 2>&1 || {
    printf '7z is not installed; skipping archive inspection.\n' >&2
    return 0
  }
  local listing
  listing="$(7z l "${archive}")"
  for required in \
    "README-OFFLINE.md" \
    "LICENSE" \
    "licenses/PROJECT-LICENSE.txt" \
    "licenses/THIRD_PARTY_NOTICES.md" \
    "licenses/go-modules.txt" \
    "components-manifest.json" \
    "config/production.yaml" \
    "config/production-container.yaml" \
    "config/offline-airgap.yaml" \
    ".env.production.example" \
    "docker-compose.production.yml"; do
    case "${listing}" in
      *"${required}"*) ;;
      *)
      printf 'Archive %s is missing %s\n' "${archive}" "${required}" >&2
      exit 1
        ;;
    esac
  done
}

require_file LICENSE
require_file docs/DISTRIBUTION.md
require_file docs/SECURITY.md
require_file config/production.yaml
require_file config/production-container.yaml
require_file config/offline-airgap.yaml
require_file .env.production.example
require_file docker-compose.production.yml

if [ -d dist ]; then
  archive="$(find_latest_archive)"
  if [ -n "${archive}" ]; then
    check_archive "${archive}"
  fi
fi

while IFS= read -r configure; do
  [ -n "${configure}" ] || continue
  if grep -q -- '--enable-nonfree' "${configure}"; then
    printf 'Release output contains a nonfree ffmpeg build; the package is not redistributable as-is.\n' >&2
    exit 1
  fi
done < <(find dist -path '*/licenses/ffmpeg-configure.txt' -type f 2>/dev/null)

printf 'License and package manifest checks passed.\n'
