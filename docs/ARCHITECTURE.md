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
- `internal/discovery`: fast fixture-safe discovery and lazy SHA-512 hashing handlers with cancellation checks. Discovery intentionally avoids heavy media parsing; metadata enrichment is a separate job.
- `internal/metadata`: explicit metadata enrichment jobs for images, videos, and GPX tracks. Image dimensions use Go decoders, video metadata uses optional ffprobe, and GPX enrichment computes point counts, bbox, duration, distance, and elevation where possible.
- `internal/jobs`: job model, state transitions, counters, progress, logs, cancellation, leases, and retry scheduling.
- `internal/workers`: async worker loop, lease acquisition, heartbeats, panic recovery, and graceful stop.
- `internal/auth`: local admin bootstrap, password hashing, password rotation, session/API-token auth, token scopes, CSRF flow, persisted auth store contracts, and explicit `dev_no_auth` development mode.
- `internal/gpx`: dependency-free GPX parser, route/waypoint support, track analysis, haversine distance, and deterministic simplification.
- `internal/preview`: preview status, cache-key/path safety, cache cleanup, preview generation jobs, and JPEG preview generation for decodable images.
- `internal/media`: SHA-512 streaming hash and optional ffprobe video metadata detection/extraction.
- `internal/plugins`: built-in and filesystem plugin manifests, dependency topological sort, sidecar HTTP manifest validation, and plugin status/health stubs.
- `internal/transcoding`: bounded ffmpeg/ffprobe capability inventory, encoder parsing, and hardware hints. It does not transcode originals.
- `internal/server`: REST API, original streaming, cached preview serving, and WebUI static serving.

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
- `POST /api/v1/hash/start`
- `POST /api/v1/metadata/enrich/start`
- `POST /api/v1/previews/start`
- `GET /api/v1/assets`
- `GET /api/v1/assets/{id}`
- `GET /api/v1/explorer`
- `GET /api/v1/tracks`
- `GET /api/v1/tracks/{track_asset_id}`
- `GET /api/v1/sync/candidates?asset_id=...`
- `GET /api/v1/sync/links`
- `POST /api/v1/sync/links`
- `DELETE /api/v1/sync/links/{id}`
- `GET /api/v1/videos/{asset_id}/track-sync?time_ms=...`
- `GET /api/v1/map?bbox=minLon,minLat,maxLon,maxLat&zoom=...`
- `GET /api/v1/transcoding/status`
- `GET /api/v1/transcoding/capabilities`
- `GET /api/v1/transcoding/presets`
- `GET /api/v1/ai/status`
- `GET /api/v1/ai/accelerators`
- `GET /api/v1/vector/status`
- `GET /api/v1/stats`
- `GET /api/v1/backend/status`
- `GET /api/v1/media/{asset_id}/original`
- `GET /api/v1/media/{asset_id}/preview`

`POST /api/v1/discovery/start`, `POST /api/v1/hash/start`, `POST /api/v1/metadata/enrich/start`, and `POST /api/v1/previews/start` enqueue jobs and return quickly in the app runtime. The worker loop leases and executes queued jobs asynchronously. Tests can opt into a synchronous server dependency path for deterministic fixture checks.

Original streaming uses the read-only storage registry and `http.ServeContent`, which provides HTTP Range and HEAD support when the underlying file supports seeking. Preview generation decodes standard-library-supported image formats, writes cached JPEG previews under the configured Cartolensia cache directory, serves the generated preview, and never writes near originals. Unsupported formats return a clean JSON status.

## WebUI

The WebUI is Vue 3 + TypeScript + Vite with no CDN resources. It contains:

- app shell and navigation;
- Explorer table backed by `/api/v1/explorer`, including folder grouping and breadcrumbs;
- asset detail view backed by `/api/v1/assets/{id}`;
- Discovery page with scan and hash actions;
- Jobs page with counts, detail/log view, cancel, and retry controls;
- Metadata page with explicit enrichment and preview generation actions;
- GPS Tracks page backed by parsed GPX track points and enriched distance/duration metadata;
- Map page backed by GeoJSON from the map API with simple SVG rendering and point clustering status;
- Storages page;
- Plugins page and plugin detail health/status surface;
- Stats page;
- Settings page for password rotation and API token management;
- stub/control surfaces for Albums, Transcoding, Base AI, and AI Classification.

Browser route state is saved in `localStorage`.

## Safety Boundary

- The default storage mode is `strict_read_only`.
- Originals are immutable.
- The filesystem adapter exposes read/list/stat/open only.
- Write, delete, move, mkdir, and similar operations return explicit read-only errors.
- Path traversal and absolute paths are rejected before filesystem access.
- `..` path segments are rejected before cleaning, including encoded URL traversal attempts.
- Symlinks are skipped during recursive discovery and opening a symlink that escapes the root is rejected.
- Write-like endpoints pass through auth and authorization hooks; `dev_no_auth` is the default fixture mode.
- `local` auth mode requires configured admin email and a password supplied through an environment variable or ignored bootstrap file. No production password is hardcoded.
- Cookie-authenticated write requests require a CSRF header obtained from `GET /api/v1/auth/csrf`. Bearer API tokens bypass CSRF but must carry sufficient scopes.
- API token scopes currently include `read`, `write`, `jobs:write`, `plugins:write`, `media:read`, and `admin`.
- `/mnt/Models/rclone` is not required and was not touched by the MVP tests.

## Database Capability Policy

PostGIS, pgvector, and pg_trgm are detected at startup. PostGIS may be installed by the development Docker image. pgvector and pg_trgm are optional for the MVP. Missing optional extensions do not block core startup.

## Future Interfaces

- Vector search should be implemented through a `VectorStore` abstraction. The schema stores JSON embedding placeholders without requiring pgvector.
- Sidecar HTTP plugins are represented in manifests but are user-managed services; Cartolensia does not auto-start arbitrary plugin binaries.
- Live video-track sync is represented in schema by `track_points` and `asset_track_links`, including `time_offset_ms`, with a marker interpolation API for linked videos.
- Transcoding contracts and capability detection exist, but transcoding jobs and outputs are still future work.
