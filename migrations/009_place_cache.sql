create table if not exists place_cache (
    id uuid primary key,
    name text not null,
    normalized_name text not null,
    aliases_json jsonb not null default '[]'::jsonb,
    provider text not null default 'local',
    display_name text not null default '',
    country text not null default '',
    region text not null default '',
    city text not null default '',
    road text not null default '',
    lat double precision not null,
    lon double precision not null,
    min_lon double precision not null,
    min_lat double precision not null,
    max_lon double precision not null,
    max_lat double precision not null,
    source text not null default 'operator_cache',
    metadata_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    last_used_at timestamptz null
);

create unique index if not exists idx_place_cache_provider_normalized on place_cache(provider, normalized_name);
create index if not exists idx_place_cache_name on place_cache(normalized_name);
create index if not exists idx_place_cache_bbox on place_cache(min_lon, min_lat, max_lon, max_lat);
