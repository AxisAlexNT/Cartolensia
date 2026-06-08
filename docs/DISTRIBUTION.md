# Offline Distribution

Cartolensia can build a Linux x86_64 offline archive intended for machines with no Internet access. The distribution is assembled from normal package-manager inputs and includes a license/notice bundle so redistribution decisions are auditable.

## Goals

- Include the Cartolensia Go backend binary.
- Include the built Vue WebUI.
- Include launcher scripts and default offline configs.
- Include optional local runtime tools:
  - ffmpeg and ffprobe;
  - Tesseract OCR and installed language data;
  - PostgreSQL runtime binaries for a local metadata database;
  - Python runtime and AI sidecar packages.
- Include an AGPL source snapshot.
- Produce a `.7z` archive and `.sha256` checksum.

## Non-Goals And Limits

- GPU drivers are not bundled. GPU AI can only work on hosts that already have compatible kernel/GPU drivers.
- Public map tiles, public geocoding data, and paid/proprietary codecs are not bundled automatically.
- Model weights are bundled only when explicitly requested and when a license-reviewed model cache is available.
- Elasticsearch/OpenSearch are not bundled.

## Local Build

Install normal build prerequisites on the build machine:

- Go 1.22+;
- Node.js and npm;
- Python 3.11+ if bundling AI;
- `7z`/`p7zip`;
- optional `ffmpeg`, `tesseract`, OCR language packs, and PostgreSQL packages.

Build a compact runtime/OCR-capable archive:

```bash
CARTOLENSIA_DIST_AI_FLAVOR=runtime \
CARTOLENSIA_DIST_INCLUDE_TOOLS=1 \
CARTOLENSIA_DIST_INCLUDE_POSTGRES=1 \
bash scripts/dist/build-offline-linux.sh
```

Build a CPU AI archive:

```bash
CARTOLENSIA_DIST_AI_FLAVOR=cpu \
CARTOLENSIA_BUNDLED_PYTHON_ROOT="$pythonLocation" \
bash scripts/dist/build-offline-linux.sh
```

Bundle a reviewed local model cache:

```bash
CARTOLENSIA_DIST_INCLUDE_MODELS=1 \
CARTOLENSIA_MODELS_DIR=.cartolensia/models \
bash scripts/dist/build-offline-linux.sh
```

The output appears under `dist/`:

- `cartolensia-<version>-linux-x86_64-offline.7z`
- `cartolensia-<version>-linux-x86_64-offline.7z.sha256`
- `cartolensia-<version>-linux-x86_64-offline-RELEASE_NOTES.md`

## GitHub Actions Release Build

The workflow `.github/workflows/offline-release.yml` can be started manually from GitHub Actions. It:

1. checks out source;
2. installs Go, Node.js, Python, p7zip, ffmpeg, Tesseract language packs, and PostgreSQL from the runner package manager;
3. runs Go/WebUI verification;
4. builds the offline `.7z`;
5. uploads the archive as a workflow artifact;
6. creates or updates a GitHub release and attaches the `.7z` plus checksum.

Workflow inputs:

- `tag_name`: release tag to create/update.
- `release_name`: release title.
- `prerelease`: whether to mark the release as prerelease.
- `ai_flavor`: `none`, `runtime`, `cpu`, or `cuda128`.
- `include_postgres`: include PostgreSQL binaries.
- `include_tools`: include ffmpeg/ffprobe/Tesseract.
- `include_models`: copy `.cartolensia/models` if a reviewed cache exists in the runner workspace.

Use `runtime` for an OCR-capable sidecar without heavy PyTorch model packages. Use `cpu` for a package that can run approved local AI models on CPU after weights are bundled. Use `cuda128` only after reviewing PyTorch CUDA wheel and NVIDIA component redistribution terms.

## Archive Layout

```text
cartolensia-.../
  bin/cartolensia
  webui/dist/
  config/offline-memory.yaml
  config/offline-postgres.yaml
  start-cartolensia.sh
  stop-cartolensia.sh
  external/bin/
  external/lib/
  external/postgres/
  external/share/tessdata/
  python/
  ai/python-site/
  services/ai/
  .cartolensia/models/
  media/
  runtime/
  logs/
  licenses/
  source/cartolensia-source.tar.gz
```

`start-cartolensia.sh` uses bundled PostgreSQL when present, otherwise it falls back to memory mode. It starts the AI sidecar when a bundled Python environment exists. The media directory defaults to `strict_read_only`.

## License And Redistribution Review

Every archive contains:

- `licenses/PROJECT-LICENSE.txt`;
- `licenses/THIRD_PARTY_NOTICES.md`;
- `licenses/go-modules.txt`;
- `licenses/npm-production-tree.json`;
- `components-manifest.json` and `licenses/components-manifest.json`;
- `licenses/python-packages.txt` when AI is bundled;
- Debian/Ubuntu copyright files for copied system packages when available;
- `source/cartolensia-source.tar.gz` when source bundling is enabled.

The component manifest records each staged component key, name, category, source type, package-relative path, version string, license note, provenance URL, and redistribution note. It is intended for release review and for the in-app Component Manager import flow.

The packager captures the bundled FFmpeg configure line to `licenses/ffmpeg-configure.txt` when FFmpeg is included. A configure line containing `--enable-nonfree` fails packaging by default because that build is not suitable for ordinary redistribution. `--enable-gpl` is recorded in `licenses/build-manifest.env` so release operators can mark the archive as a GPL-tools bundle. Only set `CARTOLENSIA_DIST_ALLOW_NONFREE_FFMPEG=1` for private/internal packages after a separate legal review.

Before publishing a public release, review:

- ffmpeg build license and codec flags;
- Tesseract and language data licenses;
- PostgreSQL package notices;
- Python package metadata;
- model weight licenses and training-data provenance;
- CUDA/NVIDIA redistribution terms when using CUDA wheels.

The package builder records dependency facts but does not replace legal review.

## Multimodal Component Packaging

Audio/document/ASR features are component-managed in offline packages.

- FFmpeg/FFprobe are required for rich audio/video metadata and are listed in the component manifest when bundled.
- Tesseract and tessdata language packs are required for OCR and are listed individually.
- faster-whisper, ctranslate2, librosa/scipy, Marker/PDF tooling, and any advanced video/genre models are optional payloads. Bundle them only after license/provenance review.
- ASR and document models must be stored under package-local component/model/runtime directories, never under original media roots.
- Offline packages should expose missing ASR/Marker/genre components as actionable Component Manager states if they are not bundled.
- Current ASR/audio component keys are `asr-faster-whisper`, `asr-ctranslate2`, `asr-model-small`, `asr-model-medium`, `audio-librosa`, and `audio-soundfile`.
- `faster-whisper-small` can be bundled only when model weight provenance has been reviewed and the package manifest records the source cache path under `.cartolensia/models/faster-whisper`.
- PyMuPDF is AGPL-3.0-or-later and is tracked as `document-pymupdf`; include it only when the release package and notices satisfy its terms.
- Audio feature labels are currently heuristic when no reviewed genre model is bundled; do not advertise offline packages as having a production genre classifier unless a reviewed model is included.

The current Linux package flow can include the application, WebUI, OCR/media tools, PostgreSQL runtime, Python runtime, and reviewed model cache. It does not bundle host GPU drivers, public map data, online geocoders, or unreviewed model weights.
