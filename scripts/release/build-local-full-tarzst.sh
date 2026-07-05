#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONFIG_FILE="${1:-${ROOT_DIR}/config/local-full-tarzst-build.env}"
if [ -f "${CONFIG_FILE}" ]; then
  # shellcheck disable=SC1090
  source "${CONFIG_FILE}"
fi

VERSION="${CARTOLENSIA_RELEASE_VERSION:-local-full-$(date -u +%Y%m%dT%H%M%SZ)}"
PACKAGE_NAME="${CARTOLENSIA_LOCAL_FULL_PACKAGE_NAME:-cartolensia-${VERSION}-linux-x86_64-full}"
DIST_DIR="$(cd "${ROOT_DIR}" && mkdir -p "${CARTOLENSIA_RELEASE_DIST_DIR:-dist/release}" && cd "${CARTOLENSIA_RELEASE_DIST_DIR:-dist/release}" && pwd)"
WORK_DIR="$(cd "${ROOT_DIR}" && mkdir -p "${CARTOLENSIA_LOCAL_FULL_WORK_DIR:-dist/local-full-work}" && cd "${CARTOLENSIA_LOCAL_FULL_WORK_DIR:-dist/local-full-work}" && pwd)"
CACHE_DIR="$(cd "${ROOT_DIR}" && mkdir -p "${CARTOLENSIA_LOCAL_FULL_CACHE_DIR:-dist/local-full-cache}" && cd "${CARTOLENSIA_LOCAL_FULL_CACHE_DIR:-dist/local-full-cache}" && pwd)"
STAGE="${WORK_DIR}/${PACKAGE_NAME}"
ARCHIVE_FORMAT="${CARTOLENSIA_LOCAL_FULL_ARCHIVE_FORMAT:-tar.zst}"
case "${ARCHIVE_FORMAT}" in
  tar.zst) ARCHIVE="${DIST_DIR}/${PACKAGE_NAME}.tar.zst" ;;
  7z) ARCHIVE="${DIST_DIR}/${PACKAGE_NAME}.7z" ;;
  *) printf '[cartolensia-full] error: unsupported CARTOLENSIA_LOCAL_FULL_ARCHIVE_FORMAT=%s\n' "${ARCHIVE_FORMAT}" >&2; exit 1 ;;
esac
CHECKSUM="${ARCHIVE}.sha256"

INCLUDE_FFMPEG="${CARTOLENSIA_LOCAL_FULL_INCLUDE_FFMPEG:-1}"
INCLUDE_TESSERACT="${CARTOLENSIA_LOCAL_FULL_INCLUDE_TESSERACT:-1}"
INCLUDE_POSTGRES="${CARTOLENSIA_LOCAL_FULL_INCLUDE_POSTGRES:-1}"
INCLUDE_AI_ENVS="${CARTOLENSIA_LOCAL_FULL_INCLUDE_AI_ENVS:-1}"
INCLUDE_MODELS="${CARTOLENSIA_LOCAL_FULL_INCLUDE_MODELS:-1}"
INCLUDE_OFFLINE_MAPS="${CARTOLENSIA_LOCAL_FULL_INCLUDE_OFFLINE_MAPS:-0}"
INCLUDE_FACENET="${CARTOLENSIA_LOCAL_FULL_INCLUDE_FACENET:-0}"
INCLUDE_MARKER="${CARTOLENSIA_LOCAL_FULL_INCLUDE_MARKER:-0}"
INCLUDE_OLLAMA="${CARTOLENSIA_LOCAL_FULL_INCLUDE_OLLAMA:-0}"
REQUIRE_ALL_AI_FLAVORS="${CARTOLENSIA_LOCAL_FULL_REQUIRE_ALL_AI_FLAVORS:-1}"
SKIP_PIP_INSTALL="${CARTOLENSIA_LOCAL_FULL_SKIP_PIP_INSTALL:-0}"
PREPARE_MODELS="${CARTOLENSIA_LOCAL_FULL_PREPARE_MODELS:-1}"
REQUIRE_MODELS="${CARTOLENSIA_LOCAL_FULL_REQUIRE_MODELS:-1}"

FFMPEG_BUNDLE_URL="${CARTOLENSIA_FFMPEG_BUNDLE_URL:-https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-gpl-shared.tar.xz}"
PYPI_INDEX_URL="${CARTOLENSIA_PYPI_INDEX_URL:-https://pypi.org/simple}"
PYTORCH_CPU_INDEX_URL="${CARTOLENSIA_PYTORCH_CPU_INDEX_URL:-https://download.pytorch.org/whl/cpu}"
PYTORCH_CUDA_INDEX_URL="${CARTOLENSIA_PYTORCH_CUDA_INDEX_URL:-https://download.pytorch.org/whl/cu128}"
PYTORCH_INTEL_INDEX_URL="${CARTOLENSIA_PYTORCH_INTEL_INDEX_URL:-${PYTORCH_CPU_INDEX_URL}}"
PYTORCH_ROCM_INDEX_URL="${CARTOLENSIA_PYTORCH_ROCM_INDEX_URL:-}"
PYTHON_BIN="${CARTOLENSIA_LOCAL_FULL_PYTHON:-python3}"
AI_FLAVORS="${CARTOLENSIA_LOCAL_FULL_AI_FLAVORS:-cpu-avx2 cpu-avx512 nvidia-cu128 intel-arc rocm-radeon}"
MODELS_DIR="${CARTOLENSIA_MODELS_DIR:-${ROOT_DIR}/.cartolensia/models}"
OLLAMA_BIN="${CARTOLENSIA_LOCAL_FULL_OLLAMA_BIN:-$(command -v ollama || true)}"
OLLAMA_MODELS_DIR="${CARTOLENSIA_LOCAL_FULL_OLLAMA_MODELS_DIR:-${MODELS_DIR}/ollama}"
OLLAMA_MODEL_NAME="${CARTOLENSIA_LOCAL_FULL_OLLAMA_MODEL:-qwen3:8b}"
OFFLINE_MAPS_DIR="${CARTOLENSIA_OFFLINE_MAPS_DIR:-}"
PG_CONFIG="${CARTOLENSIA_PG_CONFIG:-$(command -v pg_config || true)}"
TESSDATA_DIR="${CARTOLENSIA_TESSDATA_DIR:-}"
export GOCACHE="${CARTOLENSIA_LOCAL_FULL_GOCACHE:-${CACHE_DIR}/go-build}"
mkdir -p "${GOCACHE}"

