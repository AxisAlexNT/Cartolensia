# Cartolensia

Cartolensia is an idea of open-source, self-hosted multimedia archive for large photo, video, and GPS-track collections. Currently W.I.P.

It is designed for tourists, bikers, hikers, transport fans, and people who manage multi-year, multi-device, multi-terabyte media collections. The goal is not only to create albums or nostalgic memory views, but to make large personal media archives searchable, map-addressable, deduplicatable, and technically manageable.

## Status

Phase 1 vertical-slice MVP. The repository contains a runnable Go backend, PostgreSQL-capable metadata store, async discovery/hash workers, strict read-only fixture storage, Vue 3 WebUI, Docker Compose development database, and smoke/integration test scripts.

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

## License

Cartolensia is licensed under AGPL-3.0-or-later.
