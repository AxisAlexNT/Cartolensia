drop index if exists idx_jobs_running_lease;

create index idx_jobs_running_lease
    on jobs(lease_expires_at)
    where status in ('running', 'cancel_requested');
