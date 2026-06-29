# Run Report

## 2026-06-06 Supervised Preflight For Next Long Run

### Scope

- Interactive preflight only.
- No feature implementation started.
- Added `docs/NEXT_LONG_RUN_PLAN.md` as the exact plan for the next unattended implementation pass.
- No commit was made.
- No push was done.

### Repository And Safety Audit

Commands run:

- `git status --short --untracked-files=all`
- `git diff --stat`
- `git diff --check`
- `rg --files`
- `sed`/`rg` inspection of requested docs, storage configs, scripts, DB scripts, dependency files, and code entry points
- `git check-ignore -v .env .env.local webui/dist webui/node_modules testdata/synthetic_media .cartolensia/cache tmp temp logs`
- repo-local search only: `rg -n "/mnt/Models/rclone|rclone" -S .`

Results:

- Initial working tree was clean.
- `git diff --check` passed.
- `node_modules` and `webui/dist` exist locally but are ignored and not staged.
- No `.env`, preview cache, database volume, temporary media, or synthetic generated media is staged.
- `.gitignore` ignores `.env`, `node_modules`, `webui/dist` through `dist/`, `.cartolensia/`, and `testdata/synthetic_media/`.
- Config examples point only at `./testdata/media_fixture` in `strict_read_only` mode.
- `scripts/scan-rclone-readonly.sh` refuses to inspect `/mnt/Models/rclone` unless `CARTOLENSIA_ALLOW_RCLONE_SCAN=1` is set.
- No current app config automatically references `/mnt/Models/rclone`.
- Repo-local references to `/mnt/Models/rclone` are safety docs, this report, and the explicitly gated scan script only.

### Baseline Verification

Commands run and results:

- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...` passed.
- `go test ./...` passed.
- `npm --prefix webui run build` passed.
- `bash scripts/smoke-test.sh` passed.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config` passed.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres` started the dev PostgreSQL/PostGIS service.
- `bash scripts/test-db.sh` passed.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml stop postgres` stopped the dev PostgreSQL/PostGIS service.

Additional dependency-inspection commands:

- `go list -m all` failed in the sandbox because it attempted to resolve uncached transitive modules through `proxy.golang.org`; this is an environment/network restriction, not a test failure.
- `GOPROXY=off go list -m all` also failed because the full module graph is not cached.
- `go.mod`, `webui/package.json`, and `webui/package-lock.json` were used as the current dependency source of truth.
- `npm view ol version license` returned `10.9.0`, `BSD-2-Clause`.
- `npm view exifr version license` returned `7.1.3`, `MIT`.
- `npm view pica version license` returned `10.0.1`, `MIT`.
- `go list -m -versions github.com/rwcarlsen/goexif` returned module metadata but no version tags.
- `go list -m -versions github.com/dsoprea/go-exif/v3` returned `v3.0.1`.
- `go list -m -versions golang.org/x/image` failed because latest `golang.org/x/image@v0.41.0` requires Go 1.25 while this repo currently uses Go 1.22.

### Dependency Recommendations

Current dependencies:

- Backend direct dependencies: `github.com/jackc/pgx/v5`, `golang.org/x/crypto`, `gopkg.in/yaml.v3`.
- Frontend dependencies: `vue`; dev dependencies are Vite, TypeScript, Vue plugin, and `vue-tsc`.
- Password hashing currently uses `golang.org/x/crypto/bcrypt`, which is adequate for the next long run. No new password hashing dependency is recommended now.

Proposed dependency approvals for the long run:

- `ol`
  - Command: `npm --prefix webui install ol`
  - Package metadata: version `10.9.0`, license `BSD-2-Clause`
  - Why: substantially improves the map plugin MVP with pan/zoom, vector layers, feature selection, and future tile integration.
  - Optional: yes. Implementation can continue with the current SVG/GeoJSON map fallback if not approved.
  - Recommendation: approve for the next long run.
- `github.com/dsoprea/go-exif/v3`
  - Command: `go get github.com/dsoprea/go-exif/v3@v3.0.1`
  - License: not reported by `go list`; verify license/provenance before installing.
  - Why: server-side EXIF/GPS extraction is needed for photo/map MVP and cannot reliably be done in frontend code for large archives.
  - Optional: yes. Implementation can proceed with typed metadata plumbing and fixture hints first.
  - Recommendation: defer unless license is verified and scope remains small.
- `exifr`
  - Command: `npm --prefix webui install exifr`
  - Package metadata: version `7.1.3`, license `MIT`
  - Why: frontend EXIF parsing can help local upload/prototype flows, but it is not the right core path for server-side archive indexing.
  - Optional: yes.
  - Recommendation: do not add for the next long run unless upload-local-file workflows become part of scope.
- `golang.org/x/image/draw`
  - Command if needed: `go get golang.org/x/image@<Go-1.22-compatible-version>`
  - License: Go project BSD-style license; latest module currently requires Go 1.25 and is not compatible with this repo toolchain.
  - Why: better preview resize quality than current nearest-neighbor implementation.
  - Optional: yes. Current standard-library preview generation is sufficient for dry-run readiness.
  - Recommendation: do not add now; if needed later, pin a Go-1.22-compatible version explicitly.
- `pica`
  - Command: `npm --prefix webui install pica`
  - Package metadata: version `10.0.1`, license `MIT`
  - Why: high-quality browser resize, but Cartolensia preview generation is backend/cache based.
  - Optional: yes.
  - Recommendation: do not add for this backend preview/cache long run.

Approval requested:

- Approve adding OpenLayers in the next long run with `npm --prefix webui install ol`, or leave the long run on the current SVG/GeoJSON map UI.

### Database State Recommendation

- No DB reset is needed for the next long run.
- The long run should use schema migrations only plus isolated/gated test data.
- DB tests should continue using `scripts/test-db.sh` and `CARTOLENSIA_RUN_DB_TESTS=1`.
- Do not run `scripts/reset-dev-db.sh` unless explicitly approved; it stops Compose and removes the named dev volume `cartolensia_cartolensia_pgdata`.
- The long run should not drop user databases. Prefer isolated schemas/test records and forward-only migrations.

### Exact Long-Run Prompt Filename Recommendation

- Use `docs/NEXT_LONG_RUN_PLAN.md` as the plan file for the next unattended run.

Recommended prompt:

```text
Continue from the current Cartolensia repository. Read AGENTS.md, README.md, RUN_REPORT.md, docs/ARCHITECTURE.md, docs/DB_SCHEMA.md, docs/STORAGE_MODEL.md, docs/PLUGIN_MODEL.md, docs/ROADMAP.md, docs/OPERATIONS.md, docs/SECURITY.md, docs/REAL_ARCHIVE_DRY_RUN.md, docs/NEXT_LONG_RUN_PLAN.md, and ideas/general_description.md. Work autonomously through docs/NEXT_LONG_RUN_PLAN.md. Do not touch /mnt/Models/rclone. Use only testdata/media_fixture, ignored synthetic fixtures, and /tmp temporary fixtures. Do not reset any database unless explicitly approved in the prompt. Do not commit and do not push. Implement the safest useful slices for albums MVP, photo/map plugin MVP, GPS track manager MVP, scoped dry-run readiness, DB-backed pagination/query hardening, worker stress tests, metadata/preview/cache hardening, and UI integration. If OpenLayers has been approved, add it with npm --prefix webui install ol and use it for the map UI; otherwise keep the SVG/GeoJSON fallback. Run gofmt, git diff --check, GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./..., go test ./..., npm --prefix webui run build, bash scripts/smoke-test.sh, docker compose -f docker-compose.yml -f docker-compose.dev.yml config, and DB tests if Docker/PostgreSQL is available. Update RUN_REPORT.md honestly with implemented features, commands, failures, limitations, and confirmation that /mnt/Models/rclone was not touched.
```

## 2026-06-06 Albums/Map/GPS/Dry-Run Milestone Run

### Start State

- Continued from the supervised preflight state.
- Initial tracked/untracked status included the preflight `RUN_REPORT.md` update and new `docs/NEXT_LONG_RUN_PLAN.md`.
- `/mnt/Models/rclone` was not touched.

### Dependencies Added

- Frontend: `ol` installed with `npm --prefix webui install ol`.
  - Local package metadata: `ol@10.9.0`, license `BSD-2-Clause`.
  - Used for bundled OpenLayers vector map rendering. No CDN.
- Backend: `github.com/rwcarlsen/goexif v0.0.0-20190401172101-9e8deecbddbd`.
  - License verified from local module cache as BSD-style.
  - Used for server-side JPEG EXIF parsing and GPS extraction.
- `npm --prefix webui audit --omit dev` could not complete because the sandboxed environment could not resolve `registry.npmjs.org`; this is documented as an environment/network block, not as a clean audit.

### Implemented Features

- Added forward migration `006_albums_map_tracks_dryrun_cache.sql` for:
  - albums and album items;
  - typed asset geotags;
  - GPS track summaries;
  - scan runs/dry-run reports;
  - preview cache entries;
  - plugin settings;
  - supporting query indexes.
- Extended memory and PostgreSQL store contracts for:
  - DB-backed asset query/filter/pagination;
  - albums;
  - geotags/map queries;
  - GPS track summaries/point queries/track media lookup;
  - scan runs;
  - preview cache index/stats/cleanup.
- Albums plugin MVP:
  - `GET/POST /api/v1/albums`;
  - `GET/PATCH/DELETE /api/v1/albums/{id}`;
  - `GET/POST /api/v1/albums/{id}/items`;
  - `DELETE /api/v1/albums/{id}/items/{asset_id}`;
  - metadata-only album deletion; assets/originals are not deleted or moved;
  - WebUI Albums page, create form, album item list, remove item, add selected Explorer assets.
- Photo/map plugin MVP:
  - OpenLayers vector map in WebUI;
  - `GET /api/v1/map/status`;
  - `GET /api/v1/map/assets`;
  - `GET /api/v1/map/tracks`;
  - existing `/api/v1/map` now uses typed `asset_geo` for asset points;
  - album/media/source/track filters and deterministic grid clustering.
- GPS track manager MVP:
  - `GET /api/v1/gps/tracks`;
  - `GET/PATCH /api/v1/gps/tracks/{track_asset_id}`;
  - `GET /api/v1/gps/tracks/{track_asset_id}/points`;
  - `GET /api/v1/gps/tracks/{track_asset_id}/assets`;
  - `POST /api/v1/gps/tracks/{track_asset_id}/snap-media`;
  - `geo_snap` worker that conservatively interpolates track positions and does not overwrite EXIF/real/manual geotags.
- EXIF/geotag extraction:
  - new `internal/exif` wrapper;
  - JPEG EXIF metadata extraction during metadata enrichment;
  - EXIF GPS upserts typed `asset_geo` with source `exif`;
  - timezone-less EXIF datetimes are stored as raw metadata rather than blindly setting `taken_at`.
- Scoped dry-run readiness:
  - bounded prefix walker in filesystem adapter;
  - `POST /api/v1/discovery/dry-run`;
  - `GET /api/v1/discovery/dry-run/{job_id}/report`;
  - dry-run worker is report-only and does not index assets;
  - guard defaults: required non-empty prefixes, `max_files <= 50` unless explicitly overridden, 2 GiB default byte cap, no missing marking;
  - added `config/rclone-dryrun.example.yaml`;
  - added guarded `scripts/rclone-dry-run-preflight.sh`, not executed.
- Preview/cache hardening:
  - preview generation indexes cache entries;
  - on-demand preview serving marks cache entries accessed;
  - `GET /api/v1/previews/status`;
  - `GET /api/v1/previews/cache`;
  - `POST /api/v1/previews/cleanup`;
  - WebUI Metadata page shows cache status and dry-run cleanup.
- Worker/synthetic tooling:
  - added `scripts/worker-stress-test.sh`;
  - ran synthetic fixture generation and perf-smoke under `/tmp`.
- Tests added:
  - album metadata deletion does not delete assets;
  - geotag priority protects EXIF;
  - preview cache index/stats/cleanup in memory store;
  - dry-run report-only behavior with bounded temp fixtures;
  - EXIF no-EXIF/malformed-input safety;
  - track snapping interpolation and no EXIF overwrite.
- Documentation updated:
  - `README.md`;
  - `docs/ARCHITECTURE.md`;
  - `docs/DB_SCHEMA.md`;
  - `docs/STORAGE_MODEL.md`;
  - `docs/PLUGIN_MODEL.md`;
  - `docs/ROADMAP.md`;
  - `docs/OPERATIONS.md`;
  - `docs/SECURITY.md`;
  - `docs/REAL_ARCHIVE_DRY_RUN.md`;
  - `docs/NEXT_LONG_RUN_PLAN.md`;
  - this `RUN_REPORT.md`.

### Commands Run

- Baseline:
  - `git status --short --untracked-files=all`
  - `git diff --stat`
  - `git diff --check`
  - `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
  - `go test ./...`
  - `npm --prefix webui run build`
  - `bash scripts/smoke-test.sh`
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres`
  - `bash scripts/test-db.sh`
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml stop postgres`
- Dependencies/provenance:
  - `npm --prefix webui install ol`
  - `curl -L https://raw.githubusercontent.com/rwcarlsen/goexif/master/LICENSE`
  - `go get github.com/rwcarlsen/goexif@v0.0.0-20190401172101-9e8deecbddbd`
  - `go list -m -json github.com/rwcarlsen/goexif`
  - `node -p "const p=require('./webui/node_modules/ol/package.json'); JSON.stringify({name:p.name,version:p.version,license:p.license})"`
  - `npm --prefix webui audit --omit dev` failed due sandbox DNS/network block.
- Iterative verification:
  - `gofmt -w ...` on changed Go files
  - `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
  - `npm --prefix webui run build`
- Final verification:
  - `go test ./...` passed.
  - `bash scripts/smoke-test.sh` passed.
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml config` passed.
  - `git diff --check` passed.
  - `CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/generate-synthetic-fixture.sh` passed and generated 1001 dummy files under `/tmp`.
  - `CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/perf-smoke.sh` passed and generated/reported 1001 dummy files under `/tmp`.
  - `bash scripts/worker-stress-test.sh` passed memory/job/worker checks and skipped DB-backed stress because DB stress env vars were not set for that script.
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres` passed.
  - `bash scripts/test-db.sh` passed.
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml stop postgres` passed.

### Failures And Fixes

- Initial expanded DB store work required adding PostgreSQL implementations for the new catalog interface; fixed by adding `internal/database/extended.go`.
- A SQL placeholder helper initially handled repeated placeholders incorrectly; fixed before runtime tests.
- The server map handler initially still used metadata JSON lat/lon; fixed to use typed `asset_geo`.
- The frontend OpenLayers build hit a nullable extent type error; fixed with a null guard before fitting the map view.
- A new preview cache mapper name collided with the existing preview cleanup `CacheEntry` type; fixed by renaming the mapper to `IndexEntry`.
- `npm audit` was blocked by sandbox DNS/network restrictions and was not retried with approval because this was an unattended run.

### Known Limitations

- Dry-run discovery is intentionally report-only and does not index assets yet.
- Scoped rescan/missing-file marking remains deferred.
- The synthetic perf smoke still measures fixture generation, not a full discovery benchmark against a temporary storage config.
- DB-backed pagination is implemented for the asset list and common store queries, but some folder grouping and track-media helper paths still do bounded in-process composition.
- EXIF fixture coverage does not include a generated positive GPS EXIF image; runtime parser integration is wired, and no-EXIF/malformed behavior is tested.
- OpenLayers map has vector features but no real/offline tile layer yet.
- Video posters remain unsupported.
- `geo_snap` is conservative and currently based on asset `taken_at`; richer EXIF timezone and camera-clock offset workflows remain future work.
- `webui/dist` was generated by builds and remains ignored.
- `/tmp/cartolensia_synthetic_media` was created by verification scripts.

### Changed Files Summary

- Backend: `internal/app`, `internal/catalog`, `internal/database`, `internal/discovery`, `internal/exif`, `internal/metadata`, `internal/preview`, `internal/server`, `internal/storage`.
- Migrations: `migrations/006_albums_map_tracks_dryrun_cache.sql`.
- WebUI: `webui/src/App.vue`, `webui/src/api.ts`, `webui/src/style.css`, package lock/package metadata.
- Config/scripts: `config/rclone-dryrun.example.yaml`, `scripts/rclone-dry-run-preflight.sh`, `scripts/worker-stress-test.sh`.
- Docs: README, architecture/schema/storage/plugin/roadmap/operations/security/real-archive dry-run/next long-run plan, run report.

### Future Tiny Real-Archive Manual Steps

Do not run these without a supervised approval turn:

1. Review `docs/REAL_ARCHIVE_DRY_RUN.md`.
2. Review `config/rclone-dryrun.example.yaml`; confirm `rclone_dryrun`, `strict_read_only`, and cache outside the archive.
3. Start Cartolensia locally with auth enabled and that config.
4. Choose one non-empty relative prefix and keep `max_files <= 50`.
5. Run the guarded script only with all required variables set:

```bash
CARTOLENSIA_ALLOW_RCLONE_DRY_RUN=1 \
CARTOLENSIA_RCLONE_DRY_RUN_PREFIX='some/non-empty/prefix' \
CARTOLENSIA_EXECUTE_RCLONE_DRY_RUN=1 \
bash scripts/rclone-dry-run-preflight.sh
```

6. Inspect the job and scan-run report before any further action.
7. Do not request hash/metadata/previews/missing marking during the first tiny dry run.

### Repository Safety

- `/mnt/Models/rclone` was not read, listed, scanned, mounted, written, deleted, renamed, chmodded, statted, or probed.
- No generated media, preview cache, DB volume, `.env`, credentials, `node_modules`, or `webui/dist` is intentionally committed.
- No commit was made.
- No push was done.

### Exact Recommended Next Prompt

```text
Continue from the current Cartolensia repo. Read AGENTS.md, README.md, RUN_REPORT.md, docs/ARCHITECTURE.md, docs/DB_SCHEMA.md, docs/STORAGE_MODEL.md, docs/PLUGIN_MODEL.md, docs/ROADMAP.md, docs/OPERATIONS.md, docs/SECURITY.md, docs/REAL_ARCHIVE_DRY_RUN.md, and docs/NEXT_LONG_RUN_PLAN.md. Do not touch /mnt/Models/rclone. Implement the next hardening slice: scoped rescan/missing-file semantics, DB-backed Explorer folder grouping and track-media pagination, PostgreSQL integration tests for albums/geotags/dry-run/preview-cache, a real synthetic discovery benchmark, video poster preview design or MVP, and positive EXIF GPS fixture coverage if feasible. Use only testdata/media_fixture, ignored synthetic fixtures, and /tmp temporary fixtures. Run gofmt, git diff --check, GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./..., go test ./..., npm --prefix webui run build, bash scripts/smoke-test.sh, docker compose -f docker-compose.yml -f docker-compose.dev.yml config, bash scripts/worker-stress-test.sh, and bash scripts/test-db.sh if Docker/PostgreSQL is available. Update RUN_REPORT.md honestly. Do not commit and do not push.
```

### `/mnt/Models/rclone`

Skipped entirely. It was not read, listed, scanned, mounted, written, deleted, renamed, chmodded, statted, or probed during this preflight.

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

## 2026-06-06 Final Report Ordering Note

The latest implementation section is `2026-06-06 Albums/Map/GPS/Dry-Run Milestone Run` above. It was inserted before the earlier `Phase 2/3 Foundation Run` section during this unattended pass, so this note makes the file tail unambiguous.

Final sanity commands after that report update:

- `git status --short --untracked-files=all` showed only reviewable working-tree changes and new files in the repository.
- `git diff --check` passed.

No commit was created, no push was done, and `/mnt/Models/rclone` was not touched.

## 2026-06-06 Real-Peek UI Bugfix Runtime Session

### Scope

Interactive/live bugfix run against the temporary `cartolensia_realpeek` PostgreSQL project and localhost app on `127.0.0.1:18080`. The temporary DB was preserved and not reset.

### Diagnostics

- `/api/v1/health`: healthy.
- Initial `/api/v1/stats` during diagnosis showed the original real-peek subset had `50` assets, `48` photos, `2` videos, and `0` tracks.
- `/api/v1/assets?limit=20` showed EXIF metadata already present for many indexed assets, including GPS latitude/longitude.
- `/api/v1/explorer` returned flat asset rows.
- `/api/v1/albums` showed the existing `Test` album with `0` items.
- `/api/v1/tracks` and `/api/v1/gps/tracks` returned JSON `null`, which explained GPS page null crashes.
- `/api/v1/map/status` reported GeoJSON/vector map support.
- `/api/v1/map` and `/api/v1/map/assets` returned photo point features when not constrained by the frontend's old hardcoded bbox.
- `/api/v1/map/tracks` returned an empty FeatureCollection.

### Fixes Implemented

- Fixed frontend routing so explicit `?page=...` wins over saved `localStorage` route state; unknown `page` falls back to Explorer.
- Added frontend API normalization for nullable arrays/objects.
- Fixed backend track list responses to return `[]`, not `null`.
- Removed the frontend's hardcoded map bbox that excluded the current real-data coordinates.
- Added map status counts/warnings and ensured GeoJSON `features` is always an array.
- Added table/tile toggle for Explorer and Albums.
- Added Explorer selection controls and a clearer add-selected-to-album workflow.
- Added album detail tile view and metadata-only remove action.
- Added shared gallery overlay with photo/video support, arrow buttons, keyboard arrows, Escape close, Open Original, and Asset Detail.
- Added inline asset detail media panel with image preview/original fallback and HTML5 video player.
- Added safe video quality selector and `GET /api/v1/media/{asset_id}/stream-options`; transcoding options are visible but disabled because transcoding jobs are not implemented.
- Changed WebUI discovery action to bounded-only using visible storage/prefix/max fields.
- Added backend guard refusing unbounded discovery against `/mnt/Models/rclone`.

### Important Runtime Event

A stale unbounded discovery job with payload `{"storage":"all"}` already existed in the temporary DB and resumed after app restart. It was cancelled immediately when discovered:

- job id: `7e5ba768-621c-400d-a50a-2b9d63128174`
- final status: `canceled`

After the guard was added, a later stale/unbounded discovery attempt failed safely with:

- `unbounded discovery is refused for real archive storage; provide storage, prefixes, max_files, and max_bytes`

The temporary DB therefore no longer represents only the original 50-file subset. Current live counts after cancellation:

- assets: `13744`
- locations: `13744`
- photos: `12553`
- videos: `953`
- track-kind assets: `238`
- `/api/v1/gps/tracks`: `[]`
- geotagged assets from map status: `48`
- running jobs: `0`

### Verification

- `git diff --check`: passed.
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`: passed.
- `go test ./...`: passed.
- `npm --prefix webui run build`: passed, producing `/assets/index-pAxjkaoD.js`.
- `bash scripts/smoke-test.sh`: passed after temporarily stopping the live app so the smoke script could bind its default `127.0.0.1:18080`.

Failed/handled checks:

- First `bash scripts/smoke-test.sh` attempt failed because the live real-peek app occupied port `18080`.
- Alternate-port smoke with `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081` failed with sandbox `socket: operation not permitted`.
- Browser console automation was not run because local `playwright` and `puppeteer` packages are not installed.

### Live App

- App URL: `http://127.0.0.1:18080/`
- Config: `.cartolensia/runtime/realpeek.yaml`
- PID file: `.cartolensia/runtime/realpeek.pid`
- Current frontend bundle: `/assets/index-pAxjkaoD.js`
- Runtime note: `.cartolensia/runtime/REAL_PEEK_FIX_STATUS.md`

Test pages:

- `http://127.0.0.1:18080/?page=explorer`
- `http://127.0.0.1:18080/?page=albums`
- `http://127.0.0.1:18080/?page=map`
- `http://127.0.0.1:18080/?page=gps-tracks`
- `http://127.0.0.1:18080/?page=jobs`

### Safety

- PostgreSQL was not reset.
- No commit was made.
- No push was done.
- No missing marking was run.
- No destructive storage mode was enabled.
- No files were written, deleted, renamed, moved, chmodded, trashed, transcoded into, or cached inside `/mnt/Models/rclone`.
- The stale unbounded job read beyond the intended initial sample before cancellation; this changed only temporary PostgreSQL metadata, not real files.

## 2026-06-06 Clean Real-Peek Reset, Guard Hardening, And Final Verification

### Scope

Continued from the live real-peek correction run. The polluted temporary PostgreSQL state was diagnosed, then reset with the supervised temporary Compose project as requested. A clean bounded real-peek session was started afterward.

No commit was made and no push was done.

### Polluted Runtime Diagnostics Before Reset

Queried:

- `/api/v1/stats`
- `/api/v1/jobs?limit=50`
- `/api/v1/assets?limit=10`
- `/api/v1/map/status`
- `/api/v1/map/assets?limit=10`
- `/api/v1/albums`
- `/api/v1/gps/tracks`

Findings:

- The polluted temporary DB had `13744` assets because a stale unbounded `storage=all` discovery job resumed before the guard was added.
- Running jobs were stopped/cancelled before reset.
- Map state had some geotags in polluted data, but that state was intentionally discarded.
- GPS track summaries were still empty.

Reset executed:

```bash
docker compose -p cartolensia_realpeek -f docker-compose.yml -f docker-compose.dev.yml down -v
rm -rf .cartolensia/runtime .cartolensia/realpeek-cache
```

The reset removed only temporary app/runtime/cache data and the temporary PostgreSQL volume for Compose project `cartolensia_realpeek`.

### Implemented Since The Polluted Runtime

- Hard real-archive guardrails:
  - `storage=all` rejected when any configured storage root is `/mnt/Models/rclone` or inside it.
  - discovery/hash against real archive storage requires explicit storage, non-empty adapter-relative prefix, `max_files`, and `max_bytes`.
  - empty/root/dot/dot-dot/archive-root-equivalent prefixes are rejected.
  - safe absolute prefixes under the configured root are normalized to adapter-relative prefixes; unsafe absolute prefixes are rejected.
  - stale unsafe queued jobs fail safely before scanning.
- `scripts/real-peek-start.sh` and `scripts/real-peek-reset.sh`.
- Bounded hash workflow for selected assets, current prefix, and albums.
- Original-quality gallery overlay:
  - opened image uses `/api/v1/media/{asset_id}/original`;
  - preview is only a tile/detail fallback;
  - Fit/100% controls, keyboard navigation, Escape close, video player, Open Original, and Asset Detail close behavior.
- Inline asset media viewer/player and truthful stream options endpoint.
- Stable tile/gallery card layout and album filter reset controls.
- On-demand OSM tile proxy/cache:
  - `/api/v1/tiles/osm/{z}/{x}/{y}.png`;
  - `/api/v1/map/tile-sources`;
  - coordinate/path validation, cache under Cartolensia cache directory, attribution metadata, no public-OSM bulk prefetch endpoint.
- Map status warnings explain empty map causes and show indexed/geotagged/track counts.
- GPS tracks empty states are robust and explain when track-like assets exist without parsed GPX summaries.
- Discovery UI labels now distinguish report-only dry run from bounded indexing.
- Explorer DB-backed filters/pagination/sorting surfaced in WebUI.
- Month buckets endpoint/UI integration.
- Report-only duplicate grouping by SHA-512+size:
  - `/api/v1/duplicates`;
  - stats include duplicate groups/locations.
- Moved-file/content identity foundation:
  - `migrations/007_content_identity.sql`;
  - content uniqueness by `(sha512, size_bytes)`;
  - new hashed locations relink to existing logical assets when content equality is confirmed.
- Optional HTTPS:
  - configured certificate/key support remains;
  - added in-memory self-signed TLS via `http.tls_auto_self_signed`;
  - backend status reports HTTP/TLS mode.
- Docs updated:
  - `README.md`;
  - `docs/ARCHITECTURE.md`;
  - `docs/DB_SCHEMA.md`;
  - `docs/STORAGE_MODEL.md`;
  - `docs/ROADMAP.md`;
  - `docs/OPERATIONS.md`;
  - `docs/SECURITY.md`;
  - `docs/REAL_ARCHIVE_DRY_RUN.md`.

### Clean Real-Peek Session

Clean session settings:

- App URL: `http://127.0.0.1:18080`
- Compose project: `cartolensia_realpeek`
- Storage: `rclone_peek`
- Root: `/mnt/Models/rclone`
- Mode: `strict_read_only`
- Prefix indexed: `Cartolensia-photos`
- Max files: `50`
- Max bytes: `2147483648`
- Missing marking: `false`
- Metadata enrichment: not run
- Preview generation: not run
- Hash after index: run only for the bounded indexed prefix subset.

Clean counts after restart:

- assets: `50`
- locations: `50`
- photos: `48`
- videos: `2`
- tracks: `0`
- hashed: `50`
- unhashed: `0`
- duplicate groups: `0`
- duplicate locations: `0`
- geotagged assets: `0`
- track summaries: `0`
- total bytes: `614190070`

Jobs:

- Discovery job `0ffef45b-2d79-4947-bfbf-fd336d7b2a7a`: `succeeded`, `scanned=50`, `created=50`, `bytes=614190070`, `errors=0`.
- Hash job `1c0f5e9e-3390-4e80-9b10-e59dadd23386`: `succeeded`, `hashed=50`, `bytes=614190070`, `errors=0`.

Runtime notes:

- `.cartolensia/runtime/REAL_PEEK_STATUS.md`
- `.cartolensia/runtime/REAL_PEEK_FIX_STATUS.md`

### Verification Commands

Passed:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

Handled failure:

- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh` failed with sandbox `listen tcp 127.0.0.1:18081: socket: operation not permitted`. The approved/default smoke test on `18080` passed after temporarily stopping and then restarting the live real-peek app.

### Live App For Inspection

The app was restarted after final verification.

- App URL: `http://127.0.0.1:18080`
- PID file: `.cartolensia/runtime/realpeek.pid`
- Current indexed state: clean bounded 50-file subset.

Pages:

- `http://127.0.0.1:18080/?page=explorer`
- `http://127.0.0.1:18080/?page=albums`
- `http://127.0.0.1:18080/?page=map`
- `http://127.0.0.1:18080/?page=gps-tracks`
- `http://127.0.0.1:18080/?page=jobs`
- `http://127.0.0.1:18080/?page=duplicates`

Stop/reset when inspection is done:

```bash
bash scripts/real-peek-reset.sh
```

### Known Limitations

- The clean real-peek subset has no map asset points because metadata enrichment was not run and no typed geotags exist yet.
- The clean real-peek subset has no GPS track summaries because no GPX metadata enrichment was run and no parsed GPX tracks were present in the bounded subset.
- OSM tiles are on-demand through Cartolensia and require network for tiles not already cached; packaged offline tiles remain future work.
- Video transcoding options are visible but disabled except original/direct streaming.
- Public album sharing, SMB/NFS/S3 adapters, upload/WebDAV, reverse geocoding, AI inference, and real transcoding jobs remain future work.

### Safety Confirmation

- `/mnt/Models/rclone` was not written to.
- No preview/cache/transcoded/database/temp files were created under `/mnt/Models/rclone`.
- No missing marking was run.
- No unbounded scan was run after the clean reset.
- Hashing ran only for the clean bounded indexed prefix subset.
- Real storage remained `strict_read_only`.
- No commit was made.
- No push was done.

### Recommended Next Prompt

