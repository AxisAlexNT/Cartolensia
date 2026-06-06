create table if not exists schema_migrations (
    version text primary key,
    checksum text not null,
    applied_at timestamptz not null default now()
);

create table if not exists config_snapshots (
    id uuid primary key,
    source text not null,
    effective_config jsonb not null,
    created_at timestamptz not null default now()
);

create table if not exists storage_backends (
    id uuid primary key,
    name text unique not null,
    kind text not null,
    root text not null,
    mode text not null,
    config_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists contents (
    id uuid primary key,
    sha512 bytea unique not null,
    size_bytes bigint not null,
    first_hashed_at timestamptz not null default now(),
    collision_group_id uuid null
);

create table if not exists assets (
    id uuid primary key,
    media_kind text not null,
    display_name text not null,
    first_seen_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    taken_at timestamptz null,
    metadata_json jsonb not null default '{}'::jsonb
);

create table if not exists asset_locations (
    id uuid primary key,
    asset_id uuid not null references assets(id) on delete cascade,
    storage_id uuid not null references storage_backends(id),
    url text unique not null,
    relative_path text not null,
    file_name text not null,
    extension text not null,
    mime_type text not null,
    media_kind text not null,
    size_bytes bigint not null,
    mtime timestamptz not null,
    content_id uuid null references contents(id),
    hash_status text not null default 'unhashed',
    last_seen_at timestamptz not null default now(),
    missing_at timestamptz null
);

create table if not exists plugins (
    id text primary key,
    name text not null,
    version text not null,
    enabled boolean not null default true,
    runtime text not null default 'builtin',
    status text not null default 'stub',
    manifest_json jsonb not null,
    loaded_at timestamptz not null default now()
);

create table if not exists jobs (
    id uuid primary key,
    kind text not null,
    status text not null,
    payload_json jsonb not null default '{}'::jsonb,
    counters_json jsonb not null default '{}'::jsonb,
    progress_current bigint not null default 0,
    progress_total bigint null,
    attempts int not null default 0,
    max_attempts int not null default 3,
    worker_id text null,
    lease_expires_at timestamptz null,
    cancel_requested_at timestamptz null,
    created_at timestamptz not null default now(),
    started_at timestamptz null,
    finished_at timestamptz null,
    error text null
);

create table if not exists job_logs (
    id bigserial primary key,
    job_id uuid not null references jobs(id) on delete cascade,
    level text not null,
    message text not null,
    created_at timestamptz not null default now()
);

create table if not exists track_points (
    id bigserial primary key,
    track_asset_id uuid null references assets(id) on delete cascade,
    recorded_at timestamptz not null,
    lat double precision not null,
    lon double precision not null,
    elevation_m double precision null,
    speed_mps double precision null,
    source text not null default 'gpx'
);

create table if not exists asset_track_links (
    id uuid primary key,
    asset_id uuid not null references assets(id) on delete cascade,
    track_asset_id uuid not null references assets(id) on delete cascade,
    match_status text not null default 'candidate',
    overlap_start timestamptz null,
    overlap_end timestamptz null,
    time_offset_ms bigint not null default 0,
    confidence double precision null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique(asset_id, track_asset_id)
);

create index if not exists idx_asset_locations_storage_path on asset_locations(storage_id, relative_path);
create index if not exists idx_asset_locations_asset on asset_locations(asset_id);
create index if not exists idx_asset_locations_content on asset_locations(content_id);
create index if not exists idx_asset_locations_seen on asset_locations(last_seen_at);
create index if not exists idx_assets_kind on assets(media_kind);
create index if not exists idx_assets_taken_at on assets(taken_at);
create index if not exists idx_jobs_status_kind_created on jobs(status, kind, created_at);
create index if not exists idx_jobs_running_lease on jobs(lease_expires_at) where status = 'running';
create index if not exists idx_job_logs_job_created on job_logs(job_id, created_at desc);
create index if not exists idx_track_points_track_time on track_points(track_asset_id, recorded_at);
create index if not exists idx_asset_track_links_asset on asset_track_links(asset_id);
