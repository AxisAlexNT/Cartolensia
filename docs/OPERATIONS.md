# Operations

This runbook is for local development and fixture-only validation. It does not require any real media archive.

## Quick Start

Run the fixture smoke path:

```bash
make smoke
```

Run the backend in memory mode:

```bash
go run ./cmd/cartolensia
```

Run the WebUI build:

```bash
npm --prefix webui run build
```

## PostgreSQL Development

Start the development PostgreSQL/PostGIS container:

```bash
make dev-db
```

Run the app against PostgreSQL:

```bash
go run ./cmd/cartolensia -config config/dev-postgres.yaml
```

Enable HTTPS with an existing certificate/key:

```yaml
http:
  addr: "127.0.0.1:18443"
  tls_cert_file: "/path/to/cert.pem"
  tls_key_file: "/path/to/key.pem"
```

For private local testing only, use an in-memory self-signed certificate:

```yaml
http:
  addr: "127.0.0.1:18443"
  tls_auto_self_signed: true
  tls_hosts: ["127.0.0.1", "localhost"]
```

Run gated DB integration tests:

```bash
bash scripts/test-db.sh
```

Reset the dev DB only when you intend to discard local metadata:

```bash
bash scripts/reset-dev-db.sh
```

## Auth

The default fixture mode is:

```yaml
auth:
  mode: dev_no_auth
```

For local auth, set an admin email in config and provide the first password through `CARTOLENSIA_ADMIN_PASSWORD` or a configured ignored `admin_password_file`.

Useful endpoints:

