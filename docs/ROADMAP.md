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

Remaining Phase 1 hardening:

- asynchronous worker loop with durable leases;
- richer PostgreSQL integration tests;
- auth interface stub;
- embedded migrations via Go `embed` instead of filesystem migration loading.

## Phase 2: Media Usability

- Original streaming with robust Range support.
- Image previews generated on demand into a cache outside originals.
- ffprobe metadata extraction where available.
- Missing-file detection and rescan behavior.
- Improved folder and table filtering.

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
