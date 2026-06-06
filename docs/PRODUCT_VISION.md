# Product Vision

Cartolensia is a self-hosted multimedia archive for large photo, video, and GPS-track collections where place, movement, time, routes, and technical observations matter.

The product is aimed at tourists, hikers, bikers, transport fans, and people managing multi-year, multi-device archives. It should make archives searchable and map-addressable without requiring the user to remember the exact year, device, or folder.

Core goals:

- browse media through folders, maps, tracks, and time;
- index original files without modifying them;
- keep metadata and jobs durable in PostgreSQL;
- separate logical assets, byte content, and storage locations;
- stream originals safely with HTTP Range support;
- use fast metadata discovery first and lazy SHA-512 hashing later;
- support PostGIS when available without making it mandatory for core startup;
- leave vector search behind a future `VectorStore` interface so pgvector is optional;
- support plugin-delivered features without using raw Go `.so` plugins as the main path.

The current MVP proves the vertical slice on generated fixture data only. Heavy areas such as albums, maps, GPS track editing, transcoding, and AI classification are exposed as built-in plugin manifests and WebUI stubs until their contracts are ready.
