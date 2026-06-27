do $$
begin
    create extension if not exists vector;
exception when others then
    raise notice 'pgvector extension is not available: %', sqlerrm;
end $$;

do $$
begin
    if exists(select 1 from pg_extension where extname = 'vector') then
        execute 'alter table asset_embeddings add column if not exists embedding_vector vector(512)';
        execute 'create index if not exists idx_asset_embeddings_vector_cosine on asset_embeddings using ivfflat (embedding_vector vector_cosine_ops) with (lists = 100)';
    end if;
exception when others then
    raise notice 'optional pgvector embedding setup skipped: %', sqlerrm;
end $$;
