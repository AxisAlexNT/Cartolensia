# Operations

This runbook is for local development and fixture-only validation. It does not require any real media archive.

## Quick Start

Run the fixture smoke path:

```bash
make smoke
```

Run the backend in memory mode:

```bash
go run ./cmd/cartolensia
```

Run the WebUI build:

```bash
npm --prefix webui run build
```

## PostgreSQL Development

Start the development PostgreSQL/PostGIS container:

```bash
make dev-db
```

Run the app against PostgreSQL:

```bash
go run ./cmd/cartolensia -config config/dev-postgres.yaml
```

Run gated DB integration tests:

```bash
bash scripts/test-db.sh
```

Reset the dev DB only when you intend to discard local metadata:

```bash
bash scripts/reset-dev-db.sh
```

## Auth

The default fixture mode is:

```yaml
auth:
  mode: dev_no_auth
```

For local auth, set an admin email in config and provide the first password through `CARTOLENSIA_ADMIN_PASSWORD` or a configured ignored `admin_password_file`.

Useful endpoints:

- `GET /api/v1/auth/me`
- `GET /api/v1/auth/csrf`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/password`
- `GET/POST /api/v1/auth/tokens`

Cookie-authenticated write requests need the CSRF header from `/auth/csrf`. Bearer API tokens use scopes and do not need CSRF.

## Jobs

Job APIs:

- `GET /api/v1/jobs`
- `GET /api/v1/jobs/stats`
- `GET /api/v1/jobs/{id}`
- `GET /api/v1/jobs/{id}/logs`
- `POST /api/v1/jobs/{id}/cancel`
- `POST /api/v1/jobs/{id}/retry`

Main job starters:

- `POST /api/v1/discovery/start`
- `POST /api/v1/discovery/dry-run`
- `POST /api/v1/hash/start`
- `POST /api/v1/metadata/enrich/start`
- `POST /api/v1/previews/start`
- `POST /api/v1/gps/tracks/{track_asset_id}/snap-media`

Workers lease jobs, heartbeat while running, recover panics into failed jobs, and retry transient failures until `max_attempts`.

## Synthetic Scale Testing

Prefer a temporary root for unattended runs:

```bash
CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/generate-synthetic-fixture.sh
CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/perf-smoke.sh
```

The scripts generate dummy files only. Remove the temporary root when done:

```bash
rm -rf /tmp/cartolensia_synthetic_media
```

Run bounded worker/job stress checks:

```bash
bash scripts/worker-stress-test.sh
```

DB-backed worker stress remains gated:

```bash
CARTOLENSIA_RUN_DB_TESTS=1 CARTOLENSIA_TEST_DATABASE_URL=postgres://... bash scripts/worker-stress-test.sh
```

## Dry-Run Reports

Scoped discovery dry runs are report-only and require non-empty prefixes. Defaults are conservative: `max_files <= 50`, `max_bytes` defaults to 2 GiB, and missing marking is rejected.

Example payload for fixture/synthetic storage:

```json
{
  "storage": "fixture",
  "prefixes": ["photos"],
  "max_files": 50,
  "max_bytes": 2147483648,
  "include_extensions": ["jpg", "jpeg", "png"],
  "hash": false,
  "metadata": false,
  "previews": false,
  "mark_missing": false
}
```

For a future real archive dry run, start from `config/rclone-dryrun.example.yaml` and `scripts/rclone-dry-run-preflight.sh`, but do not execute the script unless a supervised prompt explicitly authorizes it.

## Verification Commands

Recommended local verification:

```bash
gofmt -w $(find internal cmd -name '*.go' -print)
git diff --check
GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...
go test ./...
npm --prefix webui run build
bash scripts/smoke-test.sh
docker compose -f docker-compose.yml -f docker-compose.dev.yml config
bash scripts/test-db.sh
```

If Docker is unavailable, skip `scripts/test-db.sh` and document that block.

## Safety

Use `testdata/media_fixture/` and synthetic temporary fixtures for tests. Do not use `/mnt/Models/rclone` unless a future supervised dry run explicitly permits it. Original storage roots are read-only; previews and generated data belong in Cartolensia cache/work directories or ignored synthetic roots.
