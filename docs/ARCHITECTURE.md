# Architecture

The implemented codebase is a Go backend, PostgreSQL-capable metadata store, strict read-only filesystem storage adapter, async worker runtime, local auth foundation, media/track metadata services, and Vue 3 + TypeScript WebUI.

## Runtime Modes

Cartolensia can currently run in two modes:

- `memory` store: default when no database URL is configured. This is useful for fixture smoke tests and development without PostgreSQL.
- `postgres` store: enabled by `database.url` or `CARTOLENSIA_DATABASE_URL`. Startup applies SQL migrations, snapshots effective config, stores configured storage backends, stores built-in plugin manifests, and detects optional database capabilities.

PostgreSQL is the intended durable mode. The memory store is a fallback and test aid, not the long-term production store.

Runtime migrations are embedded into the Go binary from `migrations/*.sql`. Disk-loaded migrations remain available only when `database.migrations_dir` or `CARTOLENSIA_MIGRATIONS_DIR` is explicitly configured.

## Backend Packages

- `internal/app`: startup wiring, config loading, storage registry, plugin loading, and database selection.
- `internal/config`: YAML configuration, environment overrides, defaults, validation, and absolute path normalization.
- `internal/database`: pgx-backed PostgreSQL connection, migrations, config snapshots, plugin/storage upserts, catalog store, jobs, logs, stats, and capability detection.
- `internal/storage`: universal `fs://storage/path` URLs, strict read-only filesystem adapter, traversal prevention, MIME/media-kind detection, and safe open/list/stat behavior.
- `internal/catalog`: logical assets, storage locations, content/hash status, folder-style Explorer grouping, stats, and store contract.
- `internal/discovery`: fast fixture-safe discovery, bounded prefix dry-run reports, and lazy SHA-512 hashing handlers with cancellation checks. Discovery intentionally avoids heavy media parsing; metadata enrichment is a separate job.
- `internal/exif`: small server-side EXIF wrapper around `github.com/rwcarlsen/goexif` with safe no-EXIF handling and conservative timezone policy.
- `internal/metadata`: explicit metadata enrichment jobs for images, videos, GPX/KML/KMZ tracks, and track-snapped geotags. Image dimensions use Go decoders, JPEG EXIF can populate metadata and typed `asset_geo`, video metadata uses optional ffprobe, and track enrichment computes point counts, bbox, duration, distance, and elevation where possible.
- `internal/jobs`: job model, state transitions, counters, progress, logs, cancellation, leases, and retry scheduling.
- `internal/workers`: async worker loop, lease acquisition, heartbeats, panic recovery, and graceful stop.
- `internal/auth`: local admin bootstrap, password hashing, password rotation, session/API-token auth, token scopes, CSRF flow, persisted auth store contracts, and explicit `dev_no_auth` development mode.
- `internal/gpx`: dependency-free GPX parser, route/waypoint support, track analysis, haversine distance, and deterministic simplification.
- `internal/kml`: dependency-free KML/KMZ parser for practical `Point`, line coordinate, and `gx:Track` ingestion.
- `internal/preview`: preview status, cache-key/path safety, cache cleanup, preview generation jobs, and JPEG preview generation for decodable images.
- `internal/media`: SHA-512 streaming hash and optional ffprobe video metadata detection/extraction.
- `internal/plugins`: built-in and filesystem plugin manifests, dependency topological sort, sidecar HTTP manifest validation, and plugin status/health stubs.
- `internal/transcoding`: bounded ffmpeg/ffprobe capability inventory, encoder parsing, and hardware hints.
- `internal/server`: REST API, indexing pipeline/status surface, original streaming, cached preview serving, track preview/thumbnail rendering, cache-scoped HLS transcode sessions and preset APIs, search, settings/export MVP, tile proxy, AI worker registry stubs, and WebUI static serving.
- `internal/tlsutil`: dependency-free in-memory self-signed TLS certificate generation for local/private HTTPS deployments.

## REST API

Implemented endpoints:

