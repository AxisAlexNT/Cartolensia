# Security

Cartolensia is still pre-release software. The current security model is intended for local/self-hosted development and cautious private deployments, not exposed public hosting.

## Auth Modes

`dev_no_auth`

- Explicit development mode.
- Fixture workflows remain easy.
- Backend status includes a warning when auth is disabled.
- Do not expose this mode to untrusted networks.

`local`

- Local admin user is bootstrapped from config plus `CARTOLENSIA_ADMIN_PASSWORD` or an ignored password file.
- No production admin password is hardcoded.
- Bootstrap is idempotent; password rotation from bootstrap config is explicit.
- Passwords are hashed before storage.
- Login creates persisted sessions.
- Logout and password change invalidate appropriate session state.

OAuth/OIDC remains disabled-by-default stub behavior.

## Sessions And CSRF

Session cookies are:

- `HttpOnly`
- `SameSite=Lax`
- `Secure` when configured for HTTPS deployments

Cookie-authenticated write requests must include a CSRF header. Get it from:

```http
GET /api/v1/auth/csrf
```

Bearer API token requests do not need CSRF because they are not browser-cookie authenticated.

## API Token Scopes

Implemented scopes:

- `read`
- `write`
- `jobs:write`
- `plugins:write`
- `media:read`
- `admin`

Admin sessions can perform all actions. API tokens must carry a sufficient scope; role alone is not enough for a token without scopes.

Protected write-like endpoints include discovery/hash/metadata/preview starts, plugin rescan, job cancel/retry, password change, and token creation/revocation.

## Storage Safety

Original media is immutable in the implemented adapter.

- Storage mode defaults to `strict_read_only`.
- Filesystem writes, deletes, moves, and mkdir operations return explicit read-only errors.
- Path traversal and absolute paths are rejected.
- Recursive discovery skips symlinks.
- Opening a symlink that escapes the root is rejected.
- Original media is streamed only through the registry.

Preview files are generated only under Cartolensia cache/work directories. Preview cache cleanup verifies deletion targets stay inside the cache root.

Scoped dry-run discovery is guarded separately:

- storage must exist and be `strict_read_only`;
- prefixes are required and cannot be empty/root;
- default `max_files` is 50;
- `mark_missing` is rejected;
- current dry-run behavior is report-only and does not index assets.

When a configured storage root is `/mnt/Models/rclone` or under it, normal discovery and hashing have additional real-archive guardrails:

- `storage=all` is rejected;
- a concrete storage name is required;
- a non-empty adapter-relative prefix is required;
- `max_files` and `max_bytes` are required;
- empty, root, dot, dot-dot, and archive-root-equivalent prefixes are rejected;
- absolute archive prefixes are normalized only when they are safely inside the configured storage root.

The guarded real-peek scripts use a temporary PostgreSQL Compose project, repo-local cache/runtime directories, and `strict_read_only` storage. They are not test fixtures and should stay supervised.

## Media And External Tools

ffprobe and ffmpeg are detected best-effort. Missing tools do not fail discovery or core startup. Stream options expose direct original streaming by default. If ffmpeg is available, cache-scoped transcode sessions can be started manually. H.264 profiles use HLS. AV1 profiles use a browser-oriented WebM route when a compatible encoder such as `libsvtav1` is available. Generated transcode files stay under the configured Cartolensia cache directory and must never be written into original storage.

Transcoding preset records are metadata only. Built-in presets cannot be deleted, custom presets are validated before use, and session output is scoped to the configured transcode cache. The browser uses locally bundled `hls.js` where native HLS playback is unavailable; it does not fetch player code from a CDN.

The OpenStreetMap tile proxy is on-demand only. It caches tiles actively viewed by the browser under the Cartolensia cache directory, provides attribution metadata, and does not implement public-OSM region prefetching.

Track thumbnails/previews for GPX/KML/KMZ/GPZ assets are generated under the Cartolensia preview/cache root. They are never written beside original track files.

Universal search uses PostgreSQL/local metadata by default. Place-name matching is cache-only through Cartolensia local place entries such as Yerevan, Vanadzor, Lori Province, and Armenia. The app does not call public reverse-geocoding/geocoding APIs automatically; future online provider support must be user-triggered, rate-limited, and cached before reuse.

OCR is a manual AI job/action. OCR text and bounding boxes are metadata records and must not write temporary inputs, OCR cache files, or derived text sidecars into original storage. Missing OCR engines/language packs must be reported as job/worker errors rather than silently falling back to remote services.

Settings DB exports are metadata/config JSON files written under the configured Cartolensia cache export directory. Import planning is validation-only and does not perform destructive restore while the app is live.

## HTTP/TLS

Plain HTTP is the default and should be bound to localhost for development. HTTPS can use configured certificate/key files or an in-memory self-signed certificate via `http.tls_auto_self_signed`. Self-signed TLS is intended for private/local deployments and does not replace a real certificate for exposed services.

## AI And Dependency Provenance

AI/vector APIs are explicit, local, and bounded. The backend does not download models by itself and does not use remote inference APIs. When an operator installs the approved Python dependencies and model weights, the optional sidecar can run local torchvision classification, OpenCV YuNet face detection, Falconsai safety classification, OpenCLIP embeddings, and BLIP captioning through user-triggered jobs. AI outputs are predictions, not truth, and are stored as Cartolensia metadata only.