```text
Continue from the current Cartolensia repository and live real-peek state. Do not reset PostgreSQL unless explicitly asked. Do not touch /mnt/Models/rclone except through the existing strict read-only bounded real-peek session. First inspect the UI on http://127.0.0.1:18080 and verify Explorer tile/table, gallery overlay, asset viewer/player, Albums add/remove, Map empty-state explanation, GPS Tracks empty state, Jobs, and Duplicates. If approved, run metadata enrichment only for the current 50 indexed assets, then verify EXIF geotags and map features. Keep missing marking off. Do not commit or push.
```

## 2026-06-06 Live Pipeline, Map, KML/KMZ, Settings, And Transcode Productization

### Live Diagnosis

Queried the current live service before implementing additional fixes:

- `/api/v1/stats`
- `/api/v1/jobs?limit=30`
- `/api/v1/assets?limit=10`
- `/api/v1/assets?limit=10&hash_status=hashed`
- `/api/v1/assets?limit=10&hash_status=unhashed`
- `/api/v1/map/status`
- `/api/v1/map/assets?limit=50&clusters=false`
- `/api/v1/map/assets?limit=50&clusters=true`
- `/api/v1/previews/status`
- `/api/v1/previews/cache`
- `/api/v1/gps/tracks`
- `/api/v1/duplicates`
- `/api/v1/storages`
- `/api/v1/config/effective`

Findings:

- Current DB has `50` assets, `48` photos, `2` videos, `50` hashed, `0` unhashed, and `614190070` total bytes.
- Metadata enrichment had already succeeded for `50 / 50`; `48` photos have EXIF GPS geotags.
- Preview cache has `48` ready entries. Later `preview_generate 0 / 0` jobs were no-op runs because all scoped photos were already cached.
- Later `hash 0 / 0` jobs were no-op runs because all scoped assets were already hashed. The original bounded hash job hashed `50 / 50`.
- Map raw mode returned `48` point features. Cluster mode at zoom 10 previously collapsed most points too coarsely.
- GPS/KML tracks are empty in the current 50-file subset.
- Storage remains `rclone_peek`, root `/mnt/Models/rclone`, mode `strict_read_only`.

### Implemented

- Added KML/KMZ support:
  - KML coordinate parsing for points/lines/rings and `gx:Track`;
  - KMZ parsing with `archive/zip`;
  - `.kmz` classified as track media;
  - metadata enrichment now parses GPX/KML/KMZ into track summaries/points.
- Added single indexing pipeline UI and backend surface:
  - `POST /api/v1/indexing/start`;
  - `GET /api/v1/indexing/latest`;
  - `GET /api/v1/indexing/{pipeline_id}`;
  - `POST /api/v1/indexing/{pipeline_id}/cancel`.
- Discovery UI now has one primary “Start indexing pipeline” action plus “Stop current pipeline”.
- Pipeline stages preserve scoped storage/prefix between discovery, hash, metadata, preview, track/geotag, and map-refresh stages.
- No new hash/preview job is enqueued from the UI when scope metrics already show all assets hashed or all photos cached.
- Hash/preview no-target jobs now log clear reasons.
- Explorer, Albums, and Track media tiles now show hash/geotag badges.
- Asset detail shows hash status, short SHA-512, geotag state, media metadata, and a video quality selector.
- Map cluster payloads now include count and per-kind counts, centroid and bbox, sample assets, and preview/original/detail URLs.
- Map UI now opens point/cluster popups, can zoom spatial clusters, and shows scrollable mini-gallery samples for clusters.
- High-zoom clustering returns individual assets except identical-coordinate groups.
- Added Settings MVP:
  - tabs;
  - effective config;
  - restart-required YAML fields;
  - runtime preference patch endpoint;
  - cache-scoped DB metadata export;
  - guarded import-plan endpoint.
- Added cache-scoped HLS transcode session MVP:
  - start session;
  - serve playlist/segments;
  - stop session;
  - stream options expose direct original plus H.264 profiles when `ffmpeg` is available.

### Current Live State After Restart

- App URL: `http://127.0.0.1:18080`
- Config: `.cartolensia/runtime/realpeek.yaml`
- Runtime start: direct `go run ./cmd/cartolensia -config .cartolensia/runtime/realpeek.yaml`
- Storage: `rclone_peek`
- Root: `/mnt/Models/rclone`
- Mode: `strict_read_only`
- Prefix indexed: `Cartolensia-photos`
- Assets: `50`
- Photos: `48`
- Videos: `2`
- Tracks: `0`
- Hashed: `50`
- Unhashed: `0`
- Geotagged assets: `48`
- Preview cache entries ready: `48`
- Duplicate groups: `0`

### Verification Commands

Passed:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

Handled:

- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh` failed with sandbox `listen tcp 127.0.0.1:18081: socket: operation not permitted`; the default-port smoke passed after temporarily stopping and restarting the live app.
- `scripts/real-peek-start.sh` was blocked by sandbox Docker socket access on restart; PostgreSQL was already running, so the app was restarted directly with the existing config and unchanged temporary DB.
- H.264-low transcode session creation and stop succeeded; HLS playlist polling was unreliable in the sandbox, so direct/original streaming remains the known-good browser path until manual HLS playback is checked.

### Local Runtime Notes

- `.cartolensia/runtime/REAL_PEEK_STATUS.md`
- `.cartolensia/runtime/REAL_PEEK_FIX_STATUS.md`

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No preview/cache/transcoded/database/temp files were created under `/mnt/Models/rclone`.
- No missing marking was run.
- No unbounded scan was run.
- Real storage remained `strict_read_only`.
- No commit was made.
- No push was done.

### Recommended Next Prompt

```text
Continue from the current Cartolensia repository and live real-peek state. Inspect http://127.0.0.1:18080 manually. Verify Discovery pipeline, Explorer hash/geotag badges, gallery overlay, asset detail viewer/player, map clusters/popups, Settings tabs, and video direct streaming. Do not reset PostgreSQL unless explicitly requested. Keep storage rclone_peek strict_read_only and do not modify /mnt/Models/rclone. If HLS playback is not supported by the browser, add hls.js or a progressive fragmented-MP4 fallback in a future supervised dependency-approved run. Do not commit or push.
```

## 2026-06-07 Live Productization Follow-Up: HLS, GPS/KML, Map, Settings

### Live Diagnosis

Queried the live real-peek service during this run:

- `/api/v1/stats`: `54` assets, `54` locations, `48` photos, `2` videos, `4` track files, `54` hashed, `0` unhashed.
- `/api/v1/jobs?limit=30`: recent scoped GPSLogger metadata jobs originally showed KML/GPX parse errors on truncated XML; after parser fixes the latest scoped job succeeded with `4 / 4` updated and `0` errors.
- `/api/v1/map/status`: `48` geotagged assets, `4` parsed tracks, OSM proxy enabled, `screen_distance` clustering.
- `/api/v1/map/assets?...clusters=true`: zoom 10 returns one large `48`-asset cluster; zoom 20 splits into individual markers and same-coordinate clusters with count/sample metadata.
- `/api/v1/gps/tracks`: now returns `4` summaries: two GPX and two KML tracks.
- `/api/v1/previews/status`: `48` ready preview entries under `.cartolensia/realpeek-cache`.
- `/api/v1/media/<video>/stream-options`: direct/original plus `h264_720p_lan` and `h264_low_bitrate` HLS session profiles.
- `/api/v1/settings`: includes pending YAML metadata and the raw/effective config tab.

### Implemented

- Added `hls.js` frontend dependency, license verified locally from package metadata: `Apache-2.0`, version `1.6.16`.
- Fixed HLS transcode playback flow:
  - session status endpoint;
  - stable profile IDs;
  - playlist/segment MIME types;
  - ffmpeg stderr tail in status;
  - frontend waits for ready playlist/segment before switching;
  - hls.js attach path for browsers without native HLS;
  - Original/direct remains default and fallback.
- Verified live `h264_low_bitrate` transcode session:
  - ffmpeg produced H.264/AAC HLS under `.cartolensia/realpeek-cache/transcode/<session>`;
  - playlist served as `application/vnd.apple.mpegurl`;
  - test session was stopped and its cache directory was removed through the API.
- Improved map clustering and UI:
  - backend screen-distance clustering with `cluster_distance_px`;
  - cluster count/sample metadata;
  - marker count labels;
  - OpenLayers overlay popup with scrollable mini-gallery;
  - cluster zoom no longer gets immediately undone by extent refit.
- Improved gallery overlay:
  - original-quality opened image source;
  - wheel zoom;
  - double-click fit/100% toggle;
  - WASD panning for zoomed photos;
  - arrow keys still navigate assets.
- Fixed Discovery/Indexing UI semantics:
  - shared settings panel applies to both preview report and indexing pipeline;
  - requested default extensions: `jpg,jpeg,png,gpx,kml,kmz,gpz,heif,heic,mp4,mov`.
- Added robust track parsing:
  - `.gpz` classified as track-like;
  - GPZ zip helper for GPX/KML payloads;
  - tolerant GPX parse for truncated XML with complete points;
  - KML/KMZ raw coordinate salvage for truncated coordinate blocks;
  - no-time KML geometry still creates queryable summaries using synthetic timestamps marked in metadata.
- GPS page now says `GPS/KML Tracks`, shows source format, and has a scoped “Parse track files for current prefix” action.
- Base AI page is now a dashboard instead of raw JSON only.
- Settings page now has editable controls for YAML-bound settings, pending YAML save/clear/download, runtime settings, plugin settings JSON controls, and a Raw YAML / Effective Config tab.
- Added tests for GPX/KML truncation salvage, KML no-time geometry, GPZ parsing, HLS helpers, and screen-distance clustering.

### Current Live State

- App URL: `http://127.0.0.1:18080`
- Config: `.cartolensia/runtime/realpeek.yaml`
- Storage: `rclone_peek`
- Root: `/mnt/Models/rclone`
- Mode: `strict_read_only`
- Prefixes represented in the temporary DB: `Cartolensia-photos`, `Cartolensia-photos/GPSLogger`
- Assets: `54`
- Photos: `48`
- Videos: `2`
- Track files: `4`
- Parsed GPS/KML track summaries: `4`
- Hashed: `54`
- Unhashed: `0`
- Geotagged assets: `48`
- Preview cache ready: `48`
- Duplicate groups: `0`

### Verification Commands

Passed:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh` with escalation for localhost socket binding
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

Other results:

- First non-escalated `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh` failed with sandbox-only `listen tcp 127.0.0.1:18081: socket: operation not permitted`; rerunning with approved escalation passed.
- `npm --prefix webui install hls.js` initially timed out without network access; rerun with approved escalation succeeded.
- `npm install` reported `2 moderate severity vulnerabilities`; no automatic `npm audit fix --force` was run because that could introduce unrelated dependency churn.

### Remaining Limitations

- Manual browser validation is still needed for hls.js playback controls in Chromium/Firefox/Safari.
- KML salvage can recover coordinate geometry from truncated files but cannot recover real timestamps from unclosed coordinate-only blocks; synthetic timestamps are used only for DB query compatibility.
- Settings pending YAML is saved under the configured cache directory and requires manual restart/application; it does not rewrite the active real-peek YAML automatically.
- Map popup reverse-geocoded location labels remain `null`; no local geocoder has been added.
- The frontend bundle is over Vite’s 500 KB warning threshold due to OpenLayers plus hls.js; build passes.

### Safety Confirmation

- `/mnt/Models/rclone` was read through strict read-only storage only.
- `/mnt/Models/rclone` was not written to, deleted from, renamed in, chmodded, transcode-written, tile-cached, preview-cached, or temp-written.
- HLS output was written only under `.cartolensia/realpeek-cache/transcode`; the test session cache was deleted through the API.
- OSM tile cache and preview cache remain under `.cartolensia/realpeek-cache`, not under the archive.
- No missing marking was run.
- No unbounded scan was run.
- No commit was made.
- No push was done.

### Recommended Next Prompt

```text
Continue from the current Cartolensia repository and live real-peek service. Do not reset PostgreSQL unless explicitly asked. Manually inspect http://127.0.0.1:18080 and verify: Low bitrate LAN HLS playback, map cluster labels/popups, GPS/KML Tracks showing 4 tracks, Discovery shared pipeline settings, Settings pending YAML forms, and gallery wheel/WASD zoom. Keep /mnt/Models/rclone strict read-only, do not mark missing, do not run unbounded scans, do not commit, and do not push.
```

## 2026-06-07 Long Productization Run: Track Popups, Search, Presets, Settings, AI Scaffold

### Live Diagnosis

Queried the current real-peek service before and after the run:

- `/api/v1/stats`: `54` assets, `54` locations, `48` photos, `2` videos, `4` track files, `54` hashed, `0` unhashed, `619580406` total bytes.
- `/api/v1/gps/tracks`: `4` parsed summaries, including `2` GPX and `2` KML tracks.
- `/api/v1/map/status`: `48` geotagged assets, `4` tracks, PostGIS available, Cartolensia OSM tile proxy, `screen_distance` clustering.
- `/api/v1/map/assets?limit=100&clusters=true`: far zoom clusters the current photo set; high zoom splits into individual points and same-coordinate clusters.
- `/api/v1/transcoding/presets`: built-ins present; AV1 preset is disabled with a clear encoder/browser-safety reason.
- `/api/v1/ai/workers`: CPU/NVIDIA/ROCm/Intel profile entries are present and `not_configured`.
- `/api/v1/search?q=jpg&limit=3`: returned results with filename/path/extension match explanations.

No new broad scan was run. No missing marking was run.

### Dependencies Added

- `bootstrap` via npm, license verified from package metadata as `MIT`.
- `bootstrap-icons` via npm, license verified from package metadata as `MIT`.

Previously added and still in use:

- `ol` (`BSD-2-Clause`) for OpenLayers.
- `hls.js` (`Apache-2.0`) for browser HLS playback.
- `github.com/rwcarlsen/goexif` (BSD-style cached module license) for EXIF parsing.

### Implemented

- Fixed map click priority: asset and cluster markers are above track layers and win click handling before tracks.
- Added track click popups:
  - clicked coordinate;
  - nearest track point and distance;
  - relative/absolute time when timestamps exist;
  - speed and elevation when available;
  - buttons/actions for Track Manager, time-based media, nearby media by meters, and show-only-this-track.
- Added backend track helper APIs:
  - `GET /api/v1/gps/tracks/{id}/point-info`
  - `GET /api/v1/gps/tracks/{id}/nearby-assets`
- Added track asset previews:
  - `GET /api/v1/media/{asset_id}/track-preview`
  - `GET /api/v1/media/{asset_id}/track-thumbnail`
  - generated thumbnails stay under `.cartolensia/realpeek-cache`.
- Improved gallery overlay pan/zoom:
  - wheel zoom at cursor;
  - pointer/touch/pen drag pan;
  - pinch zoom;
  - double-click fit/100%;
  - Fit/100/Reset buttons;
  - WASD pan;
  - ArrowLeft/ArrowRight always navigate.
- Fixed Explorer nested-prefix usability: folder tiles now render when a scanned prefix only contains child folders, so `Root / Cartolensia-photos` is not a blank page.
- Added universal Explorer search:
  - endpoint `GET /api/v1/search?q=...`;
  - filename/path/extension/media/date/hash/metadata/tag/album/track matching;
  - result explanations.
- Added transcoding preset persistence:
  - built-in presets are non-removable;
  - custom presets can be saved/removed;
  - backend validates preset shape and HLS profile use.
- Added advanced video-player controls:
  - hardware selector;
  - codec/encoder selector;
  - quality/quantizer/bitrate mode;
  - custom preset save/remove;
  - browser local preference reuse.
- Reorganized Settings:
  - distinct category tabs instead of one repeated combined list;
  - useful GPS/KML Tracks tab;
  - Map/Tiles, Preview, Transcoding, Metadata, Indexing, AI, Backups tabs;
  - second-row plugin tabs;
  - UI/YAML config toggle per plugin;
  - settings schema endpoints.
- Added Bootstrap/Bootstrap Icons local bundle imports. No CDN is required.
- Added AI sidecar foundation:
  - `services/ai/worker.py` dummy HTTP JSON worker;
  - Docker Compose profiles for `ai-cpu`, `ai-nvidia`, `ai-rocm`, `ai-intel`;
  - AI worker registry/status APIs;
  - not-configured classify/faces/describe job endpoints;
  - schema migration for `asset_tags`, `ai_predictions`, `face_detections`, `face_clusters`, and `user_preferences`.
- Updated README and docs for the implemented safety boundaries, APIs, settings, AI scaffold, search, track previews, and transcoding presets.

### Verification Commands

Passed:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

Notes:

- `npm --prefix webui run build` passes with Vite’s large chunk warning. The bundle now includes OpenLayers, Bootstrap, Bootstrap Icons, and HLS support.
- The smoke test was run on `127.0.0.1:18081` to avoid disturbing the live app on `127.0.0.1:18080`.

### Current Live State For Manual Inspection

- App URL: `http://127.0.0.1:18080`
- Config: `.cartolensia/runtime/realpeek.yaml`
- Storage: `rclone_peek`
- Root: `/mnt/Models/rclone`
- Mode: `strict_read_only`
- Assets: `54`
- Hashed: `54`
- Geotagged assets: `48`
- Parsed GPX/KML tracks: `4`
- Preview cache ready: `48`
- Current live HLS inspection session: `cce1d680-88ea-419a-b47b-4cfef39f0113`, under `.cartolensia/realpeek-cache/transcode`.

Pages to inspect:

- Explorer/search: `http://127.0.0.1:18080/?page=explorer`
- Map: `http://127.0.0.1:18080/?page=map`
- GPS/KML Tracks: `http://127.0.0.1:18080/?page=gps-tracks`
- Asset/video detail: `http://127.0.0.1:18080/?page=explorer`, then open a video asset.
- Settings: `http://127.0.0.1:18080/?page=settings`
- Base AI: `http://127.0.0.1:18080/?page=base-ai`

### Known Limitations

- Track thumbnails currently use the dark fallback renderer. OSM-background thumbnail compositing is represented in settings but not implemented.
- Last selected video preset is stored in browser local storage. The `user_preferences` table exists, but user preference API wiring is future work.
- Custom preset UI is functional for metadata/session selection, but hardware-specific encoder failure handling still depends on ffmpeg runtime feedback.
- AI sidecar and schema foundation are present, but no real model inference, embeddings, face clustering, or caption generation runs yet.
- Settings schemas are generic; custom plugin-provided WebUI components remain future work.
- No local reverse geocoder exists, so map cluster `location_label` remains `null`.
- Built assets may include upstream license/reference URLs in bundled package comments; runtime CSS/JS/icon assets are bundled locally and not loaded from a CDN.

### Safety Confirmation

- `/mnt/Models/rclone` was not written to, deleted from, renamed in, moved in, chmodded, transcode-written, preview-cached, tile-cached, model-cached, exported into, dumped into, or temp-written.
- Real storage remained `strict_read_only`.
- No unbounded scan was run.
- No `storage=all` real archive action was run.
- No missing marking was run.
- Cache/work outputs stayed under `.cartolensia/realpeek-cache`, `.cartolensia/exports`, `.cartolensia/models`, or other repo-local ignored paths.
- No commit was made.
- No push was done.

### Recommended Next Prompt

```text
Continue from the current Cartolensia repository and live real-peek service. Do not reset PostgreSQL unless explicitly asked. Manually inspect http://127.0.0.1:18080 and verify: map marker/cluster click priority, track click popups, track thumbnails/gallery previews, gallery mouse/touch zoom, Explorer universal search, custom video transcoding presets, Settings category/plugin tabs, and Base AI dashboard. Keep /mnt/Models/rclone strict read-only, do not mark missing, do not run unbounded scans, do not commit, and do not push.
```

## 2026-06-07 Interactive Preflight For Next Implementation Pass

### Scope

This was a supervised preflight only. No large implementation was started.

Hard safety status:

- `/mnt/Models/rclone` was not written to, deleted from, renamed in, moved in, chmodded, transcode-written, preview-cached, tile-cached, model-cached, exported into, dumped into, temp-written, or newly scanned.
- No new real-data prefixes were scanned.
- PostgreSQL was not reset.
- Missing-file marking was not run.
- No Docker images were pulled.
- No model files were downloaded.
- No dependency installs were performed.
- No commit was made.
- No push was done.

### Files Updated

- `docs/NEXT_INTERACTIVE_PREFLIGHT.md`
- `docs/NEXT_LONG_RUN_PLAN.md`
- `docs/AI_SERVICE_PLAN.md`
- `docs/TRANSCODING_HARDWARE_PLAN.md`
- `RUN_REPORT.md`

### Live API Audit

Queried the current app at `http://127.0.0.1:18080`:

- `GET /api/v1/stats`
- `GET /api/v1/assets?limit=5`
- `GET /api/v1/gps/tracks`
- `GET /api/v1/map/status`
- `GET /api/v1/map/assets?limit=100&clusters=true`
- `GET /api/v1/map/tracks`
- `GET /api/v1/transcoding/capabilities`
- `GET /api/v1/transcoding/presets`
- `GET /api/v1/ai/status`
- `GET /api/v1/ai/workers`
- `GET /api/v1/settings`
- `GET /api/v1/settings/schema`
- `GET /api/v1/storages`
- `GET /api/v1/search?q=jpg`
- `GET /api/v1/jobs?limit=20`
- `GET /api/v1/assets?media_kind=video&limit=1`
- `GET /api/v1/media/ce8b4866-33bd-474e-84ab-a0fd9388a313/stream-options`

Current live counts:

- Assets: `54`
- Locations: `54`
- Photos: `48`
- Videos: `2`
- Track files: `4`
- Parsed GPX/KML summaries: `4`
- Hashed: `54`
- Unhashed: `0`
- Geotagged assets: `48`
- Duplicate groups: `0`
- Total bytes: `619580406`

Current storage:

- Name: `rclone_peek`
- Root: `/mnt/Models/rclone`
- Mode: `strict_read_only`

Jobs:

- Latest 20 jobs were terminal; no active job was observed.
- Recent bounded `Cartolensia-photos` discovery jobs reached `max_files=50` and report incomplete due to the explicit bound, not because of failure.
- Historical zero-target hash/preview jobs remain in job history; the next run should make these explicit no-op/rejected jobs instead of confusing succeeded/queued jobs.

Map/tracks:

- `/api/v1/map/status` reports PostGIS available, `48` geotagged assets, `4` tracks, OSM tile proxy, and `screen_distance` clustering.
- GPS/KML track summaries exist for `2` GPX and `2` KML files.
- Track Manager still needs a dedicated detail route with OpenLayers map, altitude profile, speed profile, and media-query actions.

Transcoding:

- ffmpeg/ffprobe are available through the app.
- Stream options for video asset `ce8b4866-33bd-474e-84ab-a0fd9388a313` include direct/original, CPU HLS presets, disabled AV1, and custom NVIDIA preset `nv-750k`.
- The custom NVIDIA preset is only statically validated today. It needs real dry-run validation, bitrate normalization, NVENC-specific command flags, and stderr display.

AI:

- AI workers exist as not-configured profiles.
- No real inference worker is running.
- Dummy worker file exists, but native `python -m cartolensia_ai.server` packaging is not implemented yet.

Settings/search:

- Settings schema covers runtime indexing/preview/map/transcoding and pending metadata/GPS/preview/map/transcoding.
- Settings layout still needs per-tab cleanup and stronger pending YAML UI.
- Search MVP works for `jpg`, returning `48` results with match explanations.

### Local Hardware And Tooling Probes

Commands run:

- `uname -a`
- `lscpu | sed -n '1,80p' || true`
- `lspci | grep -Ei 'vga|3d|display|nvidia|amd|intel|arc' || true`
- `ls -l /dev/dri || true`
- `nvidia-smi || true`
- `ffmpeg -hide_banner -version || true`
- `ffmpeg -hide_banner -hwaccels || true`
- `ffmpeg -hide_banner -encoders | grep -Ei 'nvenc|vaapi|qsv|amf|av1|264|265|hevc' || true`
- `ffprobe -hide_banner -version || true`
- `docker --version || true`
- `docker compose version || true`
- `docker info --format '{{json .Runtimes}}' || true`
- `docker images | grep -Ei 'cuda|pytorch|rocm|intel|cartolensia|ai' || true`
- `docker compose --profile ai-nvidia -f docker-compose.yml -f docker-compose.dev.yml config`
- `python3 --version`
- Python import probe for `torch`, `torchvision`, `fastapi`, `uvicorn`, `PIL`, `cv2`, and `onnxruntime`.

Hardware detected:

- CPU: AMD Ryzen 9 7900X, 24 threads, AVX2/AVX512 available.
- NVIDIA: GeForce RTX 3090 Ti detected by `nvidia-smi`.
- NVIDIA driver: `570.124.06`.
- CUDA runtime reported by driver: `12.8`.
- AMD/ATI Raphael integrated GPU appears in `lspci`.
- `/dev/dri` is absent in the shell environment.
- `vainfo` is not installed.

ffmpeg/ffprobe:

- ffmpeg: `6.1.1-3ubuntu5`.
- ffprobe: `6.1.1-3ubuntu5`.
- Hardware acceleration methods advertised: `vdpau`, `cuda`, `vaapi`, `qsv`, `drm`, `opencl`, `vulkan`.
- Encoders advertised:
  - NVIDIA: `h264_nvenc`, `hevc_nvenc`, `av1_nvenc`.
  - VAAPI: `h264_vaapi`, `hevc_vaapi`, `av1_vaapi`.
  - QSV: `h264_qsv`, `hevc_qsv`, `av1_qsv`.
  - CPU: `libx264`, `libx265`, `libsvtav1`, `libaom-av1`, `librav1e`.

Important diagnosis:

- Host NVIDIA/NVENC is promising for native ffmpeg tests.
- Docker GPU is not ready yet: `docker info` reported only `runc`/`io.containerd.runc.v2`, not an NVIDIA runtime.
- The app's hardware status currently reports `/dev/dri`/VAAPI/QSV availability too optimistically compared with the shell probe. The next run should separate "ffmpeg encoder compiled in" from "device actually available".

Docker:

- Docker CLI: `29.2.1`.
- Docker Compose: `v5.0.2`.
- No local CUDA/PyTorch/ROCm/Intel AI image was found.
- No Docker container was started during this preflight.

Python:

- Python: `3.12.3`.
- Present: `PIL`.
- Missing: `torch`, `torchvision`, `fastapi`, `uvicorn`, `cv2`, `onnxruntime`.

Frontend dependency licenses verified from local metadata:

- `ol` `10.9.0`: `BSD-2-Clause`.
- `bootstrap` `5.3.8`: `MIT`.
- `bootstrap-icons` `1.13.1`: `MIT`.
- `hls.js`: `Apache-2.0`.

### Proposed Dependencies And Models

No new dependency is required for track altitude/speed charts if implemented with plain SVG/canvas.

AI dependencies proposed but not installed:

- `fastapi`, for robust Python sidecar HTTP API.
- `uvicorn`, for serving FastAPI.
- `numpy`, for image/model pipelines.
- `torch` and `torchvision`, for future classification/embedding models.
- Optional `onnxruntime` or `opencv-python-headless` only if face detection model/provenance is approved.

Model proposal, without downloads:

- Stage 1: dummy/no-model worker only.
- Stage 2: small torchvision classification model after weight license/provenance and size are approved.
- Stage 3: face detection only after model license is clear.
- Stage 4: captioning/description deferred because likely model size is much larger.
- Stage 5: CLIP-like embeddings deferred until model license, size, and vector store approach are approved.

Model cache location:

- `.cartolensia/models`
- Never `/mnt/Models/rclone`.

### Approvals Needed Before The Next Long Run Uses External Resources

Implementation without extra approval:

- Go/Vue code changes.
- Docs/tests.
- Dummy AI worker packaging.
- Command builder and API validation tests using sample strings.
- Plain SVG/canvas charts.

Approval needed:

- Short native ffmpeg NVENC dry-run on the current indexed 7-second video, output only under `/tmp` or `.cartolensia/realpeek-cache/transcode-test`.
- Python venv creation and package installation for AI sidecar dependencies.
- Docker image pulls/builds for CUDA/PyTorch/ROCm/Intel images.
- Model downloads into `.cartolensia/models`.
- Docker GPU probe using `docker run --gpus all` if/when a suitable image exists or a small CUDA image pull is approved.

Approval not requested:

- No DB reset.
- No new real-data scan.
- No missing marking.
- No long transcode job.
- No model download.
- No Docker image pull.

### Plans Written

- `docs/NEXT_INTERACTIVE_PREFLIGHT.md` records the live state and hardware/tooling audit.
- `docs/NEXT_LONG_RUN_PLAN.md` now contains the exact next implementation plan.
- `docs/AI_SERVICE_PLAN.md` defines the AI sidecar layout, native `server` entrypoint, Docker profiles, contracts, schema direction, dependency/model proposal, and approval gates.
- `docs/TRANSCODING_HARDWARE_PLAN.md` defines the NVENC/custom-preset validation plan, Apply-before-Save flow, hardware-test API, command builder rules, and safety requirements.

### Recommended Next Prompt

```text
Continue from the current Cartolensia repository and live real-peek service. Implement the next long-run plan in docs/NEXT_LONG_RUN_PLAN.md, using docs/AI_SERVICE_PLAN.md and docs/TRANSCODING_HARDWARE_PLAN.md as constraints. Do not reset PostgreSQL unless explicitly asked. Do not scan new real-data prefixes, do not mark missing, and keep /mnt/Models/rclone strict read-only. Do not pull Docker images, download models, install Python AI dependencies, or run ffmpeg hardware dry-runs unless approval has been granted. Do not commit and do not push.
```

## 2026-06-07 Long Implementation Run

### Implemented

- Track detail page/backend hardening:
  - added `/api/v1/gps/tracks/{id}/profile?metric=altitude|speed`;
  - track media lookup now defaults to `photo,video` and excludes track assets;
  - GPS/KML Track Manager now opens a real detail view with an OpenLayers track map, altitude/speed SVG profiles, stats, source asset link, time-based media lookup, and nearby-geotag media lookup.
- Map interaction polish:
  - confirmed asset/cluster layer has priority over track layer;
  - track clicks open a popup rather than navigating away;
  - popup actions call time-based and nearby-media APIs.
- Track previews:
  - existing `/api/v1/media/{asset_id}/track-preview` and `/track-thumbnail` paths verified and covered by tests;
  - track thumbnails are generated under the Cartolensia cache, never beside originals.
- Transcoding:
  - added preset validation and hardware-test APIs;
  - advanced UI now has Apply, Test current hardware configuration, Save, and Remove actions;
  - gallery keyboard handling is disabled while the transcode modal is open;
  - command builder now uses NVENC-compatible `p5` preset, normalizes bare bitrates (`750` -> `750k`), reports stderr/command/session stats, and avoids the failing `min(1280,iw)` scale expression.
- Storage management:
  - registry now preserves storage mode and supports safe runtime add/update validation;
  - added guarded `POST /api/v1/storages`, `PATCH /api/v1/storages/{name}`, and `GET /api/v1/storages/{name}/validate`;
  - `/mnt/Models/rclone` roots are locked to `strict_read_only`; write-capable modes remain disabled.
- Preview/cache policy:
  - default `indexing.previews_after_index` is now `false`, making on-demand previews the conservative default.
- AI sidecar foundation:
  - added packaged `services/ai/cartolensia_ai` FastAPI dummy/no-model sidecar;
  - native entrypoint: `python -m cartolensia_ai.server --host 127.0.0.1 --port 19090`;
  - added CPU/NVIDIA/ROCm/Intel requirements stubs and updated Docker AI entrypoint to the packaged server;
  - backend AI worker status now probes local dummy sidecar health.