- `GET /api/v1/health`
- `GET /api/v1/version`
- `GET /api/v1/config/effective`
- `GET /api/v1/auth/me`
- `GET /api/v1/auth/csrf`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/password`
- `GET /api/v1/auth/tokens`
- `POST /api/v1/auth/tokens`
- `DELETE /api/v1/auth/tokens/{id}`
- `GET /api/v1/storages`
- `GET /api/v1/plugins`
- `GET /api/v1/plugins/{id}`
- `GET /api/v1/plugins/{id}/health`
- `POST /api/v1/plugins/rescan`
- `GET /api/v1/jobs`
- `GET /api/v1/jobs/stats`
- `GET /api/v1/jobs/{id}`
- `GET /api/v1/jobs/{id}/logs`
- `POST /api/v1/jobs/{id}/cancel`
- `POST /api/v1/jobs/{id}/retry`
- `POST /api/v1/discovery/start`
- `POST /api/v1/indexing/start`
- `GET /api/v1/indexing/latest`
- `GET /api/v1/indexing/{pipeline_id}`
- `POST /api/v1/indexing/{pipeline_id}/cancel`
- `POST /api/v1/discovery/dry-run`
- `GET /api/v1/discovery/dry-run/{job_id}/report`
- `POST /api/v1/hash/start`
- `POST /api/v1/metadata/enrich/start`
- `POST /api/v1/previews/start`
- `GET /api/v1/previews/status`
- `GET /api/v1/previews/cache`
- `POST /api/v1/previews/cleanup`
- `GET /api/v1/assets`
- `GET /api/v1/assets/months`
- `GET /api/v1/assets/{id}`
- `GET /api/v1/duplicates`
- `GET /api/v1/albums`
- `POST /api/v1/albums`
- `GET /api/v1/albums/{id}`
- `PATCH /api/v1/albums/{id}`
- `DELETE /api/v1/albums/{id}`
- `GET /api/v1/albums/{id}/items`
- `POST /api/v1/albums/{id}/items`
- `DELETE /api/v1/albums/{id}/items/{asset_id}`
- `GET /api/v1/explorer`
- `GET /api/v1/tracks`
- `GET /api/v1/tracks/{track_asset_id}`
- `GET /api/v1/gps/tracks`
- `GET /api/v1/gps/tracks/{track_asset_id}`
- `PATCH /api/v1/gps/tracks/{track_asset_id}`
- `GET /api/v1/gps/tracks/{track_asset_id}/points`
- `GET /api/v1/gps/tracks/{track_asset_id}/assets`
- `GET /api/v1/gps/tracks/{track_asset_id}/point-info?lat=...&lon=...`
- `GET /api/v1/gps/tracks/{track_asset_id}/nearby-assets?distance_m=...`
- `POST /api/v1/gps/tracks/{track_asset_id}/snap-media`
- `GET /api/v1/sync/candidates?asset_id=...`
- `GET /api/v1/sync/links`
- `POST /api/v1/sync/links`
- `DELETE /api/v1/sync/links/{id}`
- `GET /api/v1/videos/{asset_id}/track-sync?time_ms=...`
- `GET /api/v1/map?bbox=minLon,minLat,maxLon,maxLat&zoom=...`
- `GET /api/v1/map/status`
- `GET /api/v1/map/assets`
- `GET /api/v1/map/tracks`
- `GET /api/v1/map/tile-sources`
- `GET /api/v1/transcoding/status`
- `GET /api/v1/transcoding/capabilities`
- `GET /api/v1/transcoding/presets`
- `POST /api/v1/transcoding/presets`
- `DELETE /api/v1/transcoding/presets/{id}`
- `GET /api/v1/settings`
- `GET /api/v1/settings/effective`
- `GET /api/v1/settings/schema`
- `PATCH /api/v1/settings/runtime`
- `GET /api/v1/settings/pending`
- `PATCH /api/v1/settings/pending`
- `DELETE /api/v1/settings/pending`
- `GET /api/v1/settings/pending/download`
- `GET /api/v1/settings/restart-required`
- `GET /api/v1/plugins/{id}/settings`
- `PATCH /api/v1/plugins/{id}/settings`
- `GET /api/v1/plugins/{id}/settings/schema`
- `POST /api/v1/admin/db/export`
- `GET /api/v1/admin/db/exports`
- `GET /api/v1/admin/db/exports/{id}/download`
- `POST /api/v1/admin/db/import-plan`
- `GET /api/v1/ai/status`
- `GET /api/v1/ai/accelerators`
- `GET /api/v1/ai/workers`
- `GET /api/v1/ai/workers/{id}`
- `GET /api/v1/ai/summary`
- `GET /api/v1/ai/tags`
- `GET /api/v1/ai/predictions`
- `GET /api/v1/ai/faces`
- `GET /api/v1/ai/safety`
- `POST /api/v1/ai/jobs/classify`
- `POST /api/v1/ai/jobs/faces`
- `POST /api/v1/ai/jobs/describe`
- `POST /api/v1/ai/jobs/ocr`
- `GET /api/v1/assets/{asset_id}/ocr`
- `GET /api/v1/ocr/runs`
- `GET /api/v1/vector/status`
- `GET /api/v1/search?q=...`
- `GET /api/v1/search/places`
- `GET /api/v1/files/browse`
- `GET /api/v1/stats`
- `GET /api/v1/backend/status`
- `GET /api/v1/media/{asset_id}/original`
- `GET /api/v1/media/{asset_id}/preview`
- `GET /api/v1/media/{asset_id}/track-preview`
- `GET /api/v1/media/{asset_id}/track-thumbnail`
- `GET /api/v1/media/{asset_id}/stream-options`
- `POST /api/v1/media/{asset_id}/transcode-session`
- `GET /api/v1/media/transcode-sessions/{session_id}/master.m3u8`
- `GET /api/v1/media/transcode-sessions/{session_id}/{segment}`
- `GET /api/v1/media/transcode-sessions/{session_id}/status`
- `DELETE /api/v1/media/transcode-sessions/{session_id}/stop`
- `GET /api/v1/tiles/{source}/{z}/{x}/{y}.png`

`POST /api/v1/indexing/start` is the user-facing scoped pipeline entry point. It validates storage/prefix/max bounds, starts bounded discovery, and lets the WebUI orchestrate follow-up hash, metadata/EXIF, preview, track parse, geotag, and map-refresh stages against the same scope. `POST /api/v1/discovery/start`, `POST /api/v1/hash/start`, `POST /api/v1/metadata/enrich/start`, and `POST /api/v1/previews/start` remain lower-level job starters and return quickly in the app runtime. The worker loop leases and executes queued jobs asynchronously. Tests can opt into a synchronous server dependency path for deterministic fixture checks.

Metadata enrichment uses the same shared asset-query service as Explorer/Search for normal storage/prefix-scoped jobs. PostgreSQL-backed runs page through matching assets in bounded batches and keep the matching scoped location first, avoiding an all-assets flattening step on large archives. Explicit selected-asset jobs resolve only the requested IDs.

Original streaming uses the read-only storage registry and `http.ServeContent`, which provides HTTP Range and HEAD support when the underlying file supports seeking. Preview generation decodes standard-library-supported image formats, writes cached JPEG previews under the configured Cartolensia cache directory, serves the generated preview, and never writes near originals. Unsupported formats return a clean JSON status.

Video stream options truthfully expose direct original streaming plus cache-scoped HLS transcode-session profiles only when `ffmpeg` is available. Built-in presets are protected and custom presets are persisted in metadata. HLS session output is written under the configured Cartolensia cache directory and can be stopped through the API. Originals are never modified.

The tile proxy is intentionally on-demand only. It validates tile coordinates, caches only tiles actively requested by the browser under the Cartolensia cache directory, exposes attribution metadata through `/api/v1/map/tile-sources`, and provides no bulk prefetch endpoint against public OSM servers.

## WebUI

The WebUI is Vue 3 + TypeScript + Vite with no CDN resources. It contains:

- app shell and navigation;
- Explorer table backed by `/api/v1/explorer`, including folder grouping and breadcrumbs;
- asset detail view backed by `/api/v1/assets/{id}`;
- Discovery page with a scoped indexing pipeline and staged hash/metadata/preview/track/map controls;
- Jobs page with counts, detail/log view, cancel, and retry controls;
- Metadata page with explicit enrichment and preview generation actions;
- Albums page for virtual album creation, item management, and map handoff;
- GPS Tracks page backed by parsed GPX/KML/KMZ track points and enriched distance/duration metadata, including media lookup and snap-media controls;
- Map page backed by GeoJSON from the map API with bundled OpenLayers vector rendering, screen-distance clustering, count labels, click-priority asset/cluster layers above tracks, cluster/point mini-gallery popups, track click popups, album filters, media filters, and track filters;
- universal Explorer search with result explanations, table/tile/gallery reuse, an explicit `postgres_local` `SearchBackend`, OCR/caption/AI metadata matching, and cache-first place matching backed by the durable `place_cache`;
- Duplicates page for report-only SHA-512+size content groups;
- Storages page;
- Plugins page and plugin detail health/status surface;
- Stats page;
- Settings page with categorized tabs for effective config, runtime preferences, Search/Places cache-first reverse-geocoder controls for local cache, Nominatim-compatible, Photon, Pelias, and opt-in Google providers, restart-required YAML settings, schema-based per-plugin settings, cache-scoped DB metadata export, password rotation, and API token management;
- Transcoding page with capability inventory, preset management, auto-selection rule drafts, command-template validation, cache-only job planning, metrics status, and cache-scoped HLS session status;
- Base AI dashboard with native-vs-Docker worker status, GPU policy, model cards, vector fallback status, and visible scoped action/job results;
- AI Classification page for AI tag/category browsing, predictions, safety candidates, face detections, and vector text search.

Browser route state is saved in `localStorage`.

## Safety Boundary

- The default storage mode is `strict_read_only`.
- Originals are immutable.
- The filesystem adapter exposes read/list/stat/open only.
- Write, delete, move, mkdir, and similar operations return explicit read-only errors.
- Path traversal and absolute paths are rejected before filesystem access.
- `..` path segments are rejected before cleaning, including encoded URL traversal attempts.
- Symlinks are skipped during recursive discovery and opening a symlink that escapes the root is rejected.
- Scoped discovery dry-run uses required non-empty prefixes, max-files/max-bytes defaults, strict read-only storage, no missing marking, and report-only behavior.
- Scan-run reports and preview-cache records are stored in PostgreSQL/memory metadata only; generated previews stay under the configured cache root.
- Write-like endpoints pass through auth and authorization hooks; `dev_no_auth` is the default fixture mode.
- `local` auth mode requires configured admin email and a password supplied through an environment variable or ignored bootstrap file. No production password is hardcoded.
- Cookie-authenticated write requests require a CSRF header obtained from `GET /api/v1/auth/csrf`. Bearer API tokens bypass CSRF but must carry sufficient scopes.
- API token scopes currently include `read`, `write`, `jobs:write`, `plugins:write`, `media:read`, and `admin`.
- Real-archive guardrails reject `storage=all`, empty/root prefixes, missing max limits, and unsafe absolute prefixes when a configured storage root is `/mnt/Models/rclone` or inside it.
- `/mnt/Models/rclone` is not required and was not touched by the MVP tests.

## HTTP And HTTPS

The app serves one configured listener at `http.addr`. Plain HTTP is the default. HTTPS can be enabled either by providing both `http.tls_cert_file` and `http.tls_key_file`, or by setting `http.tls_auto_self_signed: true` for a generated in-memory self-signed certificate. Self-signed certificates are intended for local/private deployments and are not persisted to disk.

## Database Capability Policy

PostGIS, pgvector, and pg_trgm are detected at startup. PostGIS may be installed by the development Docker image. pgvector and pg_trgm are optional for the MVP. Missing optional extensions do not block core startup.

## AI Sidecar Foundation

The optional AI sidecar contract is HTTP JSON. The packaged FastAPI worker under `services/ai/cartolensia_ai` supports dummy/no-model mode and approved local inference mode. In local inference mode it uses established libraries instead of custom model code: torchvision for EfficientNet-B0/MobileNetV3 image classification, OpenCV YuNet for face detection, Transformers/Safetensors for Falconsai safety classification and BLIP captioning, and OpenCLIP for image/text embeddings. The backend probes `127.0.0.1:19090`, dispatches only explicit bounded AI jobs, reads media through Cartolensia read-only media URLs, and stores tags, predictions, face detections, and JSON embeddings in PostgreSQL. Model/cache paths are expected under repo-local `.cartolensia/models` or another configured non-archive directory.

AI worker status distinguishes the active native sidecar from optional Docker Compose profiles. A native CUDA worker is represented as `ai-local` with its endpoint, selected device, and model state. Docker profile rows such as `ai-nvidia` describe optional containerized workers and report Docker NVIDIA runtime availability separately, so a not-configured Docker profile does not imply that native CUDA is unavailable.

AI actions currently create visible job records and then execute bounded work through the API handler. This keeps the UI and Jobs page auditable while durable leased AI workers remain future hardening.

## Server-Side Path Picker

Settings path fields use `GET /api/v1/files/browse` for an allowlisted file/folder picker. The picker lists only configured roots such as `.cartolensia`, `/tmp`, `/mnt`, `/media`, `/srv`, home, and configured storage roots. It rejects traversal, returns readability/selectability metadata, and performs no writes. Real archive roots are marked read-only with warnings.

## Component Manager

The Component Manager persists tool/model/runtime records in `components` and `component_events`. It exposes `/api/v1/components`, `/api/v1/components/{key}/check`, `/provide-path`, `/provide-archive`, `/enable`, `/disable`, and `/events`. Downloads are represented as jobs but are intentionally gated unless a reviewed source is configured; user-provided paths and archives are the active workflow in this slice.

Component archives are extracted only below `.cartolensia/components/<component-key>`. The extractor rejects absolute paths, `..` traversal, symlinks, hardlinks, and unsupported archive formats, then validates expected files before accepting the component. System-path components are checked in place and never copied unless the user imports an archive. `/mnt/Models/rclone` is rejected as a component path or archive source.

Settings -> Components groups media tools, OCR language packs, AI runtime packages, and AI models with status, version/path, license/provenance, check/import controls, and event logs. Component records are also used by asset-detail AI actions to provide actionable missing-component messages.

## Future Interfaces

- Vector search is implemented through a local JSON/PostgreSQL fallback using stored float arrays and bounded brute-force cosine search for small local collections. pgvector remains optional for later scaling.
- Search is implemented behind a small `SearchBackend` interface. The current backend is `postgres_local`; Elasticsearch/OpenSearch are intentionally future optional adapters.
- OCR is represented as a Base AI sidecar contract and stored metadata/prediction surface with bounding boxes. The Tesseract runtime can feed this contract without changing asset-detail/search UI.
- Sidecar HTTP plugins are represented in manifests but are user-managed services; Cartolensia does not auto-start arbitrary plugin binaries.
- Live video-track sync is represented in schema by `track_points` and `asset_track_links`, including `time_offset_ms`, with a marker interpolation API for linked videos.
- Transcoding contracts and capability detection exist. Cache-scoped HLS sessions and preset management are implemented as a safe MVP; durable transcoding jobs and managed output libraries are still future work.

## Distribution Architecture

Offline distribution is handled by `scripts/dist/build-offline-linux.sh`. The packager builds a Linux x86_64 application archive from normal package-manager/runtime inputs instead of vendoring third-party source into the repository. A staged package contains the Go backend binary, built WebUI assets, offline YAML configs, launcher scripts, docs, license manifests, optional source snapshot, and optional runtime toolchains.

Optional bundled components are isolated by directory:

- `external/bin`, `external/lib`, and `external/share/tessdata` for ffmpeg/ffprobe/Tesseract and OCR language data;
- `external/postgres` for a local PostgreSQL runtime discovered from `pg_config`;
- `python` and `ai/python-site` for a copied Python runtime plus target-installed sidecar packages;
- `.cartolensia/models` for explicitly reviewed model weights.

The launcher prefers bundled PostgreSQL when available and falls back to the memory config otherwise. It starts the AI sidecar only when a bundled Python runtime and sidecar site-packages are present. Runtime state is kept under the extracted package's `runtime`, `logs`, and `.cartolensia/cache` directories; media defaults to a `strict_read_only` local `media` directory.

The GitHub Actions offline release workflow is manual. It verifies Go/WebUI builds, assembles the package, uploads workflow artifacts, and attaches the archive/checksum to a GitHub release. GPU drivers and public map/geocoder data are not bundled.

The packager also writes `components-manifest.json` into the archive root and `licenses/components-manifest.json` for release review. FFmpeg configure flags are captured when FFmpeg is bundled; `--enable-nonfree` fails the package by default, and `--enable-gpl` is recorded so the archive can be labeled as a GPL-tools bundle.

## Multimodal Metadata Architecture

Cartolensia now treats audio and documents as first-class media kinds alongside photos, videos, and GPS tracks. The storage classifier recognizes common audio extensions (`wav`, `mp3`, `m4a`, `flac`, `ogg`, `opus`, `aac`, `amr`, `3gp`, `3gpp`) and document extensions (`pdf`, `djvu`, `txt`, `md`, `markdown`). Discovery still obeys the same bounded/strict-read-only safety model.

FFprobe probing is shared across video and audio. Audio metadata stored on assets includes duration, audio codec, container, bitrate, sample rate, channel count, and stream-presence flags. Audio enrichment also writes a durable `audio_features` record using the `ffprobe_metadata` model so future analyzers can extend the same row with tempo, key, loudness, speech/music ratio, and genre labels.

New normalized metadata tables support multimodal search and asset-detail pages:

- `asset_transcripts` and `asset_transcript_segments` for ASR output;
- `audio_features` for audio analysis;
- `video_frame_captions` for sampled-frame descriptions;
- `document_text` for OCR/Marker/PDF markdown output.

The current `postgres_local` search backend indexes these records through bounded asset-scoped lookups and PostgreSQL indexes. Elasticsearch/OpenSearch remain future optional adapters behind the SearchBackend abstraction.

Asset detail exposes OCR full text, transcripts, audio features, video frame captions, and document text through dedicated subroutes. Heavy engines such as ASR, Marker, advanced video description, and genre classifiers remain optional component-managed integrations.

## 2026-06-07 Update

- GPS/KML track detail now has a dedicated API/UI flow with OpenLayers geometry preview, altitude and speed profiles, point-info lookup, media-by-time lookup, and nearby-geotag media lookup.
- Runtime storage management can add or validate non-destructive filesystem storages in the active registry. Real archive roots remain locked to `strict_read_only`; write-capable modes are disabled.
- The AI sidecar is now a packaged FastAPI service under `services/ai/cartolensia_ai`; backend AI worker status probes a local sidecar on `127.0.0.1:19090`, and approved local models can run classification, safety, face detection, captions, and embeddings without remote APIs.
- Transcoding hardware validation has an explicit API and UI flow. NVIDIA H.264 NVENC command generation is validated with a short null-output dry run when approved.
- Universal search now includes filename/path/extension/hash/date/metadata plus implemented `album:` and `track:` token handling through existing store data.

## 2026-06-07 Workflow Stabilization Update

- Face management now has explicit backend APIs for cluster listing, cluster naming, cluster assets, detection assignment, and detection ignore metadata. When model clustering has not produced stable cluster IDs, the backend exposes deterministic provisional folders so users can still review detected faces.
- Geo alignment is represented by scoped in-memory sessions and DB-only geotag override application. The API combines existing EXIF geotags, selected track interpolation candidates, and manual marker changes. EXIF writeback is deliberately disabled for strict read-only storage.
- Video-track playback is represented by scoped in-memory sessions. The position endpoint interpolates selected tracks against video playback time when a reliable video timestamp exists and returns a warning when synchronization inputs are insufficient.
- Job list responses summarize very large payloads by default to keep operational pages responsive. Full payloads remain available to callers that explicitly request them.
- Transcoding capability reporting now includes metrics filter detection for `libvmaf`, `ssim`, and `psnr`. AV1 live HLS is treated as unsupported unless a verified browser-compatible route is selected.

## 2026-06-08 OCR And Place Cache Update

- The AI sidecar now includes a real `ocr_image` implementation backed by the local Tesseract CLI. OCR is still explicit/manual, reads only bounded localhost or safe temp/cache inputs, and returns text blocks with bounding boxes and confidence.
- OCR records are stored as Cartolensia metadata/prediction rows so the existing asset-detail overlays and PostgreSQL/local search can index OCR text without modifying originals or writing sidecars.
- `place_cache` is now a durable PostgreSQL/memory store concept. The app seeds cache-only entries for Yerevan, Armenia, Lori Province, and Vanadzor, and exposes operator CRUD through `/api/v1/places`.
- Search and asset-detail reverse-place rows now read from durable local place entries with built-in defaults only as fallback. Online geocoding remains intentionally absent from automatic flows.

## 2026-06-08 ASR And Audio Analysis Update

- The AI sidecar now exposes `POST /transcribe-audio` for local faster-whisper ASR. It accepts bounded media URLs or safe local temp/cache paths, materializes input under `/tmp` when needed, and deletes temporary copies after inference.
- ASR metadata is stored in normalized PostgreSQL tables:
  - `asset_transcripts` for full transcript text, source kind, language, model, and metadata;
  - `asset_transcript_segments` for timestamped text segments and confidence metadata.
- Backend endpoints are available for transcript workflows:
  - `POST /api/v1/ai/jobs/transcribe`;
  - `GET /api/v1/assets/{id}/transcripts`;
  - `GET /api/v1/transcripts`;
  - `DELETE /api/v1/transcripts/{id}`.
- The AI sidecar also exposes `POST /analyze-audio` using librosa/SoundFile. The backend routes `/api/v1/audio/analyze/start` and `/api/v1/ai/jobs/audio-analyze` persist tempo, key, mode, loudness, speech/music ratio, spectral summary, and heuristic labels into the existing `audio_features` table.
- `postgres_local` search now matches transcript text and audio features. Supported audio tokens include `transcript:...`, `genre:...`, `key:...`, exact tempo values, and tempo ranges such as `tempo:120..140`.
- Component Manager tracks ASR/audio dependencies and models as first-class components: `asr-faster-whisper`, `asr-ctranslate2`, `asr-model-small`, `asr-model-medium`, `audio-librosa`, and `audio-soundfile`.

## 2026-06-09 Indexing, Tracks, And Geocoding Update

- Supported discovery extensions are centralized in `internal/storage` and surfaced through runtime setting `indexing.supported_extensions`. The default covers photos, videos, GPS/KML/KMZ/GPX, audio, and lightweight document/text formats.
- Normal discovery/indexing jobs use `max_files=-1` and `max_bytes=-1` as explicit unlimited sentinels. Real archive storage still requires a named storage and non-root prefix; omitted/zero limits remain invalid for guarded real-archive flows.
- Dry-run/preview discovery keeps conservative caps and reports that those caps are preview-only.
- Track rendering now has a shared OpenLayers style path for direction arrows. Runtime setting `gps.track_arrow_interval_m` controls spacing, defaults to `500`, and `0` disables arrows.
- Reverse geocoding is cache-first. `/api/v1/places/reverse` resolves coordinates against durable `place_cache` bounding boxes and nearby centroids, then uses the configured online provider only when no provider-backed reverse-geocode row already covers the coordinate. Broad local seed matches are kept and returned, but they do not block one unique online cache-fill result. Online cache-fill is enabled by default, rate-limited, and every provider result is persisted back to `place_cache` for offline reuse.
- The current map clustering implementation remains screen-distance based. Persisted zoom-level cluster cache endpoints are still planned; frontend cluster state should continue to replace stale layer data on zoom changes.

## 2026-06-09 Metadata Context And Timestamp Candidates

Cartolensia now derives bounded timestamp candidates for media records instead of relying only on a trusted `taken_at` column. Candidate sources are ranked by confidence:

- trusted `taken_at`;
- EXIF `exif_datetime_original_raw`, interpreted in the runtime/default local timezone when EXIF is timezone-less;
- filename timestamps for common phone patterns such as `PXL_YYYYMMDD_HHMMSS` and `VID_YYYYMMDD_HHMMSS`;
- file mtime as a low-priority fallback.

GPS track media lookup uses those candidates plus geotag proximity, so photos with raw timezone-less EXIF and videos without GPS can still be associated with a track when metadata indicates they were created during or near that track. This is a metadata-only association and never writes back EXIF or changes originals.

Asset context is exposed through `GET /api/v1/assets/{id}/related`. The current implementation is a bounded PostgreSQL/local query service that returns grouped related assets by folder, device metadata, same local day, a short time window, and GPS-track overlap. It is intentionally not a separate graph database yet; the response gives enough related records for asset-detail navigation while avoiding unbounded scans.

Universal Search still uses the `postgres_local` backend. Plain searches now page through all candidate assets before applying local metadata/OCR/place matching, while explicit tokens such as `ext:`, `kind:`, `filename:`, and `path:` can narrow the initial asset query. Space-separated tokens are AND, comma-separated alternatives are OR within a token, and wildcard matching is supported for filename/path/text fields.

## 2026-06-27 Production Vector And AI Execution Update

The vector layer remains abstracted, but PostgreSQL/pgvector is now the preferred large-archive local backend when the `vector` extension is available. Embeddings are stored in two forms:

- `embedding_json` remains the portable JSON representation;
- `embedding_vector vector(512)` is populated for OpenCLIP ViT-B/32 embeddings and indexed with `ivfflat`.

When pgvector is unavailable, Cartolensia falls back to the previous bounded JSON cosine search. `/api/v1/vector/status` reports which backend is active.

Production AI execution is split from the main service:

- the Go backend owns authentication, media authorization, jobs, and PostgreSQL persistence;
- the Python sidecar owns model loading and local inference;
- the sidecar reads media through a loopback-only tokenized media URL, not through user cookies;
- model caches live under the configured model directory, typically `/var/lib/cartolensia/models`;
- long archive-wide AI work is driven by small missing-work batches rather than one monolithic job.

For large NAS-backed deployments, discovery and metadata enrichment remain the first pipeline stages. AI/OCR/ASR/caption/embedding work is opt-in, resumable by metadata state, and should be tuned with conservative batch sizes.

## 2026-06-27 Explorer Scale Path

Explorer folder mode now has a PostgreSQL implementation for production stores. Instead of loading every asset and grouping paths in the application, the store computes:

- immediate child folders with aggregate counts/bytes/latest mtime;
- direct file count and bytes for the current path;
- a paged direct-file result set using `limit`/`offset`.

Supporting indexes are added in migration `017_explorer_scale_indexes.sql`, including path-pattern indexes for prefix scans and sort-specific indexes for common folder orders. The in-memory catalog grouping remains as a fallback for non-PostgreSQL stores and tests.

The WebUI consumes this as a paged folder API. It renders the first page, shows the total, and asks for additional pages only when requested. This prevents a single folder with thousands of media files from freezing the browser or forcing a catalog-wide backend scan.

## 2026-06-27 Overlap-Safe Multi-Storage Discovery

Production NAS deployments may configure both a broad parent share and narrower child shares. For example, a parent storage may point at `old_drives/compressed_data` while separate child storages point at `old_drives/compressed_data/x12_los20` or `old_drives/compressed_data/Nokia5228`.

Discovery now handles that shape explicitly:

- the storage registry remains a list of independent strict-read-only filesystem adapters;
- before scanning each storage, discovery compares filesystem roots and computes nested child roots relative to the parent;
- parent scans add automatic subtree exclusions such as `x12_los20/**`;
- the bounded filesystem walker prunes excluded directories before descending into them;
- child storages are still scanned through their own storage names, preserving stable storage URLs and avoiding duplicate parent-path records.

This applies to `storage=all` jobs and to direct parent storage scans. It is intentionally metadata-only: unavailable child storage does not delete metadata, does not mark files missing, and does not write to originals. If an operator removes a child storage from config, future parent scans can cover that subtree normally.

The implementation keeps dry-run/preview caps separate from normal discovery. For normal production indexing, `max_files=-1` and `max_bytes=-1` are explicit unlimited sentinels. Missing-file marking remains disabled.

## Read-Only Search Query Layer

Cartolensia now exposes a local research-query layer on top of PostgreSQL instead of requiring a separate Elasticsearch/OpenSearch service for large personal archives.

The normal Universal Search path remains `/api/v1/search` with the `postgres_local` backend. It returns a parsed search plan so the UI can show how a query was interpreted. SQL-like user input such as:

```text
kind = video and ext = mp4 and caption contains "train"
```

is translated to safe Cartolensia search tokens. Arbitrary SQL from this path is never executed.

For advanced diagnostics and local research workflows, PostgreSQL migration `018_readonly_search_views.sql` creates curated views named `cartolensia_search_*`. The views expose denormalized, read-only projections of assets, locations, AI predictions, tags, transcripts, transcript segments, documents, video captions, audio features, tracks, and places.

`POST /api/v1/search/sql` accepts only a single `SELECT` against those views. The database runner:

- rejects semicolons and comments;
- rejects mutation and session-control keywords;
- rejects references to raw tables or unknown views;
- runs in a PostgreSQL read-only transaction;
- applies a statement timeout;
- wraps the query with a server-side row limit.

This gives the future local LLM planner a safe tool to execute planned queries without turning it into a database administrator. Any model-generated SQL must pass through the same allowlist and timeout.

The current natural-language planner is a deterministic local fallback for English/Russian requests. A future model-backed planner should run only against a local sidecar/model endpoint, should never call remote APIs by default, and should unload after an idle timeout so background AI workers can reclaim VRAM.

## Knowledge Base And Knowledge Graph

Cartolensia now has a local PostgreSQL-backed Knowledge Base/Knowledge Graph layer. It is not a replacement for canonical archive metadata; it is a human-readable, queryable projection mined from existing local facts.

Schema:

- `knowledge_facts` stores subject/predicate/object facts with source kind, source id, optional asset id, confidence, language, evidence, and JSON metadata.
- `knowledge_relations` stores graph edges between assets and/or named entities such as folders, devices, tags, transcripts, document text, audio features, and GPS tracks.
- `knowledge_conversations` and `knowledge_messages` store local chat history and tool-call traces.
- `cartolensia_search_knowledge_facts` and `cartolensia_search_knowledge_relations` expose read-only projections for the search SQL workbench and future local LLM tools.

Extraction is idempotent and bounded. Stable deterministic fact/relation IDs let repeated extraction update existing records instead of duplicating them. The current extractor mines explicit metadata only: media kind, storage location, device metadata, timestamps, geotags, tags, AI predictions, captions, OCR text, transcripts, document text, audio features, track summaries, and known track links.

The Knowledge chat API is tool-first. It always has a deterministic local English/Russian planner and can optionally call a local LLM through Ollama or an OpenAI-compatible/vLLM endpoint. In local LLM mode the model can request only allowlisted tools: bounded media search, knowledge fact search, knowledge relation search, and guarded read-only SQL against `cartolensia_search_*` views. The backend validates and executes every tool call; the model never gets database credentials or write-capable access. Remote LLM APIs are not used by default.

For direct media retrieval requests such as "find", "list", "show", or
"count" matching files, Cartolensia renders the final answer from verified tool
results even when local LLM mode is enabled. The model may help plan tools, but
it cannot replace a concrete search/count answer with free-form prose. This
keeps result counts, filenames, and action cards grounded in read-only
PostgreSQL views and avoids schema/capability essays.

Interactive chat uses `POST /api/v1/knowledge/chat/stream`, an authenticated
Server-Sent Events endpoint layered over the same planner and tool runner. The
stream separates progress (`status`), tool execution (`tool`), generated local
model text (`token`), and the compact final response (`final`). The compact
final payload keeps large fact, transcript, OCR, and relation lists from being
returned wholesale to the browser.

The chat request can include bounded text and image attachments. Text
attachments are summarized into the prompt. Image data URLs are forwarded only
to Ollama-compatible models that support image input; if the configured local
model rejects images, the server retries with text/filename context instead of
failing the whole answer. Attachments do not create originals-side sidecars and
are not persisted as archive files by the current implementation.

Design boundaries:

- extracted facts remain reviewable metadata, not ground truth;
- the graph is stored in PostgreSQL, not a separate graph database;
- no extraction step modifies originals;
- no model-generated SQL may bypass the existing read-only SQL guard.

## Durable AI Backfill

Production AI processing is now worker-owned instead of browser-session-owned. Operators can enqueue `ai_backfill` jobs that repeatedly select missing metadata in bounded batches, call the configured AI sidecar through Cartolensia read-only media URLs, and persist progress in PostgreSQL.

The selector is metadata-aware. It skips assets that already have durable outputs such as predictions, captions, OCR blocks, face detections, embeddings, transcripts, or audio features. It also records zero-result successes in `ai_asset_task_status`, so assets with no text or no faces are not retried forever. Failed assets are recorded with the task and error and can be retried by a later scoped run.

The PostgreSQL path uses task-specific queries with pagination and media-kind/duration filters. The in-memory store implements the same contract for tests. This keeps large NAS deployments resumable and prevents a single API request from needing to hold a whole archive-wide target list in memory.

Backfill jobs remain non-destructive:

- originals are read only through authenticated local media URLs;
- outputs are database metadata only;
- no sidecars are written beside original media;
- cancellation is checked between assets and batches;
- concurrency is controlled by the normal persistent worker pool.
