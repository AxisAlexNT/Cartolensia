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
- Email addresses are normalized with surrounding whitespace trimmed. Submitted
  passwords keep all internal characters but tolerate trailing CR/LF so copying
  the generated password file into the WebUI login field does not fail because
  of the terminal newline.
- In local-auth mode, protected API and original-media routes require an
  authenticated session. Anonymous access is limited to health/version,
  diagnostics needed for bootstrap, and auth login/session endpoints.
- Assets explicitly marked `public` in Cartolensia metadata are the exception:
  their public asset-detail payload and media original/preview endpoints may be
  read anonymously. Public marking is an administrator action and is reversible;
  unmarked assets stay private to authenticated users.

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

## Read-Only Query Safety

The advanced search workbench is not a general SQL console. It only accepts a single `SELECT` against curated `cartolensia_search_*` views. The backend rejects semicolons, comments, mutation keywords, session-control keywords, and raw table references before the query reaches PostgreSQL. Accepted statements are executed in a read-only transaction with a timeout and server-side row limit.

Model-planned queries must use the same guard. A local LLM may propose a query, but Cartolensia must validate the query through the read-only allowlist before execution. Remote LLM APIs are not used by default.

## Storage Safety

Original media is immutable in the implemented adapter.

- Storage mode defaults to `strict_read_only`.
- Filesystem writes, deletes, moves, and mkdir operations return explicit read-only errors.
- Path traversal and absolute paths are rejected.
- Recursive discovery skips symlinks.
- Opening a symlink that escapes the root is rejected.
- Original media is streamed only through the registry.

Preview files are generated only under Cartolensia cache/work directories. Preview cache cleanup verifies deletion targets stay inside the cache root.

Metadata enrichment reads originals through the configured storage adapter and writes only Cartolensia metadata/database rows. Storage/prefix-scoped enrichment pages through bounded asset queries and must not flatten a large archive into memory. Optional storage unavailability is a health state, not a delete/missing-marking signal.

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

Optional storage roots may report `missing` or `error` when a NAS, SMB/CIFS,
NFS, or mounted object-storage view is offline. This health state is diagnostic
only. It must not trigger metadata deletion, missing-file marking, cache purge,
or album/search removal by itself.

Universal search uses PostgreSQL/local metadata by default. Place-name matching reads the durable Cartolensia `place_cache`. Reverse geocoding is cache-first and online cache-fill is enabled by default for missing coordinates; provider calls are rate-limited and stored locally before reuse.

OCR is a manual AI job/action. OCR text and bounding boxes are metadata records and must not write temporary inputs, OCR cache files, or derived text sidecars into original storage. Missing OCR engines/language packs must be reported as job/worker errors rather than silently falling back to remote services.

Settings DB exports are metadata/config JSON files written under the configured Cartolensia cache export directory. Import planning is validation-only and does not perform destructive restore while the app is live.

## HTTP/TLS

Plain HTTP is the default and should be bound to localhost for development. HTTPS can use configured certificate/key files or an in-memory self-signed certificate via `http.tls_auto_self_signed`. Self-signed TLS is intended for private/local deployments and does not replace a real certificate for exposed services.

## AI And Dependency Provenance

AI/vector APIs are explicit, local, and bounded. The backend does not download models by itself and does not use remote inference APIs. When an operator installs the approved Python dependencies and model weights, the optional sidecar can run local torchvision classification, OpenCV YuNet face detection, Falconsai safety classification, OpenCLIP embeddings, and BLIP captioning through user-triggered jobs. AI outputs are predictions, not truth, and are stored as Cartolensia metadata only.

Model caches, worker scratch space, generated predictions, and exports must stay under `.cartolensia/models`, `.cartolensia/exports`, or another configured non-archive path. They must never be placed under `/mnt/Models/rclone`.

AI/GPU status distinguishes native workers from Docker profiles. A configured native CUDA sidecar can be active while an optional Docker `ai-nvidia` profile remains not configured. Docker GPU probes must be explicit and supervised because they may require image pulls and host GPU access.

