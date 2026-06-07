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
- Universal Explorer search with match explanations across names, paths, extensions, date fragments, hash prefixes, metadata, tags, albums, and tracks. Implemented as an MVP endpoint/UI.
- Missing-file detection and rescan behavior. Future work.
- Video poster generation. Future work.

## Phase 3: Map And Tracks Foundation

- Virtual Albums plugin MVP. Implemented.
- Track ingestion from GPX, KML, and KMZ. Implemented.
- Track analysis with bbox, distance, duration, elevation, and simplification. Implemented.
- GPS Track Manager plugin MVP with detail, point queries, media lookup, offset control, and snap-media job. Implemented.
- Track click popups with nearest-point metadata, relative time/speed/elevation when available, time-based media lookup, and geotag proximity lookup. Implemented.
- Track previews and generated thumbnails for GPX/KML/KMZ/GPZ assets. Implemented with dark cached fallback rendering; OSM-backed thumbnail compositing remains future work.
- Photo/map plugin MVP with GeoJSON APIs, album/media/source/track filters, screen-distance clustering, count labels, clickable cluster/point/track popups, typed `asset_geo`, and OpenLayers WebUI. Implemented.
- Live video-track sync candidates, manual links, deletion, and marker interpolation API. Implemented.
- On-demand OSM tile proxy/cache with attribution and no public-OSM bulk prefetch. Implemented.
- Richer manual geotag editing. Future work.

## Phase 4: Plugin Expansion

- Sidecar HTTP plugin manifest contract and health stub. Implemented.
- WebUI plugin asset mounting.
- Plugin-specific settings backed by PostgreSQL and schema-based WebUI plugin settings tabs. Implemented as a generic settings UI; custom plugin-rendered components remain future work.
- Capability and health reporting per plugin. Implemented as core/built-in status and sidecar stub.

## Phase 5: Transcoding And AI Foundations

- Transcoding preset/output schema contracts. Implemented.
- ffmpeg/ffprobe encoder and hardware capability detection. Implemented.
- Cache-scoped HLS transcode session MVP. Implemented with built-in/custom preset metadata, browser HLS/hls.js playback path, and cache cleanup safety; durable transcode job management remains future work.
- AI/vector status APIs, accelerator hints, optional HTTP sidecar worker contract, Docker Compose worker profiles, and dummy not-configured service scaffold. Implemented without required AI dependencies or model downloads.
- Embedding schema contract without required pgvector. Implemented.
- Durable transcoding jobs, managed transcoding outputs, real model execution, embeddings, face grouping, and production classification workflows. Future work.

## Phase 6: Large Archive Hardening

- Incremental scanning at scale.
- Better duplicate workflows without destructive defaults. Report-only duplicate grouping by SHA-512+size is implemented; review/merge/trash workflows remain future work.
- Observability and admin status pages.
- Offline resource packaging.
- Scoped dry-run readiness for future tiny real-archive tests. Implemented with report-only API, scan-run records, guarded config/script examples, strict default limits, and real-archive backend guardrails.
- Temporary real-peek helper scripts for supervised bounded read-only indexing. Implemented.
- Performance tests on synthetic large datasets. Implemented as bounded scripts; more metrics remain future work.

## 2026-06-07 Progress

Completed in the latest implementation run:

- Track detail manager page with map, profile charts, stats, and media lookup actions.
- Runtime-safe storage add/validate API and UI.
- Conservative preview policy default: persistent preview generation is off unless explicitly enabled.
- Transcoding preset validation and hardware-test flow, including native NVENC dry-run success for H.264 on the current real-peek video.
- Packaged dummy AI sidecar and backend worker health probing.
- Universal search improvements for date ranges, album tokens, and track tokens.

Next priorities:

- Persist runtime storage changes durably.
- Add real AI model execution only after explicit model/dependency approvals.
- Add OSM-background track thumbnail rendering.
- Split large WebUI pages into smaller components.
- Add browser automation for gallery/transcoding/map interaction checks.
