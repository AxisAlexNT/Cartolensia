# Cartolensia

Cartolensia is an idea of open-source, self-hosted multimedia archive for large photo, video, and GPS-track collections. Currently W.I.P.

It is designed for tourists, bikers, hikers, transport fans, and people who manage multi-year, multi-device, multi-terabyte media collections. The goal is not only to create albums or nostalgic memory views, but to make large personal media archives searchable, map-addressable, deduplicatable, and technically manageable.

## Status

Phase 2/3 foundation work is in progress. The repository contains a runnable Go backend, PostgreSQL-capable metadata store, async workers, strict read-only fixture storage, local auth, GPX/track APIs, image preview cache, capability inventory endpoints, Vue 3 WebUI, Docker Compose development database, and smoke/integration test scripts.

Current implemented slices:

* Memory mode for fixture development and PostgreSQL mode for durable metadata.
* Embedded SQL migrations with checksum tracking.
* Strict read-only filesystem storage using normalized `fs://storage/path` URLs.
* Fast discovery, lazy SHA-512 hashing, explicit metadata enrichment, and preview generation jobs.
* PostgreSQL-backed job leases, heartbeats, cancellation, retries, logs, stats, and worker panic recovery.
* Local admin bootstrap, login/logout, password change, sessions, API tokens, token scopes, HttpOnly cookies, and CSRF protection for cookie-authenticated write requests.
* Explorer folder grouping, DB-backed asset filtering, asset detail, original read-only streaming, and preview endpoints.
* Virtual albums plugin MVP with nested albums, item membership, and map/explorer integration.
* Server-side EXIF parsing for JPEG metadata and GPS extraction into typed geotag records.
* GPX parsing, GPS track manager APIs, track/media lookup, conservative track-snapped geotags, map GeoJSON, live video-track sync link skeleton, ffprobe metadata extraction, and transcoding/AI/vector status contracts.
* Photo/map plugin MVP in the WebUI using bundled OpenLayers vector layers; no CDN or remote tile dependency.
* Persistent preview-cache index, preview-cache status endpoints, and dry-run cleanup controls.
* Scoped discovery dry-run endpoint, scan-run report table, guarded `rclone_dryrun` example config, and future dry-run preflight script.
* Synthetic fixture generation and bounded performance smoke scripts.

## Core ideas

* Self-hosted photo, video, and GPS-track management.
* PostgreSQL-backed resilient metadata storage.
* Strict read-only indexing of original media by default.
* Storage backends for local filesystem first, with SMB, NFS, and S3 planned.
* Fast discovery pass first, lazy/background SHA-512 hashing later.
* GPS track snapping and geotag prediction.
* Map-first browsing using offline-capable maps.
* On-demand previews by default instead of permanent sidecars.
* Original video streaming with HTTP Range support.
* Optional on-the-fly transcoding through ffmpeg.
* AI-assisted search, classification, embeddings, and transport-specific recognition.
* Plugin-oriented architecture with backend and WebUI extension points.

## Documentation

* [Implementation plan](docs/IMPLEMENTATION_PLAN.md)
* [Product vision](docs/PRODUCT_VISION.md)
* [Architecture](docs/ARCHITECTURE.md)
* [Operations](docs/OPERATIONS.md)
* [Security](docs/SECURITY.md)
* [Real archive dry-run guide](docs/REAL_ARCHIVE_DRY_RUN.md)
* [Target architecture](docs/ARCHITECTURE_TARGET.md)
* [AI assistance note](docs/AI_ASSISTANCE.md)
* [Raw original idea](ideas/general_description.md)

## Development

Run local checks:

```bash
make smoke
```

Run the sandbox-friendly Go suite directly:

```bash
GOCACHE=/tmp/cartolensia-go-build GOTOOLCHAIN=local go test ./...
```

Start the development PostgreSQL/PostGIS service:

```bash
make dev-db
```

Run the backend with the fixture-only memory store:

```bash
make backend-run
```

Run the backend against the development PostgreSQL service:

```bash
go run ./cmd/cartolensia -config config/dev-postgres.yaml
```

Run gated PostgreSQL integration tests against the development database:

```bash
bash scripts/test-db.sh
```

Generate a synthetic fixture outside the repo and run a bounded performance smoke:

```bash
CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/generate-synthetic-fixture.sh
CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/perf-smoke.sh
```

Run the bounded worker stress entrypoint:

```bash
bash scripts/worker-stress-test.sh
```

Future real-archive dry-run preparation is documented in `docs/REAL_ARCHIVE_DRY_RUN.md`. Do not run `scripts/rclone-dry-run-preflight.sh` unless a supervised prompt explicitly approves it and sets the required environment variables.

## Auth Modes

The fixture workflow defaults to explicit `dev_no_auth`. Local auth is enabled by configuration:

```yaml
auth:
  mode: local
  admin_email: admin@example.local
```

The admin password must come from `CARTOLENSIA_ADMIN_PASSWORD` or a configured `admin_password_file`; no production password is hardcoded. Local auth uses sessions, HttpOnly cookies, CSRF tokens for cookie-authenticated writes, and scoped API tokens for automation.

## Safety

Original media is treated as immutable. The implemented filesystem adapter is strict read-only: list/stat/open are allowed; write/delete/move/mkdir are rejected. Preview files are generated only under the Cartolensia cache directory and never beside originals.

The test and smoke workflows use `testdata/media_fixture/` or synthetic fixtures. `/mnt/Models/rclone` is not required and must not be touched by automated tests.

## License

Cartolensia is licensed under AGPL-3.0-or-later.