Do not copy third-party source into the repository. Add dependencies only through normal package managers and document why they are needed.

## Production And Air-Gapped Deployments

Production deployments must mount originals at `/originals` and keep that mount read-only.

- `/originals` is the archive root, not a cache location.
- cache, model, component, export, and runtime paths must live outside `/originals`.
- `dev_no_auth` is development-only and should not be the production default.
- offline component imports must be reviewed and extracted only under Cartolensia-managed component directories.
- release builds should not silently assume Internet access for fonts, map tiles, OCR language packs, ffmpeg, PostgreSQL tools, Python dependencies, or model weights.

The production templates and offline release bundle are designed to make missing optional features explicit instead of silently downloading them.

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
- Place-cache editing changes Cartolensia metadata only. It does not write to media files and does not start discovery.
- Local place search uses durable cache entries plus existing geotag metadata. Online reverse geocoding is enabled by default for cache misses, rate-limited, and persisted to local `place_cache`; use self-hosted/operator-approved providers for broad production enrichment.

## 2026-06-08 Distribution Safety Notes

- Offline package builds write to repo-local `dist/` by default and must not stage archives, source snapshots, tools, AI environments, model weights, or generated runtime files under original media storage.
- The generated offline configs default to a package-local `media/` directory with `strict_read_only` mode.
- Bundled model weights are opt-in and require a license/provenance-reviewed model cache. The workflow default keeps model bundling disabled.
- Bundled ffmpeg, Tesseract, PostgreSQL, Python packages, and CUDA wheels must be reviewed before public redistribution. The packager records manifests and Debian copyright files where available, but those records do not replace legal review.
- GPU drivers are intentionally not bundled. CUDA-capable packages still depend on compatible host drivers and must not attempt to install kernel or system GPU components on the target host.
- GitHub release builds are manual. Release operators should use prerelease mode until the package has been tested on a clean offline host.

## Component Manager Security

- Component records are metadata only until an operator explicitly checks, provides, imports, enables, or disables a component.
- Component imports are restricted to `.cartolensia/components/<component-key>`. Archives with absolute paths, `..` traversal, symlinks, hardlinks, or unsupported entry types are rejected.
- Component paths and archive sources under `/mnt/Models/rclone` are rejected. The real archive remains strict read-only and is never used as a component cache or extraction target.
- Component download jobs are provenance-gated. The current handler records a job and actionable message rather than silently downloading binaries without a reviewed source URL/license note.
- FFmpeg redistributability is checked during offline package assembly. `--enable-nonfree` fails packaging by default, while `--enable-gpl` is recorded so release operators can label the package appropriately.

## Multimodal Metadata Safety

Audio, transcript, video-frame, and document metadata follow the same immutable-original policy as photos and tracks.

- Audio discovery must be bounded for real archive storage and must not use missing-file marking.
- FFprobe reads original audio/video files through the configured read-only adapter and writes only database metadata.
- `audio_features` rows are Cartolensia metadata only; they must not create sidecars next to originals.
- ASR transcripts and segments are database metadata. ASR temp files, extracted audio, and model caches must stay under `.cartolensia/realpeek-cache`, `.cartolensia/models`, `.cartolensia/ai-venv`, `.cartolensia/components`, or `/tmp`.
- Document OCR/Markdown output is stored in PostgreSQL/cache metadata and must not be written beside source PDFs/images.
- OCR full-text `.txt` downloads in the browser are user-initiated client downloads, not server writes to storage.
- Optional ASR/Marker/video/genre model downloads require component provenance and license review before bundling or release.

The real-peek archive at `/mnt/Models/rclone` remains strict read-only. Do not use it as an ASR temp directory, OCR cache, document export location, waveform cache, model cache, or component extraction target.

## 2026-06-08 ASR And Audio Analysis Safety Notes

