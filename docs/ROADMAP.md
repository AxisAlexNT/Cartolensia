# Roadmap

## Phase 0: Repository Preparation

- Normalize documentation paths.
- Add target architecture, storage, plugin, schema, and roadmap docs.
- Add Go, WebUI, Docker Compose, Makefile, scripts, environment example, and fixture scaffold.
- Record tool availability and unattended-run risks in `RUN_REPORT.md`.

## Phase 1: Core Vertical Slice

- Config loader. Implemented.
- PostgreSQL migrations and config snapshots. Implemented.
- Strict read-only filesystem storage. Implemented.
- Plugin manifest loader and dependency sort. Implemented.
- Persistent job queue in PostgreSQL and memory fallback. Implemented for synchronous jobs.
- Fixture discovery. Implemented.
- Streaming SHA-512 hashing. Implemented.
- REST API. Implemented for MVP endpoints.
- Vue Explorer, Discovery, Storages, Plugins, Stats, and plugin stubs. Implemented.
- Smoke workflow. Implemented.
- Embedded migrations with optional explicit disk loading. Implemented.
- Durable job leases, heartbeats, cancellation, retry scheduling, and panic recovery. Implemented.
- Async discovery/hash workers. Implemented.
- PostgreSQL integration tests gated by environment variables. Implemented.
- Folder-style Explorer and asset detail API/UI. Implemented.
- Local auth interfaces plus `dev_no_auth` mode. Implemented.
- Local auth bootstrap, login/logout, sessions, and API tokens. Implemented.
- Preview cache/status foundation. Implemented.
- Standard-library image preview generation. Implemented for decodable image files.
- GPX parser, track listing/detail APIs, and GPS Tracks UI. Implemented.
- Live video-track sync candidate/link skeleton. Implemented.
- GeoJSON map API and basic Map UI. Implemented.
- ffprobe detection and best-effort video metadata extraction. Implemented.

Remaining Phase 1 hardening:

- DB-backed sustained worker stress tests beyond fixture integration tests. Partially implemented via `scripts/worker-stress-test.sh`; DB stress remains gated by environment variables.
- Rescan/missing-file state transitions for explicitly bounded scopes.
- Real map tiles. OpenLayers vector rendering is implemented; OSM base tiles can be fetched on demand through a local Cartolensia tile cache/proxy, but packaged offline tile sets remain future work.

## Phase 2: Media Usability

- Original streaming with robust Range and HEAD support. Implemented for read-only filesystem storage.
- Image previews generated on demand and by queued job into a cache outside originals. Implemented for Go-decodable images.
- Persistent preview-cache index/table and status/cleanup APIs. Implemented.
- Metadata enrichment job for image dimensions, JPEG EXIF metadata/GPS, ffprobe video metadata, and GPX analysis. Implemented.
- Job dashboard APIs and WebUI controls for stats/detail/logs/cancel/retry. Implemented.
- Explorer/asset list filtering, sorting, pagination headers. Implemented; PostgreSQL asset queries use DB-backed filtering for common cases.
- Universal Explorer search with match explanations across names, paths, extensions, date fragments, hash prefixes, metadata, tags, albums, and tracks. Implemented as an MVP endpoint/UI.
- Missing-file detection and rescan behavior. Future work.
- Video poster generation. Future work.

## Phase 3: Map And Tracks Foundation

- Virtual Albums plugin MVP. Implemented.
- Track ingestion from GPX, KML, and KMZ. Implemented.
- Track analysis with bbox, distance, duration, elevation, and simplification. Implemented.
- GPS Track Manager plugin MVP with detail, point queries, media lookup, offset control, and snap-media job. Implemented.
- Track click popups with nearest-point metadata, relative time/speed/elevation when available, time-based media lookup, and geotag proximity lookup. Implemented.
- Track previews and generated thumbnails for GPX/KML/KMZ/GPZ assets. Implemented with dark cached fallback rendering; OSM-backed thumbnail compositing remains future work.
- Photo/map plugin MVP with GeoJSON APIs, album/media/source/track filters, screen-distance clustering, count labels, clickable cluster/point/track popups, typed `asset_geo`, and OpenLayers WebUI. Implemented.
- Live video-track sync candidates, manual links, deletion, and marker interpolation API. Implemented.
- On-demand OSM tile proxy/cache with attribution and no public-OSM bulk prefetch. Implemented.
- Richer manual geotag editing. Future work.

## Phase 4: Plugin Expansion

- Sidecar HTTP plugin manifest contract and health stub. Implemented.
- WebUI plugin asset mounting.
- Plugin-specific settings backed by PostgreSQL and schema-based WebUI plugin settings tabs. Implemented as a generic settings UI; custom plugin-rendered components remain future work.
- Capability and health reporting per plugin. Implemented as core/built-in status and sidecar stub.

