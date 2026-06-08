package database

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/id"
)

func (db *DB) UpsertTranscript(ctx context.Context, transcript catalog.Transcript, segments []catalog.TranscriptSegment) (catalog.Transcript, error) {
	if transcript.ID == "" {
		transcript.ID = id.NewUUID()
	}
	meta, err := json.Marshal(orEmptyMap(transcript.Metadata))
	if err != nil {
		return catalog.Transcript{}, err
	}
	_, err = db.pool.Exec(ctx, `
		insert into asset_transcripts(id, asset_id, source_kind, language, model, full_text, metadata_json)
		values($1, $2, $3, $4, $5, $6, $7::jsonb)
		on conflict(id) do update set
			source_kind=excluded.source_kind,
			language=excluded.language,
			model=excluded.model,
			full_text=excluded.full_text,
			metadata_json=excluded.metadata_json
	`, transcript.ID, transcript.AssetID, transcript.SourceKind, transcript.Language, transcript.Model, transcript.FullText, meta)
	if err != nil {
		return catalog.Transcript{}, err
	}
	if segments != nil {
		if _, err := db.pool.Exec(ctx, `delete from asset_transcript_segments where transcript_id=$1`, transcript.ID); err != nil {
			return catalog.Transcript{}, err
		}
		for _, segment := range segments {
			if segment.ID == "" {
				segment.ID = id.NewUUID()
			}
			segment.TranscriptID = transcript.ID
			segment.AssetID = transcript.AssetID
			segMeta, err := json.Marshal(orEmptyMap(segment.Metadata))
			if err != nil {
				return catalog.Transcript{}, err
			}
			_, err = db.pool.Exec(ctx, `
				insert into asset_transcript_segments(id, transcript_id, asset_id, start_ms, end_ms, text, confidence, speaker, metadata_json)
				values($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
			`, segment.ID, segment.TranscriptID, segment.AssetID, segment.StartMS, segment.EndMS, segment.Text, segment.Confidence, segment.Speaker, segMeta)
			if err != nil {
				return catalog.Transcript{}, err
			}
		}
	}
	out, err := db.ListTranscripts(ctx, transcript.AssetID, 100)
	if err != nil {
		return catalog.Transcript{}, err
	}
	for _, item := range out {
		if item.ID == transcript.ID {
			return item, nil
		}
	}
	return transcript, nil
}

