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

### First Production Login

Boot-managed production installs created by `scripts/remote/bootstrap-cartolensia-user.sh`
use local auth by default. The first admin account is configured through the
service environment, and the generated admin password is stored in the
configured password file on the host, typically:

```bash
sudo cat /etc/cartolensia/admin-password
```

Log in with the configured admin email and the exact file contents. The WebUI
and backend ignore trailing CR/LF characters when a password is pasted from the
file, so copying the line from a terminal should work without manually removing
the final newline.

When `auth.mode=local`, normal API and media-original routes require an
authenticated session. Anonymous users can call only the public bootstrap
endpoints needed to render the app and perform login, such as health/version,
`/api/v1/auth/me`, `/api/v1/auth/csrf`, and `/api/v1/auth/login`.

Administrators can mark individual assets as Public from Asset Detail. Public
assets appear in the anonymous Public Gallery and their original/preview media
URLs are readable without a session. Unmarking an asset immediately returns it
to authenticated-only access. Public sharing is metadata-only and never moves,
rewrites, or copies the original file.

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

## Production Deployment

For a production archive mounted at `/originals`:

- keep storage mode `strict_read_only`;
- keep cache/model/component/export directories outside `/originals`;
- use `config/production.yaml` for a VM or bare-metal host;
- use `config/production-container.yaml` with `docker-compose.production.yml` for containers;
- bootstrap auth from `CARTOLENSIA_ADMIN_PASSWORD` or `CARTOLENSIA_ADMIN_PASSWORD_FILE`;
- do not leave `dev_no_auth` active in production.

For large archives, set `max_files = -1` only for normal indexing jobs. Dry-run and preview reports remain capped and are explicitly preview-only.

Recommended container startup:

```bash
cp .env.production.example .env.production
$EDITOR .env.production
docker compose -f docker-compose.production.yml --env-file .env.production up -d
```

For million-file archives, keep preview generation optional and scope discovery to the explicit storage and prefix you intend to index.

### Large Read-Only NAS Mounts

For SMB/NFS/object-storage views mounted at `/originals`, keep the mount
strictly read-only and place every mutable Cartolensia directory under
`/var/lib/cartolensia` or another local disk path:

- PostgreSQL data: `/var/lib/cartolensia/postgres`
- cache/previews/work files: `/var/lib/cartolensia/cache`
- components/tools: `/var/lib/cartolensia/components`
- AI model cache: `/var/lib/cartolensia/models`
- exports/backups: `/var/lib/cartolensia/exports`
- logs/run state: `/var/lib/cartolensia/logs` and `/var/lib/cartolensia/run`

Use explicit storage and prefix selections for real archives. Do not use
`storage=all` against production originals. Do not enable missing-file marking
for read-only NAS indexing.

Recommended first full indexing settings for a large, messy archive:

- `storage`: `originals`
- `prefix`: a top-level folder, or a prioritized list of year/folder prefixes
- `max_files`: `-1` for normal indexing only
- `max_bytes`: `-1` or omitted
- previews: optional, usually off for the first pass
- OCR/ASR/AI: opt-in and scoped after core metadata is visible
- folder workers: conservative first, then increase after watching I/O, DB, RAM,
  and network utilization

The discovery worker streams directory results and updates folder/file counters
in the Jobs page. Gallery results should appear while discovery continues;
Explorer and Search APIs must remain paginated and must not load the whole
archive into memory.

Metadata enrichment is also scoped and paginated. For normal storage/prefix
runs it pages through the shared asset query service instead of loading the
entire catalog first, and selected-asset runs resolve only the requested IDs.
For very large archives, keep metadata extraction scoped to explicit storages
and top-level prefixes so progress and retries remain understandable.

Optional NAS storages may be temporarily unavailable. Cartolensia should report
storage health as `available`, `missing`, or `error` in Settings/Storages and
continue serving already-indexed metadata from PostgreSQL. Do not use
unavailability as a reason to delete asset records, cached metadata, or previews;
run explicit rescan/reconciliation workflows only after the storage is mounted
again and the operator has reviewed the scope.

