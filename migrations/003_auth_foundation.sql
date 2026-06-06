create table if not exists users (
    id uuid primary key,
    email text not null unique,
    display_name text not null,
    password_hash text,
    role text not null default 'admin',
    disabled_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists sessions (
    id uuid primary key,
    user_id uuid not null references users(id) on delete cascade,
    token_hash bytea not null unique,
    expires_at timestamptz not null,
    created_at timestamptz not null default now(),
    last_seen_at timestamptz
);

create table if not exists api_tokens (
    id uuid primary key,
    user_id uuid not null references users(id) on delete cascade,
    name text not null,
    token_hash bytea not null unique,
    scopes text[] not null default '{}',
    expires_at timestamptz,
    created_at timestamptz not null default now(),
    last_used_at timestamptz,
    revoked_at timestamptz
);

create index if not exists idx_sessions_user_id on sessions(user_id);
create index if not exists idx_api_tokens_user_id on api_tokens(user_id);
