create table if not exists asset_midi_transcriptions (
    id uuid primary key,
    asset_id uuid not null references assets(id) on delete cascade,
    source_kind text not null default '',
    provider text not null default '',
    model text not null default '',
    status text not null default 'succeeded',
    midi_cache_path text not null default '',
    duration_seconds double precision,
    note_count integer not null default 0,
    instrument_count integer not null default 0,
    instruments_json jsonb not null default '[]'::jsonb,
    summary text not null default '',
    created_at timestamptz not null default now(),
    metadata_json jsonb not null default '{}'::jsonb
);

create unique index if not exists idx_asset_midi_provider_model
    on asset_midi_transcriptions(asset_id, provider, model);
create index if not exists idx_asset_midi_asset_time
    on asset_midi_transcriptions(asset_id, created_at desc);
create index if not exists idx_asset_midi_summary_fts
    on asset_midi_transcriptions
    using gin (to_tsvector('simple', coalesce(summary, '')));

create table if not exists asset_music_stems (
    id uuid primary key,
    asset_id uuid not null references assets(id) on delete cascade,
    source_kind text not null default '',
    provider text not null default '',
    model text not null default '',
    status text not null default 'succeeded',
    stem_set text not null default '',
    output_dir text not null default '',
    stems_json jsonb not null default '[]'::jsonb,
    created_at timestamptz not null default now(),
    metadata_json jsonb not null default '{}'::jsonb
);

create unique index if not exists idx_asset_music_stems_provider_model_set
    on asset_music_stems(asset_id, provider, model, stem_set);
create index if not exists idx_asset_music_stems_asset_time
    on asset_music_stems(asset_id, created_at desc);
create index if not exists idx_asset_music_stems_status
    on asset_music_stems(status);
