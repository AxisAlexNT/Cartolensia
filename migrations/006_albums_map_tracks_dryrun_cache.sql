create table if not exists albums (
    id uuid primary key,
    parent_id uuid null references albums(id) on delete cascade,
    slug text not null,
    title text not null,
    description text not null default '',
    sort_order int not null default 0,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create unique index if not exists idx_albums_root_slug
    on albums(slug)
    where parent_id is null;

create unique index if not exists idx_albums_parent_slug
    on albums(parent_id, slug)
    where parent_id is not null;

create index if not exists idx_albums_parent_sort_title
    on albums(parent_id, sort_order, title);

create table if not exists album_items (
    album_id uuid not null references albums(id) on delete cascade,
    asset_id uuid not null references assets(id) on delete cascade,
    note text not null default '',
    sort_order int not null default 0,
    added_at timestamptz not null default now(),
    primary key(album_id, asset_id)
);

create index if not exists idx_album_items_album_sort
    on album_items(album_id, sort_order, added_at);

create index if not exists idx_album_items_asset
    on album_items(asset_id);

create table if not exists asset_geo (
    asset_id uuid primary key references assets(id) on delete cascade,
    lat double precision not null,
    lon double precision not null,
    source text not null,
    confidence double precision null,
    taken_at timestamptz null,
    track_asset_id uuid null references assets(id) on delete set null,
    metadata_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint asset_geo_source_check check (source in ('real', 'exif', 'track_snapped', 'estimated', 'manual', 'unknown')),
    constraint asset_geo_lat_check check (lat >= -90 and lat <= 90),
    constraint asset_geo_lon_check check (lon >= -180 and lon <= 180)
);

create index if not exists idx_asset_geo_lat_lon
    on asset_geo(lat, lon);

create index if not exists idx_asset_geo_taken_at
    on asset_geo(taken_at);

create index if not exists idx_asset_geo_source
    on asset_geo(source);

create index if not exists idx_asset_geo_track
    on asset_geo(track_asset_id);

create table if not exists gps_tracks (
    track_asset_id uuid primary key references assets(id) on delete cascade,
    title text not null,
    description text not null default '',
    point_count int not null default 0,
    start_at timestamptz null,
    end_at timestamptz null,
    min_lat double precision null,
    min_lon double precision null,
    max_lat double precision null,
    max_lon double precision null,
    distance_m double precision null,
    duration_seconds double precision null,
    elevation_min_m double precision null,
    elevation_max_m double precision null,
    metadata_json jsonb not null default '{}'::jsonb,
    updated_at timestamptz not null default now()
);

create index if not exists idx_gps_tracks_start_end
    on gps_tracks(start_at, end_at);

create index if not exists idx_gps_tracks_bbox
    on gps_tracks(min_lon, min_lat, max_lon, max_lat);

create index if not exists idx_gps_tracks_title
    on gps_tracks(title);

create table if not exists scan_runs (
    id uuid primary key,
    job_id uuid null references jobs(id) on delete set null,
    storage_name text not null,
    mode text not null,
    prefixes_json jsonb not null default '[]'::jsonb,
    max_files int not null,
    max_bytes bigint not null,
    hash_requested boolean not null default false,
    metadata_requested boolean not null default false,
    previews_requested boolean not null default false,
    mark_missing boolean not null default false,
    dry_run boolean not null default true,
    started_at timestamptz null,
    finished_at timestamptz null,
    report_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now()
);

create index if not exists idx_scan_runs_job
    on scan_runs(job_id);

create index if not exists idx_scan_runs_storage
    on scan_runs(storage_name, created_at desc);

create table if not exists preview_cache_entries (
    id uuid primary key,
    asset_id uuid not null references assets(id) on delete cascade,
    content_id uuid null references contents(id) on delete set null,
    variant text not null,
    width int not null,
    height int not null,
    format text not null,
    cache_path text not null,
    status text not null,
    size_bytes bigint not null default 0,
    created_at timestamptz not null default now(),
    last_accessed_at timestamptz null,
    error text null,
    unique(asset_id, variant, width, height, format)
);

create index if not exists idx_preview_cache_entries_asset
    on preview_cache_entries(asset_id);

create index if not exists idx_preview_cache_entries_content
    on preview_cache_entries(content_id);

create index if not exists idx_preview_cache_entries_status
    on preview_cache_entries(status);

create index if not exists idx_preview_cache_entries_accessed
    on preview_cache_entries(last_accessed_at);

create table if not exists plugin_settings (
    plugin_id text primary key references plugins(id) on delete cascade,
    settings_json jsonb not null default '{}'::jsonb,
    updated_at timestamptz not null default now()
);

create index if not exists idx_asset_locations_storage_kind_path
    on asset_locations(storage_id, media_kind, relative_path);

create index if not exists idx_asset_locations_extension
    on asset_locations(extension);

create index if not exists idx_asset_locations_hash_status
    on asset_locations(hash_status);

create index if not exists idx_assets_display_name
    on assets(display_name);

create index if not exists idx_asset_geo_source_taken_at
    on asset_geo(source, taken_at);

create index if not exists idx_jobs_status_kind_created_desc
    on jobs(status, kind, created_at desc);