- `faster-whisper`, CTranslate2, librosa, SoundFile, and PyMuPDF were installed into the repo-local `.cartolensia/ai-venv`; they are not vendored into source and must be reviewed before redistribution.
- `faster-whisper-small` was cached under `.cartolensia/models/faster-whisper`; ASR model weights must stay component-managed and out of original media storage.
- `/transcribe-audio` and `/analyze-audio` accept only bounded Cartolensia media URLs or safe local temp/cache paths. Temporary media copies are deleted after sidecar processing.
- ASR transcripts, timestamped segments, and audio features are PostgreSQL metadata only. They do not create sidecars and do not modify media files.
- Audio-analysis genre labels are heuristic until a reviewed classifier model is added; UI/reporting should avoid presenting them as authoritative identity or safety decisions.

## 2026-06-09 Full Prefix Indexing Safety Notes

- `max_files=-1` is accepted only as an explicit unlimited sentinel for normal indexing jobs. Real archive storage still requires a named storage and explicit non-root prefix.
- Dry-run/preview caps are intentionally retained and labeled as preview-only so operators can inspect a sample without accidentally starting a full scan.
- The full `Cartolensia-photos` run used `rclone_peek` in `strict_read_only` mode and did not run missing-file marking.
- Track arrows and reverse-geocoding changes are metadata/UI operations only. They do not modify tracks, photos, or original geotags.
- `/api/v1/places/reverse` is cache-first. Missing coordinates use the configured online provider when runtime setting `search.online_geocoding=true`; automatic provider results are cached locally and broad public-provider bulk enrichment remains disallowed.
- Component downloads remain provenance-gated. Even with operator approval, the Component Manager refuses silent downloads without a reviewed source URL and records an actionable failed job instead.

## 2026-06-09 Context/Search Safety Notes

- Timestamp candidates are metadata interpretations only. They do not rewrite EXIF, change file mtimes, or modify original media.
- Track media matching now uses timestamp candidates and geotag proximity, but it still only returns metadata associations from PostgreSQL/local state.
- Asset related/context endpoints are bounded local queries. They do not trigger discovery, hashing, AI inference, geocoding, transcodes, or writes.
- Audio previews in Explorer/Search/Gallery stream originals through the existing read-only media endpoint; they do not create sidecars or waveform files under original storage.
- Search wildcard and field-token support is executed through the existing PostgreSQL/local backend and bounded paging. It does not enable shell-style filesystem globbing against storage roots.
- Middle-click/new-tab support uses browser anchors for navigation only. It does not change authorization or storage access rules.

## 2026-06-27 Production HTTPS And AI Media Security Notes

- Production deployments should serve the UI/API over HTTPS. Self-signed certificates are acceptable for private LAN deployments when operators explicitly accept the browser warning.
- Local-auth session cookies are marked `Secure` in production templates. Operators must use the HTTPS URL for login and media playback.
- Anonymous users may access only explicitly public routes and explicitly public media. Protected API routes and original media endpoints require authentication.
- AI sidecars must not receive reusable user cookies. Cartolensia provides a narrow loopback-only AI media endpoint guarded by `CARTOLENSIA_AI_MEDIA_TOKEN`; it only serves `GET`/`HEAD` original media to loopback callers with the exact token.
- Never expose the AI media token, admin password file, SMB credentials, PostgreSQL password, or session cookies in logs or reports.
- The AI sidecar reads media through Cartolensia and writes only metadata to PostgreSQL. It must not write temp files, models, OCR cache, ASR cache, or components under originals/Samba storage.
- pgvector stores embeddings in PostgreSQL metadata. It does not copy or modify original files.
- Large archive backfill should be low-concurrency and missing-work based. Avoid broad AI jobs that would repeatedly reread the same originals or create unbounded write amplification.
- Explorer pagination and folder aggregation are metadata-only PostgreSQL reads. They must not trigger discovery, hashing, missing-file marking, AI inference, or storage writes.
- Optional NAS/original storage unavailability is a health state only. Cartolensia must not delete metadata, previews, embeddings, OCR, transcripts, captions, or DB rows because a read-only original mount is temporarily missing.
- Full archive hashing is read-only but high-impact; treat it as an explicit operator maintenance action because it can read many terabytes from NAS storage and compete with interactive use.

