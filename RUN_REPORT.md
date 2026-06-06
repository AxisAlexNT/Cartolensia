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
