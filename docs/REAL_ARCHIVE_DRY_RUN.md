# Real Archive Dry Run

This document is a future supervised procedure. It was not executed during the unattended implementation run.

Do not run these steps without an explicit approval turn.

## Preconditions

- Confirm the archive root is read-only from the OS perspective if possible.
- Confirm Cartolensia storage config uses `strict_read_only`.
- Confirm auth is enabled if the server is reachable from anything beyond localhost.
- Confirm the cache/work directory is outside the original archive.
- Confirm backups or an independent file listing exist if the archive is valuable.

## Suggested Future Procedure

1. Start PostgreSQL and the app.
2. Add a temporary storage config for the real archive with `strict_read_only`.
3. Run a tiny bounded scan, for example one explicitly chosen prefix and `max_files` once scoped scan limits are implemented.
4. Inspect `/api/v1/storages`, `/api/v1/jobs`, `/api/v1/assets`, `/api/v1/explorer`, and `/api/v1/stats`.
5. Cancel the job if anything looks unexpected.
6. Run hashing only on a very small selected subset.
7. Run metadata enrichment only on a small selected subset.
8. Verify preview cache files are under Cartolensia cache/work directory, never beside originals.
9. Increase scope gradually only after each bounded run is reviewed.

## Stop Conditions

Stop immediately if:

- any command attempts to write into original storage;
- traversal/read-only errors appear repeatedly;
- job progress counters do not match the bounded scan scope;
- cache files appear under the archive root;
- missing-file marking affects paths outside the intended scope.

## Current Limitation

The current implementation updates `last_seen_at` for discovered files but does not yet implement automatic scoped missing-file marking. That is intentional. A bounded real archive dry run should avoid any missing/deletion workflow until rescan semantics are implemented and tested.

## Explicitly Not Done In This Run

`/mnt/Models/rclone` was not read, listed, scanned, mounted, written, or probed. All tests used `testdata/media_fixture/`, temporary synthetic files, and Docker PostgreSQL where available.