- `GET /api/v1/auth/me`
- `GET /api/v1/auth/csrf`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/password`
- `GET/POST /api/v1/auth/tokens`

Cookie-authenticated write requests need the CSRF header from `/auth/csrf`. Bearer API tokens use scopes and do not need CSRF.

## Jobs

Job APIs:

- `GET /api/v1/jobs`
- `GET /api/v1/jobs/stats`
- `GET /api/v1/jobs/{id}`
- `GET /api/v1/jobs/{id}/logs`
- `POST /api/v1/jobs/{id}/cancel`
- `POST /api/v1/jobs/{id}/retry`

Main job starters:

- `POST /api/v1/indexing/start`
- `GET /api/v1/indexing/latest`
- `POST /api/v1/indexing/{pipeline_id}/cancel`
- `POST /api/v1/discovery/start`
- `POST /api/v1/discovery/dry-run`
- `POST /api/v1/hash/start`
- `POST /api/v1/metadata/enrich/start`
- `POST /api/v1/previews/start`
- `POST /api/v1/gps/tracks/{track_asset_id}/snap-media`

Use the indexing pipeline from the WebUI for real-peek-style work. It preserves the same bounded storage/prefix scope across discovery, hash, metadata/EXIF, previews, GPS/KML/KMZ parsing, geotagging, and map refresh.

Workers lease jobs, heartbeat while running, recover panics into failed jobs, and retry transient failures until `max_attempts`.

## Synthetic Scale Testing

Prefer a temporary root for unattended runs:

```bash
CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/generate-synthetic-fixture.sh
CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/perf-smoke.sh
```

The scripts generate dummy files only. Remove the temporary root when done:

```bash
rm -rf /tmp/cartolensia_synthetic_media
```

Run bounded worker/job stress checks:

```bash
bash scripts/worker-stress-test.sh
```

DB-backed worker stress remains gated:

```bash
CARTOLENSIA_RUN_DB_TESTS=1 CARTOLENSIA_TEST_DATABASE_URL=postgres://... bash scripts/worker-stress-test.sh
```

## Dry-Run Reports

Scoped discovery dry runs are report-only and require non-empty prefixes. Defaults are conservative: `max_files <= 50`, `max_bytes` defaults to 2 GiB, and missing marking is rejected.

Example payload for fixture/synthetic storage:

```json
{
  "storage": "fixture",
  "prefixes": ["photos"],
  "max_files": 50,
  "max_bytes": 2147483648,
  "include_extensions": ["jpg", "jpeg", "png"],
  "hash": false,
  "metadata": false,
  "previews": false,
  "mark_missing": false
}
```

For a future real archive dry run, start from `config/rclone-dryrun.example.yaml` and `scripts/rclone-dry-run-preflight.sh`, but do not execute the script unless a supervised prompt explicitly authorizes it.

## Real-Peek Helper Scripts

The supervised real-peek workflow uses a temporary Compose project and a repo-local ignored runtime/cache directory:

```bash
CARTOLENSIA_REAL_PEEK_PREFIX='Cartolensia-photos' \
CARTOLENSIA_REAL_PEEK_EXECUTE=1 \
bash scripts/real-peek-start.sh
```

Important defaults:

- storage name `rclone_peek`;
- storage root `/mnt/Models/rclone`;
- storage mode `strict_read_only`;
- server bound to `127.0.0.1:18080`;
- default `max_files=50`;
- default `max_bytes=2147483648`;
- missing marking disabled;
- hash-after-index enabled for the bounded subset;
- metadata/previews disabled unless their explicit environment toggles are set.

Reset that temporary session only after inspection is complete:

```bash
bash scripts/real-peek-reset.sh
```

The reset script stops the app, removes the temporary PostgreSQL volume for project `cartolensia_realpeek`, and deletes `.cartolensia/runtime` plus `.cartolensia/realpeek-cache`. It does not touch `/mnt/Models/rclone`.

## Map Tiles

The WebUI uses locally bundled OpenLayers, Bootstrap, Bootstrap Icons, and app assets. Vector asset/track layers work without network tiles. If the browser requests OSM base tiles, Cartolensia proxies them through:

```text
GET /api/v1/tiles/osm/{z}/{x}/{y}.png
```

The proxy validates tile coordinates, fetches on demand only, stores cache files under the configured Cartolensia cache directory, and provides no region prefetch endpoint against public OSM. Future offline tile packs should use user-provided PMTiles/MBTiles or a self-hosted tile service.

Track previews use the same cache boundary. GPX/KML/KMZ/GPZ assets can expose:

- `GET /api/v1/media/{asset_id}/track-preview`
- `GET /api/v1/media/{asset_id}/track-thumbnail`

Generated thumbnails are cache files under the Cartolensia cache root. They are never written beside originals.

## Settings, Search, And Exports

The Settings page exposes categorized runtime preferences, effective YAML-bound settings, restart-required pending changes, schema-based plugin settings tabs, plugin YAML editors, and guarded DB metadata exports.

Useful endpoints:

- `GET /api/v1/settings`
- `GET /api/v1/settings/schema`
- `PATCH /api/v1/settings/runtime`
- `GET /api/v1/plugins/{id}/settings/schema`
- `PATCH /api/v1/plugins/{id}/settings`
- `POST /api/v1/admin/db/export`
- `GET /api/v1/admin/db/exports`
- `POST /api/v1/admin/db/import-plan`

Exports are metadata/config JSON files written under the configured Cartolensia cache export directory. They are not destructive restore scripts.

Explorer search is available through:

```text
GET /api/v1/search?q=jpg
```

It supports practical MVP tokens for filenames, paths, extensions, media kinds, date fragments/ranges, hash prefixes, metadata text, tags, album names, and track names. Results include match explanations so the UI can show why an asset matched.

## Video Streaming

Original video streaming uses `/api/v1/media/{asset_id}/original` with HTTP Range support. When `ffmpeg` is available, `/api/v1/media/{asset_id}/stream-options` exposes cache-scoped HLS transcode session profiles. Session output is written only under the configured Cartolensia cache directory and can be stopped through `DELETE /api/v1/media/transcode-sessions/{session_id}/stop`.

Transcoding presets are metadata records:

- built-ins are non-removable;
- custom presets can be saved and removed;
- selected hardware/codec/mode/parameter are validated before session start;
- unsupported hardware or encoders stay disabled in the UI.

Useful endpoints:

- `GET /api/v1/transcoding/presets`
- `POST /api/v1/transcoding/presets`
- `DELETE /api/v1/transcoding/presets/{id}`
- `GET /api/v1/media/transcode-sessions/{session_id}/status`

## AI Sidecar Foundation

AI inference is optional and explicitly operator-controlled. The packaged sidecar is under `services/ai/cartolensia_ai` and can run in dummy/no-model mode or local inference mode after dependencies and model files are installed under `.cartolensia/ai-venv` and `.cartolensia/models`. It uses local libraries and model caches only; no remote inference APIs are used.

Status endpoints:

- `GET /api/v1/ai/status`
- `GET /api/v1/ai/accelerators`
- `GET /api/v1/ai/workers`
- `POST /api/v1/ai/jobs/classify`
- `POST /api/v1/ai/jobs/faces`
- `POST /api/v1/ai/jobs/safety`
- `POST /api/v1/ai/jobs/embed`
- `POST /api/v1/ai/jobs/describe`
- `GET /api/v1/search/vector?q=...`

Model and worker cache paths must stay outside original media roots. The default intended location is `.cartolensia/models`. Run AI jobs only on selected assets or bounded/current indexed scopes.

## Verification Commands

Recommended local verification:

```bash
gofmt -w $(find internal cmd -name '*.go' -print)
git diff --check
GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...
go test ./...
npm --prefix webui run build
bash scripts/smoke-test.sh
docker compose -f docker-compose.yml -f docker-compose.dev.yml config
bash scripts/test-db.sh
```

If Docker is unavailable, skip `scripts/test-db.sh` and document that block.

## Safety

Use `testdata/media_fixture/` and synthetic temporary fixtures for tests. Do not use `/mnt/Models/rclone` unless a future supervised dry run explicitly permits it. Original storage roots are read-only; previews and generated data belong in Cartolensia cache/work directories or ignored synthetic roots.

## Current Real-Peek Runtime Notes

- The supervised real-peek app runs with `.cartolensia/runtime/realpeek.yaml` on `127.0.0.1:18080`.
- Stop only the app without resetting PostgreSQL:

```bash
if [ -f .cartolensia/runtime/realpeek.pid ]; then
  kill "$(cat .cartolensia/runtime/realpeek.pid)" || true
