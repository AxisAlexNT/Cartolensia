# Cartolensia

Cartolensia is an idea of open-source, self-hosted multimedia archive for large photo, video, and GPS-track collections. Currently W.I.P.

It is designed for tourists, bikers, hikers, transport fans, and people who manage multi-year, multi-device, multi-terabyte media collections. The goal is not only to create albums or nostalgic memory views, but to make large personal media archives searchable, map-addressable, deduplicatable, and technically manageable.

## Status

Very early development. The repository currently contains the product vision, target architecture, and the first implementation plan.

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
* [Project vision](docs/PROJECT_VISION.md)
* [Target architecture](docs/ARCHITECTURE_TARGET.md)
* [AI assistance note](docs/AI_ASSISTANCE.md)
* [Raw original idea](ideas/general_description.md)

## License

Cartolensia is licensed under AGPL-3.0-or-later.
