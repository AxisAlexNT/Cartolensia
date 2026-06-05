# MVP Implementation Plan

This plan defines the next unattended implementation target. The goal is a runnable vertical-slice MVP that indexes a tiny local fixture and proves the backend, database, job, storage, API, and WebUI foundations.

## Engineering Assumptions

- Backend is Go 1.22+.
- Frontend is Vue 3 + TypeScript + Vite, using npm for the initial workflow.
- PostgreSQL 16 is the development database. PostGIS is optional at runtime but should be detected when present.
- The MVP uses only local filesystem storage in `strict_read_only` mode.
- The first storage root is `testdata/media_fixture/`; do not touch `/mnt/Models/rclone`.
- Original files are never modified, renamed, deleted, chmodded, transcoded into, or used as cache targets.
- Heavy product areas are represented by stable interfaces and documented stubs until the vertical slice is working.

## MVP Deliverable

By the end of the MVP run, `make smoke` should prove:

- backend packages compile and tests pass;
- migrations can apply to a local PostgreSQL database;
- a discovery job can scan `testdata/media_fixture/`;
- discovered files are visible through a REST endpoint;
- a hash job can stream SHA-512 for discovered files;
- the WebUI builds and shows Explorer, Jobs, and Settings surfaces backed by API calls or explicit stub states.

## Exact Build Order

1. Config foundation
   - Implement `internal/config`.
   - Load YAML config file path from `CARTOLENSIA_CONFIG` or default `config/cartolensia.yaml`.
   - Allow environment overrides for HTTP address and database URL.
   - Validate storage names, storage roots, and safety modes.
   - Add unit tests for defaults, env overrides, invalid modes, and missing required database URL.

2. Database and migrations
   - Implement `internal/database`.
   - Use embedded SQL migrations from `migrations/`.
   - Store applied migrations in `schema_migrations`.
   - Detect extensions: `postgis`, `pgvector`, and `pg_trgm`; do not require them for MVP startup.
   - Add tests that migration ordering is deterministic and idempotent.

3. Storage registry and filesystem adapter
   - Implement `internal/storage`.
   - Parse and normalize universal URLs such as `fs://fixture/photos/photo_001.jpg`.
   - Implement strict path containment and traversal prevention.
   - Implement read, stat, list, and open operations.
   - Return explicit unsupported errors for write, delete, move, mkdir, chmod, and trash operations.
   - Add tests for traversal attempts, symlink/path normalization where feasible, URL normalization, and strict read-only behavior.

4. Plugin manifests
   - Implement `internal/plugins`.
   - Load manifests from `plugins/<id>/plugin.yaml` when present and built-in manifests from code.
   - Validate IDs, names, versions, dependencies, and duplicate IDs.
   - Topologically sort dependencies with cycle detection.
   - Add built-in stub manifests for `core.explorer`, `core.discovery`, `media.map`, `tracks.manager`, `video.transcode`, `ai.base`, and `ai.classification`.
   - Add tests for sorting, missing dependencies, duplicate IDs, and cycles.

5. Persistent jobs
   - Implement `internal/jobs`.
   - Store jobs in PostgreSQL with status, kind, payload, attempts, progress, log lines, timestamps, and worker lease fields.
   - Implement enqueue, lease, heartbeat, progress update, complete, fail, cancel requested, and list operations.
   - Add tests for valid state transitions and lease expiration behavior.

6. Discovery indexing
   - Implement `internal/discovery`.
   - Stage 1 scan captures storage URL, path, name, extension, MIME guess, size, mtime, and coarse media kind.
   - Store logical asset and storage location records without hashing first.
   - Use upserts so repeat scans are idempotent.
   - Never infer destructive duplicate actions from quick metadata.
   - Add fixture-based tests using only `testdata/media_fixture/`.

7. Lazy SHA-512 hashing
   - Implement streaming hash jobs in `internal/media` or `internal/discovery`.
   - Read through the storage adapter and never load whole files into memory.
   - Store SHA-512 on content records and link locations to content when known.
   - Add tests using generated fixture files and a larger temporary file.

8. REST API and server
   - Implement `internal/server`.
   - Required endpoints:
     - `GET /api/health`
     - `GET /api/version`
     - `GET /api/storages`
     - `GET /api/jobs`
     - `POST /api/jobs/discovery`
     - `POST /api/jobs/hash`
     - `GET /api/assets`
     - `GET /api/assets/{id}`
     - `GET /api/assets/{id}/original` with HTTP Range support when possible
   - Keep authentication stubbed behind an interface; do not build public sharing yet.
   - Add handler tests for health, job creation, asset list, and invalid IDs.

9. WebUI shell
   - Implement Vue routes for Explorer, Jobs, and Settings.
   - Use a typed API client.
   - Explorer should show folders/files, media kind, size, mtime, hash status, and storage URL.
   - Jobs should show status, progress, timestamps, and latest log lines.
   - Settings should show configured storages and extension availability.
   - Build should succeed without CDN resources.

10. Smoke workflow
    - Update `Makefile` targets as implementation grows.
    - `make smoke` should run backend tests, migrate against development PostgreSQL when configured, scan the fixture, and build the WebUI when dependencies are installed.
    - Update `RUN_REPORT.md` with commands, failures, fixes, limitations, and next task.

## Out Of Scope For MVP

- SMB, NFS, S3, WebDAV, upload, delete, move, trash, or replacement flows.
- Real AI inference, embeddings, vector search, face recognition, or classifier training.
- Album editing and sharing.
- Full map clustering or offline tile server.
- Video transcoding presets, VMAF, or hardware accelerator orchestration.
- Authentication beyond a clearly isolated interface/stub.
- Any access to `/mnt/Models/rclone`.

## Acceptance Checks

- `go test ./...` passes.
- `make check-tools` reports tool availability.
- `make smoke` passes or documents unavailable dependency reasons.
- Running the backend with fixture storage does not write outside PostgreSQL or local cache/temp paths.
- A repeated discovery scan of the fixture does not create duplicate logical assets.
- The WebUI build has no CDN dependencies.
- `RUN_REPORT.md` is updated at the end of the unattended run.
