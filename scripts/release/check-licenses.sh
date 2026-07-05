#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

find_latest_archive() {
  ls -1t dist/*.7z dist/release/*.7z 2>/dev/null | head -n 1 || true
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

  local required_files=()
  if [[ "${listing}" == *"manifest/components-manifest.json"* && "${listing}" == *"FIRST_RUN.md"* ]]; then
    required_files=(
      "README.md"
      "FIRST_RUN.md"
      "bin/cartolensia"
      "webui/dist/index.html"
      "config/production-bundle.yaml"
      "manifest/components-manifest.json"
      "manifest/build-manifest.env"
      "licenses/PROJECT-LICENSE.txt"
      "licenses/THIRD_PARTY_NOTICES.md"
      "licenses/go-modules.txt"
      "licenses/ffmpeg-version.txt"
      "components/ffmpeg-btbn/bin/ffmpeg"
      "ai-envs/cpu-avx2/venv/bin/python"
      "models/faster-whisper"
      "models/huggingface"
      "models/opencv"
    )
  else
    required_files=(
      "README-OFFLINE.md"
      "LICENSE"
      "licenses/PROJECT-LICENSE.txt"
      "licenses/THIRD_PARTY_NOTICES.md"
      "licenses/go-modules.txt"
      "components-manifest.json"
      "config/production.yaml"
      "config/production-container.yaml"
      "config/offline-airgap.yaml"
      ".env.production.example"
      "docker-compose.production.yml"
    )
  fi

  local required
  for required in "${required_files[@]}"; do
    case "${listing}" in
      *"${required}"*) ;;
      *)
      printf 'Archive %s is missing %s\n' "${archive}" "${required}" >&2
      exit 1
        ;;
    esac
  done

  if [[ "${listing}" == *"models/ollama/"* && "${listing}" != *"components/ollama/bin/ollama"* ]]; then
    printf 'Archive %s contains an Ollama model cache but no bundled Ollama runtime.\n' "${archive}" >&2
    exit 1
  fi
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
done < <(find dist \( -path '*/licenses/ffmpeg-configure.txt' -o -path '*/licenses/ffmpeg-version.txt' \) -type f 2>/dev/null)

printf 'License and package manifest checks passed.\n'