## Phase 5: Transcoding And AI Foundations

- Transcoding preset/output schema contracts. Implemented.
- ffmpeg/ffprobe encoder and hardware capability detection. Implemented.
- Cache-scoped HLS transcode session MVP. Implemented with built-in/custom preset metadata, browser HLS/hls.js playback path, and cache cleanup safety; durable transcode job management remains future work.
- AI/vector status APIs, accelerator hints, optional HTTP sidecar worker contract, Docker Compose worker profiles, and packaged local sidecar. Implemented.
- Local AI inference with approved models: torchvision classification, OpenCV YuNet face detection, Falconsai safety classification, OpenCLIP image/text embeddings, BLIP captioning smoke, prediction/tag/face/embedding persistence, and local brute-force vector search. Implemented as explicit bounded jobs.
- Embedding schema contract without required pgvector. Implemented with JSON/PostgreSQL fallback.
- Durable transcoding jobs, managed transcoding outputs, face identity/grouping workflows, and production-grade AI review queues. Future work.

## Phase 6: Large Archive Hardening

- Incremental scanning at scale.
- Better duplicate workflows without destructive defaults. Report-only duplicate grouping by SHA-512+size is implemented; review/merge/trash workflows remain future work.
- Observability and admin status pages.
- Offline resource packaging.
- Scoped dry-run readiness for future tiny real-archive tests. Implemented with report-only API, scan-run records, guarded config/script examples, strict default limits, and real-archive backend guardrails.
- Temporary real-peek helper scripts for supervised bounded read-only indexing. Implemented.
- Performance tests on synthetic large datasets. Implemented as bounded scripts; more metrics remain future work.

## 2026-06-07 Progress

Completed in the latest implementation run:

- Track detail manager page with map, profile charts, stats, and media lookup actions.
- Runtime-safe storage add/validate API and UI.
- Conservative preview policy default: persistent preview generation is off unless explicitly enabled.
- Transcoding preset validation and hardware-test flow, including native NVENC dry-run success for H.264 on the current real-peek video.
- Packaged AI sidecar with dummy and local inference modes; backend worker health probing and scoped AI jobs.
- Universal search improvements for date ranges, album tokens, track tokens, AI tags/categories/safety/captions, and vector text-to-image search.

Next priorities:

- Persist runtime storage changes durably.
- Move visible AI action records into durable leased background AI workers for longer inference jobs.
- Harden AI review workflows, performance controls, and browser visualizations around the now-functional local AI pipeline.
- Add OSM-background track thumbnail rendering.
- Split large WebUI pages into smaller components.
- Add browser automation for gallery/transcoding/map interaction checks.

## 2026-06-07 Focused Productization Progress

Completed:

- Native CUDA and optional Docker NVIDIA profile status are now separated in API/UI.
- Docker NVIDIA runtime probing was validated with the approved CUDA base image.
- AI action buttons now create visible job records and return job IDs.
- AI Classification page now exposes tags/categories, predictions, safety candidates, face detections, and vector search instead of a stub.
- Track preview rendering now reports status and draws track vectors above OSM tiles in detail/gallery contexts.
- Settings gained a read-only allowlisted file/folder picker for path settings.
- Transcoding page gained configuration tabs for presets, auto-selection rule drafts, command templates, job planning, and metrics foundation.

Still future:

- Durable AI worker leases for long jobs.
- Full transcode job execution/planning with persistent outputs outside originals.
- Browser automation coverage for the new file picker, AI pages, track previews, and transcoding settings.

## 2026-06-07 Workflow Stabilization Progress

Completed:

- File picker modal layering was fixed so the dialog remains above the backdrop and usable.
- Track overlay sizing/refit behavior was improved for gallery/detail contexts.
- Map cluster single-click behavior was stabilized; zooming into a cluster is now explicit.
- Face Gallery MVP was added with provisional face folders, naming, cluster asset lists, ignored detections, and asset-detail face boxes.
- Large AI job payloads are summarized in job lists to avoid UI stalls.
- Transcoding metrics availability is visible for `libvmaf`, `ssim`, and `psnr`.
- AV1 live HLS is disabled with a clear reason instead of failing after expensive startup attempts.
- Geo Align and Video Track Player MVP APIs/pages were added with safe scoped behavior.

Next:

- Replace provisional face grouping with embedding-based local clustering and merge/split review workflows.
- Make Geo Align sessions durable and add the full OpenLayers shift-drag marker editor.
- Harden face clustering with embedding-based merge/split workflows, representative crops, and better user review queues.
- Promote AV1 WebM preview into durable queued AV1 jobs for longer videos while keeping direct/original and H.264 HLS as fast live paths.
- Add browser automation for OSM tile visibility, track vector overlays, face rectangles, and AV1/HLS playback controls.
- Add user-provided video start/end timestamp controls for reliable video-track synchronization.
- Implement a verified AV1 browser playback route with WebM or fragmented MP4 when encoders and browser support are confirmed.
- Add browser automation for gallery track previews, map cluster popups, modal focus/backdrop behavior, and face review.

