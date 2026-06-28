#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

APP_NAME="cartolensia"
VERSION="${CARTOLENSIA_DIST_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || date -u +%Y%m%d%H%M%S)}"
TARGET="${CARTOLENSIA_DIST_TARGET:-linux-x86_64}"
DIST_DIR="${CARTOLENSIA_DIST_DIR:-${ROOT_DIR}/dist}"
PACKAGE_NAME="${CARTOLENSIA_DIST_PACKAGE_NAME:-cartolensia-${VERSION}-${TARGET}-offline}"
STAGE="${DIST_DIR}/${PACKAGE_NAME}"
ARCHIVE="${DIST_DIR}/${PACKAGE_NAME}.7z"
AI_FLAVOR="${CARTOLENSIA_DIST_AI_FLAVOR:-runtime}" # none, runtime, cpu, cuda128
INCLUDE_TOOLS="${CARTOLENSIA_DIST_INCLUDE_TOOLS:-1}"
INCLUDE_POSTGRES="${CARTOLENSIA_DIST_INCLUDE_POSTGRES:-1}"
INCLUDE_MODELS="${CARTOLENSIA_DIST_INCLUDE_MODELS:-0}"
INCLUDE_PYTHON_RUNTIME="${CARTOLENSIA_DIST_INCLUDE_PYTHON_RUNTIME:-1}"
INCLUDE_OFFLINE_MAPS="${CARTOLENSIA_DIST_INCLUDE_OFFLINE_MAPS:-0}"
INCLUDE_SOURCE="${CARTOLENSIA_DIST_INCLUDE_SOURCE:-1}"
ALLOW_NONFREE_FFMPEG="${CARTOLENSIA_DIST_ALLOW_NONFREE_FFMPEG:-0}"
MODELS_DIR="${CARTOLENSIA_MODELS_DIR:-${ROOT_DIR}/.cartolensia/models}"
BUNDLED_PYTHON_ROOT="${CARTOLENSIA_BUNDLED_PYTHON_ROOT:-}"
PYTHON_BIN="${CARTOLENSIA_DIST_PYTHON:-python3}"
GOOS_VALUE="${CARTOLENSIA_DIST_GOOS:-linux}"
GOARCH_VALUE="${CARTOLENSIA_DIST_GOARCH:-amd64}"
RSYNC_ARCHIVE_FLAGS=(-a --no-owner --no-group --no-perms)

prepend_path() {
  local dir="$1"
  [ -n "${dir}" ] || return 0
  [ -d "${dir}" ] || return 0
  PATH="${dir}:${PATH}"
}

prepend_path "${CARTOLENSIA_FFMPEG_BIN_DIR:-}"
prepend_path "${CARTOLENSIA_TESSERACT_BIN_DIR:-}"
prepend_path "${CARTOLENSIA_PG_BIN_DIR:-}"
export PATH
if [ -n "${CARTOLENSIA_TESSDATA_DIR:-}" ] && [ -d "${CARTOLENSIA_TESSDATA_DIR}" ]; then
  export TESSDATA_PREFIX="${CARTOLENSIA_TESSDATA_DIR}"
fi

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Required tool missing: %s\n' "$1" >&2
    exit 1
  fi
}

note() {
  printf '[dist] %s\n' "$*"
}

json_escape() {
  local value="${1:-}"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/}"
  printf '%s' "${value}"
}

copy_file() {
  local src="$1"
  local dst="$2"
  if [ -e "${src}" ]; then
    mkdir -p "$(dirname "${dst}")"
    cp -a "${src}" "${dst}"
  fi
}

stage_generated_note() {
  local path="$1"
  local message="$2"
  mkdir -p "$(dirname "${path}")"
  printf '%s\n' "${message}" > "${path}"
}

copy_tree() {
  local src="$1"
  local dst="$2"
  if [ -d "${src}" ]; then
    mkdir -p "${dst}"
    rsync "${RSYNC_ARCHIVE_FLAGS[@]}" --delete "${src}/" "${dst}/"
  fi
}

