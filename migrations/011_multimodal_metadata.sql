create table if not exists asset_transcripts (
    id uuid primary key,
    asset_id uuid not null references assets(id) on delete cascade,
    source_kind text not null default '',
    language text not null default '',
    model text not null default '',
    full_text text not null default '',
    created_at timestamptz not null default now(),
    metadata_json jsonb not null default '{}'::jsonb
);

create table if not exists asset_transcript_segments (
    id uuid primary key,
    transcript_id uuid not null references asset_transcripts(id) on delete cascade,
    asset_id uuid not null references assets(id) on delete cascade,
    start_ms bigint not null default 0,
    end_ms bigint not null default 0,
    text text not null default '',
    confidence double precision null,
    speaker text not null default '',
    metadata_json jsonb not null default '{}'::jsonb
);

create table if not exists audio_features (
    asset_id uuid primary key references assets(id) on delete cascade,
    duration_seconds double precision null,
    tempo_bpm double precision null,
    musical_key text not null default '',
    musical_mode text not null default '',
    loudness double precision null,
    speech_music_ratio double precision null,
    genre_labels jsonb not null default '[]'::jsonb,
    model text not null default '',
    created_at timestamptz not null default now(),
    metadata_json jsonb not null default '{}'::jsonb
);

create table if not exists video_frame_captions (
    id uuid primary key,
    asset_id uuid not null references assets(id) on delete cascade,
    timestamp_ms bigint not null default 0,
    fraction double precision not null default 0,
    caption text not null default '',
    model text not null default '',
    created_at timestamptz not null default now(),
    metadata_json jsonb not null default '{}'::jsonb
);

create table if not exists document_text (
    asset_id uuid primary key references assets(id) on delete cascade,
    page_count integer not null default 0,
    title text not null default '',
    author text not null default '',
    text text not null default '',
    markdown text not null default '',
    engine text not null default '',
    created_at timestamptz not null default now(),
    metadata_json jsonb not null default '{}'::jsonb
);

create index if not exists idx_asset_transcripts_asset_time on asset_transcripts(asset_id, created_at desc);
create index if not exists idx_asset_transcript_segments_asset_time on asset_transcript_segments(asset_id, start_ms);
create index if not exists idx_video_frame_captions_asset_time on video_frame_captions(asset_id, timestamp_ms);
create index if not exists idx_audio_features_key_mode on audio_features(musical_key, musical_mode);
create index if not exists idx_audio_features_tempo on audio_features(tempo_bpm);
create index if not exists idx_asset_transcripts_full_text_fts on asset_transcripts using gin (to_tsvector('simple', coalesce(full_text, '')));
create index if not exists idx_asset_transcript_segments_text_fts on asset_transcript_segments using gin (to_tsvector('simple', coalesce(text, '')));
create index if not exists idx_video_frame_captions_caption_fts on video_frame_captions using gin (to_tsvector('simple', coalesce(caption, '')));
create index if not exists idx_document_text_text_fts on document_text using gin (to_tsvector('simple', coalesce(text, '') || ' ' || coalesce(markdown, '')));
