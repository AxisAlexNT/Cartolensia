# Architecture

The implemented MVP is a Go backend, PostgreSQL-capable metadata store, strict read-only filesystem storage adapter, and Vue 3 + TypeScript WebUI.

## Runtime Modes

Cartolensia can currently run in two modes:

- `memory` store: default when no database URL is configured. This is useful for fixture smoke tests and development without PostgreSQL.
- `postgres` store: enabled by `database.url` or `CARTOLENSIA_DATABASE_URL`. Startup applies SQL migrations, snapshots effective config, stores configured storage backends, stores built-in plugin manifests, and detects optional database capabilities.

PostgreSQL is the intended durable mode. The memory store is a fallback and test aid, not the long-term production store.

## Backend Packages

- `internal/app`: startup wiring, config loading, storage registry, plugin loading, and database selection.
- `internal/config`: YAML configuration, environment overrides, defaults, validation, and absolute path normalization.
- `internal/database`: pgx-backed PostgreSQL connection, migrations, config snapshots, plugin/storage upserts, catalog store, jobs, logs, stats, and capability detection.
- `internal/storage`: universal `fs://storage/path` URLs, strict read-only filesystem adapter, traversal prevention, MIME/media-kind detection, and safe open/list/stat behavior.
- `internal/catalog`: logical assets, storage locations, content/hash status, stats, and store contract.
- `internal/discovery`: fast fixture-safe discovery and lazy SHA-512 hashing jobs.
- `internal/jobs`: job model, state transitions, counters, progress, and logs.
- `internal/plugins`: built-in plugin manifests and dependency topological sort.
- `internal/server`: REST API, original streaming, preview not-implemented response, and WebUI static serving.

## REST API

Implemented endpoints:

- `GET /api/v1/health`
- `GET /api/v1/version`
- `GET /api/v1/config/effective`
- `GET /api/v1/storages`
- `GET /api/v1/plugins`
- `POST /api/v1/plugins/rescan`
- `GET /api/v1/jobs`
- `POST /api/v1/discovery/start`
- `POST /api/v1/hash/start`
- `GET /api/v1/assets`
- `GET /api/v1/explorer`
- `GET /api/v1/stats`
- `GET /api/v1/backend/status`
- `GET /api/v1/media/{asset_id}/original`
- `GET /api/v1/media/{asset_id}/preview`

Original streaming uses the read-only storage registry and `http.ServeContent`, which provides HTTP Range support when the underlying file supports seeking. Preview generation intentionally returns `501 Not Implemented` in the MVP.

## WebUI

The WebUI is Vue 3 + TypeScript + Vite with no CDN resources. It contains:

- app shell and navigation;
- Explorer table backed by `/api/v1/explorer`;
- Discovery page with scan and hash actions;
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
- Symlinks are not followed during recursive discovery.
- `/mnt/Models/rclone` is not required and was not touched by the MVP tests.

## Database Capability Policy

PostGIS, pgvector, and pg_trgm are detected at startup. PostGIS may be installed by the development Docker image. pgvector and pg_trgm are optional for the MVP. Missing optional extensions do not block core startup.

## Future Interfaces

- Vector search should be implemented through a `VectorStore` abstraction.
- Sidecar HTTP/gRPC plugins are the intended future plugin runtime.
- Live video-track sync is represented in schema by `track_points` and `asset_track_links`, including `time_offset_ms`.
