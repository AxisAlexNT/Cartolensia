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

- richer asset metadata extraction;
- preview generation workers for videos and unsupported image formats;
- richer map rendering with real map tiles/OpenLayers;
- durable job dashboard filters and retry controls.

## Phase 2: Media Usability

- Original streaming with robust Range support.
- Image previews generated on demand into a cache outside originals.
- ffprobe metadata extraction where available.
- Missing-file detection and rescan behavior.
- Improved folder and table filtering beyond the current folder MVP.

## Phase 3: Map And Tracks Foundation

- Track ingestion.
- Basic map view with OpenLayers.
- Geotag display and inferred-location flags.
- Time filters and track/media overlap queries.

## Phase 4: Plugin Expansion

- Sidecar HTTP plugin contract.
- WebUI plugin asset mounting.
- Plugin-specific settings backed by PostgreSQL.
- Capability and health reporting per plugin.

## Phase 5: Transcoding And AI Foundations

- Transcoding job model and preset storage.
- Hardware capability detection.
- AI runtime inventory and worker contract.
- Embedding and classifier interfaces.

## Phase 6: Large Archive Hardening

- Incremental scanning at scale.
- Better duplicate workflows without destructive defaults.
- Observability and admin status pages.
- Offline resource packaging.
- Performance tests on synthetic large datasets.
