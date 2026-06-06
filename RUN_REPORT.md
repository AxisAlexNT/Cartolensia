# Run Report

## 2026-06-06 Phase 1/2 Slice Run

### Implemented Features

- Local auth persistence and bootstrap:
  - local admin bootstrap from config plus password env;
  - no hardcoded production admin password;
  - persisted users, sessions, and API tokens in PostgreSQL;
  - memory auth store for unit tests and fixture workflows;
  - login, logout, current principal, token create/list/revoke APIs;
  - explicit `dev_no_auth` mode retained;
  - write-like endpoints stay protected when auth is enabled;
  - WebUI login/logout state for local auth mode.
- Job retry/error classification:
  - permanent, transient, canceled, panic, and unknown error classes;
  - permanent storage/config/safety failures do not retry-spin;
  - worker panic recovery records failed jobs instead of crashing the worker;
  - retry behavior covered in memory-store tests.
- GPX and track MVP:
  - dependency-light GPX parser;
  - discovery parses GPX files from configured read-only storage;
  - track points are stored in memory/PostgreSQL stores;
  - `GET /api/v1/tracks` and `GET /api/v1/tracks/{track_asset_id}`;
  - synthetic GPX fixture coverage in unit/server/DB tests.
- Live video-track sync skeleton:
  - candidate matching by overlapping time spans;
  - manual `time_offset_ms` saved in `asset_track_links`;
  - `GET /api/v1/sync/candidates`;
  - `GET/POST /api/v1/sync/links`.
- Map API/UI skeleton:
  - `GET /api/v1/map?bbox=minLon,minLat,maxLon,maxLat&zoom=...`;
  - returns simple GeoJSON for track lines and geotagged asset points;
  - clustering is explicitly reported as `not_implemented`;
  - WebUI Map page can display the returned GeoJSON.
- Preview generation MVP:
  - no permanent sidecars;
  - generated previews live only under the Cartolensia cache directory;
  - cache path is derived from asset/content identity, not raw user paths;
  - built-in image preview generation for Go-supported image formats;
  - unsupported formats return clean API statuses;
  - cache path safety covered by tests.
- ffprobe metadata extraction:
  - ffprobe detection exposed in backend status;
  - discovery attempts best-effort video duration/resolution enrichment;
  - discovery does not fail when ffprobe is unavailable or cannot parse a fixture.
- Migration/schema upkeep:
  - auth persistence migration is in place;
  - added forward-only lease index migration for `running` and `cancel_requested` jobs.

### Commands Run

- Read and inspected project docs and code with `sed`, `rg`, `rg --files`, `git status`, and `git diff`.
- `gofmt -w internal/app/app.go internal/auth/auth.go internal/auth/memory.go internal/auth/auth_test.go internal/catalog/catalog.go internal/catalog/catalog_test.go internal/config/config.go internal/database/database.go internal/database/integration_test.go internal/discovery/discovery.go internal/gpx/gpx.go internal/gpx/gpx_test.go internal/jobs/jobs.go internal/media/ffprobe.go internal/media/hash.go internal/media/hash_test.go internal/preview/preview.go internal/preview/preview_test.go internal/server/server.go internal/server/server_test.go internal/workers/workers.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...` passed.
- `go test ./...` passed.
- `npm --prefix webui run build` passed.
- `bash scripts/smoke-test.sh` passed.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config` passed.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres` started the dev PostgreSQL/PostGIS service.
- `bash scripts/test-db.sh` passed against the dev PostgreSQL service.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml stop postgres` stopped the dev PostgreSQL/PostGIS service.
- `git diff --check` passed.

### Tests Passed

- Go unit and fixture integration tests:
  - `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
  - `go test ./...`
- Frontend:
  - `npm --prefix webui run build`
- Local smoke:
  - `bash scripts/smoke-test.sh`
- Docker/PostgreSQL:
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
  - `bash scripts/test-db.sh`

### Failures And Fixes

- A diagnostic `sed` command referenced stale migration filenames (`002_app_settings.sql`, `003_job_leases.sql`, `004_auth.sql`) and failed; reran inspection against the actual migration files (`002_phase1_hardening.sql`, `003_auth_foundation.sql`, `004_job_lease_cancel_index.sql`).
- A stale report entry said preview generation and auth flows were still unimplemented; this run replaced those statements with current status.
- The broad root `media/` ignore rule was hiding `internal/media`; `.gitignore` now explicitly unignores `internal/media/**`.
- Asset detail preview status previously reported photos as `not_implemented` even though the preview endpoint can generate them; it now reports `queued` or `ready` from the cache state.
- The documented cancel-requested lease index could be skipped because an older same-name index already existed; a forward migration now drops/recreates the index with the intended predicate.

### Known Limitations

- Local auth is admin-only for now; OIDC/OAuth remains a disabled future stub.
- API token scopes are stored and exposed but authorization is still role-based admin gating.
- Preview generation supports only image formats available through Go image decoders; video previews remain `not_implemented`.
- Map output is simple GeoJSON; no real tile renderer, PostGIS geometry, or clustering yet.
- GPX support covers common track point fields only.
- ffprobe enrichment is best-effort and not yet a scheduled metadata job.
- DB concurrency tests cover lease ownership behavior, but not sustained multi-process worker load.
- `webui/dist` was generated by verification and is ignored by git.
- Docker state was used only for the dev PostgreSQL integration test; the container was stopped afterward and the named volume was preserved.

### Changed Files Summary

- Backend: app auth wiring, config, local auth service/store, catalog track/sync methods, PostgreSQL auth/track/sync methods, discovery GPX/metadata enrichment, job error classes, preview generation, server routes, worker panic classification.
- New backend packages/files: `internal/gpx`, `internal/media/ffprobe.go`, auth tests/store, preview tests.
- Frontend: API client types/methods, login/logout state, GPS Tracks page, Map page, auth/topbar styling.
- Database: `migrations/004_job_lease_cancel_index.sql` and schema docs.
- Config/docs: auth env/config options, architecture/schema/storage/roadmap updates, `.gitignore` fix for `internal/media`.

### `/mnt/Models/rclone`

Skipped entirely. It was not read, written, listed, scanned, mounted, or required for tests.

### Exact Recommended Next Prompt

```text
Continue from the current Cartolensia repo. Read AGENTS.md, README.md, RUN_REPORT.md, docs/ARCHITECTURE.md, docs/DB_SCHEMA.md, docs/STORAGE_MODEL.md, docs/PLUGIN_MODEL.md, docs/ROADMAP.md, and docs/PHASE_1_HARDENING_PLAN.md. Build the next useful slice: harden local auth with CSRF/session cleanup/password rotation, add real token-scope authorization, add DB-backed worker concurrency stress tests, implement a metadata enrichment job for GPX/ffprobe, add a track detail/map visualization in WebUI, add image preview UI integration, and improve map bbox filtering/pagination. Keep using testdata/media_fixture only. Do not touch /mnt/Models/rclone. Run GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./..., go test ./..., npm --prefix webui run build, bash scripts/smoke-test.sh, docker compose config, and DB integration tests if Docker/PostgreSQL is available. Update RUN_REPORT.md honestly and do not push.
```
