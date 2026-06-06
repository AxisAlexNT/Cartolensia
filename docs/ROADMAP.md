# Roadmap

## Phase 0: Repository Preparation

- Normalize documentation paths.
- Add target architecture, storage, plugin, schema, and roadmap docs.
- Add Go, WebUI, Docker Compose, Makefile, scripts, environment example, and fixture scaffold.
- Record tool availability and unattended-run risks in `RUN_REPORT.md`.

## Phase 1: Core Vertical Slice

- Config loader. Implemented.
- PostgreSQL migrations and config snapshots. Implemented.
- Strict read-only filesystem storage. Implemented.
- Plugin manifest loader and dependency sort. Implemented.
- Persistent job queue in PostgreSQL and memory fallback. Implemented for synchronous jobs.
- Fixture discovery. Implemented.
- Streaming SHA-512 hashing. Implemented.
- REST API. Implemented for MVP endpoints.
- Vue Explorer, Discovery, Storages, Plugins, Stats, and plugin stubs. Implemented.
- Smoke workflow. Implemented.
- Embedded migrations with optional explicit disk loading. Implemented.
- Durable job leases, heartbeats, cancellation, retry scheduling, and panic recovery. Implemented.
- Async discovery/hash workers. Implemented.
- PostgreSQL integration tests gated by environment variables. Implemented.
- Folder-style Explorer and asset detail API/UI. Implemented.
- Local auth interfaces plus `dev_no_auth` mode. Implemented.
- Local auth bootstrap, login/logout, sessions, and API tokens. Implemented.
- Preview cache/status foundation. Implemented.
- Standard-library image preview generation. Implemented for decodable image files.
- GPX parser, track listing/detail APIs, and GPS Tracks UI. Implemented.
- Live video-track sync candidate/link skeleton. Implemented.
- GeoJSON map API and basic Map UI. Implemented.
- ffprobe detection and best-effort video metadata extraction. Implemented.

Remaining Phase 1 hardening:

- DB-backed sustained worker stress tests beyond fixture integration tests.
- Rescan/missing-file state transitions for explicitly bounded scopes.
- Richer map rendering with real map tiles/OpenLayers.
- Persistent preview-cache index/table if cache management needs cross-process introspection.

## Phase 2: Media Usability

- Original streaming with robust Range and HEAD support. Implemented for read-only filesystem storage.
- Image previews generated on demand and by queued job into a cache outside originals. Implemented for Go-decodable images.
- Metadata enrichment job for image dimensions, ffprobe video metadata, and GPX analysis. Implemented.
- Job dashboard APIs and WebUI controls for stats/detail/logs/cancel/retry. Implemented.
- Explorer/asset list filtering, sorting, pagination headers. Implemented over current store result sets.
- Missing-file detection and rescan behavior. Future work.
- Video poster generation. Future work.

## Phase 3: Map And Tracks Foundation

- Track ingestion from GPX. Implemented.
- Track analysis with bbox, distance, duration, elevation, and simplification. Implemented.
- Basic map GeoJSON API with bbox/time/media filters and deterministic point clustering. Implemented.
- Basic SVG map WebUI without external tile dependency. Implemented.
- Live video-track sync candidates, manual links, deletion, and marker interpolation API. Implemented.
- Real map tiles/OpenLayers and richer geotag editing. Future work.

## Phase 4: Plugin Expansion

- Sidecar HTTP plugin manifest contract and health stub. Implemented.
- WebUI plugin asset mounting.
- Plugin-specific settings backed by PostgreSQL.
- Capability and health reporting per plugin. Implemented as core/built-in status and sidecar stub.

## Phase 5: Transcoding And AI Foundations

- Transcoding preset/output schema contracts. Implemented.
- ffmpeg/ffprobe encoder and hardware capability detection. Implemented as inventory only.
- AI/vector status APIs and accelerator hints. Implemented without AI dependencies.
- Embedding schema contract without required pgvector. Implemented.
- Actual transcoding jobs, model execution, embeddings, and classification. Future work.

## Phase 6: Large Archive Hardening

- Incremental scanning at scale.
- Better duplicate workflows without destructive defaults.
- Observability and admin status pages.
- Offline resource packaging.
- Performance tests on synthetic large datasets. Implemented as bounded scripts; more metrics remain future work.
