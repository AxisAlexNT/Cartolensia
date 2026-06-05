# AGENTS.md

## Project

Cartolensia is an AGPL-3.0-or-later self-hosted multimedia archive for large photo, video, and GPS-track collections.

The first goal is a runnable vertical-slice MVP, not a fake demo.

## Hard safety rules

* Treat `/mnt/Models/rclone` as strictly read-only.
* Never write, rename, delete, chmod, transcode into, or modify anything inside `/mnt/Models/rclone`.
* Do not commit media files, caches, generated previews, model weights, database volumes, `.env`, credentials, or local machine paths containing secrets.
* Do not implement destructive storage operations unless the safety model is explicit and tested.
* Originals are immutable by default.
* Default storage mode is `strict_read_only`.

## License and provenance

* Project license: AGPL-3.0-or-later.
* Do not copy source code from external repositories or webpages.
* Use dependencies through normal package managers only.
* Prefer permissive dependencies: MIT, BSD, Apache-2.0, ISC, MPL-2.0.
* Avoid GPL/AGPL dependencies unless explicitly justified.
* Record important dependencies and licenses.
* If uncertain about provenance, write original code instead.

## Technical stack

* Backend: Go.
* Frontend: Vue 3 + TypeScript + Vite.
* Database: PostgreSQL.
* Geospatial: PostGIS when available.
* Vector search: optional pgvector through an abstract VectorStore interface.
* Runtime: native binary + PostgreSQL minimum; Docker Compose recommended.
* Configuration: YAML.
* Jobs: PostgreSQL-backed persistent jobs.
* Maps: OpenLayers frontend, offline-capable map cache design.
* Media: no permanent sidecars by default; on-demand previews/cache.

## Architecture priorities

Implement real core first:

1. Config loader.
2. PostgreSQL migrations.
3. Storage registry.
4. Strict read-only filesystem adapter.
5. Plugin manifest loader and dependency topological sort.
6. Job queue.
7. Originals discovery.
8. Fast metadata indexing.
9. Lazy/background SHA-512 hashing.
10. REST API.
11. Vue UI shell and Explorer.
12. Tests and smoke scripts.

Stub heavy features behind clean interfaces:

* SMB/NFS/S3 adapters.
* Albums.
* Map clustering.
* GPS track manager.
* Video transcoding manager.
* Base AI manager.
* AI classification manager.
* Vector search.
* Live video-track sync.

## Discovery policy

Use two-stage discovery.

Stage 1: fast safe scan.

* storage URL
* relative path
* file name
* size
* mtime
* MIME
* extension
* cheap image/video metadata if available
* EXIF/GPS if feasible

Stage 2: lazy/background hashing.

* streaming SHA-512
* never load whole files into memory
* prioritize likely duplicates, moved files, selected files, and integrity checks

Do not deduplicate, replace, trash, or modify anything based only on quick metadata.

## Storage model

Use internal IDs. SHA-512 is an indexed content key, not the only identity.

A file can have multiple storage locations. Moving a file should update/add location records, not create a duplicate logical asset when content is confirmed equal.

Universal URL examples:

* `fs://originals/2020/trip/photo.jpg`
* `smb://nas-share/path/photo.jpg`
* `s3://bucket/key`

## Plugin model

Do not rely on Go `.so` plugins as the main plugin mechanism.

Use:

* manifest-discovered built-in Go plugins first
* sidecar HTTP plugin runtime later
* sidecar gRPC future
* webui static assets per plugin
* experimental Go `.so` only as future developer mode

Each plugin lives under `plugins/<id>/` and may contain:

* `plugin.yaml`
* `config.yaml`
* `webui/dist/`

## Frontend requirements

* Vue 3 + TypeScript.
* No CDN resources.
* Modern, clean UI.
* Backend-persistent app/job state.
* Browser may store last route/session token.
* Pages should compile even when backend feature is still stubbed.

## Tests

Add tests for:

* config loading
* plugin dependency sorting
* storage URL normalization
* path traversal prevention
* strict read-only FS behavior
* SHA-512 streaming hashing
* job state transitions
* discovery indexing on fixture files

Run all feasible tests before finishing. If a test cannot run due to missing system dependencies, document it in `RUN_REPORT.md`.

## Reporting

Every substantial autonomous run must update `RUN_REPORT.md` with:

* what was implemented
* commands run
* tests run
* failures and fixes
* known limitations
* next recommended task
