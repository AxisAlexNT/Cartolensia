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

SHA-512 is an indexed content key. It is not the sole primary key and should not be available during the first discovery pass.

## Discovery Stages

Stage 1: fast scan.

- storage name and URL;
- relative path and file name;
- extension and MIME guess;
- size and mtime;
- coarse media kind;
- cheap metadata when available and safe.

Stage 2: lazy hash.

- streaming SHA-512;
- no whole-file buffering;
- priority for selected assets, likely duplicates, and integrity checks.

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

## Non-Media Files

Discovery scans storage roots but indexes only media-like kinds for the MVP:

- photos/images;
- videos;
- GPS/track files.

Fixture helper files such as README and manifest JSON are intentionally skipped.
