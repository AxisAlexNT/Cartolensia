package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/id"
)

func (db *DB) QueryAssets(ctx context.Context, query catalog.AssetQuery) (catalog.AssetPage, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	where, args := assetQueryWhere(query)
	countSQL := `select count(distinct a.id)
		from assets a
		left join asset_locations l on l.asset_id=a.id
		left join storage_backends s on s.id=l.storage_id
		left join asset_geo g on g.asset_id=a.id`
	if where != "" {
		countSQL += " where " + where
	}
	var total int
	if err := db.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return catalog.AssetPage{}, err
	}
	orderBy := assetIDOrderBy(query.Sort)
	idArgs := append([]any(nil), args...)
	idArgs = append(idArgs, limit, offset)
	idSQL := `select a.id::text
		from assets a
		left join asset_locations l on l.asset_id=a.id
		left join storage_backends s on s.id=l.storage_id
		left join asset_geo g on g.asset_id=a.id`
	if where != "" {
		idSQL += " where " + where
	}
	idSQL += " group by a.id, a.display_name, a.media_kind, a.taken_at order by " + orderBy + fmt.Sprintf(" limit $%d offset $%d", len(idArgs)-1, len(idArgs))
	rows, err := db.pool.Query(ctx, idSQL, idArgs...)
	if err != nil {
		return catalog.AssetPage{}, err
	}
	var ids []string
	for rows.Next() {
		var assetID string
		if err := rows.Scan(&assetID); err != nil {
			rows.Close()
			return catalog.AssetPage{}, err
		}
		ids = append(ids, assetID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return catalog.AssetPage{}, err
	}
	rows.Close()
	if len(ids) == 0 {
		return catalog.AssetPage{Page: catalog.Page{Limit: limit, Offset: offset, Total: total}}, nil
	}
	rows, err = db.pool.Query(ctx, assetSelectSQL()+`
		where a.id::text = any($1)
		order by `+assetRowOrderBy(query.Sort)+`, l.url`, ids)
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

func assetQueryWhere(query catalog.AssetQuery) (string, []any) {
	var clauses []string
	var args []any
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	addRepeated := func(clause string, value any) {
		args = append(args, value)
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf(clause, idx, idx))
	}
	q := strings.TrimSpace(query.Q)
	if q != "" {
		addRepeated("(lower(a.display_name) like $%d or lower(coalesce(l.relative_path, '')) like $%d)", "%"+strings.ToLower(q)+"%")
	}
	if query.MediaKind != "" {
		addRepeated("(a.media_kind=$%d or l.media_kind=$%d)", query.MediaKind)
	}
	if query.HashStatus != "" {
		add("l.hash_status=$%d", query.HashStatus)
	}
	if query.Storage != "" {
		add("s.name=$%d", query.Storage)
	}
	if query.Extension != "" {
		add("lower(trim(leading '.' from l.extension))=$%d", strings.TrimPrefix(strings.ToLower(query.Extension), "."))
	}
	if query.TakenFrom != nil {
		add("a.taken_at >= $%d", *query.TakenFrom)
	}
	if query.TakenTo != nil {
		add("a.taken_at <= $%d", *query.TakenTo)
	}
	if query.AlbumID != "" {
		add("exists(select 1 from album_items ai where ai.asset_id=a.id and ai.album_id=$%d)", query.AlbumID)
	}
	if query.GeoSource != "" {
		add("g.source=$%d", query.GeoSource)
	}
	return strings.Join(clauses, " and "), args
}

func assetIDOrderBy(sortKey string) string {
	switch sortKey {
	case "size":
		return "min(l.size_bytes) asc nulls last, a.display_name, a.id"
	case "mtime":
		return "max(l.mtime) desc nulls last, a.display_name, a.id"
	case "media_kind":
		return "a.media_kind, a.display_name, a.id"
	case "taken_at":
		return "a.taken_at desc nulls last, a.display_name, a.id"
	default:
		return "a.display_name, a.id"
	}
}

func assetRowOrderBy(sortKey string) string {
	switch sortKey {
	case "size":
		return "l.size_bytes asc nulls last, a.display_name"
	case "mtime":
		return "l.mtime desc nulls last, a.display_name"
	case "media_kind":
		return "a.media_kind, a.display_name"
	case "taken_at":
		return "a.taken_at desc nulls last, a.display_name"
	default:
		return "a.display_name"
	}
}

