create table if not exists ai_asset_task_status (
    asset_id uuid not null references assets(id) on delete cascade,
    task text not null,
    status text not null default 'succeeded',
    worker_id text not null default '',
    model_name text not null default '',
    stored_count integer not null default 0,
    error text not null default '',
    metadata_json jsonb not null default '{}'::jsonb,
    updated_at timestamptz not null default now(),
    primary key(asset_id, task)
);

create index if not exists idx_ai_asset_task_status_task_status on ai_asset_task_status(task, status, updated_at desc);
create index if not exists idx_ai_asset_task_status_asset on ai_asset_task_status(asset_id);