For NVIDIA plus AMD/Radeon hosts, keep AI and transcoding scratch/output under
the Cartolensia cache directory. NVIDIA NVENC/NVDEC and VAAPI dry-runs should be
validated from the Transcoding page before enabling long-running transcode
sessions. The service chooses a VAAPI render node from `CARTOLENSIA_VAAPI_DEVICE`
or `LIBVA_RENDER_DEVICE` first; when unset, it prefers AMD/Intel DRI render
nodes over NVIDIA render nodes for VAAPI.

### Upgrading A Boot-Managed Host

For hosts created with `scripts/remote/bootstrap-cartolensia-user.sh`, use the
Git upgrade helper when the machine has Internet access:

```bash
sudo CARTOLENSIA_REPO_URL=https://github.com/<owner>/<repo>.git \
  CARTOLENSIA_BRANCH=main \
  bash /opt/cartolensia/current/scripts/remote/upgrade-cartolensia-from-git.sh
```

The helper creates a PostgreSQL backup before swapping releases, keeps
`/var/lib/cartolensia` intact, preserves bundled external tools from the
previous release, and refuses to continue unless `/originals` is mounted
read-only. It checks mount options only and does not write probe files under
`/originals`.

Rollback is a symlink change plus service restart:

```bash
sudo ln -sfn /opt/cartolensia/previous /opt/cartolensia/current
sudo chown -h cartolensia:cartolensia /opt/cartolensia/current
sudo systemctl restart cartolensia-ai cartolensia
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

Search is PostgreSQL/local by default. Place-name search is cache-only unless a future operator-enabled provider flow is added. The current local cache can be inspected with:

```bash
curl -fsS http://127.0.0.1:18080/api/v1/search/places
curl -fsS 'http://127.0.0.1:18080/api/v1/search?q=Yerevan'
```

The Settings → Search/Places tab shows the same cache-only geocoder mode, provider status, and local place cache match counts. No public geocoder is called automatically. Current built-in local entries include Yerevan, Vanadzor, Lori Province, and Armenia.

Universal Search reports the active backend in each response. The current backend is `postgres_local`; Elasticsearch/OpenSearch are not required for the MVP and should be added only when archive size and operations needs justify another service.

OCR is manual-only. `POST /api/v1/ai/jobs/ocr` uses the configured AI sidecar OCR contract when available, and stored OCR blocks can be inspected with:

```bash
curl -fsS http://127.0.0.1:18080/api/v1/ocr/runs
curl -fsS http://127.0.0.1:18080/api/v1/assets/<asset-id>/ocr
```

If Tesseract or a required language pack is missing, the OCR job should fail/report through the AI job path; it must not write OCR cache files into original storage.

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
ffmpeg -hide_banner -hwaccels || true
```

When NVIDIA Container Toolkit is installed, a supervised GPU probe can be run with:

```bash
docker run --rm --gpus all nvidia/cuda:12.8.0-base-ubuntu24.04 nvidia-smi
```

This probe may pull the CUDA base image if it is not local. Do not pull heavy PyTorch/ROCm/Intel images unless explicitly approved.

For production systemd installs, the bundled PostgreSQL launcher uses
`dynamic_shared_memory_type=mmap`. This avoids fragile `/dev/shm` POSIX dynamic
shared-memory segments in VM/LXC-style deployments and allows the Cartolensia
backend service to restart independently from the bundled PostgreSQL service.

For AMD/Radeon transcoding, verify that the service account can read
`/dev/dri/renderD*` and that FFmpeg reports `vaapi`, `drm`, `opencl`, or
`vulkan` in `ffmpeg -hwaccels`. For NVIDIA transcoding, verify `nvidia-smi` and
FFmpeg `cuda`/NVENC encoder availability. Host GPU drivers and device passthrough
are operator responsibilities and are not bundled by Cartolensia.

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

## OCR And Place Cache Operations

Check OCR engine status through the sidecar and backend:

```bash
curl -fsS http://127.0.0.1:19090/capabilities
curl -fsS http://127.0.0.1:18080/api/v1/ai/status
curl -fsS http://127.0.0.1:18080/api/v1/ocr/runs
```

