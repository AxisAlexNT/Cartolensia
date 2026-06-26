# Building

## Development Build

```bash
GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...
npm --prefix webui ci
npm --prefix webui run build
```

## Offline Release Build

Use the release wrapper:

```bash
bash scripts/release/build-linux.sh
```

For a local machine build that is intended to bundle everything you already downloaded or installed locally, use:

```bash
cp config/local-full-build.env.example config/local-full-build.env
$EDITOR config/local-full-build.env
bash scripts/release/build-local-full.sh
```

For a private self-contained `tar.zst` bundle built on an Internet-connected local machine, including the reviewed BtbN FFmpeg GPL shared build, PostgreSQL binaries, AI executor environments, and reviewed model cache, use:

```bash
cp config/local-full-tarzst-build.env.example config/local-full-tarzst-build.env
$EDITOR config/local-full-tarzst-build.env
bash scripts/release/build-local-full-tarzst.sh config/local-full-tarzst-build.env
```

Run a no-network smoke package first:

```bash
bash scripts/release/smoke-local-full-tarzst.sh
```

Common release inputs are environment variables:

- `CARTOLENSIA_RELEASE_INCLUDE_TOOLS`
- `CARTOLENSIA_RELEASE_INCLUDE_POSTGRES`
- `CARTOLENSIA_RELEASE_INCLUDE_PYTHON_RUNTIME`
- `CARTOLENSIA_RELEASE_INCLUDE_MODELS`
- `CARTOLENSIA_RELEASE_INCLUDE_OFFLINE_MAPS`
- `CARTOLENSIA_RELEASE_AI_FLAVOR`
- `CARTOLENSIA_RELEASE_VERSION`

The local full-bundle config also accepts explicit paths to reviewed tool/model/runtime roots:

- `CARTOLENSIA_FFMPEG_BIN_DIR`
- `CARTOLENSIA_TESSERACT_BIN_DIR`
- `CARTOLENSIA_TESSDATA_DIR`
- `CARTOLENSIA_PG_BIN_DIR`
- `CARTOLENSIA_PG_CONFIG`
- `CARTOLENSIA_BUNDLED_PYTHON_ROOT`
- `CARTOLENSIA_MODELS_DIR`
- `CARTOLENSIA_OFFLINE_MAPS_DIR`

The tar.zst builder also accepts:

- `CARTOLENSIA_FFMPEG_BUNDLE_URL`
- `CARTOLENSIA_FFMPEG_ARCHIVE`
- `CARTOLENSIA_LOCAL_FULL_AI_FLAVORS`
- `CARTOLENSIA_PYTORCH_CPU_INDEX_URL`
- `CARTOLENSIA_PYTORCH_CUDA_INDEX_URL`
- `CARTOLENSIA_PYTORCH_INTEL_INDEX_URL`
- `CARTOLENSIA_PYTORCH_ROCM_INDEX_URL`
- `CARTOLENSIA_LOCAL_FULL_PREPARE_MODELS`
- `CARTOLENSIA_LOCAL_FULL_SKIP_PIP_INSTALL`

Build modes:

- minimal: app + WebUI + docs + configs only;
- tools: add reviewed ffmpeg / Tesseract when available;
- ai-runtime: add the Python runtime without model weights;
- full-local-ai: add reviewed model caches only when explicitly requested.

## Release Checks

Run the license and archive validation script:

```bash
bash scripts/release/check-licenses.sh
```

Validate the private tar.zst packaging path:

```bash
bash -n scripts/release/build-local-full-tarzst.sh
bash scripts/release/smoke-local-full-tarzst.sh
```

Validate the production Compose file:

```bash
docker compose -f docker-compose.production.yml config
```

## Source Snapshot

The offline packager can embed a source snapshot for redistribution review. It does not bundle source code from external repositories beyond normal package-manager metadata and generated notices.