## 2026-06-07 Overnight Continuation Progress

Completed:

- Geo Align marker editing was hardened with Shift-drag marker movement, marker popups, track point popups, and per-marker reset/apply behavior.
- Asset detail photo layout was constrained so original/view-size images remain inside the image workbench and face overlays align to the displayed image.
- Face Gallery gained representative face thumbnails through a read-only face-crop endpoint.
- Universal search gained cache-only local place matching; `Yerevan`, `Vanadzor`, `Lori Province`, and `Armenia` are represented in the built-in local cache.
- Universal search now reports an explicit `postgres_local` backend through a small `SearchBackend` abstraction; Elasticsearch/OpenSearch remain deferred.
- OCR gained a manual Base AI job/API/UI contract with bounding-box metadata and search integration through stored OCR text predictions.
- Settings gained a Search/Places tab showing geocoder mode, online-geocoding status, provider, and local place-cache entries.
- AV1 WebM output was validated on the current bounded short video using the cache-scoped route; HLS remains the H.264/NVENC interactive path.

Next:

- Replace built-in place entries with a durable operator-editable `place_cache` table and import/export UI.
- Install and validate Tesseract OCR language packs for English, Russian, Armenian, and Chinese in the supervised environment, then wire the sidecar OCR runtime into `/ocr-image`.
- Add browser automation for Geo Align Shift-drag editing, track point popups, place search cards, and face-crop thumbnails.
- Move long AV1 jobs to durable queued/offline transcoding with progress and cancellation.

## 2026-06-08 OCR/Place Cache Progress

Completed:

- Tesseract OCR runtime is wired into the sidecar `/ocr-image` endpoint.
- English, Russian, Armenian, Simplified Chinese, and Traditional Chinese language availability is reported in AI worker status.
- OCR blocks are searchable metadata with bounding boxes and asset-detail delete controls.
- Durable `place_cache` migration/store/API is implemented for local/cache-only place search.
- Settings Search/Places now provides an operator place-cache editor.
- Linux x86_64 offline distribution packaging is implemented with a local `7z` packager, manual GitHub Actions release workflow, launcher scripts, optional OCR/media/PostgreSQL/Python runtime bundling, dependency manifests, and release checksums.
- Component Manager MVP is implemented with persistent component records/events, Settings UI, safe user path/archive import, component checks, and distribution component manifests. Direct downloader implementations remain provenance-gated future work.

Next:

- Add JSON import/export for place cache entries under `.cartolensia/exports`.
- Add browser automation around OCR overlay highlighting, place-cache editing, and place-search navigation.
- Add long-caption workflow controls and storage.
- Continue safety/private visibility and WebDAV/productization work.
- Add cross-platform distribution targets after the Linux x86_64 package has been validated on clean offline hosts.
- Add a release-time license review checklist for model weights, CUDA wheels, and ffmpeg codec flags.

## 2026-06-08 Multimodal Roadmap Update

Completed:

- Audio media kind detection and ffprobe metadata extraction.
- Document media kind detection.
- Durable schema for transcripts, transcript segments, audio features, video frame captions, and document text.
- Asset-detail APIs and UI panels for OCR full text, transcripts, audio features, frame captions, and document text.
- PostgreSQL/local universal search hooks for OCR, transcripts, documents, frame captions, and audio features.
- Cache-safe bounded live indexing of the current Sound Records audio samples.
- faster-whisper ASR sidecar endpoint, backend transcription jobs, asset transcript UI, and transcript search are implemented and validated on one bounded Sound Records WAV.
- Component Manager now tracks ASR/audio runtime components and the installed `faster-whisper-small` model.
- librosa/SoundFile audio analysis jobs now persist tempo, key, loudness, speech/music ratio, spectral summary, and heuristic labels.
- Tempo range search such as `tempo:120..140` is supported.

Next:

- Add a reviewed genre classifier model beyond heuristic labels.
- Optionally install/review `faster-whisper-medium` for higher quality ASR.
- Add Marker/PDF document extraction with Tesseract fallback and Markdown storage.
- Add simple video frame sampling jobs that use existing image classification/caption/OCR tools.
- Add waveform peak generation stored in PostgreSQL/cache metadata.
- Add durable media-track player sessions for audio as well as video.
- Implement persisted zoom-level map cluster cache after the current screen-distance cluster path is fully characterized.

## 2026-06-09 Production Release Preparation

Completed:

