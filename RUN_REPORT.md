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