- Universal search:
  - `album:` and `track:` structured tokens now use existing album/track store data;
  - date ranges like `2026-05..2026-06` are parsed as real bounds;
  - search still returns match explanations.

### Live Real-Peek State

- App is running at `http://127.0.0.1:18080` with `.cartolensia/runtime/realpeek.yaml`.
- App runtime binary: `.cartolensia/runtime/cartolensia-realpeek`.
- Dummy AI sidecar is running at `127.0.0.1:19090` and is reachable by the app.
- Live counts after restart:
  - `54` assets;
  - `54` hashed;
  - `48` geotagged assets;
  - `2` videos;
  - `4` GPS/KML tracks.
- Storage remains `rclone_peek`, root `/mnt/Models/rclone`, mode `strict_read_only`.

### NVENC Validation

- Approved short native ffmpeg dry-run was run against the already indexed 7-second video `PXL_20260516_163309946.mp4`.
- Sandboxed ffmpeg failed with `CUDA_ERROR_NO_DEVICE`, confirming sandbox GPU isolation.
- Native/outside-sandbox dry-run succeeded with `h264_nvenc`, `p5`, `750k`, `scale=w=1280:h=-2`, null muxer output.
- The live app hardware-test endpoint also succeeded:
  - `dry_run_ok: true`;
  - elapsed about `1.7s`;
  - no output file written;
  - command summary redacted the original absolute path.

### Commands Run

- `git diff --check` passed.
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...` passed.
- `go test ./...` passed.
- `npm --prefix webui run build` passed, with only Vite chunk-size warning.
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh` passed.
- Plain `bash scripts/smoke-test.sh` failed because port `18080` was occupied by the live real-peek app; the no-body discovery call correctly hit real-peek guards and returned `400`.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config` passed.
- `bash scripts/test-db.sh` passed.
- `.cartolensia/ai-venv/bin/python -c "from cartolensia_ai.server import create_app; ..."` passed.

### Dependencies

- Installed into repo-local `.cartolensia/ai-venv`:
  - `fastapi`;
  - `uvicorn`;
  - `numpy`;
  - transitive packages from those dependencies.
- No Torch, torchvision, CUDA packages, model files, or Docker images were installed or downloaded.

### Known Limitations

- AI sidecar is dummy/no-model only; real classification, faces, captions, and embeddings still need approved model/dependency downloads.
- The app reports the local dummy worker as configured only while the sidecar process is running.
- Track thumbnails still use the dark fallback renderer; OSM-background compositing remains future work.
- Storage runtime additions are active in-process but are not yet persisted as durable DB/YAML runtime storage records; pending YAML remains the durable path.
- The default smoke script should be run on a non-live port when real-peek occupies `18080`.

### Safety Confirmation

- `/mnt/Models/rclone` was read only for approved original/media reads and ffmpeg dry-run input.
- No files were written, cached, exported, transcoded, dumped, or placed under `/mnt/Models/rclone`.
- No new real-data prefixes were scanned.
- No missing marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-08 Multimodal Audio/OCR Productization Pass

Implemented:

- Added first-class `audio` and `document` media kinds.
- Extended `/api/v1/stats` with `audio` and `documents` counts.
- Extended ffprobe probing to capture audio codec, sample rate, channels, bitrate, duration, container, and stream presence.
- Added metadata enrichment for audio and document assets.
- Added durable multimodal metadata schema and store methods:
  - `asset_transcripts`;
  - `asset_transcript_segments`;
  - `audio_features`;
  - `video_frame_captions`;
  - `document_text`.
- Added per-asset APIs:
  - `GET /api/v1/assets/{id}/transcripts`;
  - `GET /api/v1/assets/{id}/audio-features`;
  - `GET /api/v1/assets/{id}/frame-captions`;
  - `GET /api/v1/assets/{id}/document`;
  - `GET /api/v1/audio/{id}/metadata`;
  - `POST /api/v1/audio/analyze/start`.
- Added OCR full-text aggregation:
  - `GET /api/v1/assets/{id}/ocr` now returns `full_text` and `summary`;
  - asset detail now exposes `ocr_full_text` and `ocr_summary`;
  - UI shows copyable/downloadable full OCR text above the block list.
- Expanded PostgreSQL/local universal search:
  - `ocr:...`;
  - `transcript:...`;
  - `document:...`;
  - `caption:...` including video frame captions;
  - `genre:...`, `key:...`, and `tempo:...` from audio feature rows.
- Added asset-detail UI panels for:
  - audio playback and ffprobe metadata;
  - transcripts;
  - audio features;
  - video frame captions;
  - document text/markdown.
- Fixed track detail/gallery ghost overlay by hiding the static fallback SVG after the interactive OpenLayers vector layer has loaded.
- Fixed asset-detail top-level `metadata` to return `asset.Metadata` instead of an empty object.

Live bounded audio scan:

- Scanned only the approved prefix: `/mnt/Models/rclone/Cartolensia-photos/Sound Records`.
- Request was bounded with `max_files=20`, `max_bytes=2147483648`, and audio extensions only.
- Discovery job `f5f0bc3d-4902-4c22-b11c-0d77e3a3ce96` succeeded:
  - `3` files indexed;
  - `3` created;
  - `617252484` bytes observed;
  - hashing disabled;
  - missing marking disabled.
- Metadata/audio analysis job `e3cd0d9f-f923-4bea-8575-31f97b1fefd2` succeeded and updated all `3` audio assets.
- Live stats after scan:
  - `57` assets;
  - `48` photos;
  - `2` videos;
  - `3` audio;
  - `4` tracks;
  - `54` hashed;
  - `3` unhashed.
- Sample audio asset `74625869-da0b-474a-a233-93984a8fb982`:
  - codec `pcm_s16le`;
  - sample rate `44100`;
  - channels `2`;
  - duration `456.282993` seconds;
  - `audio_features` record created with model `ffprobe_metadata`.

Verification:

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- Focused live checks:
  - `/api/v1/stats`;
  - `/api/v1/components/status`;
  - `/api/v1/search?q=ocr:test`;
  - `/api/v1/search?q=audio`;
  - `/api/v1/search?q=wav`;
  - `/api/v1/search?q=transcript:test`;
  - `/api/v1/search?q=caption:train`;
  - `/api/v1/assets/{audio_id}/audio-features`;
  - `/api/v1/audio/{audio_id}/metadata`;
  - `/api/v1/map/status`.

Known limitations:

- ASR/faster-whisper, librosa feature extraction, Marker document extraction, and advanced video-caption models remain component/model dependent. The durable schema/API/UI/search contracts are in place, but model-backed jobs are not enabled unless the relevant components are installed and reviewed.
- Audio feature MVP currently stores ffprobe-backed duration/container/codec metadata and marks genre classification as `model_missing`.
- Document discovery is supported for extensions and durable text storage, but PDF/Marker extraction remains pending component integration.
- Map clustering still reports `screen_distance`; the deeper persisted zoom-level cluster cache remains future work.

Safety confirmation:

- `/mnt/Models/rclone` was read only.
- No generated files, models, components, transcodes, caches, OCR dumps, exports, or sidecars were written under `/mnt/Models/rclone`.
- Only the explicitly approved `Cartolensia-photos/Sound Records` prefix was scanned.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-08 ASR And Audio Analysis Productization Pass

### Implemented

- Added real ASR support to the local AI sidecar:
  - `POST /transcribe-audio`;
  - faster-whisper provider with CUDA when available and CPU fallback;
  - explicit language support for English, Russian, Armenian, Chinese, and auto-detect;
  - safe media materialization under `/tmp` or approved cache/model roots only;
  - returned full transcript, timestamped segments, language probability, model/device metadata, and confidence proxies.
- Added Component Manager records for `asr-faster-whisper`, `asr-ctranslate2`, `asr-model-small`, `asr-model-medium`, `audio-librosa`, `audio-soundfile`, and `document-pymupdf`.
- Installed approved repo-local Python packages into `.cartolensia/ai-venv`, including faster-whisper, CTranslate2, librosa, SoundFile, PyMuPDF, and supporting packages.
- Downloaded and registered the approved `faster-whisper-small` model under `.cartolensia/models/faster-whisper`; it is now visible as `asr-model-small`.
- Added backend ASR job/action support:
  - `POST /api/v1/ai/jobs/transcribe`;
  - `GET /api/v1/assets/{id}/transcripts`;
  - `GET /api/v1/transcripts`;
  - `DELETE /api/v1/transcripts/{id}`;
  - persisted `asset_transcripts` and `asset_transcript_segments`.
- Added asset-detail UI support:
  - audio/video transcription action buttons;
  - transcript section with full text, copy button, and clickable timestamped segments that seek the audio/video player.
- Added real audio feature analysis:
  - sidecar `POST /analyze-audio` using librosa/SoundFile;
  - backend `/api/v1/audio/analyze/start` and `/api/v1/ai/jobs/audio-analyze`;
  - persisted tempo, key, mode, loudness, speech/music ratio, spectral summary, and heuristic labels in `audio_features`;
  - audio asset action button for bounded per-asset analysis.
- Hardened multimodal search:
  - transcript text is searchable through `transcript:...` and plain local search;
  - audio feature search supports `genre:...`, `key:...`, exact tempo, and tempo ranges such as `tempo:120..140`.

### Live Validation

- Synthetic `/tmp/cartolensia-asr-tone.wav` ASR probe succeeded through the sidecar using faster-whisper `small`.
- Bounded real-peek ASR run on `Hamalir (2026-03-15 19_19_40).wav` completed:
  - job `142200aa-866a-4ad3-9a21-ecb64aa1c218`;
  - 1 target processed;
  - 12 timestamped segments plus full transcript stored;
  - transcript search `transcript:story` returned the audio asset.
- Bounded real-peek audio analysis on the same asset completed:
  - job `5a0a9fcb-e8fb-4caa-afac-2057f1ff711a`;
  - tempo `126.048` BPM;
  - key `C major`;
  - loudness `-27.79`;
  - labels `music-like`, `mid-tempo`;
  - searches `genre:music-like`, `key:C`, and `tempo:120..140` returned the asset.
- Live `/api/v1/ai/workers` now advertises `transcribe_audio` and `analyze_audio`.
- Live `/api/v1/components/status` reports ASR/audio packages and `asr-model-small` installed; VMAF/libvmaf, MobileNetV3 fallback weights, and `asr-model-medium` remain missing/optional.

### Verification

Passed during this pass:

- `.cartolensia/ai-venv/bin/python -m py_compile services/ai/cartolensia_ai/server.py services/ai/cartolensia_ai/models/real.py services/ai/cartolensia_ai/models/dummy.py`
- `gofmt -w internal/server/server.go internal/server/server_test.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server ./internal/catalog ./internal/database`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server`
- `npm --prefix webui run build`

Final full verification is recorded in the latest closeout notes.

### Known Limitations

- ASR quality depends on the selected faster-whisper model and source audio; the live sample auto-detected English with moderate probability.
- Genre classification is currently heuristic and marked `heuristic_labels_model_missing`; no dedicated genre model was downloaded.
- `asr-model-medium` is optional and remains missing to avoid a larger download in this pass.
- Document/Marker extraction remains component-scaffolded; PyMuPDF is installed and registered, but a full document extraction job was not completed in this pass.
- Persisted map cluster cache remains future work; current map clustering behavior was not changed in this pass.

### Safety Confirmation

- `/mnt/Models/rclone` was read only through Cartolensia media URLs and was not modified.
- No generated files, model files, components, transcripts, caches, or exports were written under `/mnt/Models/rclone`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-09 Full Cartolensia-Photos Read-Only Indexing And UI Hardening

Implemented:

- Geo Align layer switches were widened and normalized to Bootstrap switch dimensions so they no longer render as a small white circle inside a blue circle.
- Discovery supported extensions are now centralized in the backend storage package and exposed through runtime settings. The default list covers supported photo, video, GPS/KML, audio, and document extensions.
- Normal indexing now accepts `max_files=-1` and `max_bytes=-1` as unlimited for an explicit storage/prefix. Real-archive validation still rejects omitted/zero limits and root/all-storage scans. Dry-run previews remain conservatively capped unless explicitly over-limit, and the UI now says the cap is preview-only.
- Runtime setting `gps.track_arrow_interval_m` was added with default `500`. OpenLayers track visualizations now render directional arrowheads at that interval and hide them when the value is set to `0`.
- Track direction arrows were wired into GPS track detail/gallery preview styles, the main map track layer style, and Geo Align track styling.
- Added local-first reverse geocoding endpoint `GET/POST /api/v1/places/reverse`. It checks cached place bounding boxes first, supports optional user-triggered Nominatim-compatible online reverse lookup when enabled, and caches online results into `place_cache`.
- Asset detail place refresh now calls the reverse-geocode endpoint and shows cache/source notes.

Full read-only indexing run for `rclone_peek` prefix `Cartolensia-photos`:

- Discovery job `1b5d749c-ae3f-4a09-871d-dd2d3dbbf204`: `616` files seen/indexed, `509` assets created, `107` updated, `37,704,581,680` bytes observed, `0` errors.
- Hash job `21292c01-3433-445a-aafe-801966605795`: `509/509` files hashed in the job; final stats show `616/616` assets hashed.
- Metadata job `bd2ed3e6-21e6-44f0-a7dc-b8dcdd36e489`: `616/616` assets updated, `0` errors.
- Preview job `568b878a-d39b-449d-a7fb-d6d2163a0071`: `443/443` missing photo previews generated in cache only, `0` errors.

Final indexed media counts:

- `616` assets and `616` locations.
- `541` photos, `52` videos, `5` audio files, `18` GPS/KML/GPX tracks.
- `200` geotagged assets reported by map status.
- `0` unhashed assets.
- `37,704,581,680` total bytes.

AI and metadata jobs run on the explicit indexed scope:

- Classification: `541` image assets processed, `75` non-image assets skipped, `3,202` rows stored, `0` errors.
- Safety/NSFW: `541` image assets processed, `75` skipped, `1,625` rows stored, `2` unsafe/potentially unsafe results, `0` errors.
- Face detection: `541` image assets processed, `75` skipped, `1,062` rows stored, `0` errors.
- Embeddings: `541` image assets processed, `75` skipped, `541` embeddings stored, `0` errors.
- Captions: `541` image assets processed, `75` skipped, `1,082` caption rows stored, `0` errors.
- OCR: `541` image assets processed, `75` skipped, `1,702` OCR blocks stored, `0` errors.
- ASR transcription: `57` audio/video assets processed, `559` photos/tracks skipped, `253` transcript/segment records stored, `0` errors.
- Audio analysis: `5` audio assets processed, `611` non-audio assets skipped, `12` feature records stored, `0` errors.

Component status:

- `23` components installed/available after checks.
- Remaining failed/provenance-gated components:
  - `vmaf`: no reviewed `source_url`; current FFmpeg build still lacks the `libvmaf` filter.
  - `asr-model-medium`: no reviewed `source_url`; small faster-whisper model remains installed and active.
  - `mobilenetv3-large`: no reviewed `source_url`; EfficientNet-B0 is installed and active as the primary classifier.
- The Component Manager correctly refused silent downloads for unreviewed sources and recorded failed component jobs with actionable messages.

Live validation:

- `/api/v1/stats`: `616` assets, `616` hashed, `541` photos, `52` videos, `5` audio, `18` tracks.
- `/api/v1/map/status`: PostGIS enabled, `screen_distance` clustering active, `200` geotagged assets, `18` tracks.
- `/api/v1/settings`: `indexing.default_max_files=-1`, `gps.track_arrow_interval_m=500`, and supported extension list includes audio/document types.
- `/api/v1/search?q=Yerevan`: local place cache matched `Yerevan, Armenia`, returned place/media/track matches.
- `/api/v1/search?q=Armenia`: local place cache matched `Armenia`.
- `/api/v1/search?q=audio`: returned `12` audio-related results.
- `/api/v1/search?q=caption:train`: returned `31` caption-related results.
- `/api/v1/search?q=ocr:%D0%B0`: returned `17` OCR text matches.
- `/api/v1/assets/de7abf69-f077-4d4e-9648-46c850a4aa8e/ocr`: returned `5` OCR blocks and full text.
- `/api/v1/assets/b0f0896d-21be-41a2-9310-c4b310b13b76/transcripts`: returned one transcript record.
- `/api/v1/assets/b0f0896d-21be-41a2-9310-c4b310b13b76/audio-features`: returned tempo/key/loudness/speech-music feature metadata.
- `/api/v1/gps/tracks`: returned `18` tracks; first checked track had `17,207` points.

Validation commands:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`

Notes and limitations:

- The explicit `/api/v1/map/clusters/refresh` and `/api/v1/map/clusters/status` endpoints are not present yet; current map status still reports the existing `screen_distance` clustering path. Track direction arrows are implemented in frontend vector styles.
- Online reverse geocoding remains disabled by default and must be operator-triggered. The new endpoint supports cache-first reverse lookup and Nominatim-compatible provider caching when enabled.
- The built binary had to be launched outside the sandbox to bind `127.0.0.1:18080` and connect to local PostgreSQL during final validation. It is left running for inspection, and the AI sidecar remains running on `127.0.0.1:19090`.
- Vite build passed with the existing large-chunk warning.

Safety confirmation:

- `/mnt/Models/rclone` was read only through the approved `rclone_peek` prefix `Cartolensia-photos`.
- No writes, chmods, moves, transcodes, exports, OCR caches, models, or component files were placed under `/mnt/Models/rclone`.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

### Final Closeout Verification

Passed after the final edits:

- `git diff --check`
- `bash -n scripts/smoke-test.sh`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`

Focused live checks passed:

- `/api/v1/health`: `ok`;
- AI sidecar `/health`: `ok`, with `transcribe_audio` and `analyze_audio`;
- `/api/v1/stats`: `57` assets, `48` photos, `2` videos, `3` audio, `4` tracks;
- `/api/v1/components/status`: ASR/audio components installed and optional missing components clearly reported;
- `/api/v1/search?q=transcript:story`: returned the transcribed audio asset;
- `/api/v1/search?q=tempo:120..140`: returned the analyzed audio asset;
- `/api/v1/assets/edfa9b20-b6d8-42ea-b87b-10609f48c511/transcripts`: returned full transcript plus 12 segments;
- `/api/v1/assets/edfa9b20-b6d8-42ea-b87b-10609f48c511/audio-features`: returned librosa feature metadata.

Additional fix during verification:

- `scripts/smoke-test.sh` expected the old fixture count of `4`; current fixture tests and discovery include `5` assets including a document. The smoke script now checks `assets:5` and `hashed:5`.

Live services were restarted and left running:

- App: `http://127.0.0.1:18080`;
- AI sidecar: `http://127.0.0.1:19090`.

## 2026-06-08 OCR Runtime And Durable Place Cache Pass

This pass continued from the live real-peek service after the operator installed OCR packages. I did not install packages, download models, pull Docker images, scan new real-data prefixes, reset PostgreSQL, run missing-file marking, commit, or push.

### Implemented

- Real Tesseract OCR sidecar runtime:
  - added `/ocr-image` to the FastAPI sidecar in real and dummy modes;
  - the real backend calls the local `tesseract` CLI, parses TSV output, returns text blocks with bounding boxes/confidence, and reports structured missing-engine/language errors;
  - required OCR languages are English, Russian, Armenian, Simplified Chinese, and Traditional Chinese;
  - temporary OCR inputs are bounded and stay under safe temp/cache paths, never original storage.
- OCR backend/UI hardening:
  - added metadata-only OCR block deletion through `DELETE /api/v1/assets/{id}/ocr/{block_id}`;
  - added an OCR delete button to asset detail text records;
  - tightened sidecar default OCR confidence filtering to reduce false positive blocks.
- Durable local place cache:
  - added forward migration `009_place_cache.sql`;
  - added store methods and PostgreSQL/memory implementations for place cache CRUD;
  - seeded local cache entries for `Yerevan`, `Armenia`, `Lori Province`, and `Vanadzor`;
  - `/api/v1/places` now lists/creates operator-managed cache entries;
  - `/api/v1/places/{id}` patches/deletes entries;
  - universal search and asset-detail place rows now read from durable place entries with fallback defaults.
- Settings Search/Places UI:
  - added an operator place-cache editor with filter, add, edit, delete, and “Search this place” actions;
  - kept online geocoding disabled/cache-only; no public geocoder calls are made automatically.

### Live Validation

- App restarted on `http://127.0.0.1:18080`.
- AI sidecar restarted on `http://127.0.0.1:19090`.
- `/api/v1/stats`: `54` assets, `54` hashed, `48` photos, `2` videos, `4` tracks.
- `/api/v1/places`: returned durable seeded entries for Armenia, Lori Province, Vanadzor, and Yerevan.
- `/api/v1/search?q=Yerevan`: returned backend `postgres_local`, `48` media matches, and `4` track matches.
- `/api/v1/search?q=Armenia&limit=5`: returned place-cache matches for current assets.
- `/api/v1/search?q=Vanadzor&limit=5`: returned the cached Vanadzor place with `0` current media, expected for the bounded Yerevan real-peek set.
- `/api/v1/ai/status`: native sidecar `ok`; OCR model reports `/usr/bin/tesseract` and installed languages `eng`, `rus`, `hye`, `chi_sim`, `chi_tra`.
- Synthetic OCR smoke on `/tmp/cartolensia_ocr_live_smoke.png` through `POST http://127.0.0.1:19090/ocr-image`: succeeded with one recognized text block.
- The earlier low-confidence real-peek OCR smoke blocks created during this run were deleted via the new metadata-only endpoint; `/api/v1/assets/e8ba8b1b-2266-48a6-ba6d-a9171d2693ae/ocr` now returns no blocks.

### Verification

Passed:

- `.cartolensia/ai-venv/bin/python -m compileall services/ai/cartolensia_ai`
- `gofmt -w internal/catalog/catalog.go internal/catalog/extended_store.go internal/database/extended.go internal/server/server.go internal/server/server_test.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `git diff --check`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

### Known Limitations

- VMAF remains unavailable: current ffmpeg has SSIM/PSNR filters but not `libvmaf`; package-manager VMAF packages were not available in the earlier probe.
- Online geocoding remains intentionally disabled and unimplemented beyond cache scaffolding.
- OCR job history still records the earlier smoke job payload, but the persisted noisy OCR block metadata was removed.
- Long-caption workflows, safety/private hiding, WebDAV, and full multi-storage adapters remain future work.

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No generated files, OCR temp/cache files, model files, previews, transcodes, exports, database files, or caches were written under `/mnt/Models/rclone`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-07 Long Productization/Workflow Stabilization Run

Start state:

- Live real-peek app was available at `http://127.0.0.1:18080`.
- Current DB state remained bounded: `54` assets, `54` hashed, `48` photos, `2` videos, `4` tracks.
- `/api/v1/jobs?limit=5` showed a very large `ai_detect_faces` payload in list responses, confirming the Jobs UI noise/performance issue.

Implemented:

- File picker/modal stacking fix: dedicated Cartolensia modal backdrop with explicit z-index ordering.
- Track preview and overlay hardening: full-height track gallery layout, repeated OpenLayers `updateSize()`/fit after layout settles, and track controls above OpenLayers layers.
- Map cluster stability: single-click cluster auto-zoom was removed; cluster popups now stay open and expose an explicit `Zoom to cluster` action.
- Face management: added local face cluster store/API support, `Face Gallery` page, cluster naming, ignore actions, and asset-detail face box overlays.
- Jobs progress hardening: list responses summarize large AI payloads by default while individual job details can still expose the full payload.
- AV1 truthfulness: custom AV1 live-HLS presets now fail validation before spawning ffmpeg with an actionable browser-safety message.
- Transcoding metrics: added `GET /api/v1/transcoding/metrics/status` for SSIM/PSNR/libvmaf filter status and fixed ffmpeg encoder legend parsing.
- Photo/GPS alignment MVP: added bounded in-memory session API and `Geo Align` page; apply writes DB-only `manual_user` geotags; EXIF writeback is disabled for strict read-only storage.
- Video/track synchronized player MVP: added session/position API and `Video Track Player` page with video selection, track selection, timestamp mode, offset, and synchronized position payload.

Live validation:

- `/api/v1/faces/clusters`: returned `10` provisional face folders from existing detections.
- `/api/v1/transcoding/metrics/status`: returned SSIM/PSNR available and libvmaf unavailable.
- `/api/v1/jobs?limit=2`: large AI face job payload is summarized in list response.
- `/api/v1/media/56ff84bf-b7ae-4f23-baf4-ea0e6b5d633f/track-preview?max_points=500`: returned non-empty KML LineString preview.
- `POST /api/v1/geo-align/session`: created a bounded session over current indexed media and selected track without writing originals.
- `POST /api/v1/video-track-player/session`: created a session and returned a clear warning that the selected video has no `taken_at`.

Verification passed:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

Known limitations:

- Face Gallery currently uses provisional per-asset clusters until reviewed/named; true embedding-based clustering remains future work.
- Geo Align map is an MVP marker preview, not yet a full OpenLayers drag map. Apply is DB-only and EXIF writeback is intentionally disabled for `rclone_peek`.
- Video Track Player computes positions only when a video has usable `taken_at`; the current test video did not.
- AV1 live playback is disabled rather than attempted because the current HLS route is not browser-safe for AV1.
- Vite still reports the existing large chunk warning; build passes.

Safety confirmation:

- `/mnt/Models/rclone` was not modified.
- No generated files, AI models, previews, tiles, exports, transcodes, temp files, database files, or caches were written under `/mnt/Models/rclone`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-07 Supervised AI/Transcoding Approval Preflight

### Scope

- Short supervised probe/planning run only.
- No feature implementation started.
- No PyTorch install, model download, Docker image pull, real-data scan, DB reset, commit, or push.
- Added `docs/AI_MODEL_APPROVALS.md`.
- Updated `docs/NEXT_LONG_RUN_PLAN.md` with the current preflight findings and approval gates.

### Runtime State Queried

Commands/endpoints:

- `GET /api/v1/stats`
- `GET /api/v1/gps/tracks`
- `GET /api/v1/transcoding/capabilities`
- `GET /api/v1/transcoding/presets`
- `GET /api/v1/media/ce8b4866-33bd-474e-84ab-a0fd9388a313/stream-options`
- `GET /api/v1/ai/workers`
- `GET /api/v1/ai/status`
- `GET /api/v1/vector/status`
- `GET /api/v1/settings`
- `GET /api/v1/storages`
- `GET /api/v1/jobs?limit=20`

Current state:

- Stats: `54` assets, `54` hashed, `0` unhashed, `48` photos, `2` videos, `4` tracks, `619580406` indexed bytes.
- Storages: only `rclone_peek`, root `/mnt/Models/rclone`, mode `strict_read_only`.
- Tracks: `4` parsed summaries.
  - GPX tracks have real multi-hour durations.
  - KML tracks currently show synthetic short durations around `16-17` seconds after salvage parsing; next UI pass should label this timestamp policy clearly.
- Recent jobs are terminal/completed; no running job was observed.
- Transcoding:
  - ffmpeg and ffprobe are available at `/usr/bin`.
  - `h264_nvenc`, `hevc_nvenc`, `av1_nvenc`, CPU H.264/HEVC/AV1, VAAPI, and QSV encoders are advertised by ffmpeg.
  - App capability report currently marks `nvidia_smi`, `dev_dri`, `vaapi`, and `qsv` as true, but shell preflight history still notes `/dev/dri` access as environment-sensitive and VAAPI/QSV should stay unverified until a real device test passes.
  - Presets: `original`, `h264_720p_lan`, `h264_low_bitrate`, disabled `av1_low_bitrate`, and custom `nv-750k`.
  - `nv-750k` uses `h264_nvenc`, hardware `nvidia`, mode `bitrate`, parameter value `"750"`.
- AI:
  - Dummy worker is configured and healthy at `http://127.0.0.1:19090`.
  - Worker health reports `model_missing`; no real inference is configured.
  - CPU/NVIDIA/ROCm/Intel worker profiles are present but not configured.
- Vector:
  - `backend: none`.
  - `pgvector: false`.
  - No embeddings are generated in this build.
- Settings:
  - `auth.mode: dev_no_auth`.
  - cache root is repo-local `.cartolensia/realpeek-cache`.
  - runtime defaults include `indexing.hash_after_index=true`, `indexing.metadata_after_index=true`, `indexing.previews_after_index=false`, `map.tiles_enabled=true`, `transcode.session_ttl=2h`.

### NVENC Probe

Approved short probe command run:

```bash
ffmpeg -hide_banner -nostdin -y -t 2 \
  -i /mnt/Models/rclone/Cartolensia-photos/DCIM/Camera/PXL_20260516_163309946.mp4 \
  -map 0:v:0 -map 0:a? \
  -vf scale=w=1280:h=-2 \
  -c:v h264_nvenc -preset p5 \
  -b:v 750k -maxrate 750k -bufsize 1500k \
  -c:a aac -b:a 96k -pix_fmt yuv420p \
  -f hls -hls_time 1 -hls_playlist_type vod \
  -hls_segment_filename .cartolensia/realpeek-cache/transcode-test/nvenc-preflight-supervised/segment_%05d.ts \
  .cartolensia/realpeek-cache/transcode-test/nvenc-preflight-supervised/master.m3u8
```

Result:

- Exit code: `0`.
- Output path: `.cartolensia/realpeek-cache/transcode-test/nvenc-preflight-supervised`.
- Generated files before cleanup:
  - `master.m3u8`, `148` bytes.
  - `segment_00000.ts`, `222968` bytes.
- Playlist advertised one `2.033333` second VOD segment.
- `ffprobe` successfully read the playlist as HLS:
  - video: H.264 Main, `1280x720`, `30 fps`;
  - audio: AAC LC, `48000 Hz`, stereo.
- Test output directory was deleted after recording the result.

Interpretation:

- Native H.264 NVENC works for the current real-peek video when bitrate is normalized to `750k` and NVENC preset `p5` is used.
- The next implementation should focus on UI Apply/Test flow, session buffering/readiness, stderr display, and fallback behavior, not on basic NVENC availability.

### AI Approval Research

Web/package/model research was recorded in `docs/AI_MODEL_APPROVALS.md`.

Recommended staged approach:

1. Approve CUDA PyTorch/torchvision installation into `.cartolensia/ai-venv`.
2. Start real classification with torchvision MobileNetV3 Large or EfficientNet-B0.
3. Use OpenCV YuNet as the first face detector because model provenance is clearer and the model is small.
4. Treat facenet-pytorch MTCNN/recognition as optional/deferred because pretrained face weight provenance is less cleanly separated.
5. Add Falconsai NSFW detection only after explicit approval of its Hugging Face model license/provenance.
6. Add OpenCLIP LAION embeddings only after explicit approval of LAION-trained weight provenance and larger download size.
7. Defer BLIP captioning unless the user accepts the larger model and research-model caveats.

### Approvals Needed Before Next Long Run

Exact approvals requested:

- Python packages:
  - `torch`
  - `torchvision`
  - `opencv-python-headless`
  - optionally `transformers`
  - optionally `safetensors`
  - optionally `open-clip-torch`
- Model weights/files:
  - MobileNetV3 Large or EfficientNet-B0 classification weights.
  - OpenCV YuNet `face_detection_yunet_2023mar.onnx`.
  - Falconsai `nsfw_image_detection`, only if approved.
  - OpenCLIP `CLIP-ViT-B-32-laion2B-s34B-b79K`, only if approved.
  - Salesforce `blip-image-captioning-base`, only if approved/deferred decision changes.