The required OCR language set is `eng`, `rus`, `hye`, `chi_sim`, and `chi_tra`. If any language is missing, OCR jobs should fail with an actionable missing-language message rather than falling back to a remote service. OCR should be run manually on selected/current scopes only; do not run OCR as part of unbounded discovery.

Manage local place cache from Settings -> Search/Places or through:

```bash
curl -fsS http://127.0.0.1:18080/api/v1/places
curl -fsS "http://127.0.0.1:18080/api/v1/search?q=Yerevan"
```

Place search is cache-only by default. Do not bulk geocode public providers. Future online provider use must be user-triggered, rate-limited, and cached before reuse.

## Offline Distribution Builds

Build a local Linux x86_64 offline package with:

```bash
make dist-offline-linux
```

or call the packager directly:

```bash
CARTOLENSIA_DIST_AI_FLAVOR=runtime \
CARTOLENSIA_DIST_INCLUDE_TOOLS=1 \
CARTOLENSIA_DIST_INCLUDE_POSTGRES=1 \
bash scripts/dist/build-offline-linux.sh
```

The package builder writes only under repo-local `dist/` by default. It stages:

- `bin/cartolensia`;
- built `webui/dist` assets;
- offline launcher scripts and configs;
- optional ffmpeg/ffprobe/Tesseract/OCR language data;
- optional PostgreSQL runtime files;
- optional Python AI runtime packages;
- optional reviewed model cache;
- license notices, dependency manifests, and optional AGPL source snapshot.

The GitHub Actions workflow `Build Offline Distribution` is manually triggerable and creates or updates a release with a `.7z` archive and `.sha256` checksum. Use `ai_flavor=runtime` for a compact OCR-capable package, `cpu` for CPU AI packages, and `cuda128` only after reviewing CUDA/PyTorch redistribution terms.

Offline packages are self-contained application bundles for compatible Linux x86_64 hosts, but they cannot include host kernel/GPU drivers. GPU acceleration therefore still depends on compatible host drivers already being installed. Model weights are bundled only when `include_models` is enabled and the model cache has been reviewed for license/provenance.

## Component Manager Operations

Use Settings -> Components to audit and provide local runtime components:

- media tools: FFmpeg and FFprobe;
- metrics: VMAF/libvmaf detection;
- OCR: Tesseract and English/Russian/Armenian/Chinese language data;
- AI runtime: Python venv, PyTorch/torchvision, OpenCV, Transformers/Safetensors, OpenCLIP, facenet-pytorch;
- AI models: EfficientNet-B0, MobileNetV3 Large, YuNet, Falconsai NSFW, OpenCLIP ViT-B/32, BLIP base.

The Check action validates the current system or repo-local path and records component events. Provide path accepts an operator-selected executable or extracted directory after expected-file validation. Provide archive accepts `.zip`, `.tar.gz`, or `.tgz` and extracts only under `.cartolensia/components/<component-key>` after traversal/link checks.

Do not use `/mnt/Models/rclone` as a component source, destination, cache, or extraction target. The API rejects that path for component operations. Component download/install buttons create visible jobs and provenance-gated messages; they do not silently fetch unreviewed third-party binaries.

Asset detail pages use the component registry to explain missing AI/OCR/model prerequisites before starting an asset-scoped action. A missing component can be resolved by opening Settings -> Components and checking/providing the relevant component key.

## Audio And Multimodal Metadata Operations

Audio files are indexed only through the normal bounded discovery flow. For real-archive storage, always provide a non-root prefix plus `max_files` and `max_bytes`; do not run unbounded discovery or missing-file marking.

Example bounded audio scan:

```bash
curl -fsS -X POST http://127.0.0.1:18080/api/v1/indexing/start \
  -H 'Content-Type: application/json' \
  -d '{
    "storage":"rclone_peek",
    "prefix":"Cartolensia-photos/Sound Records",
    "max_files":20,
    "max_bytes":2147483648,
    "include_extensions":["wav","mp3","m4a","flac","ogg","opus","aac","amr","3gp","3gpp","webm"],
    "index_files":true,
    "hash":false,
    "metadata":true,
    "previews":false,
    "parse_tracks":false,
    "geotag_exif":false,
    "snap_to_tracks":false,
    "refresh_map":false
  }'
```