- Production config templates for `/originals` deployments.
- Offline-airgap, production-container, and `.env.production.example` templates.
- Production compose file for a PostgreSQL-backed container deployment.
- Release helper scripts for Linux packaging, license checking, and production compose smoke checks.
- CI workflow for Go/WebUI/build/config validation plus smoke-script checks.
- Release documentation for installation, air-gapped use, production deployment, offline components, building, user workflow, and release review.

Next:

- Validate the production compose path on a clean host with `/originals` mounted read-only.
- Confirm the offline bundle layout on a real extracted archive.
- Decide whether a separate host-based systemd unit template is still needed for non-container deployments.

## 2026-06-09 Full Prefix Indexing Progress

Completed:

- Full read-only `Cartolensia-photos` indexing path validated with `max_files=-1`.
- Central supported-extension defaults now include audio and lightweight document formats.
- Geo Align switch layout fixed.
- Direction arrows are available for track vector visualizations with a configurable default interval.
- Cache-first reverse geocoding endpoint exists and can optionally cache user-triggered online Nominatim-compatible lookups.
- Full explicit-prefix AI metadata pass was executed for classification, safety, faces, embeddings, captions, OCR, ASR, and audio analysis.

Next:

- Add persisted zoom/tile-level map cluster cache endpoints and UI refresh controls.
- Add place-cache import/export for offline gazetteer packages and operator-managed regional datasets.
- Add reviewed source URLs or offline package import flow for VMAF, optional Whisper medium, and MobileNetV3 fallback weights.
- Add browser automation for Geo Align switches/arrows, track previews, reverse-place refresh, and unlimited-indexing UI labels.
- Split large WebUI chunks with route-level dynamic imports.

## 2026-06-09 Context/Search Stabilization Progress

Completed:

- Audio cards and gallery overlay now provide audio playback instead of generic no-preview fallback.
- GPS track media lookup now uses EXIF raw timestamps, filename timestamps, file mtimes, and geotag proximity.
- The concrete `20260509-144424.gpx` trip now returns the expected `PXL_20260509_165208189.jpg`, `PXL_20260509_172507172.jpg`, and timestamp-matched videos.
- Video Track Player now derives usable timestamps from filename/mtime candidates and has searchable video plus track-pill selectors.
- Asset detail gained a Related Context section backed by `/api/v1/assets/{id}/related`.
- Search and Explorer counts are synchronized for MP4 extension queries; `mp4` and `ext:mp4` both report the full live set.
- Search syntax now supports AND terms, comma OR groups, wildcards, explicit filename/path/extension/kind tokens, and in-page help.
- Major asset-detail navigation links now preserve browser middle-click/new-tab behavior.
- Storage/prefix-scoped metadata enrichment now uses paginated shared asset queries instead of flattening the full catalog first.

Next:

- Implement folder-sharded discovery worker pools for NAS-scale scans.
- Turn related/context into a stronger trip/session graph with persisted scoring and richer device/place/track joins.
- Replace the Video Track Player placeholder map with a full OpenLayers synchronized map.
- Add browser automation for middle-click-safe anchors, audio previews, selector typeaheads, and search count consistency.
## 2026-06-27 Current Production Priorities

Recent progress:

- HTTPS production listener, secure local-auth cookies, and self-signed LAN deployment support are implemented.
- pgvector is supported as the default large-archive vector backend when PostgreSQL has the `vector` extension available.
- Remote NVIDIA/CUDA AI sidecar operation has been validated with classification, safety, BLIP captions, OpenCLIP embeddings, Tesseract OCR, faster-whisper ASR, and audio analysis.
- Low-concurrency missing-work AI backfill exists for large archives and avoids repeated no-result OCR/ASR work through local state files.
- Discovery can intentionally scan a storage root or all configured storages, while missing marking remains blocked for read-only/original storage.
- Discovery now auto-prunes nested child storage roots from parent/all-storage scans, so broad Samba parents and narrower child storages can coexist without double indexing.
- The WebUI shell has a mobile-friendly navigation and overlay pass.
- Explorer folder views now use PostgreSQL aggregation and paged direct-file queries, with WebUI Load More pagination and compact month filtering.
- Vite chunks are split for Vue/OpenLayers/HLS; HLS is lazy-loaded only when video transcoding playback requires it.
- Essential metadata export exists as a single `.7z` archive containing PostgreSQL dump, redacted config, storage manifest, and restore notes without originals/previews.

Next high-value work:

- Add a durable per-asset AI run ledger so tasks like face detection can record "checked, no faces" without relying on external backfill state files.
- Promote the AI backfill driver into a first-class PostgreSQL job type with pause/resume controls in the Jobs page.
- Add scheduled essential export/backup rotation and restore smoke testing.
- Add pgvector health/readiness checks to Diagnostics and Component Manager.
- Continue mobile UX testing on Android/iOS browsers, especially map/player/gallery workflows.
- Add operator controls for AI backfill rate limits, per-storage scopes, and no-speech/no-text policies.
