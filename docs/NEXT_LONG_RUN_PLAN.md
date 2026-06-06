# Next Long Run Plan

This is the implementation target for the next unattended run. It must keep using `testdata/media_fixture/`, ignored synthetic fixtures, and `/tmp` fixtures only. It must not touch `/mnt/Models/rclone`.

The goal is to make Cartolensia ready for a later tiny supervised real-archive dry run, not to run that dry run.

## Status After 2026-06-06 Implementation Run

Implemented from this plan:

- Albums plugin MVP with virtual albums and album item membership.
- Photo/map plugin MVP with OpenLayers, GeoJSON APIs, typed geotags, album/media/track filters, and deterministic grid clustering.
- GPS track manager MVP with summaries, point queries, media lookup, offset controls, and snap-media job.
- Server-side JPEG EXIF parsing through `github.com/rwcarlsen/goexif` with compatible BSD-style license.
- Forward migration for albums, `asset_geo`, `gps_tracks`, `scan_runs`, `preview_cache_entries`, and `plugin_settings`.
- Scoped discovery dry-run readiness with report-only API, scan-run records, guarded config example, and guarded preflight script.
- Persistent preview cache index and cleanup/status APIs.
- Worker stress script and additional unit tests around albums, geotag priority, dry-run reports, preview cache, and track snapping.

Remaining next useful work:

- Scoped rescan and missing-file marking semantics.
- Deeper PostgreSQL integration tests for albums/geotags/dry-run/preview cache under large synthetic datasets.
- Better real benchmark path that runs discovery against a synthetic temp storage instead of only generating fixtures.
- Real map tile strategy and offline map packaging.
- Video poster previews and richer typed metadata tables.

## 1. Albums Plugin MVP

Files/packages likely to change:

- `migrations/006_albums_and_dryrun.sql`
- `internal/catalog`
- `internal/database`
- `internal/server`
- `webui/src/api.ts`
- `webui/src/App.vue`
- `webui/src/style.css`

APIs to add/change:

- `GET /api/v1/albums`
- `POST /api/v1/albums`
- `GET /api/v1/albums/{id}`
- `PATCH /api/v1/albums/{id}`
- `DELETE /api/v1/albums/{id}` as a metadata-only delete
- `POST /api/v1/albums/{id}/items`
- `DELETE /api/v1/albums/{id}/items/{asset_id}`

Migrations to add:

- `albums(id, parent_id, name, description, sort_order, created_at, updated_at)`
- `album_items(album_id, asset_id, item_kind, sort_order, created_at)`
- indexes on `albums(parent_id)`, `album_items(album_id)`, and `album_items(asset_id)`

Frontend pages/components:

- Albums page with album tree/list.
- Album detail page with asset rows.
- Add selected Explorer assets to album.
- Clear warning that albums do not move or modify original files.

Tests to add:

- Album CRUD in memory and PostgreSQL stores.
- Nested albums.
- Same asset in multiple albums.
- Metadata-only delete does not affect assets/locations/originals.
- Auth protects write endpoints.

Acceptance criteria:

- Fixture assets can be grouped into albums without changing storage paths.
- Albums can be nested and listed efficiently.
- Existing Explorer and asset detail links still work.
- No storage write/delete/move calls are introduced.

## 2. Photo/Map Plugin MVP

Files/packages likely to change:

- `internal/catalog`
- `internal/database`
- `internal/server`
- `internal/metadata`
- `webui/src/api.ts`
- `webui/src/App.vue`
- `webui/src/style.css`
- optionally `webui/package.json` and `webui/package-lock.json` if OpenLayers is approved

APIs to add/change:

- Extend `GET /api/v1/map` with stable cursor pagination.
- Add `GET /api/v1/map/assets`.
- Add `GET /api/v1/map/tracks`.
- Add `GET /api/v1/map/status`.
- Add bbox/time/media-kind filters consistently to assets and tracks.

Migrations to add:

- Optional typed geotag columns/table only if needed:
  - `asset_geo(asset_id, lat, lon, source, confidence, taken_at, created_at, updated_at)`
  - indexes on `(lat, lon)`, `taken_at`, and `source`
- If kept in `metadata_json` for this run, add no migration and document deferral.

Frontend pages/components:

- Replace or extend the current SVG map with OpenLayers if approved.
- If OpenLayers is not approved, keep SVG/GeoJSON and add better filters, selection, and popovers.
- Map filters: media kind, time range, selected track IDs, show clusters/raw.
- Asset/track click opens detail.
- Visible "no real tile base map yet" state unless tiles are configured.

Tests to add:

- bbox filtering for assets and tracks.
- deterministic cluster output.
- cursor/limit behavior.
- time filter behavior.
- map API does not require PostGIS.

Acceptance criteria:

- Synthetic geotagged fixtures render on the map.
- Large synthetic responses are bounded by limit/cursor.
- Map UI remains usable without network tiles.
- OpenLayers is used only if approved.

## 3. GPS Track Manager Plugin MVP

Files/packages likely to change:

- `internal/gpx`
- `internal/catalog`
- `internal/database`
- `internal/server`
- `webui/src/api.ts`
- `webui/src/App.vue`
- `webui/src/style.css`

APIs to add/change:

- `GET /api/v1/tracks` with limit/offset/cursor, time filters, bbox filters, and text search.
- `GET /api/v1/tracks/{track_asset_id}` with simplified geometry option.
- `GET /api/v1/tracks/{track_asset_id}/assets` for assets overlapping the track time span.
- Extend sync APIs for offset testing and saved link inspection.

Migrations to add:

- Optional `track_summaries` table/cache if DB query cost becomes high:
  - `track_asset_id`, point count, time range, bbox, distance, duration, elevation min/max.
- Otherwise compute from `track_points` and metadata for this run.

Frontend pages/components:

- Track list filters and pagination.
- Track detail page with metadata and simplified geometry.
- "Assets near/along this track" section based on time overlap.
- Manual time offset controls for sync testing.

Tests to add:

- multi-track and multi-segment GPX.
- invalid GPX handling.
- no-time GPX behavior.
- track detail simplification.
- track-to-asset overlap with +20 minute and +97 minute offsets.

Acceptance criteria:

- Tracks page is useful with synthetic fixtures.
- Track detail does not load unbounded point arrays by default.
- Time offset workflows are testable without real video playback.

## 4. Scoped Dry-Run Readiness

Files/packages likely to change:

- `internal/config`
- `internal/storage`
- `internal/discovery`
- `internal/catalog`
- `internal/database`
- `internal/server`
- `webui/src/api.ts`
- `webui/src/App.vue`
- `docs/REAL_ARCHIVE_DRY_RUN.md`

Guard design:

- Real archive storage config name must be explicit, e.g. `rclone_dryrun`.
- Storage mode must be `strict_read_only`; reject any other mode.
- Require explicit path/prefix allowlist per dry-run request.
- Default `max_files` must be `<= 50`.
- Add a default `max_bytes`, initially conservative such as `2 GiB`.
- No hashing unless separately requested.
- No missing-file marking.
- No preview generation unless separately requested.
- Visible warning in UI for dry-run jobs.
- Job cancellation must be one click from the dry-run report page.
- Produce a dry-run report/log with scope, counters, skipped files, first errors, and exact safety settings.

APIs to add/change:

- `POST /api/v1/discovery/dry-run`
- `GET /api/v1/discovery/dry-run/{job_id}/report`
- Extend discovery payload with `storage`, `prefixes`, `max_files`, `max_bytes`, `hash`, `metadata`, `previews`, `mark_missing`.

Migrations to add:

- `scan_runs(id, job_id, storage_name, mode, prefixes_json, max_files, max_bytes, hash_requested, metadata_requested, previews_requested, mark_missing, started_at, finished_at, report_json)`
- index on `scan_runs(job_id)`

Frontend pages/components:

- Dry Run page or Discovery section.
- Storage scope form with storage name, prefixes, max files, max bytes.
- Persistent warning banner.
- Dry-run report view.
- Cancel button linked to job cancellation.

Tests to add:

- Reject non-allowlisted prefixes.
- Reject dry run with `max_files > 50` unless explicit future admin override exists.
- Reject `mark_missing=true`.
- Reject preview/hash unless explicitly requested.
- Verify no writes occur under fixture storage roots.
- Verify report content for synthetic fixture.

Acceptance criteria:

- A future supervised `/mnt/Models/rclone` dry run can be configured without code changes.
- The implementation cannot accidentally scan the whole archive from a default request.
- Hashing, previews, and missing marking are opt-in and default off.

## 5. DB-Backed Pagination/Query Hardening

Files/packages likely to change:

- `internal/catalog`
- `internal/database`
- `internal/server`
- `internal/database/integration_test.go`
- `webui/src/api.ts`
- `webui/src/App.vue`

APIs to add/change:

- Introduce query option structs for assets, locations, explorer, jobs, tracks, albums.
- Preserve current response shapes where possible.
- Add `limit`, `cursor` or `offset`, `sort`, `q`, `media_kind`, `hash_status`, `storage`, `extension`, `taken_from`, `taken_to`.
- Return total counts only when cheap or explicitly requested.

Migrations to add:

- Add indexes if query plans require them:
  - `asset_locations(storage_id, media_kind, relative_path)`
  - `asset_locations(extension)`
  - `asset_locations(hash_status)`
  - `assets(display_name)` basic btree only unless pg_trgm is explicitly enabled later
  - `jobs(status, kind, created_at desc)`

