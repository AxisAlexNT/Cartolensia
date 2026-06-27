package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/id"
)

func (db *DB) ListKnowledgeFacts(ctx context.Context, query catalog.KnowledgeQuery) (catalog.KnowledgeFactPage, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	where, args := knowledgeFactWhere(query)
	totalSQL := `select count(*)::bigint from knowledge_facts f left join assets a on a.id=f.asset_id`
	if where != "" {
		totalSQL += " where " + where
	}
	var total int64
	if err := db.pool.QueryRow(ctx, totalSQL, args...).Scan(&total); err != nil {
		return catalog.KnowledgeFactPage{}, err
	}
	args = append(args, limit, offset)
	sql := `select f.id, coalesce(f.asset_id::text, ''), f.source_kind, f.source_id, f.subject, f.predicate,
			f.object, f.confidence, f.language, f.evidence, f.created_at, f.updated_at, f.metadata_json
		from knowledge_facts f
		left join assets a on a.id=f.asset_id`
	if where != "" {
		sql += " where " + where
	}
	sql += fmt.Sprintf(" order by f.updated_at desc, f.id limit $%d offset $%d", len(args)-1, len(args))
	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		return catalog.KnowledgeFactPage{}, err
	}
	defer rows.Close()
	facts := []catalog.KnowledgeFact{}
	for rows.Next() {
		fact, err := scanKnowledgeFact(rows)
		if err != nil {
			return catalog.KnowledgeFactPage{}, err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return catalog.KnowledgeFactPage{}, err
	}
	return catalog.KnowledgeFactPage{Facts: facts, Page: catalog.PageInfo{Limit: limit, Offset: offset, Total: int(total)}}, nil
}

func (db *DB) ListKnowledgeRelations(ctx context.Context, query catalog.KnowledgeQuery) (catalog.KnowledgeRelationPage, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	where, args := knowledgeRelationWhere(query)
	totalSQL := `select count(*)::bigint from knowledge_relations r
		left join assets fa on fa.id=r.from_asset_id
		left join assets ta on ta.id=r.to_asset_id`
	if where != "" {
		totalSQL += " where " + where
	}
	var total int64
	if err := db.pool.QueryRow(ctx, totalSQL, args...).Scan(&total); err != nil {
		return catalog.KnowledgeRelationPage{}, err
	}
	args = append(args, limit, offset)
	sql := `select r.id, coalesce(r.from_asset_id::text, ''), coalesce(r.to_asset_id::text, ''),
			r.from_entity, r.to_entity, r.relation, r.confidence, r.evidence, r.created_at, r.metadata_json
		from knowledge_relations r
		left join assets fa on fa.id=r.from_asset_id
		left join assets ta on ta.id=r.to_asset_id`
	if where != "" {
		sql += " where " + where
	}
	sql += fmt.Sprintf(" order by r.created_at desc, r.id limit $%d offset $%d", len(args)-1, len(args))
	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		return catalog.KnowledgeRelationPage{}, err
	}
	defer rows.Close()
	relations := []catalog.KnowledgeRelation{}
	for rows.Next() {
		relation, err := scanKnowledgeRelation(rows)
		if err != nil {
			return catalog.KnowledgeRelationPage{}, err
		}
		relations = append(relations, relation)
	}
	if err := rows.Err(); err != nil {
		return catalog.KnowledgeRelationPage{}, err
	}
	return catalog.KnowledgeRelationPage{Relations: relations, Page: catalog.PageInfo{Limit: limit, Offset: offset, Total: int(total)}}, nil
}

