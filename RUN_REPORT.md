# Run Report

## 2026-06-06 Phase 2/3 Foundation Run

### Start State

- Started from the current Cartolensia repository with existing Phase 1/2 scaffold.
- Initial `git status --short --untracked-files=all` showed a clean tracked state before this implementation run.
- Baseline checks passed before large edits:
  - `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
  - `go test ./...`
  - `npm --prefix webui run build`
  - `bash scripts/smoke-test.sh`
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
  - `bash scripts/test-db.sh` after starting the dev PostgreSQL container

### Implemented Features

- Auth/session hardening:
  - local admin bootstrap can read password from env or ignored bootstrap file;
  - bootstrap is idempotent and rotates only when explicitly configured;
  - no production admin password is hardcoded;
  - password change endpoint and WebUI control;
  - expired session cleanup and expired-session rejection tests;
  - configurable HttpOnly/SameSite/Secure session cookie behavior;
  - stateless CSRF token flow at `GET /api/v1/auth/csrf`;
  - CSRF enforcement for cookie-authenticated write requests;
  - scoped API token authorization for write-like endpoints.
- Job dashboard/control:
  - job detail, log pagination, stats, filters, cancel, and retry endpoints;
  - retry behavior remains conservative for permanent/safety failures;
  - WebUI Jobs page with stats, progress, detail/log panel, cancel, and retry controls.
- Metadata enrichment:
  - new `metadata_enrich` job and `POST /api/v1/metadata/enrich/start`;
  - image dimension extraction through Go decoders where supported;
  - ffprobe video metadata extraction remains best-effort;
  - GPX enrichment stores points and additive metadata such as bbox, distance, duration, and elevation.
- Preview integration:
  - new `preview_generate` job and `POST /api/v1/previews/start`;
  - preview cache cleanup verifies paths stay under the configured cache root;
  - WebUI preview controls and asset-detail preview display.
- GPX/track foundation:
  - GPX parser handles multiple tracks/segments, route points, waypoints, elevation, missing time, and multiple time layouts;
  - track analysis includes bbox, distance, duration, average-speed-compatible timing, and elevation min/max;
  - deterministic simplification helper for map output.
- Live video-track sync:
  - candidate responses include overlap duration and reason;
  - manual links can be deleted;
  - `GET /api/v1/videos/{asset_id}/track-sync?time_ms=...` returns interpolated marker positions for linked video/track pairs.
- Map API/UI:
  - map endpoint accepts bbox, zoom, media kind, time, selected assets/tracks, limit, and clustering flags;
  - simple deterministic grid clustering for point features;
  - track geometry is simplified for low-zoom/large responses;
  - WebUI Map page renders returned GeoJSON with a simple SVG view.
- Explorer/asset scale improvements:
  - flat `/api/v1/assets` and `/api/v1/explorer` support filtering, sorting, limit/offset, and `X-Total-Count` headers;
  - tests cover asset and explorer filtering behavior.
- Plugin foundation:
  - manifest runtime/status defaults are normalized;
  - sidecar HTTP manifest contract is validated;
  - plugin detail and health endpoints are exposed;
  - duplicate plugin IDs, dependency cycles, and invalid sidecar manifests are tested.
- Transcoding/AI/vector foundations:
  - new migration `005_ai_transcoding_contracts.sql`;
  - embedding and transcoding contract tables without requiring pgvector;
  - ffmpeg/ffprobe capability and encoder parsing;
  - AI/vector status and accelerator hint APIs;
  - WebUI Transcoding/Base AI status surfaces.
- Synthetic large-archive tooling:
  - `scripts/generate-synthetic-fixture.sh`;
  - `scripts/perf-smoke.sh`;
  - ignored default `testdata/synthetic_media/` path and documented `/tmp` workflow.
- Documentation:
  - updated README, architecture, DB schema, storage model, plugin model, and roadmap;
  - added `docs/OPERATIONS.md`, `docs/SECURITY.md`, and `docs/REAL_ARCHIVE_DRY_RUN.md`.

### Commands Run

- Repo/status inspection:
  - `git status --short --untracked-files=all`
  - `git diff --stat`
  - `rg --files`
  - `sed`/`rg` over requested docs and code entry points
- Formatting/checking:
  - `gofmt -w` on changed Go files
  - `git diff --check`
- Backend tests:
  - `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
  - `go test ./...`
- Frontend:
  - `npm --prefix webui run build`
