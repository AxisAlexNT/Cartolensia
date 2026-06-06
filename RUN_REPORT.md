# Run Report

## 2026-06-06 Phase 1 Hardening Run

### Implemented Features

- Embedded SQL migrations from `migrations/*.sql`; runtime no longer depends on a relative `migrations/` directory unless `database.migrations_dir` or `CARTOLENSIA_MIGRATIONS_DIR` is explicitly set.
- Forward migrations:
  - `002_phase1_hardening.sql`: `app_settings`, `jobs.next_run_at`, queued/lease indexes.
  - `003_auth_foundation.sql`: `users`, `sessions`, `api_tokens`.
- PostgreSQL and memory-store job leasing:
  - enqueue;
  - lease next job;
  - heartbeat;
  - owner-only progress/log updates;
  - owner-only complete/fail/cancel;
  - cancellation request;
  - expired lease release and retry scheduling.
- Async worker manager with configurable worker ID, poll interval, lease duration, heartbeat interval, max concurrency, graceful stop, and panic recovery.
- Discovery and SHA-512 hashing now run through queued jobs in the app runtime and check cancellation between files.
- `POST /api/v1/jobs/{id}/cancel`.
- Folder-style Explorer API via `/api/v1/explorer?view=folders&path=...`, preserving the previous flat `/api/v1/explorer` response.
- `GET /api/v1/assets/{id}` asset detail API and Vue asset detail view.
- Vue Explorer breadcrumbs and folder/file distinction.
- Local auth foundation:
  - `Principal`, `Session`, `Authenticator`, `Authorizer`;
  - explicit `dev_no_auth` default;
  - write-like endpoints pass through the auth hook.
- Preview cache/status foundation:
  - cache keys and paths derive from asset/content IDs;
  - cache path stays under Cartolensia cache directory;
  - preview endpoint returns clean `not_implemented`/`unsupported` statuses.
- Gated PostgreSQL integration test:
  - `CARTOLENSIA_RUN_DB_TESTS=1`;
  - `CARTOLENSIA_TEST_DATABASE_URL=...`;
  - isolated schema per run;
  - no user database drops.
- Added `scripts/test-db.sh`.
- Strengthened tests for migration loading, job states, memory lease races, cancellation, worker panic recovery, storage traversal/symlink/read-only behavior, Explorer grouping, and asset detail.

### Commands Run

- Inspected docs and code with `sed`, `rg --files`, and `git status --short --untracked-files=all`.
- Formatted Go files with `gofmt -w ...`.
- `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...` passed.
- `go test ./...` passed.
- `npm --prefix webui run build` passed.
- `bash scripts/smoke-test.sh` passed.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml config` passed.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres` started PostgreSQL/PostGIS.
- `bash scripts/test-db.sh` passed after rerunning outside the sandbox.
- `go run ./cmd/cartolensia -config config/dev-postgres.yaml` started the app with the PostgreSQL store.
- `curl -fsS` API checks against `http://127.0.0.1:18080` verified health, stats, and jobs.
- `pkill -TERM -f "cartolensia -config config/dev-postgres.yaml"` stopped the DB-backed app.
- `docker compose -f docker-compose.yml -f docker-compose.dev.yml stop postgres` stopped the PostgreSQL container while preserving the named volume.

### Failures And Fixes

- `bash scripts/test-db.sh` first failed in the sandbox with `socket: operation not permitted` for `127.0.0.1:55432`. Reran with escalation.
- `scripts/test-db.sh` initially used the wrong default password. Fixed it to `cartolensia_dev_password`, matching Docker Compose.
- A sandboxed Docker health-poll loop failed on `/var/run/docker.sock`. Reran Docker status with escalation.
- A new encoded traversal test exposed that `%2e%2e` could normalize away after URL unescape. Fixed `NormalizeRelativePath` to reject any `..` segment before cleaning.

### Tests Passed

- Unit/integration-without-DB:
  - `GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...`
  - `go test ./...`
- Frontend:
  - `npm --prefix webui run build`
- Local app smoke:
  - `bash scripts/smoke-test.sh`
- Docker/DB:
  - `docker compose -f docker-compose.yml -f docker-compose.dev.yml config`
  - `bash scripts/test-db.sh`
  - DB-backed app smoke on `127.0.0.1:18080`

### Known Limitations

- Preview generation is still not implemented; the API reports `not_implemented` or `unsupported` and does not create preview files.
- Auth persistence/bootstrap tables exist and interfaces are wired, but local login/session/token flows are not implemented.
- Retry classification is basic: failed leased jobs retry until `max_attempts`; permanent error classification is still coarse.
- DB integration coverage is useful but still compact; it does not yet test multi-process workers under real concurrency.
- GPX parsing, track APIs, map APIs, and transcoding capability APIs remain future work.
- Plugin execution remains manifest/built-in stub only; sidecar HTTP/gRPC runtime is not implemented.
- `webui/dist` was generated for verification and is ignored by git.
- Docker state was used for PostgreSQL testing; the container was stopped and the named volume was preserved.

### Changed Files Summary

- Backend: app wiring, config, catalog store, database migrations/repository, discovery runner, job model, server routes, storage safety.
- New backend packages: `internal/auth`, `internal/preview`, `internal/workers`.
- Tests: catalog, database migration/integration, jobs, server, storage, worker.
- Frontend: API types/client, Explorer folder UI, asset detail UI, job cancellation button, styles.
- Operations: embedded migrations, new forward SQL migrations, smoke/test-db scripts, env/config samples, gitignore.
- Docs: architecture, DB schema, storage model, plugin model, roadmap, hardening plan, README.

### `/mnt/Models/rclone`

Skipped entirely. It was not read, written, listed, scanned, mounted, or required for tests.

### Exact Recommended Next Prompt

```text
Continue from the current Cartolensia repo. Read AGENTS.md, README.md, RUN_REPORT.md, docs/ARCHITECTURE.md, docs/DB_SCHEMA.md, docs/STORAGE_MODEL.md, docs/PLUGIN_MODEL.md, docs/ROADMAP.md, and docs/PHASE_1_HARDENING_PLAN.md. Build the next Phase 1 slice: local auth bootstrap/login/session/token persistence, richer job retry/error classification, preview generation worker design, GPX parser MVP with synthetic fixtures, track listing API, and map API skeleton. Keep using testdata/media_fixture only. Do not touch /mnt/Models/rclone. Run GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./..., go test ./..., npm --prefix webui run build, bash scripts/smoke-test.sh, docker compose config, and DB integration tests if Docker/PostgreSQL is available. Update RUN_REPORT.md honestly and do not push.
```
