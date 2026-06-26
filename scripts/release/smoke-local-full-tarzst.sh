#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="${TMPDIR:-/tmp}/cartolensia-local-full-smoke"
CONFIG="${TMP_DIR}/local-full-smoke.env"

rm -rf "${TMP_DIR}"
mkdir -p "${TMP_DIR}"

cat >"${CONFIG}" <<EOF
CARTOLENSIA_RELEASE_VERSION=smoke
CARTOLENSIA_LOCAL_FULL_PACKAGE_NAME=cartolensia-smoke-full
CARTOLENSIA_RELEASE_DIST_DIR=${TMP_DIR}/dist
CARTOLENSIA_LOCAL_FULL_WORK_DIR=${TMP_DIR}/work
CARTOLENSIA_LOCAL_FULL_CACHE_DIR=${TMP_DIR}/cache
CARTOLENSIA_LOCAL_FULL_INCLUDE_FFMPEG=0
CARTOLENSIA_LOCAL_FULL_INCLUDE_TESSERACT=0
CARTOLENSIA_LOCAL_FULL_INCLUDE_POSTGRES=0
CARTOLENSIA_LOCAL_FULL_INCLUDE_AI_ENVS=0
CARTOLENSIA_LOCAL_FULL_INCLUDE_MODELS=0
CARTOLENSIA_LOCAL_FULL_INCLUDE_OFFLINE_MAPS=0
EOF

bash "${ROOT_DIR}/scripts/release/build-local-full-tarzst.sh" "${CONFIG}"
test -s "${TMP_DIR}/dist/cartolensia-smoke-full.tar.zst"
test -s "${TMP_DIR}/dist/cartolensia-smoke-full.tar.zst.sha256"
echo "local full tar.zst smoke package OK: ${TMP_DIR}/dist/cartolensia-smoke-full.tar.zst"