Run or rerun bounded audio feature analysis with selected asset IDs or a bounded current scope:

```bash
curl -fsS -X POST http://127.0.0.1:18080/api/v1/audio/analyze/start \
  -H 'Content-Type: application/json' \
  -d '{
    "scope":"selected",
    "asset_ids":["<audio-asset-id>"],
    "limit":1
  }'
```

Run ASR on a selected audio/video asset with:

```bash
curl -fsS -X POST http://127.0.0.1:18080/api/v1/ai/jobs/transcribe \
  -H 'Content-Type: application/json' \
  -d '{
    "scope":"selected",
    "asset_ids":["<audio-or-video-asset-id>"],
    "limit":1,
    "model":"small",
    "language":"auto"
  }'
```

Useful inspection endpoints:

- `/api/v1/assets?media_kind=audio&limit=20`;
- `/api/v1/assets/{id}/audio-features`;
- `/api/v1/audio/{id}/metadata`;
- `/api/v1/assets/{id}/transcripts`;
- `/api/v1/assets/{id}/document`;
- `/api/v1/ai/workers`;
- `/api/v1/components/status`;
- `/api/v1/search?q=audio`;
- `/api/v1/search?q=transcript:station`;
- `/api/v1/search?q=tempo:120..140`;
- `/api/v1/search?q=document:invoice`.

ASR uses faster-whisper when `asr-faster-whisper`, `asr-ctranslate2`, and an ASR model component are installed. Audio analysis uses librosa/SoundFile. Marker, advanced video captioning, and dedicated genre classifiers are optional component/model paths. If they are missing, operators should see a missing-component state rather than silent fallback to a remote service.

## Full Cartolensia-Photos Read-Only Indexing

For the real-peek archive, full normal indexing is allowed only for an explicit storage and prefix. Do not use storage `all`, do not use the storage root, and do not enable missing-file marking.

Recommended full-prefix request for newly added files under `Cartolensia-photos`:

```bash
curl -fsS -X POST http://127.0.0.1:18080/api/v1/indexing/start \
  -H 'Content-Type: application/json' \
  -d '{
    "storage":"rclone_peek",
    "prefixes":["Cartolensia-photos"],
    "max_files":-1,
    "max_bytes":-1,
    "include_extensions":[
      "jpg","jpeg","png","heif","heic",
      "mp4","mov","webm","mkv","avi","m4v",
      "gpx","kml","kmz","gpz",
      "wav","mp3","3gp","3gpp","aac","m4a","flac","ogg","oga","opus","amr",
      "pdf","djvu","txt","md","markdown"
    ],
    "index_files":true,
    "hash":true,
    "metadata":true,
    "previews":true,
    "parse_tracks":true,
    "geotag_exif":true,
    "snap_to_tracks":true,
    "refresh_map":true
  }'
```

`max_files=-1` means no file-count limit for normal indexing. Dry-run and preview screens may still cap output at `50` files for safety and UI readability; that cap is not the normal indexing limit.

After discovery, run bounded AI jobs from the UI or API. For full explicit indexed scope, the accepted pattern is:

```bash
curl -fsS -X POST http://127.0.0.1:18080/api/v1/ai/jobs/classify \
  -H 'Content-Type: application/json' \
  -d '{"scope":"indexed","limit":-1}'
```

Use the equivalent job paths for `faces`, `safety`, `embed`, `describe`, `ocr`, `transcribe`, and `audio-analyze`. These jobs skip unsupported media kinds, keep generated metadata in PostgreSQL/cache paths, and do not write sidecars to original storage.

## GPS Track Direction Arrows

Direction arrows are drawn in OpenLayers vector styles at a configurable distance interval. Runtime setting:

- `gps.track_arrow_interval_m=500` by default.
- Set to `0` to hide track arrows.

The setting applies to the shared track style used by track previews/detail maps, Geo Align track layers, and main map track layers where those layers use the shared style helper.

