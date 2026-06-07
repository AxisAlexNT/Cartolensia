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
- `licenses/python-packages.txt` when AI is bundled;
- Debian/Ubuntu copyright files for copied system packages when available;
- `source/cartolensia-source.tar.gz` when source bundling is enabled.

Before publishing a public release, review:

- ffmpeg build license and codec flags;
- Tesseract and language data licenses;
- PostgreSQL package notices;
- Python package metadata;
- model weight licenses and training-data provenance;
- CUDA/NVIDIA redistribution terms when using CUDA wheels.

The package builder records dependency facts but does not replace legal review.