Model caches, worker scratch space, generated predictions, and exports must stay under `.cartolensia/models`, `.cartolensia/exports`, or another configured non-archive path. They must never be placed under `/mnt/Models/rclone`.

AI/GPU status distinguishes native workers from Docker profiles. A configured native CUDA sidecar can be active while an optional Docker `ai-nvidia` profile remains not configured. Docker GPU probes must be explicit and supervised because they may require image pulls and host GPU access.

Do not copy third-party source into the repository. Add dependencies only through normal package managers and document why they are needed.

Current added dependency notes:

- `ol` is bundled through npm for OpenLayers map rendering; local package metadata reports `BSD-2-Clause`.
- `bootstrap` is bundled through npm for local UI styling; local package metadata reports `MIT`.
- `bootstrap-icons` is bundled through npm for local icons; local package metadata reports `MIT`.
- `hls.js` is bundled through npm for browser HLS playback; local package metadata reports `Apache-2.0`.
- `github.com/rwcarlsen/goexif` is used for server-side JPEG EXIF parsing; the cached module license is BSD-style and compatible with the project policy.
- EXIF parsing errors are non-fatal and are recorded as metadata; timezone-less EXIF datetimes are stored as raw metadata instead of being blindly promoted to `taken_at`.

## Known Limitations

- Local auth is admin-centric and not yet a multi-user sharing system.
- CSRF tokens are stateless per session token and rotate when the session token changes.
- No brute-force throttling or account lockout is implemented yet.
- API tokens are bearer secrets; store them carefully.
- Sidecar plugin health probing is a stub and sidecar execution is not implemented.
- Real archive scan procedures must be supervised and bounded until rescan/missing-file semantics are fully hardened.
- The OSM tile cache depends on network availability unless the tiles have already been viewed and cached; future fully-offline tile packs are not implemented yet.
- HLS playback now uses native browser HLS where available and locally bundled `hls.js` elsewhere. AV1 preview playback uses cache-scoped `video/webm` output with Range support when the session has finished. Manual browser testing across Chromium/Firefox/Safari is still required.

## 2026-06-07 Safety Notes

- Runtime storage changes are limited to non-destructive filesystem adapters. `journaled_deferred` and `read_write` remain disabled.
- Storage roots at or under `/mnt/Models/rclone` are rejected unless their mode is `strict_read_only`.
- The AI sidecar stores model/cache paths outside original media roots. Inference is explicit and scoped; it reads media through Cartolensia read-only media URLs and writes predictions/tags/embeddings only to the database.
- The approved NVENC validation used null-output ffmpeg dry-runs only. No transcoded file was written to the archive or cache during the validation command.
- On-demand previews remain the preferred default to avoid write amplification; persistent preview generation is opt-in.
- The server-side file/folder picker is read-only and allowlist based. It may expose configured storage roots for operator selection, but it does not write, scan, index, or change storage mode. Real archive roots are labeled strict read-only and remain protected by backend storage guards.

## 2026-06-07 Workflow Safety Notes

- Face folder naming, ignored detections, AI predictions, and safety labels are metadata only. They never move, hide, delete, or rewrite original files.
- Geo Align apply writes only Cartolensia database geotag overrides in the current real-peek mode. EXIF writeback is explicitly blocked for `strict_read_only` storage.
- Video Track Player sessions are read-only and derive transient marker positions from existing media metadata and track points.
- Map cluster popups and track previews read existing metadata and tile/cache data only; they do not trigger discovery or missing-file marking.
- AV1 live HLS is blocked when unsupported so the system does not start expensive, failing transcodes without a clear operator decision. The verified browser route is cache-scoped `video/webm` output when a compatible encoder is available.

## 2026-06-08 OCR And Geocoding Safety Notes

- Tesseract OCR is local-only and manual. It reads bounded localhost media URLs or safe temp/cache paths and writes only database metadata; it must not create OCR sidecars or cache files under original storage.
- OCR block deletion is metadata-only and scoped to OCR prediction rows for the selected asset.
- Place-cache editing changes Cartolensia metadata only. It does not write to media files, does not start discovery, and does not call public geocoding APIs.
- Local place search uses durable cache entries plus existing geotag metadata. Online geocoder support remains disabled by default and must be user-triggered, rate-limited, and cached if implemented later.

## 2026-06-08 Distribution Safety Notes

- Offline package builds write to repo-local `dist/` by default and must not stage archives, source snapshots, tools, AI environments, model weights, or generated runtime files under original media storage.
- The generated offline configs default to a package-local `media/` directory with `strict_read_only` mode.
- Bundled model weights are opt-in and require a license/provenance-reviewed model cache. The workflow default keeps model bundling disabled.
- Bundled ffmpeg, Tesseract, PostgreSQL, Python packages, and CUDA wheels must be reviewed before public redistribution. The packager records manifests and Debian copyright files where available, but those records do not replace legal review.
- GPU drivers are intentionally not bundled. CUDA-capable packages still depend on compatible host drivers and must not attempt to install kernel or system GPU components on the target host.
- GitHub release builds are manual. Release operators should use prerelease mode until the package has been tested on a clean offline host.
