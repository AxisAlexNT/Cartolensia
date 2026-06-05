# Target Architecture

Cartolensia is a Go backend, PostgreSQL database, and Vue 3 WebUI for indexing and browsing very large photo, video, and GPS-track archives without modifying original files by default.

## MVP Boundary

The first runnable MVP is a vertical slice, not the full product:

- native Go service with configuration, migrations, storage registry, jobs, discovery, hashing, REST API, and static WebUI serving;
- PostgreSQL-backed state with PostGIS and pgvector treated as optional capabilities;
- strict read-only local filesystem storage adapter only;
- fixture-first development using `testdata/media_fixture/`;
- Vue 3 + TypeScript Explorer, Jobs, and Settings shell;
- heavy features represented by interfaces and manifest-discovered stub plugins.

## Runtime Shape

Minimum runtime:

- `cartolensia` Go binary;
- PostgreSQL 16 or compatible PostgreSQL service;
- local filesystem storage root configured in strict read-only mode.

Recommended development runtime:

- Docker Compose PostgreSQL service;
- native Go backend;
- Vite dev server for the WebUI.

Optional future runtime services:

- sidecar HTTP/gRPC plugins;
- AI worker containers;
- map tile cache/server;
- transcoding workers;
- object storage or network filesystem backends.

## Backend Modules

Planned package ownership under `internal/`:

- `app`: application bootstrap and dependency wiring.
- `config`: YAML and environment configuration loading.
- `database`: PostgreSQL connection, migrations, and capability detection.
- `storage`: storage URL parsing, registry, adapters, and read-only enforcement.
- `plugins`: plugin manifest loading, validation, and dependency sorting.
- `jobs`: persistent job queue and worker leases.
- `discovery`: fast storage scan and metadata indexing.
- `media`: MIME, metadata, preview, streaming, and hashing helpers.
- `server`: REST API, static WebUI serving, and HTTP middleware.
- `webui`: build embedding boundary if the backend later embeds frontend assets.

## Frontend Modules

Planned WebUI areas under `webui/src/`:

- app shell and routing;
- API client;
- Explorer view;
- Jobs/status view;
- Settings/storage view;
- plugin extension registry.

The MVP UI should compile while backend features are incomplete. Missing backend features should render explicit unavailable states, not blank pages.

## Data Flow

1. Config loads storages, database URL, enabled plugins, HTTP binding, and safety mode.
2. Database migrations establish core tables.
3. Plugin manifests are loaded and dependency-sorted.
4. Storage registry creates adapter instances.
5. Discovery job scans configured storage roots with no writes to originals.
6. File metadata is stored as assets and storage locations.
7. Hashing jobs stream file contents and attach SHA-512 content keys.
8. REST API serves assets, folders, jobs, status, and streamed originals.
9. WebUI displays explorer and job progress from backend state.

## Safety Defaults

- Original media is immutable by default.
- `strict_read_only` is the default storage mode.
- `/mnt/Models/rclone` must not be touched until an explicit later phase.
- Discovery must tolerate invalid, partial, and unsupported files.
- SHA-512 is an indexed content key, not the only logical identity.
- No delete, rename, write, transcode, or trash operation belongs in the MVP.

## Extension Strategy

MVP plugins are built into the Go binary and discovered through manifests. The manifest loader and WebUI registry should be real, but plugin behavior can be stubbed until the core vertical slice works.

Go `.so` plugins are not part of the main architecture. Sidecar HTTP plugins are the next planned runtime once core contracts are stable.