copy_elf_libs() {
  local binary="$1"
  local lib_dst="$2"
  [ -x "${binary}" ] || return 0
  mkdir -p "${lib_dst}"
  ldd "${binary}" 2>/dev/null | awk '
    /=> \// { print $3 }
    /^[[:space:]]*\// { print $1 }
  ' | while read -r lib; do
    [ -n "${lib}" ] || continue
    [ -e "${lib}" ] || continue
    cp -L -n "${lib}" "${lib_dst}/" 2>/dev/null || true
  done
}

copy_binary_with_libs() {
  local binary="$1"
  local dst_dir="$2"
  local lib_dst="$3"
  local resolved
  resolved="$(command -v "${binary}" 2>/dev/null || true)"
  if [ -z "${resolved}" ]; then
    note "optional binary not found: ${binary}"
    return 1
  fi
  mkdir -p "${dst_dir}"
  cp -L "${resolved}" "${dst_dir}/"
  copy_elf_libs "${resolved}" "${lib_dst}"
  record_dpkg_license_for_path "${resolved}"
}

record_dpkg_license_for_path() {
  local path="$1"
  local pkg=""
  if command -v dpkg-query >/dev/null 2>&1; then
    pkg="$(dpkg-query -S "${path}" 2>/dev/null | head -n 1 | cut -d: -f1 || true)"
  fi
  if [ -n "${pkg}" ] && [ -f "/usr/share/doc/${pkg}/copyright" ]; then
    mkdir -p "${STAGE}/licenses/debian"
    cp -L "/usr/share/doc/${pkg}/copyright" "${STAGE}/licenses/debian/${pkg}.copyright" || true
    printf '%s\t%s\n' "${pkg}" "${path}" >> "${STAGE}/licenses/debian-packages.tsv"
  fi
}