## Reverse Geocoding Operations

Reverse geocoding is local-first and safe by default:

- `GET /api/v1/places/reverse?lat=<lat>&lon=<lon>` searches cached place bounding boxes.
- `POST /api/v1/places/reverse` accepts JSON with `lat`, `lon`, and optional `online`.
- Online lookup requires runtime setting `search.online_geocoding=true` and an explicit `online=true` request.
- Online providers must be Nominatim-compatible and are configured with `search.geocoder_provider_url`.
- Results are cached into `place_cache`; no public API bulk reverse-geocoding is run automatically.

## Local Production Run On This Machine

With PostgreSQL and the AI sidecar environment already prepared, a local production-style run is:

```bash
docker compose -p cartolensia_realpeek -f docker-compose.yml -f docker-compose.dev.yml up -d postgres
GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go build -o /tmp/cartolensia-live ./cmd/cartolensia
CARTOLENSIA_HTTP_ADDR=127.0.0.1:18080 /tmp/cartolensia-live -config .cartolensia/runtime/realpeek.yaml
```

In another terminal:

```bash
.cartolensia/ai-venv/bin/python -m cartolensia_ai.server
```

For another offline machine, copy the built application/package, the reviewed `.cartolensia/components` and `.cartolensia/models` contents, the Python AI environment or recreated wheelhouse, and a PostgreSQL runtime or database service. Keep real media mounted read-only and point storage config at that read-only root. Do not copy local secrets or machine-specific `.env` files.

## Search And Context Operations

Universal Search is PostgreSQL/local in the current build. Elasticsearch/OpenSearch remain deferred.

Useful syntax:

- Space-separated terms are AND: `filename:PXL* Pixel`.
- Comma-separated alternatives are OR inside one token: `ext:jpg,mp4`.
- Quotes keep phrases together: `ocr:"station sign"`.
- Wildcards `*` and `?` are supported for filename/path/plain text matching.
- Common explicit tokens: `ext:`, `kind:`, `filename:`, `path:`, `ocr:`, `transcript:`, `caption:`, `document:`, `place:`, `camera:`, `hash:`, `track:`, `album:`, `face:`, `safety:`, `private:`.

Examples:

```bash
curl -fsS "http://127.0.0.1:18080/api/v1/search?q=ext:mp4"
curl -fsS "http://127.0.0.1:18080/api/v1/search?q=filename:PXL_20260512*"
curl -fsS "http://127.0.0.1:18080/api/v1/search?q=kind:video%20caption:train"
```

Asset context can be inspected with:

```bash
curl -fsS "http://127.0.0.1:18080/api/v1/assets/<asset-id>/related"
```

The related endpoint is bounded and metadata-only. It returns groups such as same folder, same camera/device, same day, nearby time window, and overlapping GPS tracks.

## GPS Track Media Matching

Track media matching uses timestamp candidates and geotag proximity:

- trusted `taken_at`;
- EXIF raw DateTimeOriginal interpreted with runtime local timezone;
- filename timestamps such as `PXL_YYYYMMDD_HHMMSS` and `VID_YYYYMMDD_HHMMSS`;
- file mtime as lower-confidence fallback;
- geotag proximity to track geometry.

This helps associate videos without GPS and photos with timezone-less EXIF to GPX/KML tracks. The lookup is read-only and metadata-only.

Useful checks:

```bash
curl -fsS "http://127.0.0.1:18080/api/v1/gps/tracks/<track-id>/assets?include_ungeotagged=true&limit=200"
curl -fsS "http://127.0.0.1:18080/api/v1/gps/tracks/<track-id>/nearby-assets?distance_m=1000"
```

## Video Track Player Operations

Video Track Player sessions now use the same timestamp candidates. The UI exposes a searchable video selector and a track-name pill selector. If a chosen video has no `taken_at`, Cartolensia can still use filename or file mtime candidates and reports the chosen `time_source` in session/position responses.

Manual offset controls remain important when a device encodes local filenames, EXIF, and filesystem mtimes inconsistently. If a candidate timestamp is just outside the track range, the position endpoint may clamp to a nearby track start/end rather than fail.