- CUDA PyTorch:
  - approve or reject `.cartolensia/ai-venv/bin/python -m pip install torch torchvision --index-url https://download.pytorch.org/whl/cu128`.
- Docker:
  - image pulls remain deferred unless the user explicitly approves CUDA/PyTorch/ROCm/Intel AI images.
- Real-peek AI execution:
  - approve or reject running real AI inference on the current `54` real-peek assets after implementation. Safer default is synthetic fixtures only until UI review.
- NSFW/safety:
  - approve or reject the Falconsai model license/provenance and opt-in workflow.

### Safety Confirmation

- `/mnt/Models/rclone` was read only for the approved exact video input during the NVENC probe.
- No files were written, cached, exported, transcoded, dumped, or placed under `/mnt/Models/rclone`.
- The temporary HLS probe output was repo-local and was deleted.
- No new real-data prefixes were scanned.
- No missing marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-07 Autonomous Local AI Implementation Run

### Start State

- Live real-peek app: `http://127.0.0.1:18080`.
- AI sidecar target: `http://127.0.0.1:19090`.
- Storage: `rclone_peek`, root `/mnt/Models/rclone`, mode `strict_read_only`.
- Indexed real-peek state: `54` assets, `54` hashed, `48` photos, `2` videos, `4` parsed tracks.
- No new real-data prefix scan was run.

### Dependencies And Models

Installed into `.cartolensia/ai-venv`:

- `torch` / `torchvision` from the approved CUDA 12.8 PyTorch wheel index.
- `opencv-python-headless`.
- `transformers`.
- `safetensors`.
- `open-clip-torch`.
- `facenet-pytorch` and required transitive dependencies.

Cached under `.cartolensia/models`:

- Torchvision EfficientNet-B0 weights.
- Torchvision MobileNetV3 Large weights.
- OpenCV YuNet ONNX at `.cartolensia/models/opencv/face_detection_yunet_2023mar.onnx`.
- Falconsai `nsfw_image_detection`.
- OpenCLIP `laion/CLIP-ViT-B-32-laion2B-s34B-b79K`.
- Salesforce `blip-image-captioning-base`.

Approximate local sizes:

- `.cartolensia/ai-venv`: `8.2G`.
- `.cartolensia/models`: `2.8G`.

### Implemented

- Real local AI sidecar backend in `services/ai/cartolensia_ai/models/real.py`.
- Sidecar endpoints:
  - `/classify-image`;
  - `/detect-faces`;
  - `/safety-nsfw`;
  - `/describe-image`;
  - `/embed-image`;
  - `/embed-text`.
- Device selection uses approved PyTorch/OpenCLIP/Transformers ecosystems and reports `cuda:0` on this host.
- Backend AI jobs:
  - `/api/v1/ai/jobs/classify`;
  - `/api/v1/ai/jobs/faces`;
  - `/api/v1/ai/jobs/safety`;
  - `/api/v1/ai/jobs/embed`;
  - `/api/v1/ai/jobs/describe`.
- Persistence:
  - `asset_tags`;
  - `ai_predictions`;
  - `face_detections`;
  - `asset_embeddings`.
- Safety workflow:
  - Safety predictions are stored as predictions, not truth.
  - Assets over threshold are added to virtual album `Potentially Unsafe`; the current run found none over threshold.
  - Original files are never moved, hidden, deleted, or modified.
- Vector search:
  - JSON/PostgreSQL local fallback.
  - Bounded brute-force cosine search for current small collections.
  - `/api/v1/search/vector?q=...`.
- Search integration:
  - `tag:`;
  - `category:`;
  - `safety:`;
  - `face:`;
  - `caption:`.
- Frontend:
  - Base AI page now has real scoped action buttons for classification, faces, safety, embeddings, and captions.
  - Asset detail shows AI tags, predictions, face detections, safety status, captions, and embedding summaries.
  - Advanced transcode settings use a modal-style focus isolation path.
- API quality fixes:
  - AI prediction reads now compare UUID asset IDs safely in PostgreSQL.
  - Embedding summaries are returned instead of full vectors.
  - Repeated classifier runs read back latest unique labels instead of noisy duplicates.

### Live AI Validation

Ran only against the current indexed real-peek scope approved by the user:

- Classification:
  - targets `54`;
  - processed `48` photos;
  - skipped `4` tracks and `2` videos.
- Safety:
  - processed `48` photos;
  - skipped `4` tracks and `2` videos;
  - stored `96` safety predictions plus safety tags;
  - `0` assets above review threshold.
- Face detection:
  - processed `48` photos;
  - stored `41` local-only face detections/predictions.
- Embeddings:
  - processed `48` photos;
  - stored `48` 512-dimensional OpenCLIP image embeddings.
- Captioning:
  - smoke-tested one selected photo;
  - stored caption `a brick path` as a generated suggestion.

Database AI counts after validation:

- `89` asset tags.
- `383` AI prediction rows.
- `41` face detections.
- `48` asset embeddings.

Endpoint checks:

- `/api/v1/ai/workers` reports `ai-local` configured and healthy with loaded models.
- The sidecar was restarted after validation; current worker health may show models as lazy/unloaded until the next inference request, but the validated outputs remain in PostgreSQL.
- `/api/v1/vector/status` reports `local_json_bruteforce`.
- `/api/v1/search/vector?q=brick%20path` returns OpenCLIP matches.
- `/api/v1/search?q=tag:safety:normal` returns the `48` safety-normal photos.
- `/api/v1/search?q=caption:brick` returns the single BLIP-captioned smoke-test asset.

### Safety Confirmation

- No writes to `/mnt/Models/rclone`.
- No model files, previews, transcodes, exports, dumps, temporary files, database files, or AI outputs were placed under `/mnt/Models/rclone`.
- AI read current assets through Cartolensia read-only localhost media URLs.
- No new real-data prefixes were scanned.
- No missing marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

### Verification

Passed:

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`
- `.cartolensia/ai-venv/bin/python -m compileall services/ai/cartolensia_ai`
- Synthetic sidecar HTTP smoke against `/tmp/cartolensia_ai_smoke.jpg`:
  - `classify-image`: `ok`;
  - `safety-nsfw`: `ok`;
  - `embed-image`: `ok`, 512 dimensions;
  - `describe-image`: `ok`;
  - `detect-faces`: `ok`, zero faces on synthetic drawing.

Frontend build warning:

- Vite still warns that the main JS chunk is larger than 500 kB. This is an existing bundling/code-splitting issue, not a test failure.

Known limitations:

- Face grouping/identity is not implemented; detections are local-only boxes and confidence values.
- The safety classifier is a prediction pipeline and can be wrong; review workflow remains intentionally conservative.
- BLIP captions are generated suggestions; only one current real-peek asset was captioned in this run.
- Vector search uses the local brute-force JSON/PostgreSQL fallback; pgvector is still optional/future for large collections.
- AI job execution is currently synchronous/bounded through API handlers rather than durable worker jobs; this is acceptable for the 54-asset real-peek validation but should move to background jobs before large-library inference.

## 2026-06-07 Focused UI Stabilization And Real-Peek Polish

### Scope

- Focused stabilization only: map/track preview controls, Settings layout, Map debug visibility, and Base AI/vector status clarity.
- No new real-data prefixes were scanned.
- PostgreSQL was not reset.
- No new models or Docker images were downloaded.
- No commit was made and no push was done.

### Live Audit

Queried the live real-peek service before changes:

- `/api/v1/stats`: `54` assets, `54` hashed, `48` photos, `2` videos, `4` tracks, `619580406` indexed bytes.
- `/api/v1/map/status`: GeoJSON backend, `48` geotagged assets, `4` tracks, Cartolensia OSM tile proxy, screen-distance clustering, no warnings.
- `/api/v1/gps/tracks`: `4` parsed GPX/KML summaries.
- `/api/v1/ai/status`: local AI sidecar configured at `127.0.0.1:19090`, CUDA device visible through the worker, local model cache under `.cartolensia/models`.
- `/api/v1/ai/workers`: `ai-local` healthy; Docker-style CPU/NVIDIA/ROCm/Intel profiles remain optional/not configured.
- `/api/v1/vector/status`: local JSON/brute-force fallback active; pgvector optional/not enabled.
- `/api/v1/jobs?limit=20`: recent jobs were terminal; no new indexing, metadata, preview, or AI jobs were started by this stabilization run.
- `/api/v1/settings` and `/api/v1/settings/schema`: runtime/pending settings were available; `map.tiles_enabled` was true and persistent preview generation remained false.

### Fixes Implemented

- Track preview/detail maps now use the existing Cartolensia OSM tile proxy as an optional background:
  - GPS/KML Track detail page;
  - track gallery overlay preview;
  - shared local preference for OSM-on/off.
- Added per-map layer cogwheel menus:
  - OSM tiles on/off;
  - track layer on/off;
  - photo/asset layer on/off on the main Map;
  - fit-to-track / fit-to-features actions.
- Map raw GeoJSON is hidden by default behind a Bootstrap switch and persists locally through `localStorage`.
- Main Map viewport is larger when debug is hidden, and map popup close buttons now include a Bootstrap icon.
- Settings page visual layout was cleaned up:
  - runtime and restart-required settings are separate cards;
  - responsive form grids, section headers, help text, and grouped action buttons;
  - GPS/KML, Map/Tiles, Preview Cache, Transcoding, and AI/Vector tabs now have focused status/control cards.
- Preview Cache clear action is confirmation-protected.
- Base AI page now shows:
  - worker cards with status badges;
  - model cards for classifier, face detector, safety, embeddings, and captions;
  - visible running state, latest action result, and recent browser-session AI actions;
  - vector store card showing local fallback, pgvector status, embedded asset count, and dimensions;
  - Configure Vector Store opens Settings → AI/Vector and highlights the relevant controls.
- Backend status hardening:
  - `/api/v1/ai/status` now includes AI tag/prediction/face/embedding/safety counts.
  - `/api/v1/vector/status` now includes embedded asset count, embedding count, vector dimensions, and a pgvector/fallback note.
  - memory store list methods now match PostgreSQL semantics for all-asset AI/tag/face/embedding listing.

### Verification

Passed:

- `gofmt -w internal/catalog/extended_store.go internal/server/server.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

Notes:

- Vite still reports the existing large bundle warning for the OpenLayers/HLS/Bootstrap bundle; build passes.
- Attempting to keep the compiled binary alive with `nohup` did not survive the sandboxed shell. The live app was restarted through the already-working approved `go run ./cmd/cartolensia -config .cartolensia/runtime/realpeek.yaml` path.

### Live Validation After Restart

- `http://127.0.0.1:18080/api/v1/health` returned `ok`.
- `/api/v1/stats`: still `54` assets and `54` hashed.
- `/api/v1/gps/tracks`: still `4` parsed tracks.
- `/api/v1/map/status`: still `48` geotagged assets and `4` tracks.
- `/api/v1/ai/status`: `89` AI tags, `347` latest prediction rows, `82` face detections, `48` embeddings, `0` safety candidates.
- `/api/v1/vector/status`: `local_json_bruteforce`, `48` embedded assets, `48` embeddings, `512` dimensions, pgvector optional/not enabled.

### Remaining Issues

- Manual browser inspection is still needed for the new layer menus and visual spacing.
- OSM-backed generated static track thumbnail compositing remains future work; this pass adds OSM background for interactive OpenLayers track previews.
- AI API actions remain synchronous/bounded from the UI; durable AI jobs are still a future hardening task.

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No previews, tiles, transcodes, model files, exports, database files, or temp files were written under `/mnt/Models/rclone`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-07 Focused GPU/AI/Transcoding Productization Sprint

### Scope

- Focused bugfix/productization pass only: GPU status clarity, AI action visibility, AI Classification page, track preview rendering, settings file picker, close-button polish, and Transcoding page structure.
- The live real-peek PostgreSQL database was preserved.
- No real-data scans were started.
- No new AI models were downloaded.
- No commit was made and no push was done.

### Live Audit And Probes

Queried the live service and host capability probes:

- `/api/v1/stats`: `54` assets, `54` hashed, `48` photos, `2` videos, `4` tracks, `619580406` indexed bytes.
- `/api/v1/map/status`: `48` geotagged assets, `4` track geometries, OSM through the Cartolensia tile proxy, screen-distance clustering.
- `/api/v1/gps/tracks`: `4` parsed GPX/KML summaries.
- `/api/v1/ai/status`: native sidecar healthy at `127.0.0.1:19090`, active device `cuda:0`, `89` AI tags, `347` prediction rows, `82` face detections, `48` embeddings, `0` safety candidates.
- `/api/v1/ai/workers`: `ai-local` is the active native CUDA worker; optional Docker profiles are reported separately. `ai-nvidia` is now clearly labeled as the optional Docker NVIDIA profile, not the native CUDA worker.
- `/api/v1/vector/status`: `local_json_bruteforce`, `48` embedded assets, `512` dimensions, pgvector optional/not enabled.
- `/api/v1/settings/schema`: runtime and restart-required tabs are available, including Indexing, Preview, Map, GPS/KML, Transcoding, AI/Vector, Plugins, and Raw config.
- `/api/v1/files/browse`: returns allowlisted roots only until a root is explicitly selected; the real archive storage root is marked read-only with a warning.
- `/api/v1/media/377c2280-f0b3-407c-9ee2-580503c2b5b1/track-preview?max_points=20`: returned a non-empty GPX LineString preview with `17522` source points.
- `/api/v1/transcoding/capabilities`: ffmpeg/ffprobe available; native NVIDIA, `/dev/dri`, VAAPI, and QSV capabilities are reported from probes.
- `/api/v1/transcoding/presets`: built-ins plus custom `NV 750k` preset are visible.

Host probes:

- `docker info --format '{{json .Runtimes}}'`: Docker reports the `nvidia` runtime.
- `docker run --rm --gpus all nvidia/cuda:12.8.0-base-ubuntu24.04 nvidia-smi`: approved probe succeeded and exposed the RTX 3090 Ti inside Docker.
- `nvidia-smi`: native host probe succeeded outside the sandbox and reported NVIDIA GeForce RTX 3090 Ti.
- `ls -l /dev/dri`: render nodes are present outside the sandbox.
- `ffmpeg -hide_banner -hwaccels`: CUDA, VAAPI, QSV, DRM, OpenCL, and Vulkan are listed.
- `ffmpeg -hide_banner -encoders | grep -Ei 'nvenc|vaapi|qsv|amf|264|265|hevc|av1'`: NVIDIA H.264/HEVC/AV1 encoders plus VAAPI/QSV encoders are listed.

### Fixes Implemented

- AI/GPU status model:
  - `/api/v1/ai/status` now distinguishes the native local worker, Docker profile workers, device policy, Docker NVIDIA runtime availability, native NVIDIA availability, and CPU fallback.
  - Base AI shows `Native CUDA: available`, `Docker NVIDIA profile: available/not configured`, and active device `cuda:0` rather than implying NVIDIA is unavailable.
  - Settings -> AI/Vector includes a device preference selector and model-cache path picker entry point.
- AI action visibility:
  - AI action endpoints now create visible job records even though the bounded action executes synchronously through the API handler.
  - Action results include a `job_id` so the Base AI page can link progress/results to Jobs.
  - Job records include kind, status, target/processed/skipped counts, stored-output counters, and errors/logs.
- AI Classification page:
  - Replaced the stub with a real dashboard page.
  - Added summary cards, tag/category browser, latest predictions table, safety panel, face detection table, vector search panel, and links into Explorer/assets/albums.
  - Added backend list endpoints: `/api/v1/ai/summary`, `/api/v1/ai/tags`, `/api/v1/ai/faces`, `/api/v1/ai/safety`, and bounded all-asset reads for `/api/v1/ai/predictions`.
- Track previews:
  - Hardened track preview rendering in the GPS/KML manager and gallery overlay.
  - Track vector layers now draw above OSM tiles with a visible outline/bright line style.
  - Maps fit after the vector source and OpenLayers target are ready.
  - Status overlays report loaded features/point counts or a load error.
- Safe file/folder picker:
  - Added `GET /api/v1/files/browse` with allowlisted roots only, traversal rejection, and read-only listing.
  - Added a reusable Bootstrap-like picker modal for storage roots, cache dirs, model cache dir, export dir, and ffmpeg/ffprobe-style path settings.
  - The picker does not write files and does not scan/index selected paths.
- Close-button polish:
  - Gallery, map popup, advanced transcode modal, and file picker close buttons now include Bootstrap Icons and use a red/danger hover state.
- Transcoding page:
  - Added structured tabs for Capabilities, Presets, Auto-selection Rules, Command Templates, Job Planner, and Metrics.
  - Added visible preset table, custom rule/template drafts, safe command-template validation feedback, cache-only planner copy, and metrics availability/status cards.
  - Kept long transcode execution out of this sprint.
- GPU integration settings/status:
  - Transcoding capabilities now include Docker/NVIDIA hints in addition to ffmpeg encoder discovery.
  - UI labels clarify that AMD/Intel paths are only usable when device access and encoders are actually available.

### Verification

Passed:

- `gofmt -w internal/server/server.go internal/transcoding/transcoding.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

Notes:

- Vite still warns that the main JavaScript chunk is larger than 500 kB; the build passes.
- No browser automation was run in this sprint; manual inspection should verify the new file picker modal, track preview line visibility, AI Classification page layout, and Transcoding page tabs.

### Live Validation After Restart

- Live app was restarted with `go run ./cmd/cartolensia -config .cartolensia/runtime/realpeek.yaml`.
- `http://127.0.0.1:18080/api/v1/health` returned `ok`.
- `/api/v1/ai/status`, `/api/v1/ai/summary`, `/api/v1/ai/tags`, `/api/v1/ai/predictions?limit=3`, `/api/v1/files/browse`, `/api/v1/gps/tracks`, `/api/v1/media/{track_asset_id}/track-preview`, `/api/v1/transcoding/capabilities`, and `/api/v1/transcoding/presets` all returned live data after restart.

### Remaining Issues

- AI actions are visible as durable job records, but execution still happens inside bounded API handlers. Moving AI execution into long-running leased workers remains future hardening.
- Transcoding page now has management surfaces and validation scaffolding; full durable transcode job planning/execution and VMAF/SSIM/PSNR sample runs remain future work.
- Manual browser inspection is still needed for the new UI surfaces.

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No generated files, AI models, previews, tiles, exports, transcodes, temp files, database files, or caches were written under `/mnt/Models/rclone`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-07 Long Productization/Workflow Stabilization Run

### Start State

- Live app: `http://127.0.0.1:18080`.
- Storage: `rclone_peek`, root `/mnt/Models/rclone`, mode `strict_read_only`.
- Live counts remained `54` assets, `54` hashed, `48` photos, `2` videos, and `4` tracks.
- PostgreSQL was preserved; no reset was performed.

### Implemented

- Fixed the file picker modal backdrop regression:
  - the modal and dialog now sit above the backdrop with explicit z-index rules;
  - the backdrop no longer blocks clicks on the file/folder picker.
- Improved track preview/gallery layout and reliability:
  - gallery track maps now fill the available overlay space;
  - OpenLayers `updateSize`/fit timing runs after the overlay and features settle;
  - track line visibility is preserved above OSM tiles and dark fallback backgrounds.
- Stabilized map cluster interaction:
  - single-click opens a cluster popup and no longer immediately zooms/refits;
  - cluster zoom is now an explicit popup action;
  - popup state survives normal refreshes unless the feature truly disappears.
- Added Face Gallery MVP:
  - backend face cluster APIs;
  - provisional face folders when embedding clusters are not yet assigned;
  - cluster naming;
  - cluster asset listing;
  - detection ignore metadata;
  - asset-detail face boxes.
- Made AI job lists more scalable:
  - large AI job payloads are summarized by default;
  - callers can request full payloads explicitly with `full_payload=true`.
- Made AV1 behavior truthful:
  - AV1 live HLS sessions are rejected with a clear validation reason instead of timing out after expensive startup attempts;
  - H.264/NVENC remains the usable live-streaming path.
- Added transcoding metrics capability status:
  - `/api/v1/transcoding/metrics/status`;
  - current ffmpeg reports `ssim` and `psnr`, but not `libvmaf`.
- Added Geo Align MVP:
  - scoped sessions over selected/current media and optional tracks;
  - candidate positions from existing geotags and track interpolation;
  - DB-only manual geotag override application;
  - EXIF writeback disabled for strict read-only storage.
- Added Video Track Player MVP:
  - scoped video/track sessions;
  - position interpolation endpoint;
  - clear warning when the selected video lacks reliable `taken_at` metadata.

### Live Validation

- `GET /api/v1/health`: returned `ok`.
- `GET /api/v1/stats`: returned `54` assets and `54` hashed.
- `GET /api/v1/faces/clusters`: returned face folders for the current bounded sample.
- `GET /api/v1/transcoding/metrics/status`: returned `ssim` and `psnr` available, `libvmaf` unavailable.
- `GET /api/v1/media/{track_asset_id}/track-preview`: returned non-empty track geometry for checked current tracks.
- `POST /api/v1/geo-align/session`: created a scoped session without scanning or writing originals.
- `POST /api/v1/video-track-player/session`: created a scoped session and reported missing timestamp metadata for the current sample video.
- The live app was restarted at `http://127.0.0.1:18080` with the same `.cartolensia/runtime/realpeek.yaml` and existing PostgreSQL data.

### Verification

Passed:

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

### Known Limitations

- Face folders are provisional until local embedding-based clustering is hardened.
- Geo Align has safe backend/session behavior and a first UI, but the full OpenLayers shift-drag marker editor remains future work.
- Video Track Player needs user-entered start/end timestamp or offset controls for videos without reliable capture timestamps.
- AV1 live playback remains disabled until a verified browser-compatible AV1 WebM/fMP4 or HLS fMP4 path is implemented.
- Browser automation was not added in this pass; manual inspection should verify modal layering, track overlay sizing, cluster popup stability, face gallery, Geo Align, and Video Track Player pages.

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No generated files, AI models, previews, tiles, exports, transcodes, temp files, database files, or caches were written under `/mnt/Models/rclone`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.
## 2026-06-07 Geo Align/Faces/Tracks/AV1/Search Stabilization

### Scope

- Focused run against the current live real-peek service and existing temporary PostgreSQL data.
- Fixed the user-visible issues on Geo Align, Face Gallery/asset faces, GPS/KML track detail maps, AV1 playback/transcoding, and universal search.
- No new real-data scan was run.
- PostgreSQL was not reset.

### Implemented

- Geo Align page:
  - replaced the static grid-only preview with an OpenLayers map using the existing Cartolensia OSM tile proxy;
  - added a left sidebar with scope, track IDs, layer toggles, status legend, session metrics, DB-only apply controls, and modified-media list;
  - added photo/video marker thumbnails directly in the OpenLayers marker layer;
  - kept Write EXIF disabled in strict-read-only real-peek mode.
- GPS/KML track detail and track previews:
  - forced OSM tile visibility defaults back on for track preview contexts;
  - added a fallback SVG track path rendered from the same GeoJSON so track geometry remains visible even if OpenLayers tiles or sizing fail;
  - added repeated `updateSize()`/fit calls after the map target and vector features settle;
  - added click-to-point-info on the track detail map, returning clicked coordinates, nearest point, distance, timestamp/relative time when available, speed, and elevation.
- Face Gallery and asset face management:
  - face-folder cards now include representative image previews from cluster metadata;
  - fixed face asset tile layout so images/text/buttons stay inside the tile;
  - added a face search field on Face Gallery;
  - asset detail now has an optional face-rectangle toggle;
  - asset detail now has a scrollable face record list with thumbnail, name, confidence, and a Delete action;
  - selecting a face record highlights the rectangle on the image;
  - clicking a face name or thumbnail opens the corresponding Face Gallery folder;
  - added manual face rectangle drawing on asset detail and a backend endpoint to save the new detection/name as metadata;
  - Delete marks a detection ignored/deleted in metadata rather than touching originals.
- Universal Search:
  - added a Search page and sidebar entry;
  - search accepts plain words and searches filenames, paths, extensions, hashes, dates, EXIF/camera metadata, tags/categories, AI predictions/captions, albums, faces, and GPS/KML tracks;
  - `/api/v1/search` now returns both multimedia results and track results with match explanations;
  - search uses inclusive plain-token matching so a simple word returns anything matching by at least one metadata surface.
- AV1 playback/transcoding:
  - AV1 presets are now enabled when ffmpeg exposes a software or hardware AV1 encoder;
  - current machine detects `libsvtav1`, so the built-in AV1 preset is available as CPU/WebM;
  - AV1 no longer uses the broken HLS route. It creates a cache-scoped WebM output and the frontend switches to that browser URL only after the session is ready/finished;
  - H.264/NVENC HLS routes remain unchanged.

### Live Validation

- `GET /api/v1/health`: `ok`.
- `GET /api/v1/stats`: `54` assets, `54` hashed, `48` photos, `2` videos, `4` tracks.
- `GET /api/v1/map/status`: `48` geotagged assets, `4` track geometries, Cartolensia OSM tile proxy enabled.
- `GET /api/v1/media/56ff84bf-b7ae-4f23-baf4-ea0e6b5d633f/track-preview?max_points=1200`: returned one LineString feature with non-empty coordinates for the KML track.
- `GET /api/v1/gps/tracks/56ff84bf-b7ae-4f23-baf4-ea0e6b5d633f/point-info?lat=40.19&lon=44.49`: returned nearest point details, distance, timestamp, relative time, speed, and elevation.
- `GET /api/v1/search?q=jpg&limit=5`: returned photo media results matched by extension/filename/path.
- `GET /api/v1/search?q=20260516&limit=5`: returned photo/track media plus track result rows.
- `GET /api/v1/faces/clusters`: returned face folders with representative asset IDs/names for card previews.
- `GET /api/v1/media/ce8b4866-33bd-474e-84ab-a0fd9388a313/stream-options`: returned AV1 low bitrate as available, CPU, `libsvtav1`, WebM.
- `POST /api/v1/media/ce8b4866-33bd-474e-84ab-a0fd9388a313/transcode-session` with `av1_low_bitrate`: succeeded on the current 7-second video.
- `GET /api/v1/media/transcode-sessions/86955015-121d-4d4a-93fa-36beb5dcc77b/output.webm`: returned `Content-Type: video/webm`, `Accept-Ranges: bytes`, `Content-Length: 5679003`.

### Verification

Passed:

- `gofmt -w internal/server/server.go internal/server/transcode_sessions.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `docker compose -p cartolensia_realpeek -f docker-compose.yml -f docker-compose.dev.yml ps`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

### Known Limitations

- Face folders are still provisional until embedding-based merge/split clustering is hardened.
- Manual face creation uses the displayed image coordinate model; browser inspection should verify rectangles line up on all image aspect ratios.
- Geo Align has OpenLayers markers and sidebar controls; shift-drag map editing remains a future hardening target.
- AV1 WebM works for the current short sample. Longer videos may need a durable queued job/offline mode rather than waiting in a request path.
- Browser automation was not run; manual inspection should verify map tile visibility, face rectangle editing, and AV1 playback controls.

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No generated files, AI models, previews, tiles, exports, database files, or caches were written under `/mnt/Models/rclone`.
- The AV1 test wrote only to `.cartolensia/realpeek-cache/transcode/86955015-121d-4d4a-93fa-36beb5dcc77b/output.webm`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-07 Overnight Stabilization Continuation

This continuation kept the existing live real-peek PostgreSQL database and did not run any discovery, missing marking, or reset. The stop sentinel `.cartolensia/STOP_AFTER_CURRENT_TASK` was checked and was absent during the work.

### Baseline

- `GET /api/v1/stats`: `54` assets, `54` hashed, `48` photos, `2` videos, `4` tracks, `619580406` bytes.
- `GET /api/v1/gps/tracks`: `4` parsed GPX/KML summaries.
- `GET /api/v1/map/status`: `48` geotagged assets, `4` tracks, Cartolensia OSM tile proxy enabled.
- `GET /api/v1/ai/status`: local vector fallback active with `48` embeddings, `89` tags, `347` predictions, `123` face detections; native sidecar was not running at audit time.
- `GET /api/v1/jobs?limit=30`: latest AI face-detection job visible and succeeded.
- `GET /api/v1/transcoding/metrics/status`: `ssim` and `psnr` available; `libvmaf` unavailable in current ffmpeg.
- `GET /api/v1/vector/status`: `local_json_bruteforce`, `512` dimensions, `48` embedded assets.
- `GET /api/v1/search?q=Yerevan`: returned `1` place match, `48` media results, and `4` track results from the local place cache.

### Implemented

- Added a read-only `GET /api/v1/search/places` endpoint exposing the local place cache and current bbox match counts.
- Added a Search/Places Settings tab:
  - default search limit;
  - cache-only geocoder mode;
  - online geocoding toggle shown off by default;
  - local provider status;
  - visible Yerevan/Armenia cache cards with match counts and quick “Search this place” action.
- Consolidated local place definitions so universal search and place-cache status use the same source of truth.
- Extended tests to cover:
  - Yerevan local place search;
  - `GET /api/v1/search/places`;
  - Search/Places settings tab/schema visibility.

### Verification

Passed after this continuation:

- `gofmt -w internal/server/server.go internal/server/server_test.go internal/server/settings.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

### Known Limitations

- The place cache is currently built in (`Yerevan`, `Armenia`) and cache-only. A durable `place_cache` table and user-triggered online geocoder import remain future work.
- The native AI sidecar was not running during the latest audit; stored AI predictions/tags/faces remain visible from PostgreSQL metadata.
- Browser automation was not run in this continuation; manual inspection should verify Geo Align shift-drag, track map interactivity, and Search/Places settings ergonomics.

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No generated files, models, previews, transcodes, exports, database files, or caches were written under `/mnt/Models/rclone`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-08 Search/OCR/Map Productization Continuation

This continuation kept the real-peek archive in `strict_read_only` mode and did not run discovery, missing marking, model downloads, Docker pulls, or package installs. The stop sentinel `.cartolensia/STOP_AFTER_CURRENT_TASK` was checked and was absent during the work.

### Baseline

- `GET /api/v1/stats`: `54` assets, `54` hashed, `48` photos, `2` videos, `4` tracks.
- `GET /api/v1/search?q=Yerevan`: returned the current bounded Yerevan-area set through the local place cache.
- `GET /api/v1/search?q=Vanadzor`: returned no media before this run because Vanadzor/Lori were not in the local place cache.
- `GET /api/v1/settings/schema`: Search/Places tab was present.

### Implemented

- Search backend hardening:
  - added a small `SearchBackend` interface and explicit `postgres_local` backend identity in `/api/v1/search`;
  - kept Elasticsearch/OpenSearch intentionally out of this run;
  - search responses now disclose backend mode for future backend pluggability.
- Local place cache and reverse-place display:
  - added local entries for `Vanadzor` and `Lori Province` alongside existing `Yerevan` and `Armenia`;
  - asset detail now returns computed `places` rows for stored `asset_geo` and EXIF/metadata coordinates;
  - asset detail UI now shows cache-only reverse-geocoded places, coordinate source, provider/source, and coordinates.
- OCR foundation:
  - added `ocr_image` as a manual Base AI action routed to the sidecar `/ocr-image` contract;
  - added `GET /api/v1/assets/{id}/ocr`;
  - added `GET /api/v1/ocr/runs`;
  - OCR text blocks are represented through existing AI prediction storage with bounding boxes, making OCR text searchable without a risky migration in this stabilization slice;
  - asset detail now has OCR box visibility, OCR text list, click-to-highlight, and copy text controls.
