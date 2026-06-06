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

- DB-backed sustained worker stress tests beyond fixture integration tests. Partially implemented via `scripts/worker-stress-test.sh`; DB stress remains gated by environment variables.
- Rescan/missing-file state transitions for explicitly bounded scopes.
- Real map tiles. OpenLayers vector rendering is implemented; OSM base tiles can be fetched on demand through a local Cartolensia tile cache/proxy, but packaged offline tile sets remain future work.

## Phase 2: Media Usability

- Original streaming with robust Range and HEAD support. Implemented for read-only filesystem storage.
- Image previews generated on demand and by queued job into a cache outside originals. Implemented for Go-decodable images.
- Persistent preview-cache index/table and status/cleanup APIs. Implemented.
- Metadata enrichment job for image dimensions, JPEG EXIF metadata/GPS, ffprobe video metadata, and GPX analysis. Implemented.
- Job dashboard APIs and WebUI controls for stats/detail/logs/cancel/retry. Implemented.
- Explorer/asset list filtering, sorting, pagination headers. Implemented; PostgreSQL asset queries use DB-backed filtering for common cases.
- Missing-file detection and rescan behavior. Future work.
- Video poster generation. Future work.

## Phase 3: Map And Tracks Foundation

- Virtual Albums plugin MVP. Implemented.
- Track ingestion from GPX, KML, and KMZ. Implemented.
- Track analysis with bbox, distance, duration, elevation, and simplification. Implemented.
- GPS Track Manager plugin MVP with detail, point queries, media lookup, offset control, and snap-media job. Implemented.
- Photo/map plugin MVP with GeoJSON APIs, album/media/source/track filters, deterministic point clustering, clickable cluster/point popups, typed `asset_geo`, and OpenLayers WebUI. Implemented.
- Live video-track sync candidates, manual links, deletion, and marker interpolation API. Implemented.
- On-demand OSM tile proxy/cache with attribution and no public-OSM bulk prefetch. Implemented.
- Richer manual geotag editing. Future work.

## Phase 4: Plugin Expansion

- Sidecar HTTP plugin manifest contract and health stub. Implemented.
- WebUI plugin asset mounting.
- Plugin-specific settings backed by PostgreSQL. Implemented as a core settings table; plugin-specific UI is future work.
- Capability and health reporting per plugin. Implemented as core/built-in status and sidecar stub.

## Phase 5: Transcoding And AI Foundations

- Transcoding preset/output schema contracts. Implemented.
- ffmpeg/ffprobe encoder and hardware capability detection. Implemented.
- Cache-scoped HLS transcode session MVP. Implemented; browser compatibility and durable transcode job management remain future work.
- AI/vector status APIs and accelerator hints. Implemented without AI dependencies.
- Embedding schema contract without required pgvector. Implemented.
- Durable transcoding jobs, managed transcoding outputs, model execution, embeddings, and classification. Future work.

## Phase 6: Large Archive Hardening

- Incremental scanning at scale.
- Better duplicate workflows without destructive defaults. Report-only duplicate grouping by SHA-512+size is implemented; review/merge/trash workflows remain future work.
- Observability and admin status pages.
- Offline resource packaging.
- Scoped dry-run readiness for future tiny real-archive tests. Implemented with report-only API, scan-run records, guarded config/script examples, strict default limits, and real-archive backend guardrails.
- Temporary real-peek helper scripts for supervised bounded read-only indexing. Implemented.
- Performance tests on synthetic large datasets. Implemented as bounded scripts; more metrics remain future work.
