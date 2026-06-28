package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/id"
)

func (db *DB) ExplorerView(ctx context.Context, opts catalog.ExplorerOptions) (catalog.ExplorerView, error) {
	current, err := normalizeExplorerDBPath(opts.Path)
	if err != nil {
		return catalog.ExplorerView{}, err
	}
	limit, offset := normalizeDBPage(opts.Limit, opts.Offset)
	prefix := ""
	substrStart := 1
	if current != "" {
		prefix = current + "/"
		substrStart = len(prefix) + 1
	}
	where, args := explorerLocationWhere(opts, current, prefix)
	remainingExpr := fmt.Sprintf("case when $%d = '' then l.relative_path else substr(l.relative_path, $%d) end", len(args)+1, len(args)+2)
	argsWithPath := append(append([]any(nil), args...), current, substrStart)
	filteredSQL := ` from asset_locations l
		join assets a on a.id=l.asset_id
		join storage_backends s on s.id=l.storage_id
		left join contents c on c.id=l.content_id`
	if where != "" {
		filteredSQL += " where " + where
	}
	folderNameExpr := `split_part(` + remainingExpr + `, '/', 1)`
	folderSQL := `select ` + folderNameExpr + ` as folder_name,
			count(*)::bigint,
			coalesce(sum(l.size_bytes), 0)::bigint,
			max(l.mtime)
		` + filteredSQL + `
		group by ` + folderNameExpr + `
		having ` + folderNameExpr + ` <> '' and count(*) filter (where position('/' in ` + remainingExpr + `) > 0) > 0
		order by ` + folderNameExpr
	rows, err := db.pool.Query(ctx, folderSQL, argsWithPath...)
	if err != nil {
		return catalog.ExplorerView{}, err
	}
	view := catalog.ExplorerView{
		CurrentPath: current,
		ParentPath:  parentExplorerDBPath(current),
		Folders:     []catalog.ExplorerFolder{},
		Files:       []catalog.ExplorerFile{},
		Limit:       limit,
		Offset:      offset,
	}
	for rows.Next() {
		var name string
		var count int64
		var bytes int64
		var latest time.Time
		if err := rows.Scan(&name, &count, &bytes, &latest); err != nil {
			rows.Close()
			return catalog.ExplorerView{}, err
		}
		view.Folders = append(view.Folders, catalog.ExplorerFolder{
			Name:        name,
			Path:        joinExplorerDBPath(current, name),
			FileCount:   int(count),
			TotalBytes:  bytes,
			LatestMTime: latest,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return catalog.ExplorerView{}, err
	}
	rows.Close()
	view.FolderCount = len(view.Folders)

	totalSQL := `select count(*)::bigint, coalesce(sum(l.size_bytes), 0)::bigint
		` + filteredSQL + `
		and ` + remainingExpr + ` <> ''
		and position('/' in ` + remainingExpr + `) = 0`
	if where == "" {
		totalSQL = `select count(*)::bigint, coalesce(sum(l.size_bytes), 0)::bigint
			` + filteredSQL + `
			where ` + remainingExpr + ` <> ''
			and position('/' in ` + remainingExpr + `) = 0`
	}
	var fileCount int64
	var directBytes int64
	if err := db.pool.QueryRow(ctx, totalSQL, argsWithPath...).Scan(&fileCount, &directBytes); err != nil {
		return catalog.ExplorerView{}, err
	}
	view.FileCount = int(fileCount)
	view.TotalBytes = directBytes
	for _, folder := range view.Folders {
		view.TotalBytes += folder.TotalBytes
	}

	fileArgs := append(append([]any(nil), argsWithPath...), limit, offset)
	fileSQL := `select a.id::text, l.file_name, l.media_kind, l.url, l.relative_path,
			l.size_bytes, l.mtime, l.hash_status, coalesce(encode(c.sha512, 'hex'), '')
		` + filteredSQL + `
		and ` + remainingExpr + ` <> ''
		and position('/' in ` + remainingExpr + `) = 0
		order by ` + explorerLocationOrderBy(opts.Sort) + fmt.Sprintf(" limit $%d offset $%d", len(fileArgs)-1, len(fileArgs))
	if where == "" {
		fileSQL = `select a.id::text, l.file_name, l.media_kind, l.url, l.relative_path,
				l.size_bytes, l.mtime, l.hash_status, coalesce(encode(c.sha512, 'hex'), '')
			` + filteredSQL + `
			where ` + remainingExpr + ` <> ''
			and position('/' in ` + remainingExpr + `) = 0
			order by ` + explorerLocationOrderBy(opts.Sort) + fmt.Sprintf(" limit $%d offset $%d", len(fileArgs)-1, len(fileArgs))
	}
	rows, err = db.pool.Query(ctx, fileSQL, fileArgs...)
	if err != nil {
		return catalog.ExplorerView{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var file catalog.ExplorerFile
		if err := rows.Scan(&file.AssetID, &file.Name, &file.MediaKind, &file.StorageURL, &file.RelativePath, &file.SizeBytes, &file.MTime, &file.HashStatus, &file.SHA512Hex); err != nil {
			return catalog.ExplorerView{}, err
		}
		view.Files = append(view.Files, file)
	}
	return view, rows.Err()
}

func explorerLocationWhere(opts catalog.ExplorerOptions, current, prefix string) (string, []any) {
	var parts []string
	var args []any
	if opts.Storage != "" {
		args = append(args, opts.Storage)
		parts = append(parts, fmt.Sprintf("s.name=$%d", len(args)))
	}
	if opts.MediaKind != "" {
		args = append(args, opts.MediaKind)
		parts = append(parts, fmt.Sprintf("l.media_kind=$%d", len(args)))
	}
	if opts.HashStatus != "" {
		args = append(args, opts.HashStatus)
		parts = append(parts, fmt.Sprintf("l.hash_status=$%d", len(args)))
	}
	if opts.Extension != "" {
		args = append(args, strings.TrimPrefix(strings.ToLower(opts.Extension), "."))
		parts = append(parts, fmt.Sprintf("lower(trim(leading '.' from l.extension))=$%d", len(args)))
	}
	if q := strings.TrimSpace(opts.Q); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		parts = append(parts, fmt.Sprintf("lower(a.display_name || ' ' || l.file_name || ' ' || l.relative_path) like $%d", len(args)))
	}
	if current != "" {
		args = append(args, prefix+"%")
		parts = append(parts, fmt.Sprintf("l.relative_path like $%d", len(args)))
	}
	return strings.Join(parts, " and "), args
}

func explorerLocationOrderBy(sortKey string) string {
	switch strings.TrimSpace(strings.ToLower(sortKey)) {
	case "mtime", "modified", "taken_at":
		return "l.mtime desc, lower(l.file_name), l.id"
	case "size":
		return "l.size_bytes desc, lower(l.file_name), l.id"
	case "media_kind", "kind":
		return "l.media_kind, lower(l.file_name), l.id"
	default:
		return "lower(l.file_name), l.file_name, l.id"
	}
}

func normalizeExplorerDBPath(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	value = strings.Trim(value, "/")
	if value == "" || value == "." {
		return "", nil
	}
	clean := path.Clean(value)
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid explorer path %q", value)
	}
	return clean, nil
}

func parentExplorerDBPath(value string) string {
	if value == "" {
		return ""
	}
	parent := path.Dir(value)
	if parent == "." {
		return ""
	}
	return parent
}

func joinExplorerDBPath(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}

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
		return catalog.AssetPage{Assets: []catalog.Asset{}, Page: catalog.Page{Limit: limit, Offset: offset, Total: total}}, nil
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
	if len(query.Prefixes) > 0 {
		var prefixClauses []string
		for _, prefix := range query.Prefixes {
			prefix = strings.Trim(strings.TrimSpace(prefix), "/")
			if prefix == "" {
				continue
			}
			args = append(args, prefix)
			exactIdx := len(args)
			args = append(args, prefix+"/%")
			childIdx := len(args)
			prefixClauses = append(prefixClauses, fmt.Sprintf("(trim(both '/' from coalesce(l.relative_path, '')) = $%d or trim(both '/' from coalesce(l.relative_path, '')) like $%d)", exactIdx, childIdx))
		}
		if len(prefixClauses) > 0 {
			clauses = append(clauses, "("+strings.Join(prefixClauses, " or ")+")")
		}
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
	if query.PublicOnly {
		clauses = append(clauses, `(a.metadata_json @> '{"public": true}'::jsonb or a.metadata_json @> '{"is_public": true}'::jsonb or a.metadata_json @> '{"visibility_public": true}'::jsonb or lower(coalesce(a.metadata_json->>'visibility', '')) = 'public')`)
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
	albums := []catalog.Album{}
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
	metadata, err := json.Marshal(metadataOrEmpty(geo.Metadata))
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
	limit, offset := normalizeGeoPage(query.Limit, query.Offset)
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

func normalizeGeoPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 10000
	}
	if limit > 100000 {
		limit = 100000
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
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
			distance_m, duration_seconds, elevation_min_m, elevation_max_m, metadata_json::text
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
		var metadataText string
		if err := rows.Scan(&summary.TrackAssetID, &summary.Name, &summary.PointCount, &summary.StartTime, &summary.EndTime, &summary.MinLat, &summary.MinLon,
			&summary.MaxLat, &summary.MaxLon, &summary.DistanceM, &summary.DurationSec, &summary.ElevationMin, &summary.ElevationMax, &metadataText); err != nil {
			return nil, err
		}
		applyTrackMetadata(&summary, metadataText)
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

func (db *DB) QueryTrackPointsBatch(ctx context.Context, query catalog.TrackPointBatchQuery) (map[string][]catalog.TrackPoint, error) {
	trackIDs := compactDBStrings(query.TrackAssetIDs)
	if len(trackIDs) == 0 {
		return map[string][]catalog.TrackPoint{}, nil
	}
	maxPoints := query.MaxPointsPerTrack
	if maxPoints <= 0 {
		maxPoints = 500
	}
	if maxPoints > 5000 {
		maxPoints = 5000
	}
	cachedOut := map[string][]catalog.TrackPoint{}
	if detailLevel := trackRenderDetailLevelForMaxPoints(maxPoints); detailLevel != "" {
		placeholders := make([]string, 0, len(trackIDs))
		args := make([]any, 0, len(trackIDs)+1)
		for i, trackID := range trackIDs {
			args = append(args, trackID)
			placeholders = append(placeholders, fmt.Sprintf("$%d::uuid", i+1))
		}
		args = append(args, detailLevel)
		detailArg := len(args)
		rows, err := db.pool.Query(ctx, fmt.Sprintf(`
			select track_asset_id::text, recorded_at, lat, lon, elevation_m, speed_mps, source, ordinal
			from gps_track_render_points
			where detail_level = $%d and track_asset_id = any(array[%s])
			order by track_asset_id, ordinal
		`, detailArg, strings.Join(placeholders, ",")), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var point catalog.TrackPoint
			var ordinal int
			if err := rows.Scan(&point.TrackAssetID, &point.RecordedAt, &point.Lat, &point.Lon, &point.ElevationM, &point.SpeedMPS, &point.Source, &ordinal); err != nil {
				rows.Close()
				return nil, err
			}
			cachedOut[point.TrackAssetID] = append(cachedOut[point.TrackAssetID], point)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		missing := make([]string, 0)
		for _, trackID := range trackIDs {
			if len(cachedOut[trackID]) == 0 {
				missing = append(missing, trackID)
			}
		}
		if len(missing) == 0 {
			return cachedOut, nil
		}
		trackIDs = missing
	}
	placeholders := make([]string, 0, len(trackIDs))
	args := make([]any, 0, len(trackIDs)+1)
	for i, trackID := range trackIDs {
		args = append(args, trackID)
		placeholders = append(placeholders, fmt.Sprintf("$%d::uuid", i+1))
	}
	if maxPoints <= 4 {
		fallback, err := db.queryTrackEndpointPoints(ctx, trackIDs)
		if err != nil {
			return nil, err
		}
		for trackID, points := range fallback {
			cachedOut[trackID] = points
		}
		return cachedOut, nil
	}
	args = append(args, maxPoints)
	maxPointsArg := len(args)
	rows, err := db.pool.Query(ctx, fmt.Sprintf(`
		with ranked as (
			select id, track_asset_id::text as track_asset_id, recorded_at, lat, lon, elevation_m, speed_mps, source,
				row_number() over (partition by track_asset_id order by recorded_at nulls last, id) as rn,
				count(*) over (partition by track_asset_id) as cnt
			from track_points
			where track_asset_id = any(array[%s])
		), sampled as (
			select *,
				greatest(1, ceil(cnt::numeric / greatest($%d::int, 1))::int) as stride
			from ranked
		)
		select id, track_asset_id, recorded_at, lat, lon, elevation_m, speed_mps, source
		from sampled
		where cnt <= $%d or rn = 1 or rn = cnt or ((rn - 1) %% stride) = 0
		order by track_asset_id, rn
	`, strings.Join(placeholders, ","), maxPointsArg, maxPointsArg), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := cachedOut
	if out == nil {
		out = make(map[string][]catalog.TrackPoint, len(trackIDs))
	}
	for rows.Next() {
		var point catalog.TrackPoint
		if err := rows.Scan(&point.ID, &point.TrackAssetID, &point.RecordedAt, &point.Lat, &point.Lon, &point.ElevationM, &point.SpeedMPS, &point.Source); err != nil {
			return nil, err
		}
		out[point.TrackAssetID] = append(out[point.TrackAssetID], point)
	}
	return out, rows.Err()
}

func (db *DB) queryTrackEndpointPoints(ctx context.Context, trackIDs []string) (map[string][]catalog.TrackPoint, error) {
	trackIDs = compactDBStrings(trackIDs)
	if len(trackIDs) == 0 {
		return map[string][]catalog.TrackPoint{}, nil
	}
	placeholders := make([]string, 0, len(trackIDs))
	args := make([]any, 0, len(trackIDs))
	for i, trackID := range trackIDs {
		args = append(args, trackID)
		placeholders = append(placeholders, fmt.Sprintf("$%d::uuid", i+1))
	}
	rows, err := db.pool.Query(ctx, fmt.Sprintf(`
			with selected(track_asset_id) as (
				select unnest(array[%s])
			), sampled as (
				select p.*
				from selected s
				join lateral (
					(select id, track_asset_id::text as track_asset_id, recorded_at, lat, lon, elevation_m, speed_mps, source, 1 as ordinal
					 from track_points
					 where track_asset_id = s.track_asset_id
					 order by recorded_at nulls last, id
					 limit 1)
					union all
					(select id, track_asset_id::text as track_asset_id, recorded_at, lat, lon, elevation_m, speed_mps, source, 2 as ordinal
					 from track_points
					 where track_asset_id = s.track_asset_id
					 order by recorded_at desc nulls last, id desc
					 limit 1)
				) p on true
			)
			select id, track_asset_id, recorded_at, lat, lon, elevation_m, speed_mps, source
			from sampled
			order by track_asset_id, ordinal
		`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]catalog.TrackPoint, len(trackIDs))
	for rows.Next() {
		var point catalog.TrackPoint
		if err := rows.Scan(&point.ID, &point.TrackAssetID, &point.RecordedAt, &point.Lat, &point.Lon, &point.ElevationM, &point.SpeedMPS, &point.Source); err != nil {
			return nil, err
		}
		out[point.TrackAssetID] = append(out[point.TrackAssetID], point)
	}
	return out, rows.Err()
}

func (db *DB) TrackRenderCacheStatus(ctx context.Context) (catalog.TrackRenderCacheStatus, error) {
	status := catalog.TrackRenderCacheStatus{
		RequiredLevel:    "z16",
		RefreshBatchHint: 200,
		PointsByLevel:    map[string]int{},
	}
	if err := db.pool.QueryRow(ctx, `select count(*)::int from gps_tracks where point_count > 0`).Scan(&status.TracksTotal); err != nil {
		return status, err
	}
	if err := db.pool.QueryRow(ctx, `
		select count(distinct rp.track_asset_id)::int
		from gps_track_render_points rp
		join gps_tracks g on g.track_asset_id=rp.track_asset_id
		where rp.detail_level=$1 and g.point_count > 0`, status.RequiredLevel).Scan(&status.TracksWithCache); err != nil {
		return status, err
	}
	status.MissingCache = status.TracksTotal - status.TracksWithCache
	if status.MissingCache < 0 {
		status.MissingCache = 0
	}
	rows, err := db.pool.Query(ctx, `
		select detail_level, count(*)::int
		from gps_track_render_points
		group by detail_level
		order by detail_level`)
	if err != nil {
		return status, err
	}
	defer rows.Close()
	for rows.Next() {
		var level string
		var count int
		if err := rows.Scan(&level, &count); err != nil {
			return status, err
		}
		status.PointsByLevel[level] = count
	}
	return status, rows.Err()
}

func (db *DB) RefreshTrackRenderCache(ctx context.Context, limit int) (catalog.TrackRenderCacheRefreshResult, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	requiredLevel := "z16"
	result := catalog.TrackRenderCacheRefreshResult{
		RequiredLevel: requiredLevel,
		Levels:        []string{"overview", "z0", "z6", "z10", "z13", "z16"},
		SafeNote:      "Track render cache refresh writes only compact Cartolensia DB rows; original track files are never modified.",
	}
	rows, err := db.pool.Query(ctx, `
		select g.track_asset_id::text
		from gps_tracks g
		where g.point_count > 0
		  and not exists (
			select 1 from gps_track_render_points rp
			where rp.track_asset_id = g.track_asset_id and rp.detail_level = $1
		)
		order by g.start_at desc nulls last, g.title, g.track_asset_id
		limit $2`, requiredLevel, limit)
	if err != nil {
		return result, err
	}
	var ids []string
	for rows.Next() {
		var trackID string
		if err := rows.Scan(&trackID); err != nil {
			rows.Close()
			return result, err
		}
		ids = append(ids, trackID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	for _, trackID := range ids {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.Processed++
		points, err := db.QueryTrackPoints(ctx, catalog.TrackPointQuery{TrackAssetID: trackID})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", trackID, err))
			continue
		}
		tx, err := db.pool.Begin(ctx)
		if err != nil {
			return result, err
		}
		if _, err := tx.Exec(ctx, `delete from gps_track_render_points where track_asset_id=$1`, trackID); err != nil {
			_ = tx.Rollback(ctx)
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", trackID, err))
			continue
		}
		if err := insertTrackRenderPoints(ctx, tx, trackID, points); err != nil {
			_ = tx.Rollback(ctx)
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", trackID, err))
			continue
		}
		if err := tx.Commit(ctx); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", trackID, err))
			continue
		}
		result.Refreshed++
	}
	var remaining int
	if err := db.pool.QueryRow(ctx, `
		select count(*)::int
		from gps_tracks g
		where g.point_count > 0
		  and not exists (
			select 1 from gps_track_render_points rp
			where rp.track_asset_id = g.track_asset_id and rp.detail_level = $1
		)`, requiredLevel).Scan(&remaining); err == nil {
		result.Remaining = remaining
	}
	return result, nil
}

func compactDBStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (db *DB) QueryTrackAssets(ctx context.Context, query catalog.TrackAssetQuery) (catalog.AssetPage, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
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
		return catalog.AssetPage{Assets: []catalog.Asset{}, Page: catalog.Page{Limit: limit, Offset: offset}}, nil
	}
	start := track.StartTime.Add(time.Duration(query.OffsetSeconds) * time.Second)
	end := track.EndTime.Add(time.Duration(query.OffsetSeconds) * time.Second)
	mediaKinds := catalog.TrackAssetAllowedMediaKinds(query.MediaKind, query.ExcludeTrackAssets)
	assetQueryMediaKind := ""
	if len(mediaKinds) == 1 {
		assetQueryMediaKind = mediaKinds[0]
	}
	page, err := db.QueryAssets(ctx, catalog.AssetQuery{MediaKind: assetQueryMediaKind, Limit: 500, Offset: 0, Sort: "taken_at"})
	if err != nil {
		return catalog.AssetPage{}, err
	}
	allAssets := append([]catalog.Asset(nil), page.Assets...)
	for nextOffset := len(page.Assets); nextOffset < page.Page.Total; {
		if len(page.Assets) == 0 {
			break
		}
		next, err := db.QueryAssets(ctx, catalog.AssetQuery{MediaKind: assetQueryMediaKind, Limit: 500, Offset: nextOffset, Sort: "taken_at"})
		if err != nil {
			return catalog.AssetPage{}, err
		}
		allAssets = append(allAssets, next.Assets...)
		nextOffset += len(next.Assets)
		page = next
	}
	points, _ := db.QueryTrackPoints(ctx, catalog.TrackPointQuery{TrackAssetID: query.TrackAssetID, Simplify: true, MaxPoints: 4000})
	filtered := make([]catalog.Asset, 0, len(allAssets))
	seen := map[string]bool{}
	for _, asset := range allAssets {
		if !catalog.TrackAssetMediaKindAllowed(asset.MediaKind, mediaKinds, query.ExcludeTrackAssets) {
			continue
		}
		_, err := db.GetAssetGeo(ctx, asset.ID)
		geotagged := err == nil
		if geotagged && !query.IncludeGeotagged {
			continue
		}
		if !geotagged && !query.IncludeUngeotagged {
			continue
		}
		_, timeMatch := catalog.AssetTimestampInRange(asset, start, end, 90*time.Minute, time.Local)
		geoMatch := false
		if geotagged && len(points) > 0 {
			if lat, lon, ok := assetLatLon(asset); ok {
				geoMatch = nearestTrackDistanceM(points, lat, lon) <= 1000
			}
		}
		if !timeMatch && !geoMatch {
			continue
		}
		if !seen[asset.ID] {
			filtered = append(filtered, asset)
			seen[asset.ID] = true
		}
	}
	total := len(filtered)
	if offset >= len(filtered) {
		filtered = []catalog.Asset{}
	} else {
		endIndex := offset + limit
		if endIndex > len(filtered) {
			endIndex = len(filtered)
		}
		filtered = filtered[offset:endIndex]
	}
	page.Assets = filtered
	page.Page = catalog.Page{Limit: limit, Offset: offset, Total: total}
	return page, nil
}

func assetLatLon(asset catalog.Asset) (float64, float64, bool) {
	lat, okLat := metadataFloat(asset.Metadata, "lat")
	lon, okLon := metadataFloat(asset.Metadata, "lon")
	if okLat && okLon {
		return lat, lon, true
	}
	lat, okLat = metadataFloat(asset.Metadata, "gps_lat")
	lon, okLon = metadataFloat(asset.Metadata, "gps_lon")
	return lat, lon, okLat && okLon
}

func nearestTrackDistanceM(points []catalog.TrackPoint, lat, lon float64) float64 {
	best := math.MaxFloat64
	for _, point := range points {
		dist := haversineMeters(lat, lon, point.Lat, point.Lon)
		if dist < best {
			best = dist
		}
	}
	return best
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusM = 6371000.0
	toRad := math.Pi / 180
	phi1 := lat1 * toRad
	phi2 := lat2 * toRad
	dPhi := (lat2 - lat1) * toRad
	dLambda := (lon2 - lon1) * toRad
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
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

func (db *DB) ListTranscodingPresets(ctx context.Context) ([]catalog.TranscodingPreset, error) {
	rows, err := db.pool.Query(ctx, `
		select id, name, config_json, created_at, updated_at
		from transcoding_presets
		order by name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var presets []catalog.TranscodingPreset
	for rows.Next() {
		var preset catalog.TranscodingPreset
		var configBytes []byte
		if err := rows.Scan(&preset.ID, &preset.Name, &configBytes, &preset.CreatedAt, &preset.UpdatedAt); err != nil {
			return nil, err
		}
		applyTranscodingPresetConfig(&preset, configBytes)
		presets = append(presets, preset)
	}
	return presets, rows.Err()
}

func (db *DB) UpsertTranscodingPreset(ctx context.Context, preset catalog.TranscodingPreset) (catalog.TranscodingPreset, error) {
	if preset.ID == "" {
		preset.ID = slugifyDB(preset.Name)
	}
	if preset.Name == "" {
		preset.Name = preset.ID
	}
	preset.BuiltIn = false
	preset.Available = true
	configBytes, err := json.Marshal(transcodingPresetConfig(preset))
	if err != nil {
		return catalog.TranscodingPreset{}, err
	}
	_, err = db.pool.Exec(ctx, `
		insert into transcoding_presets(id, name, config_json)
		values($1, $2, $3)
		on conflict(id) do update set
			name=excluded.name,
			config_json=excluded.config_json,
			updated_at=now()
	`, preset.ID, preset.Name, configBytes)
	if err != nil {
		return catalog.TranscodingPreset{}, err
	}
	return db.getTranscodingPreset(ctx, preset.ID)
}

func (db *DB) DeleteTranscodingPreset(ctx context.Context, presetID string) error {
	cmd, err := db.pool.Exec(ctx, `delete from transcoding_presets where id=$1`, presetID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

func (db *DB) getTranscodingPreset(ctx context.Context, presetID string) (catalog.TranscodingPreset, error) {
	rows, err := db.pool.Query(ctx, `
		select id, name, config_json, created_at, updated_at
		from transcoding_presets
		where id=$1
	`, presetID)
	if err != nil {
		return catalog.TranscodingPreset{}, err
	}
	defer rows.Close()
	presets := []catalog.TranscodingPreset{}
	for rows.Next() {
		var preset catalog.TranscodingPreset
		var configBytes []byte
		if err := rows.Scan(&preset.ID, &preset.Name, &configBytes, &preset.CreatedAt, &preset.UpdatedAt); err != nil {
			return catalog.TranscodingPreset{}, err
		}
		applyTranscodingPresetConfig(&preset, configBytes)
		presets = append(presets, preset)
	}
	if err := rows.Err(); err != nil {
		return catalog.TranscodingPreset{}, err
	}
	if len(presets) == 0 {
		return catalog.TranscodingPreset{}, catalog.ErrNotFound
	}
	return presets[0], nil
}

func applyTranscodingPresetConfig(preset *catalog.TranscodingPreset, configBytes []byte) {
	var config map[string]any
	_ = json.Unmarshal(configBytes, &config)
	preset.BuiltIn = boolFromConfig(config, "built_in")
	preset.Available = true
	if v, _ := config["available"].(bool); !v && config["available"] != nil {
		preset.Available = false
	}
	preset.DisabledReason, _ = config["disabled_reason"].(string)
	preset.Hardware, _ = config["hardware"].(string)
	preset.Codec, _ = config["codec"].(string)
	preset.FFmpegEncoder, _ = config["ffmpeg_encoder"].(string)
	preset.Mode, _ = config["mode"].(string)
	preset.ParameterValue, _ = config["parameter_value"].(string)
	preset.Container, _ = config["container"].(string)
	if extra, ok := config["extra_args"].(map[string]any); ok {
		preset.ExtraArgs = extra
	}
}

func transcodingPresetConfig(preset catalog.TranscodingPreset) map[string]any {
	return map[string]any{
		"built_in":        preset.BuiltIn,
		"available":       preset.Available,
		"disabled_reason": preset.DisabledReason,
		"hardware":        preset.Hardware,
		"codec":           preset.Codec,
		"ffmpeg_encoder":  preset.FFmpegEncoder,
		"mode":            preset.Mode,
		"parameter_value": preset.ParameterValue,
		"container":       preset.Container,
		"extra_args":      metadataOrEmpty(preset.ExtraArgs),
	}
}

func boolFromConfig(config map[string]any, key string) bool {
	if value, ok := config[key].(bool); ok {
		return value
	}
	return false
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
	return sanitizeJSONMap(metadata)
}

func sanitizeJSONMap(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = sanitizeJSONValue(value)
	}
	return out
}

func sanitizeJSONValue(value any) any {
	switch v := value.(type) {
	case nil, string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		json.Number:
		return v
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return v
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
		return v
	case map[string]any:
		return sanitizeJSONMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sanitizeJSONValue(item)
		}
		return out
	case []float64:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sanitizeJSONValue(item)
		}
		return out
	case []float32:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sanitizeJSONValue(item)
		}
		return out
	case []map[string]any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sanitizeJSONMap(item)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = item
		}
		return out
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Map:
			if rv.IsNil() {
				return nil
			}
			out := make(map[string]any, rv.Len())
			iter := rv.MapRange()
			for iter.Next() {
				out[fmt.Sprint(iter.Key().Interface())] = sanitizeJSONValue(iter.Value().Interface())
			}
			return out
		case reflect.Slice, reflect.Array:
			if rv.Kind() == reflect.Slice && rv.IsNil() {
				return nil
			}
			out := make([]any, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				out[i] = sanitizeJSONValue(rv.Index(i).Interface())
			}
			return out
		}
		return v
	}
}

func (db *DB) UpsertAssetTag(ctx context.Context, tag catalog.AssetTag) (catalog.AssetTag, error) {
	if tag.Source == "" {
		tag.Source = "manual"
	}
	if tag.Metadata == nil {
		tag.Metadata = map[string]any{}
	}
	metadata, err := json.Marshal(metadataOrEmpty(tag.Metadata))
	if err != nil {
		return catalog.AssetTag{}, err
	}
	err = db.pool.QueryRow(ctx, `
		insert into asset_tags(asset_id, tag, source, confidence, plugin_id, metadata_json)
		values($1,$2,$3,$4,nullif($5,''),$6::jsonb)
		on conflict(asset_id, tag, source) do update set
			confidence=excluded.confidence,
			plugin_id=excluded.plugin_id,
			metadata_json=excluded.metadata_json
		returning asset_id, tag, source, confidence, coalesce(plugin_id,''), metadata_json, created_at`,
		tag.AssetID, tag.Tag, tag.Source, tag.Confidence, tag.PluginID, metadata,
	).Scan(&tag.AssetID, &tag.Tag, &tag.Source, &tag.Confidence, &tag.PluginID, &metadata, &tag.CreatedAt)
	if err != nil {
		return catalog.AssetTag{}, err
	}
	_ = json.Unmarshal(metadata, &tag.Metadata)
	return tag, nil
}

func (db *DB) ListAssetTags(ctx context.Context, assetID string) ([]catalog.AssetTag, error) {
	rows, err := db.pool.Query(ctx, `
		select asset_id, tag, source, confidence, coalesce(plugin_id,''), metadata_json, created_at
		from asset_tags
		where ($1='' or asset_id::text=$1)
		order by tag, source`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.AssetTag{}
	for rows.Next() {
		var tag catalog.AssetTag
		var metadata []byte
		if err := rows.Scan(&tag.AssetID, &tag.Tag, &tag.Source, &tag.Confidence, &tag.PluginID, &metadata, &tag.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &tag.Metadata); err != nil || tag.Metadata == nil {
			tag.Metadata = map[string]any{}
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

func (db *DB) CreateAIPrediction(ctx context.Context, pred catalog.AIPrediction) (catalog.AIPrediction, error) {
	if pred.ID == "" {
		pred.ID = id.NewUUID()
	}
	if pred.WorkerID == "" {
		pred.WorkerID = "local"
	}
	metadata, err := json.Marshal(metadataOrEmpty(pred.Metadata))
	if err != nil {
		return catalog.AIPrediction{}, err
	}
	err = db.pool.QueryRow(ctx, `
		insert into ai_predictions(id, asset_id, plugin_id, worker_id, task, label, confidence, model_name, model_version, metadata_json)
		values($1,$2,nullif($3,''),$4,$5,$6,$7,$8,$9,$10::jsonb)
		returning id, asset_id, coalesce(plugin_id,''), worker_id, task, label, confidence, model_name, model_version, metadata_json, created_at`,
		pred.ID, pred.AssetID, pred.PluginID, pred.WorkerID, pred.Task, pred.Label, pred.Confidence, pred.ModelName, pred.ModelVersion, metadata,
	).Scan(&pred.ID, &pred.AssetID, &pred.PluginID, &pred.WorkerID, &pred.Task, &pred.Label, &pred.Confidence, &pred.ModelName, &pred.ModelVersion, &metadata, &pred.CreatedAt)
	if err != nil {
		return catalog.AIPrediction{}, err
	}
	_ = json.Unmarshal(metadata, &pred.Metadata)
	return pred, nil
}

func (db *DB) ListAIPredictions(ctx context.Context, assetID string) ([]catalog.AIPrediction, error) {
	rows, err := db.pool.Query(ctx, `
		select id, asset_id, coalesce(plugin_id,''), worker_id, task, label, confidence, model_name, model_version, metadata_json, created_at
		from (
			select distinct on (asset_id, task, label, model_name, model_version)
				id, asset_id, plugin_id, worker_id, task, label, confidence, model_name, model_version, metadata_json, created_at
			from ai_predictions
			where ($1='' or asset_id::text=$1)
			order by asset_id, task, label, model_name, model_version, created_at desc
		) latest
		where ($1='' or asset_id::text=$1)
		order by created_at desc
		limit 500`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.AIPrediction{}
	for rows.Next() {
		var pred catalog.AIPrediction
		var metadata []byte
		if err := rows.Scan(&pred.ID, &pred.AssetID, &pred.PluginID, &pred.WorkerID, &pred.Task, &pred.Label, &pred.Confidence, &pred.ModelName, &pred.ModelVersion, &metadata, &pred.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &pred.Metadata); err != nil || pred.Metadata == nil {
			pred.Metadata = map[string]any{}
		}
		out = append(out, pred)
	}
	return out, rows.Err()
}

func (db *DB) QueryAIPredictions(ctx context.Context, query catalog.AIPredictionQuery) (catalog.AIPredictionPage, error) {
	limit := query.Limit
	if limit <= 0 || limit > 5000 {
		limit = 100
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	args := []any{}
	where := []string{"true"}
	if query.AssetID != "" {
		args = append(args, query.AssetID)
		where = append(where, fmt.Sprintf("asset_id::text=$%d", len(args)))
	}
	if len(query.Tasks) > 0 {
		tasks := make([]string, 0, len(query.Tasks))
		for _, task := range query.Tasks {
			task = strings.ToLower(strings.TrimSpace(task))
			if task != "" {
				tasks = append(tasks, task)
			}
		}
		if len(tasks) > 0 {
			args = append(args, tasks)
			where = append(where, fmt.Sprintf("lower(task)=any($%d::text[])", len(args)))
		}
	}
	if q := strings.ToLower(strings.TrimSpace(query.Q)); q != "" {
		args = append(args, "%"+q+"%")
		idx := len(args)
		where = append(where, fmt.Sprintf(`(
			lower(label) like $%d
			or lower(task) like $%d
			or lower(model_name) like $%d
			or lower(coalesce(model_version,'')) like $%d
			or asset_id::text like $%d
			or lower(metadata_json::text) like $%d
		)`, idx, idx, idx, idx, idx, idx))
	}
	filter := strings.Join(where, " and ")
	baseSQL := fmt.Sprintf(`
		with latest as (
			select distinct on (asset_id, task, label, model_name, model_version)
				id, asset_id, coalesce(plugin_id,'') as plugin_id, worker_id, task, label, confidence, model_name, model_version, metadata_json, created_at
			from ai_predictions
			order by asset_id, task, label, model_name, model_version, created_at desc
		),
		filtered as (
			select * from latest
			where %s
		)`, filter)
	countSQL := baseSQL + ` select count(*) from filtered`
	var total int
	if err := db.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return catalog.AIPredictionPage{}, err
	}
	pageArgs := append(append([]any(nil), args...), limit, offset)
	pageSQL := baseSQL + fmt.Sprintf(`
		select id, asset_id, plugin_id, worker_id, task, label, confidence, model_name, model_version, metadata_json, created_at
		from filtered
		order by created_at desc
		limit $%d offset $%d`, len(args)+1, len(args)+2)
	rows, err := db.pool.Query(ctx, pageSQL, pageArgs...)
	if err != nil {
		return catalog.AIPredictionPage{}, err
	}
	predictions, err := scanAIPredictions(rows)
	if err != nil {
		return catalog.AIPredictionPage{}, err
	}
	return catalog.AIPredictionPage{Predictions: predictions, Total: total}, nil
}

func scanAIPredictions(rows pgx.Rows) ([]catalog.AIPrediction, error) {
	defer rows.Close()
	out := []catalog.AIPrediction{}
	for rows.Next() {
		var pred catalog.AIPrediction
		var metadata []byte
		if err := rows.Scan(&pred.ID, &pred.AssetID, &pred.PluginID, &pred.WorkerID, &pred.Task, &pred.Label, &pred.Confidence, &pred.ModelName, &pred.ModelVersion, &metadata, &pred.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &pred.Metadata); err != nil || pred.Metadata == nil {
			pred.Metadata = map[string]any{}
		}
		out = append(out, pred)
	}
	return out, rows.Err()
}

func (db *DB) DeleteAIPrediction(ctx context.Context, assetID, predictionID string) error {
	tag, err := db.pool.Exec(ctx, `
		delete from ai_predictions
		where id::text=$1 and asset_id::text=$2 and task in ('ocr_image','ocr','ocr_text')`,
		predictionID, assetID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

func (db *DB) UpsertPlace(ctx context.Context, place catalog.PlaceCacheEntry) (catalog.PlaceCacheEntry, error) {
	if place.ID == "" {
		place.ID = id.NewUUID()
	}
	if place.NormalizedName == "" {
		place.NormalizedName = normalizeDBPlaceName(place.Name)
	}
	if place.Provider == "" {
		place.Provider = "local"
	}
	if place.DisplayName == "" {
		place.DisplayName = place.Name
	}
	if place.Source == "" {
		place.Source = "operator_cache"
	}
	aliases, err := json.Marshal(place.Aliases)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	metadata, err := json.Marshal(metadataOrEmpty(place.Metadata))
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	var aliasesOut []byte
	var metadataOut []byte
	err = db.pool.QueryRow(ctx, `
		insert into place_cache(id, name, normalized_name, aliases_json, provider, display_name, country, region, city, road, lat, lon, min_lon, min_lat, max_lon, max_lat, source, metadata_json)
		values($1,$2,$3,$4::jsonb,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18::jsonb)
		on conflict(provider, normalized_name) do update set
			name=excluded.name,
			aliases_json=excluded.aliases_json,
			display_name=excluded.display_name,
			country=excluded.country,
			region=excluded.region,
			city=excluded.city,
			road=excluded.road,
			lat=excluded.lat,
			lon=excluded.lon,
			min_lon=excluded.min_lon,
			min_lat=excluded.min_lat,
			max_lon=excluded.max_lon,
			max_lat=excluded.max_lat,
			source=excluded.source,
			metadata_json=excluded.metadata_json,
			updated_at=now()
		returning id, name, normalized_name, aliases_json, provider, display_name, country, region, city, road, lat, lon, min_lon, min_lat, max_lon, max_lat, source, metadata_json, created_at, updated_at, last_used_at`,
		place.ID, place.Name, place.NormalizedName, aliases, place.Provider, place.DisplayName, place.Country, place.Region, place.City, place.Road,
		place.Lat, place.Lon, place.BBox.MinLon, place.BBox.MinLat, place.BBox.MaxLon, place.BBox.MaxLat, place.Source, metadata,
	).Scan(
		&place.ID, &place.Name, &place.NormalizedName, &aliasesOut, &place.Provider, &place.DisplayName, &place.Country, &place.Region, &place.City, &place.Road,
		&place.Lat, &place.Lon, &place.BBox.MinLon, &place.BBox.MinLat, &place.BBox.MaxLon, &place.BBox.MaxLat, &place.Source, &metadataOut, &place.CreatedAt, &place.UpdatedAt, &place.LastUsedAt,
	)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	_ = json.Unmarshal(aliasesOut, &place.Aliases)
	if err := json.Unmarshal(metadataOut, &place.Metadata); err != nil || place.Metadata == nil {
		place.Metadata = map[string]any{}
	}
	return place, nil
}

func (db *DB) ListPlaces(ctx context.Context, query catalog.PlaceQuery) ([]catalog.PlaceCacheEntry, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	q := normalizeDBPlaceName(query.Q)
	rows, err := db.pool.Query(ctx, `
		select id, name, normalized_name, aliases_json, provider, display_name, country, region, city, road, lat, lon, min_lon, min_lat, max_lon, max_lat, source, metadata_json, created_at, updated_at, last_used_at
		from place_cache
		where $1='' or normalized_name like '%' || $1 || '%' or lower(display_name) like '%' || $1 || '%' or lower(country) like '%' || $1 || '%' or lower(region) like '%' || $1 || '%' or lower(city) like '%' || $1 || '%' or lower(road) like '%' || $1 || '%' or lower(aliases_json::text) like '%' || $1 || '%'
		order by name asc, id asc
		limit $2 offset $3`,
		q, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.PlaceCacheEntry{}
	for rows.Next() {
		var place catalog.PlaceCacheEntry
		var aliases []byte
		var metadata []byte
		if err := rows.Scan(
			&place.ID, &place.Name, &place.NormalizedName, &aliases, &place.Provider, &place.DisplayName, &place.Country, &place.Region, &place.City, &place.Road,
			&place.Lat, &place.Lon, &place.BBox.MinLon, &place.BBox.MinLat, &place.BBox.MaxLon, &place.BBox.MaxLat, &place.Source, &metadata, &place.CreatedAt, &place.UpdatedAt, &place.LastUsedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(aliases, &place.Aliases)
		if err := json.Unmarshal(metadata, &place.Metadata); err != nil || place.Metadata == nil {
			place.Metadata = map[string]any{}
		}
		out = append(out, place)
	}
	return out, rows.Err()
}

func (db *DB) QueryPlaceHierarchy(ctx context.Context, query catalog.PlaceHierarchyQuery) (catalog.PlaceHierarchyPage, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	q := normalizeDBPlaceName(query.Q)
	countSQL := `
		select count(*)::int
		from place_cache
		where $1='' or normalized_name like '%' || $1 || '%' or lower(display_name) like '%' || $1 || '%' or lower(country) like '%' || $1 || '%' or lower(region) like '%' || $1 || '%' or lower(city) like '%' || $1 || '%' or lower(road) like '%' || $1 || '%' or lower(aliases_json::text) like '%' || $1 || '%'`
	var total int
	if err := db.pool.QueryRow(ctx, countSQL, q).Scan(&total); err != nil {
		return catalog.PlaceHierarchyPage{}, err
	}
	rows, err := db.pool.Query(ctx, `
		select id, name, normalized_name, aliases_json, provider, display_name, country, region, city, road, lat, lon, min_lon, min_lat, max_lon, max_lat, source, metadata_json, created_at, updated_at, last_used_at,
			(select count(*)::int from asset_geo ag where ag.lat between p.min_lat and p.max_lat and ag.lon between p.min_lon and p.max_lon) as asset_count,
			(select count(*)::int from gps_tracks gt where gt.max_lon >= p.min_lon and gt.min_lon <= p.max_lon and gt.max_lat >= p.min_lat and gt.min_lat <= p.max_lat) as track_count
		from place_cache p
		where $1='' or normalized_name like '%' || $1 || '%' or lower(display_name) like '%' || $1 || '%' or lower(country) like '%' || $1 || '%' or lower(region) like '%' || $1 || '%' or lower(city) like '%' || $1 || '%' or lower(road) like '%' || $1 || '%' or lower(aliases_json::text) like '%' || $1 || '%'
		order by nullif(country, '') nulls last, nullif(region, '') nulls last, nullif(city, '') nulls last, nullif(road, '') nulls last, name, id
		limit $2 offset $3`,
		q, limit, offset,
	)
	if err != nil {
		return catalog.PlaceHierarchyPage{}, err
	}
	defer rows.Close()
	page := catalog.PlaceHierarchyPage{
		Entries: []catalog.PlaceHierarchyEntry{},
		Page:    catalog.Page{Limit: limit, Offset: offset, Total: total},
	}
	for rows.Next() {
		var place catalog.PlaceCacheEntry
		var aliases []byte
		var metadata []byte
		entry := catalog.PlaceHierarchyEntry{}
		if err := rows.Scan(
			&place.ID, &place.Name, &place.NormalizedName, &aliases, &place.Provider, &place.DisplayName, &place.Country, &place.Region, &place.City, &place.Road,
			&place.Lat, &place.Lon, &place.BBox.MinLon, &place.BBox.MinLat, &place.BBox.MaxLon, &place.BBox.MaxLat, &place.Source, &metadata, &place.CreatedAt, &place.UpdatedAt, &place.LastUsedAt,
			&entry.AssetCount, &entry.TrackCount,
		); err != nil {
			return catalog.PlaceHierarchyPage{}, err
		}
		_ = json.Unmarshal(aliases, &place.Aliases)
		if err := json.Unmarshal(metadata, &place.Metadata); err != nil || place.Metadata == nil {
			place.Metadata = map[string]any{}
		}
		entry.Place = place
		entry.Hierarchy = placeHierarchyParts(place)
		entry.Level = placeHierarchyLevel(place)
		entry.Label = placeHierarchyLabel(place)
		page.Entries = append(page.Entries, entry)
	}
	return page, rows.Err()
}

func placeHierarchyParts(place catalog.PlaceCacheEntry) []string {
	return compactDBStrings([]string{place.Country, place.Region, place.City, place.Road, place.Name})
}

func placeHierarchyLevel(place catalog.PlaceCacheEntry) string {
	switch {
	case strings.TrimSpace(place.Road) != "":
		return "road"
	case strings.TrimSpace(place.City) != "":
		return "city"
	case strings.TrimSpace(place.Region) != "":
		return "region"
	case strings.TrimSpace(place.Country) != "":
		return "country"
	default:
		return "place"
	}
}

func placeHierarchyLabel(place catalog.PlaceCacheEntry) string {
	if strings.TrimSpace(place.DisplayName) != "" {
		return place.DisplayName
	}
	return strings.Join(placeHierarchyParts(place), " / ")
}

func (db *DB) DeletePlace(ctx context.Context, placeID string) error {
	tag, err := db.pool.Exec(ctx, `delete from place_cache where id::text=$1`, placeID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

func normalizeDBPlaceName(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func (db *DB) CreateFaceDetection(ctx context.Context, face catalog.FaceDetection) (catalog.FaceDetection, error) {
	if face.ID == "" {
		face.ID = id.NewUUID()
	}
	metadata, err := json.Marshal(metadataOrEmpty(face.Metadata))
	if err != nil {
		return catalog.FaceDetection{}, err
	}
	err = db.pool.QueryRow(ctx, `
		insert into face_detections(id, asset_id, plugin_id, x, y, width, height, confidence, cluster_id, metadata_json)
		values($1,$2,nullif($3,''),$4,$5,$6,$7,$8,nullif($9,'')::uuid,$10::jsonb)
		returning id, asset_id, coalesce(plugin_id,''), x, y, width, height, confidence, coalesce(cluster_id::text,''), metadata_json, created_at`,
		face.ID, face.AssetID, face.PluginID, face.X, face.Y, face.Width, face.Height, face.Confidence, face.ClusterID, metadata,
	).Scan(&face.ID, &face.AssetID, &face.PluginID, &face.X, &face.Y, &face.Width, &face.Height, &face.Confidence, &face.ClusterID, &metadata, &face.CreatedAt)
	if err != nil {
		return catalog.FaceDetection{}, err
	}
	_ = json.Unmarshal(metadata, &face.Metadata)
	return face, nil
}

func (db *DB) ListFaceDetections(ctx context.Context, assetID string) ([]catalog.FaceDetection, error) {
	rows, err := db.pool.Query(ctx, `
		select id, asset_id, coalesce(plugin_id,''), x, y, width, height, confidence, coalesce(cluster_id::text,''), metadata_json, created_at
		from face_detections
		where ($1='' or asset_id::text=$1)
		order by created_at desc
		limit 500`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.FaceDetection{}
	for rows.Next() {
		var face catalog.FaceDetection
		var metadata []byte
		if err := rows.Scan(&face.ID, &face.AssetID, &face.PluginID, &face.X, &face.Y, &face.Width, &face.Height, &face.Confidence, &face.ClusterID, &metadata, &face.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &face.Metadata); err != nil || face.Metadata == nil {
			face.Metadata = map[string]any{}
		}
		out = append(out, face)
	}
	return out, rows.Err()
}

func (db *DB) UpsertFaceCluster(ctx context.Context, cluster catalog.FaceCluster) (catalog.FaceCluster, error) {
	if cluster.ID == "" {
		cluster.ID = id.NewUUID()
	}
	metadata, err := json.Marshal(metadataOrEmpty(cluster.Metadata))
	if err != nil {
		return catalog.FaceCluster{}, err
	}
	var representative *string
	if strings.TrimSpace(cluster.RepresentativeFaceID) != "" {
		value := strings.TrimSpace(cluster.RepresentativeFaceID)
		representative = &value
	}
	err = db.pool.QueryRow(ctx, `
		insert into face_clusters(id, label, representative_face_id, metadata_json)
		values($1,$2,$3,$4::jsonb)
		on conflict(id) do update set
			label=excluded.label,
			representative_face_id=coalesce(excluded.representative_face_id, face_clusters.representative_face_id),
			metadata_json=excluded.metadata_json,
			updated_at=now()
		returning id, label, coalesce(representative_face_id::text,''), metadata_json, created_at, updated_at`,
		cluster.ID, cluster.Label, representative, metadata,
	).Scan(&cluster.ID, &cluster.Label, &cluster.RepresentativeFaceID, &metadata, &cluster.CreatedAt, &cluster.UpdatedAt)
	if err != nil {
		return catalog.FaceCluster{}, err
	}
	if err := json.Unmarshal(metadata, &cluster.Metadata); err != nil || cluster.Metadata == nil {
		cluster.Metadata = map[string]any{}
	}
	return cluster, nil
}

func (db *DB) ListFaceClusters(ctx context.Context) ([]catalog.FaceCluster, error) {
	rows, err := db.pool.Query(ctx, `
		select c.id, c.label, coalesce(c.representative_face_id::text,''), c.metadata_json, c.created_at, c.updated_at,
			count(d.id)::int as face_count,
			count(distinct d.asset_id)::int as asset_count,
			coalesce(sum(case when coalesce((d.metadata_json->>'ignored')::boolean, false) then 1 else 0 end), 0)::int as ignored_count
		from face_clusters c
		left join face_detections d on d.cluster_id=c.id
		group by c.id, c.label, c.representative_face_id, c.metadata_json, c.created_at, c.updated_at
		order by nullif(c.label,'') nulls last, c.updated_at desc
		limit 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.FaceCluster{}
	for rows.Next() {
		var cluster catalog.FaceCluster
		var metadata []byte
		if err := rows.Scan(&cluster.ID, &cluster.Label, &cluster.RepresentativeFaceID, &metadata, &cluster.CreatedAt, &cluster.UpdatedAt, &cluster.FaceCount, &cluster.AssetCount, &cluster.IgnoredCount); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &cluster.Metadata); err != nil || cluster.Metadata == nil {
			cluster.Metadata = map[string]any{}
		}
		out = append(out, cluster)
	}
	return out, rows.Err()
}

func (db *DB) UpdateFaceDetection(ctx context.Context, face catalog.FaceDetection) (catalog.FaceDetection, error) {
	metadata, err := json.Marshal(metadataOrEmpty(face.Metadata))
	if err != nil {
		return catalog.FaceDetection{}, err
	}
	err = db.pool.QueryRow(ctx, `
		update face_detections
		set plugin_id=nullif($2,''),
			x=$3,
			y=$4,
			width=$5,
			height=$6,
			confidence=$7,
			cluster_id=nullif($8,'')::uuid,
			metadata_json=$9::jsonb
		where id=$1
		returning id, asset_id, coalesce(plugin_id,''), x, y, width, height, confidence, coalesce(cluster_id::text,''), metadata_json, created_at`,
		face.ID, face.PluginID, face.X, face.Y, face.Width, face.Height, face.Confidence, face.ClusterID, metadata,
	).Scan(&face.ID, &face.AssetID, &face.PluginID, &face.X, &face.Y, &face.Width, &face.Height, &face.Confidence, &face.ClusterID, &metadata, &face.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return catalog.FaceDetection{}, catalog.ErrNotFound
		}
		return catalog.FaceDetection{}, err
	}
	if err := json.Unmarshal(metadata, &face.Metadata); err != nil || face.Metadata == nil {
		face.Metadata = map[string]any{}
	}
	return face, nil
}

func (db *DB) UpsertEmbeddingModel(ctx context.Context, model catalog.EmbeddingModel) (catalog.EmbeddingModel, error) {
	if model.ID == "" {
		model.ID = model.ModelName + ":" + model.Version + ":" + model.Modality
	}
	metadata, err := json.Marshal(metadataOrEmpty(model.Metadata))
	if err != nil {
		return catalog.EmbeddingModel{}, err
	}
	err = db.pool.QueryRow(ctx, `
		insert into embedding_models(id, modality, model_name, version, dimension, plugin_id, metadata_json)
		values($1,$2,$3,$4,$5,nullif($6,''),$7::jsonb)
		on conflict(id) do update set
			modality=excluded.modality,
			model_name=excluded.model_name,
			version=excluded.version,
			dimension=excluded.dimension,
			plugin_id=excluded.plugin_id,
			metadata_json=excluded.metadata_json
		returning id, modality, model_name, version, coalesce(dimension,0), coalesce(plugin_id,''), metadata_json, created_at`,
		model.ID, model.Modality, model.ModelName, model.Version, model.Dimension, model.PluginID, metadata,
	).Scan(&model.ID, &model.Modality, &model.ModelName, &model.Version, &model.Dimension, &model.PluginID, &metadata, &model.CreatedAt)
	if err != nil {
		return catalog.EmbeddingModel{}, err
	}
	_ = json.Unmarshal(metadata, &model.Metadata)
	return model, nil
}

func (db *DB) UpsertAssetEmbedding(ctx context.Context, embedding catalog.AssetEmbedding) (catalog.AssetEmbedding, error) {
	if embedding.ID == "" {
		embedding.ID = id.NewUUID()
	}
	if embedding.SourceRef == "" {
		embedding.SourceRef = "asset"
	}
	payload, err := json.Marshal(map[string]any{"vector": embedding.Vector})
	if err != nil {
		return catalog.AssetEmbedding{}, err
	}
	metadata, err := json.Marshal(metadataOrEmpty(embedding.Metadata))
	if err != nil {
		return catalog.AssetEmbedding{}, err
	}
	err = db.pool.QueryRow(ctx, `
		insert into asset_embeddings(id, asset_id, model_id, modality, source_ref, embedding_json, metadata_json)
		values($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb)
		on conflict(asset_id, model_id, modality, source_ref) do update set
			embedding_json=excluded.embedding_json,
			metadata_json=excluded.metadata_json,
			created_at=now()
		returning id, asset_id, model_id, modality, source_ref, embedding_json, metadata_json, created_at`,
		embedding.ID, embedding.AssetID, embedding.ModelID, embedding.Modality, embedding.SourceRef, payload, metadata,
	).Scan(&embedding.ID, &embedding.AssetID, &embedding.ModelID, &embedding.Modality, &embedding.SourceRef, &payload, &metadata, &embedding.CreatedAt)
	if err != nil {
		return catalog.AssetEmbedding{}, err
	}
	embedding.Vector = vectorFromJSON(payload)
	_ = db.storeEmbeddingVector(ctx, embedding.ID, embedding.Vector)
	_ = json.Unmarshal(metadata, &embedding.Metadata)
	return embedding, nil
}

func (db *DB) ListAssetEmbeddings(ctx context.Context, assetID string) ([]catalog.AssetEmbedding, error) {
	rows, err := db.pool.Query(ctx, `
		select id, asset_id, model_id, modality, source_ref, embedding_json, metadata_json, created_at
		from asset_embeddings
		where ($1='' or asset_id::text=$1)
		order by created_at desc
		limit 500`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.AssetEmbedding{}
	for rows.Next() {
		var embedding catalog.AssetEmbedding
		var payload, metadata []byte
		if err := rows.Scan(&embedding.ID, &embedding.AssetID, &embedding.ModelID, &embedding.Modality, &embedding.SourceRef, &payload, &metadata, &embedding.CreatedAt); err != nil {
			return nil, err
		}
		embedding.Vector = vectorFromJSON(payload)
		if err := json.Unmarshal(metadata, &embedding.Metadata); err != nil || embedding.Metadata == nil {
			embedding.Metadata = map[string]any{}
		}
		out = append(out, embedding)
	}
	return out, rows.Err()
}

func (db *DB) AIDataCounts(ctx context.Context) (catalog.AIDataCounts, error) {
	var counts catalog.AIDataCounts
	err := db.pool.QueryRow(ctx, `
		select
			(select count(*) from asset_tags),
			(select count(*) from (
				select distinct on (asset_id, task, label, model_name, model_version)
					id
				from ai_predictions
				order by asset_id, task, label, model_name, model_version, created_at desc
			) latest_predictions),
			(select count(*) from face_detections),
			(select count(*) from asset_embeddings),
			(select count(distinct asset_id) from asset_embeddings),
			(select count(*) from (
				select distinct on (asset_id, task, label, model_name, model_version)
					task, label, confidence
				from ai_predictions
				order by asset_id, task, label, model_name, model_version, created_at desc
			) safety
			where (lower(task) like '%safety%' or lower(task) like '%nsfw%')
			  and (lower(label) like '%unsafe%' or lower(label) like '%nsfw%')
			  and (confidence is null or confidence >= 0.75))
	`).Scan(
		&counts.AssetTags,
		&counts.Predictions,
		&counts.FaceDetections,
		&counts.AssetEmbeddings,
		&counts.EmbeddedAssets,
		&counts.SafetyCandidates,
	)
	return counts, err
}

func (db *DB) VectorSearch(ctx context.Context, modelID string, vector []float64, limit int) ([]catalog.VectorSearchResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if len(vector) == 512 && db.PGVectorReady(ctx) {
		if results, err := db.vectorSearchPGVector(ctx, modelID, vector, limit); err == nil {
			return results, nil
		}
	}
	rows, err := db.pool.Query(ctx, `
		select asset_id, model_id, embedding_json
		from asset_embeddings
		where ($1='' or model_id=$1)
		limit 5000`, modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type candidate struct {
		assetID string
		modelID string
		score   float64
	}
	candidates := []candidate{}
	for rows.Next() {
		var assetID, matchedModelID string
		var payload []byte
		if err := rows.Scan(&assetID, &matchedModelID, &payload); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate{assetID: assetID, modelID: matchedModelID, score: dbCosineSimilarity(vector, vectorFromJSON(payload))})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	out := []catalog.VectorSearchResult{}
	for _, candidate := range candidates {
		if len(out) >= limit {
			break
		}
		asset, err := db.GetAsset(ctx, candidate.assetID)
		if err != nil {
			continue
		}
		out = append(out, catalog.VectorSearchResult{Asset: asset, Score: candidate.score, Match: candidate.modelID})
	}
	return out, nil
}

func (db *DB) vectorSearchPGVector(ctx context.Context, modelID string, vector []float64, limit int) ([]catalog.VectorSearchResult, error) {
	literal := pgVectorLiteral(vector)
	rows, err := db.pool.Query(ctx, `
		select asset_id::text, model_id, 1 - (embedding_vector <=> $2::vector) as score
		from asset_embeddings
		where ($1='' or model_id=$1)
		  and embedding_vector is not null
		order by embedding_vector <=> $2::vector
		limit $3`, modelID, literal, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type candidate struct {
		assetID string
		modelID string
		score   float64
	}
	candidates := []candidate{}
	for rows.Next() {
		var candidate candidate
		if err := rows.Scan(&candidate.assetID, &candidate.modelID, &candidate.score); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []catalog.VectorSearchResult{}
	for _, candidate := range candidates {
		asset, err := db.GetAsset(ctx, candidate.assetID)
		if err != nil {
			continue
		}
		out = append(out, catalog.VectorSearchResult{Asset: asset, Score: candidate.score, Match: candidate.modelID})
	}
	return out, nil
}

func (db *DB) storeEmbeddingVector(ctx context.Context, embeddingID string, vector []float64) error {
	if len(vector) != 512 {
		return nil
	}
	if !db.PGVectorReady(ctx) {
		return nil
	}
	_, err := db.pool.Exec(ctx, `update asset_embeddings set embedding_vector=$2::vector where id=$1`, embeddingID, pgVectorLiteral(vector))
	return err
}

func (db *DB) UpsertComponent(ctx context.Context, component catalog.Component) (catalog.Component, error) {
	if component.ID == "" {
		component.ID = id.NewUUID()
	}
	if strings.TrimSpace(component.Key) == "" {
		return catalog.Component{}, errors.New("component key is required")
	}
	if component.Status == "" {
		component.Status = "missing"
	}
	if component.SourceType == "" {
		component.SourceType = "system_path"
	}
	metadata, err := json.Marshal(metadataOrEmpty(component.Metadata))
	if err != nil {
		return catalog.Component{}, err
	}
	var metadataOut []byte
	err = db.pool.QueryRow(ctx, `
		insert into components(id, key, name, category, version, status, source_type, source_url, license_name, provenance_url, install_path, executable_path, checksum, size_bytes, last_checked_at, installed_at, error, metadata_json)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18::jsonb)
		on conflict(key) do update set
			name=excluded.name,
			category=excluded.category,
			version=excluded.version,
			status=excluded.status,
			source_type=excluded.source_type,
			source_url=excluded.source_url,
			license_name=excluded.license_name,
			provenance_url=excluded.provenance_url,
			install_path=excluded.install_path,
			executable_path=excluded.executable_path,
			checksum=excluded.checksum,
			size_bytes=excluded.size_bytes,
			last_checked_at=excluded.last_checked_at,
			installed_at=excluded.installed_at,
			error=excluded.error,
			metadata_json=excluded.metadata_json
		returning id, key, name, category, version, status, source_type, source_url, license_name, provenance_url, install_path, executable_path, checksum, size_bytes, last_checked_at, installed_at, error, metadata_json`,
		component.ID, component.Key, component.Name, component.Category, component.Version, component.Status, component.SourceType, component.SourceURL, component.LicenseName, component.ProvenanceURL, component.InstallPath, component.ExecutablePath, component.Checksum, component.SizeBytes, component.LastCheckedAt, component.InstalledAt, component.Error, metadata,
	).Scan(&component.ID, &component.Key, &component.Name, &component.Category, &component.Version, &component.Status, &component.SourceType, &component.SourceURL, &component.LicenseName, &component.ProvenanceURL, &component.InstallPath, &component.ExecutablePath, &component.Checksum, &component.SizeBytes, &component.LastCheckedAt, &component.InstalledAt, &component.Error, &metadataOut)
	if err != nil {
		return catalog.Component{}, err
	}
	if err := json.Unmarshal(metadataOut, &component.Metadata); err != nil || component.Metadata == nil {
		component.Metadata = map[string]any{}
	}
	return component, nil
}

func (db *DB) ListComponents(ctx context.Context, query catalog.ComponentQuery) ([]catalog.Component, error) {
	limit, offset := normalizeDBPage(query.Limit, query.Offset)
	q := strings.ToLower(strings.TrimSpace(query.Q))
	rows, err := db.pool.Query(ctx, `
		select id, key, name, category, version, status, source_type, source_url, license_name, provenance_url, install_path, executable_path, checksum, size_bytes, last_checked_at, installed_at, error, metadata_json
		from components
		where ($1='' or category=$1)
			and ($2='' or status=$2)
			and ($3='' or lower(key || ' ' || name || ' ' || category || ' ' || version || ' ' || status || ' ' || source_type || ' ' || license_name) like '%' || $3 || '%')
		order by category, key
		limit $4 offset $5`,
		query.Category, query.Status, q, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.Component{}
	for rows.Next() {
		component, err := scanComponentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, component)
	}
	return out, rows.Err()
}

func (db *DB) GetComponent(ctx context.Context, key string) (catalog.Component, error) {
	rows, err := db.pool.Query(ctx, `
		select id, key, name, category, version, status, source_type, source_url, license_name, provenance_url, install_path, executable_path, checksum, size_bytes, last_checked_at, installed_at, error, metadata_json
		from components
		where key=$1`, strings.TrimSpace(key))
	if err != nil {
		return catalog.Component{}, err
	}
	defer rows.Close()
	if rows.Next() {
		return scanComponentRow(rows)
	}
	if err := rows.Err(); err != nil {
		return catalog.Component{}, err
	}
	return catalog.Component{}, catalog.ErrNotFound
}

type componentScanner interface {
	Scan(dest ...any) error
}

func scanComponentRow(row componentScanner) (catalog.Component, error) {
	var component catalog.Component
	var metadata []byte
	if err := row.Scan(&component.ID, &component.Key, &component.Name, &component.Category, &component.Version, &component.Status, &component.SourceType, &component.SourceURL, &component.LicenseName, &component.ProvenanceURL, &component.InstallPath, &component.ExecutablePath, &component.Checksum, &component.SizeBytes, &component.LastCheckedAt, &component.InstalledAt, &component.Error, &metadata); err != nil {
		return catalog.Component{}, err
	}
	if err := json.Unmarshal(metadata, &component.Metadata); err != nil || component.Metadata == nil {
		component.Metadata = map[string]any{}
	}
	return component, nil
}

func (db *DB) AddComponentEvent(ctx context.Context, event catalog.ComponentEvent) (catalog.ComponentEvent, error) {
	if event.ID == "" {
		event.ID = id.NewUUID()
	}
	if event.Level == "" {
		event.Level = "info"
	}
	metadata, err := json.Marshal(metadataOrEmpty(event.Metadata))
	if err != nil {
		return catalog.ComponentEvent{}, err
	}
	var metadataOut []byte
	err = db.pool.QueryRow(ctx, `
		insert into component_events(id, component_key, level, message, metadata_json)
		values($1,$2,$3,$4,$5::jsonb)
		returning id, component_key, level, message, created_at, metadata_json`,
		event.ID, event.ComponentKey, event.Level, event.Message, metadata,
	).Scan(&event.ID, &event.ComponentKey, &event.Level, &event.Message, &event.CreatedAt, &metadataOut)
	if err != nil {
		return catalog.ComponentEvent{}, err
	}
	if err := json.Unmarshal(metadataOut, &event.Metadata); err != nil || event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	return event, nil
}

func (db *DB) ListComponentEvents(ctx context.Context, key string, limit int) ([]catalog.ComponentEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := db.pool.Query(ctx, `
		select id, component_key, level, message, created_at, metadata_json
		from component_events
		where component_key=$1
		order by created_at desc
		limit $2`, strings.TrimSpace(key), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.ComponentEvent{}
	for rows.Next() {
		var event catalog.ComponentEvent
		var metadata []byte
		if err := rows.Scan(&event.ID, &event.ComponentKey, &event.Level, &event.Message, &event.CreatedAt, &metadata); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &event.Metadata); err != nil || event.Metadata == nil {
			event.Metadata = map[string]any{}
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func vectorFromJSON(payload []byte) []float64 {
	var doc struct {
		Vector []float64 `json:"vector"`
	}
	if err := json.Unmarshal(payload, &doc); err == nil && len(doc.Vector) > 0 {
		return doc.Vector
	}
	var raw []float64
	if err := json.Unmarshal(payload, &raw); err == nil {
		return raw
	}
	return nil
}

func dbCosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func pgVectorLiteral(vector []float64) string {
	var builder strings.Builder
	builder.WriteByte('[')
	for i, value := range vector {
		if i > 0 {
			builder.WriteByte(',')
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			value = 0
		}
		builder.WriteString(fmt.Sprintf("%.9g", value))
	}
	builder.WriteByte(']')
	return builder.String()
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