func assetSelectSQL() string {
	return `select a.id::text, a.media_kind, a.display_name, a.taken_at, a.metadata_json, a.first_seen_at, a.updated_at,
			coalesce(l.id::text, ''), coalesce(l.storage_id::text, ''), coalesce(s.name, ''), coalesce(l.url, ''), coalesce(l.relative_path, ''),
			coalesce(l.file_name, ''), coalesce(l.extension, ''), coalesce(l.mime_type, ''), coalesce(l.media_kind, ''),
			coalesce(l.size_bytes, 0), coalesce(l.mtime, a.first_seen_at), coalesce(l.hash_status, ''), coalesce(encode(c.sha512, 'hex'), ''),
			coalesce(l.content_id::text, ''), coalesce(l.last_seen_at, a.first_seen_at)
		from assets a
		left join asset_locations l on l.asset_id=a.id
		left join storage_backends s on s.id=l.storage_id
		left join contents c on c.id=l.content_id `
}

func normalizeDBPage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (db *DB) CreateAlbum(ctx context.Context, album catalog.Album) (catalog.Album, error) {
	now := time.Now().UTC()
	if album.ID == "" {
		album.ID = id.NewUUID()
	}
	if album.Slug == "" {
		album.Slug = slugifyDB(album.Title)
	}
	if album.Title == "" {
		album.Title = album.Slug
	}
	if album.CreatedAt.IsZero() {
		album.CreatedAt = now
	}
	album.UpdatedAt = now
	_, err := db.pool.Exec(ctx, `
		insert into albums(id, parent_id, slug, title, description, sort_order, created_at, updated_at)
		values($1, $2, $3, $4, $5, $6, $7, $8)
	`, album.ID, nullString(album.ParentID), album.Slug, album.Title, album.Description, album.SortOrder, album.CreatedAt, album.UpdatedAt)
	if err != nil {
		return catalog.Album{}, err
	}
	return db.GetAlbum(ctx, album.ID)
}

func (db *DB) UpdateAlbum(ctx context.Context, album catalog.Album) (catalog.Album, error) {
	current, err := db.GetAlbum(ctx, album.ID)
	if err != nil {
		return catalog.Album{}, err
	}
	if album.Slug != "" {
		current.Slug = album.Slug
	}
	if album.Title != "" {
		current.Title = album.Title
	}
	current.ParentID = album.ParentID
	current.Description = album.Description
	current.SortOrder = album.SortOrder
	cmd, err := db.pool.Exec(ctx, `
		update albums
		set parent_id=$2, slug=$3, title=$4, description=$5, sort_order=$6, updated_at=now()
		where id=$1
	`, current.ID, nullString(current.ParentID), current.Slug, current.Title, current.Description, current.SortOrder)
	if err != nil {
		return catalog.Album{}, err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.Album{}, catalog.ErrNotFound
	}
	return db.GetAlbum(ctx, current.ID)
}

