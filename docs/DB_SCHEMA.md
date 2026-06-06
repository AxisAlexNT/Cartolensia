# Database Schema

This describes the implemented schema as of the current Phase 2/3 foundation. Migrations live under `migrations/` and are embedded into the Go binary.

## Extensions

Required:

- `pgcrypto` when available for UUID generation, otherwise generate UUIDs in Go.

Optional:

- `postgis` for geospatial fields and indexes.
- `vector` for future embeddings through pgvector.
- `pg_trgm` for text search acceleration.

The MVP must start without optional extensions.

## Core Tables

`schema_migrations`

- `version text primary key`
- `applied_at timestamptz not null default now()`
- `checksum text not null`

`app_settings`

- `key text primary key`
- `value_json jsonb not null`
- `updated_at timestamptz not null default now()`

`config_snapshots`

- `id uuid primary key`
- `source text not null`
- `effective_config jsonb not null`
- `created_at timestamptz not null default now()`

`storage_backends`

- `id uuid primary key`
- `name text unique not null`
- `kind text not null`
- `root text not null`
- `mode text not null`
- `config_json jsonb not null default '{}'::jsonb`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

`assets`

- `id uuid primary key`
- `media_kind text not null`
- `display_name text not null`
- `first_seen_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`
- `taken_at timestamptz null`
- `metadata_json jsonb not null default '{}'::jsonb`

Current metadata keys used by the MVP are optional and additive:

- `duration_seconds`
- `width`
- `height`
- `codec`
- `container`
- `bitrate_bps`
- `frame_rate`
- `ffprobe_available`
- `track_point_count`
- `track_start_at`
- `track_end_at`
- `distance_m`
- `elevation_min_m`
- `elevation_max_m`
- `min_lat`
- `min_lon`
- `max_lat`
- `max_lon`
- `lat`
- `lon`
- `metadata_extracted_at`

`asset_locations`

- `id uuid primary key`
- `asset_id uuid not null references assets(id)`
- `storage_id uuid not null references storage_backends(id)`
- `url text unique not null`
- `relative_path text not null`
- `file_name text not null`
- `extension text not null`
- `mime_type text not null`
- `media_kind text not null`
- `size_bytes bigint not null`
- `mtime timestamptz not null`
- `content_id uuid null references contents(id)`
- `hash_status text not null default 'unhashed'`
- `last_seen_at timestamptz not null default now()`
- `missing_at timestamptz null`

`contents`

- `id uuid primary key`
- `sha512 bytea unique not null`
- `size_bytes bigint not null`
- `first_hashed_at timestamptz not null default now()`
- `collision_group_id uuid null`

`jobs`

- `id uuid primary key`
- `kind text not null`
- `status text not null`
- `payload_json jsonb not null default '{}'::jsonb`
- `counters_json jsonb not null default '{}'::jsonb`
- `progress_current bigint not null default 0`
- `progress_total bigint null`
- `attempts int not null default 0`
- `max_attempts int not null default 3`
- `worker_id text null`
- `lease_expires_at timestamptz null`
- `cancel_requested_at timestamptz null`
- `next_run_at timestamptz null`
- `created_at timestamptz not null default now()`
- `started_at timestamptz null`
- `finished_at timestamptz null`
- `error text null`

`job_logs`

- `id bigserial primary key`
- `job_id uuid not null references jobs(id) on delete cascade`
- `level text not null`
- `message text not null`
- `created_at timestamptz not null default now()`

`plugins`

- `id text primary key`
- `name text not null`
- `version text not null`
- `enabled boolean not null default true`
- `runtime text not null default 'builtin'`
- `status text not null default 'stub'`
- `manifest_json jsonb not null`
- `loaded_at timestamptz not null default now()`

Plugin `manifest_json` stores built-in and filesystem manifest fields, including future sidecar HTTP contract fields such as runtime, capabilities, permissions, base URL, and health path.

`users`

- `id uuid primary key`
- `email text unique not null`
- `display_name text not null`
- `password_hash text null`
- `role text not null default 'admin'`
- `disabled_at timestamptz null`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

`sessions`

