package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
)

func (db *DB) QueryAIMissingAssets(ctx context.Context, query catalog.AIMissingQuery) (catalog.AssetPage, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	where, args, err := aiMissingWhere(query)
	if err != nil {
		return catalog.AssetPage{}, err
	}
	countSQL := `select count(*) from assets a where ` + where
	var total int
	if err := db.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return catalog.AssetPage{}, err
	}
	idArgs := append([]any(nil), args...)
	idArgs = append(idArgs, limit, offset)
	idSQL := `select a.id::text
		from assets a
		where ` + where + fmt.Sprintf(`
		order by a.taken_at desc nulls last, a.display_name, a.id
		limit $%d offset $%d`, len(idArgs)-1, len(idArgs))
	rows, err := db.pool.Query(ctx, idSQL, idArgs...)
	if err != nil {
		return catalog.AssetPage{}, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return catalog.AssetPage{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return catalog.AssetPage{}, err
	}
	rows.Close()
	if len(ids) == 0 {
		return catalog.AssetPage{Assets: []catalog.Asset{}, Page: catalog.Page{Limit: limit, Offset: offset, Total: total}}, nil
	}
	rows, err = db.pool.Query(ctx, assetSelectSQL()+`
		where a.id::text = any($1)
		order by a.taken_at desc nulls last, a.display_name, l.url`, ids)
	if err != nil {
		return catalog.AssetPage{}, err
	}
	defer rows.Close()
	assets, err := scanAssets(rows)
	if err != nil {
		return catalog.AssetPage{}, err
	}
	return catalog.AssetPage{Assets: assets, Page: catalog.Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func aiMissingWhere(query catalog.AIMissingQuery) (string, []any, error) {
	task := strings.TrimSpace(query.Task)
	mediaKind := strings.TrimSpace(query.MediaKind)
	if mediaKind == "" {
		return "", nil, fmt.Errorf("AI missing asset query requires media kind")
	}
	var completionClause string
	switch task {
	case "classify_image", "safety_nsfw", "describe_image", "ocr_image":
		completionClause = `not exists(select 1 from ai_predictions p where p.asset_id=a.id and p.task=$2)`
	case "detect_faces":
		completionClause = `not exists(select 1 from face_detections f where f.asset_id=a.id)`
	case "embed_image":
		completionClause = `not exists(select 1 from asset_embeddings e where e.asset_id=a.id)`
	case "transcribe_audio":
		completionClause = `not exists(select 1 from asset_transcripts t where t.asset_id=a.id)`
	case "analyze_audio":
		completionClause = `not exists(select 1 from audio_features af where af.asset_id=a.id)`
	case "music_midi":
		completionClause = `not exists(select 1 from asset_midi_transcriptions mt where mt.asset_id=a.id and mt.status='succeeded')`
	case "music_stems":
		completionClause = `not exists(select 1 from asset_music_stems ms where ms.asset_id=a.id and ms.status='succeeded')`
	default:
		return "", nil, fmt.Errorf("unsupported AI missing task %q", task)
	}
	args := []any{mediaKind, task}
	clauses := []string{
		`a.media_kind=$1`,
		`not exists(select 1 from ai_asset_task_status ats where ats.asset_id=a.id and ats.task=$2 and (` +
			`ats.status in ('succeeded', 'skipped') or ` +
			`(ats.status='running' and ats.updated_at > now() - interval '6 hours') or (` +
			`ats.status='failed' and not (` + aiRetryableTaskFailureSQL("ats") + `)` +
			`)))`,
		completionClause,
	}
	if extensions := aiSupportedTaskExtensions(task, mediaKind); len(extensions) > 0 {
		args = append(args, extensions)
		clauses = append(clauses, fmt.Sprintf(`exists(
			select 1 from asset_locations al
			where al.asset_id=a.id and lower(trim(leading '.' from al.extension)) = any($%d)
		)`, len(args)))
	}
	if query.MaxDurationSeconds > 0 {
		args = append(args, query.MaxDurationSeconds)
		clauses = append(clauses, fmt.Sprintf(`coalesce(
			case when jsonb_typeof(a.metadata_json->'duration_seconds')='number' then (a.metadata_json->>'duration_seconds')::double precision end,
			case when jsonb_typeof(a.metadata_json->'duration')='number' then (a.metadata_json->>'duration')::double precision end,
			0
		) <= $%d`, len(args)))
	}
	if len(query.ExcludeAssetIDs) > 0 {
		args = append(args, query.ExcludeAssetIDs)
		clauses = append(clauses, fmt.Sprintf(`not (a.id::text = any($%d))`, len(args)))
	}
	return strings.Join(clauses, " and "), args, nil
}

func aiRetryableTaskFailureSQL(alias string) string {
	errorExpr := "lower(coalesce(" + alias + ".error, ''))"
	return strings.Join([]string{
		errorExpr + ` like '%connection refused%'`,
		errorExpr + ` like '%no ai sidecar is reachable%'`,
		errorExpr + ` like '%ai sidecar is not reachable%'`,
		errorExpr + ` like '%client.timeout exceeded%'`,
		errorExpr + ` like '%context deadline exceeded%'`,
		errorExpr + ` like '%temporary failure%'`,
		errorExpr + ` like '%connection reset by peer%'`,
		errorExpr + ` like '%unexpected eof%'`,
		errorExpr + ` = 'ai sidecar returned status error'`,
	}, " or ")
}

func aiSupportedTaskExtensions(task, mediaKind string) []string {
	switch task {
	case "classify_image", "safety_nsfw", "embed_image", "describe_image":
		return []string{"jpg", "jpeg", "png", "webp", "bmp", "gif", "tif", "tiff"}
	case "ocr_image":
		return []string{"jpg", "jpeg", "png", "webp", "bmp", "tif", "tiff"}
	case "detect_faces":
		return []string{"jpg", "jpeg", "png", "webp", "bmp", "tif", "tiff"}
	case "transcribe_audio":
		if mediaKind == "video" {
			return []string{"mp4", "mov", "m4v", "webm", "mkv", "avi", "3gp", "3gpp"}
		}
		return []string{"mp3", "wav", "flac", "ogg", "oga", "opus", "m4a", "aac", "amr", "3gp", "3gpp", "webm"}
	case "analyze_audio":
		return []string{"mp3", "wav", "flac", "ogg", "oga", "opus", "m4a", "aac", "amr", "3gp", "3gpp", "webm"}
	case "music_midi", "music_stems":
		if mediaKind == "video" {
			return []string{"mp4", "mov", "m4v", "webm", "mkv", "avi", "3gp", "3gpp"}
		}
		return []string{"mp3", "wav", "flac", "ogg", "oga", "opus", "m4a", "aac", "amr", "3gp", "3gpp", "webm"}
	default:
		return nil
	}
}

func (db *DB) UpsertAIAssetTaskStatus(ctx context.Context, status catalog.AIAssetTaskStatus) (catalog.AIAssetTaskStatus, error) {
	if strings.TrimSpace(status.Task) == "" {
		return catalog.AIAssetTaskStatus{}, fmt.Errorf("AI task is required")
	}
	if status.Status == "" {
		status.Status = "succeeded"
	}
	if status.Metadata == nil {
		status.Metadata = map[string]any{}
	}
	meta, err := json.Marshal(status.Metadata)
	if err != nil {
		return catalog.AIAssetTaskStatus{}, err
	}
	if status.UpdatedAt.IsZero() {
		status.UpdatedAt = time.Now().UTC()
	}
	err = db.pool.QueryRow(ctx, `
		insert into ai_asset_task_status(asset_id, task, status, worker_id, model_name, stored_count, error, metadata_json, updated_at)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9)
		on conflict(asset_id, task) do update set
			status=excluded.status,
			worker_id=excluded.worker_id,
			model_name=excluded.model_name,
			stored_count=excluded.stored_count,
			error=excluded.error,
			metadata_json=excluded.metadata_json,
			updated_at=excluded.updated_at
		returning updated_at`,
		status.AssetID, status.Task, status.Status, status.WorkerID, status.ModelName, status.StoredCount, status.Error, meta, status.UpdatedAt,
	).Scan(&status.UpdatedAt)
	if err != nil {
		return catalog.AIAssetTaskStatus{}, err
	}
	return status, nil
}
