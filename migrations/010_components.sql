create table if not exists components (
    id uuid primary key,
    key text not null unique,
    name text not null,
    category text not null,
    version text not null default '',
    status text not null default 'missing',
    source_type text not null default 'system_path',
    source_url text not null default '',
    license_name text not null default '',
    provenance_url text not null default '',
    install_path text not null default '',
    executable_path text not null default '',
    checksum text not null default '',
    size_bytes bigint not null default 0,
    last_checked_at timestamptz null,
    installed_at timestamptz null,
    error text not null default '',
    metadata_json jsonb not null default '{}'::jsonb
);

create table if not exists component_events (
    id uuid primary key,
    component_key text not null references components(key) on delete cascade,
    level text not null default 'info',
    message text not null,
    created_at timestamptz not null default now(),
    metadata_json jsonb not null default '{}'::jsonb
);

create index if not exists idx_components_category_status on components(category, status);
create index if not exists idx_component_events_key_time on component_events(component_key, created_at desc);
