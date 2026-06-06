create table if not exists embedding_models (
    id text primary key,
    modality text not null,
    model_name text not null,
    version text not null,
    dimension int null,
    plugin_id text null references plugins(id),
    metadata_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now()
);

create table if not exists asset_embeddings (
    id uuid primary key,
    asset_id uuid not null references assets(id) on delete cascade,
    model_id text not null references embedding_models(id),
    modality text not null,
    source_ref text not null default 'asset',
    embedding_json jsonb not null default '{}'::jsonb,
    metadata_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    unique(asset_id, model_id, modality, source_ref)
);

create table if not exists transcoding_presets (
    id text primary key,
    name text not null,
    config_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists transcoding_outputs (
    id uuid primary key,
    source_asset_id uuid not null references assets(id) on delete cascade,
    output_storage text not null,
    output_url text null,
    status text not null default 'planned',
    safety_policy text not null default 'no_original_writes',
    metadata_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists idx_asset_embeddings_asset on asset_embeddings(asset_id);
create index if not exists idx_asset_embeddings_model on asset_embeddings(model_id);
create index if not exists idx_transcoding_outputs_source on transcoding_outputs(source_asset_id);