write_launcher_scripts() {
  mkdir -p "${STAGE}/config" "${STAGE}/runtime" "${STAGE}/logs" "${STAGE}/media" "${STAGE}/.cartolensia/cache" "${STAGE}/.cartolensia/models"
  cat > "${STAGE}/config/offline-memory.yaml" <<'YAML'
http:
  addr: "127.0.0.1:18080"
cache:
  dir: ".cartolensia/cache"
  persistent_previews: false
storages:
  - name: "offline_media"
    kind: "fs"
    root: "./media"
    mode: "strict_read_only"
workers:
  enabled: true
auth:
  mode: "dev_no_auth"
YAML
  cat > "${STAGE}/config/offline-postgres.yaml" <<'YAML'
http:
  addr: "127.0.0.1:18080"
database:
  url: "postgres://cartolensia@127.0.0.1:55433/cartolensia?sslmode=disable"
cache:
  dir: ".cartolensia/cache"
  persistent_previews: false
storages:
  - name: "offline_media"
    kind: "fs"
    root: "./media"
    mode: "strict_read_only"
workers:
  enabled: true
auth:
  mode: "dev_no_auth"
YAML
  cat > "${STAGE}/start-cartolensia.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${ROOT}"

export PATH="${ROOT}/external/bin:${ROOT}/external/postgres/bin:${PATH}"
export LD_LIBRARY_PATH="${ROOT}/external/lib:${ROOT}/external/postgres/lib:${ROOT}/python/lib:${LD_LIBRARY_PATH:-}"
export TESSDATA_PREFIX="${ROOT}/external/share/tessdata"
export CARTOLENSIA_AI_MODEL_DIR="${ROOT}/.cartolensia/models"

mkdir -p runtime logs .cartolensia/cache .cartolensia/models media

cleanup() {
  if [ -f runtime/cartolensia.pid ]; then
    kill "$(cat runtime/cartolensia.pid)" >/dev/null 2>&1 || true
    rm -f runtime/cartolensia.pid
  fi
  if [ -f runtime/ai.pid ]; then
    kill "$(cat runtime/ai.pid)" >/dev/null 2>&1 || true
    rm -f runtime/ai.pid
  fi
  if [ -x external/postgres/bin/pg_ctl ] && [ -d runtime/postgres ]; then
    external/postgres/bin/pg_ctl -D runtime/postgres -m fast stop >/dev/null 2>&1 || true
  fi
}
trap cleanup INT TERM

CONFIG="config/offline-memory.yaml"
if [ "${CARTOLENSIA_USE_BUNDLED_POSTGRES:-auto}" != "0" ] && [ -x external/postgres/bin/postgres ] && [ -x external/postgres/bin/initdb ]; then
  if [ ! -s runtime/postgres/PG_VERSION ]; then
    external/postgres/bin/initdb -D runtime/postgres -U cartolensia --auth-local=trust --auth-host=trust > logs/postgres-init.log 2>&1
  fi
  external/postgres/bin/pg_ctl -D runtime/postgres -l logs/postgres.log -o "-h 127.0.0.1 -p 55433" start
  external/postgres/bin/createdb -h 127.0.0.1 -p 55433 -U cartolensia cartolensia >/dev/null 2>&1 || true
  CONFIG="config/offline-postgres.yaml"
fi

if [ "${CARTOLENSIA_START_AI:-1}" != "0" ]; then
  if [ -x python/bin/python3 ] && [ -d ai/python-site ]; then
    PYTHONPATH="${ROOT}/ai/python-site:${ROOT}/services/ai" \
      python/bin/python3 -m cartolensia_ai.server --host 127.0.0.1 --port 19090 > logs/ai.log 2>&1 &
    echo "$!" > runtime/ai.pid
  elif [ -x python/bin/python ] && [ -d ai/python-site ]; then
    PYTHONPATH="${ROOT}/ai/python-site:${ROOT}/services/ai" \
      python/bin/python -m cartolensia_ai.server --host 127.0.0.1 --port 19090 > logs/ai.log 2>&1 &
    echo "$!" > runtime/ai.pid
  fi
fi

echo "Cartolensia starting on http://127.0.0.1:18080"
echo "Put read-only media under: ${ROOT}/media"
bin/cartolensia -config "${CONFIG}" > logs/cartolensia.log 2>&1 &
echo "$!" > runtime/cartolensia.pid
wait "$(cat runtime/cartolensia.pid)"
SH
  cat > "${STAGE}/stop-cartolensia.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${ROOT}"
if [ -f runtime/cartolensia.pid ]; then
  kill "$(cat runtime/cartolensia.pid)" >/dev/null 2>&1 || true
  rm -f runtime/cartolensia.pid
fi
if [ -f runtime/ai.pid ]; then
  kill "$(cat runtime/ai.pid)" >/dev/null 2>&1 || true
  rm -f runtime/ai.pid
fi
if [ -x external/postgres/bin/pg_ctl ] && [ -d runtime/postgres ]; then
  external/postgres/bin/pg_ctl -D runtime/postgres -m fast stop >/dev/null 2>&1 || true
fi
SH
  chmod +x "${STAGE}/start-cartolensia.sh" "${STAGE}/stop-cartolensia.sh"
}

