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

Common release inputs are environment variables:

- `CARTOLENSIA_RELEASE_INCLUDE_TOOLS`
- `CARTOLENSIA_RELEASE_INCLUDE_POSTGRES`
- `CARTOLENSIA_RELEASE_INCLUDE_PYTHON_RUNTIME`
- `CARTOLENSIA_RELEASE_INCLUDE_MODELS`
- `CARTOLENSIA_RELEASE_INCLUDE_OFFLINE_MAPS`
- `CARTOLENSIA_RELEASE_AI_FLAVOR`
- `CARTOLENSIA_RELEASE_VERSION`

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

Validate the production Compose file:

```bash
docker compose -f docker-compose.production.yml config
```

## Source Snapshot

The offline packager can embed a source snapshot for redistribution review. It does not bundle source code from external repositories beyond normal package-manager metadata and generated notices.