func (db *DB) ListTranscripts(ctx context.Context, assetID string, limit int) ([]catalog.Transcript, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := db.pool.Query(ctx, `
		select id::text, asset_id::text, source_kind, language, model, full_text, created_at, metadata_json
		from asset_transcripts
		where asset_id=$1
		order by created_at desc
		limit $2
	`, assetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.Transcript{}
	for rows.Next() {
		var item catalog.Transcript
		var meta []byte
		if err := rows.Scan(&item.ID, &item.AssetID, &item.SourceKind, &item.Language, &item.Model, &item.FullText, &item.CreatedAt, &meta); err != nil {
			return nil, err
		}
		item.Metadata = decodeMap(meta)
		item.Segments, _ = db.listTranscriptSegments(ctx, item.ID, item.AssetID)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (db *DB) listTranscriptSegments(ctx context.Context, transcriptID, assetID string) ([]catalog.TranscriptSegment, error) {
	rows, err := db.pool.Query(ctx, `
		select id::text, transcript_id::text, asset_id::text, start_ms, end_ms, text, confidence, speaker, metadata_json
		from asset_transcript_segments
		where transcript_id=$1 and asset_id=$2
		order by start_ms, id
	`, transcriptID, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.TranscriptSegment{}
	for rows.Next() {
		var item catalog.TranscriptSegment
		var meta []byte
		if err := rows.Scan(&item.ID, &item.TranscriptID, &item.AssetID, &item.StartMS, &item.EndMS, &item.Text, &item.Confidence, &item.Speaker, &meta); err != nil {
			return nil, err
		}
		item.Metadata = decodeMap(meta)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (db *DB) UpsertAudioFeatures(ctx context.Context, features catalog.AudioFeatures) (catalog.AudioFeatures, error) {
	meta, err := json.Marshal(orEmptyMap(features.Metadata))
	if err != nil {
		return catalog.AudioFeatures{}, err
	}
	genres, err := json.Marshal(features.GenreLabels)
	if err != nil {
		return catalog.AudioFeatures{}, err
	}
	_, err = db.pool.Exec(ctx, `
		insert into audio_features(asset_id, duration_seconds, tempo_bpm, musical_key, musical_mode, loudness, speech_music_ratio, genre_labels, model, metadata_json)
		values($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10::jsonb)
		on conflict(asset_id) do update set
			duration_seconds=excluded.duration_seconds,
			tempo_bpm=excluded.tempo_bpm,
			musical_key=excluded.musical_key,
			musical_mode=excluded.musical_mode,
			loudness=excluded.loudness,
			speech_music_ratio=excluded.speech_music_ratio,
			genre_labels=excluded.genre_labels,
			model=excluded.model,
			created_at=now(),
			metadata_json=excluded.metadata_json
	`, features.AssetID, features.DurationSeconds, features.TempoBPM, features.Key, features.Mode, features.Loudness, features.SpeechMusicRatio, genres, features.Model, meta)
	if err != nil {
		return catalog.AudioFeatures{}, err
	}
	return db.GetAudioFeatures(ctx, features.AssetID)
}

func (db *DB) GetAudioFeatures(ctx context.Context, assetID string) (catalog.AudioFeatures, error) {
	var features catalog.AudioFeatures
	var genres, meta []byte
	err := db.pool.QueryRow(ctx, `
		select asset_id::text, duration_seconds, tempo_bpm, musical_key, musical_mode, loudness, speech_music_ratio, genre_labels, model, created_at, metadata_json
		from audio_features
		where asset_id=$1
	`, assetID).Scan(&features.AssetID, &features.DurationSeconds, &features.TempoBPM, &features.Key, &features.Mode, &features.Loudness, &features.SpeechMusicRatio, &genres, &features.Model, &features.CreatedAt, &meta)
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.AudioFeatures{}, catalog.ErrNotFound
	}
	if err != nil {
		return catalog.AudioFeatures{}, err
	}
	_ = json.Unmarshal(genres, &features.GenreLabels)
	features.Metadata = decodeMap(meta)
	return features, nil
}

func (db *DB) UpsertVideoFrameCaption(ctx context.Context, caption catalog.VideoFrameCaption) (catalog.VideoFrameCaption, error) {
	if caption.ID == "" {
		caption.ID = id.NewUUID()
	}
	meta, err := json.Marshal(orEmptyMap(caption.Metadata))
	if err != nil {
		return catalog.VideoFrameCaption{}, err
	}
	_, err = db.pool.Exec(ctx, `
		insert into video_frame_captions(id, asset_id, timestamp_ms, fraction, caption, model, metadata_json)
		values($1, $2, $3, $4, $5, $6, $7::jsonb)
		on conflict(id) do update set
			timestamp_ms=excluded.timestamp_ms,
			fraction=excluded.fraction,
			caption=excluded.caption,
			model=excluded.model,
			metadata_json=excluded.metadata_json
	`, caption.ID, caption.AssetID, caption.TimestampMS, caption.Fraction, caption.Caption, caption.Model, meta)
	if err != nil {
		return catalog.VideoFrameCaption{}, err
	}
	items, err := db.ListVideoFrameCaptions(ctx, caption.AssetID, 500)
	if err != nil {
		return catalog.VideoFrameCaption{}, err
	}
	for _, item := range items {
		if item.ID == caption.ID {
			return item, nil
		}
	}
	return caption, nil
}

func (db *DB) ListVideoFrameCaptions(ctx context.Context, assetID string, limit int) ([]catalog.VideoFrameCaption, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := db.pool.Query(ctx, `
		select id::text, asset_id::text, timestamp_ms, fraction, caption, model, created_at, metadata_json
		from video_frame_captions
		where asset_id=$1
		order by timestamp_ms, id
		limit $2
	`, assetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.VideoFrameCaption{}
	for rows.Next() {
		var item catalog.VideoFrameCaption
		var meta []byte
		if err := rows.Scan(&item.ID, &item.AssetID, &item.TimestampMS, &item.Fraction, &item.Caption, &item.Model, &item.CreatedAt, &meta); err != nil {
			return nil, err
		}
		item.Metadata = decodeMap(meta)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (db *DB) UpsertDocumentText(ctx context.Context, doc catalog.DocumentText) (catalog.DocumentText, error) {
	meta, err := json.Marshal(orEmptyMap(doc.Metadata))
	if err != nil {
		return catalog.DocumentText{}, err
	}
	_, err = db.pool.Exec(ctx, `
		insert into document_text(asset_id, page_count, title, author, text, markdown, engine, metadata_json)
		values($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
		on conflict(asset_id) do update set
			page_count=excluded.page_count,
			title=excluded.title,
			author=excluded.author,
			text=excluded.text,
			markdown=excluded.markdown,
			engine=excluded.engine,
			created_at=now(),
			metadata_json=excluded.metadata_json
	`, doc.AssetID, doc.PageCount, doc.Title, doc.Author, doc.Text, doc.Markdown, doc.Engine, meta)
	if err != nil {
		return catalog.DocumentText{}, err
	}
	return db.GetDocumentText(ctx, doc.AssetID)
}

func (db *DB) GetDocumentText(ctx context.Context, assetID string) (catalog.DocumentText, error) {
	var doc catalog.DocumentText
	var meta []byte
	err := db.pool.QueryRow(ctx, `
		select asset_id::text, page_count, title, author, text, markdown, engine, created_at, metadata_json
		from document_text
		where asset_id=$1
	`, assetID).Scan(&doc.AssetID, &doc.PageCount, &doc.Title, &doc.Author, &doc.Text, &doc.Markdown, &doc.Engine, &doc.CreatedAt, &meta)
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.DocumentText{}, catalog.ErrNotFound
	}
	if err != nil {
		return catalog.DocumentText{}, err
	}
	doc.Metadata = decodeMap(meta)
	return doc, nil
}

func orEmptyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

func decodeMap(data []byte) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	return out
}