- Gallery/map polish:
  - fixed the gallery track ghosting cause by removing the static SVG fallback overlay from the live OpenLayers map path;
  - gallery track maps now dispose/recreate cleanly on asset change and close;
  - map viewport sizing was increased;
  - Geo Align switches/sidebar were widened and the map uses more viewport height.

### Verification

Passed:

- `gofmt -w internal/server/server.go internal/server/server_test.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`

Pending final/live checks for this continuation:

- `git diff --check`
- `go test ./...`
- focused live API checks after restart.

Final verification completed:

- `git diff --check`
- `go test ./...`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`

Live validation after restart:

- `GET /api/v1/stats`: still `54` assets, `54` hashed, `48` photos, `2` videos, `4` tracks.
- `GET /api/v1/search?q=Yerevan&limit=3`: returned backend `postgres_local`, backend mode `fts_trigram_ready_metadata_place_ai_ocr`, `48` media matches, and `4` track matches.
- `GET /api/v1/search?q=Vanadzor&limit=3`: returned a local Vanadzor place-cache match with `0` current real-peek media assets, which is expected for the current bounded Yerevan dataset.
- `GET /api/v1/ocr/runs`: returned the OCR contract with English/Russian/Armenian/Chinese language codes and `0` stored real-peek OCR blocks.
- `GET /api/v1/assets/e8ba8b1b-2266-48a6-ba6d-a9171d2693ae`: returned cache-only place rows for Yerevan and Armenia from `asset_geo` and EXIF metadata.

### Known Limitations

- OCR runtime installation was not performed in this pass. The app now has the job/API/UI contract and searchable metadata storage; Tesseract/language-pack installation remains the next supervised system-package step.
- Place lookup is still local/cache-only. No public geocoder is called automatically.
- `place_cache` and `asset_places` durable database tables remain future work; current place rows are computed from local cache plus existing geotag metadata.
- Long-caption workflows, safety/private hiding, WebDAV, and full multi-storage adapters remain future hardening tasks.

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No generated files, OCR cache, model files, previews, transcodes, exports, database files, or caches were written under `/mnt/Models/rclone`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-08 Offline Distribution Packaging Run

This run added the first production-oriented offline packaging path. It did not scan new data, reset PostgreSQL, download models, pull Docker images, or write to `/mnt/Models/rclone`.

### Implemented

- Added `scripts/dist/build-offline-linux.sh`:
  - builds the Vue WebUI and Go backend;
  - stages launcher scripts, offline configs, docs, project license, third-party notices, dependency manifests, and optional AGPL source snapshot;
  - can bundle ffmpeg, ffprobe, Tesseract, OCR language data, PostgreSQL runtime files, Python runtime/site packages, and a reviewed model cache;
  - supports AI bundle flavors `none`, `runtime`, `cpu`, and `cuda128`;
  - writes `.7z`, `.7z.sha256`, and release-notes outputs under `dist/`;
  - copies runtime trees without preserving owner/group/perms so archives are portable across build filesystems.
- Added `.github/workflows/offline-release.yml`:
  - manual `workflow_dispatch` release build;
  - installs runner-side distribution dependencies, including p7zip, ffmpeg, Tesseract language packs, PostgreSQL, and `libpq-dev`;
  - runs Go/WebUI verification before packaging;
  - builds the offline Linux x86_64 archive;
  - uploads workflow artifacts;
  - creates or updates a GitHub release and attaches the `.7z` plus checksum.
- Added `docs/DISTRIBUTION.md` and `make dist-offline-linux`.
- Linked the offline distribution docs from `README.md`.
- Added a narrow `.gitignore` exception so `scripts/dist/` packager code is tracked while generated repo-root `dist/` archives remain ignored.

### Package Smoke Results

Minimal smoke package:

- Command: `CARTOLENSIA_DIST_VERSION=local-smoke CARTOLENSIA_DIST_AI_FLAVOR=none CARTOLENSIA_DIST_INCLUDE_TOOLS=0 CARTOLENSIA_DIST_INCLUDE_POSTGRES=0 CARTOLENSIA_DIST_INCLUDE_SOURCE=0 bash scripts/dist/build-offline-linux.sh`
- Archive: `dist/cartolensia-local-smoke-linux-x86_64-offline.7z`
- Size: about `3.9 MiB`
- Verified archive contents include the backend binary, WebUI assets, launcher scripts, configs, docs, and notices.

Tools/OCR smoke package:

- Command: `CARTOLENSIA_DIST_VERSION=local-tools-smoke CARTOLENSIA_DIST_AI_FLAVOR=none CARTOLENSIA_DIST_INCLUDE_TOOLS=1 CARTOLENSIA_DIST_INCLUDE_POSTGRES=0 CARTOLENSIA_DIST_INCLUDE_SOURCE=0 bash scripts/dist/build-offline-linux.sh`
- Archive: `dist/cartolensia-local-tools-smoke-linux-x86_64-offline.7z`
- Size: about `90 MiB` compressed, `318 MiB` staged.
- SHA256: `4d90c4d7d7fef9a258e86d16924cf34e6ce5011aae6cd5e750e68763d605e21c`
- Bundled tool check passed:
  - staged Tesseract lists `eng`, `rus`, `hye`, `chi_sim`, `chi_tra`, and `osd`;
  - staged ffmpeg/ffprobe start successfully with copied libraries.

### Verification

Passed:

- `bash -n scripts/dist/build-offline-linux.sh`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- minimal offline package smoke build
- tools/OCR offline package smoke build
- staged Tesseract language-data check
- staged ffmpeg/ffprobe startup check

### Known Limitations

- The offline archive is Linux x86_64 focused. Additional target OS/architecture packages need separate runners and packaging logic.
- GPU drivers cannot be bundled; CUDA AI packages still require compatible host GPU drivers.
- Model weights are included only when explicitly enabled and a license-reviewed `.cartolensia/models` cache is present.
- The package builder records dependency manifests and Debian copyright files where available, but public redistribution still needs legal review, especially for ffmpeg codec flags, CUDA wheels, and model weights.
- GitHub-hosted runners cannot include private local model caches unless the release process explicitly provides them.

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No generated files, model files, OCR cache, transcodes, exports, DB files, or package outputs were written under `/mnt/Models/rclone`.
- Package outputs were written under repo-local `dist/`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-08 Component Manager And Asset AI Actions Run

This run implemented a first-class component registry and connected asset-detail AI actions to the existing bounded AI job path. It did not download unreviewed binaries, scan new real data, reset PostgreSQL, or write to `/mnt/Models/rclone`.

### Implemented

- Added persistent component management:
  - migration `010_components.sql` for `components` and `component_events`;
  - memory and PostgreSQL store support;
  - default records for FFmpeg/FFprobe, Tesseract/language packs, VMAF, Python AI runtime, PyTorch/torchvision, OpenCV YuNet, EfficientNet-B0, MobileNetV3, Falconsai NSFW, OpenCLIP, BLIP, and facenet-pytorch.
- Added component APIs:
  - `GET /api/v1/components`;
  - `GET /api/v1/components/status`;
  - `GET /api/v1/components/{key}`;
  - `POST /api/v1/components/{key}/check`;
  - `POST /api/v1/components/{key}/download`;
  - `POST /api/v1/components/{key}/provide-path`;
  - `POST /api/v1/components/{key}/provide-archive`;
  - `POST /api/v1/components/{key}/enable|disable`;
  - `GET /api/v1/components/{key}/events`.
- Added safe component operator inputs:
  - archives extract only under `.cartolensia/components/<key>`;
  - absolute paths, `..`, symlinks, hardlinks, and unsupported archive entry types are rejected;
  - expected files are validated before accepting an archive/path;
  - `/mnt/Models/rclone` is rejected as component path or archive source.
- Added Settings -> Components UI:
  - grouped component cards for media tools, metrics, OCR, AI runtime, and AI models;
  - status, path/version, license, provenance, checksum, errors, check/download/provide/enable controls, and event logs;
  - uses the existing allowlisted file/folder picker for component paths and archives.
- Added asset-detail AI actions:
  - Run classification, face detection, OCR, safety, embedding, short caption, long caption, and all enabled AI functions for a single photo asset;
  - inline status and job links;
  - videos/tracks show clear scoped limitations;
  - existing AI job endpoints receive direct `asset_id` payloads and jobs remain visible on Jobs/Base AI.
- Added asset AI metadata APIs:
  - `GET /api/v1/assets/{id}/ai`;
  - `GET /api/v1/assets/{id}/faces`;
  - `GET /api/v1/assets/{id}/captions`;
  - `GET /api/v1/assets/{id}/classification`;
  - `GET /api/v1/assets/{id}/safety`.
- Added global AI metadata pages:
  - OCR page with text filtering and asset-detail highlight links;
  - Captions page with caption filtering and asset links;
  - Safety Review page listing loaded safety candidates and review links.
- Updated offline distribution tooling:
  - writes `components-manifest.json` in the archive root and `licenses/components-manifest.json`;
  - records FFmpeg configure flags to `licenses/ffmpeg-configure.txt`;
  - fails packaging by default for `--enable-nonfree` FFmpeg;
  - records `--enable-gpl` in `licenses/build-manifest.env` for GPL-tools bundle labeling.

### Verification

Passed during this run:

- `gofmt -w internal/server/settings.go internal/server/server_test.go internal/server/components.go internal/catalog/catalog.go internal/catalog/extended_store.go internal/database/extended.go internal/server/server.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `bash -n scripts/dist/build-offline-linux.sh`
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
- `bash scripts/test-db.sh`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`

Added tests cover:

- component default records and list API;
- user-provided component path validation;
- archive traversal rejection;
- component check job/event visibility;
- per-asset AI aggregate/faces/captions/classification/safety metadata endpoints.

Live validation after restart:

- App restarted with `.cartolensia/runtime/realpeek.yaml` and remained available at `http://127.0.0.1:18080`.
- `/api/v1/stats`: `54` assets, `54` hashed, `48` photos, `2` videos, `4` tracks.
- `/api/v1/components/status`: `17` installed components and `2` missing components.
- Installed components include FFmpeg/FFprobe, Tesseract, English/Russian/Armenian/Chinese tessdata, Python AI venv, PyTorch CUDA, torchvision, facenet-pytorch, EfficientNet-B0, YuNet, Falconsai NSFW, OpenCLIP, and BLIP base.
- Missing components are VMAF/libvmaf (`ffmpeg libvmaf filter is unavailable`) and MobileNetV3 fallback weights.
- FFmpeg configure flags were captured from the system build; it is GPL-enabled and `nonfree=false`.
- `/api/v1/assets/e8ba8b1b-2266-48a6-ba6d-a9171d2693ae/ai` returned classification, caption, OCR, face, safety, embedding, tag, and prediction records.
- `/api/v1/search?q=ocr:test` returned a valid PostgreSQL/local search response with no matches, as expected for that token.
- `/api/v1/ai/status` showed the AI sidecar healthy with OCR languages available and existing AI metadata counts.

### Known Limitations

- Component download/install is intentionally provenance-gated. The current handler creates a job and actionable message but does not silently fetch binaries without a reviewed source URL and license path.
- Long-caption action currently uses the existing `describe` job route; model quality depends on the configured sidecar model.
- OCR/Captions pages currently use the latest loaded AI prediction payload limit; deeper pagination can be added when collections grow.
- Component Manager import-from-offline-package UI remains future work; the package manifest is now produced for that flow.

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No components, models, caches, OCR data, transcodes, exports, packages, or DB files were written under `/mnt/Models/rclone`.
- No new real-data prefix scan was run.
- No missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-09 Track Context, Search Consistency, And Video Player Fix Run

This focused run fixed several real-peek browsing inconsistencies without modifying originals, resetting PostgreSQL, running missing-file marking, committing, or pushing.

### Implemented

- Added reusable timestamp-candidate logic for media assets:
  - trusted `taken_at`;
  - EXIF `exif_datetime_original_raw` interpreted in local runtime timezone;
  - filename timestamps for Pixel/phone-style `PXL_YYYYMMDD_HHMMSS`, `VID_YYYYMMDD_HHMMSS`, `IMG_...`, and `DSC_...`;
  - file mtime as lower-confidence fallback.
- Fixed GPS track media lookup to use timestamp candidates and geotag proximity:
  - time match uses candidate timestamps rather than only `taken_at`;
  - geotagged assets within roughly 1 km of the track can match even when EXIF time policy is raw-only;
  - videos without GPS can match by filename/mtime timestamp candidates.
- Fixed Video Track Player timestamp handling:
  - sessions now store `video_start_at` and `time_source`;
  - position lookup uses the selected timestamp candidate and clamps to nearby track start/end when within tolerance;
  - the previous `video timestamp unavailable` failure is avoided when filename or mtime candidates exist.
- Improved Video Track Player selectors:
  - searchable server-backed video selector by partial filename;
  - track selector uses name-based suggestions and removable pills rather than raw UUID textarea.
- Added asset related/context endpoint and UI:
  - `GET /api/v1/assets/{id}/related`;
  - groups same folder, same device, same day, 30-minute time window, and overlapping GPS tracks.
- Fixed audio preview UX:
  - audio cards now show a compact player/soundwave preview instead of generic “no preview” fallback;
  - gallery overlay opens a real audio player for audio assets.
- Improved middle-click/new-tab behavior:
  - major asset-detail links in Explorer, Search, map popups, albums, GPS media lists, face gallery, and gallery overlay now use anchors and preserve modified-click browser behavior.
- Fixed Search/Explorer mp4 inconsistency:
  - search now gathers all candidate asset pages before applying metadata/OCR/place matching;
  - explicit `ext:mp4` and plain `mp4` now report the same 53 media matches on the live real-peek DB.
- Improved search language:
  - space-separated terms are AND;
  - comma-separated terms act as OR within a token;
  - explicit `ext:`, `kind:`, `filename:`, `path:`, `ocr:`, `transcript:`, `caption:`, `document:`, `place:`, `camera:`, `hash:`, `track:`, `album:`, `face:`, `safety:`, and `private:` tokens remain supported;
  - wildcard `*`/`?` matching is supported for filename/path/plain text matching.
- Added a Search page syntax help panel and made Discovery controls less cramped with wider responsive layout rules.

### Live Validation

- App rebuilt to `/tmp/cartolensia-live`, restarted with `.cartolensia/runtime/realpeek.yaml`, and left running at `http://127.0.0.1:18080`.
- `/api/v1/search?q=ext:mp4`: total `53`, shown `53`.
- `/api/v1/search?q=mp4`: total `53`, shown `53`.
- `/api/v1/assets?media_kind=video&extension=mp4&limit=5`: returned first five MP4 videos and matches the Explorer count path.
- Video selector query `/api/v1/assets?media_kind=video&q=072546&limit=10` returned `PXL_20260512_072546131.mp4`.
- `GET /api/v1/gps/tracks/56501e5a-9704-40cc-a56a-4495628f7bb7/assets?limit=200&include_ungeotagged=true` for `20260509-144424.gpx` returned total `128` and included:
  - `PXL_20260509_165208189.jpg`;
  - `PXL_20260509_172507172.jpg`.
- The same track media lookup also returned timestamp-matched ungps-tagged trip videos such as `VID_20260509_164812_8K.mp4`, `VID_20260509_174113_8K.mp4`, and related files.
- Video Track Player session for `PXL_20260512_072546131.mp4` + `20260512-072610.gpx` returned no warning, selected `file_mtime` as the best overlapping candidate, and position lookup returned one clamped start track position instead of `video timestamp unavailable`.
- `/api/v1/assets/18357bfc-74d2-4132-a976-c3bd35ad829f/related` reported:
  - device `Google Pixel 5`;
  - folder `Cartolensia-photos/DCIM/Camera`;
  - timestamp candidates `exif_datetime_original_raw`, `filename_timestamp`, `file_mtime`;
  - `same_track: 1`, `same_device: 24`, `time_window: 13`.

### Verification

Passed:

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- live health and focused API checks listed above.

Added tests cover:

- EXIF raw timestamp candidates for the concrete `PXL_20260509_172507172.jpg` style.
- Filename timestamp candidate fallback for `PXL_20260512_072546131.mp4` style videos.
- Track range matching through timestamp candidates.

### Known Limitations

- Discovery worker-per-folder sharding was not implemented in this focused pass.
- Related/context scoring is intentionally bounded and metadata-based; it is not yet a full graph database.
- Video Track Player still uses a simplified map marker preview. It now computes positions correctly, but a richer OpenLayers synchronized map remains future polish.
- The best timestamp for `PXL_20260512_072546131.mp4` was `file_mtime`, because the filename timestamp was outside the selected GPX track window. Manual offset/mode controls remain important for devices with inconsistent local timestamp conventions.

### Safety Confirmation

- `/mnt/Models/rclone` was not modified.
- No generated files, model files, caches, transcodes, or exports were written under `/mnt/Models/rclone`.
- No new discovery or missing-file marking was run.
- PostgreSQL was not reset.
- No commit and no push were done.

## 2026-06-09 Production Release Preparation

Completed:

- Production config templates added for host, container, and air-gapped deployments:
  - `config/production.yaml`
  - `config/production-container.yaml`
  - `config/offline-airgap.yaml`
  - `.env.production.example`
  - `docker-compose.production.yml`
- Production/offline docs added:
  - `docs/INSTALLATION.md`
  - `docs/AIRGAPPED_INSTALL.md`
  - `docs/PRODUCTION_DEPLOYMENT.md`
  - `docs/OFFLINE_COMPONENTS.md`
  - `docs/BUILDING.md`
  - `docs/USER_MANUAL.md`
  - `docs/RELEASE_CHECKLIST.md`
- Release scripts added:
  - `scripts/release/build-linux.sh`
  - `scripts/release/check-licenses.sh`
  - `scripts/release/smoke-release.sh`
  - `scripts/release/smoke-production-compose.sh`
- GitHub Actions updated:
  - new `.github/workflows/ci.yml`
  - extended `.github/workflows/offline-release.yml`
- Offline packager now stages production configs, compose files, release helpers, and optional offline map bundles.

Validated:

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `docker compose -f docker-compose.production.yml config`
- `bash scripts/release/check-licenses.sh`
- `bash scripts/release/smoke-release.sh`
- `bash scripts/release/smoke-production-compose.sh`
- `bash scripts/test-db.sh`
- `CARTOLENSIA_SMOKE_ADDR=127.0.0.1:18081 bash scripts/smoke-test.sh`

Artifact produced:

- `dist/cartolensia-af917c5-dirty-linux-x86_64-offline.7z`

Known limitation:

- The minimal release mode was validated locally. AI-runtime packaging still depends on a networked or preprovisioned Python wheel source for the optional Python sidecar dependencies; the release scripts support that path, but it was not exercised in this environment.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Remote Production Bootstrap Continuation

Implemented and deployed:

- Mounted the remote SMB originals share at `/originals` as CIFS read-only with `ro,nosuid,nodev,noexec,file_mode=0440,dir_mode=0550`; verified the `cartolensia` user can read it and cannot create a probe file there.
- Installed the extracted offline bundle under `/opt/cartolensia/current` on the remote root filesystem and kept all writable state under `/var/lib/cartolensia`.
- Fixed the deployed AI sidecar launcher to support the bundle layout where Python packages are under `ai/python-site` and the interpreter is system `python3`; `cartolensia-ai.service` is now active and reports OCR, image AI, ASR, and audio-analysis capability contracts.
- Fixed bundled PostgreSQL service stability by running PostgreSQL with `dynamic_shared_memory_type=mmap`; app-only restarts now work without restarting PostgreSQL.
- Updated `scripts/remote/bootstrap-cartolensia-user.sh` so future remote installs set `CARTOLENSIA_COMPONENT_DIR=/var/lib/cartolensia/components`, preserve model path compatibility, honor AI bind/port env values, and use mmap dynamic shared memory for bundled PostgreSQL.
- Updated `scripts/release/build-local-full-tarzst.sh` so full local bundles export component/model dirs, use mmap dynamic shared memory for bundled PostgreSQL startup, and let `bin/start-ai-executor` run from either a bundled venv, bundled Python, or host `python3` plus `ai/python-site`.
- Updated Component Manager backend root selection to respect `CARTOLENSIA_COMPONENT_DIR`; remote readiness now reports `/var/lib/cartolensia/components`.
- Reworked bounded discovery walking so NAS-scale scans stream discovered file records into PostgreSQL instead of first flattening the entire archive into memory.
- Added a unit test covering streaming bounded walk cancellation.
- Updated distribution/offline/operations docs for read-only SMB `/originals`, `/var/lib/cartolensia` runtime state, remote boot services, NVIDIA AI/transcoding, AMD/Radeon VAAPI, AI launcher fallback behavior, and Postgres mmap mode.

Remote validation:

- `cartolensia-postgres`, `cartolensia-ai`, and `cartolensia` are active and enabled on boot.
- `GET /api/v1/health` returns `ok`.
- `GET /api/v1/diagnostics/readiness` reports local auth, PostgreSQL, storage `originals`, cache, component dir, model dir, ffmpeg, ffprobe, Tesseract, and AI worker as reachable/ok.
- Remote GPU/tool probes showed NVIDIA visible through `nvidia-smi`, `/dev/dri` render nodes visible, and ffmpeg hardware accelerators including `cuda`, `vaapi`, `qsv`, `drm`, `opencl`, and `vulkan`.
- Started remote read-only discovery/indexing for storage `originals` with explicit top-level prefixes, `max_files=-1`, `max_bytes=-1`, no missing marking, and multimedia/PDF extensions only. The first attempt was canceled after identifying the pre-streaming memory issue. The second job is running with streaming inserts; early stats showed assets being inserted while the SMB walk continued.
- Verified the WebUI is reachable from the operator workstation over the LAN while PostgreSQL and AI remain bound to localhost on the remote host.
- Requeued discovery with recent year folders first so the UI can populate useful recent media earlier, while still scanning the remaining explicit top-level prefixes afterward.

Tests run:

- `gofmt -w internal/discovery/discovery.go internal/storage/storage.go internal/storage/storage_test.go internal/server/components.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/storage ./internal/discovery ./internal/server`
- `git diff --check`
- `npm --prefix webui run build`

Known limitations:

- The deployed full archive does not currently contain the approved image/ASR model weights under the active model directory. The AI sidecar is healthy and OCR is available; non-OCR model-backed features remain lazy/missing until models are imported or bundled.
- The running discovery job intentionally excludes root `txt/md` files to avoid indexing non-multimedia sensitive text files from the share root. Images, videos, audio, GPS/KML/KMZ/GPX, PDFs, and DJVU are included.
- Follow-up metadata extraction, hashing, previews, AI/OCR/ASR, and map refresh should be queued after the streaming discovery job completes or in bounded batches.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- remote `/originals` mounted read-only and write probe rejected
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-26 Private Full `tar.zst` Bundle Preparation

Implemented:

- Added `scripts/release/build-local-full-tarzst.sh`, a local/private full-bundle builder that produces `dist/release/*.tar.zst` plus `.sha256`.
- Added `config/local-full-tarzst-build.env.example` with reviewed source/config knobs for BtbN FFmpeg GPL shared Linux x86_64, PostgreSQL, Tesseract/tessdata, AI Python executor environments, model caches, offline maps, and remote executor endpoints.
- Added `scripts/release/prepare-ai-model-cache.py` to prepare the approved local model cache on an Internet-connected staging host.
- Added `scripts/release/smoke-local-full-tarzst.sh` for a no-network tar.zst package smoke build.
- AI worker endpoint is now configurable through `CARTOLENSIA_AI_WORKER_ENDPOINT` and runtime setting `ai.worker_endpoint`; `/api/v1/ai/workers` and AI job dispatch use the configured endpoint instead of hardcoded localhost.
- Packaged runtime scripts generated by the tar.zst builder include `start-postgres`, `start-cartolensia`, `stop-cartolensia`, `start-ai-executor`, `start-transcode-node`, `backup-db`, and `diagnose`.
- Documentation updated in `README.md`, `docs/BUILDING.md`, `docs/DISTRIBUTION.md`, and `docs/OFFLINE_COMPONENTS.md`.

Packaging behavior:

- FFmpeg source defaults to `https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-gpl-shared.tar.xz`.
- The packager records FFmpeg provenance/version and refuses `--enable-nonfree`.
- The BtbN build is treated as a GPL-enabled tools bundle.
- AI executor flavors are staged for `cpu-avx2`, `cpu-avx512`, `nvidia-cu128`, `intel-arc`, and `rocm-radeon`; host GPU drivers/device passthrough remain target-machine requirements and are not bundled.
- Remote AI executors are real sidecar services addressed by host:port.
- Live transcoding is still an in-process ffmpeg session service; the bundle can run a transcode-capable Cartolensia node on another host/port, but a separate distributed transcode executor protocol is not implemented yet.

Validated:

- `bash -n scripts/release/build-local-full-tarzst.sh scripts/release/smoke-local-full-tarzst.sh`
- `python3 -m py_compile scripts/release/prepare-ai-model-cache.py`
- `bash scripts/release/smoke-local-full-tarzst.sh`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `bash scripts/release/check-licenses.sh`
- `docker compose -f docker-compose.production.yml config`

Verification limitation:

- `bash scripts/release/smoke-release.sh` failed with `errno=28: No space left on device`. The repo `dist/` directory is about 20 GB and the filesystem had about 13 GB free. Existing artifacts were left untouched.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-09 Video Track Player Stabilization

Fixed:

- The Video Track Player playback loop no longer self-throttles the position request before the fetch is issued.
- The synchronized map now includes an OpenLayers scale line.
- The synchronized map now shows a live HUD with coordinates, time, source, mode, speed, and altitude for the current point.
- The backend `video-track-player` position payload now includes interpolated speed/elevation and relative time metadata.
- Added a regression test for interpolated position metrics.

Live validation:

- Restarted the live app and confirmed `GET /api/v1/health` is healthy.
- `20260512-072610.gpx` and `PXL_20260512_072546131.mp4` still form a valid session.
- `GET /api/v1/video-track-player/sessions/{id}/position?time_ms=45000` now returns a different point than `time_ms=0`, so the marker advances once playback reaches the overlapping track window.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-09 Local Full-Bundle Build Path

Added:

- `config/local-full-build.env.example`
- `scripts/release/build-local-full.sh`

Purpose:

- provide an Internet-connected staging build path for a complete local 7z archive;
- let operators point the packager at official, reviewed tool/model/runtime roots before packaging;
- keep the existing offline/local release path separate from GitHub release publication.

Validated:

- `git diff --check`
- `bash -n scripts/dist/build-offline-linux.sh scripts/release/build-linux.sh scripts/release/build-local-full.sh scripts/release/check-licenses.sh scripts/release/smoke-release.sh scripts/release/smoke-production-compose.sh`
- app health at `http://127.0.0.1:18080/api/v1/health`

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Remote Boot And OoBE Stabilization

Implemented:

- Created `scripts/remote/bootstrap-cartolensia-user.sh` for one-time remote setup on an operator-managed host.
- The bootstrap creates/updates a dedicated `cartolensia` user, installs an SSH public key, adds the user to `docker`, `video`, and `render`, creates `/opt/cartolensia`, `/var/lib/cartolensia`, `/etc/cartolensia`, and `/originals`, writes production env defaults, and enables `cartolensia-postgres`, `cartolensia-ai`, and `cartolensia` systemd units.
- Remote service defaults now support NVIDIA compute/video (`NVIDIA_VISIBLE_DEVICES=all`, `NVIDIA_DRIVER_CAPABILITIES=compute,video,utility`) and AMD/Radeon VAAPI hints (`LIBVA_DRIVER_NAME=radeonsi`, `VDPAU_DRIVER=radeonsi`) while relying on host drivers and `/dev/dri`/NVIDIA device passthrough.
- Prepared a local SSH keypair and host alias outside the repository for the dedicated remote user.
- Hardened the full `tar.zst` package OoBE:
  - packaged env now includes HTTP, AI endpoint, VAAPI, VDPAU, and transcode accelerator defaults;
  - bundled PostgreSQL startup is idempotent after database bootstrap;
  - generated package includes `first-run` and `status` scripts;
  - generated `FIRST_RUN.md` documents `/originals`, admin password, NVIDIA, AMD VAAPI, and Docker GPU host requirements.
- Added `/api/v1/diagnostics/readiness` and a Settings → Readiness UI panel to show production readiness checks for auth, database, storage, cache/components/models, ffmpeg/ffprobe/tesseract, AI worker, component registry, and HTTP/TLS.

Validated:

- `bash -n scripts/remote/bootstrap-cartolensia-user.sh`
- `bash -n scripts/release/build-local-full-tarzst.sh`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server`
- `npm --prefix webui run build`

Remote access note:

- Existing noninteractive SSH access to the operator host was not available before bootstrap, so the remote bootstrap needed to be run once by the user through their working interactive SSH/sudo path.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Remote Auth, Storage Health, GPU, And Indexing Follow-Up

Implemented locally and partially deployed to the boot-managed production host:

- Hardened local-auth behavior so unauthenticated API/media-original access is blocked while health/version and login/session bootstrap endpoints remain public.
- Changed `/api/v1/auth/me` to return a 200 response with `principal: null` for anonymous sessions, letting the WebUI render the login screen without treating the locked state as a backend outage.
- Login now trims surrounding email whitespace and trailing CR/LF from the submitted password only, so pasting the generated admin password file into the WebUI works even when the copied line includes a final newline.
- Added WebUI login helper text documenting the generated admin password flow.
- Added storage health fields to `/api/v1/storages` and surfaced `available`/`missing`/`error` states in Storages and Settings. Missing optional NAS roots are diagnostic only and do not delete metadata.
- Added GPU-oriented transcode presets for NVIDIA NVENC and VAAPI, including H.264 LAN presets and low-bitrate AV1 presets.
- Added VAAPI argument handling and render-node selection that prefers configured devices first, then AMD/Intel DRI render nodes over NVIDIA render nodes for VAAPI.
- Updated Component Manager checks to respect production model/component/Python package directories outside the immutable release tree.
- Updated remote bootstrap service env so mutable components, models, and extra Python packages live under `/var/lib/cartolensia`.
- Added asset-detail document previews:
  - PDF assets render through the browser PDF viewer using the authenticated original URL.
  - Markdown assets render through a small safe text renderer, not raw HTML.
  - plain-text assets render as scrollable text.
  - extracted document/OCR text can be copied or downloaded by the browser.
- Added metadata-backed public sharing:
  - administrators can mark/unmark an asset Public from Asset Detail;
  - anonymous users see only the Public Gallery and login controls;
  - anonymous media access is allowed only for explicitly public assets;
  - unmarked assets still require authentication for API and original media access.

Remote validation:

- Production services were active: PostgreSQL, AI sidecar, and main Cartolensia service.
- Anonymous `/api/v1/stats` and original-media routes returned `401`, while anonymous `/api/v1/auth/me` returned `principal: null`.
- Admin login succeeded using the configured admin email and the password file value.
- The read-only originals mount and secondary read-only NAS mount were present; no write probes were performed during this follow-up.
- Configured optional storages reported health as available or missing. Missing optional roots remained visible as missing without metadata deletion.
- NVIDIA H.264 dry-run succeeded against a read-only original video.
- NVIDIA AV1 dry-run succeeded on the host GPU.
- VAAPI H.264 dry-run succeeded after the renderer selector chose the AMD/DRI render node instead of the NVIDIA render node.
- The AI sidecar reported CUDA-capable PyTorch packages, Tesseract OCR with English/Russian/Armenian/Chinese data, faster-whisper/CTranslate2 availability, and audio-analysis Python packages.
- A synthetic ASR request loaded the small faster-whisper model from `/var/lib/cartolensia/models` and returned a successful empty transcript for a tone fixture.
- Discovery for the primary large originals storage was still running at the latest check with zero scan errors. Current observed counters were approximately:
  - 52,724 total assets indexed so far;
  - 47,310 photos;
  - 3,295 videos;
  - 768 audio files;
  - 1,070 GPS/KML/KMZ/GPX tracks;
  - 281 documents;
  - about 1.67 TB of indexed locations.
- Queued the next safe metadata extraction stage behind the active discovery job:
  `metadata_enrich` job `1139f550-fabf-4610-8380-5b174f69c0ca`, scoped to storage `originals`, explicit top-level prefixes, all supported media/document kinds, and `max_files=-1`.

Tests run:

- `gofmt -w internal/server/components.go internal/server/server.go internal/server/server_test.go internal/server/transcode_sessions.go internal/server/transcode_sessions_test.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- focused `TestLocalAuthEndpoints` coverage for public/unpublic asset media access
- targeted remote authenticated API checks for jobs, stats, storages, and auth behavior
- targeted remote ffmpeg dry-runs for NVIDIA H.264, NVIDIA AV1, and VAAPI H.264
- synthetic remote ASR sidecar request

