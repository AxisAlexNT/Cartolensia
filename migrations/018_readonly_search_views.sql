create or replace view cartolensia_search_assets as
select
    a.id::text as asset_id,
    a.media_kind,
    a.display_name,
    a.taken_at,
    a.first_seen_at,
    a.updated_at,
    a.metadata_json->>'camera_make' as camera_make,
    a.metadata_json->>'camera_model' as camera_model,
    a.metadata_json->>'exif_datetime_original_raw' as exif_datetime_original_raw,
    g.lat,
    g.lon,
    g.source as geo_source,
    s.name as storage_name,
    l.relative_path,
    l.file_name,
    lower(trim(leading '.' from l.extension)) as extension,
    l.mime_type,
    l.size_bytes,
    l.mtime,
    l.hash_status,
    encode(c.sha512, 'hex') as sha512_hex
from assets a
left join asset_locations l on l.asset_id = a.id
left join storage_backends s on s.id = l.storage_id
left join contents c on c.id = l.content_id
left join asset_geo g on g.asset_id = a.id;

create or replace view cartolensia_search_ai_predictions as
select
    p.id::text as prediction_id,
    p.asset_id::text as asset_id,
    a.display_name,
    a.media_kind,
    p.task,
    p.label,
    p.confidence,
    p.model_name,
    p.model_version,
    p.created_at
from ai_predictions p
join assets a on a.id = p.asset_id;

create or replace view cartolensia_search_tags as
select
    t.asset_id::text as asset_id,
    a.display_name,
    a.media_kind,
    t.tag,
    t.source,
    t.confidence,
    t.created_at
from asset_tags t
join assets a on a.id = t.asset_id;

create or replace view cartolensia_search_transcripts as
select
    t.id::text as transcript_id,
    t.asset_id::text as asset_id,
    a.display_name,
    a.media_kind,
    t.source_kind,
    t.language,
    t.model,
    left(t.full_text, 2000) as full_text,
    t.created_at
from asset_transcripts t
join assets a on a.id = t.asset_id;

create or replace view cartolensia_search_transcript_segments as
select
    s.id::text as segment_id,
    s.transcript_id::text as transcript_id,
    s.asset_id::text as asset_id,
    a.display_name,
    a.media_kind,
    s.start_ms,
    s.end_ms,
    s.text,
    s.confidence
from asset_transcript_segments s
join assets a on a.id = s.asset_id;

create or replace view cartolensia_search_documents as
select
    d.asset_id::text as asset_id,
    a.display_name,
    a.media_kind,
    d.page_count,
    d.title,
    d.author,
    left(d.text, 2000) as text,
    left(d.markdown, 2000) as markdown,
    d.engine,
    d.created_at
from document_text d
join assets a on a.id = d.asset_id;

create or replace view cartolensia_search_video_captions as
select
    c.id::text as caption_id,
    c.asset_id::text as asset_id,
    a.display_name,
    a.media_kind,
    c.timestamp_ms,
    c.fraction,
    c.caption,
    c.model,
    c.created_at
from video_frame_captions c
join assets a on a.id = c.asset_id;

create or replace view cartolensia_search_audio_features as
select
    f.asset_id::text as asset_id,
    a.display_name,
    a.media_kind,
    f.duration_seconds,
    f.tempo_bpm,
    f.musical_key,
    f.musical_mode,
    f.loudness,
    f.speech_music_ratio,
    f.genre_labels,
    f.model,
    f.created_at
from audio_features f
join assets a on a.id = f.asset_id;

create or replace view cartolensia_search_tracks as
select
    g.track_asset_id::text as track_asset_id,
    a.display_name,
    g.title,
    g.description,
    g.point_count,
    g.start_at,
    g.end_at,
    g.min_lat,
    g.min_lon,
    g.max_lat,
    g.max_lon,
    g.distance_m,
    g.duration_seconds,
    g.metadata_json->>'source_format' as source_format
from gps_tracks g
join assets a on a.id = g.track_asset_id;

create or replace view cartolensia_search_places as
select
    id::text as place_id,
    name,
    display_name,
    country,
    region,
    city,
    road,
    provider,
    source,
    lat,
    lon,
    min_lon,
    min_lat,
    max_lon,
    max_lat,
    updated_at,
    last_used_at
from place_cache;
