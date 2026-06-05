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
- `..`, absolute paths, malformed URLs, and incompatible schemes are rejected.
- Symlink behavior must be explicit before enabling real user archives. MVP tests should cover traversal prevention and document any unresolved symlink policy.
- Paths are stored using slash-separated relative paths.

## Current Fixture

The only intended storage for implementation testing is:

- storage name: `fixture`;
- URL prefix: `fs://fixture/`;
- root: `testdata/media_fixture/`;
- mode: `strict_read_only`.

Do not access `/mnt/Models/rclone` during the MVP preparation or the first unattended implementation run.