## Remote Production HTTPS And AI Backfill

For the current rjazhenka-style production deployment:

- Browser URL: `https://<host-or-ip>:18443/`.
- Plain HTTP on `:18080` is a redirect-only convenience for browser users.
- Production cookies are `Secure`, so login and media playback must use HTTPS.
- The first admin login uses the configured admin email, for example `admin@example.local`, and the password stored in `/etc/cartolensia/admin-password` on the host. Copy only the file value; trailing newlines are ignored.
- Originals must remain mounted read-only. Cache, model, component, export, logs, and PostgreSQL data stay under `/var/lib/cartolensia` or another configured data root outside originals.

Check services:

```bash
systemctl status cartolensia-postgres cartolensia-ai cartolensia
curl -k https://127.0.0.1:18443/api/v1/health
curl http://127.0.0.1:19090/health
```

The AI backfill driver is intended for large archives. It selects missing metadata rows from PostgreSQL and runs small authenticated API batches:

```bash
source /etc/cartolensia/cartolensia.env
source /opt/cartolensia/current/bin/cartolensia-env
python3 /var/lib/cartolensia/run/run-ai-backfill.py
```

Useful environment controls:

- `CARTOLENSIA_AI_BACKFILL_PHOTO_BATCH`, default `8`;
- `CARTOLENSIA_AI_BACKFILL_OCR_BATCH`, default `4`;
- `CARTOLENSIA_AI_BACKFILL_AUDIO_BATCH`, default `2`;
- `CARTOLENSIA_AI_BACKFILL_TRANSCRIBE_BATCH`, default `1`;
- `CARTOLENSIA_AI_BACKFILL_MAX_AUDIO_SECONDS`, default `900`;
- `CARTOLENSIA_AI_BACKFILL_MAX_VIDEO_SECONDS`, default `900`.

Logs should go under `/var/lib/cartolensia/logs`. The state directory `/var/lib/cartolensia/run/ai-backfill-state` records successful no-result checks so blank OCR/no-speech assets are not retried forever.

For pgvector, the PostgreSQL runtime must have the `vector` extension installed in the active server library path. Cartolensia creates the optional vector column/index automatically when the extension is available. Verify with:

```bash
curl -k -b <authenticated-cookie> https://127.0.0.1:18443/api/v1/vector/status
```

## Large Explorer And Background AI Operations

For million-file or large NAS-backed archives, Explorer folder browsing should use `/api/v1/explorer?view=folders&path=...&limit=200&offset=...`. The production PostgreSQL store computes immediate folders and direct file pages in SQL. The WebUI loads the first page and exposes Load More rather than trying to materialize an entire folder in the browser.

Operational checks:

```bash
curl -k -b <authenticated-cookie> "https://127.0.0.1:18443/api/v1/explorer?view=folders&path=2026/May2026&limit=200&offset=0"
curl -k -b <authenticated-cookie> "https://127.0.0.1:18443/api/v1/jobs?limit=20"
```

The AI backfill driver is safe to leave running while discovery proceeds. It submits small authenticated batches and records successful no-result checks under `/var/lib/cartolensia/run/ai-backfill-state`. If the web service restarts, restart the driver with a valid `CARTOLENSIA_DATABASE_URL` derived from production config:

```bash
ds=$(awk '$1=="url:" {print $2; exit}' /opt/cartolensia/current/config/production-bundle.yaml | tr -d '"')
nohup env CARTOLENSIA_DATABASE_URL="$ds" \
  CARTOLENSIA_BACKFILL_BASE_URL=https://127.0.0.1:18443 \
  python3 /var/lib/cartolensia/run/run-ai-backfill.py \
  >>/var/lib/cartolensia/logs/ai-backfill-$(date -u +%Y%m%dT%H%M%SZ).log 2>&1 &
```

Do not launch full-archive hashing casually on remote NAS storage. Hashing is read-only, but it can still read many terabytes and compete with previewing, discovery, and AI. Prefer discovery and metadata enrichment first, then scoped hashing for duplicates/integrity work.