Frontend pages/components:

- Explorer filters.
- Jobs filters.
- Track filters.
- Preserve selection across pagination.

Tests to add:

- PostgreSQL asset query filters.
- Explorer folder query does not load all assets in DB mode.
- Stable sort and pagination.
- Search over name/path.
- Normal memory-mode behavior remains compatible for fixture tests.

Acceptance criteria:

- PostgreSQL mode no longer relies on loading all assets for common list endpoints.
- UI works on synthetic 1,000+ file fixture without unbounded API payloads.

## 6. Worker Stress Tests

Files/packages likely to change:

- `internal/workers`
- `internal/jobs`
- `internal/database/integration_test.go`
- `scripts/test-db.sh`
- optional `scripts/worker-stress-smoke.sh`

APIs to add/change:

- No public API required unless missing stats are needed.
- Add backend status fields for active workers if useful.

Migrations to add:

- None expected unless job error classification needs a typed column.

Frontend pages/components:

- Jobs page should show active worker IDs and last errors from existing stats.

Tests to add:

- Multiple DB workers lease distinct jobs.
- Expired lease can be acquired by another worker.
- Stale worker cannot complete after lease loss.
- Heartbeat extends leases.
- Cancellation remains idempotent under concurrency.
- Panic handler fails a job without crashing worker manager.

Acceptance criteria:

- DB-backed stress tests pass under `CARTOLENSIA_RUN_DB_TESTS=1`.
- Normal `go test ./...` still passes without Docker.

## 7. Metadata/Preview/Cache Hardening

Files/packages likely to change:

- `internal/metadata`
- `internal/preview`
- `internal/media`
- `internal/catalog`
- `internal/database`
- `internal/server`
- `webui/src/api.ts`
- `webui/src/App.vue`

APIs to add/change:

- `GET /api/v1/previews/status`
- `POST /api/v1/previews/cleanup`
- `GET /api/v1/metadata/status`
- Extend asset detail with typed metadata summary.

Migrations to add:

- `preview_cache_entries(asset_id, content_id, variant, width, height, format, cache_path, status, size_bytes, created_at, last_accessed_at, error)`
- indexes on `(asset_id)`, `(content_id)`, `(status)`, `(last_accessed_at)`
- Optional `asset_metadata_facts` only if typed metadata outgrows `metadata_json`.

Frontend pages/components:

- Preview cache status card.
- Preview cleanup button, auth-protected.
- Asset detail typed metadata panel.
- Unsupported preview status remains visible and non-fatal.

Tests to add:

- Cache index insert/update on generation.
- Cache hit updates access time.
- Cleanup never deletes outside cache root.
- Unsupported formats do not fail jobs.
- Metadata enrichment cancellation.

Acceptance criteria:

- Preview cache is inspectable and cleanup is safe.
- Metadata and preview jobs remain safe on dummy files.

## 8. UI Integration

Files/packages likely to change:

- `webui/src/App.vue`
- `webui/src/api.ts`
- `webui/src/style.css`
- optionally split into `webui/src/components/*` if the single-file UI becomes too large

APIs to add/change:

- Use APIs listed in sections above.

Migrations to add:

- None directly; UI follows backend migrations.

Frontend pages/components:

- Albums page and album detail.
- Map page with approved OpenLayers integration or improved SVG fallback.
- GPS track manager detail view.
- Discovery dry-run page/report.
- Explorer filters/pagination/selection.
- Jobs filters and active worker state.
- Preview/metadata cache/status controls.

Tests to add:

- `npm --prefix webui run build`.
- If a JS test framework is introduced later, add component tests; do not add one in this run unless justified.

Acceptance criteria:

- WebUI remains buildable without CDN resources.
- Main workflows are visible from navigation.
- Real-archive dry-run controls make unsafe defaults hard to choose.

## 9. Test And Verification Plan

Required commands before finishing the long run:

```bash
gofmt -w <changed Go files>
git diff --check
GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...
go test ./...
npm --prefix webui run build
bash scripts/smoke-test.sh
docker compose -f docker-compose.yml -f docker-compose.dev.yml config
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres
bash scripts/test-db.sh
docker compose -f docker-compose.yml -f docker-compose.dev.yml stop postgres
```

Additional long-run checks:

```bash
CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/generate-synthetic-fixture.sh
CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/perf-smoke.sh
```

Rules:

- Do not reset a database unless explicitly approved.
- Prefer isolated DB schemas/test data for integration tests.
- Do not drop user databases.
- Do not touch `/mnt/Models/rclone`.
- Do not claim success unless the command actually passed.

Acceptance criteria:

- All feasible tests pass.
- Any environment-only failure is documented.
- `RUN_REPORT.md` is updated with implemented features, commands, failures, limitations, and whether `/mnt/Models/rclone` was touched.