- `id uuid primary key`
- `user_id uuid not null references users(id) on delete cascade`
- `token_hash bytea unique not null`
- `expires_at timestamptz not null`
- `created_at timestamptz not null default now()`
- `last_seen_at timestamptz null`

`api_tokens`

- `id uuid primary key`
- `user_id uuid not null references users(id) on delete cascade`
- `name text not null`
- `token_hash bytea unique not null`
- `scopes text[] not null default '{}'`
- `expires_at timestamptz null`
- `created_at timestamptz not null default now()`
- `last_used_at timestamptz null`
- `revoked_at timestamptz null`

`track_points`

- `id bigserial primary key`
- `track_asset_id uuid null references assets(id) on delete cascade`
- `recorded_at timestamptz not null`
- `lat double precision not null`
- `lon double precision not null`
- `elevation_m double precision null`
- `speed_mps double precision null`
- `source text not null default 'gpx'`

`asset_track_links`

- `id uuid primary key`
- `asset_id uuid not null references assets(id) on delete cascade`
- `track_asset_id uuid not null references assets(id) on delete cascade`
- `match_status text not null default 'candidate'`
- `overlap_start timestamptz null`
- `overlap_end timestamptz null`
- `time_offset_ms bigint not null default 0`
- `confidence double precision null`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

The live video-track sync skeleton uses `assets.taken_at` plus `metadata_json.duration_seconds` to compute candidate overlaps. Manual links are stored in `asset_track_links` with `time_offset_ms`.

`albums`

- `id uuid primary key`
- `parent_id uuid null references albums(id) on delete cascade`
- `slug text not null`
- `title text not null`
- `description text not null default ''`
- `sort_order int not null default 0`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

Root album slugs are unique. Sibling slugs are unique for nested albums. Deleting an album deletes only album metadata and membership rows, never assets or storage locations.

`album_items`

- `album_id uuid not null references albums(id) on delete cascade`
- `asset_id uuid not null references assets(id) on delete cascade`
- `note text not null default ''`
- `sort_order int not null default 0`
- `added_at timestamptz not null default now()`
- primary key `(album_id, asset_id)`

`asset_geo`

- `asset_id uuid primary key references assets(id) on delete cascade`
- `lat double precision not null`
- `lon double precision not null`
- `source text not null`
- `confidence double precision null`
- `taken_at timestamptz null`
- `track_asset_id uuid null references assets(id) on delete set null`
- `metadata_json jsonb not null default '{}'::jsonb`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

Allowed sources are `real`, `exif`, `track_snapped`, `estimated`, `manual`, and `unknown`. The implementation does not create fake unknown coordinates. EXIF/real/manual sources are protected from lower-priority estimated or track-snapped updates unless a future force mode is explicit.

`gps_tracks`

- `track_asset_id uuid primary key references assets(id) on delete cascade`
- `title text not null`
- `description text not null default ''`
- `point_count int not null default 0`
- `start_at timestamptz null`
- `end_at timestamptz null`
- `min_lat double precision null`
- `min_lon double precision null`
- `max_lat double precision null`
- `max_lon double precision null`
- `distance_m double precision null`
- `duration_seconds double precision null`
- `elevation_min_m double precision null`
- `elevation_max_m double precision null`
- `metadata_json jsonb not null default '{}'::jsonb`
- `updated_at timestamptz not null default now()`

`scan_runs`

- `id uuid primary key`
- `job_id uuid null references jobs(id) on delete set null`
- `storage_name text not null`
- `mode text not null`
- `prefixes_json jsonb not null default '[]'::jsonb`
- `max_files int not null`
- `max_bytes bigint not null`
- `hash_requested boolean not null default false`
- `metadata_requested boolean not null default false`
- `previews_requested boolean not null default false`
- `mark_missing boolean not null default false`
- `dry_run boolean not null default true`
- `started_at timestamptz null`
- `finished_at timestamptz null`
- `report_json jsonb not null default '{}'::jsonb`
- `created_at timestamptz not null default now()`

Dry-run scan reports are report-only for asset indexing: the current dry-run worker records `files_would_index` and keeps `files_indexed` at zero.

`preview_cache_entries`

