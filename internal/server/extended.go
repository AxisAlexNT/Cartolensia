package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/discovery"
	"github.com/AxisAlexNT/Cartolensia/internal/gpx"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/metadata"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

func (s *Server) handleAlbums(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		query := r.URL.Query()
		limit, _ := strconv.Atoi(query.Get("limit"))
		offset, _ := strconv.Atoi(query.Get("offset"))
		albums, err := s.deps.Store.ListAlbums(r.Context(), catalog.AlbumQuery{
			ParentID: query.Get("parent_id"),
			Tree:     boolQuery(query.Get("tree")),
			Limit:    limit,
			Offset:   offset,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, albums)
	case http.MethodPost:
		if !s.requireWrite(w, r, "albums.write") {
			return
		}
		var req struct {
			ParentID    string `json:"parent_id"`
			Slug        string `json:"slug"`
			Title       string `json:"title"`
			Description string `json:"description"`
			SortOrder   int    `json:"sort_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Title) == "" && strings.TrimSpace(req.Slug) == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("album title or slug is required"))
			return
		}
		album, err := s.deps.Store.CreateAlbum(r.Context(), catalog.Album{
			ParentID:    strings.TrimSpace(req.ParentID),
			Slug:        strings.TrimSpace(req.Slug),
			Title:       strings.TrimSpace(req.Title),
			Description: req.Description,
			SortOrder:   req.SortOrder,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, album)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAlbumByID(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/v1/albums/"))
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	albumID := parts[0]
	if len(parts) == 1 {
		s.handleAlbumMetadata(w, r, albumID)
		return
	}
	if len(parts) == 2 && parts[1] == "items" {
		s.handleAlbumItems(w, r, albumID)
		return
	}
	if len(parts) == 3 && parts[1] == "items" {
		s.handleAlbumItemByID(w, r, albumID, parts[2])
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleAlbumMetadata(w http.ResponseWriter, r *http.Request, albumID string) {
	switch r.Method {
	case http.MethodGet:
		album, err := s.deps.Store.GetAlbum(r.Context(), albumID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, album)
	case http.MethodPatch:
		if !s.requireWrite(w, r, "albums.write") {
			return
		}
		current, err := s.deps.Store.GetAlbum(r.Context(), albumID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		var req struct {
			ParentID    *string `json:"parent_id"`
			Slug        *string `json:"slug"`
			Title       *string `json:"title"`
			Description *string `json:"description"`
			SortOrder   *int    `json:"sort_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.ParentID != nil {
			current.ParentID = strings.TrimSpace(*req.ParentID)
		}
		if req.Slug != nil {
			current.Slug = strings.TrimSpace(*req.Slug)
		}
		if req.Title != nil {
			current.Title = strings.TrimSpace(*req.Title)
		}
		if req.Description != nil {
			current.Description = *req.Description
		}
		if req.SortOrder != nil {
			current.SortOrder = *req.SortOrder
		}
		album, err := s.deps.Store.UpdateAlbum(r.Context(), current)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, album)
	case http.MethodDelete:
		if !s.requireWrite(w, r, "albums.write") {
			return
		}
		if err := s.deps.Store.DeleteAlbum(r.Context(), albumID); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "album_metadata_deleted"})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAlbumItems(w http.ResponseWriter, r *http.Request, albumID string) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		page, err := s.deps.Store.ListAlbumItems(r.Context(), catalog.AlbumItemQuery{AlbumID: albumID, Limit: limit, Offset: offset})
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	case http.MethodPost:
		if !s.requireWrite(w, r, "albums.write") {
			return
		}
		var req struct {
			AssetID  string   `json:"asset_id"`
			AssetIDs []string `json:"asset_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		assetIDs := compactStrings(req.AssetIDs)
		if req.AssetID != "" {
			assetIDs = append(assetIDs, req.AssetID)
		}
		if len(assetIDs) == 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("asset_id or asset_ids is required"))
			return
		}
		if err := s.deps.Store.AddAlbumItems(r.Context(), albumID, assetIDs); err != nil {
			writeStoreError(w, err)
			return
		}
		page, _ := s.deps.Store.ListAlbumItems(r.Context(), catalog.AlbumItemQuery{AlbumID: albumID, Limit: 200})
		writeJSON(w, http.StatusCreated, page)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAlbumItemByID(w http.ResponseWriter, r *http.Request, albumID, assetID string) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "albums.write") {
		return
	}
	if err := s.deps.Store.RemoveAlbumItem(r.Context(), albumID, assetID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "album_item_removed"})
}

func (s *Server) handleGPSTracks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query, err := gpsTrackQueryFromURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tracks, err := s.deps.Store.ListGPSTracks(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if tracks == nil {
		tracks = []catalog.TrackSummary{}
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Server) handleGPSTrackByID(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/v1/gps/tracks/"))
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	trackID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			detail, err := s.deps.Store.GetTrack(r.Context(), trackID)
			if err != nil {
				writeStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, detail)
		case http.MethodPatch:
			if !s.requireWrite(w, r, "gps.tracks.write") {
				return
			}
			var req struct {
				Title       string `json:"title"`
				Description string `json:"description"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if err := s.deps.Store.UpdateGPSTrackMetadata(r.Context(), trackID, strings.TrimSpace(req.Title), req.Description); err != nil {
				writeStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
		default:
			methodNotAllowed(w)
		}
		return
	}
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "points":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		points, err := s.deps.Store.QueryTrackPoints(r.Context(), trackPointQueryFromURL(r, trackID))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, points)
	case "profile":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		s.handleGPSTrackProfile(w, r, trackID)
	case "assets":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		query := trackAssetQueryFromURL(r, trackID)
		detail, err := s.deps.Store.GetTrack(r.Context(), trackID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		page, err := s.deps.Store.QueryTrackAssets(r.Context(), query)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, trackAssetResponse(detail.Summary, page, query))
	case "point-info":
		s.handleGPSTrackPointInfo(w, r, trackID)
	case "nearby-assets":
		s.handleGPSTrackNearbyAssets(w, r, trackID)
	case "snap-media":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if !s.requireWrite(w, r, "gps.tracks.snap") {
			return
		}
		payload := map[string]any{"track_asset_id": trackID}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&payload)
			payload["track_asset_id"] = trackID
		}
		job, err := s.deps.Store.EnqueueJob(r.Context(), jobs.New("geo_snap", payload))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if s.deps.SyncJobs {
			runner := metadata.Runner{Registry: s.deps.Registry, Store: s.deps.Store}
			if err := runner.SnapToTrack(r.Context(), &job); err != nil && !errors.Is(err, jobs.ErrCanceled) {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		writeJSON(w, http.StatusAccepted, job)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleGPSTrackProfile(w http.ResponseWriter, r *http.Request, trackID string) {
	metric := strings.ToLower(strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("metric"), "altitude")))
	if metric != "altitude" && metric != "speed" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("metric must be altitude or speed"))
		return
	}
	detail, err := s.deps.Store.GetTrack(r.Context(), trackID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	points, err := s.deps.Store.QueryTrackPoints(r.Context(), trackPointQueryFromURL(r, trackID))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	profile := buildTrackProfile(detail.Summary, points, metric)
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) handleGPSTrackPointInfo(w http.ResponseWriter, r *http.Request, trackID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	lat, err := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err != nil || lat < -90 || lat > 90 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("valid lat query parameter is required"))
		return
	}
	lon, err := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if err != nil || lon < -180 || lon > 180 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("valid lon query parameter is required"))
		return
	}
	detail, err := s.deps.Store.GetTrack(r.Context(), trackID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	points, err := s.deps.Store.QueryTrackPoints(r.Context(), catalog.TrackPointQuery{TrackAssetID: trackID})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	info, ok := nearestTrackPointInfo(detail.Summary, points, lat, lon)
	if !ok {
		writeError(w, http.StatusNotFound, catalog.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleGPSTrackNearbyAssets(w http.ResponseWriter, r *http.Request, trackID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	distanceM, err := strconv.ParseFloat(firstNonEmpty(r.URL.Query().Get("distance_m"), "100"), 64)
	if err != nil || distanceM <= 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("distance_m must be a positive number"))
		return
	}
	limit := queryInt(r, "limit", 200)
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	detail, err := s.deps.Store.GetTrack(r.Context(), trackID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	points, err := s.deps.Store.QueryTrackPoints(r.Context(), catalog.TrackPointQuery{TrackAssetID: trackID, Simplify: true, MaxPoints: 3000})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if len(points) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"track": detail.Summary, "assets": []any{}, "distance_m": distanceM, "page": catalog.Page{Limit: limit, Offset: 0, Total: 0}})
		return
	}
	geoAssets, err := s.deps.Store.QueryAssetGeo(r.Context(), catalog.GeoQuery{Limit: 10000})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	type nearby struct {
		Asset           catalog.Asset `json:"asset"`
		DistanceM       float64       `json:"distance_m"`
		NearestLat      float64       `json:"nearest_lat"`
		NearestLon      float64       `json:"nearest_lon"`
		TrackAssetID    string        `json:"track_asset_id"`
		TrackSourceName string        `json:"track_name"`
	}
	matches := make([]nearby, 0, len(geoAssets))
	for _, geoAsset := range geoAssets {
		if geoAsset.Asset.ID == trackID || geoAsset.Asset.MediaKind == "track" {
			continue
		}
		nearest, ok := nearestPointOnTrack(points, geoAsset.Geo.Lat, geoAsset.Geo.Lon)
		if !ok || nearest.DistanceM > distanceM {
			continue
		}
		matches = append(matches, nearby{
			Asset:           geoAsset.Asset,
			DistanceM:       nearest.DistanceM,
			NearestLat:      nearest.Lat,
			NearestLon:      nearest.Lon,
			TrackAssetID:    trackID,
			TrackSourceName: detail.Summary.Name,
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if math.Abs(matches[i].DistanceM-matches[j].DistanceM) < 0.001 {
			return matches[i].Asset.DisplayName < matches[j].Asset.DisplayName
		}
		return matches[i].DistanceM < matches[j].DistanceM
	})
	total := len(matches)
	if len(matches) > limit {
		matches = matches[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"track":      detail.Summary,
		"assets":     matches,
		"distance_m": distanceM,
		"page":       catalog.Page{Limit: limit, Offset: 0, Total: total},
	})
}

