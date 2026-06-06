# Phase 1 Hardening Plan

This plan assumes the current MVP is a working vertical slice: fixture discovery, SHA-512 hashing, REST API, PostgreSQL-capable storage, and a Vue WebUI. The next phase should harden behavior without expanding into real archive indexing or heavy product features.

Do not touch `/mnt/Models/rclone`. Continue using `testdata/media_fixture/` and synthetic temporary files for tests.

Status after the Phase 1 hardening run: sections 1 through 5 are implemented for the MVP path, section 6 has a gated PostgreSQL integration test, sections 7 and 9 are implemented, section 8 is implemented as interfaces/dev-mode foundation, and section 10 is implemented as a status/cache-path foundation without preview generation.

## Audit Summary

- `RUN_REPORT.md` matches the current codebase at a high level.
- The current job path is synchronous inside HTTP handlers.
- The PostgreSQL schema already has worker-oriented fields: `worker_id`, `lease_expires_at`, `cancel_requested_at`, attempts, counters, timestamps, and job logs.
- The `catalog.Store` interface does not yet expose lease, heartbeat, cancellation, retry, or dequeue operations.
- Migrations are loaded from `migrations/` at runtime; they are not embedded.
- PostgreSQL was smoke-tested previously, but there are no automated DB integration tests.
- `docs/DB_SCHEMA.md` mentions `app_settings`, but `migrations/001_core.sql` does not create it. Treat this as a hardening task and add a forward migration if the table is still needed. Do not edit an already-applied migration casually because checksums are tracked.
- The Explorer API is a flat list. It does not yet expose folder grouping or asset detail records.
- Auth is absent.
- Preview generation returns a clean `501` response; no preview cache exists yet.

## Implementation Order

1. Embedded migrations and migration safety.
2. Job repository contract for leases, heartbeats, cancellation, and retries.
3. Worker loop and HTTP job enqueue behavior.
4. DB integration test harness.
5. Explorer folder grouping and asset detail API/UI.
6. Local admin/auth bootstrap.
7. Media preview cache design and minimal API shape.

This order keeps the data foundation stable before adding UI and user/session semantics.

## 1. Embedded Migrations

Tasks:

- Add a package-level `embed.FS` for `migrations/*.sql`.
- Replace runtime-only filesystem migration loading with an embedded default loader.
- Keep a development/testing helper that can load migrations from disk when explicitly requested.
- Add a new migration instead of editing `001_core.sql` if schema needs to change.
- Resolve the `app_settings` mismatch by either:
  - adding `002_app_settings.sql`, or
  - removing `app_settings` from docs if it is not part of Phase 1.
- Add tests for migration sorting, checksum stability, already-applied migrations, and checksum mismatch detection.

Acceptance checks:

- Backend starts from a copied binary/workdir that does not contain a `migrations/` directory.
- `go test ./...` still passes.
- Existing DBs with `001_core` applied do not break.

## 2. Async PostgreSQL-Backed Workers

Tasks:

- Split job enqueue from job execution in HTTP handlers.
- Add a worker package, likely `internal/workers`.
- Define job handlers by job kind:
  - `discovery`
  - `hash`
- Add a worker loop with configurable poll interval, lease duration, heartbeat interval, and max concurrency.
- Start workers from app bootstrap when configured.
- Keep a synchronous mode only for tests or explicit development runs.
- Ensure memory-store mode still supports smoke tests without PostgreSQL.

Acceptance checks:

- `POST /api/v1/discovery/start` returns quickly with a queued/running job.
- `/api/v1/jobs` shows progress while the worker executes.
- Worker restart can resume queued jobs and expired leases.

## 3. Job Leases And Heartbeats

Tasks:

- Extend `catalog.Store` or create a `jobs.Repository` with:
  - `LeaseNextJob(ctx, workerID, kinds, leaseUntil)`
  - `HeartbeatJob(ctx, jobID, workerID, leaseUntil)`
  - `CompleteJob(ctx, jobID, workerID, counters)`
  - `FailJob(ctx, jobID, workerID, error)`
  - `ReleaseExpiredLeases(ctx, now)`
- Use `select ... for update skip locked` in PostgreSQL.
- Only the owning worker may heartbeat, complete, fail, or update progress for a leased job.
- Persist worker IDs in `worker_id`.
- Store `lease_expires_at` consistently in UTC.

Acceptance checks:

- Two workers cannot lease the same job.
- A job with an expired lease can be leased again.
- A stale worker cannot complete a job after another worker acquired it.

## 4. Cancellation

Tasks:

- Add endpoint: `POST /api/v1/jobs/{id}/cancel`.
- Persist cancellation by setting `cancel_requested_at`.
- Make long-running scan/hash loops check cancellation between files and before expensive operations.
- Define final state:
  - queued job cancellation should become `canceled`;
  - running job cancellation should become `canceled` after cooperative stop;
  - completed jobs should not be cancelable.
- Add cancellation logs.

Acceptance checks:

- Canceling queued jobs prevents execution.
- Canceling running discovery/hash stops before all files when tested with a synthetic larger fixture.
- Cancellation is idempotent.

