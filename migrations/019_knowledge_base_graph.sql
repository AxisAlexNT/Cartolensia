create table if not exists knowledge_facts (
    id text primary key,
    asset_id uuid null references assets(id) on delete cascade,
    source_kind text not null,
    source_id text not null default '',
    subject text not null,
    predicate text not null,
    object text not null,
    confidence double precision null,
    language text not null default '',
    evidence text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    metadata_json jsonb not null default '{}'::jsonb
);

create table if not exists knowledge_relations (
    id text primary key,
    from_asset_id uuid null references assets(id) on delete cascade,
    to_asset_id uuid null references assets(id) on delete cascade,
    from_entity text not null default '',
    to_entity text not null default '',
    relation text not null,
    confidence double precision null,
    evidence text not null default '',
    created_at timestamptz not null default now(),
    metadata_json jsonb not null default '{}'::jsonb
);

create table if not exists knowledge_conversations (
    id uuid primary key,
    title text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    metadata_json jsonb not null default '{}'::jsonb
);

create table if not exists knowledge_messages (
    id uuid primary key,
    conversation_id uuid not null references knowledge_conversations(id) on delete cascade,
    role text not null,
    content text not null default '',
    tool_calls_json jsonb not null default '[]'::jsonb,
    created_at timestamptz not null default now()
);

create index if not exists idx_knowledge_facts_asset on knowledge_facts(asset_id);
create index if not exists idx_knowledge_facts_source on knowledge_facts(source_kind, source_id);
create index if not exists idx_knowledge_facts_predicate on knowledge_facts(predicate);
create index if not exists idx_knowledge_facts_updated on knowledge_facts(updated_at desc);
create index if not exists idx_knowledge_facts_text_fts on knowledge_facts using gin (
    to_tsvector('simple', coalesce(subject, '') || ' ' || coalesce(predicate, '') || ' ' || coalesce(object, '') || ' ' || coalesce(evidence, ''))
);

create index if not exists idx_knowledge_relations_from_asset on knowledge_relations(from_asset_id);
create index if not exists idx_knowledge_relations_to_asset on knowledge_relations(to_asset_id);
create index if not exists idx_knowledge_relations_relation on knowledge_relations(relation);
create index if not exists idx_knowledge_relations_entities on knowledge_relations(from_entity, to_entity);
create index if not exists idx_knowledge_relations_text_fts on knowledge_relations using gin (
    to_tsvector('simple', coalesce(from_entity, '') || ' ' || coalesce(relation, '') || ' ' || coalesce(to_entity, '') || ' ' || coalesce(evidence, ''))
);

create index if not exists idx_knowledge_messages_conversation_time on knowledge_messages(conversation_id, created_at);

create or replace view cartolensia_search_knowledge_facts as
select
    f.id as fact_id,
    f.asset_id::text as asset_id,
    a.display_name,
    a.media_kind,
    f.source_kind,
    f.source_id,
    f.subject,
    f.predicate,
    left(f.object, 2000) as object,
    f.confidence,
    f.language,
    left(f.evidence, 2000) as evidence,
    f.created_at,
    f.updated_at
from knowledge_facts f
left join assets a on a.id = f.asset_id;

create or replace view cartolensia_search_knowledge_relations as
select
    r.id as relation_id,
    r.from_asset_id::text as from_asset_id,
    fa.display_name as from_asset_name,
    r.to_asset_id::text as to_asset_id,
    ta.display_name as to_asset_name,
    r.from_entity,
    r.relation,
    r.to_entity,
    r.confidence,
    left(r.evidence, 2000) as evidence,
    r.created_at
from knowledge_relations r
left join assets fa on fa.id = r.from_asset_id
left join assets ta on ta.id = r.to_asset_id;
