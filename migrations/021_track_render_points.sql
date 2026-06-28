create table if not exists gps_track_render_points (
    track_asset_id uuid not null references gps_tracks(track_asset_id) on delete cascade,
    detail_level text not null default 'overview',
    ordinal int not null,
    recorded_at timestamptz null,
    lat double precision not null,
    lon double precision not null,
    elevation_m double precision null,
    speed_mps double precision null,
    source text not null default 'track_points',
    updated_at timestamptz not null default now(),
    primary key(track_asset_id, detail_level, ordinal)
);

create index if not exists idx_gps_track_render_points_level
    on gps_track_render_points(detail_level, track_asset_id, ordinal);