func (db *DB) DeleteAlbum(ctx context.Context, albumID string) error {
	cmd, err := db.pool.Exec(ctx, `delete from albums where id=$1`, albumID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

func (db *DB) GetAlbum(ctx context.Context, albumID string) (catalog.Album, error) {
	var album catalog.Album
	err := db.pool.QueryRow(ctx, `
		select a.id::text, coalesce(a.parent_id::text, ''), a.slug, a.title, a.description, a.sort_order,
			a.created_at, a.updated_at, count(ai.asset_id)::int
		from albums a
		left join album_items ai on ai.album_id=a.id
		where a.id=$1
		group by a.id
	`, albumID).Scan(&album.ID, &album.ParentID, &album.Slug, &album.Title, &album.Description, &album.SortOrder, &album.CreatedAt, &album.UpdatedAt, &album.ItemCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.Album{}, catalog.ErrNotFound
	}
	return album, err
}

func (db *DB) ListAlbums(ctx context.Context, query catalog.AlbumQuery) ([]catalog.Album, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	args := []any{}
	where := ""
	if !query.Tree {
		if query.ParentID == "" {
			where = "where a.parent_id is null"
		} else {
			args = append(args, query.ParentID)
			where = "where a.parent_id=$1"
		}
	}
	args = append(args, limit, offset)
	rows, err := db.pool.Query(ctx, `
		select a.id::text, coalesce(a.parent_id::text, ''), a.slug, a.title, a.description, a.sort_order,
			a.created_at, a.updated_at, count(ai.asset_id)::int
		from albums a
		left join album_items ai on ai.album_id=a.id
		`+where+`
		group by a.id
		order by a.sort_order, a.title
		limit $`+fmt.Sprint(len(args)-1)+` offset $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var albums []catalog.Album
	for rows.Next() {
		var album catalog.Album
		if err := rows.Scan(&album.ID, &album.ParentID, &album.Slug, &album.Title, &album.Description, &album.SortOrder, &album.CreatedAt, &album.UpdatedAt, &album.ItemCount); err != nil {
			return nil, err
		}
		albums = append(albums, album)
	}
	return albums, rows.Err()
}

func (db *DB) AddAlbumItems(ctx context.Context, albumID string, assetIDs []string) error {
	if _, err := db.GetAlbum(ctx, albumID); err != nil {
		return err
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	for _, assetID := range assetIDs {
		if _, err := tx.Exec(ctx, `
			insert into album_items(album_id, asset_id)
			values($1, $2)
			on conflict(album_id, asset_id) do nothing
		`, albumID, assetID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (db *DB) RemoveAlbumItem(ctx context.Context, albumID, assetID string) error {
	cmd, err := db.pool.Exec(ctx, `delete from album_items where album_id=$1 and asset_id=$2`, albumID, assetID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

func (db *DB) ListAlbumItems(ctx context.Context, query catalog.AlbumItemQuery) (catalog.AlbumItemPage, error) {
	if _, err := db.GetAlbum(ctx, query.AlbumID); err != nil {
		return catalog.AlbumItemPage{}, err
	}
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	var total int
	if err := db.pool.QueryRow(ctx, `select count(*) from album_items where album_id=$1`, query.AlbumID).Scan(&total); err != nil {
		return catalog.AlbumItemPage{}, err
	}
	rows, err := db.pool.Query(ctx, `
		select asset_id::text, note, sort_order, added_at
		from album_items
		where album_id=$1
		order by sort_order, added_at, asset_id
		limit $2 offset $3
	`, query.AlbumID, limit, offset)
	if err != nil {
		return catalog.AlbumItemPage{}, err
	}
	defer rows.Close()
	var items []catalog.AlbumItem
	for rows.Next() {
		var item catalog.AlbumItem
		item.AlbumID = query.AlbumID
		var assetID string
		if err := rows.Scan(&assetID, &item.Note, &item.SortOrder, &item.AddedAt); err != nil {
			return catalog.AlbumItemPage{}, err
		}
		asset, err := db.GetAsset(ctx, assetID)
		if err != nil {
			return catalog.AlbumItemPage{}, err
		}
		item.Asset = asset
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return catalog.AlbumItemPage{}, err
	}
	return catalog.AlbumItemPage{Items: items, Page: catalog.Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (db *DB) UpsertAssetGeo(ctx context.Context, geo catalog.AssetGeo, force bool) (catalog.AssetGeo, error) {
	current, err := db.GetAssetGeo(ctx, geo.AssetID)
	if err == nil && !force && geoPriorityDB(geo.Source) < geoPriorityDB(current.Source) {
		return current, nil
	}
	if err != nil && !errors.Is(err, catalog.ErrNotFound) {
		return catalog.AssetGeo{}, err
	}
	if geo.Source == "" {
		geo.Source = "unknown"
	}
	if geo.Metadata == nil {
		geo.Metadata = map[string]any{}
	}
	metadata, err := json.Marshal(geo.Metadata)
	if err != nil {
		return catalog.AssetGeo{}, err
	}
	_, err = db.pool.Exec(ctx, `
		insert into asset_geo(asset_id, lat, lon, source, confidence, taken_at, track_asset_id, metadata_json)
		values($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict(asset_id) do update set
			lat=excluded.lat,
			lon=excluded.lon,
			source=excluded.source,
			confidence=excluded.confidence,
			taken_at=coalesce(excluded.taken_at, asset_geo.taken_at),
			track_asset_id=excluded.track_asset_id,
			metadata_json=asset_geo.metadata_json || excluded.metadata_json,
			updated_at=now()
	`, geo.AssetID, geo.Lat, geo.Lon, geo.Source, geo.Confidence, geo.TakenAt, nullString(geo.TrackAssetID), metadata)
	if err != nil {
		return catalog.AssetGeo{}, err
	}
	return db.GetAssetGeo(ctx, geo.AssetID)
}

func (db *DB) GetAssetGeo(ctx context.Context, assetID string) (catalog.AssetGeo, error) {
	rows, err := db.pool.Query(ctx, `
		select asset_id::text, lat, lon, source, confidence, taken_at, coalesce(track_asset_id::text, ''),
			metadata_json, created_at, updated_at
		from asset_geo where asset_id=$1
	`, assetID)
	if err != nil {
		return catalog.AssetGeo{}, err
	}
	defer rows.Close()
	geos, err := scanAssetGeos(rows)
	if err != nil {
		return catalog.AssetGeo{}, err
	}
	if len(geos) == 0 {
		return catalog.AssetGeo{}, catalog.ErrNotFound
	}
	return geos[0], nil
}

func (db *DB) QueryAssetGeo(ctx context.Context, query catalog.GeoQuery) ([]catalog.GeoAsset, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	where, args := geoQueryWhere(query)
	args = append(args, limit, offset)
	sql := `select g.asset_id::text, g.lat, g.lon, g.source, g.confidence, g.taken_at, coalesce(g.track_asset_id::text, ''),
			g.metadata_json, g.created_at, g.updated_at
		from asset_geo g
		join assets a on a.id=g.asset_id
		left join asset_locations l on l.asset_id=a.id
		left join album_items ai on ai.asset_id=a.id`
	if where != "" {
		sql += " where " + where
	}
	sql += fmt.Sprintf(" group by g.asset_id order by min(a.display_name) limit $%d offset $%d", len(args)-1, len(args))
	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	geos, err := scanAssetGeos(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := make([]catalog.GeoAsset, 0, len(geos))
	for _, geo := range geos {
		asset, err := db.GetAsset(ctx, geo.AssetID)
		if err != nil {
			return nil, err
		}
		out = append(out, catalog.GeoAsset{Asset: asset, Geo: geo})
	}
	return out, nil
}

func geoQueryWhere(query catalog.GeoQuery) (string, []any) {
	var clauses []string
	var args []any
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if query.BBox != nil {
		add("g.lon >= $%d", query.BBox.MinLon)
		add("g.lon <= $%d", query.BBox.MaxLon)
		add("g.lat >= $%d", query.BBox.MinLat)
		add("g.lat <= $%d", query.BBox.MaxLat)
	}
	if query.Source != "" {
		add("g.source=$%d", query.Source)
	}
	if query.MediaKind != "" {
		add("a.media_kind=$%d", query.MediaKind)
	}
	if query.AlbumID != "" {
		add("ai.album_id=$%d", query.AlbumID)
	}
	if query.TrackID != "" {
		add("g.track_asset_id=$%d", query.TrackID)
	}
	if query.TimeFrom != nil {
		add("coalesce(g.taken_at, a.taken_at) >= $%d", *query.TimeFrom)
	}
	if query.TimeTo != nil {
		add("coalesce(g.taken_at, a.taken_at) <= $%d", *query.TimeTo)
	}
	return strings.Join(clauses, " and "), args
}

func scanAssetGeos(rows pgx.Rows) ([]catalog.AssetGeo, error) {
	defer rows.Close()
	var out []catalog.AssetGeo
	for rows.Next() {
		var geo catalog.AssetGeo
		var metadata []byte
		if err := rows.Scan(&geo.AssetID, &geo.Lat, &geo.Lon, &geo.Source, &geo.Confidence, &geo.TakenAt, &geo.TrackAssetID, &metadata, &geo.CreatedAt, &geo.UpdatedAt); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &geo.Metadata)
		}
		if geo.Metadata == nil {
			geo.Metadata = map[string]any{}
		}
		out = append(out, geo)
	}
	return out, rows.Err()
}

func geoPriorityDB(source string) int {
	switch source {
	case "real", "exif", "manual":
		return 100
	case "track_snapped":
		return 70
	case "estimated":
		return 40
	default:
		return 0
	}
}

func (db *DB) UpsertGPSTrackSummary(ctx context.Context, summary catalog.TrackSummary, metadata map[string]any) error {
	if summary.TrackAssetID == "" {
		return catalog.ErrNotFound
	}
	title := summary.Name
	if title == "" {
		title = summary.TrackAssetID
	}
	description, _ := metadata["description"].(string)
	metadataBytes, err := json.Marshal(metadataOrEmpty(metadata))
	if err != nil {
		return err
	}
	_, err = db.pool.Exec(ctx, `
		insert into gps_tracks(track_asset_id, title, description, point_count, start_at, end_at, min_lat, min_lon, max_lat, max_lon,
			distance_m, duration_seconds, elevation_min_m, elevation_max_m, metadata_json)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		on conflict(track_asset_id) do update set
			title=excluded.title,
			description=excluded.description,
			point_count=excluded.point_count,
			start_at=excluded.start_at,
			end_at=excluded.end_at,
			min_lat=excluded.min_lat,
			min_lon=excluded.min_lon,
			max_lat=excluded.max_lat,
			max_lon=excluded.max_lon,
			distance_m=excluded.distance_m,
			duration_seconds=excluded.duration_seconds,
			elevation_min_m=excluded.elevation_min_m,
			elevation_max_m=excluded.elevation_max_m,
			metadata_json=gps_tracks.metadata_json || excluded.metadata_json,
			updated_at=now()
	`, summary.TrackAssetID, title, description, summary.PointCount, summary.StartTime, summary.EndTime, summary.MinLat, summary.MinLon,
		summary.MaxLat, summary.MaxLon, summary.DistanceM, summary.DurationSec, summary.ElevationMin, summary.ElevationMax, metadataBytes)
	return err
}

func (db *DB) ListGPSTracks(ctx context.Context, query catalog.GPSTrackQuery) ([]catalog.TrackSummary, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	var clauses []string
	var args []any
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	addRepeated := func(clause string, value any) {
		args = append(args, value)
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf(clause, idx, idx))
	}
	if query.Q != "" {
		addRepeated("(lower(title) like $%d or lower(description) like $%d)", "%"+strings.ToLower(query.Q)+"%")
	}
	if query.TimeFrom != nil {
		add("end_at >= $%d", *query.TimeFrom)
	}
	if query.TimeTo != nil {
		add("start_at <= $%d", *query.TimeTo)
	}
	if query.BBox != nil {
		add("max_lon >= $%d", query.BBox.MinLon)
		add("min_lon <= $%d", query.BBox.MaxLon)
		add("max_lat >= $%d", query.BBox.MinLat)
		add("min_lat <= $%d", query.BBox.MaxLat)
	}
	args = append(args, limit, offset)
	sql := `select track_asset_id::text, title, point_count, start_at, end_at, min_lat, min_lon, max_lat, max_lon,
			distance_m, duration_seconds, elevation_min_m, elevation_max_m
		from gps_tracks`
	if len(clauses) > 0 {
		sql += " where " + strings.Join(clauses, " and ")
	}
	sql += " order by " + gpsTrackOrder(query.Sort) + fmt.Sprintf(" limit $%d offset $%d", len(args)-1, len(args))
	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.TrackSummary
	for rows.Next() {
		var summary catalog.TrackSummary
		if err := rows.Scan(&summary.TrackAssetID, &summary.Name, &summary.PointCount, &summary.StartTime, &summary.EndTime, &summary.MinLat, &summary.MinLon,
			&summary.MaxLat, &summary.MaxLon, &summary.DistanceM, &summary.DurationSec, &summary.ElevationMin, &summary.ElevationMax); err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 && query.Q == "" && query.BBox == nil && query.TimeFrom == nil && query.TimeTo == nil && offset == 0 {
		legacy, err := db.ListTracks(ctx)
		if err != nil {
			return nil, err
		}
		if len(legacy) > limit {
			legacy = legacy[:limit]
		}
		return legacy, nil
	}
	return out, nil
}

func gpsTrackOrder(sortKey string) string {
	switch sortKey {
	case "name":
		return "title, track_asset_id"
	case "points":
		return "point_count desc, title"
	case "duration":
		return "duration_seconds desc nulls last, title"
	default:
		return "start_at desc nulls last, title"
	}
}

func (db *DB) UpdateGPSTrackMetadata(ctx context.Context, trackAssetID, title, description string) error {
	cmd, err := db.pool.Exec(ctx, `
		update gps_tracks
		set title=case when $2 <> '' then $2 else title end,
			description=$3,
			updated_at=now()
		where track_asset_id=$1
	`, trackAssetID, title, description)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	metadata := map[string]any{"gps_track_description": description}
	return db.UpdateAssetMetadata(ctx, trackAssetID, nil, metadata)
}

func (db *DB) QueryTrackPoints(ctx context.Context, query catalog.TrackPointQuery) ([]catalog.TrackPoint, error) {
	var clauses []string
	var args []any
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	add("track_asset_id=$%d", query.TrackAssetID)
	if query.TimeFrom != nil {
		add("recorded_at >= $%d", *query.TimeFrom)
	}
	if query.TimeTo != nil {
		add("recorded_at <= $%d", *query.TimeTo)
	}
	rows, err := db.pool.Query(ctx, `
		select id, track_asset_id::text, recorded_at, lat, lon, elevation_m, speed_mps, source
		from track_points
		where `+strings.Join(clauses, " and ")+`
		order by recorded_at, id
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var points []catalog.TrackPoint
	for rows.Next() {
		var point catalog.TrackPoint
		if err := rows.Scan(&point.ID, &point.TrackAssetID, &point.RecordedAt, &point.Lat, &point.Lon, &point.ElevationM, &point.SpeedMPS, &point.Source); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if query.Simplify {
		maxPoints := query.MaxPoints
		if maxPoints <= 0 {
			maxPoints = 500
		}
		points = downsampleTrackPoints(points, maxPoints)
	}
	return points, nil
}

func (db *DB) QueryTrackAssets(ctx context.Context, query catalog.TrackAssetQuery) (catalog.AssetPage, error) {
	tracks, err := db.ListGPSTracks(ctx, catalog.GPSTrackQuery{Limit: 500})
	if err != nil {
		return catalog.AssetPage{}, err
	}
	var track catalog.TrackSummary
	found := false
	for _, candidate := range tracks {
		if candidate.TrackAssetID == query.TrackAssetID {
			track = candidate
			found = true
			break
		}
	}
	if !found {
		detail, err := db.GetTrack(ctx, query.TrackAssetID)
		if err != nil {
			return catalog.AssetPage{}, err
		}
		track = detail.Summary
	}
	if track.StartTime == nil || track.EndTime == nil {
		limit, offset := normalizeDBPage(query.Limit, query.Offset)
		return catalog.AssetPage{Page: catalog.Page{Limit: limit, Offset: offset}}, nil
	}
	start := track.StartTime.Add(time.Duration(query.OffsetSeconds) * time.Second)
	end := track.EndTime.Add(time.Duration(query.OffsetSeconds) * time.Second)
	page, err := db.QueryAssets(ctx, catalog.AssetQuery{MediaKind: query.MediaKind, TakenFrom: &start, TakenTo: &end, Limit: query.Limit, Offset: query.Offset, Sort: "taken_at"})
	if err != nil {
		return catalog.AssetPage{}, err
	}
	filtered := page.Assets[:0]
	for _, asset := range page.Assets {
		_, err := db.GetAssetGeo(ctx, asset.ID)
		geotagged := err == nil
		if geotagged && !query.IncludeGeotagged {
			continue
		}
		if !geotagged && !query.IncludeUngeotagged {
			continue
		}
		filtered = append(filtered, asset)
	}
	page.Assets = filtered
	page.Page.Total = len(filtered)
	return page, nil
}

func downsampleTrackPoints(points []catalog.TrackPoint, maxPoints int) []catalog.TrackPoint {
	if maxPoints <= 0 || len(points) <= maxPoints {
		return points
	}
	if maxPoints == 1 {
		return []catalog.TrackPoint{points[0]}
	}
	step := float64(len(points)-1) / float64(maxPoints-1)
	out := make([]catalog.TrackPoint, 0, maxPoints)
	for i := 0; i < maxPoints; i++ {
		idx := int(float64(i) * step)
		if idx >= len(points) {
			idx = len(points) - 1
		}
		out = append(out, points[idx])
	}
	return out
}

func (db *DB) CreateScanRun(ctx context.Context, run catalog.ScanRun) (catalog.ScanRun, error) {
	if run.ID == "" {
		run.ID = id.NewUUID()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	prefixes, err := json.Marshal(run.Prefixes)
	if err != nil {
		return catalog.ScanRun{}, err
	}
	report, err := json.Marshal(metadataOrEmpty(run.Report))
	if err != nil {
		return catalog.ScanRun{}, err
	}
	_, err = db.pool.Exec(ctx, `
		insert into scan_runs(id, job_id, storage_name, mode, prefixes_json, max_files, max_bytes, hash_requested,
			metadata_requested, previews_requested, mark_missing, dry_run, started_at, finished_at, report_json, created_at)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, run.ID, nullString(run.JobID), run.StorageName, run.Mode, prefixes, run.MaxFiles, run.MaxBytes, run.HashRequested,
		run.MetadataRequested, run.PreviewsRequested, run.MarkMissing, run.DryRun, run.StartedAt, run.FinishedAt, report, run.CreatedAt)
	if err != nil {
		return catalog.ScanRun{}, err
	}
	if run.JobID != "" {
		return db.GetScanRunByJob(ctx, run.JobID)
	}
	return db.getScanRun(ctx, run.ID)
}

func (db *DB) UpdateScanRunReport(ctx context.Context, runID string, report map[string]any) error {
	data, err := json.Marshal(metadataOrEmpty(report))
	if err != nil {
		return err
	}
	cmd, err := db.pool.Exec(ctx, `update scan_runs set report_json=$2 where id=$1`, runID, data)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

func (db *DB) FinishScanRun(ctx context.Context, runID string, report map[string]any) error {
	data, err := json.Marshal(metadataOrEmpty(report))
	if err != nil {
		return err
	}
	cmd, err := db.pool.Exec(ctx, `update scan_runs set report_json=$2, finished_at=now() where id=$1`, runID, data)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

func (db *DB) GetScanRunByJob(ctx context.Context, jobID string) (catalog.ScanRun, error) {
	rows, err := db.pool.Query(ctx, `
		select id::text, coalesce(job_id::text, ''), storage_name, mode, prefixes_json, max_files, max_bytes,
			hash_requested, metadata_requested, previews_requested, mark_missing, dry_run, started_at, finished_at, report_json, created_at
		from scan_runs where job_id=$1 order by created_at desc limit 1
	`, jobID)
	if err != nil {
		return catalog.ScanRun{}, err
	}
	defer rows.Close()
	runs, err := scanRuns(rows)
	if err != nil {
		return catalog.ScanRun{}, err
	}
	if len(runs) == 0 {
		return catalog.ScanRun{}, catalog.ErrNotFound
	}
	return runs[0], nil
}

func (db *DB) getScanRun(ctx context.Context, runID string) (catalog.ScanRun, error) {
	rows, err := db.pool.Query(ctx, `
		select id::text, coalesce(job_id::text, ''), storage_name, mode, prefixes_json, max_files, max_bytes,
			hash_requested, metadata_requested, previews_requested, mark_missing, dry_run, started_at, finished_at, report_json, created_at
		from scan_runs where id=$1
	`, runID)
	if err != nil {
		return catalog.ScanRun{}, err
	}
	defer rows.Close()
	runs, err := scanRuns(rows)
	if err != nil {
		return catalog.ScanRun{}, err
	}
	if len(runs) == 0 {
		return catalog.ScanRun{}, catalog.ErrNotFound
	}
	return runs[0], nil
}

func (db *DB) ListScanRuns(ctx context.Context, query catalog.ScanRunQuery) ([]catalog.ScanRun, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	args := []any{}
	where := ""
	if query.StorageName != "" {
		args = append(args, query.StorageName)
		where = "where storage_name=$1"
	}
	args = append(args, limit, offset)
	rows, err := db.pool.Query(ctx, `
		select id::text, coalesce(job_id::text, ''), storage_name, mode, prefixes_json, max_files, max_bytes,
			hash_requested, metadata_requested, previews_requested, mark_missing, dry_run, started_at, finished_at, report_json, created_at
		from scan_runs
		`+where+`
		order by created_at desc
		limit $`+fmt.Sprint(len(args)-1)+` offset $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

func scanRuns(rows pgx.Rows) ([]catalog.ScanRun, error) {
	var out []catalog.ScanRun
	for rows.Next() {
		var run catalog.ScanRun
		var prefixes []byte
		var report []byte
		if err := rows.Scan(&run.ID, &run.JobID, &run.StorageName, &run.Mode, &prefixes, &run.MaxFiles, &run.MaxBytes, &run.HashRequested,
			&run.MetadataRequested, &run.PreviewsRequested, &run.MarkMissing, &run.DryRun, &run.StartedAt, &run.FinishedAt, &report, &run.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(prefixes, &run.Prefixes)
		_ = json.Unmarshal(report, &run.Report)
		if run.Report == nil {
			run.Report = map[string]any{}
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (db *DB) UpsertPreviewCacheEntry(ctx context.Context, entry catalog.PreviewCacheEntry) (catalog.PreviewCacheEntry, error) {
	if entry.ID == "" {
		entry.ID = id.NewUUID()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	_, err := db.pool.Exec(ctx, `
		insert into preview_cache_entries(id, asset_id, content_id, variant, width, height, format, cache_path, status,
			size_bytes, created_at, last_accessed_at, error)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		on conflict(asset_id, variant, width, height, format) do update set
			content_id=excluded.content_id,
			cache_path=excluded.cache_path,
			status=excluded.status,
			size_bytes=excluded.size_bytes,
			last_accessed_at=excluded.last_accessed_at,
			error=excluded.error
	`, entry.ID, entry.AssetID, nullString(entry.ContentID), entry.Variant, entry.Width, entry.Height, entry.Format, entry.CachePath,
		entry.Status, entry.SizeBytes, entry.CreatedAt, entry.LastAccessedAt, nullString(entry.Error))
	if err != nil {
		return catalog.PreviewCacheEntry{}, err
	}
	return db.GetPreviewCacheEntry(ctx, entry.AssetID, entry.Variant, entry.Width, entry.Height, entry.Format)
}

func (db *DB) GetPreviewCacheEntry(ctx context.Context, assetID, variant string, width, height int, format string) (catalog.PreviewCacheEntry, error) {
	rows, err := db.pool.Query(ctx, `
		select id::text, asset_id::text, coalesce(content_id::text, ''), variant, width, height, format, cache_path,
			status, size_bytes, created_at, last_accessed_at, coalesce(error, '')
		from preview_cache_entries
		where asset_id=$1 and variant=$2 and width=$3 and height=$4 and format=$5
	`, assetID, variant, width, height, format)
	if err != nil {
		return catalog.PreviewCacheEntry{}, err
	}
	defer rows.Close()
	entries, err := scanPreviewCacheEntries(rows)
	if err != nil {
		return catalog.PreviewCacheEntry{}, err
	}
	if len(entries) == 0 {
		return catalog.PreviewCacheEntry{}, catalog.ErrNotFound
	}
	return entries[0], nil
}

func (db *DB) ListPreviewCacheEntries(ctx context.Context, query catalog.PreviewCacheQuery) ([]catalog.PreviewCacheEntry, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	var clauses []string
	var args []any
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if query.AssetID != "" {
		add("asset_id=$%d", query.AssetID)
	}
	if query.Status != "" {
		add("status=$%d", query.Status)
	}
	args = append(args, limit, offset)
	sql := `select id::text, asset_id::text, coalesce(content_id::text, ''), variant, width, height, format, cache_path,
			status, size_bytes, created_at, last_accessed_at, coalesce(error, '')
		from preview_cache_entries`
	if len(clauses) > 0 {
		sql += " where " + strings.Join(clauses, " and ")
	}
	sql += fmt.Sprintf(" order by created_at desc limit $%d offset $%d", len(args)-1, len(args))
	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPreviewCacheEntries(rows)
}

func (db *DB) MarkPreviewAccessed(ctx context.Context, entryID string) error {
	cmd, err := db.pool.Exec(ctx, `update preview_cache_entries set last_accessed_at=now() where id=$1`, entryID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

func (db *DB) PreviewCacheStats(ctx context.Context) (catalog.PreviewCacheStats, error) {
	var stats catalog.PreviewCacheStats
	err := db.pool.QueryRow(ctx, `
		select count(*)::int,
			coalesce(sum(case when status='ready' then 1 else 0 end), 0)::int,
			coalesce(sum(case when status='failed' then 1 else 0 end), 0)::int,
			coalesce(sum(size_bytes), 0),
			coalesce(extract(epoch from min(created_at))::bigint, 0)
		from preview_cache_entries
	`).Scan(&stats.Entries, &stats.Ready, &stats.Failed, &stats.Bytes, &stats.OldestUnix)
	return stats, err
}

func (db *DB) CleanupPreviewCacheEntries(ctx context.Context, olderThan time.Time, maxBytes int64) ([]catalog.PreviewCacheEntry, error) {
	entries, err := db.ListPreviewCacheEntries(ctx, catalog.PreviewCacheQuery{Limit: 500})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.Before(entries[j].CreatedAt) })
	var total int64
	for _, entry := range entries {
		total += entry.SizeBytes
	}
	var deleted []catalog.PreviewCacheEntry
	for _, entry := range entries {
		if (olderThan.IsZero() || !entry.CreatedAt.Before(olderThan)) && (maxBytes <= 0 || total <= maxBytes) {
			continue
		}
		cmd, err := db.pool.Exec(ctx, `delete from preview_cache_entries where id=$1`, entry.ID)
		if err != nil {
			return nil, err
		}
		if cmd.RowsAffected() > 0 {
			deleted = append(deleted, entry)
			total -= entry.SizeBytes
		}
	}
	return deleted, nil
}

func scanPreviewCacheEntries(rows pgx.Rows) ([]catalog.PreviewCacheEntry, error) {
	var out []catalog.PreviewCacheEntry
	for rows.Next() {
		var entry catalog.PreviewCacheEntry
		if err := rows.Scan(&entry.ID, &entry.AssetID, &entry.ContentID, &entry.Variant, &entry.Width, &entry.Height, &entry.Format, &entry.CachePath,
			&entry.Status, &entry.SizeBytes, &entry.CreatedAt, &entry.LastAccessedAt, &entry.Error); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func metadataOrEmpty(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}

func slugifyDB(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "album"
	}
	return out
}