func (db *DB) ExtractKnowledge(ctx context.Context, limit int) (catalog.KnowledgeExtractionResult, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	result := catalog.KnowledgeExtractionResult{
		Limit:  limit,
		Counts: map[string]int{},
		Note:   "knowledge extraction is idempotent; inserted_or_updated counts include PostgreSQL upserts",
	}
	steps := []struct {
		name string
		sql  string
	}{
		{"asset_kind", assetKindFactsSQL},
		{"asset_locations", assetLocationFactsSQL},
		{"asset_device", assetDeviceFactsSQL},
		{"asset_taken_at", assetTakenAtFactsSQL},
		{"asset_geo", assetGeoFactsSQL},
		{"asset_tags", assetTagFactsSQL},
		{"ai_predictions", aiPredictionFactsSQL},
		{"transcripts", transcriptFactsSQL},
		{"documents", documentFactsSQL},
		{"audio_features", audioFeatureFactsSQL},
		{"gps_tracks", trackFactsSQL},
	}
	for _, step := range steps {
		affected, err := db.execKnowledgeInsert(ctx, step.sql, limit)
		if err != nil {
			return result, fmt.Errorf("%s: %w", step.name, err)
		}
		result.FactsInserted += affected
		result.Counts[step.name] = int(affected)
	}
	relationSteps := []struct {
		name string
		sql  string
	}{
		{"stored_in_folder", folderRelationSQL},
		{"captured_with_device", deviceRelationSQL},
		{"has_tag", tagRelationSQL},
		{"track_link", trackLinkRelationSQL},
		{"has_transcript", transcriptRelationSQL},
		{"has_document_text", documentRelationSQL},
		{"has_audio_features", audioFeatureRelationSQL},
	}
	for _, step := range relationSteps {
		affected, err := db.execKnowledgeInsert(ctx, step.sql, limit)
		if err != nil {
			return result, fmt.Errorf("%s: %w", step.name, err)
		}
		result.RelationsInserted += affected
		result.Counts[step.name] = int(affected)
	}
	return result, nil
}

func (db *DB) EnsureKnowledgeConversation(ctx context.Context, conversationID, title string) (catalog.KnowledgeConversation, error) {
	if conversationID == "" {
		conversationID = id.NewUUID()
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Knowledge conversation"
	}
	_, err := db.pool.Exec(ctx, `
		insert into knowledge_conversations(id, title, metadata_json)
		values($1, $2, '{}'::jsonb)
		on conflict(id) do update set title=coalesce(nullif(knowledge_conversations.title, ''), excluded.title), updated_at=now()
	`, conversationID, title)
	if err != nil {
		return catalog.KnowledgeConversation{}, err
	}
	var conv catalog.KnowledgeConversation
	var meta []byte
	err = db.pool.QueryRow(ctx, `
		select id::text, title, created_at, updated_at, metadata_json
		from knowledge_conversations
		where id=$1
	`, conversationID).Scan(&conv.ID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt, &meta)
	if err != nil {
		return catalog.KnowledgeConversation{}, err
	}
	conv.Metadata = decodeMap(meta)
	return conv, nil
}

func (db *DB) AddKnowledgeMessage(ctx context.Context, message catalog.KnowledgeMessage) (catalog.KnowledgeMessage, error) {
	if message.ID == "" {
		message.ID = id.NewUUID()
	}
	toolCalls, err := json.Marshal(message.ToolCalls)
	if err != nil {
		return catalog.KnowledgeMessage{}, err
	}
	_, err = db.pool.Exec(ctx, `
		insert into knowledge_messages(id, conversation_id, role, content, tool_calls_json)
		values($1, $2, $3, $4, $5::jsonb)
	`, message.ID, message.ConversationID, message.Role, message.Content, toolCalls)
	if err != nil {
		return catalog.KnowledgeMessage{}, err
	}
	err = db.pool.QueryRow(ctx, `
		select id::text, conversation_id::text, role, content, tool_calls_json, created_at
		from knowledge_messages
		where id=$1
	`, message.ID).Scan(&message.ID, &message.ConversationID, &message.Role, &message.Content, &toolCalls, &message.CreatedAt)
	if err != nil {
		return catalog.KnowledgeMessage{}, err
	}
	_ = json.Unmarshal(toolCalls, &message.ToolCalls)
	return message, nil
}

