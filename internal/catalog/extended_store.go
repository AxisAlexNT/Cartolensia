package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/id"
)

func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func paginateAssets(assets []Asset, limit, offset int) []Asset {
	limit, offset = normalizePage(limit, offset)
	if offset >= len(assets) {
		return []Asset{}
	}
	end := offset + limit
	if end > len(assets) {
		end = len(assets)
	}
	return assets[offset:end]
}

func (s *MemoryStore) QueryAssets(ctx context.Context, query AssetQuery) (AssetPage, error) {
	assets, err := s.ListAssets(ctx)
	if err != nil {
		return AssetPage{}, err
	}
	assets = filterAssets(assets, query, s)
	sortAssets(assets, query.Sort)
	total := len(assets)
	limit, offset := normalizePage(query.Limit, query.Offset)
	return AssetPage{Assets: paginateAssets(assets, limit, offset), Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (s *MemoryStore) QueryAIMissingAssets(_ context.Context, query AIMissingQuery) (AssetPage, error) {
	limit, offset := normalizePage(query.Limit, query.Offset)
	s.mu.RLock()
	defer s.mu.RUnlock()
	excluded := map[string]struct{}{}
	for _, id := range query.ExcludeAssetIDs {
		if strings.TrimSpace(id) != "" {
			excluded[id] = struct{}{}
		}
	}
	out := make([]Asset, 0)
	for _, asset := range s.assets {
		if _, ok := excluded[asset.ID]; ok {
			continue
		}
		if query.MediaKind != "" && asset.MediaKind != query.MediaKind {
			continue
		}
		if query.MaxDurationSeconds > 0 && assetDurationSeconds(asset) > query.MaxDurationSeconds {
			continue
		}
		if s.aiAssetTaskCompleteLocked(asset.ID, query.Task) {
			continue
		}
		out = append(out, cloneAsset(asset))
	}
	sortAssets(out, "taken_at")
	total := len(out)
	if offset >= len(out) {
		return AssetPage{Assets: []Asset{}, Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return AssetPage{Assets: out[offset:end], Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (s *MemoryStore) UpsertAIAssetTaskStatus(_ context.Context, status AIAssetTaskStatus) (AIAssetTaskStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[status.AssetID]; !ok {
		return AIAssetTaskStatus{}, ErrNotFound
	}
	if strings.TrimSpace(status.Task) == "" {
		return AIAssetTaskStatus{}, errors.New("AI task is required")
	}
	if status.Status == "" {
		status.Status = "succeeded"
	}
	if status.Metadata == nil {
		status.Metadata = map[string]any{}
	}
	status.UpdatedAt = time.Now().UTC()
	s.aiTaskStatuses[status.AssetID+"\x00"+status.Task] = status
	return status, nil
}

func (s *MemoryStore) aiAssetTaskCompleteLocked(assetID, task string) bool {
	if status, ok := s.aiTaskStatuses[assetID+"\x00"+task]; ok && status.Status == "succeeded" {
		return true
	}
	switch task {
	case "classify_image", "safety_nsfw", "describe_image", "ocr_image":
		for _, prediction := range s.aiPredictions[assetID] {
			if prediction.Task == task {
				return true
			}
		}
	case "detect_faces":
		return len(s.faceDetections[assetID]) > 0
	case "embed_image":
		for key := range s.assetEmbeddings {
			if strings.HasPrefix(key, assetID+"\x00") {
				return true
			}
		}
	case "transcribe_audio":
		return len(s.transcripts[assetID]) > 0
	case "analyze_audio":
		_, ok := s.audioFeatures[assetID]
		return ok
	}
	return false
}

func assetDurationSeconds(asset Asset) float64 {
	if asset.Metadata == nil {
		return 0
	}
	for _, key := range []string{"duration_seconds", "duration"} {
		switch value := asset.Metadata[key].(type) {
		case float64:
			return value
		case float32:
			return float64(value)
		case int:
			return float64(value)
		case int64:
			return float64(value)
		case json.Number:
			f, _ := value.Float64()
			return f
		case string:
			f, _ := strconv.ParseFloat(value, 64)
			return f
		}
	}
	return 0
}

func filterAssets(assets []Asset, query AssetQuery, store *MemoryStore) []Asset {
	assets = SearchAssets(assets, query.Q)
	if query.MediaKind == "" && query.HashStatus == "" && query.Storage == "" && query.Extension == "" && len(query.Prefixes) == 0 &&
		query.AlbumID == "" && query.GeoSource == "" && query.TakenFrom == nil && query.TakenTo == nil {
		if !query.PublicOnly {
			return assets
		}
	}
	albumAssets := map[string]struct{}{}
	if query.AlbumID != "" && store != nil {
		for assetID := range store.albumItems[query.AlbumID] {
			albumAssets[assetID] = struct{}{}
		}
	}
	out := make([]Asset, 0, len(assets))
	for _, asset := range assets {
		if query.PublicOnly && !assetMetadataPublic(asset.Metadata) {
			continue
		}
		if query.AlbumID != "" {
			if _, ok := albumAssets[asset.ID]; !ok {
				continue
			}
		}
		if query.GeoSource != "" && store != nil {
			geo, ok := store.assetGeo[asset.ID]
			if !ok || geo.Source != query.GeoSource {
				continue
			}
		}
		if query.TakenFrom != nil && (asset.TakenAt == nil || asset.TakenAt.Before(*query.TakenFrom)) {
			continue
		}
		if query.TakenTo != nil && (asset.TakenAt == nil || asset.TakenAt.After(*query.TakenTo)) {
			continue
		}
		if assetLocationMatches(asset, query) {
			out = append(out, asset)
		}
	}
	return out
}

func assetMetadataPublic(metadata map[string]any) bool {
	for _, key := range []string{"public", "is_public", "visibility_public"} {
		if value, ok := metadata[key].(bool); ok {
			return value
		}
	}
	if value, ok := metadata["visibility"].(string); ok {
		return strings.EqualFold(value, "public")
	}
	return false
}

func assetLocationMatches(asset Asset, query AssetQuery) bool {
	if query.MediaKind == "" && query.HashStatus == "" && query.Storage == "" && query.Extension == "" && len(query.Prefixes) == 0 {
		return true
	}
	for _, loc := range asset.Locations {
		if query.MediaKind != "" && loc.MediaKind != query.MediaKind {
			continue
		}
		if query.HashStatus != "" && loc.HashStatus != query.HashStatus {
			continue
		}
		if query.Storage != "" && loc.StorageName != query.Storage {
			continue
		}
		if len(query.Prefixes) > 0 && !relativePathInAnyPrefix(loc.RelativePath, query.Prefixes) {
			continue
		}
		if query.Extension != "" && strings.TrimPrefix(strings.ToLower(loc.Extension), ".") != strings.TrimPrefix(strings.ToLower(query.Extension), ".") {
			continue
		}
		return true
	}
	return false
}

func relativePathInAnyPrefix(relativePath string, prefixes []string) bool {
	relativePath = strings.Trim(strings.TrimSpace(relativePath), "/")
	for _, prefix := range prefixes {
		prefix = strings.Trim(strings.TrimSpace(prefix), "/")
		if prefix != "" && (relativePath == prefix || strings.HasPrefix(relativePath, prefix+"/")) {
			return true
		}
	}
	return false
}

func sortAssets(assets []Asset, key string) {
	sort.SliceStable(assets, func(i, j int) bool {
		left, _ := FirstLocation(assets[i])
		right, _ := FirstLocation(assets[j])
		switch key {
		case "size":
			if left.SizeBytes == right.SizeBytes {
				return assets[i].DisplayName < assets[j].DisplayName
			}
			return left.SizeBytes < right.SizeBytes
		case "mtime":
			if left.MTime.Equal(right.MTime) {
				return assets[i].DisplayName < assets[j].DisplayName
			}
			return left.MTime.After(right.MTime)
		case "media_kind":
			if assets[i].MediaKind == assets[j].MediaKind {
				return assets[i].DisplayName < assets[j].DisplayName
			}
			return assets[i].MediaKind < assets[j].MediaKind
		case "taken_at":
			if assets[i].TakenAt == nil || assets[j].TakenAt == nil {
				return assets[i].TakenAt != nil && assets[j].TakenAt == nil
			}
			if assets[i].TakenAt.Equal(*assets[j].TakenAt) {
				return assets[i].DisplayName < assets[j].DisplayName
			}
			return assets[i].TakenAt.After(*assets[j].TakenAt)
		default:
			return assets[i].DisplayName < assets[j].DisplayName
		}
	})
}

func (s *MemoryStore) CreateAlbum(_ context.Context, album Album) (Album, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if album.ID == "" {
		album.ID = id.NewUUID()
	}
	if album.Slug == "" {
		album.Slug = slugify(album.Title)
	}
	if album.Title == "" {
		album.Title = album.Slug
	}
	album.CreatedAt = now
	album.UpdatedAt = now
	s.albums[album.ID] = album
	return album, nil
}

func (s *MemoryStore) UpdateAlbum(_ context.Context, album Album) (Album, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.albums[album.ID]
	if !ok {
		return Album{}, ErrNotFound
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
	current.UpdatedAt = time.Now().UTC()
	s.albums[album.ID] = current
	return current, nil
}

func (s *MemoryStore) DeleteAlbum(_ context.Context, albumID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.albums[albumID]; !ok {
		return ErrNotFound
	}
	deleteAlbumTreeLocked(s, albumID)
	return nil
}

func deleteAlbumTreeLocked(s *MemoryStore, albumID string) {
	for id, album := range s.albums {
		if album.ParentID == albumID {
			deleteAlbumTreeLocked(s, id)
		}
	}
	delete(s.albums, albumID)
	delete(s.albumItems, albumID)
}

func (s *MemoryStore) GetAlbum(_ context.Context, albumID string) (Album, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	album, ok := s.albums[albumID]
	if !ok {
		return Album{}, ErrNotFound
	}
	album.ItemCount = len(s.albumItems[albumID])
	return album, nil
}

func (s *MemoryStore) ListAlbums(_ context.Context, query AlbumQuery) ([]Album, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Album{}
	for _, album := range s.albums {
		if !query.Tree && album.ParentID != query.ParentID {
			continue
		}
		album.ItemCount = len(s.albumItems[album.ID])
		out = append(out, album)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder == out[j].SortOrder {
			return out[i].Title < out[j].Title
		}
		return out[i].SortOrder < out[j].SortOrder
	})
	limit, offset := normalizePage(query.Limit, query.Offset)
	if offset >= len(out) {
		return []Album{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return append([]Album(nil), out[offset:end]...), nil
}

func (s *MemoryStore) AddAlbumItems(_ context.Context, albumID string, assetIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.albums[albumID]; !ok {
		return ErrNotFound
	}
	if s.albumItems[albumID] == nil {
		s.albumItems[albumID] = map[string]AlbumItem{}
	}
	now := time.Now().UTC()
	for _, assetID := range assetIDs {
		asset, ok := s.assets[assetID]
		if !ok {
			return ErrNotFound
		}
		s.albumItems[albumID][assetID] = AlbumItem{AlbumID: albumID, Asset: cloneAsset(asset), AddedAt: now}
	}
	return nil
}

func (s *MemoryStore) RemoveAlbumItem(_ context.Context, albumID, assetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, ok := s.albumItems[albumID]
	if !ok {
		return ErrNotFound
	}
	delete(items, assetID)
	return nil
}

func (s *MemoryStore) ListAlbumItems(_ context.Context, query AlbumItemQuery) (AlbumItemPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]AlbumItem, 0, len(s.albumItems[query.AlbumID]))
	for _, item := range s.albumItems[query.AlbumID] {
		item.Asset = cloneAsset(item.Asset)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder == items[j].SortOrder {
			return items[i].Asset.DisplayName < items[j].Asset.DisplayName
		}
		return items[i].SortOrder < items[j].SortOrder
	})
	total := len(items)
	limit, offset := normalizePage(query.Limit, query.Offset)
	if offset >= len(items) {
		items = []AlbumItem{}
	} else {
		end := offset + limit
		if end > len(items) {
			end = len(items)
		}
		items = items[offset:end]
	}
	return AlbumItemPage{Items: items, Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (s *MemoryStore) UpsertAssetGeo(_ context.Context, geo AssetGeo, force bool) (AssetGeo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[geo.AssetID]; !ok {
		return AssetGeo{}, ErrNotFound
	}
	now := time.Now().UTC()
	if current, ok := s.assetGeo[geo.AssetID]; ok && !force && geoPriority(geo.Source) < geoPriority(current.Source) {
		return current, nil
	}
	if geo.CreatedAt.IsZero() {
		geo.CreatedAt = now
	}
	geo.UpdatedAt = now
	if geo.Metadata == nil {
		geo.Metadata = map[string]any{}
	}
	s.assetGeo[geo.AssetID] = geo
	return geo, nil
}

func (s *MemoryStore) GetAssetGeo(_ context.Context, assetID string) (AssetGeo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	geo, ok := s.assetGeo[assetID]
	if !ok {
		return AssetGeo{}, ErrNotFound
	}
	return cloneGeo(geo), nil
}

func (s *MemoryStore) QueryAssetGeo(_ context.Context, query GeoQuery) ([]GeoAsset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []GeoAsset
	for assetID, geo := range s.assetGeo {
		asset, ok := s.assets[assetID]
		if !ok || !geoMatches(geo, query) {
			continue
		}
		if query.MediaKind != "" && asset.MediaKind != query.MediaKind {
			continue
		}
		out = append(out, GeoAsset{Asset: cloneAsset(asset), Geo: cloneGeo(geo)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Asset.DisplayName < out[j].Asset.DisplayName })
	limit, offset := normalizePage(query.Limit, query.Offset)
	if offset >= len(out) {
		return []GeoAsset{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func geoMatches(geo AssetGeo, query GeoQuery) bool {
	if query.Source != "" && geo.Source != query.Source {
		return false
	}
	if query.BBox != nil && (geo.Lon < query.BBox.MinLon || geo.Lon > query.BBox.MaxLon || geo.Lat < query.BBox.MinLat || geo.Lat > query.BBox.MaxLat) {
		return false
	}
	if query.TimeFrom != nil && (geo.TakenAt == nil || geo.TakenAt.Before(*query.TimeFrom)) {
		return false
	}
	if query.TimeTo != nil && (geo.TakenAt == nil || geo.TakenAt.After(*query.TimeTo)) {
		return false
	}
	return true
}

func geoPriority(source string) int {
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

func cloneGeo(geo AssetGeo) AssetGeo {
	if geo.Metadata != nil {
		metadata := make(map[string]any, len(geo.Metadata))
		for k, v := range geo.Metadata {
			metadata[k] = v
		}
		geo.Metadata = metadata
	}
	return geo
}

func (s *MemoryStore) UpsertGPSTrackSummary(_ context.Context, summary TrackSummary, _ map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gpsTracks[summary.TrackAssetID] = summary
	return nil
}

func (s *MemoryStore) ListGPSTracks(ctx context.Context, query GPSTrackQuery) ([]TrackSummary, error) {
	tracks, err := s.ListTracks(ctx)
	if err != nil {
		return nil, err
	}
	for i := range tracks {
		if stored, ok := s.gpsTracks[tracks[i].TrackAssetID]; ok {
			tracks[i] = stored
		}
	}
	var out []TrackSummary
	q := strings.ToLower(strings.TrimSpace(query.Q))
	for _, track := range tracks {
		if q != "" && !strings.Contains(strings.ToLower(track.Name), q) {
			continue
		}
		if query.BBox != nil && !trackIntersectsBBox(track, *query.BBox) {
			continue
		}
		if query.TimeFrom != nil && (track.EndTime == nil || track.EndTime.Before(*query.TimeFrom)) {
			continue
		}
		if query.TimeTo != nil && (track.StartTime == nil || track.StartTime.After(*query.TimeTo)) {
			continue
		}
		out = append(out, track)
	}
	limit, offset := normalizePage(query.Limit, query.Offset)
	if offset >= len(out) {
		return []TrackSummary{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func trackIntersectsBBox(track TrackSummary, bbox BBox) bool {
	if track.MinLat == nil || track.MinLon == nil || track.MaxLat == nil || track.MaxLon == nil {
		return false
	}
	return *track.MinLon <= bbox.MaxLon && *track.MaxLon >= bbox.MinLon && *track.MinLat <= bbox.MaxLat && *track.MaxLat >= bbox.MinLat
}

func (s *MemoryStore) UpdateGPSTrackMetadata(_ context.Context, trackAssetID, title, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	summary, ok := s.gpsTracks[trackAssetID]
	if !ok {
		asset, ok := s.assets[trackAssetID]
		if !ok {
			return ErrNotFound
		}
		summary = summarizeTrack(asset, s.trackPoints[trackAssetID])
	}
	if title != "" {
		summary.Name = title
	}
	s.gpsTracks[trackAssetID] = summary
	if asset, ok := s.assets[trackAssetID]; ok {
		if title != "" {
			asset.DisplayName = title
		}
		if asset.Metadata == nil {
			asset.Metadata = map[string]any{}
		}
		asset.Metadata["gps_track_description"] = description
		s.assets[trackAssetID] = asset
	}
	return nil
}

func (s *MemoryStore) QueryTrackPoints(_ context.Context, query TrackPointQuery) ([]TrackPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	points := append([]TrackPoint(nil), s.trackPoints[query.TrackAssetID]...)
	var out []TrackPoint
	for _, point := range points {
		if query.TimeFrom != nil && point.RecordedAt.Before(*query.TimeFrom) {
			continue
		}
		if query.TimeTo != nil && point.RecordedAt.After(*query.TimeTo) {
			continue
		}
		out = append(out, point)
	}
	if query.Simplify {
		maxPoints := query.MaxPoints
		if maxPoints <= 0 {
			maxPoints = 500
		}
		out = simplifyCatalogPoints(out, maxPoints)
	}
	return out, nil
}

func (s *MemoryStore) QueryTrackPointsBatch(ctx context.Context, query TrackPointBatchQuery) (map[string][]TrackPoint, error) {
	out := make(map[string][]TrackPoint, len(query.TrackAssetIDs))
	for _, trackID := range query.TrackAssetIDs {
		points, err := s.QueryTrackPoints(ctx, TrackPointQuery{TrackAssetID: trackID, Simplify: true, MaxPoints: query.MaxPointsPerTrack})
		if err != nil {
			return nil, err
		}
		out[trackID] = points
	}
	return out, nil
}

func simplifyCatalogPoints(points []TrackPoint, maxPoints int) []TrackPoint {
	if maxPoints <= 0 || len(points) <= maxPoints {
		return points
	}
	step := float64(len(points)-1) / float64(maxPoints-1)
	out := make([]TrackPoint, 0, maxPoints)
	for i := 0; i < maxPoints; i++ {
		idx := int(float64(i) * step)
		if idx >= len(points) {
			idx = len(points) - 1
		}
		out = append(out, points[idx])
	}
	return out
}

func (s *MemoryStore) QueryTrackAssets(ctx context.Context, query TrackAssetQuery) (AssetPage, error) {
	detail, err := s.GetTrack(ctx, query.TrackAssetID)
	if err != nil {
		return AssetPage{}, err
	}
	limit, offset := normalizePage(query.Limit, query.Offset)
	if detail.Summary.StartTime == nil || detail.Summary.EndTime == nil {
		return AssetPage{Assets: []Asset{}, Page: Page{Limit: limit, Offset: offset}}, nil
	}
	start := detail.Summary.StartTime.Add(time.Duration(query.OffsetSeconds) * time.Second)
	end := detail.Summary.EndTime.Add(time.Duration(query.OffsetSeconds) * time.Second)
	mediaKinds := TrackAssetAllowedMediaKinds(query.MediaKind, query.ExcludeTrackAssets)
	assetQueryMediaKind := ""
	if len(mediaKinds) == 1 {
		assetQueryMediaKind = mediaKinds[0]
	}
	page, err := s.QueryAssets(ctx, AssetQuery{MediaKind: assetQueryMediaKind, Limit: 500, Offset: 0})
	if err != nil {
		return AssetPage{}, err
	}
	allAssets := append([]Asset(nil), page.Assets...)
	for nextOffset := len(page.Assets); nextOffset < page.Page.Total; nextOffset += len(page.Assets) {
		if len(page.Assets) == 0 {
			break
		}
		next, err := s.QueryAssets(ctx, AssetQuery{MediaKind: assetQueryMediaKind, Limit: 500, Offset: nextOffset})
		if err != nil {
			return AssetPage{}, err
		}
		allAssets = append(allAssets, next.Assets...)
		page = next
	}
	var filtered []Asset
	for _, asset := range allAssets {
		if !TrackAssetMediaKindAllowed(asset.MediaKind, mediaKinds, query.ExcludeTrackAssets) {
			continue
		}
		_, geotagged := s.assetGeo[asset.ID]
		if geotagged && !query.IncludeGeotagged {
			continue
		}
		if !geotagged && !query.IncludeUngeotagged {
			continue
		}
		if _, ok := AssetTimestampInRange(asset, start, end, 90*time.Minute, time.Local); !ok {
			continue
		}
		filtered = append(filtered, asset)
	}
	total := len(filtered)
	if offset >= len(filtered) {
		filtered = []Asset{}
	} else {
		endIndex := offset + limit
		if endIndex > len(filtered) {
			endIndex = len(filtered)
		}
		filtered = filtered[offset:endIndex]
	}
	page.Assets = filtered
	page.Page = Page{Limit: limit, Offset: offset, Total: total}
	return page, nil
}

func TrackAssetAllowedMediaKinds(raw string, excludeTracks bool) []string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" && excludeTracks {
		return []string{"photo", "video"}
	}
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if excludeTracks && (part == "track" || part == "gps" || part == "gpx" || part == "kml") {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return out
}

func TrackAssetMediaKindAllowed(kind string, allowed []string, excludeTracks bool) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if excludeTracks && kind == "track" {
		return false
	}
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if candidate == kind {
			return true
		}
	}
	return false
}

func (s *MemoryStore) CreateScanRun(_ context.Context, run ScanRun) (ScanRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run.ID == "" {
		run.ID = id.NewUUID()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	if run.Report == nil {
		run.Report = map[string]any{}
	}
	s.scanRuns[run.ID] = run
	return run, nil
}

func (s *MemoryStore) UpdateScanRunReport(_ context.Context, runID string, report map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.scanRuns[runID]
	if !ok {
		return ErrNotFound
	}
	run.Report = report
	s.scanRuns[runID] = run
	return nil
}

func (s *MemoryStore) FinishScanRun(_ context.Context, runID string, report map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.scanRuns[runID]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	run.FinishedAt = &now
	run.Report = report
	s.scanRuns[runID] = run
	return nil
}

func (s *MemoryStore) GetScanRunByJob(_ context.Context, jobID string) (ScanRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, run := range s.scanRuns {
		if run.JobID == jobID {
			return run, nil
		}
	}
	return ScanRun{}, ErrNotFound
}

func (s *MemoryStore) ListScanRuns(_ context.Context, query ScanRunQuery) ([]ScanRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ScanRun
	for _, run := range s.scanRuns {
		if query.StorageName != "" && run.StorageName != query.StorageName {
			continue
		}
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	limit, offset := normalizePage(query.Limit, query.Offset)
	if offset >= len(out) {
		return []ScanRun{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (s *MemoryStore) UpsertPreviewCacheEntry(_ context.Context, entry PreviewCacheEntry) (PreviewCacheEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		entry.ID = id.NewUUID()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	key := previewEntryKey(entry.AssetID, entry.Variant, entry.Width, entry.Height, entry.Format)
	if existing, ok := s.previewEntries[key]; ok {
		entry.ID = existing.ID
		entry.CreatedAt = existing.CreatedAt
	}
	s.previewEntries[key] = entry
	return entry, nil
}

func (s *MemoryStore) GetPreviewCacheEntry(_ context.Context, assetID, variant string, width, height int, format string) (PreviewCacheEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.previewEntries[previewEntryKey(assetID, variant, width, height, format)]
	if !ok {
		return PreviewCacheEntry{}, ErrNotFound
	}
	return entry, nil
}

func (s *MemoryStore) ListPreviewCacheEntries(_ context.Context, query PreviewCacheQuery) ([]PreviewCacheEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []PreviewCacheEntry
	for _, entry := range s.previewEntries {
		if query.AssetID != "" && entry.AssetID != query.AssetID {
			continue
		}
		if query.Status != "" && entry.Status != query.Status {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	limit, offset := normalizePage(query.Limit, query.Offset)
	if offset >= len(out) {
		return []PreviewCacheEntry{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (s *MemoryStore) MarkPreviewAccessed(_ context.Context, entryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for key, entry := range s.previewEntries {
		if entry.ID == entryID {
			entry.LastAccessedAt = &now
			s.previewEntries[key] = entry
			return nil
		}
	}
	return ErrNotFound
}

func (s *MemoryStore) PreviewCacheStats(_ context.Context) (PreviewCacheStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var stats PreviewCacheStats
	for _, entry := range s.previewEntries {
		stats.Entries++
		stats.Bytes += entry.SizeBytes
		if entry.Status == "ready" {
			stats.Ready++
		}
		if entry.Status == "failed" {
			stats.Failed++
		}
		if stats.OldestUnix == 0 || entry.CreatedAt.Unix() < stats.OldestUnix {
			stats.OldestUnix = entry.CreatedAt.Unix()
		}
	}
	return stats, nil
}

func (s *MemoryStore) CleanupPreviewCacheEntries(_ context.Context, olderThan time.Time, maxBytes int64) ([]PreviewCacheEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var entries []PreviewCacheEntry
	var total int64
	for _, entry := range s.previewEntries {
		entries = append(entries, entry)
		total += entry.SizeBytes
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.Before(entries[j].CreatedAt) })
	var deleted []PreviewCacheEntry
	for _, entry := range entries {
		if (!olderThan.IsZero() && entry.CreatedAt.Before(olderThan)) || (maxBytes > 0 && total > maxBytes) {
			delete(s.previewEntries, previewEntryKey(entry.AssetID, entry.Variant, entry.Width, entry.Height, entry.Format))
			deleted = append(deleted, entry)
			total -= entry.SizeBytes
		}
	}
	return deleted, nil
}

func (s *MemoryStore) ListTranscodingPresets(_ context.Context) ([]TranscodingPreset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TranscodingPreset, 0, len(s.transcodePresets))
	for _, preset := range s.transcodePresets {
		out = append(out, cloneTranscodingPreset(preset))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *MemoryStore) UpsertTranscodingPreset(_ context.Context, preset TranscodingPreset) (TranscodingPreset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if preset.ID == "" {
		preset.ID = slugify(preset.Name)
	}
	if preset.Name == "" {
		preset.Name = preset.ID
	}
	if preset.CreatedAt.IsZero() {
		if current, ok := s.transcodePresets[preset.ID]; ok {
			preset.CreatedAt = current.CreatedAt
		} else {
			preset.CreatedAt = now
		}
	}
	preset.UpdatedAt = now
	preset.BuiltIn = false
	preset.Available = true
	s.transcodePresets[preset.ID] = cloneTranscodingPreset(preset)
	return cloneTranscodingPreset(preset), nil
}

func (s *MemoryStore) DeleteTranscodingPreset(_ context.Context, presetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.transcodePresets[presetID]; !ok {
		return ErrNotFound
	}
	delete(s.transcodePresets, presetID)
	return nil
}

func (s *MemoryStore) UpsertAssetTag(_ context.Context, tag AssetTag) (AssetTag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[tag.AssetID]; !ok {
		return AssetTag{}, ErrNotFound
	}
	if tag.Tag == "" {
		return AssetTag{}, ErrInvalid
	}
	if tag.Source == "" {
		tag.Source = "manual"
	}
	if tag.Metadata == nil {
		tag.Metadata = map[string]any{}
	}
	if tag.CreatedAt.IsZero() {
		tag.CreatedAt = time.Now().UTC()
	}
	if s.assetTags[tag.AssetID] == nil {
		s.assetTags[tag.AssetID] = map[string]AssetTag{}
	}
	key := tag.Source + "\x00" + tag.Tag
	s.assetTags[tag.AssetID][key] = tag
	return tag, nil
}

func (s *MemoryStore) ListAssetTags(_ context.Context, assetID string) ([]AssetTag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []AssetTag{}
	if assetID == "" {
		for _, tags := range s.assetTags {
			for _, tag := range tags {
				out = append(out, tag)
			}
		}
	} else {
		for _, tag := range s.assetTags[assetID] {
			out = append(out, tag)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tag == out[j].Tag {
			return out[i].Source < out[j].Source
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}

func (s *MemoryStore) CreateAIPrediction(_ context.Context, pred AIPrediction) (AIPrediction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[pred.AssetID]; !ok {
		return AIPrediction{}, ErrNotFound
	}
	if pred.ID == "" {
		pred.ID = id.NewUUID()
	}
	if pred.WorkerID == "" {
		pred.WorkerID = "local"
	}
	if pred.Metadata == nil {
		pred.Metadata = map[string]any{}
	}
	if pred.CreatedAt.IsZero() {
		pred.CreatedAt = time.Now().UTC()
	}
	s.aiPredictions[pred.AssetID] = append(s.aiPredictions[pred.AssetID], pred)
	return pred, nil
}

func (s *MemoryStore) ListAIPredictions(_ context.Context, assetID string) ([]AIPrediction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []AIPrediction{}
	if assetID == "" {
		for _, predictions := range s.aiPredictions {
			out = append(out, predictions...)
		}
	} else {
		out = append(out, s.aiPredictions[assetID]...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) DeleteAIPrediction(_ context.Context, assetID, predictionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	predictions := s.aiPredictions[assetID]
	for idx, prediction := range predictions {
		if prediction.ID == predictionID {
			s.aiPredictions[assetID] = append(predictions[:idx], predictions[idx+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (s *MemoryStore) UpsertPlace(_ context.Context, place PlaceCacheEntry) (PlaceCacheEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if place.ID == "" {
		place.ID = id.NewUUID()
	}
	if place.NormalizedName == "" {
		place.NormalizedName = normalizePlaceName(place.Name)
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
	if place.Metadata == nil {
		place.Metadata = map[string]any{}
	}
	if existing, ok := s.places[place.ID]; ok && !existing.CreatedAt.IsZero() {
		place.CreatedAt = existing.CreatedAt
	}
	if place.CreatedAt.IsZero() {
		place.CreatedAt = now
	}
	place.UpdatedAt = now
	s.places[place.ID] = place
	return place, nil
}

func (s *MemoryStore) ListPlaces(_ context.Context, query PlaceQuery) ([]PlaceCacheEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit, offset := normalizePage(query.Limit, query.Offset)
	q := normalizePlaceName(query.Q)
	out := []PlaceCacheEntry{}
	for _, place := range s.places {
		if q != "" && !placeMatchesQuery(place, q) {
			continue
		}
		out = append(out, place)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	if offset > len(out) {
		return []PlaceCacheEntry{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return append([]PlaceCacheEntry{}, out[offset:end]...), nil
}

func (s *MemoryStore) DeletePlace(_ context.Context, placeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.places[placeID]; !ok {
		return ErrNotFound
	}
	delete(s.places, placeID)
	return nil
}

func normalizePlaceName(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func placeMatchesQuery(place PlaceCacheEntry, query string) bool {
	if query == "" {
		return true
	}
	text := normalizePlaceName(strings.Join([]string{
		place.Name,
		place.NormalizedName,
		place.DisplayName,
		place.Country,
		place.Region,
		place.City,
		place.Road,
		strings.Join(place.Aliases, " "),
	}, " "))
	return strings.Contains(text, query)
}

func (s *MemoryStore) CreateFaceDetection(_ context.Context, face FaceDetection) (FaceDetection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[face.AssetID]; !ok {
		return FaceDetection{}, ErrNotFound
	}
	if face.ID == "" {
		face.ID = id.NewUUID()
	}
	if face.Metadata == nil {
		face.Metadata = map[string]any{}
	}
	if face.CreatedAt.IsZero() {
		face.CreatedAt = time.Now().UTC()
	}
	s.faceDetections[face.AssetID] = append(s.faceDetections[face.AssetID], face)
	return face, nil
}

func (s *MemoryStore) ListFaceDetections(_ context.Context, assetID string) ([]FaceDetection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []FaceDetection{}
	if assetID == "" {
		for _, detections := range s.faceDetections {
			out = append(out, detections...)
		}
	} else {
		out = append(out, s.faceDetections[assetID]...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) UpsertFaceCluster(_ context.Context, cluster FaceCluster) (FaceCluster, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if cluster.ID == "" {
		cluster.ID = id.NewUUID()
	}
	existing := s.faceClusters[cluster.ID]
	if cluster.Label == "" {
		cluster.Label = existing.Label
	}
	if cluster.Metadata == nil {
		cluster.Metadata = existing.Metadata
	}
	if cluster.Metadata == nil {
		cluster.Metadata = map[string]any{}
	}
	if existing.CreatedAt.IsZero() {
		cluster.CreatedAt = now
	} else {
		cluster.CreatedAt = existing.CreatedAt
	}
	cluster.UpdatedAt = now
	s.faceClusters[cluster.ID] = cluster
	return cluster, nil
}

func (s *MemoryStore) ListFaceClusters(_ context.Context) ([]FaceCluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FaceCluster, 0, len(s.faceClusters))
	for _, cluster := range s.faceClusters {
		out = append(out, cluster)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Label == out[j].Label {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].Label < out[j].Label
	})
	return out, nil
}

func (s *MemoryStore) UpdateFaceDetection(_ context.Context, face FaceDetection) (FaceDetection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	detections := s.faceDetections[face.AssetID]
	for i := range detections {
		if detections[i].ID == face.ID {
			if face.Metadata == nil {
				face.Metadata = map[string]any{}
			}
			if face.CreatedAt.IsZero() {
				face.CreatedAt = detections[i].CreatedAt
			}
			detections[i] = face
			s.faceDetections[face.AssetID] = detections
			return face, nil
		}
	}
	return FaceDetection{}, ErrNotFound
}

func (s *MemoryStore) UpsertEmbeddingModel(_ context.Context, model EmbeddingModel) (EmbeddingModel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if model.ID == "" {
		model.ID = model.ModelName + ":" + model.Version + ":" + model.Modality
	}
	if model.Metadata == nil {
		model.Metadata = map[string]any{}
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now().UTC()
	}
	s.embeddingModels[model.ID] = model
	return model, nil
}

func (s *MemoryStore) UpsertAssetEmbedding(_ context.Context, embedding AssetEmbedding) (AssetEmbedding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[embedding.AssetID]; !ok {
		return AssetEmbedding{}, ErrNotFound
	}
	if _, ok := s.embeddingModels[embedding.ModelID]; !ok {
		return AssetEmbedding{}, ErrNotFound
	}
	if embedding.ID == "" {
		embedding.ID = id.NewUUID()
	}
	if embedding.SourceRef == "" {
		embedding.SourceRef = "asset"
	}
	if embedding.Metadata == nil {
		embedding.Metadata = map[string]any{}
	}
	if embedding.CreatedAt.IsZero() {
		embedding.CreatedAt = time.Now().UTC()
	}
	key := embedding.AssetID + "\x00" + embedding.ModelID + "\x00" + embedding.Modality + "\x00" + embedding.SourceRef
	s.assetEmbeddings[key] = embedding
	return embedding, nil
}

func (s *MemoryStore) ListAssetEmbeddings(_ context.Context, assetID string) ([]AssetEmbedding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []AssetEmbedding{}
	for key, embedding := range s.assetEmbeddings {
		if assetID == "" || strings.HasPrefix(key, assetID+"\x00") {
			out = append(out, embedding)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) VectorSearch(_ context.Context, modelID string, vector []float64, limit int) ([]VectorSearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	results := []VectorSearchResult{}
	for _, embedding := range s.assetEmbeddings {
		if modelID != "" && embedding.ModelID != modelID {
			continue
		}
		score := cosineSimilarity(vector, embedding.Vector)
		asset, ok := s.assets[embedding.AssetID]
		if !ok {
			continue
		}
		results = append(results, VectorSearchResult{Asset: cloneAsset(asset), Score: score, Match: embedding.ModelID})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *MemoryStore) UpsertComponent(_ context.Context, component Component) (Component, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	component.Key = strings.TrimSpace(component.Key)
	if component.Key == "" {
		return Component{}, errors.New("component key is required")
	}
	if existing, ok := s.components[component.Key]; ok && component.ID == "" {
		component.ID = existing.ID
	}
	if component.ID == "" {
		component.ID = id.NewUUID()
	}
	if component.Status == "" {
		component.Status = "missing"
	}
	if component.SourceType == "" {
		component.SourceType = "system_path"
	}
	if component.Metadata == nil {
		component.Metadata = map[string]any{}
	}
	s.components[component.Key] = component
	return component, nil
}

func (s *MemoryStore) ListComponents(_ context.Context, query ComponentQuery) ([]Component, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit, offset := normalizePage(query.Limit, query.Offset)
	q := strings.ToLower(strings.TrimSpace(query.Q))
	out := []Component{}
	for _, component := range s.components {
		if query.Category != "" && component.Category != query.Category {
			continue
		}
		if query.Status != "" && component.Status != query.Status {
			continue
		}
		if q != "" {
			text := strings.ToLower(strings.Join([]string{component.Key, component.Name, component.Category, component.Version, component.Status, component.SourceType, component.LicenseName}, " "))
			if !strings.Contains(text, q) {
				continue
			}
		}
		out = append(out, component)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category == out[j].Category {
			return out[i].Key < out[j].Key
		}
		return out[i].Category < out[j].Category
	})
	if offset > len(out) {
		return []Component{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return append([]Component{}, out[offset:end]...), nil
}

func (s *MemoryStore) GetComponent(_ context.Context, key string) (Component, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	component, ok := s.components[strings.TrimSpace(key)]
	if !ok {
		return Component{}, ErrNotFound
	}
	return component, nil
}

func (s *MemoryStore) AddComponentEvent(_ context.Context, event ComponentEvent) (ComponentEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event.ComponentKey = strings.TrimSpace(event.ComponentKey)
	if event.ComponentKey == "" {
		return ComponentEvent{}, errors.New("component key is required")
	}
	if event.ID == "" {
		event.ID = id.NewUUID()
	}
	if event.Level == "" {
		event.Level = "info"
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	s.componentEvents[event.ComponentKey] = append(s.componentEvents[event.ComponentKey], event)
	return event, nil
}

func (s *MemoryStore) ListComponentEvents(_ context.Context, key string, limit int) ([]ComponentEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	events := append([]ComponentEvent{}, s.componentEvents[strings.TrimSpace(key)]...)
	sort.Slice(events, func(i, j int) bool { return events[i].CreatedAt.After(events[j].CreatedAt) })
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

func cosineSimilarity(a, b []float64) float64 {
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

func cloneTranscodingPreset(preset TranscodingPreset) TranscodingPreset {
	if preset.ExtraArgs != nil {
		extra := make(map[string]any, len(preset.ExtraArgs))
		for key, value := range preset.ExtraArgs {
			extra[key] = value
		}
		preset.ExtraArgs = extra
	}
	return preset
}

func previewEntryKey(assetID, variant string, width, height int, format string) string {
	return assetID + "|" + variant + "|" + format + "|" + strconv.Itoa(width) + "|" + strconv.Itoa(height)
}

func slugify(input string) string {
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
