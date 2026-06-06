create table if not exists app_settings (
    key text primary key,
    value_json jsonb not null default '{}'::jsonb,
    updated_at timestamptz not null default now()
);

alter table jobs add column if not exists next_run_at timestamptz;

create index if not exists idx_jobs_queued_next_run
    on jobs(next_run_at, created_at)
    where status = 'queued';

create index if not exists idx_jobs_running_lease
    on jobs(lease_expires_at)
    where status in ('running', 'cancel_requested');
