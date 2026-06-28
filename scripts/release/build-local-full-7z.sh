#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

export CARTOLENSIA_LOCAL_FULL_ARCHIVE_FORMAT=7z
exec "${ROOT_DIR}/scripts/release/build-local-full-tarzst.sh" "$@"
