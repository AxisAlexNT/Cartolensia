# Run Report

## Implemented Features

- Real Go backend bootstrap with config loading, storage registry, plugin loading, database selection, REST API, and static WebUI serving.
- YAML config with defaults, environment overrides, validation, and absolute storage/cache paths.
- PostgreSQL support through `pgx`, SQL migrations, config snapshots, storage/plugin upserts, assets, locations, contents, jobs, job logs, stats, and optional capability detection.
- Development PostgreSQL/PostGIS Docker Compose service on host port `55432` to avoid conflicts with local PostgreSQL.
- Built-in plugin manifests with dependency sorting for `albums`, `mapview`, `gpstracks`, `transcoding`, `ai-base`, and `ai-classification`.
- Strict read-only filesystem storage adapter with universal `fs://` URLs, path traversal protection, symlink-skip discovery, read/list/stat/open behavior, and explicit read-only errors for modifying operations.
- Fast discovery scan over generated fixture media: URL, relative path, name, extension, MIME, size, mtime, media kind, and `unhashed` status.
- Lazy SHA-512 hashing job that streams files through the storage adapter.
- Job model with state transitions, progress, counters, and logs.
- REST API under `/api/v1`, including health, version, effective config, storages, plugins, plugin rescan, jobs, discovery start, hash start, assets, explorer, stats, backend status, original streaming, and preview `501` response.
- Original endpoint uses safe read-only storage access and supports HTTP Range via `http.ServeContent`.
- Vue 3 + TypeScript WebUI with shell/navigation, Explorer, Discovery, Storages, Plugins, Stats, and stub pages for Albums, Map, GPS Tracks, Transcoding, Base AI, and AI Classification.
- Dockerfile, Compose app service, dev DB scripts, reset script, smoke test script, and guarded read-only rclone scan script.
- GPS/video sync schema skeleton: `track_points`, `asset_track_links`, overlap fields, candidate status, and manual `time_offset_ms`.
- Documentation updates: `docs/ARCHITECTURE.md`, `docs/PRODUCT_VISION.md`, storage model, plugin model, DB schema, roadmap, and README development commands.

## Tools Detected

| Tool | Status | Version output |
| --- | --- | --- |
| go | found | `go version go1.22.2 linux/amd64` |
| node | found | `v24.15.0` |
| npm | found | `11.15.0` |
| pnpm | missing | `pnpm: command not found` |
| docker | found | `Docker version 29.2.1, build a5c7197` |
| docker compose | found | `Docker Compose version v5.0.2` |
| docker-compose | missing | `docker-compose: command not found` |
| psql | found | `psql (PostgreSQL) 16.11 (Ubuntu 16.11-0ubuntu0.24.04.1)` |
| ffmpeg | found | `ffmpeg version 6.1.1-3ubuntu5` |
| ffprobe | found | `ffprobe version 6.1.1-3ubuntu5` |

## Commands Run

- Read project docs and prompt files with `sed`.
- Inspected files and status with `rg --files`, `find`, `git status --short`.
- Dependency/setup commands:
  - `go get github.com/jackc/pgx/v5 ...`
  - `go get github.com/jackc/pgx/v5@v5.5.5`
  - `go get github.com/jackc/pgx/v5/pgconn@v5.5.5 github.com/jackc/pgx/v5/pgxpool@v5.5.5`
  - `go get github.com/rogpeppe/go-internal@v1.13.1`
  - `GOTOOLCHAIN=local go mod tidy`
  - `npm --prefix webui install`
- Formatting and permissions:
  - `gofmt -w ...`
  - `chmod +x scripts/*.sh`
- Verification:
  - `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
  - `npm --prefix webui run build`
  - `bash scripts/smoke-test.sh`
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres`
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml ps postgres`
  - `go run ./cmd/cartolensia -config config/dev-postgres.yaml`
  - `curl -fsS ...` against local API endpoints
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml down`
- Process cleanup:
  - targeted `pkill -TERM -f 'cartolensia -config config/dev-postgres.yaml'`

## Tests Passed

- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`: passed.
- `npm --prefix webui run build`: passed.
- `bash scripts/smoke-test.sh`: passed after running outside the sandbox so the backend could bind to `127.0.0.1:18080`.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`: passed.
- Docker PostgreSQL/PostGIS smoke:
  - PostgreSQL container started healthy on `55432`.
  - Backend started with `postgres` store.
  - Migrations applied.
  - Config snapshot/storage/plugin rows were written.
  - `/api/v1/backend/status` reported PostGIS installed, pgvector absent, pg_trgm available but not installed.
  - `/api/v1/discovery/start` indexed 4 fixture media/track files.
  - `/api/v1/hash/start` hashed 4 files.
  - `/api/v1/stats` returned 4 assets, 4 locations, 2 photos, 1 video, 1 track, 4 hashed, 908 bytes.
  - `/api/v1/media/{asset_id}/original` returned Range bytes from a dummy fixture file.

## Failures And Fixes

- `pgx v5.10.0` required a newer Go toolchain. Fixed by pinning `pgx v5.5.5`.
- `go mod tidy` initially selected `github.com/rogpeppe/go-internal v1.15.0`, which requires Go 1.25. Fixed by pinning `github.com/rogpeppe/go-internal v1.13.1` and running `GOTOOLCHAIN=local go mod tidy`.
- Docker Compose initially failed to bind PostgreSQL to host port `5432` because that port was already in use. Fixed by changing the dev default to `55432`.
- Sandboxed smoke test failed to bind `127.0.0.1:18080`. Fixed by running the approved smoke script outside the sandbox.
- Discovery initially indexed fixture README/manifest helper files. Fixed by indexing only media/track kinds and reporting progress over indexable files.
- Storage API initially emitted Go field names. Fixed by adding JSON tags.
- A SHA-512 test expected value was incorrect. Fixed the test vector to the actual SHA-512 of `cartolensia`.

## Known Limitations

- The exact file `CARTOLENSIA_FIRST_15_MIN_PROMPT.md` was absent; `.agents/FirstNightPrompt.md` contained the same prompt content and was used as the available local instruction source.
- Jobs run synchronously inside API requests; durable worker leasing tables exist, but a background worker loop is not implemented yet.
- PostgreSQL integration is smoke-tested through Docker, but there are no isolated automated DB unit tests yet.
- Migrations are loaded from the filesystem instead of embedded with Go `embed`.
- Authentication is not implemented.
- Preview generation returns a clean `501 Not Implemented` response; no preview cache files are created.
- Plugin execution is manifest/stub only. Sidecar HTTP/gRPC runtime is designed but not implemented.
- Albums, maps, GPS editing, transcoding, AI base, and AI classification are WebUI/API stubs.
- No real media is included or required; tests use generated dummy fixture files only.
- `webui/node_modules` and `webui/dist` were created locally for verification and are ignored by git.
- The Docker image pull created local Docker image/volume state outside the repo; Compose services were stopped with `docker compose down`.

## `/mnt/Models/rclone`

Skipped entirely. The path was not read, written, listed, scanned, mounted, or required for tests.

The added `scripts/scan-rclone-readonly.sh` refuses to scan by default and requires `CARTOLENSIA_ALLOW_RCLONE_SCAN=1`. It is read-only and only lists files if explicitly enabled later.

## Next Recommended Prompt

```text
Continue from the current Cartolensia MVP. Read AGENTS.md, README.md, docs/ARCHITECTURE.md, docs/IMPLEMENTATION_PLAN.md, docs/DB_SCHEMA.md, docs/STORAGE_MODEL.md, docs/PLUGIN_MODEL.md, docs/ROADMAP.md, and RUN_REPORT.md. Harden Phase 1: add asynchronous PostgreSQL-backed job workers with leases/heartbeats, add DB integration tests that can run against Docker PostgreSQL, embed migrations with Go embed, add an auth interface stub, and improve Explorer folder grouping. Keep using testdata/media_fixture only. Do not touch /mnt/Models/rclone. Run go test ./..., npm --prefix webui run build, bash scripts/smoke-test.sh, and a DB-backed smoke test. Update RUN_REPORT.md honestly.
```