## 2026-06-27 Backup And Overlapping Storage Safety

Overlapping storage roots are allowed only as read-only metadata sources. Discovery automatically prunes nested child roots from parent scans to avoid duplicate records, but this is not a destructive deduplication operation. It does not delete old metadata, does not modify files, and does not infer that an unavailable storage means files are gone.

Operational constraints:

- all Samba/NFS/original mounts must be mounted read-only for production originals;
- `strict_read_only` remains the default storage mode;
- missing-file marking is disabled for NAS/original refreshes;
- exports and backups are written only to configured Cartolensia data/export directories, never to originals;
- essential `.7z` exports contain a PostgreSQL dump and should be treated as sensitive application data;
- redacted config exports must not include database passwords, admin password file contents, API tokens, SMB credentials, or local secret files.

The `create-essential-export.sh` helper intentionally excludes originals, thumbnails, model caches, component caches, and secret files. Operators who need a full disaster-recovery bundle must separately back up secrets and external original storage according to their own access-control policy.

## 2026-06-27 Samba Credential And Outage Diagnostics

SMB/CIFS diagnostics are designed to improve operator feedback without weakening the originals safety model.

Security rules:

- Store Samba passwords in root-owned credentials files or service environment files, not in source control, WebUI text fields, reports, or logs.
- A credentials file used by Cartolensia diagnostics may be `root:cartolensia` with mode `0640`; it must not be world-readable.
- The WebUI stores and displays only non-secret metadata such as host, share, subpath, credentials-file path, or password environment-variable name.
- `smbclient` probes use `-A <credentials_file>` so the password is not placed on the process command line.
- Probe output is redacted for password-looking lines before it is returned in API details.
- Optional original-storage outages must not trigger deletion, missing-file marking, or metadata cleanup.

When an original-media request fails, Cartolensia returns a structured error that separates:

- host offline/unresolved;
- share/export unavailable;
- credentials rejected;
- credentials file unreadable;
- local mount unavailable;
- original file missing while storage is otherwise readable.

These diagnostics are metadata about storage reachability only. They do not grant write access, remount shares, alter credentials, or modify originals.

## 2026-06-28 Knowledge Base / Knowledge Graph Safety

The Knowledge Base and Knowledge Graph are derived metadata. They must follow the same originals safety model as OCR, captions, transcripts, embeddings, and previews:

- extraction reads only PostgreSQL/local metadata and does not modify originals;
- facts and relations can contain AI/OCR/ASR errors and must not be treated as verified ground truth;
- conversation records and tool-call traces can reveal private archive metadata and require normal authentication;
- unauthenticated users must not access non-public KB/KG APIs;
- local LLM planners, when enabled, must not call remote APIs by default;
- local LLM tool requests are advisory only; Cartolensia validates and executes only bounded media search, knowledge fact/relation search, and read-only SQL against `cartolensia_search_*` views;
- concrete find/list/count media answers are rendered from backend-verified
  tool results; local LLM synthesis cannot replace them with ungrounded prose;
- model-generated SQL must be executed only through the existing single-`SELECT`, read-only, allowlisted `cartolensia_search_*` view runner;
- streaming chat uses authenticated SSE and exposes only server-approved tool events plus compact citations/actions; it does not expose database credentials or raw table access;
- chat attachments are bounded and sent only to the authenticated Cartolensia server and configured local LLM endpoint. Text-like attachments are prompt context; image attachments require a local vision-capable model and otherwise degrade to filename/text context;
- KB/KG exports should be treated as sensitive metadata because they can summarize private files without containing the original media.
