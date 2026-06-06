# Architecture

The implemented MVP is a Go backend, PostgreSQL-capable metadata store, strict read-only filesystem storage adapter, and Vue 3 + TypeScript WebUI.

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
- `internal/discovery`: fast fixture-safe discovery and lazy SHA-512 hashing handlers with cancellation checks.
- `internal/jobs`: job model, state transitions, counters, progress, logs, cancellation, leases, and retry scheduling.
- `internal/workers`: async worker loop, lease acquisition, heartbeats, panic recovery, and graceful stop.
- `internal/auth`: local admin bootstrap, session/API-token auth, persisted auth store contracts, and explicit `dev_no_auth` development mode.
- `internal/gpx`: dependency-free GPX track point parser.
- `internal/preview`: preview status, cache-key/path safety, and JPEG preview generation for decodable images.
- `internal/media`: SHA-512 streaming hash and optional ffprobe video metadata detection/extraction.
- `internal/plugins`: built-in plugin manifests and dependency topological sort.
- `internal/server`: REST API, original streaming, cached preview serving, and WebUI static serving.

## REST API

Implemented endpoints:

- `GET /api/v1/health`
- `GET /api/v1/version`
- `GET /api/v1/config/effective`
- `GET /api/v1/auth/me`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/tokens`
- `POST /api/v1/auth/tokens`
- `DELETE /api/v1/auth/tokens/{id}`
- `GET /api/v1/storages`
- `GET /api/v1/plugins`
- `POST /api/v1/plugins/rescan`
- `GET /api/v1/jobs`
- `POST /api/v1/jobs/{id}/cancel`
- `POST /api/v1/discovery/start`
- `POST /api/v1/hash/start`
- `GET /api/v1/assets`
- `GET /api/v1/assets/{id}`
- `GET /api/v1/explorer`
- `GET /api/v1/tracks`
- `GET /api/v1/tracks/{track_asset_id}`
- `GET /api/v1/sync/candidates?asset_id=...`
- `GET /api/v1/sync/links`
- `POST /api/v1/sync/links`
- `GET /api/v1/map?bbox=minLon,minLat,maxLon,maxLat&zoom=...`
- `GET /api/v1/stats`
- `GET /api/v1/backend/status`
- `GET /api/v1/media/{asset_id}/original`
- `GET /api/v1/media/{asset_id}/preview`

`POST /api/v1/discovery/start` and `POST /api/v1/hash/start` enqueue jobs and return quickly in the app runtime. The worker loop leases and executes queued jobs asynchronously. Tests can opt into a synchronous server dependency path for deterministic fixture checks.

Original streaming uses the read-only storage registry and `http.ServeContent`, which provides HTTP Range support when the underlying file supports seeking. Preview generation decodes standard-library-supported image formats, writes cached JPEG previews under the configured Cartolensia cache directory, serves the generated preview, and never writes near originals. Unsupported formats return a clean JSON status.

## WebUI

The WebUI is Vue 3 + TypeScript + Vite with no CDN resources. It contains:

- app shell and navigation;
- Explorer table backed by `/api/v1/explorer`, including folder grouping and breadcrumbs;
- asset detail view backed by `/api/v1/assets/{id}`;
- Discovery page with scan and hash actions;
- GPS Tracks page backed by parsed GPX track points;
- Map page backed by GeoJSON from the map API;
- Storages page;
- Plugins page;
- Stats page;
- stub surfaces for Albums, Map, GPS Tracks, Transcoding, Base AI, and AI Classification.

Browser route state is saved in `localStorage`.

## Safety Boundary

- The default storage mode is `strict_read_only`.
- Originals are immutable.
- The filesystem adapter exposes read/list/stat/open only.
- Write, delete, move, mkdir, and similar operations return explicit read-only errors.
- Path traversal and absolute paths are rejected before filesystem access.
- `..` path segments are rejected before cleaning, including encoded URL traversal attempts.
- Symlinks are skipped during recursive discovery and opening a symlink that escapes the root is rejected.
- Write-like endpoints pass through an auth hook; `dev_no_auth` is the default fixture mode.
- `local` auth mode requires configured admin email and a password supplied through the configured password environment variable. No production password is hardcoded.
- `/mnt/Models/rclone` is not required and was not touched by the MVP tests.

## Database Capability Policy

PostGIS, pgvector, and pg_trgm are detected at startup. PostGIS may be installed by the development Docker image. pgvector and pg_trgm are optional for the MVP. Missing optional extensions do not block core startup.

## Future Interfaces

- Vector search should be implemented through a `VectorStore` abstraction.
- Sidecar HTTP/gRPC plugins are the intended future plugin runtime.
- Live video-track sync is represented in schema by `track_points` and `asset_track_links`, including `time_offset_ms`.
