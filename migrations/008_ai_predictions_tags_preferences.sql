create table if not exists asset_tags (
    asset_id uuid not null references assets(id) on delete cascade,
    tag text not null,
    source text not null default 'manual',
    confidence double precision null,
    plugin_id text null references plugins(id),
    metadata_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    primary key(asset_id, tag, source)
);

create table if not exists ai_predictions (
    id uuid primary key,
    asset_id uuid not null references assets(id) on delete cascade,
    plugin_id text null references plugins(id),
    worker_id text not null default '',
    task text not null,
    label text not null,
    confidence double precision null,
    model_name text not null default '',
    model_version text not null default '',
    metadata_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now()
);

create table if not exists face_detections (
    id uuid primary key,
    asset_id uuid not null references assets(id) on delete cascade,
    plugin_id text null references plugins(id),
    x double precision not null,
    y double precision not null,
    width double precision not null,
    height double precision not null,
    confidence double precision null,
    cluster_id uuid null,
    metadata_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now()
);

create table if not exists face_clusters (
    id uuid primary key,
    label text not null default '',
    representative_face_id uuid null references face_detections(id) on delete set null,
    metadata_json jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists user_preferences (
    principal_id text not null,
    key text not null,
    value_json jsonb not null default '{}'::jsonb,
    updated_at timestamptz not null default now(),
    primary key(principal_id, key)
);

create index if not exists idx_asset_tags_tag on asset_tags(tag);
create index if not exists idx_asset_tags_asset on asset_tags(asset_id);
create index if not exists idx_ai_predictions_asset on ai_predictions(asset_id);
create index if not exists idx_ai_predictions_task_label on ai_predictions(task, label);
create index if not exists idx_face_detections_asset on face_detections(asset_id);