Known limitations:

- The running discovery job should not be interrupted unless necessary; deployment of the latest document-preview UI can wait until that job completes or can be followed by a safe requeue if the lease expires.
- Image model weights for full classification/captioning/embedding may still need Component Manager import/check on the remote host before those jobs are treated as fully operational.
- Public sharing is intentionally per-asset metadata only. There is no multi-user ACL, public album management, or expiring share-link model yet.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-28 Durable Worker-Owned AI Backfill

Implemented:

- Added `ai_asset_task_status` migration so successful zero-output AI runs are recorded without polluting OCR/caption/search rows.
- Added `QueryAIMissingAssets` to the catalog contract with PostgreSQL-backed missing-target selection for:
  - classification;
  - safety/NSFW;
  - captions;
  - embeddings;
  - face detection;
  - OCR;
  - audio features;
  - audio/video transcription.
- Added durable `ai_backfill` worker jobs:
  - jobs are claimed by the existing PostgreSQL worker queue;
  - progress/counters/logs are visible in Jobs;
  - cancellation uses the existing job cancellation path;
  - each task repeatedly pulls only assets still missing that output;
  - old completed outputs and new zero-output task markers prevent endless reprocessing.
- Added `POST /api/v1/ai/backfill/start`.
- Added a Base AI button, `Backfill all missing AI metadata`, which queues one durable backfill job per task instead of holding a browser request open.
- Refactored API AI execution so synchronous asset actions and worker backfills share the same sidecar call and persistence path.

Validation:

- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `git diff --check`

Remote deployment:

- Built and deployed a new rjazhenka release: `/opt/cartolensia/releases/local-20260628T-ai-backfill`.
- Restarted `cartolensia`; PostgreSQL and AI sidecar remained active.
- Migration applied on startup through embedded migrations.
- Increased production worker concurrency from `2` to `6` so discovery, metadata, hash, and multiple AI lanes can progress together.
- Enqueued durable production AI backfill with no per-task limit and conservative batch size `64`.

Remote validation:

- `cartolensia`, `cartolensia-ai`, and `cartolensia-postgres` are active.
- Authenticated stats after deploy: about `558,898` assets, `443,933` photos, `28,580` videos, `8,781` audio, `133,635` documents, `5,521` tracks, about `12.56 TB`.
- AI status: enabled, CUDA active, pgvector already active, sidecar reachable.
- Active durable AI backfill jobs include classification, safety, captions, embeddings, faces, OCR, audio features, and audio transcript tasks.
- GPU check during backfill: RTX 4060 Ti around `73%` utilization, about `3.1 GB / 16 GB` VRAM used.

Known limitations:

- Sparse per-batch AI errors remain for unreadable/problem assets; jobs continue and count them instead of failing the whole backfill.
- Existing pre-patch synchronous audit jobs can remain visible as stale `api-ai` history until canceled/expired; new production backfill uses `ai_backfill` worker jobs.
- Current AI worker parallelism is process/job-level, not a full VRAM-aware scheduler. The new durable jobs unblock production-scale operation, but a future per-device scheduler can improve model residency and batching.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-28 AI Targeting, Knowledge UI, And Remote Production Stabilization

Implemented:

- Fixed broad AI job targeting so scoped image/audio AI runs query only media kinds each action can actually process:
  - image actions now select photos instead of spending the first page on GPS tracks;
  - audio transcription selects audio/video assets;
  - audio analysis selects audio assets.
- Fixed the AI resolver to page internally in 500-row chunks, so `limit:1000` and larger scoped AI requests are no longer truncated by the normal UI/API list cap.
- Added regression coverage for broad AI scoped jobs with many photos and interleaved unsupported track/audio/video rows.
- Added local LLM integration hooks for Knowledge/Search:
  - deterministic mode remains the default;
  - Ollama and OpenAI-compatible/vLLM endpoints can be configured through runtime settings;
  - local LLM prompts receive only read-only tool results/facts/relations, never DB credentials or write tools.
- Added Russian/English deterministic natural-language search parsing improvements, including Russian month ranges such as `май-август 2025`.
- Added Knowledge Base pagination controls and citation cards with clickable asset links.
- Added a lightweight Knowledge Graph SVG preview with bounded node/relation rendering for large graphs.
- Added in-browser console capture and a Console page for recent WebUI errors.
- Hardened Settings API normalization and WebUI guards for nullable settings fields that previously crashed the Settings page.
- Added reverse-geocode radius support for local place-cache matching; default is controlled by `search.reverse_geocode_radius_m`.
- Sidebar navigation now uses anchors so normal browser middle-click/new-tab behavior works, and stale `asset_id` is removed when navigating to non-asset pages.

Remote production validation:

- Rebuilt backend and WebUI; deployed both to the remote production service.
- HTTPS frontend serves built assets and the AI sidecar health endpoint reports CUDA-backed real mode.
- PostgreSQL/pgvector is active; vector status reports `pgvector_ivfflat`.
- Current remote stats at validation time:
  - assets: `614582`;
  - photos: `443933`;
  - videos: `28580`;
  - audio: `8781`;
  - documents: `133635`;
  - tracks: `5521`;
  - hashed: `135211`;
  - unhashed: `485239`;
  - total bytes: about `12.56 TB`.
- Storage health:
  - `originals`, `old_compressed_data`, `old_nokia5228`, and `old_x12_los20` are readable strict-read-only storages;
  - two optional child storage paths remain unavailable because their configured directories are not present under the mounted share; Cartolensia reports this without deleting metadata.
- AI batch root cause confirmed:
  - previous broad AI jobs targeted `200` assets and skipped all `200` because the first page was tracks;
  - after the fix, image jobs process supported photos with zero unsupported skips.
- Corrected AI batch results after deploy:
  - embed, safety, classify, and describe processed `1000 / 1000` photo assets in the current corrected run;
  - face detection, OCR, transcription, and audio analysis were relaunched and were progressing in Jobs at the last poll.
- OCR endpoint showed a running OCR job around `477 / 1000` with stored updates increasing, and transcripts returned CUDA/faster-whisper metadata.
- A slow caption search probe on the large index was stopped manually; search/index optimization remains a next performance target.

Local validation:

- `gofmt -w internal/server/server.go internal/server/server_test.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `git diff --check`

Known limitations / next work:

- A local LLM runtime is not installed on the remote host yet (`ollama` absent and `vllm` absent in the current AI venv). The Cartolensia integration is ready for an offline-provided Ollama or OpenAI-compatible/vLLM endpoint.
- Synchronous API-launched AI jobs can leave stale `cancel_requested` audit rows if the app is restarted mid-request. The production path should move these AI actions into the durable job worker/scheduler.
- The current AI backfill still tends to repeat the first eligible assets unless a higher-level missing-work scheduler is used. A durable per-task “missing AI metadata” scheduler should be the next scaling improvement.
- GPU utilization is improved by concurrent corrected batches, but the long-term solution is a VRAM-aware multi-lane AI scheduler with model residency and backpressure.
- Full graph LOD/clustering, all-track map rendering at very large scale, and faster combined caption/OCR/transcript search remain performance hardening targets.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-28 Knowledge Base, Knowledge Graph, And Local Query Tools

Implemented:

- Added PostgreSQL migration `019_knowledge_base_graph.sql`:
  - `knowledge_facts`;
  - `knowledge_relations`;
  - `knowledge_conversations`;
  - `knowledge_messages`;
  - full-text and relation indexes;
  - read-only views `cartolensia_search_knowledge_facts` and `cartolensia_search_knowledge_relations`.
- Extended the read-only SQL allowlist so advanced local queries can inspect KB/KG views without raw table access.
- Added PostgreSQL store methods for:
  - paged fact browsing;
  - paged relation browsing;
  - idempotent bounded fact/relation extraction from existing metadata;
  - local conversation/message persistence.
- Added API endpoints:
  - `GET /api/v1/knowledge/facts`;
  - `GET /api/v1/knowledge/relations`;
  - `POST /api/v1/knowledge/extract`;
  - `POST /api/v1/knowledge/chat`.
- Added WebUI navigation pages:
  - `Knowledge Base` for human-readable extracted facts and local chat;
  - `Knowledge Graph` for relation browsing and compact edge preview.
- Knowledge extraction currently mines explicit local facts from:
  - asset kind/location/timestamps/device metadata;
  - geotags;
  - tags and AI predictions;
  - OCR/caption predictions;
  - transcripts;
  - document text;
  - audio features;
  - GPS track summaries;
  - folder/device/tag/track/transcript/document/audio-feature relations.
- Knowledge chat currently uses a deterministic local tool runner:
  - plans English/Russian questions with the local parser;
  - searches facts and relations;
  - records tool calls and conversation messages;
  - does not call remote LLM APIs.

Remote deployment and validation on rjazhenka:

- Deployed updated backend binary to `/opt/cartolensia/current/bin/cartolensia`.
- Deployed updated WebUI to `/opt/cartolensia/current/webui/dist`.
- Deployed migration `019_knowledge_base_graph.sql`.
- Restarted only `cartolensia.service`; did not reset PostgreSQL.
- Authenticated API validation:
  - `/api/v1/knowledge/facts` and `/api/v1/knowledge/relations` returned successfully;
  - bounded extraction `limit=100` upserted `1000` facts and `500` relations;
  - larger extraction `limit=5000` upserted `36381` facts and `17875` relations;
  - `/api/v1/knowledge/chat` returned `5` facts, `3` relations, and `9` local tool calls for a test request.
- Remote status after deployment:
  - assets: `615339`;
  - photos: `443932`;
  - videos: `28580`;
  - tracks: `5521`;
  - hashed: `91097`;
  - unhashed: `529091`;
  - jobs: `3` running, `0` queued;
  - vector store: `pgvector_ivfflat`;
  - knowledge facts: `36381`;
  - knowledge relations: `17875`.
- AI sidecar health:
  - active;
  - CUDA device;
  - classifier, YuNet, NSFW, OpenCLIP, BLIP, Tesseract OCR, faster-whisper, and audio analysis available;
  - Tesseract languages present: `eng`, `rus`, `hye`, `chi_sim`, `chi_tra`.

Tests:

- `gofmt -w internal/catalog/catalog.go internal/database/knowledge.go internal/database/search_query.go internal/database/search_query_test.go internal/server/knowledge.go internal/server/search_plan.go internal/server/search_plan_test.go internal/server/server.go`
- `go test ./internal/database ./internal/catalog`
- `go test ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `git diff --check`

Known limitations:

- The KB/KG extractor is explicit and bounded; it is not yet a persistent job worker. Operators can rerun extraction as OCR, ASR, captions, and metadata accumulate.
- The chat panel is a local deterministic tool runner. A future local LLM can sit in front of the same safe tools, but generated SQL must remain restricted to read-only views.
- The relation graph preview is intentionally lightweight; it avoids a heavy graph rendering dependency until the data model stabilizes.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-28 Read-Only Search Query Layer And Natural-Language Planner

Implemented:

- Added safe search planning endpoints:
  - `GET /api/v1/search/parse?q=...` parses Cartolensia search tokens and SQL-like clauses;
  - `POST /api/v1/search/plan` creates a deterministic local English/Russian query plan when no local LLM is configured;
  - `/api/v1/search` now returns the parsed plan and preview alongside results.
- Added curated PostgreSQL read-only search views in migration `018_readonly_search_views.sql`:
  - `cartolensia_search_assets`;
  - `cartolensia_search_ai_predictions`;
  - `cartolensia_search_tags`;
  - `cartolensia_search_transcripts`;
  - `cartolensia_search_transcript_segments`;
  - `cartolensia_search_documents`;
  - `cartolensia_search_video_captions`;
  - `cartolensia_search_audio_features`;
  - `cartolensia_search_tracks`;
  - `cartolensia_search_places`.
- Added `POST /api/v1/search/sql`, a guarded read-only query endpoint:
  - accepts only a single `SELECT`;
  - rejects semicolons, comments, mutation/session-control keywords, and raw table access;
  - only allows `cartolensia_search_*` views;
  - runs inside a PostgreSQL read-only transaction with statement timeout and server-side row limit.
- Updated the Search page:
  - explicit parse button;
  - English/Russian “Ask Cartolensia” planner;
  - parsed-query preview;
  - collapsed read-only SQL workbench for advanced diagnostics.

Remote production deployment:

- Deployed rebuilt backend, WebUI assets, and migration `018_readonly_search_views.sql` to rjazhenka.
- App restarted successfully on HTTPS `:18443`; PostgreSQL and AI sidecar remained running.
- Migration validation: 10 `cartolensia_search_*` views present.
- Authenticated validation:
  - `kind = video and ext = mp4` parsed to `kind:video ext:mp4`;
  - Russian request `покажи видео с поездом` planned to `kind:video поездом`;
  - read-only SQL query against `cartolensia_search_assets` returned MP4 rows and rejected no safety checks.

Remote indexing state after deployment:

- Assets: about `615k`.
- Active/queued production jobs after restart:
  - `discovery` running;
  - `metadata_enrich` running;
  - `hash` queued after lease recovery.
- AI sidecar remained active. Existing AI metadata counts at validation time:
  - predictions about `28k`;
  - transcripts `302`;
  - audio feature rows `2532`;
  - embeddings `2824`.
- Restarted the remote AI backfill supervisor with the production environment loaded. The first launch attempt without env failed with `CARTOLENSIA_DATABASE_URL is required`; the corrected launch is running as `python3 /var/lib/cartolensia/run/run-ai-backfill.py`.
- Backfill validation showed active missing-work batches:
  - classify completed `8` assets;
  - safety completed `8` assets;
  - caption completed `8` assets;
  - embed completed `8` assets;
  - OCR completed `4` assets;
  - faces completed `8` assets;
  - audio feature and transcript batches ran.

Tests:

- `gofmt -w internal/server/search_plan.go internal/server/search_plan_test.go internal/database/search_query.go internal/database/search_query_test.go internal/server/server.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `git diff --check`

Known limitations / next work:

- The local LLM planner is currently a deterministic rule-based fallback. A model-backed planner should call a local sidecar endpoint, still pass generated SQL through the same read-only allowlist, and unload after the configured idle period.
- The search workbench is intentionally read-only and bounded; it is for diagnostic/research queries, not arbitrary database administration.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Samba Storage Diagnostics And Settings

Implemented:

- Added per-storage SMB diagnostic metadata:
  - `source_url`;
  - `smb.host`;
  - `smb.share`;
  - `smb.path`;
  - `smb.domain`;
  - `smb.username`;
  - `smb.credentials_file`;
  - `smb.password_env`.
- Runtime and YAML-bound storage settings now expose these fields in Settings -> Storage.
- Storage readiness and `/api/v1/storages` now return specific health codes:
  - `host_unresolved`;
  - `host_offline`;
  - `export_unavailable`;
  - `credentials_invalid`;
  - `credentials_file_missing`;
  - `credentials_file_unreadable`;
  - `export_or_mount_unavailable`;
  - `original_file_missing` for original-media requests when the storage is readable but the indexed file path is gone.
- Original-media failures now return structured JSON with storage name, relative path, health code, and action-oriented message instead of a generic server error.
- Settings UI now shows source URL, SMB host/share/path, credentials-file status, health code, and probe details.

Remote rjazhenka validation:

- Deployed updated backend and WebUI to `/opt/cartolensia/current`.
- Added non-secret SMB metadata to the active production config; Samba credentials remain in `/etc/cartolensia/smb-multimedia.credentials`.
- Set the credentials file to `root:cartolensia` mode `0640` so the service can probe SMB without exposing the secret in UI or command arguments.
- Live status now distinguishes the current outage as:
  - host reachable on TCP 445;
  - credentials file readable;
  - configured exports unavailable (`export_unavailable`, `NT_STATUS_BAD_NETWORK_NAME`).
- Authenticated checks confirmed metadata-only service still works while exports are unavailable:
  - stats returned `615353` assets;
  - asset metadata listing returned rows from PostgreSQL;
  - original-media request returned `503 storage_unavailable` with storage health `export_unavailable`.

Tests:

- `gofmt -w internal/storage/storage.go internal/config/config.go internal/app/app.go internal/database/database.go internal/server/server.go internal/server/readiness.go internal/server/readiness_test.go internal/storage/storage_test.go internal/config/config_test.go internal/server/server_test.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/config ./internal/storage ./internal/database ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `git diff --check`

Known limitations:

- If `smbclient` is unavailable on a deployment host, Cartolensia still distinguishes host reachability and local mount/path failures, but cannot independently verify share names or credentials through SMB protocol.
- The current rjazhenka Samba server is reachable, but reports the configured shares as unavailable. No metadata was deleted and no missing-file marking was run.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Post-Outage Remote Startup And Offline Storage Degradation

Issue:

- After the utility outage, rjazhenka booted PostgreSQL and the AI sidecar, but the main Cartolensia app did not serve UI/API while the Samba-backed originals roots were unavailable.
- Root cause: filesystem storage initialization treated `filepath.EvalSymlinks` errors such as `ENODEV` from unavailable CIFS mounts as fatal, so one offline originals storage could block the whole metadata service.
- Readiness diagnostics also probed offline storage roots sequentially, taking about 26 seconds.
- On the 615k-asset remote index, `ext:mp4` search timed out because search loaded all matching assets before pagination.

Implemented:

- Storage registry now tolerates unavailable filesystem roots at startup for expected offline mount errors (`not exist`, `ENODEV`, `ENOTCONN`, host unreachable/down, timeout, `EIO`).
- File access remains strict:
  - relative paths are still normalized;
  - traversal checks still run;
  - storage writes remain disabled by the read-only adapter.
- Readiness storage checks are now timeout-bounded and parallelized.
- Unavailable originals now report readiness `warn`, not deployment `error`, when DB/cache/AI/UI are otherwise healthy.
- Search now has a paginated fast path for explicit asset filters:
  - `ext:mp4`;
  - `extension:mp4`;
  - `kind:video` / `media:video`;
  - plain tokens that exactly match a supported extension, such as `mp4`.

Remote validation on rjazhenka:

- Services active:
  - `cartolensia-postgres`;
  - `cartolensia-ai`;
  - `cartolensia`.
- Listeners active:
  - PostgreSQL `127.0.0.1:15432`;
  - AI sidecar `0.0.0.0:19090`;
  - HTTPS `*:18443`;
  - HTTP redirect `*:18080`.
- LAN URL: `https://192.168.237.126:18443/`.
- Authenticated readiness:
  - overall `warn`;
  - `0` errors;
  - `11` ok checks;
  - `6` storage warnings for currently unavailable Samba/original roots;
  - readiness time improved to about `0.77 s`.
- Metadata-only API checks while originals were unavailable:
  - stats returned `615353` assets, `443932` photos, `28580` videos, `8781` audio files, `5521` tracks, and `12.5 TB` total indexed bytes;
  - GPS tracks endpoint returned immediately;
  - `ext:mp4` search returned `10 / 24735` matches in about `0.22 s`;
  - plain `mp4` search returned `10 / 24735` matches in about `0.22 s`.

Tests:

- `gofmt -w internal/storage/storage.go internal/storage/storage_test.go internal/server/readiness.go internal/server/readiness_test.go internal/server/server.go internal/server/server_test.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/storage ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`

Known limitations / next work:

- App startup still took roughly 25 seconds before binding HTTPS after restart. It no longer fails when Samba is absent, but startup timing should be profiled separately.
- Offline originals mean original media playback/open-original can fail until the Samba server is back, but metadata, tracks, jobs, AI records, search, and cached data can continue to serve.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 OCR/Captions Asset Navigation Fix

Fixed and deployed to rjazhenka:

- The global `openAsset` path now opens Asset Detail immediately after loading the asset record instead of waiting for related/context and video stream option calls.
- Related/context and stream options now hydrate asynchronously after the page is visible, preventing large-database context queries from leaving the UI stuck on `Loading`.
- OCR and Captions page rows now use real asset-detail anchors with the shared `openAssetLink`/OCR-highlight handlers.
- OCR/Captions rows guard against missing asset IDs and show a warning badge instead of trying to navigate to an invalid asset URL.
- Middle-click/Ctrl-click/Meta-click behavior is preserved for Captions and OCR asset links.

Tests run:

- `npm --prefix webui run build`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `git diff --check`
- WebUI assets synced to rjazhenka; no service restart was required.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Explorer Load All Control

Fixed and deployed to rjazhenka:

- Explorer pagination now has two explicit controls:
  - `Load more` fetches the next page;
  - `Load all` fetches every remaining page for the current folder/filter.
- Bottom-scroll auto-loading remains active and fetches only the next page at a time.
- `Load all` uses larger paged requests internally and deduplicates by asset/location key while appending.

Tests run:

- `npm --prefix webui run build`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `git diff --check`
- WebUI assets synced to rjazhenka; no service restart was required.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Explorer Scale, AI Backfill, And Remote Deployment Update

Implemented locally and deployed to rjazhenka:

- Replaced the folder-mode Explorer fallback that could load the full asset catalog with a PostgreSQL-backed folder aggregation and direct-file page query.
- Added production path indexes for Explorer-scale browsing:
  - `relative_path text_pattern_ops`;
  - `(storage_id, relative_path text_pattern_ops)`;
  - folder sort helpers for name, mtime, and size.
- Updated the WebUI Explorer to request a first page of 200 files and expose a Load More control. Large folders no longer silently stop at the first slice.
- Added near-bottom automatic loading with an IntersectionObserver sentinel, with Load More retained as a manual fallback.
- Replaced the hundreds-of-buttons month strip with a compact month selector plus a short quick-month disclosure.
- Split the new Explorer controls into `MonthFilterBar.vue` and `PagedFileControls.vue`.
- Added Vite manual chunks for Vue, OpenLayers, and HLS; HLS is loaded dynamically only when HLS playback needs it.

Remote rjazhenka validation after deploy:

- Production URL remains `https://192.168.237.126:18443/`.
- Authenticated stats at latest poll:
  - `249,219` assets;
  - `218,903` photos;
  - `20,592` videos;
  - `2,359` audio files;
  - `5,560` documents;
  - `1,806` GPS/KML tracks;
  - about `12.26 TB` indexed metadata footprint.
- pgvector is active:
  - backend `pgvector_ivfflat`;
  - 512 dimensions;
  - `257` embedded assets at the validation poll and increasing through backfill.
- AI sidecar is active and CUDA-backed. The restarted AI backfill is running as PID `1746707`, latest log `/var/lib/cartolensia/logs/ai-backfill-20260627T131128Z.log`.
- Latest AI backfill entries show successful classification, NSFW safety, captions, embeddings, OCR, audio feature extraction, and audio/video transcript jobs.
- Explorer performance checks:
  - root folder page: about `168 ms`;
  - `2026`: about `26 ms`;
  - `2026/May2026`: about `23 ms` for `200 / 1,894` files;
  - offsets `200`, `400`, and `1800` returned in about `17-18 ms`, with the final page correctly returning `94` files.
- Read-only discovery jobs were queued for currently available storages:
  - `old_nokia5228`: `61069b05-b450-4c27-b8b3-cb59f47a3c6d`;
  - `old_x12_los20`: `a1004700-8e5f-47c7-970e-6426ec3ad1f7`;
  - `originals`: `6a17fd7b-ef0c-4f6b-86f0-2ab5b46a0966`.
- Metadata enrichment was queued as job `3804c8e9-e48d-4b9d-b514-f872e3930c28`.
- Optional storage health remains metadata-preserving:
  - `originals`, `old_x12_los20`, and `old_nokia5228` available;
  - `old_p770` and `old_ze554kl` reported missing/unavailable;
  - no metadata was deleted for unavailable originals.

Tests run:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- authenticated remote production checks for health, stats, vector status, AI status, Explorer paging, storage health, queued jobs, and AI backfill logs.

Known limitations:

- The queued discovery jobs will start as production workers free capacity; AI backfill is actively submitting small synchronous API jobs, so discovery may wait briefly behind current work.
- Full archive hashing was intentionally not launched in this pass because hashing would read many terabytes over SMB/NAS and compete with interactive preview/search/AI workloads.
- HLS remains a separate large lazy chunk because the dependency itself is large; it no longer inflates the initial Explorer bundle.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Remote HTTPS, pgvector, AI Sidecar, And Backfill Stabilization

Implemented locally:

- Added production HTTPS support with a secondary TLS listener, self-signed certificate generation, and optional HTTP-to-HTTPS redirect.
- Added a narrow loopback-only AI media endpoint guarded by `CARTOLENSIA_AI_MEDIA_TOKEN` so the local AI sidecar can read protected originals through Cartolensia without exposing authenticated media routes to LAN users.
- Added optional pgvector schema setup and vector-store detection. When the PostgreSQL `vector` extension and a 512-dimensional embedding are available, embeddings are now stored in both JSON metadata and an indexed `vector(512)` column.
- Removed the synthetic requirement that every normal discovery run must specify a subpath. Empty prefixes now mean storage root, and `storage=all` can be used deliberately for normal indexing while real-archive safety guards still block missing marking and require explicit unlimited sentinels.
- Added mobile shell hardening: the sidebar becomes a horizontal sticky navigation strip, topbar/login forms reflow, content padding shrinks, and gallery/audio overlays fit small screens.
- Added `scripts/remote/run-ai-backfill.py`, a low-concurrency production AI backfill driver that selects missing metadata work from PostgreSQL and feeds small authenticated API batches. It records successful no-result checks locally under `/var/lib/cartolensia/run/ai-backfill-state` to avoid repeatedly OCRing/transcribing blank assets.

Remote rjazhenka status after deployment:

- Public LAN UI is available at `https://192.168.237.126:18443/` with a self-signed certificate.
- Authentication is local/session based. Login email is `admin@example.local`; password is the exact value in `/etc/cartolensia/admin-password` on the remote host. Do not include the trailing shell prompt when copying it; pasted trailing newlines are ignored by the server.
- Services are active: `cartolensia-postgres`, `cartolensia-ai`, and `cartolensia`.
- HTTPS is active on `:18443`; HTTP `:18080` redirects to HTTPS for browser traffic.
- Current indexed stats at the latest poll:
  - `245,895` assets;
  - `215,702` photos;
  - `20,592` videos;
  - `2,238` audio files;
  - `5,558` documents;
  - `1,806` tracks;
  - about `12.19 TB` indexed metadata.
- Discovery is still running:
  - status `running`;
  - progress `51,250` current scan units at the latest job poll;
  - no observed current error.
- Metadata enrichment is still running:
  - status `running`;
  - progress `23,145 / 245,617`;
  - no observed current error.
- AI sidecar is reachable at `http://127.0.0.1:19090`, CUDA-backed, and reports:
  - `classify_image`;
  - `detect_faces`;
  - `safety_nsfw`;
  - `describe_image`;
  - `embed_image`;
  - `embed_text`;
  - `ocr_image`;
  - `transcribe_audio`;
  - `analyze_audio`.
- Reviewed local model caches were synced into `/var/lib/cartolensia/models`:
  - OpenCV YuNet;
  - Falconsai NSFW;
  - OpenCLIP ViT-B/32;
  - BLIP base;
  - faster-whisper small was already present.
- pgvector is enabled and active:
  - vector backend `pgvector_ivfflat`;
  - dimensions `512`;
  - latest observed embedding count `33` and increasing through backfill.
- AI backfill is running in small batches:
  - latest log `/var/lib/cartolensia/logs/ai-backfill-20260627T123644Z.log`;
  - observed successful classification, safety, captioning, embeddings, OCR, audio feature extraction, audio transcripts, and video transcripts;
  - no observed backfill API errors in the latest log segment.

Tests and checks run:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `go test ./...`
- `npm --prefix webui run build`
- `docker compose -f docker-compose.production.yml config`
- `bash scripts/release/check-licenses.sh`
- `bash scripts/release/build-linux.sh` reached WebUI/Go build but failed during AI Python runtime packaging because local DNS could not resolve `pypi.org`.
- `CARTOLENSIA_DIST_INCLUDE_PYTHON_RUNTIME=0 bash scripts/release/build-linux.sh` succeeded and produced `dist/cartolensia-65e5382-dirty-linux-x86_64-offline.7z`.
- authenticated remote checks for health, auth login, stats, jobs, AI status, and vector status.

Known limitations:

- AI backfill is intentionally single-process and low-concurrency. It is designed to keep the UI usable on a large NAS-backed archive, not to finish 200k+ photos immediately.
- Full local AI-runtime archive assembly still requires either Internet DNS/package access or a prepared Python wheelhouse. The minimal/tools/PostgreSQL offline archive path succeeds without fetching Python packages.
- Face detection can load YuNet and run on demand, but the continuous backfill driver does not yet include a durable "no faces found" marker. This avoids an infinite rerun loop over face-free images.
- Older hash jobs include a previous lease-expired record. Current discovery, metadata enrichment, and AI backfill are running.
- The browser must use HTTPS on port `18443` for local-auth cookies because production cookies are marked Secure.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Metadata Scalability And Remote Status Follow-Up

Implemented locally:

- Added prefix filtering to the shared PostgreSQL `QueryAssets` path so storage/prefix-scoped jobs can use the same bounded query service as Explorer/Search instead of listing all assets and filtering in application memory.
- Refactored metadata enrichment:
  - selected `asset_ids` are resolved directly with `GetAsset`;
  - normal storage/prefix jobs page through `QueryAssets` in batches of 500;
  - storage/prefix/media-kind scoped jobs enrich from the matching asset location, not an arbitrary first location;
  - cancellation checks remain between pages and assets;
  - skipped rows are counted in job counters.
- Added regression coverage for prefix-scoped metadata enrichment so only matching-prefix documents are updated.

Remote status observed after local verification:

- Production services were active.
- Anonymous `/api/v1/auth/me` returned `principal: null`; authenticated status checks succeeded with the remote password file.
- Current remote stats at the latest poll:
  - `65,280` assets;
  - `58,893` photos;
  - `4,181` videos;
  - `793` audio files;
  - `1,107` tracks;
  - `306` documents;
  - about `2.33 TB` indexed.
- Discovery is still running and should not be interrupted:
  - job `9b52a892-02b8-4e21-ab11-e6852244ab43`;
  - progress `65,250` scanned;
  - created `28,315`, updated `36,935`;
  - errors `0`;
  - folders scanned `126`.