note() { printf '[cartolensia-full] %s\n' "$*"; }
warn() { printf '[cartolensia-full] warning: %s\n' "$*" >&2; }
die() { printf '[cartolensia-full] error: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

require_command() {
  have "$1" || die "required command not found: $1"
}

copy_tree() {
  local src="$1"
  local dst="$2"
  [ -e "${src}" ] || return 0
  mkdir -p "${dst}"
  cp -a "${src}/." "${dst}/"
}

copy_linked_libs() {
  local binary="$1"
  local libdir="$2"
  have ldd || return 0
  mkdir -p "${libdir}"
  ldd "${binary}" 2>/dev/null | awk '
    /=> \// {print $(NF-1)}
    /^\// {print $1}
  ' | while IFS= read -r lib; do
    [ -f "${lib}" ] || continue
    cp -L "${lib}" "${libdir}/" 2>/dev/null || true
  done
}

append_component() {
  mkdir -p "${STAGE}/manifest"
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" "$5" "$6" >>"${STAGE}/manifest/components.tsv"
}

download_file() {
  local url="$1"
  local dst="$2"
  mkdir -p "$(dirname "${dst}")"
  if [ -s "${dst}" ]; then
    note "using cached $(basename "${dst}")"
    return 0
  fi
  require_command curl
  note "downloading ${url}"
  curl -fL --retry 3 --retry-delay 3 -o "${dst}.tmp" "${url}"
  mv "${dst}.tmp" "${dst}"
}

reset_stage() {
  case "${STAGE}" in
    "${WORK_DIR}"/*) rm -rf "${STAGE}" ;;
    *) die "refusing to remove unexpected stage path: ${STAGE}" ;;
  esac
  mkdir -p "${STAGE}"/{bin,config,docs,licenses,manifest,services,components,models,ai-envs,data/{cache,components,models,exports,logs,run}}
  : >"${STAGE}/manifest/components.tsv"
}

build_application() {
  note "building backend and WebUI"
  (cd "${ROOT_DIR}" && go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "${STAGE}/bin/cartolensia" ./cmd/cartolensia)
  (cd "${ROOT_DIR}" && npm --prefix webui run build)
  copy_tree "${ROOT_DIR}/webui/dist" "${STAGE}/webui/dist"
  append_component "cartolensia-backend" "Cartolensia backend binary" "app" "AGPL-3.0-or-later" "local source" "bin/cartolensia"
  append_component "cartolensia-webui" "Built WebUI assets" "app" "AGPL-3.0-or-later" "local source" "webui/dist"
}

stage_project_files() {
  note "staging configs, docs, service code, and release helpers"
  copy_tree "${ROOT_DIR}/docs" "${STAGE}/docs"
  copy_tree "${ROOT_DIR}/config" "${STAGE}/config/templates"
  copy_tree "${ROOT_DIR}/services/ai" "${STAGE}/services/ai"
  copy_tree "${ROOT_DIR}/scripts/release" "${STAGE}/scripts/release"
  copy_tree "${ROOT_DIR}/scripts/dist" "${STAGE}/scripts/dist"
  cp "${ROOT_DIR}/README.md" "${STAGE}/README.md"
  cp "${ROOT_DIR}/LICENSE" "${STAGE}/LICENSE"
  cp "${ROOT_DIR}/LICENSE" "${STAGE}/licenses/PROJECT-LICENSE.txt"
  cp "${ROOT_DIR}/AGENTS.md" "${STAGE}/docs/AGENTS.md" 2>/dev/null || true
  {
    printf '# Third-party notices\n\n'
    printf 'This private local-full package contains Cartolensia plus operator-selected runtime components.\n'
    printf 'See `manifest/components-manifest.json` for bundled component provenance and source notes.\n\n'
    printf 'Important notes:\n\n'
    printf '- Cartolensia is AGPL-3.0-or-later; see `licenses/PROJECT-LICENSE.txt`.\n'
    printf '- The bundled BtbN FFmpeg build is GPL-enabled; see `licenses/ffmpeg-version.txt` and `licenses/ffmpeg-provenance.env`.\n'
    printf '- Tesseract is Apache-2.0; language-data licensing can vary by language pack and source.\n'
    printf '- Python package licenses are retained in each virtual environment under package `.dist-info` metadata.\n'
    printf '- AI model provenance is recorded in `manifest/components-manifest.json`; model-license terms remain model-specific.\n'
    printf '- Local LLM runtimes/models, when included, are operator-selected private-local components; verify model-specific terms before redistribution.\n'
  } >"${STAGE}/licenses/THIRD_PARTY_NOTICES.md"
  if have go; then
    (cd "${ROOT_DIR}" && go list -m all) >"${STAGE}/licenses/go-modules.txt"
  else
    printf 'go command unavailable while writing notices\n' >"${STAGE}/licenses/go-modules.txt"
  fi
  cat >"${STAGE}/config/production-bundle.yaml" <<'YAML'
http:
  addr: ":18080"
  tls_addr: ":18443"
  redirect_http_to_https: true
  tls_auto_self_signed: true
  tls_hosts:
    - "127.0.0.1"
    - "localhost"
database:
  url: "postgres://cartolensia:cartolensia@127.0.0.1:15432/cartolensia?sslmode=disable"
  migrations_dir: "./internal/catalog/migrations"
cache:
  dir: "./data/cache"
  persistent_previews: false
storages:
  - name: originals
    kind: fs
    root: /originals
    mode: strict_read_only
workers:
  enabled: true
  worker_id: bundled-main
  poll_interval: 1s
  lease_duration: 30s
  heartbeat_interval: 10s
  max_concurrency: 2
auth:
  mode: local
  admin_email: admin@example.invalid
  admin_display_name: Administrator
  admin_password_env: CARTOLENSIA_ADMIN_PASSWORD
  session_ttl: 24h
  api_token_ttl: 2160h
  cookie_name: cartolensia_session
  cookie_secure: true
  csrf_header: X-CSRF-Token
YAML
  cat >"${STAGE}/config/remote-executors.example.env" <<'ENV'
# Copy to .env.local or export before starting Cartolensia.
# AI executor can run on this host or a separate machine:
CARTOLENSIA_AI_WORKER_ENDPOINT=http://127.0.0.1:19090

# Transcoding is currently an in-process Cartolensia ffmpeg session service.
# To use a separate transcode-capable node, run another Cartolensia instance
# from this bundle on that node and route/proxy transcode API requests to it.
CARTOLENSIA_TRANSCODE_NODE_ENDPOINT=http://127.0.0.1:18081
ENV
  cat >"${STAGE}/config/ai-executor.env" <<'ENV'
CARTOLENSIA_AI_MODE=real
CARTOLENSIA_AI_MODEL_DIR=./models
CARTOLENSIA_AI_DEVICE=auto
CARTOLENSIA_AI_CLASSIFIER=efficientnet_b0
CARTOLENSIA_AI_SAFETY_MODEL=Falconsai/nsfw_image_detection
CARTOLENSIA_AI_CAPTION_MODEL=Salesforce/blip-image-captioning-base
CARTOLENSIA_AI_OPENCLIP_MODEL=ViT-B-32
CARTOLENSIA_AI_WHISPER_MODEL=small
ENV
  cat >"${STAGE}/config/llm-executor.env" <<'ENV'
# Optional local LLM endpoint for Ask Cartolensia.
# Recommended air-gapped setup: import/provide a reviewed Ollama or vLLM
# runtime and model cache, then point Cartolensia at localhost or another LAN
# executor. No remote LLM APIs are used by default.
CARTOLENSIA_KNOWLEDGE_RUNNER_MODE=local_llm
CARTOLENSIA_KNOWLEDGE_LLM_PROVIDER=ollama
CARTOLENSIA_KNOWLEDGE_LLM_ENDPOINT=http://127.0.0.1:11434
CARTOLENSIA_KNOWLEDGE_LLM_MODEL=qwen3:8b
CARTOLENSIA_KNOWLEDGE_LLM_TIMEOUT_SECONDS=90
CARTOLENSIA_KNOWLEDGE_LLM_IDLE_UNLOAD_MINUTES=5
CARTOLENSIA_KNOWLEDGE_LLM_MAX_CONTEXT_ITEMS=24
ENV
}

bundle_ffmpeg() {
  [ "${INCLUDE_FFMPEG}" = "1" ] || return 0
  local archive="${CARTOLENSIA_FFMPEG_ARCHIVE:-${CACHE_DIR}/ffmpeg/$(basename "${FFMPEG_BUNDLE_URL}")}"
  if [ -z "${CARTOLENSIA_FFMPEG_ARCHIVE:-}" ]; then
    download_file "${FFMPEG_BUNDLE_URL}" "${archive}"
  fi
  require_command tar
  local extract_dir="${CACHE_DIR}/ffmpeg/extracted"
  rm -rf "${extract_dir}"
  mkdir -p "${extract_dir}"
  note "extracting FFmpeg bundle"
  tar -xJf "${archive}" -C "${extract_dir}"
  local ffmpeg_bin
  ffmpeg_bin="$(find "${extract_dir}" -type f -path '*/bin/ffmpeg' -print -quit)"
  [ -n "${ffmpeg_bin}" ] || die "downloaded FFmpeg archive did not contain bin/ffmpeg"
  local ffmpeg_root
  ffmpeg_root="$(cd "$(dirname "${ffmpeg_bin}")/.." && pwd)"
  copy_tree "${ffmpeg_root}" "${STAGE}/components/ffmpeg-btbn"
  "${STAGE}/components/ffmpeg-btbn/bin/ffmpeg" -hide_banner -version >"${STAGE}/licenses/ffmpeg-version.txt" 2>&1 || true
  if grep -q -- '--enable-nonfree' "${STAGE}/licenses/ffmpeg-version.txt"; then
    die "FFmpeg configure line contains --enable-nonfree; refusing to bundle"
  fi
  if grep -q -- '--enable-gpl' "${STAGE}/licenses/ffmpeg-version.txt"; then
    printf 'ffmpeg_license_scope=GPL-enabled tools bundle\n' >>"${STAGE}/licenses/build-manifest.env"
  fi
  printf 'source_url=%s\narchive=%s\n' "${FFMPEG_BUNDLE_URL}" "${archive}" >"${STAGE}/licenses/ffmpeg-provenance.env"
  append_component "ffmpeg-btbn-gpl-shared" "BtbN FFmpeg GPL shared Linux x86_64 build" "tool" "GPL-enabled FFmpeg build; see licenses/ffmpeg-version.txt" "${FFMPEG_BUNDLE_URL}" "components/ffmpeg-btbn"
}

bundle_tesseract() {
  [ "${INCLUDE_TESSERACT}" = "1" ] || return 0
  local tesseract_bin="${CARTOLENSIA_TESSERACT_BIN:-$(command -v tesseract || true)}"
  if [ -z "${tesseract_bin}" ] || [ ! -x "${tesseract_bin}" ]; then
    warn "tesseract not found; OCR runtime will need an offline component import"
    append_component "tesseract" "Tesseract OCR" "ocr" "not bundled" "https://github.com/tesseract-ocr/tesseract" "missing"
    return 0
  fi
  mkdir -p "${STAGE}/components/tesseract/bin"
  cp -L "${tesseract_bin}" "${STAGE}/components/tesseract/bin/tesseract"
  copy_linked_libs "${tesseract_bin}" "${STAGE}/components/tesseract/lib"
  "${tesseract_bin}" --version >"${STAGE}/licenses/tesseract-version.txt" 2>&1 || true
  if [ -z "${TESSDATA_DIR}" ]; then
    TESSDATA_DIR="$("${tesseract_bin}" --print-parameters 2>/dev/null | awk '$1=="tessdata_manager_debug_level"{print ""; exit}' || true)"
  fi
  for candidate in "${CARTOLENSIA_TESSDATA_DIR:-}" /usr/share/tesseract-ocr/*/tessdata /usr/share/tessdata; do
    if [ -d "${candidate}" ]; then
      copy_tree "${candidate}" "${STAGE}/components/tesseract/share/tessdata"
      break
    fi
  done
  append_component "tesseract" "Tesseract OCR executable and detected language data" "ocr" "Apache-2.0 Tesseract; language data licenses vary" "https://github.com/tesseract-ocr/tesseract" "components/tesseract"
}

bundle_postgres() {
  [ "${INCLUDE_POSTGRES}" = "1" ] || return 0
  if [ -z "${PG_CONFIG}" ] || [ ! -x "${PG_CONFIG}" ]; then
    warn "pg_config not found; PostgreSQL binaries were not bundled"
    append_component "postgresql" "PostgreSQL runtime" "database" "not bundled" "https://www.postgresql.org/" "missing"
    return 0
  fi
  local bindir sharedir pkglibdir
  bindir="$("${PG_CONFIG}" --bindir)"
  sharedir="$("${PG_CONFIG}" --sharedir)"
  pkglibdir="$("${PG_CONFIG}" --pkglibdir)"
  mkdir -p "${STAGE}/components/postgres/bin" "${STAGE}/components/postgres/share" "${STAGE}/components/postgres/pkglib" "${STAGE}/components/postgres/lib"
  cp -a "${bindir}/." "${STAGE}/components/postgres/bin/"
  copy_tree "${sharedir}" "${STAGE}/components/postgres/share"
  copy_tree "${pkglibdir}" "${STAGE}/components/postgres/pkglib"
  for bin in initdb pg_ctl postgres psql pg_dump pg_restore createdb createuser; do
    [ -x "${STAGE}/components/postgres/bin/${bin}" ] && copy_linked_libs "${STAGE}/components/postgres/bin/${bin}" "${STAGE}/components/postgres/lib"
  done
  "${STAGE}/components/postgres/bin/postgres" --version >"${STAGE}/licenses/postgres-version.txt" 2>&1 || true
  append_component "postgresql" "PostgreSQL server/client runtime from pg_config" "database" "PostgreSQL License" "https://www.postgresql.org/" "components/postgres"
}

torch_index_for_flavor() {
  case "$1" in
    cpu-avx2|cpu-avx512) printf '%s\n' "${PYTORCH_CPU_INDEX_URL}" ;;
    nvidia-cu128) printf '%s\n' "${PYTORCH_CUDA_INDEX_URL}" ;;
    intel-arc) printf '%s\n' "${PYTORCH_INTEL_INDEX_URL}" ;;
    rocm-radeon)
      if [ -n "${PYTORCH_ROCM_INDEX_URL}" ]; then
        printf '%s\n' "${PYTORCH_ROCM_INDEX_URL}"
      else
        printf '%s\n' "${PYTORCH_CPU_INDEX_URL}"
      fi
      ;;
    *) printf '%s\n' "${PYTORCH_CPU_INDEX_URL}" ;;
  esac
}

write_ai_requirements() {
  local flavor="$1"
  local req="${STAGE}/ai-envs/${flavor}/requirements-full.txt"
  mkdir -p "$(dirname "${req}")"
  cat >"${req}" <<'REQ'
fastapi
uvicorn
numpy
pillow
opencv-python-headless
transformers
safetensors
open-clip-torch
faster-whisper
ctranslate2
librosa
soundfile
audioread
mutagen
pydub
scipy
scikit-learn
webrtcvad
pymupdf
REQ
  if [ "${INCLUDE_FACENET}" = "1" ]; then
    cat >>"${req}" <<'REQ'
facenet-pytorch
REQ
  fi
  if [ "${INCLUDE_MARKER}" = "1" ]; then
    cat >>"${req}" <<'REQ'
marker-pdf
REQ
  fi
}

write_ai_constraints() {
  local flavor="$1"
  local env_dir="$2"
  local constraints="${STAGE}/ai-envs/${flavor}/constraints.txt"
  "${env_dir}/bin/python" - <<'PY' >"${constraints}"
from importlib import metadata
for package in ("torch", "torchvision"):
    try:
        print(f"{package}=={metadata.version(package)}")
    except metadata.PackageNotFoundError:
        pass
PY
  printf '%s\n' "${constraints}"
}

build_ai_env() {
  local flavor="$1"
  [ "${INCLUDE_AI_ENVS}" = "1" ] || return 0
  local env_dir="${STAGE}/ai-envs/${flavor}/venv"
  write_ai_requirements "${flavor}"
  if [ "${SKIP_PIP_INSTALL}" = "1" ]; then
    warn "CARTOLENSIA_LOCAL_FULL_SKIP_PIP_INSTALL=1; wrote requirements for ${flavor} but did not build venv"
    append_component "ai-env-${flavor}" "AI Python requirements for ${flavor}" "python" "not installed in dry package" "${PYPI_INDEX_URL}" "ai-envs/${flavor}/requirements-full.txt"
    return 0
  fi
  note "building AI Python environment: ${flavor}"
  "${PYTHON_BIN}" -m venv "${env_dir}"
  "${env_dir}/bin/python" -m pip install --upgrade pip setuptools wheel --index-url "${PYPI_INDEX_URL}"
  local torch_index
  torch_index="$(torch_index_for_flavor "${flavor}")"
  "${env_dir}/bin/python" -m pip install torch torchvision --index-url "${torch_index}"
  local constraints
  constraints="$(write_ai_constraints "${flavor}" "${env_dir}")"
  "${env_dir}/bin/python" -m pip install -r "${STAGE}/ai-envs/${flavor}/requirements-full.txt" --constraint "${constraints}" --index-url "${PYPI_INDEX_URL}" --extra-index-url "${torch_index}"
  "${env_dir}/bin/python" - <<'PY'
import importlib
for name in ("torch", "torchvision", "fastapi", "uvicorn", "PIL", "cv2", "transformers", "open_clip", "faster_whisper", "librosa", "fitz"):
    importlib.import_module(name)
print("ai environment import smoke passed")
PY
  append_component "ai-env-${flavor}" "AI Python environment ${flavor}" "python" "package licenses recorded by pip metadata" "${PYPI_INDEX_URL}; torch=${torch_index}" "ai-envs/${flavor}/venv"
}

bundle_ai_envs() {
  [ "${INCLUDE_AI_ENVS}" = "1" ] || return 0
  require_command "${PYTHON_BIN}"
  local failed=0
  for flavor in ${AI_FLAVORS}; do
    if ! build_ai_env "${flavor}"; then
      failed=1
      warn "failed to build AI flavor ${flavor}"
      append_component "ai-env-${flavor}" "AI Python environment ${flavor}" "python" "failed" "${PYPI_INDEX_URL}" "missing"
    fi
  done
  if [ "${failed}" = "1" ] && [ "${REQUIRE_ALL_AI_FLAVORS}" = "1" ]; then
    die "one or more requested AI flavors failed; set CARTOLENSIA_LOCAL_FULL_REQUIRE_ALL_AI_FLAVORS=0 for best-effort bundles"
  fi
}

bundle_models() {
  [ "${INCLUDE_MODELS}" = "1" ] || return 0
  if [ "${PREPARE_MODELS}" = "1" ]; then
    prepare_model_cache
  fi
  if [ ! -d "${MODELS_DIR}" ]; then
    if [ "${REQUIRE_MODELS}" = "1" ]; then
      die "model cache not found: ${MODELS_DIR}"
    fi
    warn "model cache not found: ${MODELS_DIR}; skipping model bundle"
    append_component "ai-model-cache" "AI model cache" "model" "not bundled" "docs/AI_MODEL_APPROVALS.md" "missing"
    return 0
  fi
  note "copying reviewed AI model cache from ${MODELS_DIR}"
  copy_tree "${MODELS_DIR}" "${STAGE}/models"
  append_component "ai-model-cache" "Reviewed AI model cache" "model" "varies by model; see docs/AI_MODEL_APPROVALS.md" "docs/AI_MODEL_APPROVALS.md" "models"
}

prepare_model_cache() {
  if [ "${SKIP_PIP_INSTALL}" = "1" ]; then
    warn "skipping model preparation because CARTOLENSIA_LOCAL_FULL_SKIP_PIP_INSTALL=1"
    return 0
  fi
  local first_flavor first_python
  first_flavor="$(printf '%s\n' ${AI_FLAVORS} | head -n 1)"
  first_python="${STAGE}/ai-envs/${first_flavor}/venv/bin/python"
  if [ ! -x "${first_python}" ]; then
    first_python="${PYTHON_BIN}"
  fi
  note "preparing reviewed AI model cache at ${MODELS_DIR}"
  if ! "${first_python}" "${ROOT_DIR}/scripts/release/prepare-ai-model-cache.py" --model-dir "${MODELS_DIR}" --whisper-model "${CARTOLENSIA_LOCAL_FULL_WHISPER_MODEL:-small}"; then
    if [ "${REQUIRE_MODELS}" = "1" ]; then
      die "failed to prepare model cache"
    fi
    warn "model cache preparation failed; continuing because CARTOLENSIA_LOCAL_FULL_REQUIRE_MODELS=0"
  fi
}

bundle_offline_maps() {
  [ "${INCLUDE_OFFLINE_MAPS}" = "1" ] || return 0
  if [ -z "${OFFLINE_MAPS_DIR}" ] || [ ! -d "${OFFLINE_MAPS_DIR}" ]; then
    warn "offline maps requested but CARTOLENSIA_OFFLINE_MAPS_DIR is not a directory"
    append_component "offline-maps" "Offline map tiles" "maps" "not bundled" "operator-provided PMTiles/MBTiles" "missing"
    return 0
  fi
  copy_tree "${OFFLINE_MAPS_DIR}" "${STAGE}/components/offline-maps"
  append_component "offline-maps" "Operator-provided offline map tiles" "maps" "operator-provided" "operator-provided PMTiles/MBTiles" "components/offline-maps"
}

bundle_ollama() {
  if [ "${INCLUDE_OLLAMA}" != "1" ]; then
    append_component "ollama-runtime" "Ollama local LLM runtime" "llm" "not bundled" "https://ollama.com/" "missing"
    append_component "ollama-model-${OLLAMA_MODEL_NAME}" "Ollama local LLM model ${OLLAMA_MODEL_NAME}" "llm-model" "not bundled" "ollama://${OLLAMA_MODEL_NAME}" "missing"
    return 0
  fi
  if [ -z "${OLLAMA_BIN}" ] || [ ! -x "${OLLAMA_BIN}" ]; then
    warn "CARTOLENSIA_LOCAL_FULL_INCLUDE_OLLAMA=1 but no executable Ollama binary was found"
    append_component "ollama-runtime" "Ollama local LLM runtime" "llm" "missing" "https://ollama.com/" "missing"
    append_component "ollama-model-${OLLAMA_MODEL_NAME}" "Ollama local LLM model ${OLLAMA_MODEL_NAME}" "llm-model" "missing runtime" "ollama://${OLLAMA_MODEL_NAME}" "missing"
    return 0
  fi
  mkdir -p "${STAGE}/components/ollama/bin"
  cp -L "${OLLAMA_BIN}" "${STAGE}/components/ollama/bin/ollama"
  chmod +x "${STAGE}/components/ollama/bin/ollama"
  "${STAGE}/components/ollama/bin/ollama" --version >"${STAGE}/licenses/ollama-version.txt" 2>&1 || true
  append_component "ollama-runtime" "Ollama local LLM runtime" "llm" "operator-provided local binary; verify upstream terms before redistribution" "https://ollama.com/" "components/ollama"

  if [ -d "${OLLAMA_MODELS_DIR}" ] && [ -n "$(find "${OLLAMA_MODELS_DIR}" -mindepth 1 -print -quit 2>/dev/null)" ]; then
    copy_tree "${OLLAMA_MODELS_DIR}" "${STAGE}/models/ollama"
    append_component "ollama-model-${OLLAMA_MODEL_NAME}" "Ollama local LLM model ${OLLAMA_MODEL_NAME}" "llm-model" "model-specific terms; private-local operator bundle" "ollama://${OLLAMA_MODEL_NAME}" "models/ollama"
  else
    warn "Ollama model cache is missing or empty: ${OLLAMA_MODELS_DIR}"
    append_component "ollama-model-${OLLAMA_MODEL_NAME}" "Ollama local LLM model ${OLLAMA_MODEL_NAME}" "llm-model" "missing" "ollama://${OLLAMA_MODEL_NAME}" "missing"
  fi
}

write_runtime_scripts() {
  note "writing runtime scripts"
  cat >"${STAGE}/bin/cartolensia-env" <<'SH'
#!/usr/bin/env bash
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
for env_file in "${ROOT}/.env" "${ROOT}/.env.local" "${ROOT}/config/remote-executors.local.env"; do
  if [ -f "${env_file}" ]; then
    set -a
    # shellcheck source=/dev/null
    source "${env_file}"
    set +a
  fi
done
export CARTOLENSIA_HOME="${CARTOLENSIA_HOME:-${ROOT}}"
export CARTOLENSIA_DATA_DIR="${CARTOLENSIA_DATA_DIR:-${ROOT}/data}"
export CARTOLENSIA_CONFIG="${CARTOLENSIA_CONFIG:-${ROOT}/config/production-bundle.yaml}"
export CARTOLENSIA_DATABASE_URL="${CARTOLENSIA_DATABASE_URL:-postgres://cartolensia:cartolensia@127.0.0.1:15432/cartolensia?sslmode=disable}"
export CARTOLENSIA_HTTP_ADDR="${CARTOLENSIA_HTTP_ADDR:-:18080}"
export CARTOLENSIA_HTTP_TLS_ADDR="${CARTOLENSIA_HTTP_TLS_ADDR:-:18443}"
export CARTOLENSIA_HTTP_REDIRECT_HTTP_TO_HTTPS="${CARTOLENSIA_HTTP_REDIRECT_HTTP_TO_HTTPS:-true}"
export CARTOLENSIA_HTTP_TLS_AUTO_SELF_SIGNED="${CARTOLENSIA_HTTP_TLS_AUTO_SELF_SIGNED:-true}"
export CARTOLENSIA_HTTP_TLS_HOSTS="${CARTOLENSIA_HTTP_TLS_HOSTS:-127.0.0.1,localhost}"
export CARTOLENSIA_AUTH_COOKIE_SECURE="${CARTOLENSIA_AUTH_COOKIE_SECURE:-true}"
export CARTOLENSIA_AI_WORKER_ENDPOINT="${CARTOLENSIA_AI_WORKER_ENDPOINT:-http://127.0.0.1:19090}"
export CARTOLENSIA_KNOWLEDGE_RUNNER_MODE="${CARTOLENSIA_KNOWLEDGE_RUNNER_MODE:-deterministic}"
export CARTOLENSIA_KNOWLEDGE_LLM_PROVIDER="${CARTOLENSIA_KNOWLEDGE_LLM_PROVIDER:-ollama}"
export CARTOLENSIA_KNOWLEDGE_LLM_ENDPOINT="${CARTOLENSIA_KNOWLEDGE_LLM_ENDPOINT:-http://127.0.0.1:11434}"
export CARTOLENSIA_KNOWLEDGE_LLM_MODEL="${CARTOLENSIA_KNOWLEDGE_LLM_MODEL:-qwen3:8b}"
export CARTOLENSIA_KNOWLEDGE_LLM_TIMEOUT_SECONDS="${CARTOLENSIA_KNOWLEDGE_LLM_TIMEOUT_SECONDS:-90}"
export CARTOLENSIA_KNOWLEDGE_LLM_IDLE_UNLOAD_MINUTES="${CARTOLENSIA_KNOWLEDGE_LLM_IDLE_UNLOAD_MINUTES:-5}"
export CARTOLENSIA_KNOWLEDGE_LLM_MAX_CONTEXT_ITEMS="${CARTOLENSIA_KNOWLEDGE_LLM_MAX_CONTEXT_ITEMS:-24}"
export CARTOLENSIA_LIBVA_DRIVER_NAME="${CARTOLENSIA_LIBVA_DRIVER_NAME:-radeonsi}"
export CARTOLENSIA_VDPAU_DRIVER="${CARTOLENSIA_VDPAU_DRIVER:-radeonsi}"
export CARTOLENSIA_TRANSCODE_PREFERRED_ACCELERATORS="${CARTOLENSIA_TRANSCODE_PREFERRED_ACCELERATORS:-nvidia,vaapi,cpu}"
export OLLAMA_MODELS="${OLLAMA_MODELS:-${ROOT}/models/ollama}"
export PATH="${ROOT}/components/ollama/bin:${ROOT}/components/ffmpeg-btbn/bin:${ROOT}/components/tesseract/bin:${ROOT}/components/postgres/bin:${PATH}"
export LD_LIBRARY_PATH="${ROOT}/components/ffmpeg-btbn/lib:${ROOT}/components/tesseract/lib:${ROOT}/components/postgres/lib:${ROOT}/components/postgres/pkglib:${LD_LIBRARY_PATH:-}"
export TESSDATA_PREFIX="${TESSDATA_PREFIX:-${ROOT}/components/tesseract/share/tessdata}"
export CARTOLENSIA_COMPONENT_DIR="${CARTOLENSIA_COMPONENT_DIR:-${ROOT}/components}"
export CARTOLENSIA_AI_MODEL_DIR="${CARTOLENSIA_AI_MODEL_DIR:-${ROOT}/models}"
export CARTOLENSIA_MODEL_DIR="${CARTOLENSIA_MODEL_DIR:-${CARTOLENSIA_AI_MODEL_DIR}}"
export CARTOLENSIA_POSTGRES_TUNING_OPTS="${CARTOLENSIA_POSTGRES_TUNING_OPTS:--c dynamic_shared_memory_type=mmap -c wal_compression=on -c checkpoint_timeout=15min -c checkpoint_completion_target=0.9 -c max_wal_size=8GB -c effective_io_concurrency=200 -c random_page_cost=1.1 -c maintenance_work_mem=512MB -c autovacuum_vacuum_scale_factor=0.05 -c autovacuum_analyze_scale_factor=0.02}"
mkdir -p "${CARTOLENSIA_DATA_DIR}/"{cache,components,models,exports,logs,run}
SH
  cat >"${STAGE}/bin/ensure-postgres-db" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
PGDATA="${CARTOLENSIA_PGDATA:-${CARTOLENSIA_DATA_DIR}/postgres}"
PORT="${CARTOLENSIA_POSTGRES_PORT:-15432}"
RUN_DIR="${CARTOLENSIA_DATA_DIR}/run"
LOG="${CARTOLENSIA_DATA_DIR}/logs/postgres-bootstrap.log"
if [ ! -x "${ROOT}/components/postgres/bin/pg_ctl" ]; then
  echo "Bundled PostgreSQL is not present. Configure CARTOLENSIA_DATABASE_URL for an external PostgreSQL server." >&2
  exit 1
fi
mkdir -p "${PGDATA}" "${RUN_DIR}" "$(dirname "${LOG}")"
if [ ! -d "${PGDATA}/base" ]; then
  INITDB_SHARE="${ROOT}/components/postgres/share"
  if [ -d "${INITDB_SHARE}" ]; then
    "${ROOT}/components/postgres/bin/initdb" -D "${PGDATA}" -L "${INITDB_SHARE}" --encoding=UTF8 --locale=C --auth=trust
  else
    "${ROOT}/components/postgres/bin/initdb" -D "${PGDATA}" --encoding=UTF8 --locale=C --auth=trust
  fi
fi
if ! "${ROOT}/components/postgres/bin/pg_ctl" -D "${PGDATA}" status >/dev/null 2>&1; then
  "${ROOT}/components/postgres/bin/pg_ctl" -D "${PGDATA}" -l "${LOG}" -o "-p ${PORT} -k ${RUN_DIR} ${CARTOLENSIA_POSTGRES_TUNING_OPTS}" -w start
  started_here=1
else
  started_here=0
fi
"${ROOT}/components/postgres/bin/psql" -h "${RUN_DIR}" -p "${PORT}" -d postgres -v ON_ERROR_STOP=1 <<'SQL'
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'cartolensia') THEN
    CREATE ROLE cartolensia LOGIN PASSWORD 'cartolensia';
  END IF;
END
$$;
SELECT 'CREATE DATABASE cartolensia OWNER cartolensia'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'cartolensia')\gexec
GRANT ALL PRIVILEGES ON DATABASE cartolensia TO cartolensia;
SQL
touch "${PGDATA}/.cartolensia-db-ready"
if [ "${started_here}" = "1" ] && [ "${CARTOLENSIA_KEEP_BOOTSTRAP_POSTGRES_RUNNING:-0}" != "1" ]; then
  "${ROOT}/components/postgres/bin/pg_ctl" -D "${PGDATA}" -w stop
fi
SH
  cat >"${STAGE}/bin/start-postgres" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
PGDATA="${CARTOLENSIA_PGDATA:-${CARTOLENSIA_DATA_DIR}/postgres}"
LOG="${CARTOLENSIA_DATA_DIR}/logs/postgres.log"
if [ ! -x "${ROOT}/components/postgres/bin/pg_ctl" ]; then
  echo "Bundled PostgreSQL is not present. Configure database.url for an external PostgreSQL server." >&2
  exit 1
fi
CARTOLENSIA_KEEP_BOOTSTRAP_POSTGRES_RUNNING=1 "${ROOT}/bin/ensure-postgres-db"
if "${ROOT}/components/postgres/bin/pg_ctl" -D "${PGDATA}" status >/dev/null 2>&1; then
  echo "PostgreSQL is already running for ${PGDATA}."
else
  "${ROOT}/components/postgres/bin/pg_ctl" -D "${PGDATA}" -l "${LOG}" -o "-p ${CARTOLENSIA_POSTGRES_PORT:-15432} -k ${CARTOLENSIA_DATA_DIR}/run ${CARTOLENSIA_POSTGRES_TUNING_OPTS}" start
fi
SH
  cat >"${STAGE}/bin/start-cartolensia" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
if [ -z "${CARTOLENSIA_ADMIN_PASSWORD:-}" ]; then
  echo "Set CARTOLENSIA_ADMIN_PASSWORD before starting production auth." >&2
  exit 1
fi
echo "Starting Cartolensia with ${CARTOLENSIA_CONFIG}"
nohup "${ROOT}/bin/cartolensia" -config "${CARTOLENSIA_CONFIG}" >"${CARTOLENSIA_DATA_DIR}/logs/cartolensia.log" 2>&1 &
echo "$!" >"${CARTOLENSIA_DATA_DIR}/run/cartolensia.pid"
echo "Cartolensia PID $(cat "${CARTOLENSIA_DATA_DIR}/run/cartolensia.pid")"
HTTP_PORT="${CARTOLENSIA_HTTP_ADDR##*:}"
HTTPS_PORT="${CARTOLENSIA_HTTP_TLS_ADDR##*:}"
HEALTH_URL="https://127.0.0.1:${HTTPS_PORT}/api/v1/health"
echo "Waiting for ${HEALTH_URL} ..."
for _ in $(seq 1 30); do
  if curl -kfsS "${HEALTH_URL}" >/dev/null 2>&1; then
    echo "Cartolensia is ready."
    exit 0
  fi
  sleep 1
done
echo "Cartolensia did not become healthy within 30 seconds. Check ${CARTOLENSIA_DATA_DIR}/logs/cartolensia.log" >&2
SH
  cat >"${STAGE}/bin/stop-cartolensia" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
PIDFILE="${CARTOLENSIA_DATA_DIR}/run/cartolensia.pid"
if [ -f "${PIDFILE}" ]; then
  kill "$(cat "${PIDFILE}")" 2>/dev/null || true
  rm -f "${PIDFILE}"
fi
SH
  cat >"${STAGE}/bin/status" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
HTTP_PORT="${CARTOLENSIA_HTTP_ADDR##*:}"
HTTPS_PORT="${CARTOLENSIA_HTTP_TLS_ADDR##*:}"
echo "Cartolensia root: ${ROOT}"
echo "Data dir: ${CARTOLENSIA_DATA_DIR}"
echo "HTTP redirect: http://127.0.0.1:${HTTP_PORT}"
echo "HTTPS app: https://127.0.0.1:${HTTPS_PORT}"
if [ -f "${CARTOLENSIA_DATA_DIR}/run/cartolensia.pid" ]; then
  PID="$(cat "${CARTOLENSIA_DATA_DIR}/run/cartolensia.pid")"
  if kill -0 "${PID}" 2>/dev/null; then
    echo "App process: running (pid ${PID})"
  else
    echo "App process: stale pid file (${PID})"
  fi
else
  echo "App process: no pid file"
fi
if command -v pg_ctl >/dev/null 2>&1 && [ -d "${CARTOLENSIA_DATA_DIR}/postgres" ]; then
  pg_ctl -D "${CARTOLENSIA_DATA_DIR}/postgres" status || true
fi
if command -v curl >/dev/null 2>&1; then
  HTTPS_PORT="${CARTOLENSIA_HTTP_TLS_ADDR##*:}"
  echo "Health:"
  curl -kfsS "https://127.0.0.1:${HTTPS_PORT}/api/v1/health" || true
  printf '\nReadiness:\n'
  curl -kfsS "https://127.0.0.1:${HTTPS_PORT}/api/v1/diagnostics/readiness" || true
  printf '\n'
fi
SH
  cat >"${STAGE}/bin/first-run" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
if [ ! -r /originals ]; then
  echo "/originals is not readable. Mount originals read-only at /originals before indexing." >&2
  exit 1
fi
if [ -z "${CARTOLENSIA_ADMIN_PASSWORD:-}" ]; then
  if [ -n "${CARTOLENSIA_ADMIN_PASSWORD_FILE:-}" ] && [ -r "${CARTOLENSIA_ADMIN_PASSWORD_FILE}" ]; then
    export CARTOLENSIA_ADMIN_PASSWORD="$(tr -d '\r\n' <"${CARTOLENSIA_ADMIN_PASSWORD_FILE}")"
  else
    echo "Set CARTOLENSIA_ADMIN_PASSWORD or CARTOLENSIA_ADMIN_PASSWORD_FILE before first run." >&2
    exit 1
  fi
fi
"${ROOT}/bin/start-postgres"
"${ROOT}/bin/start-cartolensia"
"${ROOT}/bin/status"
SH
  cat >"${STAGE}/bin/start-ai-executor" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
FLAVOR="${1:-cpu-avx2}"
HOST="${2:-0.0.0.0}"
PORT="${3:-19090}"
VENV="${ROOT}/ai-envs/${FLAVOR}/venv"
PY=""
if [ -n "${CARTOLENSIA_AI_PYTHON:-}" ] && [ -x "${CARTOLENSIA_AI_PYTHON}" ]; then
  PY="${CARTOLENSIA_AI_PYTHON}"
elif [ -x "${VENV}/bin/python" ]; then
  PY="${VENV}/bin/python"
elif [ -x "${ROOT}/python/bin/python3" ]; then
  PY="${ROOT}/python/bin/python3"
elif [ -x "${ROOT}/python/bin/python" ]; then
  PY="${ROOT}/python/bin/python"
elif command -v python3 >/dev/null 2>&1; then
  PY="$(command -v python3)"
elif command -v python >/dev/null 2>&1; then
  PY="$(command -v python)"
else
  echo "No Python runtime found for AI executor flavor ${FLAVOR}." >&2
  exit 1
fi
export PYTHONPATH="${ROOT}/ai/python-site:${ROOT}/services/ai:${PYTHONPATH:-}"
export CARTOLENSIA_AI_MODE="${CARTOLENSIA_AI_MODE:-real}"
export CARTOLENSIA_AI_MODEL_DIR="${CARTOLENSIA_AI_MODEL_DIR:-${ROOT}/models}"
case "${FLAVOR}" in
  nvidia*) export CARTOLENSIA_AI_DEVICE="${CARTOLENSIA_AI_DEVICE:-cuda}" ;;
  intel*) export CARTOLENSIA_AI_DEVICE="${CARTOLENSIA_AI_DEVICE:-xpu}" ;;
  rocm*) export CARTOLENSIA_AI_DEVICE="${CARTOLENSIA_AI_DEVICE:-cuda}" ;;
  *) export CARTOLENSIA_AI_DEVICE="${CARTOLENSIA_AI_DEVICE:-cpu}" ;;
esac
exec "${PY}" -m cartolensia_ai.server --host "${HOST}" --port "${PORT}"
SH
  cat >"${STAGE}/bin/start-llm-executor" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
PROVIDER="${1:-${CARTOLENSIA_KNOWLEDGE_LLM_PROVIDER:-ollama}}"
MODEL="${2:-${CARTOLENSIA_KNOWLEDGE_LLM_MODEL:-qwen3:8b}}"
case "${PROVIDER}" in
  ollama)
    OLLAMA_CMD="${ROOT}/components/ollama/bin/ollama"
    if [ ! -x "${OLLAMA_CMD}" ]; then
      OLLAMA_CMD="$(command -v ollama || true)"
    fi
    if [ -z "${OLLAMA_CMD}" ] || [ ! -x "${OLLAMA_CMD}" ]; then
      echo "Ollama is not bundled or available on PATH. Provide it offline, then rerun this command." >&2
      exit 1
    fi
    export OLLAMA_HOST="${OLLAMA_HOST:-127.0.0.1:11434}"
    export OLLAMA_MODELS="${OLLAMA_MODELS:-${ROOT}/models/ollama}"
    mkdir -p "${OLLAMA_MODELS}"
    echo "Starting Ollama on ${OLLAMA_HOST}; model=${MODEL}; models=${OLLAMA_MODELS}" >&2
    exec "${OLLAMA_CMD}" serve
    ;;
  vllm)
    PY="${CARTOLENSIA_VLLM_PYTHON:-}"
    if [ -z "${PY}" ]; then
      for candidate in "${ROOT}/ai-envs/nvidia-cu128/venv/bin/python" "${ROOT}/ai-envs/cpu-avx2/venv/bin/python" python3; do
        if command -v "${candidate}" >/dev/null 2>&1 || [ -x "${candidate}" ]; then
          PY="${candidate}"
          break
        fi
      done
    fi
    HOST="${CARTOLENSIA_VLLM_HOST:-0.0.0.0}"
    PORT="${CARTOLENSIA_VLLM_PORT:-8000}"
    echo "Starting vLLM OpenAI-compatible server on ${HOST}:${PORT}; model=${MODEL}" >&2
    exec "${PY}" -m vllm.entrypoints.openai.api_server --host "${HOST}" --port "${PORT}" --model "${MODEL}" --trust-remote-code false
    ;;
  *)
    echo "Unsupported LLM provider ${PROVIDER}; expected ollama or vllm." >&2
    exit 1
    ;;
esac
SH
  cat >"${STAGE}/bin/start-transcode-node" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
export CARTOLENSIA_CONFIG="${CARTOLENSIA_TRANSCODE_CONFIG:-${ROOT}/config/production-bundle.yaml}"
export CARTOLENSIA_HTTP_ADDR="${CARTOLENSIA_TRANSCODE_HTTP_ADDR:-:18081}"
cat >&2 <<'NOTE'
Starting a transcode-capable Cartolensia node.
Current Cartolensia live transcoding is an in-process ffmpeg session service;
route/proxy transcode API requests to this node, or run it as the main app with
shared PostgreSQL/originals/cache. A distributed transcode queue executor is not
yet a separate protocol.
NOTE
exec "${ROOT}/bin/cartolensia" -config "${CARTOLENSIA_CONFIG}"
SH
  cat >"${STAGE}/bin/backup-db" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
OUT="${1:-${CARTOLENSIA_DATA_DIR}/exports/cartolensia-$(date -u +%Y%m%dT%H%M%SZ).dump}"
mkdir -p "$(dirname "${OUT}")"
pg_dump "${CARTOLENSIA_DATABASE_URL:-postgres://cartolensia:cartolensia@127.0.0.1:15432/cartolensia?sslmode=disable}" -Fc -f "${OUT}"
echo "${OUT}"
SH
  cat >"${STAGE}/bin/diagnose" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${ROOT}/bin/cartolensia-env"
echo "Cartolensia root: ${ROOT}"
echo "Config: ${CARTOLENSIA_CONFIG}"
echo "Originals readable: $(test -r /originals && echo yes || echo no)"
echo "Cache writable: $(test -w "${CARTOLENSIA_DATA_DIR}/cache" && echo yes || echo no)"
command -v ffmpeg >/dev/null && ffmpeg -hide_banner -version | head -n 1 || echo "ffmpeg missing"
command -v tesseract >/dev/null && tesseract --version | head -n 1 || echo "tesseract missing"
command -v postgres >/dev/null && postgres --version || echo "postgres missing"
if command -v curl >/dev/null; then
  HTTP_PORT="${CARTOLENSIA_HTTP_ADDR##*:}"
  curl -fsS "http://127.0.0.1:${HTTP_PORT}/api/v1/diagnostics/readiness" || true
fi
SH
  cat >"${STAGE}/FIRST_RUN.md" <<'MD'
# Cartolensia First Run

1. Mount originals read-only at `/originals`.
2. Set a production admin password:

   ```bash
   export CARTOLENSIA_ADMIN_PASSWORD='replace-with-a-long-secret'
   ```

3. Start bundled PostgreSQL and Cartolensia:

   ```bash
   ./bin/first-run
   ```

4. Open `https://127.0.0.1:18443` and accept the self-signed certificate warning, or use `http://127.0.0.1:18080` to be redirected to HTTPS.
5. Run diagnostics:

   ```bash
   ./bin/diagnose
   ```

For a remote AI executor:

```bash
./bin/start-ai-executor nvidia-cu128 0.0.0.0 19090
```

Then set `CARTOLENSIA_AI_WORKER_ENDPOINT=http://ai-node:19090` in `.env.local` on the main node.

GPU notes: NVIDIA, AMD, and Intel GPU drivers/device passthrough are host responsibilities. The bundle contains user-space Python/package payloads and FFmpeg, not kernel drivers.

For AMD Ryzen iGPU VAAPI transcoding, make sure the runtime user can read
`/dev/dri/renderD*` and is in the `video` and `render` groups. For NVIDIA
transcoding or AI, host NVIDIA drivers must be installed; Docker GPU passthrough
requires the NVIDIA Container Toolkit on the host.
MD
  chmod +x "${STAGE}/bin/"*
}

write_manifests() {
  note "writing manifests"
  {
    printf 'package_name=%s\n' "${PACKAGE_NAME}"
    printf 'version=%s\n' "${VERSION}"
    printf 'built_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'ffmpeg_url=%s\n' "${FFMPEG_BUNDLE_URL}"
    printf 'include_ffmpeg=%s\n' "${INCLUDE_FFMPEG}"
    printf 'include_tesseract=%s\n' "${INCLUDE_TESSERACT}"
    printf 'include_postgres=%s\n' "${INCLUDE_POSTGRES}"
    printf 'include_ai_envs=%s\n' "${INCLUDE_AI_ENVS}"
    printf 'ai_flavors=%s\n' "${AI_FLAVORS}"
    printf 'include_models=%s\n' "${INCLUDE_MODELS}"
    printf 'include_facenet=%s\n' "${INCLUDE_FACENET}"
    printf 'include_marker=%s\n' "${INCLUDE_MARKER}"
    printf 'include_ollama=%s\n' "${INCLUDE_OLLAMA}"
    printf 'ollama_model=%s\n' "${OLLAMA_MODEL_NAME}"
    printf 'prepare_models=%s\n' "${PREPARE_MODELS}"
    printf 'safety_originals=package never writes to /originals; configure strict_read_only storage\n'
  } >"${STAGE}/manifest/build-manifest.env"
  if have "${PYTHON_BIN}"; then
    COMPONENTS_TSV="${STAGE}/manifest/components.tsv" COMPONENTS_JSON="${STAGE}/manifest/components-manifest.json" "${PYTHON_BIN}" - <<'PY'
import csv, json, os
rows = []
with open(os.environ["COMPONENTS_TSV"], newline="", encoding="utf-8") as fh:
    for row in csv.reader(fh, delimiter="\t"):
        if len(row) != 6:
            continue
        rows.append({
            "key": row[0],
            "name": row[1],
            "category": row[2],
            "license": row[3],
            "source": row[4],
            "path": row[5],
        })
with open(os.environ["COMPONENTS_JSON"], "w", encoding="utf-8") as out:
    json.dump({"components": rows}, out, indent=2, sort_keys=True)
    out.write("\n")
PY
  fi
}

create_archive() {
  rm -f "${ARCHIVE}" "${CHECKSUM}"
  case "${ARCHIVE_FORMAT}" in
    tar.zst)
      require_command tar
      require_command zstd
      note "creating ${ARCHIVE}"
      (cd "${WORK_DIR}" && tar --sort=name --owner=0 --group=0 --numeric-owner -I "zstd -T0 -19" -cf "${ARCHIVE}" "${PACKAGE_NAME}")
      ;;
    7z)
      require_command 7z
      note "creating ${ARCHIVE}"
      (cd "${WORK_DIR}" && 7z a -t7z -m0=lzma2 -mx=7 "${ARCHIVE}" "${PACKAGE_NAME}" >/dev/null)
      ;;
  esac
  sha256sum "${ARCHIVE}" >"${CHECKSUM}"
  note "archive: ${ARCHIVE}"
  note "checksum: ${CHECKSUM}"
}

main() {
  require_command go
  require_command npm
  reset_stage
  build_application
  stage_project_files
  bundle_ffmpeg
  bundle_tesseract
  bundle_postgres
  bundle_ai_envs
  bundle_models
  bundle_offline_maps
  bundle_ollama
  write_runtime_scripts
  write_manifests
  create_archive
}

main "$@"
