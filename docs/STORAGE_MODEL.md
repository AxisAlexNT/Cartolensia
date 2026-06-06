# Storage Model

Cartolensia treats storage backends as adapters that expose immutable originals through universal URLs. The MVP implements only local filesystem storage in strict read-only mode.

## Universal URLs

Examples:

- `fs://fixture/photos/2024-trip/photo_001.jpg`
- `smb://nas-share/path/photo.jpg`
- `s3://bucket/key`

URL parts:

- scheme: storage adapter kind, such as `fs`;
- authority: configured storage name;
- path: adapter-relative path normalized with forward slashes.

The database should store normalized URLs and separate storage location fields so backend-specific paths can change without losing logical identity.

## Identity

Use three separate concepts:

- asset: logical media item known to Cartolensia;
- storage location: one concrete URL where bytes currently exist;
- content: byte identity once SHA-512 is known.

SHA-512 is an indexed content signal. It is not the sole primary key and should not be available during the first discovery pass. The current content identity uniqueness rule is SHA-512 plus file size, leaving room for future collision handling and alternate identity providers. When a moved/copied file later hashes to the same SHA-512 and size as known content, Cartolensia relinks the new storage location to the existing logical asset rather than keeping a duplicate logical asset.

## Discovery Stages

Stage 1: fast scan.

- storage name and URL;
- relative path and file name;
- extension and MIME guess;
- size and mtime;
- coarse media kind;
- fixture text hints when available and safe.

Stage 2: lazy hash.

- streaming SHA-512;
- no whole-file buffering;
- priority for selected assets, likely duplicates, and integrity checks.

Stage 3: explicit metadata enrichment.

- image dimensions through Go decoders where supported;
- server-side JPEG EXIF parsing where available, including camera metadata and GPS coordinates;
- video metadata through optional ffprobe;
- GPX point ingest, bbox, distance, duration, elevation, and time span;
- additive metadata JSON patches only.

Stage 4: optional preview generation.

- cache under Cartolensia cache/work directory only;
- no sidecars and no writes near originals;
- cache keys derived from asset/content IDs and requested dimensions;
- persistent `preview_cache_entries` records track generated/unsupported/failed statuses;
- unsupported formats return clean statuses.

Stage 5: scoped dry-run readiness.

- dry-run requests require a storage name and non-empty prefixes;
- default max files is 50 and default max bytes is 2 GiB;
- dry-run reports never mark missing files;
- current dry-run behavior is report-only and does not insert assets;
- hash, metadata, and previews are off by default and must be requested separately in future supervised runs.

Real archive storage roots such as `/mnt/Models/rclone` get stricter guards for normal indexing too: no `storage=all`, no empty/root prefixes, no missing max limits, and no unsafe absolute prefixes. WebUI/API callers should send adapter-relative prefixes such as `Cartolensia-photos`, not host absolute paths.

## Safety Modes

MVP mode:

- `strict_read_only`: list, stat, and read only. Any modifying operation returns an explicit unsupported error.

Future modes:

- `journaled_deferred`: record requested destructive operations for later human review, then execute only after confirmation.
- `full_access`: allow direct modifying operations where configured.

No future mode may bypass adapter-level safety checks. Delete-like operations should move to a configured trash target when available.

## Filesystem Adapter Rules

- Configured roots are absolute after config load.
- Adapter-relative paths must never escape the root.
- `..` segments are rejected before path cleaning, including URL-encoded traversal attempts.
- Absolute paths, malformed URLs, and incompatible schemes are rejected.
- Recursive discovery skips symlink entries. Opening a symlink that resolves outside the configured root is rejected.
- Paths are stored using slash-separated relative paths.

## Current Fixture

The only intended storage for implementation testing is:

- storage name: `fixture`;
- URL prefix: `fs://fixture/`;
- root: `testdata/media_fixture/`;
- mode: `strict_read_only`.

Do not access `/mnt/Models/rclone` during the MVP preparation or the first unattended implementation run.

## Implemented MVP Behavior

- `internal/storage` parses and normalizes `fs://<storage>/<relative-path>` URLs.
- Relative paths are slash-normalized and must stay inside the configured root.
- Empty paths, absolute paths, `..`, and traversal attempts are rejected.
- Recursive discovery skips symlink entries and non-file directories.
- File metadata includes URL, relative path, file name, extension, MIME, size, mtime, and media kind.
- Write, delete, move, and mkdir operations return a read-only error.
- Original streaming opens files only through the registry and serves them read-only with HTTP Range support through the Go HTTP stack.
- On-demand preview cache paths are derived from asset/content IDs and live under the Cartolensia cache directory. Preview code verifies generated cache paths stay inside that directory and must never create files next to originals.
- Built-in preview generation supports decodable image formats provided by Go's standard image decoders and writes JPEG cache files. Unsupported formats such as dummy text fixtures and HEIC return a clean unsupported response.
- Preview cache cleanup only walks the `previews/` subtree under the configured cache root and verifies each deletion target stays inside that root.
- Metadata/preview/hash jobs check cancellation between files.
- Bounded dry-run walking uses adapter-relative prefixes and stops when max-files or max-bytes limits are reached.

## Synthetic Scale Fixtures

`scripts/generate-synthetic-fixture.sh` creates configurable dummy directory trees for scale testing. By default it writes under ignored `testdata/synthetic_media/`; for unattended or review-friendly runs, prefer an explicit temporary root:

```bash
CARTOLENSIA_SYNTHETIC_ROOT=/tmp/cartolensia_synthetic_media bash scripts/generate-synthetic-fixture.sh
```

`scripts/perf-smoke.sh` can run a bounded discovery/hash smoke against that tree. The generated files are text/image-like/GPX/MP4-like dummy files, not real media.

## Missing Files And Rescan Policy

`asset_locations.last_seen_at` and `missing_at` exist in the schema. Discovery updates `last_seen_at` for seen URLs. Automatic missing marking is intentionally deferred until scoped rescan semantics are fully implemented. Future missing-file marking must be limited to an explicitly scanned storage/prefix scope so a small bounded scan cannot mark an unrelated archive subtree missing.

## Non-Media Files

Discovery scans storage roots but indexes only media-like kinds for the MVP:

- photos/images;
- videos;
- GPS/track files.

Fixture helper files such as README and manifest JSON are intentionally skipped.