## 5. Retry And Error Model

Tasks:

- Define error classes:
  - permanent input/config/storage safety errors;
  - transient filesystem/database/process errors;
  - canceled jobs;
  - panic recovery errors.
- Add `attempts`, `max_attempts`, and retry scheduling semantics.
- Consider adding `next_run_at timestamptz` in a new migration.
- Preserve last error in `jobs.error`.
- Store structured error metadata in `payload_json` or a future `job_errors` table if needed.
- Add panic recovery around job handlers.

Acceptance checks:

- Transient job failures retry until `max_attempts`.
- Permanent failures do not spin.
- Panics become failed jobs with logs, not crashed workers.

## 6. DB Integration Tests

Tasks:

- Add an integration test mode gated by environment variables, for example:
  - `CARTOLENSIA_TEST_DATABASE_URL`
  - `CARTOLENSIA_RUN_DB_TESTS=1`
- Provide `scripts/test-db.sh` or extend `scripts/dev-db.sh` with a test database option.
- Tests should create isolated schemas or unique databases per run.
- Cover:
  - migration idempotence;
  - config snapshot insert;
  - storage/plugin upsert;
  - discovery upsert idempotence;
  - SHA-512 content linking;
  - job leasing and heartbeat race behavior;
  - cancellation and retry transitions.
- Keep normal `go test ./...` passing without Docker.

Acceptance checks:

- Unit tests pass without Docker.
- DB integration tests pass when Docker PostgreSQL is available.
- Test cleanup does not drop user databases.

## 7. Explorer Folder Grouping

Tasks:

- Add backend model for explorer folders:
  - current path;
  - parent path;
  - child folders;
  - files at current path;
  - counts and total bytes.
- Add query parameters:
  - `storage`
  - `path`
  - optional media kind filter.
- Implement grouping from `asset_locations.relative_path`.
- Add indexes if needed in a forward migration.
- Update WebUI Explorer with folder navigation and breadcrumbs.

Acceptance checks:

- Root explorer shows `photos`, `tracks`, and `videos` for the fixture.
- Navigating into `photos/2024-trip` shows the two photo fixtures.
- Existing flat `/api/v1/explorer` behavior remains available or is versioned cleanly.

## 8. Local Admin/Auth Bootstrap

Tasks:

- Add auth interfaces without building public sharing yet:
  - `Principal`
  - `Session`
  - `Authenticator`
  - `Authorizer`
- Add local admin bootstrap config:
  - disabled by default in memory mode or uses explicit dev token;
  - password/token secret never committed;
  - `.env.example` documents variables.
- Add tables in a new migration if implementing persistence:
  - `users`
  - `sessions`
  - `api_tokens`
- Add middleware that can run in `dev_no_auth` mode for the fixture.
- Protect write-like endpoints such as discovery start, hash start, plugin rescan, and future cancel.

Acceptance checks:

- Development fixture workflow remains easy.
- No default production admin password is hardcoded.
- Auth-disabled mode is explicit in backend status.

## 9. Asset Detail Page

Tasks:

- Add `GET /api/v1/assets/{id}`.
- Return:
  - logical asset metadata;
  - all storage locations;
  - content/hash state;
  - basic job/history references when available;
  - preview/original URLs.
- Add WebUI asset detail route/state.
- Link Explorer rows to asset detail instead of directly to original only.

Acceptance checks:

- Detail page opens for all four fixture assets.
- Original link still streams the file.
- Missing asset returns 404 with a JSON error.

## 10. Media Preview Cache Design

Tasks:

- Document cache root policy:
  - always inside Cartolensia cache/work dir;
  - never under original storage roots;
  - safe path derivation from asset/content IDs;
  - bounded cleanup policy.
- Define preview API statuses:
  - ready;
  - queued;
  - unsupported;
  - failed;
  - not implemented.
- Add tables or columns if previews become persistent jobs:
  - `preview_cache_entries`
  - `preview_jobs`, or reuse `jobs`.
- For the first implementation, generate only trivial safe previews if feasible:
  - text placeholder for unsupported fixture dummy files;
  - no real image/video transformation unless dependency behavior is clear.

Acceptance checks:

- Preview endpoint never writes into storage roots.
- Preview cache path is stable and traversal-safe.
- Unsupported media returns a UI-friendly response.

## Verification Commands

Baseline commands for the hardening run:

```bash
go test ./...
npm --prefix webui run build
bash scripts/smoke-test.sh
docker compose -f docker-compose.yml -f docker-compose.dev.yml config
```

DB-backed checks when Docker is available:

```bash
make dev-db
go run ./cmd/cartolensia -config config/dev-postgres.yaml
```

Then use local API smoke requests for health, discovery, hash, jobs, stats, explorer, and original streaming.

## Non-Goals For This Phase

- Real `/mnt/Models/rclone` scanning.
- SMB, NFS, S3, upload, delete, move, or trash operations.
- Real AI inference, embeddings, or classifier training.
- Real transcoding.
- Public sharing.
- Full map rendering.