- `id uuid primary key`
- `asset_id uuid not null references assets(id) on delete cascade`
- `content_id uuid null references contents(id) on delete set null`
- `variant text not null`
- `width int not null`
- `height int not null`
- `format text not null`
- `cache_path text not null`
- `status text not null`
- `size_bytes bigint not null default 0`
- `created_at timestamptz not null default now()`
- `last_accessed_at timestamptz null`
- `error text null`
- unique `(asset_id, variant, width, height, format)`

`plugin_settings`

- `plugin_id text primary key references plugins(id) on delete cascade`
- `settings_json jsonb not null default '{}'::jsonb`
- `updated_at timestamptz not null default now()`

`embedding_models`

- `id text primary key`
- `modality text not null`
- `model_name text not null`
- `version text not null`
- `dimension int null`
- `plugin_id text null references plugins(id)`
- `metadata_json jsonb not null default '{}'::jsonb`
- `created_at timestamptz not null default now()`

`asset_embeddings`

- `id uuid primary key`
- `asset_id uuid not null references assets(id) on delete cascade`
- `model_id text not null references embedding_models(id)`
- `modality text not null`
- `source_ref text not null default 'asset'`
- `embedding_json jsonb not null default '{}'::jsonb`
- `metadata_json jsonb not null default '{}'::jsonb`
- `created_at timestamptz not null default now()`
- `unique(asset_id, model_id, modality, source_ref)`

This schema intentionally does not require pgvector. A future migration may add vector columns when the extension is available.

`transcoding_presets`

- `id text primary key`
- `name text not null`
- `config_json jsonb not null default '{}'::jsonb`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

`transcoding_outputs`

- `id uuid primary key`
- `source_asset_id uuid not null references assets(id) on delete cascade`
- `output_storage text not null`
- `output_url text null`
- `status text not null default 'planned'`
- `safety_policy text not null default 'no_original_writes'`
- `metadata_json jsonb not null default '{}'::jsonb`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

Transcoding output rows are contract placeholders. The current backend detects capabilities only and does not write transcoded files.

## Indexes

Initial indexes:

- `asset_locations(storage_id, relative_path)`
- `asset_locations(asset_id)`
- `asset_locations(content_id)`
- `asset_locations(last_seen_at)`
- `assets(media_kind)`
- `assets(taken_at)`
- `contents(sha512)`
- `jobs(status, kind, created_at)`
- `jobs(lease_expires_at) where status in ('running', 'cancel_requested')`
- `jobs(next_run_at, created_at) where status = 'queued'`
- `job_logs(job_id, created_at desc)`
- `sessions(user_id)`
- `api_tokens(user_id)`
- `track_points(track_asset_id, recorded_at)`
- `asset_track_links(asset_id)`
- `asset_embeddings(asset_id)`
- `asset_embeddings(model_id)`
- `transcoding_outputs(source_asset_id)`

Future geospatial indexes:

- `assets` or `asset_locations` geometry/geography columns when PostGIS is available.
- Separate track tables and simplified geometry tables for map rendering.

## Migration Policy

- Migrations live in `migrations/` and are embedded into the Go binary for runtime startup.
- Disk migration loading is available only when explicitly configured for development/testing.
- Migrations must be deterministic and idempotence-tested through the migration runner.
- Do not use destructive migrations without an explicit rollback or backup strategy.
- Extension creation should use `CREATE EXTENSION IF NOT EXISTS` only for optional capabilities and tolerate absence where PostgreSQL permissions do not allow installation.

Implemented migrations:

- `migrations/001_core.sql`: core metadata, assets, locations, contents, jobs, plugins, and GPS/video sync skeleton.
- `migrations/002_phase1_hardening.sql`: `app_settings`, queued-job scheduling with `next_run_at`, and lease indexes.
- `migrations/003_auth_foundation.sql`: users, sessions, and API tokens for local auth bootstrap.
- `migrations/004_job_lease_cancel_index.sql`: forward-only replacement of the lease-expiry index so it covers both `running` and `cancel_requested` jobs.
- `migrations/005_ai_transcoding_contracts.sql`: AI/vector contract tables and transcoding preset/output placeholders without requiring pgvector or a transcoding engine.