write_distribution_docs() {
  cat > "${STAGE}/README-OFFLINE.md" <<EOF
# Cartolensia Offline Distribution

Version: ${VERSION}
Target: ${TARGET}

This archive is designed to run without Internet access after extraction on a compatible Linux x86_64 host.

## Start

\`\`\`bash
./start-cartolensia.sh
\`\`\`

Then open:

- http://127.0.0.1:18080

Put media to index under \`media/\` or edit \`config/offline-*.yaml\` to point at another local path. Storage defaults to \`strict_read_only\`.

## Runtime Layout

- \`bin/cartolensia\`: Go backend binary.
- \`webui/dist\`: bundled Vue WebUI assets.
- \`config/production.yaml\`: host / VM production template targeting \`/originals\`.
- \`config/production-container.yaml\`: container production template.
- \`config/offline-airgap.yaml\`: offline air-gapped production template.
- \`.env.production.example\`: shell/env bootstrap template for container deployments.
- \`docker-compose.production.yml\`: production container orchestration example.
- \`external/bin\`: optional bundled tools such as ffmpeg, ffprobe, and tesseract.
- \`external/postgres\`: optional bundled PostgreSQL runtime.
- \`python\` and \`ai/python-site\`: optional Python runtime and AI sidecar environment.
- \`.cartolensia/models\`: optional local AI model cache copied into the archive when explicitly enabled.
- \`offline-maps\`: optional reviewed offline map bundle when included by the operator.
- \`runtime\`, \`logs\`, and \`.cartolensia/cache\`: generated runtime data. Do not place originals here.

## Offline AI

The AI sidecar starts automatically when a bundled Python runtime and \`ai/python-site\` exist. Model weights are included only when the package was built with \`CARTOLENSIA_DIST_INCLUDE_MODELS=1\` and a reviewed model cache was available.

GPU acceleration still requires compatible host kernel/GPU drivers. Those drivers cannot be legally or technically bundled as part of a normal application archive.

## Licensing

Cartolensia is AGPL-3.0-or-later. This archive includes \`source/cartolensia-source.tar.gz\` when source bundling is enabled. Third-party notices and package manifests are under \`licenses/\`.
EOF

  cat > "${STAGE}/licenses/THIRD_PARTY_NOTICES.md" <<EOF
# Third-Party Notices

This distribution is assembled from Cartolensia source plus package-manager-installed dependencies. The build records dependency manifests in this directory. Operators must review these notices before public redistribution, especially when enabling CUDA AI wheels or bundling model weights.

## Project

- Cartolensia: AGPL-3.0-or-later. See \`PROJECT-LICENSE.txt\`.

## Bundled Frontend Runtime Dependencies

- Vue: MIT.
- Bootstrap: MIT.
- Bootstrap Icons: MIT.
- hls.js: Apache-2.0.
- OpenLayers \`ol\`: BSD-2-Clause.

## Backend Go Dependencies

See \`go-modules.txt\`. Go module licenses are not inferred automatically by this script.

## Python/AI Dependencies

See \`python-packages.txt\` when the AI environment is bundled. Model weights are not included unless explicitly requested. Weight licenses/provenance must be reviewed separately from package code licenses.

## External Tools

When present, ffmpeg/ffprobe, Tesseract, PostgreSQL, and their dynamic libraries are copied from the build host package manager. Debian/Ubuntu copyright files are copied to \`licenses/debian/\` when available.

## Important Redistribution Caveats

- FFmpeg may include GPL/LGPL and codec/patent-sensitive components depending on the build-host package.
- CUDA wheels and GPU drivers can carry NVIDIA-specific redistribution terms; GPU drivers are not bundled.
- Public map/geocoding tile data is not bundled by default.
EOF
  cp LICENSE "${STAGE}/licenses/PROJECT-LICENSE.txt"
}

bundle_external_tools() {
  [ "${INCLUDE_TOOLS}" = "1" ] || return 0
  note "bundling external media/OCR tools when available"
  copy_binary_with_libs ffmpeg "${STAGE}/external/bin" "${STAGE}/external/lib" || true
  copy_binary_with_libs ffprobe "${STAGE}/external/bin" "${STAGE}/external/lib" || true
  copy_binary_with_libs tesseract "${STAGE}/external/bin" "${STAGE}/external/lib" || true
  if command -v tesseract >/dev/null 2>&1; then
    for candidate in /usr/share/tesseract-ocr/5/tessdata /usr/share/tessdata; do
      if [ -d "${candidate}" ]; then
        mkdir -p "${STAGE}/external/share/tessdata"
        rsync "${RSYNC_ARCHIVE_FLAGS[@]}" "${candidate}/" "${STAGE}/external/share/tessdata/"
        break
      fi
    done
  fi
}

ffmpeg_configure_line() {
  local ffmpeg_bin="${STAGE}/external/bin/ffmpeg"
  [ -x "${ffmpeg_bin}" ] || return 0
  LD_LIBRARY_PATH="${STAGE}/external/lib:${LD_LIBRARY_PATH:-}" "${ffmpeg_bin}" -hide_banner -version 2>/dev/null \
    | awk '/^configuration:/ { sub(/^configuration:[[:space:]]*/, ""); print; exit }'
}

validate_ffmpeg_redistribution() {
  [ "${INCLUDE_TOOLS}" = "1" ] || return 0
  local configure
  configure="$(ffmpeg_configure_line || true)"
  [ -n "${configure}" ] || return 0
  printf '%s\n' "${configure}" > "${STAGE}/licenses/ffmpeg-configure.txt"
  if printf '%s\n' "${configure}" | grep -q -- '--enable-nonfree'; then
    if [ "${ALLOW_NONFREE_FFMPEG}" != "1" ]; then
      printf 'Bundled ffmpeg was built with --enable-nonfree; refusing redistributable offline package. Set CARTOLENSIA_DIST_ALLOW_NONFREE_FFMPEG=1 only for private/internal packaging after legal review.\n' >&2
      exit 1
    fi
    printf 'ffmpeg_nonfree=true\n' >> "${STAGE}/licenses/build-manifest.env"
  fi
  if printf '%s\n' "${configure}" | grep -q -- '--enable-gpl'; then
    printf 'ffmpeg_gpl=true\n' >> "${STAGE}/licenses/build-manifest.env"
  fi
}

bundle_postgres() {
  [ "${INCLUDE_POSTGRES}" = "1" ] || return 0
  local pg_config_bin
  pg_config_bin="${CARTOLENSIA_PG_CONFIG:-$(command -v pg_config 2>/dev/null || true)}"
  if [ -z "${pg_config_bin}" ]; then
    note "pg_config not found; skipping bundled PostgreSQL"
    return 0
  fi
  local pg_bindir pg_sharedir pg_pkglibdir
  pg_bindir="$("${pg_config_bin}" --bindir)"
  pg_sharedir="$("${pg_config_bin}" --sharedir)"
  pg_pkglibdir="$("${pg_config_bin}" --pkglibdir)"
  mkdir -p "${STAGE}/external/postgres/bin" "${STAGE}/external/postgres/lib" "${STAGE}/external/postgres/share"
  for binary in postgres initdb pg_ctl createdb psql; do
    if [ -x "${pg_bindir}/${binary}" ]; then
      cp -L "${pg_bindir}/${binary}" "${STAGE}/external/postgres/bin/"
      copy_elf_libs "${pg_bindir}/${binary}" "${STAGE}/external/postgres/lib"
      record_dpkg_license_for_path "${pg_bindir}/${binary}"
    fi
  done
  if [ -d "${pg_sharedir}" ]; then
    rsync "${RSYNC_ARCHIVE_FLAGS[@]}" "${pg_sharedir}/" "${STAGE}/external/postgres/share/"
  fi
  if [ -d "${pg_pkglibdir}" ]; then
    rsync "${RSYNC_ARCHIVE_FLAGS[@]}" "${pg_pkglibdir}/" "${STAGE}/external/postgres/lib/"
  fi
}

bundle_python_ai() {
  if [ "${INCLUDE_PYTHON_RUNTIME}" != "1" ]; then
    note "Python runtime bundle disabled; skipping AI sidecar environment"
    return 0
  fi
  if [ "${AI_FLAVOR}" = "none" ]; then
    note "AI flavor none; skipping Python sidecar environment"
    return 0
  fi
  mkdir -p "${STAGE}/ai/python-site" "${STAGE}/services/ai"
  rsync "${RSYNC_ARCHIVE_FLAGS[@]}" services/ai/ "${STAGE}/services/ai/"

  if [ -n "${BUNDLED_PYTHON_ROOT}" ]; then
    note "copying Python runtime from ${BUNDLED_PYTHON_ROOT}"
    mkdir -p "${STAGE}/python"
    rsync "${RSYNC_ARCHIVE_FLAGS[@]}" --delete "${BUNDLED_PYTHON_ROOT}/" "${STAGE}/python/"
    PYTHON_BIN="${STAGE}/python/bin/python3"
    if [ ! -x "${PYTHON_BIN}" ] && [ -x "${STAGE}/python/bin/python" ]; then
      PYTHON_BIN="${STAGE}/python/bin/python"
    fi
  fi

  if ! "${PYTHON_BIN}" -c 'import sys; print(sys.version)' >/dev/null; then
    printf 'Python runtime is not usable: %s\n' "${PYTHON_BIN}" >&2
    exit 1
  fi

  note "installing AI Python dependencies (${AI_FLAVOR})"
  local req_file
  req_file="$(mktemp)"
  {
    printf '%s\n' fastapi uvicorn numpy pillow
    case "${AI_FLAVOR}" in
      runtime)
        ;;
      cpu)
        printf '%s\n' torch torchvision opencv-python-headless transformers safetensors open-clip-torch facenet-pytorch
        ;;
      cuda128)
        printf '%s\n' opencv-python-headless transformers safetensors open-clip-torch facenet-pytorch
        ;;
      *)
        printf 'Unsupported CARTOLENSIA_DIST_AI_FLAVOR=%s\n' "${AI_FLAVOR}" >&2
        exit 1
        ;;
    esac
  } > "${req_file}"
  "${PYTHON_BIN}" -m pip install --break-system-packages --upgrade pip
  if [ "${AI_FLAVOR}" = "cuda128" ]; then
    "${PYTHON_BIN}" -m pip install --break-system-packages --target "${STAGE}/ai/python-site" torch torchvision --index-url https://download.pytorch.org/whl/cu128
  fi
  "${PYTHON_BIN}" -m pip install --break-system-packages --target "${STAGE}/ai/python-site" -r "${req_file}"
  rm -f "${req_file}"
  "${PYTHON_BIN}" -m pip freeze > "${STAGE}/licenses/python-packages.txt" || true
}

bundle_models() {
  if [ "${INCLUDE_MODELS}" != "1" ]; then
    cat > "${STAGE}/.cartolensia/models/README.txt" <<'EOF'
Model weights were not bundled in this archive. To create a fully offline AI package,
build with CARTOLENSIA_DIST_INCLUDE_MODELS=1 and CARTOLENSIA_MODELS_DIR pointing at
a license-reviewed model cache.
EOF
    return 0
  fi
  if [ ! -d "${MODELS_DIR}" ]; then
    printf 'Requested model bundling but model directory does not exist: %s\n' "${MODELS_DIR}" >&2
    exit 1
  fi
  note "copying reviewed model cache from ${MODELS_DIR}"
  rsync "${RSYNC_ARCHIVE_FLAGS[@]}" --delete "${MODELS_DIR}/" "${STAGE}/.cartolensia/models/"
}

component_version() {
  local binary="$1"
  [ -x "${binary}" ] || return 0
  LD_LIBRARY_PATH="${STAGE}/external/lib:${LD_LIBRARY_PATH:-}" "${binary}" --version 2>/dev/null | head -n 1 \
    || LD_LIBRARY_PATH="${STAGE}/external/lib:${LD_LIBRARY_PATH:-}" "${binary}" -version 2>/dev/null | head -n 1 \
    || true
}

write_components_manifest() {
  note "writing component manifest"
  local manifest="${STAGE}/components-manifest.json"
  local first=1
  {
    printf '{\n'
    printf '  "package_name": "%s",\n' "$(json_escape "${PACKAGE_NAME}")"
    printf '  "version": "%s",\n' "$(json_escape "${VERSION}")"
    printf '  "target": "%s",\n' "$(json_escape "${TARGET}")"
    printf '  "generated_at_utc": "%s",\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf '  "policy": "All bundled components are staged inside the offline package. Original media storage is never used as a component source or destination.",\n'
    printf '  "components": [\n'
  } > "${manifest}"

  append_component() {
    local key="$1" name="$2" category="$3" source_type="$4" path="$5" license="$6" provenance="$7" version="$8" note="${9:-}"
    [ -e "${STAGE}/${path}" ] || return 0
    if [ "${first}" = "0" ]; then
      printf ',\n' >> "${manifest}"
    fi
    first=0
    {
      printf '    {\n'
      printf '      "key": "%s",\n' "$(json_escape "${key}")"
      printf '      "name": "%s",\n' "$(json_escape "${name}")"
      printf '      "category": "%s",\n' "$(json_escape "${category}")"
      printf '      "source_type": "%s",\n' "$(json_escape "${source_type}")"
      printf '      "path": "%s",\n' "$(json_escape "${path}")"
      printf '      "version": "%s",\n' "$(json_escape "${version}")"
      printf '      "license_name": "%s",\n' "$(json_escape "${license}")"
      printf '      "provenance_url": "%s",\n' "$(json_escape "${provenance}")"
      printf '      "note": "%s"\n' "$(json_escape "${note}")"
      printf '    }'
    } >> "${manifest}"
  }

  append_component "ffmpeg" "FFmpeg" "tool" "package_manager" "external/bin/ffmpeg" "GPL/LGPL depending on configure flags" "https://ffmpeg.org" "$(component_version "${STAGE}/external/bin/ffmpeg")" "See licenses/ffmpeg-configure.txt and Debian copyright files when available."
  append_component "ffprobe" "FFprobe" "tool" "package_manager" "external/bin/ffprobe" "GPL/LGPL depending on configure flags" "https://ffmpeg.org" "$(component_version "${STAGE}/external/bin/ffprobe")" "Bundled with FFmpeg package when present."
  append_component "tesseract" "Tesseract OCR" "ocr" "package_manager" "external/bin/tesseract" "Apache-2.0" "https://github.com/tesseract-ocr/tesseract" "$(component_version "${STAGE}/external/bin/tesseract")" "Language data is recorded as tessdata components when present."
  append_component "tessdata" "Tesseract language data" "ocr" "package_manager" "external/share/tessdata" "Apache-2.0" "https://github.com/tesseract-ocr/tessdata" "installed" "Includes only language files present on the build host."
  append_component "postgres" "PostgreSQL runtime" "database" "package_manager" "external/postgres/bin/postgres" "PostgreSQL License" "https://www.postgresql.org" "$(component_version "${STAGE}/external/postgres/bin/postgres")" "Optional local metadata database runtime."
  append_component "python-runtime" "Python runtime" "python" "user_provided_or_system" "python" "Python Software Foundation License plus bundled package licenses" "https://www.python.org" "bundled" "Only present when a Python runtime root was provided."
  append_component "ai-python-site" "Cartolensia AI Python site-packages" "python" "package_manager" "ai/python-site" "Package licenses vary" "https://pypi.org" "${AI_FLAVOR}" "See licenses/python-packages.txt."
  append_component "ai-model-cache" "AI model cache" "model" "user_reviewed_cache" ".cartolensia/models" "Model licenses/provenance vary" "docs/AI_MODEL_APPROVALS.md" "optional" "Included only when CARTOLENSIA_DIST_INCLUDE_MODELS=1."

  {
    printf '\n  ]\n'
    printf '}\n'
  } >> "${manifest}"
  cp "${manifest}" "${STAGE}/licenses/components-manifest.json"
}

write_source_snapshot() {
  [ "${INCLUDE_SOURCE}" = "1" ] || return 0
  note "creating AGPL source snapshot"
  mkdir -p "${STAGE}/source"
  tar \
    --exclude='./.git' \
    --exclude='./.cartolensia' \
    --exclude='./dist' \
    --exclude='./webui/node_modules' \
    --exclude='./webui/dist' \
    --exclude='./tmp' \
    --exclude='./logs' \
    -czf "${STAGE}/source/cartolensia-source.tar.gz" .
}

write_manifests() {
  note "writing dependency manifests"
  mkdir -p "${STAGE}/licenses"
  if ! go list -m all > "${STAGE}/licenses/go-modules.txt" 2> "${STAGE}/licenses/go-modules.error.log"; then
    {
      printf 'go list -m all failed in the build environment; falling back to go.mod/go.sum snapshot.\n'
      printf 'Review go-modules.error.log for details.\n\n'
      cat go.mod
      printf '\n--- go.sum ---\n'
      cat go.sum
    } > "${STAGE}/licenses/go-modules.txt"
  fi
  npm --prefix webui ls --omit=dev --json > "${STAGE}/licenses/npm-production-tree.json" 2>/dev/null || true
  {
    printf 'package_name=%s\n' "${PACKAGE_NAME}"
    printf 'version=%s\n' "${VERSION}"
    printf 'target=%s\n' "${TARGET}"
    printf 'ai_flavor=%s\n' "${AI_FLAVOR}"
    printf 'include_tools=%s\n' "${INCLUDE_TOOLS}"
    printf 'include_postgres=%s\n' "${INCLUDE_POSTGRES}"
    printf 'include_models=%s\n' "${INCLUDE_MODELS}"
    printf 'include_python_runtime=%s\n' "${INCLUDE_PYTHON_RUNTIME}"
    printf 'include_offline_maps=%s\n' "${INCLUDE_OFFLINE_MAPS}"
    printf 'built_at_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } > "${STAGE}/licenses/build-manifest.env"
}

need go
need npm
need rsync
need tar
need 7z

note "cleaning stage ${STAGE}"
rm -rf "${STAGE}" "${ARCHIVE}" "${ARCHIVE}.sha256"
mkdir -p "${STAGE}/bin" "${STAGE}/webui" "${STAGE}/licenses"

note "building WebUI"
if [ "${CARTOLENSIA_DIST_NPM_CI:-auto}" = "1" ] || [ ! -x webui/node_modules/.bin/vue-tsc ] || [ ! -x webui/node_modules/.bin/vite ]; then
  npm --prefix webui ci
else
  note "webui/node_modules exists; skipping npm ci (set CARTOLENSIA_DIST_NPM_CI=1 to force)"
fi
npm --prefix webui run build

note "building Go backend"
GOOS="${GOOS_VALUE}" GOARCH="${GOARCH_VALUE}" CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w -X github.com/AxisAlexNT/Cartolensia/internal/app.Version=${VERSION}" \
  -o "${STAGE}/bin/cartolensia" ./cmd/cartolensia

note "copying runtime assets"
copy_tree webui/dist "${STAGE}/webui/dist"
copy_tree docs "${STAGE}/docs"
copy_file README.md "${STAGE}/README.md"
copy_file LICENSE "${STAGE}/LICENSE"
copy_file docs/AI_MODEL_APPROVALS.md "${STAGE}/docs/AI_MODEL_APPROVALS.md"

write_launcher_scripts
write_distribution_docs
write_manifests
copy_file config/production.yaml "${STAGE}/config/production.yaml"
copy_file config/production-container.yaml "${STAGE}/config/production-container.yaml"
copy_file config/offline-airgap.yaml "${STAGE}/config/offline-airgap.yaml"
copy_file .env.production.example "${STAGE}/.env.production.example"
copy_file docker-compose.production.yml "${STAGE}/docker-compose.production.yml"
copy_tree scripts/release "${STAGE}/scripts/release"
bundle_external_tools
validate_ffmpeg_redistribution
bundle_postgres
bundle_python_ai
bundle_models
if [ "${INCLUDE_OFFLINE_MAPS}" = "1" ]; then
  if [ -n "${CARTOLENSIA_OFFLINE_MAPS_DIR:-}" ] && [ -d "${CARTOLENSIA_OFFLINE_MAPS_DIR}" ]; then
    note "copying offline map bundle from ${CARTOLENSIA_OFFLINE_MAPS_DIR}"
    copy_tree "${CARTOLENSIA_OFFLINE_MAPS_DIR}" "${STAGE}/offline-maps"
  else
    stage_generated_note "${STAGE}/offline-maps/README.txt" "Offline map tiles were not bundled with this archive. Provide PMTiles/MBTiles or a self-hosted tile bundle after extraction, or place the files under offline-maps/ and point Settings -> Map/Tiles at that source."
  fi
fi
write_components_manifest
write_source_snapshot

note "creating archive ${ARCHIVE}"
(cd "${DIST_DIR}" && 7z a -t7z -mx=9 "${ARCHIVE}" "${PACKAGE_NAME}" >/dev/null)
sha256sum "${ARCHIVE}" > "${ARCHIVE}.sha256"

cat > "${DIST_DIR}/${PACKAGE_NAME}-RELEASE_NOTES.md" <<EOF
# ${PACKAGE_NAME}

Offline Cartolensia distribution for ${TARGET}.

- Archive: \`${PACKAGE_NAME}.7z\`
- SHA256: \`$(cut -d' ' -f1 "${ARCHIVE}.sha256")\`
- AI flavor: \`${AI_FLAVOR}\`
- Bundled external tools: \`${INCLUDE_TOOLS}\`
- Bundled PostgreSQL: \`${INCLUDE_POSTGRES}\`
- Bundled models: \`${INCLUDE_MODELS}\`

See \`README-OFFLINE.md\` and \`licenses/THIRD_PARTY_NOTICES.md\` inside the archive.
EOF

note "done"
printf '%s\n' "${ARCHIVE}"
