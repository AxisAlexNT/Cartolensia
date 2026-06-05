# Database Schema

This is the target schema for the MVP implementation. Names can change during implementation if tests and migrations document the change.

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

`asset_locations`

- `id uuid primary key`
- `asset_id uuid not null references assets(id)`
- `storage_id uuid not null references storage_backends(id)`
- `url text unique not null`
- `relative_path text not null`
- `file_name text not null`
- `extension text not null`
- `mime_type text not null`
- `size_bytes bigint not null`
- `mtime timestamptz not null`
- `content_id uuid null references contents(id)`
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
- `progress_current bigint not null default 0`
- `progress_total bigint null`
- `attempts int not null default 0`
- `max_attempts int not null default 3`
- `worker_id text null`
- `lease_expires_at timestamptz null`
- `cancel_requested_at timestamptz null`
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
- `manifest_json jsonb not null`
- `loaded_at timestamptz not null default now()`

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
- `jobs(lease_expires_at) where status = 'running'`
- `job_logs(job_id, created_at desc)`

Future geospatial indexes:

- `assets` or `asset_locations` geometry/geography columns when PostGIS is available.
- Separate track tables and simplified geometry tables for map rendering.

## Migration Policy

- Migrations live in `migrations/`.
- Migrations must be deterministic and idempotence-tested through the migration runner.
- Do not use destructive migrations without an explicit rollback or backup strategy.
- Extension creation should use `CREATE EXTENSION IF NOT EXISTS` only for optional capabilities and tolerate absence where PostgreSQL permissions do not allow installation.
