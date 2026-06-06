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
2. Copy and review `config/rclone-dryrun.example.yaml`; do not make it the default config.
3. Confirm the storage name is `rclone_dryrun`, mode is `strict_read_only`, and cache dir is outside the archive.
4. Start the app with that config only during a supervised session.
5. Use one explicitly chosen non-empty prefix and `max_files <= 50`.
6. Run the guarded preflight script only after setting all required flags:

```bash
CARTOLENSIA_ALLOW_RCLONE_DRY_RUN=1 \
CARTOLENSIA_RCLONE_DRY_RUN_PREFIX='some/non-empty/prefix' \
CARTOLENSIA_EXECUTE_RCLONE_DRY_RUN=1 \
bash scripts/rclone-dry-run-preflight.sh
```

7. Inspect `/api/v1/storages`, `/api/v1/jobs`, `/api/v1/discovery/dry-run/{job_id}/report`, `/api/v1/assets`, `/api/v1/explorer`, and `/api/v1/stats`.
8. Cancel the job if anything looks unexpected.
9. Run hashing only on a very small selected subset.
10. Run metadata enrichment only on a small selected subset.
11. Verify preview cache files are under Cartolensia cache/work directory, never beside originals.
12. Increase scope gradually only after each bounded run is reviewed.

## Stop Conditions

Stop immediately if:

- any command attempts to write into original storage;
- traversal/read-only errors appear repeatedly;
- job progress counters do not match the bounded scan scope;
- cache files appear under the archive root;
- missing-file marking affects paths outside the intended scope.

## Current Behavior

The implemented dry-run endpoint is report-only: it records a job and scan-run report, counts files that would be considered, and does not index assets. Normal discovery updates `last_seen_at` for discovered files, but automatic scoped missing-file marking is still intentionally deferred. A bounded real archive dry run must avoid any missing/deletion workflow until rescan semantics are implemented and tested.

## Explicitly Not Done In This Run

`/mnt/Models/rclone` was not read, listed, scanned, mounted, written, or probed. All tests used `testdata/media_fixture/`, temporary synthetic files, and Docker PostgreSQL where available.