func (db *DB) ListKnowledgeMessages(ctx context.Context, conversationID string, limit int) ([]catalog.KnowledgeMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.pool.Query(ctx, `
		select id::text, conversation_id::text, role, content, tool_calls_json, created_at
		from knowledge_messages
		where conversation_id=$1
		order by created_at asc, id
		limit $2
	`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.KnowledgeMessage{}
	for rows.Next() {
		var item catalog.KnowledgeMessage
		var toolCalls []byte
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.Role, &item.Content, &toolCalls, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(toolCalls, &item.ToolCalls)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (db *DB) execKnowledgeInsert(ctx context.Context, sql string, limit int) (int64, error) {
	tag, err := db.pool.Exec(ctx, sql, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

type factRowScanner interface {
	Scan(dest ...any) error
}

func scanKnowledgeFact(row factRowScanner) (catalog.KnowledgeFact, error) {
	var fact catalog.KnowledgeFact
	var meta []byte
	if err := row.Scan(&fact.ID, &fact.AssetID, &fact.SourceKind, &fact.SourceID, &fact.Subject, &fact.Predicate, &fact.Object, &fact.Confidence, &fact.Language, &fact.Evidence, &fact.CreatedAt, &fact.UpdatedAt, &meta); err != nil {
		return catalog.KnowledgeFact{}, err
	}
	fact.Metadata = decodeMap(meta)
	return fact, nil
}

func scanKnowledgeRelation(row factRowScanner) (catalog.KnowledgeRelation, error) {
	var relation catalog.KnowledgeRelation
	var meta []byte
	if err := row.Scan(&relation.ID, &relation.FromAssetID, &relation.ToAssetID, &relation.FromEntity, &relation.ToEntity, &relation.Relation, &relation.Confidence, &relation.Evidence, &relation.CreatedAt, &meta); err != nil {
		return catalog.KnowledgeRelation{}, err
	}
	relation.Metadata = decodeMap(meta)
	return relation, nil
}

func knowledgeFactWhere(query catalog.KnowledgeQuery) (string, []any) {
	var parts []string
	var args []any
	if query.AssetID != "" {
		args = append(args, query.AssetID)
		parts = append(parts, fmt.Sprintf("f.asset_id::text=$%d", len(args)))
	}
	if query.SourceKind != "" {
		args = append(args, query.SourceKind)
		parts = append(parts, fmt.Sprintf("f.source_kind=$%d", len(args)))
	}
	if query.Predicate != "" {
		args = append(args, query.Predicate)
		parts = append(parts, fmt.Sprintf("f.predicate=$%d", len(args)))
	}
	if q := strings.TrimSpace(query.Q); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		parts = append(parts, fmt.Sprintf("lower(coalesce(a.display_name, '') || ' ' || f.subject || ' ' || f.predicate || ' ' || f.object || ' ' || f.evidence) like $%d", len(args)))
	}
	return strings.Join(parts, " and "), args
}

func knowledgeRelationWhere(query catalog.KnowledgeQuery) (string, []any) {
	var parts []string
	var args []any
	if query.AssetID != "" {
		args = append(args, query.AssetID)
		parts = append(parts, fmt.Sprintf("(r.from_asset_id::text=$%d or r.to_asset_id::text=$%d)", len(args), len(args)))
	}
	if query.Relation != "" {
		args = append(args, query.Relation)
		parts = append(parts, fmt.Sprintf("r.relation=$%d", len(args)))
	}
	if q := strings.TrimSpace(query.Q); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		parts = append(parts, fmt.Sprintf("lower(coalesce(fa.display_name, '') || ' ' || coalesce(ta.display_name, '') || ' ' || r.from_entity || ' ' || r.relation || ' ' || r.to_entity || ' ' || r.evidence) like $%d", len(args)))
	}
	return strings.Join(parts, " and "), args
}

func knowledgeUpsertSuffix(kind string) string {
	if kind == "relation" {
		return ` on conflict(id) do update set
			from_asset_id=excluded.from_asset_id,
			to_asset_id=excluded.to_asset_id,
			from_entity=excluded.from_entity,
			to_entity=excluded.to_entity,
			relation=excluded.relation,
			confidence=excluded.confidence,
			evidence=excluded.evidence,
			metadata_json=excluded.metadata_json`
	}
	return ` on conflict(id) do update set
		asset_id=excluded.asset_id,
		source_kind=excluded.source_kind,
		source_id=excluded.source_id,
		subject=excluded.subject,
		predicate=excluded.predicate,
		object=excluded.object,
		confidence=excluded.confidence,
		language=excluded.language,
		evidence=excluded.evidence,
		updated_at=now(),
		metadata_json=excluded.metadata_json`
}

var assetKindFactsSQL = `
with scoped as (
	select id, display_name, media_kind from assets order by updated_at desc limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, evidence, metadata_json)
select md5('asset-kind:' || id::text), id, 'asset', id::text, display_name, 'media_kind', media_kind, 'indexed asset media kind', '{}'::jsonb
from scoped` + knowledgeUpsertSuffix("fact")

var assetLocationFactsSQL = `
with scoped as (
	select a.id as asset_id, a.display_name, l.id as location_id, s.name as storage_name, l.relative_path, l.file_name, l.extension
	from asset_locations l
	join assets a on a.id=l.asset_id
	join storage_backends s on s.id=l.storage_id
	order by l.last_seen_at desc
	limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, evidence, metadata_json)
select md5('location:' || location_id::text), asset_id, 'location', location_id::text, display_name, 'stored_at',
	storage_name || ':' || relative_path,
	'asset location in strict read-only or configured storage',
	jsonb_build_object('storage', storage_name, 'file_name', file_name, 'extension', lower(trim(leading '.' from extension)))
from scoped` + knowledgeUpsertSuffix("fact")

var assetDeviceFactsSQL = `
with scoped as (
	select id, display_name,
		trim(coalesce(metadata_json->>'camera_make', '') || ' ' || coalesce(metadata_json->>'camera_model', '')) as device
	from assets
	where coalesce(metadata_json->>'camera_make', metadata_json->>'camera_model', '') <> ''
	order by updated_at desc
	limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, evidence, metadata_json)
select md5('device:' || id::text), id, 'metadata', id::text, display_name, 'captured_with', device,
	'camera/device metadata extracted from asset metadata', '{}'::jsonb
from scoped
where device <> ''` + knowledgeUpsertSuffix("fact")

var assetTakenAtFactsSQL = `
with scoped as (
	select id, display_name, taken_at from assets where taken_at is not null order by taken_at desc limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, evidence, metadata_json)
select md5('taken-at:' || id::text), id, 'metadata', id::text, display_name, 'taken_at', taken_at::text,
	'trusted indexed timestamp', '{}'::jsonb
from scoped` + knowledgeUpsertSuffix("fact")

var assetGeoFactsSQL = `
with scoped as (
	select a.id, a.display_name, g.lat, g.lon, g.source, g.confidence
	from asset_geo g
	join assets a on a.id=g.asset_id
	order by g.updated_at desc
	limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, confidence, evidence, metadata_json)
select md5('geo:' || id::text), id, 'geotag', id::text, display_name, 'located_at_coordinates',
	lat::text || ', ' || lon::text, confidence, 'stored geotag source: ' || source,
	jsonb_build_object('lat', lat, 'lon', lon, 'source', source)
from scoped` + knowledgeUpsertSuffix("fact")

var assetTagFactsSQL = `
with scoped as (
	select a.id, a.display_name, t.tag, t.source, t.confidence
	from asset_tags t
	join assets a on a.id=t.asset_id
	order by t.created_at desc
	limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, confidence, evidence, metadata_json)
select md5('tag:' || id::text || ':' || tag || ':' || source), id, 'tag', tag || ':' || source, display_name, 'tagged_as',
	tag, confidence, 'tag source: ' || source, '{}'::jsonb
from scoped` + knowledgeUpsertSuffix("fact")

var aiPredictionFactsSQL = `
with scoped as (
	select a.display_name, p.*
	from ai_predictions p
	join assets a on a.id=p.asset_id
	order by p.created_at desc
	limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, confidence, evidence, metadata_json)
select md5('ai:' || id::text), asset_id, 'ai_prediction', id::text, display_name,
	case
		when task in ('describe_image', 'caption_short', 'caption_long') then 'caption'
		when task in ('ocr_image', 'ocr', 'ocr_text') then 'ocr_text'
		when task in ('safety_nsfw', 'safety') then 'safety_prediction'
		else 'ai_label'
	end,
	label, confidence,
	trim(task || ' ' || model_name || ' ' || model_version),
	metadata_json
from scoped
where label <> ''` + knowledgeUpsertSuffix("fact")

var transcriptFactsSQL = `
with scoped as (
	select a.display_name, t.*
	from asset_transcripts t
	join assets a on a.id=t.asset_id
	where t.full_text <> ''
	order by t.created_at desc
	limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, language, evidence, metadata_json)
select md5('transcript:' || id::text), asset_id, 'transcript', id::text, display_name, 'transcript',
	left(full_text, 4000), language, trim(source_kind || ' ' || model), metadata_json
from scoped` + knowledgeUpsertSuffix("fact")

var documentFactsSQL = `
with scoped as (
	select a.display_name, d.*
	from document_text d
	join assets a on a.id=d.asset_id
	where coalesce(d.title, '') <> '' or coalesce(d.text, '') <> '' or coalesce(d.markdown, '') <> ''
	order by d.created_at desc
	limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, evidence, metadata_json)
select md5('document:' || asset_id::text), asset_id, 'document', asset_id::text, display_name, 'document_text',
	left(trim(coalesce(nullif(title, ''), '') || ' ' || coalesce(nullif(markdown, ''), text)), 4000),
	trim('document extraction engine ' || engine),
	metadata_json
from scoped` + knowledgeUpsertSuffix("fact")

var audioFeatureFactsSQL = `
with scoped as (
	select a.display_name, f.*
	from audio_features f
	join assets a on a.id=f.asset_id
	order by f.created_at desc
	limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, evidence, metadata_json)
select md5('audio-features:' || asset_id::text), asset_id, 'audio_features', asset_id::text, display_name, 'audio_features',
	trim(concat_ws(' ', 'duration=' || coalesce(duration_seconds::text, ''), 'tempo=' || coalesce(tempo_bpm::text, ''), 'key=' || coalesce(musical_key, ''), 'mode=' || coalesce(musical_mode, ''), 'genre=' || coalesce(genre_labels::text, ''))),
	trim('audio model ' || model),
	metadata_json
from scoped` + knowledgeUpsertSuffix("fact")

var trackFactsSQL = `
with scoped as (
	select a.display_name, g.*
	from gps_tracks g
	join assets a on a.id=g.track_asset_id
	order by g.updated_at desc
	limit $1
)
insert into knowledge_facts(id, asset_id, source_kind, source_id, subject, predicate, object, evidence, metadata_json)
select md5('track:' || track_asset_id::text), track_asset_id, 'track', track_asset_id::text, display_name, 'track_summary',
	trim(concat_ws(' ', 'points=' || point_count::text, 'start=' || coalesce(start_at::text, ''), 'end=' || coalesce(end_at::text, ''), 'distance_m=' || coalesce(distance_m::text, ''))),
	'parsed GPS/KML track summary',
	metadata_json
from scoped` + knowledgeUpsertSuffix("fact")

var folderRelationSQL = `
with scoped as (
	select a.id, a.display_name, nullif(regexp_replace(l.relative_path, '/[^/]*$', ''), l.relative_path) as folder
	from asset_locations l
	join assets a on a.id=l.asset_id
	order by l.last_seen_at desc
	limit $1
)
insert into knowledge_relations(id, from_asset_id, from_entity, to_entity, relation, evidence, metadata_json)
select md5('folder-rel:' || id::text || ':' || coalesce(folder, 'root')), id, display_name, 'folder:' || coalesce(folder, 'root'),
	'stored_in_folder', 'asset location path', '{}'::jsonb
from scoped` + knowledgeUpsertSuffix("relation")

var deviceRelationSQL = `
with scoped as (
	select id, display_name, trim(coalesce(metadata_json->>'camera_make', '') || ' ' || coalesce(metadata_json->>'camera_model', '')) as device
	from assets
	where coalesce(metadata_json->>'camera_make', metadata_json->>'camera_model', '') <> ''
	order by updated_at desc
	limit $1
)
insert into knowledge_relations(id, from_asset_id, from_entity, to_entity, relation, evidence, metadata_json)
select md5('device-rel:' || id::text), id, display_name, 'device:' || device,
	'captured_with_device', 'camera metadata', '{}'::jsonb
from scoped
where device <> ''` + knowledgeUpsertSuffix("relation")

var tagRelationSQL = `
with scoped as (
	select a.id, a.display_name, t.tag, t.source, t.confidence
	from asset_tags t
	join assets a on a.id=t.asset_id
	order by t.created_at desc
	limit $1
)
insert into knowledge_relations(id, from_asset_id, from_entity, to_entity, relation, confidence, evidence, metadata_json)
select md5('tag-rel:' || id::text || ':' || tag || ':' || source), id, display_name, 'tag:' || tag,
	'has_tag', confidence, 'tag source: ' || source, '{}'::jsonb
from scoped` + knowledgeUpsertSuffix("relation")

var trackLinkRelationSQL = `
with scoped as (
	select l.id as link_id, l.asset_id, l.track_asset_id, a.display_name as asset_name, ta.display_name as track_name, l.confidence, l.match_status
	from asset_track_links l
	join assets a on a.id=l.asset_id
	join assets ta on ta.id=l.track_asset_id
	order by l.updated_at desc
	limit $1
)
insert into knowledge_relations(id, from_asset_id, to_asset_id, from_entity, to_entity, relation, confidence, evidence, metadata_json)
select md5('track-link-rel:' || link_id::text), asset_id, track_asset_id, asset_name, track_name,
	'linked_to_track', confidence, match_status, '{}'::jsonb
from scoped` + knowledgeUpsertSuffix("relation")

var transcriptRelationSQL = `
with scoped as (
	select a.id, a.display_name, t.id as transcript_id, t.language, t.model
	from asset_transcripts t
	join assets a on a.id=t.asset_id
	order by t.created_at desc
	limit $1
)
insert into knowledge_relations(id, from_asset_id, from_entity, to_entity, relation, evidence, metadata_json)
select md5('transcript-rel:' || transcript_id::text), id, display_name, 'transcript:' || transcript_id::text,
	'has_transcript', trim(language || ' ' || model), '{}'::jsonb
from scoped` + knowledgeUpsertSuffix("relation")

var documentRelationSQL = `
with scoped as (
	select a.id, a.display_name, d.engine
	from document_text d
	join assets a on a.id=d.asset_id
	order by d.created_at desc
	limit $1
)
insert into knowledge_relations(id, from_asset_id, from_entity, to_entity, relation, evidence, metadata_json)
select md5('document-rel:' || id::text), id, display_name, 'document_text:' || id::text,
	'has_document_text', engine, '{}'::jsonb
from scoped` + knowledgeUpsertSuffix("relation")

var audioFeatureRelationSQL = `
with scoped as (
	select a.id, a.display_name, f.model
	from audio_features f
	join assets a on a.id=f.asset_id
	order by f.created_at desc
	limit $1
)
insert into knowledge_relations(id, from_asset_id, from_entity, to_entity, relation, evidence, metadata_json)
select md5('audio-feature-rel:' || id::text), id, display_name, 'audio_features:' || id::text,
	'has_audio_features', model, '{}'::jsonb
from scoped` + knowledgeUpsertSuffix("relation")