- The metadata job is also running:
  - job `1139f550-fabf-4610-8380-5b174f69c0ca`;
  - kind `metadata_enrich`;
  - progress `12,529 / 37,695`;
  - errors `0`.
- Optional storage health remained metadata-preserving:
  - main originals and two optional NAS roots available;
  - two optional sub-roots reported missing because the NAS paths were not present;
  - no metadata deletion or missing-file marking was performed.

Deployment note:

- The latest local metadata pagination fix was not deployed during the active remote discovery/metadata jobs. Restarting the service would risk interrupting productive long-running jobs. Deploy after discovery and metadata complete or deliberately requeue with the new paginated runner.

Tests run:

- `gofmt -w internal/catalog/catalog.go internal/catalog/extended_store.go internal/database/extended.go internal/metadata/metadata.go internal/metadata/metadata_test.go internal/server/server.go internal/server/server_test.go internal/server/transcode_sessions.go internal/server/transcode_sessions_test.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `git diff --check`
- `go test ./...`
- `npm --prefix webui run build`
- authenticated remote read-only checks for stats, jobs, storages, components, and auth bootstrap.

Known limitations:

- The active remote discovery and metadata jobs are running the currently deployed implementation. The paginated metadata runner is verified locally and should be included in the next deploy/restart after those jobs complete.
- Full image AI model components still report missing on the remote until reviewed model caches are imported or installed through Component Manager.
- PyMuPDF/Marker document extraction remains optional and missing on the remote; browser PDF preview and metadata-only document handling work independently.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Parent Samba Storage, Overlap-Safe Discovery, And Essential Export

Implemented locally and deployed to rjazhenka:

- Added subtree pruning to the bounded filesystem walker. `exclude_patterns` ending in `/**` now skip whole directory trees instead of walking into them and filtering files afterward.
- Discovery now automatically detects configured nested filesystem storages. When scanning a parent storage or `storage=all`, the parent scan excludes child storage roots so the same NAS subtree is not indexed through both `old_compressed_data` and its child storage entries.
- Added regression tests:
  - excluded subtree pruning in `internal/storage`;
  - `storage=all` parent/child overlap indexing only the parent root file plus the child storage file once.
- Hardened the remote AI backfill driver:
  - short API outages are retried instead of terminating the loop;
  - face detection is now part of the missing-work backfill loop.
- Added `scripts/remote/create-essential-export.sh` for an operator-run essential backup archive:
  - PostgreSQL custom-format dump;
  - redacted production config;
  - storage manifest;
  - restore notes;
  - excludes originals, cache thumbnails, component/model caches, and local secret files.

Remote actions completed:

- Deployed the backend binary with overlap-safe discovery to rjazhenka.
- Added read-only parent storage `old_compressed_data` rooted at `/mnt/cartolensia-originals/old_drives/compressed_data`.
- Existing child storages remain configured:
  - `old_x12_los20`;
  - `old_nokia5228`;
  - `old_p770`;
  - `old_ze554kl`;
  - `originals`.
- Queued all-storage read-only work:
  - discovery job `7d4299fd-ccd8-499e-9e9d-f9f980521ede`;
  - metadata job `a249204d-ae99-4f3e-8bbd-b66f51e19e9f`;
  - hash job `9887bb70-e87e-4ab3-97ca-87d9ca2eb046`.
- At latest check, older production discovery and metadata jobs were already running and making progress, so the new all-storage jobs remain queued behind them:
  - discovery `6a17fd7b-ef0c-4f6b-86f0-2ab5b46a0966`, about `14,708` scanned at the latest poll;
  - metadata `3804c8e9-e48d-4b9d-b514-f872e3930c28`, about `12,090 / 249,219` at the latest poll.
- AI sidecar is active and AI backfill is running as PID `1801196`, logging to `/var/lib/cartolensia/logs/ai-backfill-20260627T140437Z.log`.
- Latest observed remote counts:
  - `249,219` assets;
  - `218,903` photos;
  - `20,592` videos;
  - `2,359` audio files;
  - `1,806` tracks;
  - `5,560` documents;
  - `5,309` hashed and `243,911` unhashed.
- pgvector remains active: backend `pgvector_ivfflat`, with `500` embedded assets at the latest poll and increasing through backfill.
- Essential export created:
  - `/var/lib/cartolensia/exports/cartolensia-essential-20260627T140721Z.7z`;
  - size about `466 MB`;
  - permission `0600`;
  - contains DB/config manifest only, no originals or thumbnails.

Tests and checks run:

- `gofmt -w internal/storage/storage.go internal/storage/storage_test.go internal/discovery/discovery.go internal/discovery/discovery_test.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/storage ./internal/discovery`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `python3 -m py_compile scripts/remote/run-ai-backfill.py`
- `bash -n scripts/remote/create-essential-export.sh`
- authenticated remote checks for services, stats, storages, jobs, AI status, vector status, backfill logs, and export archive.

Known limitations:

- The all-storage discovery/metadata/hash jobs are queued behind already-running production work; they should drain after the current jobs finish.
- Full hash of all unhashed NAS-backed assets is read-only but very I/O intensive and may take a long time over SMB.
- The backup archive is sensitive because it contains the PostgreSQL database dump. It is stored locally on the production host with restrictive permissions and should be transferred only over SSH.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-27 Jobs Visibility Fix

Issue observed:

- The Jobs header showed `4 running` and `3 queued`, but the visible list only showed one running job.
- Root cause: the Jobs page fetched newest jobs only. Fast AI micro-jobs were constantly pushing older long-running discovery/metadata jobs and queued all-storage jobs below the visible page.

Fixed:

- Backend now supports `GET /api/v1/jobs?sort=active`, which pins running, cancel-requested, and queued jobs before history.
- WebUI now avoids needing a backend restart for this case by fetching and merging:
  - `/api/v1/jobs?running_only=true&limit=100`;
  - `/api/v1/jobs?status=queued&sort=created_at&limit=100`;
  - recent history.
- Jobs page now has explicit `Active / queued` and `Recent history` sections.
- Active/queued jobs have a highlighted card style and show the latest log line/error summary.

Deployed:

- Rebuilt and deployed only `webui/dist` to rjazhenka to avoid interrupting active discovery/metadata jobs.
- Backend active-sort implementation is tested locally and will be picked up with the next backend deploy/restart.

Remote validation:

- `running_only=true` returned 4 jobs:
  - `ai_detect_faces`;
  - `metadata_enrich` around `30189 / 250975`;
  - `discovery` around `66203`;
  - `ai_ocr`.
- queued status returned 3 jobs:
  - `discovery`;
  - `metadata_enrich`;
  - `hash`.

Tests:

- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server`
- `npm --prefix webui run build`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`

Safety: no backend restart was performed; active production jobs were not interrupted. No originals/Samba writes, no missing marking, no DB reset, no commit, no push.

## 2026-06-27 Large List Navigation and Transcript Search

Issue observed:

- GPS/KML Tracks showed `200 tracks` while Stats reported `1,806` tracks.
- Large metadata pages such as OCR, Captions, AI Classification, and Face Gallery had implicit frontend slices without a clear way to load more.
- Large selector-style controls need search/typeahead behavior instead of huge dropdowns.
- There was no dedicated transcript search page for ASR output.

Implemented:

- Added reusable `TypeaheadSearch` Vue component with Bootstrap-style input, Go button, keyboard navigation, and dropdown results.
- GPS/KML Tracks page now:
  - loads tracks in pages of 200 using existing backend `limit`/`offset`/`q`;
  - shows loaded-vs-total track counts;
  - has searchable fuzzy track lookup by name/date/format/distance;
  - includes `Load more` and `Load all`.
- Added `Transcripts` navigation page:
  - lists stored ASR transcripts in pages;
  - supports transcript text/typeahead search;
  - can run ASR on the current scope;
  - opens matching assets from transcript rows and transcript search results.
- OCR and Captions pages now show how many rows are loaded from the shared prediction payload and have `Load more` / `Load all` controls.
- AI Classification and Face Gallery no longer hide rows via silent small template slices; face clusters have explicit `Load more` / `Load all`.
- Backend AI metadata endpoints now support higher limits, offset, and query filtering for predictions/faces. This is tested locally and will take effect on rjazhenka after the next backend deploy/restart.

Remote deployment:

- Rebuilt and deployed `webui/dist` to rjazhenka only, avoiding a backend restart while active indexing/AI jobs are running.
- The live WebUI at `https://192.168.237.126:18443/` now includes the track paging/typeahead and Transcripts page.

Tests:

- `gofmt -w internal/server/server.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `git diff --check`

Known limitations:

- AI/OCR/Captions large-result paging above the old backend cap needs the next backend restart/deploy on rjazhenka. It was intentionally not restarted during active production jobs.
- Track paging works with the current live backend because `/api/v1/gps/tracks` already supports `limit`, `offset`, and `q`.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no backend restart during active jobs
- no commit
- no push

## 2026-06-27 Remote AI Jobs, OCR/Captions Visibility, And JSON Metadata Hardening

Issues investigated:

- OCR rows existed in PostgreSQL, but the OCR page could show no rows because it loaded a mixed first page of AI predictions and filtered client-side.
- Captions had the same hidden first-page problem.
- Global AI prediction listing had an internal PostgreSQL cap before frontend pagination, so task totals were underreported.
- Some metadata enrichment rows failed with `json: unsupported value: NaN`.
- API-triggered AI audit jobs could remain at `running 0/?` when the browser/client disconnected during a long request.
- The remote AI backfill supervisor died once during an app restart because `http.client.RemoteDisconnected` was not retried.

Implemented:

- Added task-aware AI prediction pagination:
  - backend `QueryAIPredictions` returns a PostgreSQL-filtered page and total;
  - `/api/v1/ai/predictions?task=ocr_image` and `task=describe_image` now report real task totals;
  - OCR and Captions pages request task-specific rows and refresh when their filters change.
- Added JSON metadata sanitation for metadata updates, tags, geotags, and multimodal rows:
  - non-finite floats (`NaN`, `Inf`) are converted to JSON `null`;
  - nested maps/slices are sanitized recursively.
- Hardened synchronous AI audit jobs:
  - detached bounded job context survives client disconnect;
  - progress counters are updated during processing, so Jobs shows real `processed / total` values.
- Hardened `scripts/remote/run-ai-backfill.py`:
  - probes past the first already-seen candidate window;
  - retries transient `RemoteDisconnected`, timeout, and connection reset errors.
- Deployed rebuilt backend, WebUI, and backfill supervisor script to rjazhenka.
- Cleaned one stale pre-restart `audio_analyze` job metadata row; replacement backfill work is running.

Remote validation:

- rjazhenka services active:
  - `cartolensia`
  - `cartolensia-ai`
- AI sidecar health: real CUDA-backed mode, with classifier, YuNet, NSFW, OpenCLIP, BLIP, Tesseract OCR, faster-whisper, and audio analysis available.
- Authenticated API checks:
  - `/api/v1/ai/predictions?task=ocr_image&limit=3` returned total `1893`, shown `3`, first task `ocr_image`;
  - `/api/v1/ai/predictions?task=describe_image&limit=3` returned total `1528`, shown `3`, first task `describe_image`.
- Latest remote metadata counts after redeploy:
  - OCR predictions: `2001`;
  - captions: `1546`;
  - classifications: `7780`;
  - safety predictions: `3110`;
  - embeddings: `1546`;
  - transcripts: `214`;
  - audio features: `2450`.
- Active remote jobs after stale cleanup:
  - `discovery` running around `66299` indexed;
  - `hash` running around `41319 / 253858`;
  - `metadata_enrich` running around `87200 / 292344`;
  - current `audio_analyze` batch running `0 / 2`.
- Backfill supervisor relaunched and logged new classify/safety/caption/embed/OCR/faces/audio batches.

Tests:

- `gofmt -w internal/catalog/catalog.go internal/database/database.go internal/database/extended.go internal/database/multimodal.go internal/database/json_sanitize_test.go internal/server/server.go`
- `python3 -m py_compile scripts/remote/run-ai-backfill.py`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/database ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `git diff --check`

Known limitations / next work:

- The backfill driver is still sequential by modality. GPU work is now progressing and visible, but utilization is bursty while CPU-heavy OCR/audio work runs. The next production improvement should be a durable multi-lane AI scheduler with per-device concurrency and VRAM-aware model residency.
- Track map level-of-detail, all-track map date filtering, and the natural-language/LLM query planner remain future work.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to read-only originals or SMB/NAS sources
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-28 Durable Worker-Owned AI Backfill

Implemented:

- Added a durable `ai_backfill` job kind handled by the normal persistent worker pool.
- Added `POST /api/v1/ai/backfill/start`, which expands one operator action into separate missing-work jobs for classification, safety, captions, embeddings, face detection, OCR, audio features, audio transcription, and video-audio transcription.
- Added PostgreSQL table `ai_asset_task_status` to remember per-asset task outcomes, including successful zero-output runs such as no OCR text or no detected faces.
- Added catalog/store APIs for querying missing AI work in bounded batches and for upserting per-asset task status.
- Refactored AI execution so synchronous asset actions and durable worker jobs share the same sidecar call and persistence path.
- Added the Base AI button `Backfill all missing AI metadata`; it queues durable jobs and sends the user to Jobs.
- Updated operations and architecture docs for the durable backfill model.

Validation:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/catalog ./internal/database ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`

Remote deployment:

- Rebuilt the backend binary and WebUI locally.
- Installed the updated release under `/opt/cartolensia/releases/local-20260628T-ai-backfill` on rjazhenka.
- Switched `/opt/cartolensia/current` and restarted `cartolensia` only; PostgreSQL and AI sidecar remained active.
- Increased `workers.max_concurrency` to `6` for the production host so metadata/hash work and multiple AI lanes can progress together.
- Queued full missing-metadata backfill with `limit_per_task=-1`, `batch_size=64`, `max_audio_seconds=2700`, and `max_video_seconds=900`.

Current remote status at the latest poll:

- Services active: `cartolensia`, `cartolensia-ai`, `cartolensia-postgres`.
- GPU active: NVIDIA GeForce RTX 4060 Ti, about 62% utilization, about 3.1 GiB VRAM used at poll time.
- Active jobs include:
  - `hash`: `189705 / 348589`
  - `metadata_enrich`: `26203 / 558898`
  - durable `ai_backfill` lanes for classification, safety, captions, embeddings, faces, OCR, audio features, and transcription.
- `ai_asset_task_status` already contains task outcomes, including:
  - `classify_image`: `1607` succeeded, `57` failed
  - `safety_nsfw`: `1607` succeeded, `57` failed
  - `describe_image`: `1371` succeeded, `54` failed
  - `embed_image`: `6543` succeeded, `80` failed
  - `detect_faces`: `7270` succeeded, `80` failed
  - `ocr_image`: `989` succeeded, `35` failed
  - `analyze_audio`: `147` succeeded, `24` failed
  - `transcribe_audio`: `15` succeeded, `22` failed
- Current metadata output counts:
  - AI predictions total: `67068`
  - OCR predictions: `5469`
  - captions: `6634`
  - faces: `5341`
  - embeddings: `10986`
  - transcripts: `474`
  - audio features: `2908`

Known limitations:

- Some older `api-ai` jobs from the previous synchronous path can still appear in Jobs history. New archive-wide work should use `ai_backfill`.
- Backfill is now durable and parallel by worker lane, but the next performance pass should add a device-aware scheduler that explicitly budgets VRAM and prioritizes interactive LLM/search work over background batches.
- Individual unreadable/problem assets are counted as task failures; the jobs continue and can be retried later after investigation.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to Samba/originals
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-28 Map Track Overlay Count And Performance Fix

Implemented:

- Fixed the map track overlay endpoint so it pages through parsed `gps_tracks` summaries instead of inheriting the normal 200/500-row list cap.
- Fixed selected-track rendering so a `track_id` outside the first GPS-track page is fetched directly with `GetTrack`.
- Split map asset and track limits: `/api/v1/map` now uses `asset_limit` for media points and `track_limit` for track overlays, so media limits no longer truncate track lines.
- Added `track_overlay` response metadata with matched/returned/truncated counts, point budget, and points-per-track for both `/api/v1/map` and `/api/v1/map/tracks`.
- Fixed `/api/v1/map/status` to count parsed GPS/KML/KMZ summaries by pages instead of reporting only the first capped page.
- Added a compact PostgreSQL `gps_track_render_points` table for map overview drawing. New parsed tracks populate two render points automatically, and overview map requests use this cache before falling back to the full `track_points` table.
- Lowered the default all-track overlay point budget to `20000` so large libraries use the overview path by default while selected tracks still draw with detailed geometry.
- Added regression coverage for more than one GPS-track page and for selected tracks outside the first page.

Validation:

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/catalog ./internal/database ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`

Remote production validation:

- Deployed the rebuilt backend/WebUI to rjazhenka as `/opt/cartolensia/releases/local-20260628T-map-overlay-v5`.
- Restarted only the Cartolensia service; PostgreSQL, existing metadata, jobs, and originals were preserved.
- Backfilled `9468` local DB render points for existing parsed tracks in about `9.0 s`.
- Authenticated checks after backfill:
  - `/api/v1/map/status`: `5521` track-like assets, `4736` parsed drawable-track summaries counted in `0.473 s`.
  - `/api/v1/map/tracks?limit=6000&zoom=8&track_point_budget=20000`: `4734` track features returned from `4736` parsed summaries in `0.117 s`, no truncation.
  - `/api/v1/map?zoom=8&asset_limit=1000&track_limit=20000&track_point_budget=20000`: `4934` total features, including `4734` track features, in `0.274 s`, no truncation.
- The two parsed summaries not returned as map features have `point_count=0` (`12.07.2016_12_42_38.gpx` and `12.07.2016_12_42_38_3.kml`), so they cannot draw lines until reparsed with coordinates.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to Samba/originals
- only local Cartolensia DB metadata was added for the render-point cache
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-28 Settings Page Freeze Fix

Root cause:

- Opening Settings used the generic full-page `refresh()` path, which fetched unrelated production-scale data such as map GeoJSON, GPS tracks, AI prediction pages, place cache data, readiness, preview cache data, and plugin settings.
- `/api/v1/settings` also included effective YAML-bound config in the default payload, and the frontend deep-copied that into `pendingConfig` even when the Raw/YAML tabs were not open.

Implemented:

- Made `/api/v1/settings` lightweight by default. It now omits effective config unless `include_effective=1` is explicitly requested.
- Added lazy frontend loading for `/api/v1/settings/effective` only when tabs that edit/review YAML-bound config are opened (`Server`, `Storage`, `Auth`, `AI`, `Raw`).
- Added a Settings-specific refresh path that initially fetches only stats, backend status, lightweight settings, and local auth tokens.
- Moved Settings tab data to on-demand fetches:
  - Components only on Components tab.
  - Readiness only on Readiness tab.
  - GPS tracks only on GPS tab.
  - Map/tile state only on Map tab.
  - Preview cache only on Preview tab.
  - Search/place cache only on Search tab.
  - Transcoding data only on Transcoding tab.
  - AI/vector status only on AI tab.
  - Plugin settings only for the selected plugin on Plugins tab.
- Raw config rendering now uses a precomputed text buffer instead of template-time `JSON.stringify(...)` on every render.

Validation:

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server ./internal/database ./internal/catalog`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`

Remote production validation:

- Deployed to rjazhenka as `/opt/cartolensia/releases/local-20260628T-settings-fast`.
- Restarted only the Cartolensia service.
- Authenticated endpoint timings on rjazhenka:
  - `/api/v1/settings`: `3193` bytes, `0.002 s`, no `effective` field.
  - `/api/v1/settings?include_effective=1`: `6048` bytes, `0.002 s`.
  - `/api/v1/settings/effective`: `4903` bytes, `0.002 s`.
  - Settings initial API path (`stats`, `backend/status`, lightweight `settings`, `auth/tokens`): `0.571 s` total.

Safety confirmation:

- no writes to `/mnt/Models/rclone`
- no writes to Samba/originals
- no DB reset
- no missing marking
- no commit
- no push

## 2026-06-28 Local LLM Chat/Agent Hardening

Implemented a safer local LLM path for Ask Cartolensia:

- added `/api/v1/knowledge/llm/status` to report configured mode, provider, endpoint, model, reachable state, and known model IDs when the runtime exposes them;
- extended Knowledge chat responses with clickable media citations and optional read-only SQL tool summaries;
- added a local LLM tool planner that can request only allowlisted tools: bounded media search, knowledge fact search, knowledge relation search, and guarded read-only SQL over `cartolensia_search_*` views;
- kept Cartolensia as the policy enforcement point: the model never receives database credentials or write-capable tools, and generated SQL still passes through the existing read-only allowlist, timeout, and row limit;
- improved vLLM/OpenAI-compatible URL handling so both `http://host:8000` and `http://host:8000/v1` work;
- added Ollama keep-alive/low-temperature options based on the configured idle unload setting;
- improved Russian month-range parsing so direct searches like `kind:photo май-август 2025` add a `2025-05..2025-08` date token instead of treating the month range only as free text;
- updated Knowledge Base UI with local LLM readiness status and media citation cards.

Validation so far:

- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...` passed locally.
- `npm --prefix webui run build` passed locally.
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server ./internal/database` passed locally after adding restart-safe LLM env defaults.

Additional implementation:

- Local LLM defaults can now be supplied at process start:
  - `CARTOLENSIA_SEARCH_RUNNER_MODE`
  - `CARTOLENSIA_KNOWLEDGE_RUNNER_MODE`
  - `CARTOLENSIA_KNOWLEDGE_LLM_PROVIDER`
  - `CARTOLENSIA_KNOWLEDGE_LLM_ENDPOINT`
  - `CARTOLENSIA_KNOWLEDGE_LLM_MODEL`
  - `CARTOLENSIA_KNOWLEDGE_LLM_TIMEOUT_SECONDS`
  - `CARTOLENSIA_KNOWLEDGE_LLM_IDLE_UNLOAD_MINUTES`
  - `CARTOLENSIA_KNOWLEDGE_LLM_MAX_CONTEXT_ITEMS`
- This keeps local LLM mode restart-safe for production systemd/container deployments while preserving in-session Settings overrides.

Remote production work:

- Started a local Ollama runtime in Docker on rjazhenka with data under `/var/lib/cartolensia/ollama`.
- Pulled `qwen3:8b` successfully (`5.2 GB`, Q4_K_M, reported by Ollama as `8.2B` parameters).
- Deployed the LLM-enabled backend/WebUI to `/opt/cartolensia/releases/local-20260628T-llm-chat`.
- Corrected a deployment mistake where an overlay `rsync --delete` removed bundle support files such as `bin/cartolensia-env`; restored the release from `/opt/cartolensia/releases/local-20260628T-settings-fast`, overlaid the new build without deletion, and restarted Cartolensia back on the PostgreSQL store.
- Verified `/api/v1/knowledge/llm/status` sees the Ollama endpoint and the `qwen3:8b` model when runtime settings are active.

Remaining remote action:

- The last SSH deployment/configuration command was blocked by the platform approval/usage gate before the restart-safe environment file could be installed on rjazhenka. The required next remote action is to set the `CARTOLENSIA_KNOWLEDGE_*` environment variables for the `cartolensia` service, restart the service, and re-run an authenticated `/api/v1/knowledge/chat` probe.
- No workaround was attempted after the remote command was rejected.

Safety confirmation:

- no writes to `/mnt/Models/rclone` or originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-29 Settings/Tasks/Reverse-Geocode Stabilization

Implemented/updated this run:

- Added a durable `reverse_geocode` job and
  `POST /api/v1/places/reverse-geocode/start`.
- Registered the new job with the worker manager so it is visible and
  cancellable from Jobs.
- Asset Detail place rendering now uses both cached place bboxes and nearby
  cached centroids, so opening an asset or clicking `Refresh place` finds local
  hierarchy matches more consistently.
- Added a `Tasks` navigation page for starting discovery/indexing, hash,
  metadata enrichment, local reverse geocoding, previews, and scoped AI
  backfill jobs from one page.
- Reduced Settings page render pressure by keeping auth/API-token UI work under
  the Auth tab and avoiding stale `asset_id` query parameters when changing
  non-asset routes.
- Hardened Knowledge Graph preview interaction with bounded graph rendering,
  drag-to-pan, wheel zoom, selected-node details, and clickable asset links.

Validation:

- `gofmt -w internal/server/reverse_geocode.go internal/app/app.go internal/server/server.go internal/server/server_test.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- Built `/tmp/cartolensia-fix` from the verified source tree.
- Deployed the verified backend and WebUI bundle to rjazhenka and restarted only
  the `cartolensia` service.
- Authenticated remote validation:
  - `/api/v1/settings` returned successfully;
  - `/api/v1/jobs?limit=8` showed active background processing;
  - a 10-coordinate `reverse_geocode` test job was picked up by the worker with
    `10 / 10` progress and `0` errors;
  - a full local-cache reverse-geocode pass was queued with `limit=-1`,
    `batch_size=1000`, `online=false`.
- Final remote poll: `cartolensia` was `active`; the full cache-only
  `reverse_geocode` pass was running at `98,026` scanned coordinates with
  `21,350` cached-place matches and `0` errors, while AI backfills and hashing
  continued from persisted state.

Known limitations:

- The queued full reverse-geocode pass is cache-only by default. Broad online
  geocoding remains intentionally opt-in; for large libraries, use imported
  local geodata or a self-hosted Nominatim/Pelias/Photon endpoint instead of
  public bulk reverse-geocoding.
- Knowledge Graph still previews a bounded working set for browser performance;
  deeper graph paging/filtering is available through the list controls.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-28 Remote Production Stabilization, Usage, LLM Actions, And AI Backfill

Implemented and deployed:

- Added cached self-signed TLS certificate generation. Cartolensia now reuses an existing non-expired generated certificate under the configured cache directory instead of regenerating a new certificate on every restart.
- Added `GET /api/v1/environment/usage`, reporting PostgreSQL database size, largest application relations, and Cartolensia-owned cache/model/component/export/runtime directory sizes. Originals are intentionally excluded from this scan.
- Added Settings/Stats UI cards for database and environment space usage.
- Extended the local Knowledge/LLM agent action model with guarded action cards:
  - transcode recommendations based on asset metadata/ffprobe-derived metadata;
  - cache-only transcode session creation from chat actions;
  - segmented video-series detection for camera files such as numbered `mp4` parts delimited by `thm` files;
  - all actions remain non-destructive and write only to Cartolensia cache/export areas.
- Hardened AI backfill jobs:
  - long-running AI backfills now heartbeat and refresh progress/lease state;
  - failed/skipped/succeeded asset task statuses are excluded from future missing-work scans so one bad asset does not permanently block a full backfill;
  - batch failures advance past the failed asset IDs when possible instead of repeatedly retrying the same first batch.
- Added local full `.7z` package wrapper: `scripts/release/build-local-full-7z.sh`.
- Extended the local full package builder with LLM executor env/script support for Ollama/vLLM-style local endpoints.

Remote production status:

- Active remote release: `/opt/cartolensia/releases/local-20260628T-env-llm-actions2`.
- Public LAN URL remains `https://192.168.237.126:18443/`.
- `cartolensia`, `cartolensia-ai`, and `cartolensia-postgres` are active.
- TLS startup log confirms cached certificate reuse from `/var/lib/cartolensia/cache/tls/`.
- Local LLM status is configured and reachable through Ollama:
  - provider: `ollama`;
  - endpoint: `http://127.0.0.1:11434`;
  - model: `qwen3:8b`.
- AI sidecar is CUDA-backed and reachable on `http://127.0.0.1:19090`.
- pgvector remains active as `pgvector_ivfflat`.
- Current indexed production scale at validation:
  - assets: `553896`;
  - locations: `620450`;
  - photos: `443933`;
  - videos: `28580`;
  - audio: `8781`;
  - documents: `133635`;
  - tracks: `5521`;
  - total indexed bytes: about `12.56 TB`.
- Environment usage at validation:
  - PostgreSQL database: about `14.30 GB`;
  - largest relation: `track_points`, about `11.82 GB`, `61102323` rows;
  - cache directory: about `329 MB`;
  - model cache: about `3.44 GB`;
  - AI venv: about `123 MB`.
- Essential backup export completed:
  - `/var/lib/cartolensia/exports/cartolensia-essential-20260628T024819Z.7z`;
  - size: about `1009 MB`;
  - includes DB/config metadata, excludes originals and thumbnail/model/cache payloads.

AI processing status at validation:

- Verification backfill succeeded for a bounded 24-photo sample:
  - OCR: `24/24`, stored `6`;
  - face detection: `24/24`, stored `142`.
- Full missing-work backfill is running/queued:
  - running: OCR, face detection, embeddings, descriptions/captions, safety, and classification;
  - queued: audio feature extraction, audio transcripts, and video audio transcripts;
  - leases and counters are updating, so the earlier “stuck at 0” behavior is fixed for the current deployment.
- A small number of pre-fix one-batch failures remain in job history as historical rows; new backfill jobs are advancing.

Validation:

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `bash -n scripts/release/build-local-full-tarzst.sh scripts/release/build-local-full-7z.sh scripts/remote/create-essential-export.sh`
- Authenticated remote checks for stats, jobs, environment usage, AI status, LLM status, and GPU state.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-29 Local LLM Chat Streaming And Attachments

Implemented this run:

- Fixed the Knowledge Base "Ask" path that appeared to do nothing or showed a browser `NetworkError` by adding an authenticated streaming chat endpoint:
  - `POST /api/v1/knowledge/chat/stream`;
  - Server-Sent Events for `status`, `tool`, `token`, `error`, and `final`;
  - compacted final citations/tool payloads so large transcript/fact rows do not swamp the browser.
- Added token-level Ollama streaming. The UI now shows progress while local tools run and displays answer text as the local model emits tokens.
- Added a dedicated `LLM Chat` WebUI page:
  - menu entry and route `?page=llm-chat`;
  - chat bubbles, local/deterministic mode toggle, model selector, LLM health badge, tool trace details, clickable asset citations, and suggested guarded action cards;
  - attachment support with paste and file picker for images and text-like files;
  - image attachments are passed to Ollama-compatible models when possible.
- Added a safe fallback for text-only local models: if the selected Ollama model rejects image inputs, Cartolensia retries using text/filename attachment context instead of failing the whole answer.
- Kept the existing `Knowledge Base` ask box, but switched it to the same streaming API and added visible recent tool events.

Tests run locally:

- `gofmt -w internal/server/knowledge.go internal/server/knowledge_llm_test.go internal/server/server.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go build -o /tmp/cartolensia-chat-fix ./cmd/cartolensia`

Remote deployment/validation:

- Deployed the verified backend binary to `/opt/cartolensia/current/bin/cartolensia` on rjazhenka.
- Deployed the verified WebUI bundle to `/opt/cartolensia/current/webui/dist`.
- Restarted only the `cartolensia` service; PostgreSQL, originals/Samba mounts, Ollama, and the AI sidecar were not reset.
- Authenticated remote `/api/v1/knowledge/llm/status` returned local LLM mode with provider `ollama`, model `qwen3:8b`, endpoint `http://127.0.0.1:11434`, `configured=true`, and `reachable=true`.
- Authenticated remote `/api/v1/knowledge/chat/stream` returned streamed `status`, `tool`, `token`, and `final` SSE events.
- Remote AI status after deployment showed:
  - CUDA sidecar reachable and `status=ok`;
  - device `cuda`;
  - pgvector backend active;
  - ASR, OCR, captions, classifier, face detector, OpenCLIP, and safety models loaded.
- Remote jobs after restart showed long-running production backfills continuing/resuming from persistent job state. The app logged expired job leases being returned to the queue after the restart.

Known limitations:

- The current configured chat model is `qwen3:8b`, which is text-only. The WebUI/backend now support image attachments and graceful fallback, but a vision-capable local model still needs to be installed/configured before true image-question answering is available.
- The chat page is a native Vue implementation rather than Gradio. This keeps it authenticated, offline, single-bundle, and consistent with the rest of Cartolensia.
- The production indexing/AI pipeline is still not complete for the entire NAS-scale library; jobs are continuing in PostgreSQL-backed resumable queues.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-28 Continuation: SSD-Aware Preview Mode, PostgreSQL Tuning, And AI Queue Recovery

Scope:

- Focused on the production rjazhenka deployment while the full NAS/originals scan continued.
- Addressed SSD write amplification, misleading long-job progress, stale canceled jobs, and missing AI backfill lanes.
- Kept originals/Samba storage strictly read-only and did not reset PostgreSQL.

Implemented and deployed:

- Added `cache.persistent_previews` with production configs defaulting to `false`.
- Added on-demand image preview generation:
  - serves resized JPEG previews directly from memory;
  - does not write preview files;
  - does not upsert preview-cache DB rows when persistent previews are disabled.
- Updated `/api/v1/previews/status` to report preview mode (`on_demand` or `persistent`).
- Added and deployed `scripts/remote/tune-postgres-for-large-ingest.sh`.
- Tuned remote PostgreSQL for large ingest with safer SSD behavior:
  - `wal_compression=pglz`;
  - `checkpoint_timeout=15min`;
  - `checkpoint_completion_target=0.9`;
  - `max_wal_size=8GB`;
  - `effective_io_concurrency=200`;
  - `random_page_cost=1.1`;
  - `maintenance_work_mem=512MB`;
  - lower autovacuum analyze/vacuum scale factors.
- Kept `synchronous_commit` unchanged; no unsafe durability shortcut was applied.
- Fixed AI backfill resume accounting so restarted jobs keep saved `processed`, `stored`, and `failed` counters instead of looking reset.
- Fixed stale `cancel_requested` cleanup for jobs with no live worker lease.
- Fixed hash progress accounting so resumed hash jobs include already hashed files in `progress_total`.

Remote validation:

- rjazhenka is running the updated backend from `/opt/cartolensia/current/bin/cartolensia`.
- TLS is reusing the cached self-signed certificate under `/var/lib/cartolensia/cache/tls`.
- `/api/v1/previews/status` reports `mode: on_demand` and `persistent_previews: false`.
- A sample photo preview returned `X-Cartolensia-Preview-Cache: on-demand`.
- PostgreSQL tuning was applied and reloaded successfully.
- Stale canceled jobs were cleaned up; only active production work remained.
- AI sidecar is reachable and CUDA-backed; vector store is `pgvector_ivfflat`.
- Missing AI lanes were queued and then observed running:
  - OCR;
  - captions/descriptions;
  - safety/NSFW;
  - embeddings;
  - classification;
  - face detection;
  - video transcription.
- Hash progress corrected after the worker lease cycle to `157657 / 265305` instead of showing current progress above total.
- A live resource sample showed high system load and continuous sidecar requests, but low instantaneous GPU utilization. Current throughput is mostly constrained by SMB reads, image/audio decoding, OCR, and PostgreSQL writes rather than a single dense GPU batch.

Current large-table state observed:

- `track_points`: about `70.8M` rows and `20.1 GB`;
- `gps_track_render_points`: about `7.7M` rows and `1.9 GB`;
- `asset_embeddings`: about `1.6 GB`;
- PostgreSQL DB total: about `26.6 GB`;
- Cartolensia cache/model/component directories are outside originals.

Optimization status:

- Normal map rendering uses `gps_track_render_points`, not raw `track_points`.
- Raw `track_points` remain necessary for exact track detail, sync, nearest-point, and full-fidelity analysis.
- No heavy `VACUUM FULL`, index rebuild, or raw-track compaction was run during active ingest because those would cause large SSD writes.
- Planner statistics were refreshed with `ANALYZE` for high-traffic metadata/search/render tables.
- Further possible future optimization, if DB size becomes a limiting factor, is a planned migration to chunked/compressed raw track storage plus retained render-cache rows. That was not attempted in this run because it is a high-blast-radius migration.

Tests run:

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- remote authenticated checks for jobs, AI status, preview mode, PostgreSQL tuning, and GPU/service activity.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-28 Heatmap Page

Scope:

- Added a Google Photos-style density view for geotagged media and GPS track points.
- Kept the regular Map page behavior unchanged: Map still shows track polylines and media clusters, while Heatmap hides track polylines and renders track/media coordinates as a smoothed density layer.

Implemented:

- Added backend heatmap endpoint:
  - `GET /api/v1/map/heatmap`;
  - returns GeoJSON point features for heat rendering;
  - media geotags contribute weight `1.0`;
  - GPS track render samples contribute weight `0.45`;
  - uses existing bbox/date/filter parameters and the precomputed GPS track render cache;
  - supports `include_assets`, `include_tracks`, `asset_limit`, `track_limit`, `track_point_budget`, and GPS jump filtering.
- Added WebUI `Heatmap` page:
  - OpenLayers `HeatmapLayer`;
  - OSM tiles via the existing local tile proxy;
  - media clusters/asset popups remain visible and clickable like the regular Map page;
  - track polylines are hidden on the Heatmap page;
  - controls for media-kind, album, track scope, media clusters, heat radius, blur, opacity, and track point budget.
- Added direct route support:
  - `?page=heatmap`;
  - sidebar navigation entry with icon.
- Added endpoint coverage:
  - synthetic geotagged photo plus GPS track points must produce both asset and track heat points.

Tests run:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server ./internal/database`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go build -o /tmp/cartolensia-heatmap ./cmd/cartolensia`

Remote validation after deployment:

- Services active: `cartolensia`, `cartolensia-ai`, `cartolensia-postgres`.
- `/api/v1/map/heatmap?zoom=8&asset_limit=20000&track_limit=20000&track_point_budget=60000` returned:
  - `30324` heat features;
  - `20000` media geotag points;
  - `10324` GPS track sample points;
  - matched `5839` parsed track summaries;
  - returned `5162` drawable track-derived sample groups;
  - hidden large jump segments: `673`;
  - truncated: `false`.
- `/api/v1/map/assets?zoom=8&cluster=true&asset_limit=10000` returned `49` media cluster features for the same style of overlay.
- Background production jobs remained active after deploy:
  - discovery;
  - hash;
  - metadata enrichment;
  - four AI backfill workers.

Known limitations:

- Heatmap density is intentionally viewport/budget bounded. Raising the track point budget gives a denser map at the cost of browser rendering work.
- The heatmap uses cached track render samples, not raw full-resolution tracks, so it stays responsive for thousands of indexed tracks.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-28 GPS Render Cache And Places Hierarchy

Scope:

- Continued the remote production run on rjazhenka without touching originals or Samba mounts.
- Focused on the Map page freeze/coverage issue for thousands of parsed tracks and added a Places browsing surface backed by local reverse-geocode cache data.

Implemented:

- Added a PostgreSQL-backed multi-zoom GPS track render cache:
  - levels: `overview`, `z0`, `z6`, `z10`, `z13`, `z16`;
  - points are downsampled per track/level and reused by map track batch APIs;
  - new parsed tracks now precompute render rows during `UpsertTrackPoints`;
  - old parsed tracks can be refreshed through `/api/v1/map/tracks/render-cache/refresh`.
- Added render cache status endpoint:
  - `GET /api/v1/map/tracks/render-cache/status`;
  - reports total renderable tracks, cached tracks, missing tracks, and point counts per level.
- Added cache refresh endpoint:
  - `POST /api/v1/map/tracks/render-cache/refresh`;
  - writes only local PostgreSQL cache rows and skips zero-point parsed summaries.
- Changed map track batch loading to use the cached render level matching the requested point budget, with fallback for any track that does not have cached rows yet.
- Added a Places hierarchy API and page:
  - `GET /api/v1/places/hierarchy`;
  - groups cached place data by hierarchy such as country, region, city, road/street/local place;
  - reports bounded asset and track counts for each place bbox;
  - uses local `place_cache` only and does not call online geocoders automatically.
- Added frontend navigation for `Places`, local query filtering, hierarchy cards, and quick `place:<name>` search links.

Tests run:

- `gofmt -w $(find internal cmd -name '*.go' -print)`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/database ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go build -o /tmp/cartolensia-track-places ./cmd/cartolensia`

Remote validation after deployment:

- Services active: `cartolensia`, `cartolensia-ai`, `cartolensia-postgres`.
- Render cache status:
  - renderable parsed tracks: `5835`;
  - cached tracks: `5835`;
  - missing cache: `0`;
  - cached points by level:
    - `overview`: `11670`;
    - `z0`: `23336`;
    - `z6`: `93261`;
    - `z10`: `372573`;
    - `z13`: `1482182`;
    - `z16`: `5802893`.
- Map track batch check:
  - `/api/v1/map/tracks?track_limit=20000&track_point_budget=20000&zoom=8`;
  - returned `5162` drawable features;
  - matched `5839` parsed summaries;
  - hidden GPS jump segments: `673`;
  - truncated: `false`.
- Places hierarchy check:
  - `4` cached place hierarchy entries available;
  - first entry: `Vanadzor, Lori Province, Armenia`;
  - local counts for that cached place: `60` assets, `28` tracks.
- Current production jobs still active:
  - discovery;
  - hash;
  - metadata enrichment;
  - four AI backfill workers.
- AI status:
  - enabled: `true`;
  - worker: `ai-local` `ok`;
  - device: `cuda`;
  - vector store: `pgvector_ivfflat`;
  - embedded assets: `102730`;
  - predictions: `916114`;
  - face detections: `62096`;
  - asset tags: `259475`.

Known limitations:

- The Places page currently reflects the durable local place cache. It is intentionally cache-only unless an operator explicitly runs online reverse-geocoding/enrichment.
- Four parsed track summaries had zero points and are excluded from render-cache completeness because they cannot produce drawable geometry.
- Full library discovery and AI enrichment remain long-running production jobs; this run did not reset or requeue them unnecessarily.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-28 GPS Track Jump Filter

Implemented and deployed a default-on visualization filter for GPS tracks with large discontinuities, targeting spoofed GPS samples and aircraft/jump artifacts that drew long misleading lines across the map.

What changed:

- Added runtime settings:
  - `gps.hide_large_track_jumps`, default `true`;
  - `gps.track_jump_threshold_m`, default `10000`.
- Added Map page controls under the Layers panel:
  - a Bootstrap switch for hiding large GPS jumps;
  - a configurable threshold in meters.
- Applied the same filter to:
  - `/api/v1/map`;
  - `/api/v1/map/tracks`;
  - track previews and track thumbnails.
- Track geometry is split into multiple line segments when consecutive points exceed the threshold. The original parsed points and original files are not modified.
- API responses now expose filter metadata:
  - whether jump hiding was enabled;
  - the threshold;
  - how many jump segments were hidden.
- Map and track-preview warnings explain when large jumps were hidden and how to inspect raw geometry by disabling the filter.

Tests run:

- `gofmt -w internal/server/extended.go internal/server/track_preview.go internal/server/settings.go internal/server/server.go internal/server/server_test.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server ./internal/database ./internal/workers`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- `git diff --check`

Remote deployment:

- Rebuilt the backend binary with `CGO_ENABLED=0`.
- Deployed the backend to `/opt/cartolensia/current/bin/cartolensia` on rjazhenka.
- Deployed the rebuilt WebUI bundle to `/opt/cartolensia/current/webui/dist`.
- Restarted only the `cartolensia` service.

Remote validation:

- `/api/v1/map/status` reports `9368 / 9563` track-like assets parsed.
- Filtered request:
  - `/api/v1/map/tracks?track_limit=20000&track_point_budget=20000&zoom=8&hide_track_jumps=true&track_jump_threshold_m=10000`
  - returned `8214` drawable features from `9368` matched tracks;
  - reported `hidden_jumps: 1152`;
  - returned the warning: hidden GPS track jump segments over `10000 m`.
- Raw request:
  - `/api/v1/map/tracks?track_limit=20000&track_point_budget=20000&zoom=8&hide_track_jumps=false&track_jump_threshold_m=10000`
  - returned `9366` drawable features from `9368` matched tracks;
  - reported `hidden_jumps: 0`.

Known limitations:

- This is a visualization/cache filter, not data deletion. Spoofed points remain available by disabling the filter.
- The heuristic is distance-based. A future refinement could add speed-based, return-to-track, altitude, and aircraft-mode classification.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-28 Continuation: Production Queue Supervision And Settings Latency Fix

Live remote state captured before the platform blocked further SSH escalation:

- Public LAN URL remained `https://192.168.237.126:18443/`.
- AI sidecar was reachable and CUDA-backed:
  - worker status: `ok`;
  - device: `cuda`;
  - vector store: `pgvector_ivfflat`.
- Indexed production scale at the check:
  - assets: `511867`;
  - photos: `444033`;
  - videos: `28580`;
  - audio: `9506`;
  - documents: `133672`;
  - tracks: `5521`;
  - hashed: `369607`;
  - unhashed: `251705`;
  - total indexed bytes: about `12.57 TB`.
- AI metadata counts were increasing:
  - embedded assets: `47617`;
  - predictions: `383601`;
  - face detections: `20369`;
  - asset tags: `108951`.
- Active/queued remote work included:
  - running full AI backfills for classification, safety, descriptions/captions, embeddings, faces, OCR, audio transcription, and video transcription;
  - running discovery with folder worker counters updating;
  - running targeted track metadata enrichment for `ZE554KL/GPSLogger`;
  - queued hashing of unhashed assets;
  - queued metadata-only missing enrichment pass.
- GPU telemetry at the sample: about `30%` utilization, `3615/16380 MB` VRAM, `52 W`, `53 C`.

Local fixes in this continuation:

- Made Settings -> General stop auto-loading `GET /api/v1/environment/usage`.
- The environment usage card now loads only on explicit “Refresh usage” or through the Stats page, keeping Settings responsive on large installations where cache/model/component directories can contain many files.
- Added a UI note explaining that usage scanning is intentionally on-demand and excludes original storage.

Validation:

- `npm --prefix webui run build`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server ./internal/database ./internal/catalog ./internal/metadata ./internal/tlsutil`
- `git diff --check && GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`

Blocked item:

- Further remote SSH monitoring/deployment was blocked by the platform approval/usage gate after the first successful status check. No indirect workaround was attempted.
- Next remote action when SSH is available: re-check the active queue, verify the track metadata job completed under the fixed track-summary ordering code, retry only remaining unparsed track assets if needed, and deploy the Settings lazy-usage WebUI change.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-28 Continuation: Map Coverage And Production Job Recovery

Scope:

- Focused on the reported gap where many indexed GPS tracks and geotagged media were visible in list/stat pages but not fully represented in the Map page.
- Kept the remote originals/Samba mounts strict read-only.
- Did not reset PostgreSQL, run missing-file marking, commit, or push.

Root causes found:

- Map asset geotag queries reused the generic list-page normalizer, so high-limit map requests were silently capped at the normal UI list cap. This meant the map could show a small subset even while Stats/Explorer/GPS pages had far more indexed records.
- Several long-running production jobs had expired leases after restarts or long task stalls. When all worker slots were occupied, expired running leases were not released promptly before normal dispatch, so some queued AI/metadata work could appear stuck.
- Track-map coverage was also genuinely incomplete for some assets: many track-like files were indexed, but not all had parsed track geometry yet. Metadata enrichment is required to reduce that remaining gap.

Fixes implemented locally and deployed to rjazhenka:

- Added a map-specific geotag page normalizer:
  - default map geotag limit: `10000`;
  - hard cap: `100000`;
  - negative offsets clamp to `0`.
- Wired `/api/v1/map/assets` through the map-specific normalizer so map rendering can request production-scale point batches without being capped at the generic list size.
- Added worker-manager stale lease recovery on every poll before dispatch:
  - expired `running` and `cancel_requested` jobs are released/requeued through the existing persistent job lease API;
  - recovery is logged and does not delete job state.
- Raised the frontend Map page asset request from `1000` to `10000` so the live UI uses the backend's production map batch size instead of continuing to under-request geotagged photos/videos.
- Deployed the rebuilt backend binary to `/opt/cartolensia/current/bin/cartolensia`.
- Deployed the current verified WebUI bundle to `/opt/cartolensia/current/webui/dist`.
- Restarted `cartolensia` only; PostgreSQL and the AI sidecar were not reset.

Tests run:

- `gofmt -w internal/workers/workers.go internal/workers/workers_test.go internal/database/extended.go internal/database/database_test.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/workers ./internal/database`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- Re-ran `npm --prefix webui run build` after the frontend map asset-limit change and deployed the resulting WebUI bundle.

Remote validation:

- `/api/v1/map/status` improved from the earlier continuation baseline of `5328 / 9563` parsed track-like assets to `7866 / 9563`.
- Final post-verification poll improved further to `8187 / 9563` parsed track-like assets while metadata enrichment continued running.
- Current map status at validation:
  - indexed assets: `609359`;
  - track-like assets: `9563`;
  - parsed tracks: `8187`;
  - unparsed track-like assets: `1376`;
  - `tracks_truncated: false`.
- `/api/v1/map/assets?asset_limit=10000&zoom=4&clusters=false` returned `10000` raw geotag features, confirming the generic `200`-row cap is no longer applied to map asset queries.
- `/api/v1/map/tracks?track_limit=20000&track_point_budget=20000&zoom=4` returned `7865` drawable track features with `matched: 7868` and `truncated: false`.
- A prior track overlay check returned `7865` drawable track features with no truncation. The final status poll confirmed additional parsed tracks were added after that overlay check; the next map overlay refresh will include the newly parsed summaries.
- Active production jobs at validation included:
  - running discovery;
  - running hash;
  - running broad metadata enrichment;
  - running targeted track metadata enrichment;
  - running AI backfills for classification, safety, captions/descriptions, audio transcription, and video transcription;
  - queued AI backfills for embeddings, faces, and OCR after lease recovery.
- AI status at validation:
  - enabled: `true`;
  - worker: `ok`;
  - device: `cuda`;
  - vector store: `pgvector_ivfflat`;
  - embedded assets: `97595`;
  - predictions: `894054`;
  - face detections: `60243`;
  - asset tags: `255871`.

Known limitations / next work:

- The entire production library is not fully processed yet. The correct safe state is to leave discovery, metadata, hashing, and AI backfill jobs running and resumable.
- `1376` track-like assets still need metadata enrichment or have unavailable/unparseable read-only sources. The Map page now reports this as a real processing gap, not a UI cap.
- `old_p770` remained unavailable in the latest storage health checks because its configured source path was not mounted or not available from the NAS.
- GPU utilization remains bursty because the workload alternates between SMB I/O, decode/ffprobe, OCR/ASR, and model inference; the CUDA sidecar is active and jobs are advancing.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-29 Reverse-Geocoder Provider Expansion

Implemented this run:

- Added provider-aware reverse geocoding while keeping Cartolensia local/cache-first by default.
- Added `GET /api/v1/places/providers` to report provider readiness, active provider, active URL, locale, rate limit, policy notes, and redacted Google API-key status.
- Added provider support for:
  - local Cartolensia place cache;
  - OpenStreetMap Nominatim public endpoint;
  - self-hosted Nominatim-compatible endpoint;
  - Photon-compatible endpoint;
  - Pelias-compatible endpoint;
  - opt-in Google Geocoding API using `CARTOLENSIA_GOOGLE_GEOCODING_API_KEY` without returning the secret. Google caching additionally requires `CARTOLENSIA_GOOGLE_GEOCODING_CACHE_ACK=I_ACCEPT_GOOGLE_TERMS`.
- Added locale-aware online reverse-geocode calls through `search.geocoder_locale` and cache rows that remember provider+locale, e.g. `nominatim:ru,en`.
- Added `search.geocoder_user_agent`, `search.geocoder_contact_email`, and `search.geocoder_min_interval_ms` runtime settings.
- Added per-process geocoder rate limiting. Public geocoder calls remain user-triggered only and are not run as automatic bulk enrichment.
- Updated the Places page to show provider readiness cards, configured locale, policy notes, and Google secret status.
- Updated Settings -> Search/Places runtime controls and operations/architecture docs.

Tests run:

- `npm --prefix webui run build`
- `gofmt -w internal/server/server.go internal/server/settings.go internal/server/server_test.go`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `git diff --check`

Remote deployment/validation:

- Built `/tmp/cartolensia-geocoder` from the verified source tree.
- Deployed `/tmp/cartolensia-geocoder` to `/opt/cartolensia/current/bin/cartolensia` on rjazhenka.
- Deployed `webui/dist/` to `/opt/cartolensia/current/webui/dist/`.
- Restarted only the `cartolensia` service; PostgreSQL, AI sidecar, and original/Samba mounts were not reset or modified.
- Remote `/api/v1/health` returned `ok`.
- Authenticated remote `/api/v1/places/providers` returned active provider `nominatim`, `online_enabled=false`, local cache enabled, Nominatim configured, Google key redacted/not configured, and `google_cache_ack=false`.
- Authenticated remote `/api/v1/places/hierarchy?limit=5` returned `4` cached rows and confirmed the hierarchy endpoint still works after deployment.

Known limitations:

- No bulk online reverse-geocoding was started. For production-scale enrichment, use imported local geodata or a self-hosted Nominatim/Pelias/Photon endpoint.
- Google support is opt-in and terms-dependent; Cartolensia redacts the API key and refuses to cache Google reverse-geocode rows unless the explicit cache-terms acknowledgement environment variable is present.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-29 Local LLM Chat Streaming Follow-Up

Implemented/updated this run:

- Documented the new authenticated Knowledge/LLM streaming endpoint and the
  dedicated `LLM Chat` page in `docs/OPERATIONS.md`, `docs/ARCHITECTURE.md`,
  and `docs/SECURITY.md`.
- Clarified that `POST /api/v1/knowledge/chat/stream` emits Server-Sent Events:
  `status`, `tool`, `token`, `error`, and `final`.
- Documented the local-only attachment policy:
  - text attachments are summarized into the prompt;
  - image attachments are passed only to local Ollama-compatible vision models;
  - text-only models such as the current `qwen3:8b` deployment retry with
    filename/text context instead of failing.
- Clarified that the production UI stays native Vue instead of Gradio so auth,
  CSRF, offline assets, mobile layout, and guarded action cards remain inside
  Cartolensia.

Tests run:

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`

Remote validation:

- Authenticated `GET /api/v1/knowledge/llm/status` on rjazhenka reported
  provider `ollama`, model `qwen3:8b`, `configured=true`, and `reachable=true`.
- Authenticated `POST /api/v1/knowledge/chat/stream` on rjazhenka was verified
  with the production CSRF flow from `/api/v1/auth/csrf`. It emitted `status`,
  `tool`, and `token` events from the local Ollama-backed runner.
- Remote `/api/v1/ai/status` still reports the CUDA sidecar as configured with
  Tesseract OCR, faster-whisper, BLIP captioning, OpenCLIP, safety, classifier,
  and YuNet face detection loaded or available.
- Existing production backfill jobs remain active/resumable; no queue or DB
  reset was performed.

Known limitations:

- The currently configured local model, `qwen3:8b`, is text-only. The new image
  attachment path is ready for a local vision model, but a vision-capable model
  has not been installed in this pass.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-29 Explorer Month Filter And Asset Album UX Fix

Implemented/updated this run:

- Moved Explorer month filtering into the backend Explorer query path so folder
  rows and file rows are filtered consistently by `YYYY-MM`.
- Added `month` support to PostgreSQL-backed and in-memory Explorer views.
- Changed Explorer folder navigation and Apply/Clear filters to reload only the
  Explorer view instead of refreshing jobs, maps, settings, AI status, and every
  other dashboard panel.
- Changed Explorer search-box behavior to use the indexed Explorer query rather
  than replacing the folder view with separate global Search results.
- Added normal anchor behavior for Explorer breadcrumbs/folders, so middle-click
  and modified-click can open folders in a new tab.
- Added `GET /api/v1/assets/{id}/albums` backed by a direct album membership
  query.
- Added Asset Detail storage-folder links for every asset location, preserving
  storage scope when opening neighboring files in Explorer.
- Added Asset Detail album membership management:
  - current album memberships are listed on the asset page;
  - assets can be removed from albums in place;
  - assets can be added to an existing album;
  - `New album...` creates an album and immediately adds the current asset.

Tests run:

- `gofmt -w internal/catalog/catalog.go internal/catalog/catalog_test.go internal/catalog/extended_store.go internal/catalog/extended_store_test.go internal/database/extended.go internal/server/server.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/catalog`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-29 Knowledge Base LLM Retrieval Grounding Fix

Implemented/updated this run:

- Fixed the Knowledge Base / Ask Cartolensia retrieval path so concrete
  "find/list/count" questions are answered from verified read-only tool results
  instead of allowing the local LLM to replace the answer with generic schema
  prose.
- Added a PostgreSQL evidence search over safe `cartolensia_search_*` views for
  metadata that regular asset filename/path search cannot cover:
  AI predictions/classes, tags, OCR/document text, transcripts, video frame
  captions, audio features, track summaries, and knowledge facts.
- Added total-match reporting from the evidence query so direct answers can say
  how many indexed media records matched while still returning a bounded first
  page of clickable results.
- Hardened local LLM synthesis validation:
  - schema essays, "potential SQL" explanations, and relationship/capability
    summaries are rejected;
  - media retrieval answers must cite a retrieved asset near the beginning;
  - direct retrieval/count tasks skip free-form synthesis and keep the
    deterministic result list.
- Prevented local LLM tool planning from invoking transcode or segmented-video
  tools unless the user explicitly asks for transcoding/encoding or segment
  merging.

Tests run:

- `gofmt -w internal/server/knowledge.go internal/server/knowledge_llm_test.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`

Remote deployment/validation:

- Built `/tmp/cartolensia-llm-agent` from the verified source tree.
- Deployed it to `/opt/cartolensia/current/bin/cartolensia` on rjazhenka and
  restarted only the `cartolensia` service.
- Remote `https://127.0.0.1:18443/api/v1/health` returned `ok`.
- Authenticated production Knowledge Base query:
  `Please, find and count all photos with trains, made in May 2025.`
  returned HTTP 200 with:
  - `llm_status=local_llm_retrieval_answer`;
  - `media_count=12` returned on the first page;
  - `Found 73 matching media results in the current indexed metadata`;
  - first matches such as `PXL_20250524_190418459.jpg`,
    `PXL_20250524_185921826.jpg`, and other May 2025 photos matched by
    `bullet train` classifier/tag evidence.
- The final answer no longer contains the prior schema/capability essay.
- Remote job status after deployment: `9` running, `0` queued, `26` failed,
  `2625` succeeded. The visible active jobs include `ai_backfill` workers and a
  `hash` worker, so production background processing is still active.

Known limitations:

- Evidence search remains PostgreSQL-local and bounded by the read-only query
  timeout; very broad free-text requests may still need more specialized
  indexed views as the library grows.
- The configured local model `qwen3:8b` is still text-only. Vision-chat support
  is wired in the API/UI, but requires a locally installed vision-capable model.

Safety confirmation:

- no writes to `/mnt/Models/rclone`;
- no writes to Samba/originals;
- no DB reset;
- no missing-file marking;
- no commit;
- no push.

## 2026-06-29 Online Reverse-Geocoder Default

### Implemented

- Changed reverse geocoding from cache-only default to online cache-fill default:
  - `search.geocoder_mode` now defaults to `online_cache`;
  - `search.online_geocoding` now defaults to `true` and can be overridden with `CARTOLENSIA_ONLINE_GEOCODING=false`;
  - direct `/api/v1/places/reverse` requests that omit `online` use the runtime default;
  - new `/api/v1/places/reverse-geocode/start` jobs that omit `online` use the runtime default;
  - callers can still pass `online=false` for an explicit cache-only lookup.
- Kept the provider path cache-first: local `place_cache` matches return immediately and provider calls happen only for cache misses.
- Kept provider calls rate-limited and persisted to `place_cache` for offline reuse.
- Updated Settings/Tasks UI help text so reverse geocoding starts enabled by default.
- Updated operations/security/architecture documentation to reflect the new default and the public-provider caveat.

### Validation

- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
- `npm --prefix webui run build`
- Built `/tmp/cartolensia-online-geocoder` and deployed it plus the rebuilt WebUI to `rjazhenka`.
- Remote `/api/v1/places/providers` now reports `mode=online_cache`, `online_enabled=true`, active provider `nominatim`, URL `https://nominatim.openstreetmap.org`, and minimum interval `1100 ms`.
- A remote authenticated `/api/v1/places/reverse` call with omitted `online` used the default online cache-fill path and stored one Nominatim result in `place_cache`.

### Safety Notes

- No writes to originals or Samba storage.
- No DB reset.
- No missing-file marking.
- No commit or push.
- Broad production enrichment should use imported local geodata or a self-hosted/operator-approved Nominatim/Pelias/Photon endpoint instead of bulk-calling shared public providers.

## 2026-06-29 Reverse-Geocode Missing Semantics

### Implemented

- Renamed the Tasks page action from `Reverse geocode known locations` to
  `Reverse geocode all missing`.
- Renamed the asset detail action from `Refresh place` to
  `Perform reverse geocode`.
- Fixed `/api/v1/places/reverse` so it is now append-only and deduped:
  - local `place_cache` bbox/nearby matches are returned first;
  - if the coordinate does not already have a provider-backed reverse-geocode
    row, Cartolensia calls the configured online provider when enabled;
  - the provider result is upserted into `place_cache` and merged with the
    existing local matches;
  - existing place matches are not removed;
  - repeated clicks do not create duplicate provider rows once the provider
    cache entry exists.
- Fixed the batch `reverse_geocode` worker so `only_missing=true` means
  "missing a provider-backed reverse-geocode cache row", not merely "missing any
  broad local seed match." This lets broad seed places such as country/city
  rows remain visible while still filling detailed OSM/Nominatim-style rows.
- Fixed the asset detail frontend so omitted runtime settings no longer turn
  online reverse geocoding off accidentally.

### Validation

- `gofmt -w internal/server/reverse_geocode.go internal/server/server.go internal/server/server_test.go`
- `git diff --check`
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./internal/server ./cmd/cartolensia`
- `npm --prefix webui run build`
- Built `/tmp/cartolensia-reverse-geocode-missing` and deployed it plus the
  rebuilt WebUI to `rjazhenka`.
- Remote `systemctl is-active cartolensia` returned `active` and
  `https://127.0.0.1:18443/api/v1/health` returned `ok`.
- Remote authenticated `/api/v1/places/providers` still reports
  `mode=online_cache`, `online_enabled=true`, active provider `nominatim`, URL
  `https://nominatim.openstreetmap.org`, and minimum interval `1100 ms`.
- Remote WebUI bundle contains `Reverse geocode all missing` and
  `Perform reverse geocode`.
- Remote authenticated cache-only `/api/v1/places/reverse` test for a Yerevan
  coordinate returned `cached=true`, `source=local_place_cache`, two cached
  places, and the note that no online geocoder was called.

### Safety Notes

- No writes to originals or Samba storage.
- No DB reset.
- No missing-file marking.
- No commit or push.
