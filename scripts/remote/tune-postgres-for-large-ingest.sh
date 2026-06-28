#!/usr/bin/env bash
set -euo pipefail

# Tune Cartolensia's PostgreSQL for large read-only archive indexing.
# This script only changes PostgreSQL runtime configuration; it does not reset
# the database and does not touch original media.

DATABASE_URL="${CARTOLENSIA_DATABASE_URL:-postgres://cartolensia:cartolensia@127.0.0.1:15432/cartolensia?sslmode=disable}"
PSQL="${CARTOLENSIA_PSQL:-psql}"

"${PSQL}" "${DATABASE_URL}" -v ON_ERROR_STOP=1 <<'SQL'
alter system set wal_compression = 'on';
alter system set checkpoint_timeout = '15min';
alter system set checkpoint_completion_target = '0.9';
alter system set max_wal_size = '8GB';
alter system set effective_io_concurrency = '200';
alter system set random_page_cost = '1.1';
alter system set maintenance_work_mem = '512MB';
alter system set autovacuum_vacuum_scale_factor = '0.05';
alter system set autovacuum_analyze_scale_factor = '0.02';
select pg_reload_conf();
SQL

"${PSQL}" "${DATABASE_URL}" -v ON_ERROR_STOP=1 -P pager=off <<'SQL'
select name, setting, pending_restart
from pg_settings
where name in (
  'wal_compression',
  'checkpoint_timeout',
  'checkpoint_completion_target',
  'max_wal_size',
  'effective_io_concurrency',
  'random_page_cost',
  'maintenance_work_mem',
  'autovacuum_vacuum_scale_factor',
  'autovacuum_analyze_scale_factor'
)
order by name;
SQL

cat <<'NOTE'
Cartolensia PostgreSQL large-ingest tuning applied.
These settings reduce WAL/checkpoint write amplification during discovery,
metadata extraction, GPS track parsing, AI metadata writes, and render-cache
refreshes. They do not disable fsync or synchronous_commit.
NOTE