func (s *Server) handleMapSubroute(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/v1/map/"))
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	switch parts[0] {
	case "status":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		stats, _ := s.deps.Store.Stats(r.Context())
		geoAssets, _ := s.deps.Store.QueryAssetGeo(r.Context(), catalog.GeoQuery{Limit: 1})
		tracks, _ := s.deps.Store.ListGPSTracks(r.Context(), catalog.GPSTrackQuery{Limit: 10000})
		geotaggedCount := 0
		if len(geoAssets) > 0 {
			geotaggedCount = -1
		}
		if allGeo, err := s.deps.Store.QueryAssetGeo(r.Context(), catalog.GeoQuery{Limit: 10000}); err == nil {
			geotaggedCount = len(allGeo)
		}
		warnings := []string{}
		if stats.Assets > 0 && geotaggedCount == 0 {
			warnings = append(warnings, "No geotagged assets are indexed yet. Run metadata enrichment or choose media with EXIF/GPS.")
		}
		if stats.Tracks == 0 && len(tracks) == 0 {
			warnings = append(warnings, "No GPX/GPS tracks are indexed in the current scan.")
		} else if stats.Tracks > 0 && len(tracks) == 0 {
			warnings = append(warnings, fmt.Sprintf("%d track-like assets are indexed, but no parsed GPS/KML/KMZ/GPZ summaries exist yet. Run metadata enrichment for track files.", stats.Tracks))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"backend":          "geojson",
			"base_tiles":       "cartolensia_proxy",
			"base_tiles_note":  "OpenStreetMap tiles are fetched on demand through a local Cartolensia cache; no bulk prefetch is implemented.",
			"clustering":       "screen_distance",
			"postgis":          capabilityInstalled(s.deps.Capabilities, "postgis"),
			"asset_geo":        true,
			"track_geometry":   true,
			"indexed_assets":   stats.Assets,
			"geotagged_assets": geotaggedCount,
			"track_assets":     stats.Tracks,
			"tracks":           len(tracks),
			"warnings":         warnings,
		})
	case "tile-sources":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, []map[string]any{{
			"id":          "osm",
			"name":        "OpenStreetMap Standard",
			"enabled":     true,
			"template":    "/api/v1/tiles/osm/{z}/{x}/{y}.png",
			"attribution": "© OpenStreetMap contributors",
			"policy":      "on-demand cache only; no bulk prefetch against public OSM tiles",
			"cache_dir":   filepath.Join(s.deps.Config.Cache.Dir, "tiles"),
		}})
	case "assets":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		features, clustering, err := s.mapAssetFeatures(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, geoFeatureCollection(features, clustering, queryInt(r, "zoom", 10)))
	case "tracks":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		features, err := s.mapTrackFeatures(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, geoFeatureCollection(features, "none", queryInt(r, "zoom", 10)))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDiscoveryDryRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "discovery.dry_run") {
		return
	}
	payload := discovery.DryRunPayload{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payload = discovery.NormalizeDryRunPayload(payload)
	if err := validateDryRunPayload(s.deps.Registry, payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job := jobs.New("discovery_dry_run", payload)
	createdJob, err := s.deps.Store.EnqueueJob(r.Context(), job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	run, err := s.deps.Store.CreateScanRun(r.Context(), catalog.ScanRun{
		JobID:             createdJob.ID,
		StorageName:       payload.Storage,
		Mode:              payload.Mode,
		Prefixes:          payload.Prefixes,
		MaxFiles:          payload.MaxFiles,
		MaxBytes:          payload.MaxBytes,
		HashRequested:     payload.Hash,
		MetadataRequested: payload.Metadata,
		PreviewsRequested: payload.Previews,
		MarkMissing:       payload.MarkMissing,
		DryRun:            true,
		Report:            map[string]any{"status": "queued", "safety": payload.SafetySummary()},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	payload.ScanRunID = run.ID
	createdJob.Payload = payload
	if err := s.deps.Store.UpdateJob(r.Context(), createdJob); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if s.deps.SyncJobs {
		runner := s.discoveryRunner()
		if err := runner.DryRun(r.Context(), &createdJob); err != nil && !errors.Is(err, jobs.ErrCanceled) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": createdJob, "scan_run": run})
}

func (s *Server) handleDiscoveryDryRunByJob(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/v1/discovery/dry-run/"))
	if len(parts) != 2 || parts[1] != "report" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	run, err := s.deps.Store.GetScanRunByJob(r.Context(), parts[0])
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handlePreviewStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	stats, err := s.deps.Store.PreviewCacheStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cache_dir": s.deps.Config.Cache.Dir, "stats": stats})
}

func (s *Server) handlePreviewCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	entries, err := s.deps.Store.ListPreviewCacheEntries(r.Context(), catalog.PreviewCacheQuery{
		AssetID: r.URL.Query().Get("asset_id"),
		Status:  r.URL.Query().Get("status"),
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handlePreviewCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "previews.cleanup") {
		return
	}
	var req struct {
		OlderThan string `json:"older_than"`
		MaxBytes  int64  `json:"max_bytes"`
		DryRun    *bool  `json:"dry_run"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	var olderThan time.Time
	if req.OlderThan != "" {
		parsed, err := time.Parse(time.RFC3339, req.OlderThan)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		olderThan = parsed
	}
	candidates, err := previewCleanupCandidates(r.Context(), s.deps.Store, olderThan, req.MaxBytes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if dryRun {
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "candidates": candidates, "deleted": 0})
		return
	}
	for _, entry := range candidates {
		if err := removePreviewCacheFile(s.deps.Config.Cache.Dir, entry.CachePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	deleted, err := s.deps.Store.CleanupPreviewCacheEntries(r.Context(), olderThan, req.MaxBytes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dry_run": false, "candidates": candidates, "deleted": len(deleted)})
}

func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func pathParts(raw string) []string {
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func compactStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func boolQuery(raw string) bool {
	return raw == "1" || strings.EqualFold(raw, "true") || strings.EqualFold(raw, "yes")
}

func assetQueryFromURL(r *http.Request) (catalog.AssetQuery, error) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))
	takenFrom, err := parseOptionalTime(firstNonEmpty(query.Get("taken_from"), query.Get("time_from")))
	if err != nil {
		return catalog.AssetQuery{}, err
	}
	takenTo, err := parseOptionalTime(firstNonEmpty(query.Get("taken_to"), query.Get("time_to")))
	if err != nil {
		return catalog.AssetQuery{}, err
	}
	return catalog.AssetQuery{
		Q:          query.Get("q"),
		MediaKind:  query.Get("media_kind"),
		HashStatus: query.Get("hash_status"),
		Storage:    firstNonEmpty(query.Get("storage"), query.Get("storage_name")),
		Extension:  query.Get("extension"),
		TakenFrom:  takenFrom,
		TakenTo:    takenTo,
		AlbumID:    query.Get("album_id"),
		TrackID:    query.Get("track_id"),
		GeoSource:  query.Get("geo_source"),
		Limit:      limit,
		Offset:     offset,
		Sort:       query.Get("sort"),
		WithTotal:  true,
	}, nil
}

func gpsTrackQueryFromURL(r *http.Request) (catalog.GPSTrackQuery, error) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))
	bboxValue, hasBBox, err := parseBBox(query.Get("bbox"))
	if err != nil {
		return catalog.GPSTrackQuery{}, err
	}
	var bboxPtr *catalog.BBox
	if hasBBox {
		b := catalog.BBox{MinLon: bboxValue.MinLon, MinLat: bboxValue.MinLat, MaxLon: bboxValue.MaxLon, MaxLat: bboxValue.MaxLat}
		bboxPtr = &b
	}
	timeFrom, err := parseOptionalTime(query.Get("time_from"))
	if err != nil {
		return catalog.GPSTrackQuery{}, err
	}
	timeTo, err := parseOptionalTime(query.Get("time_to"))
	if err != nil {
		return catalog.GPSTrackQuery{}, err
	}
	return catalog.GPSTrackQuery{Q: query.Get("q"), BBox: bboxPtr, TimeFrom: timeFrom, TimeTo: timeTo, Limit: limit, Offset: offset, Sort: query.Get("sort")}, nil
}

func trackPointQueryFromURL(r *http.Request, trackID string) catalog.TrackPointQuery {
	query := r.URL.Query()
	maxPoints, _ := strconv.Atoi(query.Get("max_points"))
	timeFrom, _ := parseOptionalTime(query.Get("time_from"))
	timeTo, _ := parseOptionalTime(query.Get("time_to"))
	simplify := true
	if query.Has("simplify") {
		simplify = boolQuery(query.Get("simplify"))
	}
	return catalog.TrackPointQuery{TrackAssetID: trackID, TimeFrom: timeFrom, TimeTo: timeTo, Simplify: simplify, MaxPoints: maxPoints}
}

func trackAssetQueryFromURL(r *http.Request, trackID string) catalog.TrackAssetQuery {
	query := r.URL.Query()
	offsetSeconds, _ := strconv.ParseInt(firstNonEmpty(query.Get("offset_seconds"), query.Get("offset")), 10, 64)
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("page_offset"))
	mediaKind := strings.TrimSpace(query.Get("media_kind"))
	if mediaKind == "" {
		mediaKind = "photo,video"
	}
	excludeTrackAssets := true
	if query.Has("exclude_track_assets") {
		excludeTrackAssets = boolQuery(query.Get("exclude_track_assets"))
	}
	includeGeo := true
	includeUngeo := true
	if query.Has("include_geotagged") {
		includeGeo = boolQuery(query.Get("include_geotagged"))
	}
	if query.Has("include_ungeotagged") {
		includeUngeo = boolQuery(query.Get("include_ungeotagged"))
	}
	return catalog.TrackAssetQuery{
		TrackAssetID:       trackID,
		OffsetSeconds:      offsetSeconds,
		MediaKind:          mediaKind,
		ExcludeTrackAssets: excludeTrackAssets,
		IncludeGeotagged:   includeGeo,
		IncludeUngeotagged: includeUngeo,
		Limit:              limit,
		Offset:             offset,
	}
}

func trackAssetResponse(summary catalog.TrackSummary, page catalog.AssetPage, query catalog.TrackAssetQuery) map[string]any {
	if page.Assets == nil {
		page.Assets = []catalog.Asset{}
	}
	reason := ""
	switch {
	case summary.StartTime == nil || summary.EndTime == nil:
		reason = "This track has no usable timestamps, so time-based media lookup cannot run. Use nearby geotagged media instead."
	case page.Page.Total == 0 && query.MediaKind != "":
		reason = "No matching photo/video assets overlap this track with the current filters and time offset."
	case page.Page.Total == 0:
		reason = "No assets overlap this track with the current filters and time offset."
	}
	return map[string]any{
		"track":                summary,
		"assets":               page.Assets,
		"page":                 page.Page,
		"reason":               reason,
		"media_kind":           query.MediaKind,
		"exclude_track_assets": query.ExcludeTrackAssets,
		"offset_seconds":       query.OffsetSeconds,
	}
}

func parseOptionalTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func queryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func geoFeatureCollection(features []map[string]any, clustering string, zoom int) map[string]any {
	if features == nil {
		features = []map[string]any{}
	}
	return map[string]any{"type": "FeatureCollection", "features": features, "clustering": clustering, "zoom": zoom}
}

func (s *Server) mapAssetFeatures(r *http.Request) ([]map[string]any, string, error) {
	query := r.URL.Query()
	bboxValue, hasBBox, err := parseBBox(query.Get("bbox"))
	if err != nil {
		return nil, "", err
	}
	var bboxPtr *catalog.BBox
	if hasBBox {
		b := catalog.BBox{MinLon: bboxValue.MinLon, MinLat: bboxValue.MinLat, MaxLon: bboxValue.MaxLon, MaxLat: bboxValue.MaxLat}
		bboxPtr = &b
	}
	timeFrom, err := parseOptionalTime(query.Get("time_from"))
	if err != nil {
		return nil, "", err
	}
	timeTo, err := parseOptionalTime(query.Get("time_to"))
	if err != nil {
		return nil, "", err
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))
	geoAssets, err := s.deps.Store.QueryAssetGeo(r.Context(), catalog.GeoQuery{
		BBox:      bboxPtr,
		Source:    query.Get("source"),
		MediaKind: query.Get("media_kind"),
		AlbumID:   query.Get("album_id"),
		TrackID:   query.Get("track_id"),
		TimeFrom:  timeFrom,
		TimeTo:    timeTo,
		Limit:     limit,
		Offset:    offset,
		Clusters:  boolQuery(firstNonEmpty(query.Get("clusters"), query.Get("cluster"))),
		Zoom:      queryInt(r, "zoom", 10),
	})
	if err != nil {
		return nil, "", err
	}
	selectedAssetIDs := csvSet(query.Get("asset_ids"))
	features := make([]map[string]any, 0, len(geoAssets))
	for _, item := range geoAssets {
		if len(selectedAssetIDs) > 0 {
			if _, ok := selectedAssetIDs[item.Asset.ID]; !ok {
				continue
			}
		}
		features = append(features, map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []float64{item.Geo.Lon, item.Geo.Lat},
			},
			"properties": map[string]any{
				"id":           item.Asset.ID,
				"name":         item.Asset.DisplayName,
				"kind":         item.Asset.MediaKind,
				"asset_type":   "asset",
				"source":       item.Geo.Source,
				"confidence":   item.Geo.Confidence,
				"track_id":     item.Geo.TrackAssetID,
				"taken_at":     item.Geo.TakenAt,
				"clustered":    false,
				"preview_url":  "/api/v1/media/" + item.Asset.ID + "/preview",
				"detail_url":   "/?page=asset-detail&asset_id=" + item.Asset.ID,
				"original_url": "/api/v1/media/" + item.Asset.ID + "/original",
			},
		})
	}
	clustering := "raw"
	if boolQuery(firstNonEmpty(query.Get("clusters"), query.Get("cluster"))) {
		distancePx := queryInt(r, "cluster_distance_px", queryInt(r, "marker_px", 24)*2)
		features = clusterGeoJSONPoints(features, bboxValue, hasBBox, queryInt(r, "zoom", 10), distancePx)
		clustering = "screen_distance"
	}
	return features, clustering, nil
}

func (s *Server) mapTrackFeatures(r *http.Request) ([]map[string]any, error) {
	query, err := gpsTrackQueryFromURL(r)
	if err != nil {
		return nil, err
	}
	selected := csvSet(r.URL.Query().Get("track_ids"))
	if r.URL.Query().Get("track_id") != "" {
		selected[r.URL.Query().Get("track_id")] = struct{}{}
	}
	tracks, err := s.deps.Store.ListGPSTracks(r.Context(), query)
	if err != nil {
		return nil, err
	}
	zoom := queryInt(r, "zoom", 10)
	features := make([]map[string]any, 0, len(tracks))
	for _, summary := range tracks {
		if len(selected) > 0 {
			if _, ok := selected[summary.TrackAssetID]; !ok {
				continue
			}
		}
		maxPoints := maxTrackPointsForZoom(zoom)
		points, err := s.deps.Store.QueryTrackPoints(r.Context(), catalog.TrackPointQuery{TrackAssetID: summary.TrackAssetID, Simplify: true, MaxPoints: maxPoints})
		if err != nil {
			continue
		}
		if len(points) > maxPoints {
			points = gpx.Simplify(points, maxPoints)
		}
		coords := make([][]float64, 0, len(points))
		for _, point := range points {
			coords = append(coords, []float64{point.Lon, point.Lat})
		}
		if len(coords) == 0 {
			continue
		}
		features = append(features, map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "LineString",
				"coordinates": coords,
			},
			"properties": map[string]any{
				"id":               summary.TrackAssetID,
				"name":             summary.Name,
				"kind":             "track",
				"asset_type":       "track",
				"source_file":      summary.Name,
				"source_format":    summary.SourceFormat,
				"point_count":      summary.PointCount,
				"start_time":       summary.StartTime,
				"end_time":         summary.EndTime,
				"distance_m":       summary.DistanceM,
				"duration_seconds": summary.DurationSec,
				"bbox": map[string]any{
					"min_lat": summary.MinLat,
					"min_lon": summary.MinLon,
					"max_lat": summary.MaxLat,
					"max_lon": summary.MaxLon,
				},
				"simplified": len(points) < summary.PointCount,
			},
		})
	}
	return features, nil
}

func validateDryRunPayload(registry *storage.Registry, payload discovery.DryRunPayload) error {
	if registry == nil {
		return fmt.Errorf("storage registry is not configured")
	}
	if payload.Storage == "" {
		return fmt.Errorf("storage is required")
	}
	found := false
	for _, storageConfig := range registry.ListStorages() {
		if storageConfig.Name == payload.Storage {
			found = true
			if storageConfig.Mode != "strict_read_only" {
				return fmt.Errorf("dry-run storage must be strict_read_only")
			}
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown storage %q", payload.Storage)
	}
	if len(payload.Prefixes) == 0 {
		return fmt.Errorf("dry-run prefixes are required")
	}
	for _, prefix := range payload.Prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" || prefix == "." || prefix == "/" {
			return fmt.Errorf("empty dry-run prefix is not allowed")
		}
	}
	if payload.MarkMissing {
		return fmt.Errorf("mark_missing is not allowed in dry-run")
	}
	if payload.MaxFiles > 50 && !payload.AllowOverLimit {
		return fmt.Errorf("dry-run max_files above 50 requires allow_over_limit")
	}
	return nil
}

func previewCleanupCandidates(ctx context.Context, store catalog.Store, olderThan time.Time, maxBytes int64) ([]catalog.PreviewCacheEntry, error) {
	entries, err := store.ListPreviewCacheEntries(ctx, catalog.PreviewCacheQuery{Limit: 500})
	if err != nil {
		return nil, err
	}
	var total int64
	for _, entry := range entries {
		total += entry.SizeBytes
	}
	var out []catalog.PreviewCacheEntry
	for _, entry := range entries {
		if (!olderThan.IsZero() && entry.CreatedAt.Before(olderThan)) || (maxBytes > 0 && total > maxBytes) {
			out = append(out, entry)
			total -= entry.SizeBytes
		}
	}
	return out, nil
}

func removePreviewCacheFile(cacheRoot, target string) error {
	root, err := filepath.Abs(cacheRoot)
	if err != nil {
		return err
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, cleanTarget)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("preview cache deletion target escapes cache root")
	}
	return os.Remove(cleanTarget)
}

type nearestTrackResult struct {
	Lat       float64
	Lon       float64
	DistanceM float64
	Fraction  float64
	Index     int
	NextIndex int
}

func nearestTrackPointInfo(summary catalog.TrackSummary, points []catalog.TrackPoint, lat, lon float64) (map[string]any, bool) {
	nearest, ok := nearestPointOnTrack(points, lat, lon)
	if !ok {
		return nil, false
	}
	point := points[nearest.Index]
	var next *catalog.TrackPoint
	if nearest.NextIndex >= 0 && nearest.NextIndex < len(points) {
		next = &points[nearest.NextIndex]
	}
	timestamp := point.RecordedAt
	if next != nil && !point.RecordedAt.IsZero() && !next.RecordedAt.IsZero() && nearest.Fraction > 0 {
		delta := next.RecordedAt.Sub(point.RecordedAt)
		timestamp = point.RecordedAt.Add(time.Duration(float64(delta) * nearest.Fraction))
	}
	var relativeSeconds *float64
	if summary.StartTime != nil && !timestamp.IsZero() {
		value := timestamp.Sub(*summary.StartTime).Seconds()
		relativeSeconds = &value
	}
	var elevation *float64
	switch {
	case point.ElevationM != nil && next != nil && next.ElevationM != nil:
		value := *point.ElevationM + (*next.ElevationM-*point.ElevationM)*nearest.Fraction
		elevation = &value
	case point.ElevationM != nil:
		value := *point.ElevationM
		elevation = &value
	}
	var speed *float64
	if point.SpeedMPS != nil {
		value := *point.SpeedMPS
		speed = &value
	} else if next != nil && !point.RecordedAt.IsZero() && !next.RecordedAt.IsZero() {
		seconds := next.RecordedAt.Sub(point.RecordedAt).Seconds()
		if seconds > 0 {
			value := haversineMeters(point.Lat, point.Lon, next.Lat, next.Lon) / seconds
			speed = &value
		}
	}
	out := map[string]any{
		"track":                  summary,
		"clicked":                map[string]float64{"lat": lat, "lon": lon},
		"nearest":                map[string]float64{"lat": nearest.Lat, "lon": nearest.Lon},
		"nearest_point":          point,
		"nearest_segment_index":  nearest.Index,
		"distance_m":             nearest.DistanceM,
		"relative_time_seconds":  relativeSeconds,
		"timestamp":              nil,
		"speed_mps":              speed,
		"elevation_m":            elevation,
		"has_timestamps":         summary.StartTime != nil && summary.EndTime != nil,
		"source_format":          summary.SourceFormat,
		"track_distance_m":       summary.DistanceM,
		"track_duration_seconds": summary.DurationSec,
	}
	if !timestamp.IsZero() {
		out["timestamp"] = timestamp
	}
	return out, true
}

func buildTrackProfile(summary catalog.TrackSummary, points []catalog.TrackPoint, metric string) map[string]any {
	type profilePoint struct {
		Index           int        `json:"index"`
		DistanceM       float64    `json:"distance_m"`
		RelativeSeconds *float64   `json:"relative_seconds,omitempty"`
		Timestamp       *time.Time `json:"timestamp,omitempty"`
		Value           *float64   `json:"value,omitempty"`
		Lat             float64    `json:"lat"`
		Lon             float64    `json:"lon"`
	}
	series := make([]profilePoint, 0, len(points))
	var cumulative float64
	var minValue *float64
	var maxValue *float64
	var previous *catalog.TrackPoint
	for i := range points {
		point := points[i]
		if previous != nil {
			cumulative += haversineMeters(previous.Lat, previous.Lon, point.Lat, point.Lon)
		}
		var value *float64
		switch metric {
		case "altitude":
			value = point.ElevationM
		case "speed":
			if point.SpeedMPS != nil {
				value = point.SpeedMPS
			} else if previous != nil && !previous.RecordedAt.IsZero() && !point.RecordedAt.IsZero() {
				seconds := point.RecordedAt.Sub(previous.RecordedAt).Seconds()
				if seconds > 0 {
					v := haversineMeters(previous.Lat, previous.Lon, point.Lat, point.Lon) / seconds
					value = &v
				}
			}
		}
		var timestamp *time.Time
		if !point.RecordedAt.IsZero() {
			t := point.RecordedAt
			timestamp = &t
		}
		var relativeSeconds *float64
		if timestamp != nil && summary.StartTime != nil {
			v := timestamp.Sub(*summary.StartTime).Seconds()
			relativeSeconds = &v
		}
		if value != nil {
			if minValue == nil || *value < *minValue {
				v := *value
				minValue = &v
			}
			if maxValue == nil || *value > *maxValue {
				v := *value
				maxValue = &v
			}
		}
		series = append(series, profilePoint{
			Index:           i,
			DistanceM:       cumulative,
			RelativeSeconds: relativeSeconds,
			Timestamp:       timestamp,
			Value:           value,
			Lat:             point.Lat,
			Lon:             point.Lon,
		})
		previous = &points[i]
	}
	unit := "m"
	if metric == "speed" {
		unit = "m/s"
	}
	return map[string]any{
		"track":          summary,
		"metric":         metric,
		"unit":           unit,
		"series":         series,
		"min":            minValue,
		"max":            maxValue,
		"has_values":     minValue != nil,
		"has_timestamps": summary.StartTime != nil && summary.EndTime != nil,
	}
}

func nearestPointOnTrack(points []catalog.TrackPoint, lat, lon float64) (nearestTrackResult, bool) {
	if len(points) == 0 {
		return nearestTrackResult{}, false
	}
	if len(points) == 1 {
		return nearestTrackResult{
			Lat:       points[0].Lat,
			Lon:       points[0].Lon,
			DistanceM: haversineMeters(lat, lon, points[0].Lat, points[0].Lon),
			Index:     0,
			NextIndex: -1,
		}, true
	}
	best := nearestTrackResult{DistanceM: math.MaxFloat64, Index: 0, NextIndex: 1}
	for i := 0; i < len(points)-1; i++ {
		candidate := nearestOnSegment(lat, lon, points[i], points[i+1])
		candidate.Index = i
		candidate.NextIndex = i + 1
		if candidate.DistanceM < best.DistanceM {
			best = candidate
		}
	}
	return best, true
}

func nearestOnSegment(lat, lon float64, a, b catalog.TrackPoint) nearestTrackResult {
	lat0 := (lat + a.Lat + b.Lat) / 3
	ax, ay := lonLatMeters(a.Lat, a.Lon, lat0)
	bx, by := lonLatMeters(b.Lat, b.Lon, lat0)
	px, py := lonLatMeters(lat, lon, lat0)
	dx := bx - ax
	dy := by - ay
	fraction := 0.0
	denom := dx*dx + dy*dy
	if denom > 0 {
		fraction = ((px-ax)*dx + (py-ay)*dy) / denom
	}
	fraction = maxFloat(0, minFloat(1, fraction))
	nearestX := ax + dx*fraction
	nearestY := ay + dy*fraction
	nearestLat, nearestLon := metersLonLat(nearestX, nearestY, lat0)
	return nearestTrackResult{
		Lat:       nearestLat,
		Lon:       nearestLon,
		DistanceM: math.Hypot(px-nearestX, py-nearestY),
		Fraction:  fraction,
	}
}

func lonLatMeters(lat, lon, refLat float64) (float64, float64) {
	const metersPerDegreeLat = 111320.0
	x := lon * metersPerDegreeLat * math.Cos(refLat*math.Pi/180)
	y := lat * metersPerDegreeLat
	return x, y
}

func metersLonLat(x, y, refLat float64) (float64, float64) {
	const metersPerDegreeLat = 111320.0
	lat := y / metersPerDegreeLat
	cos := math.Cos(refLat * math.Pi / 180)
	if math.Abs(cos) < 1e-9 {
		return lat, 0
	}
	lon := x / (metersPerDegreeLat * cos)
	return lat, lon
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusM = 6371008.8
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dPhi := (lat2 - lat1) * math.Pi / 180
	dLambda := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(a))
}