fi
fuser -k 18080/tcp || true
```

- The optional AI sidecar runs on `127.0.0.1:19090`:

```bash
.cartolensia/ai-venv/bin/python -m cartolensia_ai.server --host 127.0.0.1 --port 19090
```

- When the live app occupies `18080`, run smoke tests on a different port:

```bash
CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh
```

## GPU And AI Status Checks

The Base AI page and `/api/v1/ai/status` separate native local inference from optional Docker worker profiles:

- `ai-local` is the native sidecar, normally `http://127.0.0.1:19090`.
- `ai-nvidia` is the optional Docker Compose NVIDIA profile. It can be not configured even when native CUDA is working.
- The active device policy is reported as `auto`, `cpu`, `nvidia`, `rocm`, or `intel`, with CPU fallback always available.

Useful probes:

```bash
curl -fsS http://127.0.0.1:18080/api/v1/ai/status
curl -fsS http://127.0.0.1:18080/api/v1/ai/workers
curl -fsS http://127.0.0.1:18080/api/v1/vector/status
docker info --format '{{json .Runtimes}}' || true
nvidia-smi || true
```

When NVIDIA Container Toolkit is installed, a supervised GPU probe can be run with:

```bash
docker run --rm --gpus all nvidia/cuda:12.8.0-base-ubuntu24.04 nvidia-smi
```

This probe may pull the CUDA base image if it is not local. Do not pull heavy PyTorch/ROCm/Intel images unless explicitly approved.

## Settings File Picker

Settings path fields can use the server-side file/folder picker. The picker is read-only and allowlist based. It can browse configured storage roots, `.cartolensia`, `/tmp`, `/mnt`, `/media`, `/srv`, and home where available, but it never writes files and it does not start discovery. Selecting a real archive path does not change storage mode or safety guards.

## Workflow Operation Notes

- Face Gallery reviews local face detections and provisional/local clusters only. Naming or ignoring a detection updates Cartolensia metadata and does not identify a person or modify media originals.
- Geo Align sessions are scoped to selected/current assets and optional selected tracks. Applying a session writes a Cartolensia DB geotag override only. `write-exif` is disabled for `strict_read_only` storage and should remain disabled for real-peek.
- Manual face rectangles and deleted/ignored face flags are metadata-only edits. They never delete, move, or rewrite original files.
- AV1 preview sessions write WebM output only under the configured transcode cache. The current live validation used `.cartolensia/realpeek-cache/transcode/.../output.webm` and did not write to original storage.
- Video Track Player sessions require a reliable media timestamp plus selected track timestamps. If a video lacks `taken_at`, the UI should report that synchronization needs a user-provided start/end time or offset.
- AV1 live playback is currently disabled for HLS sessions unless a verified browser-compatible encoder/container path exists. Prefer H.264/NVENC for interactive streaming.
- Transcoding metrics can report `ssim` and `psnr` with the current ffmpeg build. `libvmaf` requires an ffmpeg build with the `libvmaf` filter.