- Smoke/Compose/DB:
  - `bash scripts/smoke-test.sh`
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres`
  - `bash scripts/test-db.sh`
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml stop postgres`
- Synthetic tooling:
  - `CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/generate-synthetic-fixture.sh`
  - `CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/perf-smoke.sh`

### Tests Passed

- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...` passed.
- `go test ./...` passed.
- `npm --prefix webui run build` passed.
- `bash scripts/smoke-test.sh` passed.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config` passed.
- `bash scripts/test-db.sh` passed against the dev PostgreSQL/PostGIS container.
- `git diff --check` passed.
- Synthetic fixture script generated 251 dummy files with defaults under `/tmp`.
- Synthetic perf smoke generated 1,001 dummy files under `/tmp` and reported counters.

### Failures And Fixes

- A moved local `row` type caused a Go compile failure in the explorer handler; fixed by promoting it to `explorerRow`.
- A new flat explorer filter test exposed that `media_kind` forced folder mode; fixed folder mode selection to be explicit through `view=folders`, `path`, or storage scope.
- The ffprobe frame-rate parser test initially treated `(float64, bool)` as a pointer; fixed the test.

### Known Limitations

- Local auth is still admin-centric; no public sharing or multi-role user management yet.
- No brute-force throttling/account lockout yet.
- CSRF is stateless per session token and rotates with the session token.
- Preview generation supports Go-decodable images only; video posters and HEIC previews remain unsupported.
- Metadata enrichment is additive JSON patching; richer typed metadata tables are future work.
- Map rendering is a simple SVG/GeoJSON view without real tile layers.
- Grid clustering is basic and deterministic, not PostGIS-backed.
- Rescan/missing-file marking remains deferred even though `missing_at` exists in schema.
- Synthetic perf smoke currently generates and reports a bounded dummy fixture; it does not benchmark a full discovery run automatically.
- Sidecar plugin health is a safe stub; core does not contact arbitrary sidecar URLs yet.
- Transcoding APIs are inventory/status only; no transcoding job writes output.
- AI/vector endpoints are contract/status stubs; no model execution or embeddings are generated.
- `webui/dist` was generated by builds and is ignored by git.
- `/tmp/cartolensia_synthetic_media` was created for synthetic script verification.

### Changed Files Summary

- Backend core: app wiring, config, auth, catalog, database, discovery, jobs, media, plugins, preview, server, GPX, metadata, transcoding.
- Tests: auth/session/scope, job endpoints, explorer filters, GPX analysis, ffprobe parser, plugin validation, preview cleanup, metadata enrichment, database metadata helper.
- WebUI: API client, auth/login/settings, jobs, metadata/previews, asset detail previews, plugin/transcoding/AI/map surfaces, styling.
- Migrations: added AI/vector and transcoding contract migration.
- Scripts: synthetic fixture generation and perf smoke.
- Docs/config: updated README and architecture/storage/plugin/schema/roadmap docs, added operations/security/real archive dry-run docs, added auth config examples.

### Approvals And Blocks

- No new approval requests were made during the unattended run.
- Docker/PostgreSQL commands ran successfully with the current environment.
- No blocked approval-sensitive command was required.

### Repository Safety

- No intermediate commits were made.
- No push was done.
- `/mnt/Models/rclone` was not read, listed, scanned, mounted, written, deleted, renamed, chmodded, or probed.
- Tests used `testdata/media_fixture/`, temporary synthetic files under `/tmp`, normal Go/npm build caches, and Docker PostgreSQL state only.

### Exact Recommended Next Prompt

```text
Continue from the current Cartolensia repo. Read AGENTS.md, README.md, RUN_REPORT.md, docs/ARCHITECTURE.md, docs/DB_SCHEMA.md, docs/STORAGE_MODEL.md, docs/PLUGIN_MODEL.md, docs/ROADMAP.md, docs/OPERATIONS.md, docs/SECURITY.md, and docs/REAL_ARCHIVE_DRY_RUN.md. Do not touch /mnt/Models/rclone. Implement the next hardening slice: scoped rescan/missing-file semantics with tests, DB-backed query pagination instead of in-memory post-filtering, sustained worker concurrency DB tests, persistent preview-cache index and cleanup job, brute-force throttling for local auth, richer asset metadata tables, and a real discovery benchmark over synthetic fixtures. Keep using testdata/media_fixture and /tmp synthetic fixtures only. Run gofmt, git diff --check, GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./..., go test ./..., npm --prefix webui run build, bash scripts/smoke-test.sh, docker compose config, and bash scripts/test-db.sh if Docker/PostgreSQL is available. Update RUN_REPORT.md honestly. Do not commit and do not push.
```
