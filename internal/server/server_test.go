package server

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/auth"
	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/config"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/plugins"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

func TestDiscoveryHashAndMediaEndpoints(t *testing.T) {
	root, err := filepath.Abs("../../testdata/media_fixture")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	cfg := config.Defaults()
	cfg.Cache.Dir = t.TempDir()
	srv := New(Dependencies{
		Version:      "test",
		Config:       cfg,
		Plugins:      plugins.BuiltIns(),
		Registry:     registry,
		Store:        store,
		StoreBackend: "memory",
		SyncJobs:     true,
	})
	post := httptest.NewRequest(http.MethodPost, "/api/v1/discovery/start", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, post)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("discovery status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/hash/start", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("hash status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/metadata/enrich/start", strings.NewReader(`{"include_video":true,"include_images":true,"include_tracks":true}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("metadata status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"assets":5`) || !strings.Contains(rec.Body.String(), `"hashed":5`) || !strings.Contains(rec.Body.String(), `"documents":1`) {
		t.Fatalf("stats status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/explorer", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "photo_001.jpg") {
		t.Fatalf("explorer status %d body %s", rec.Code, rec.Body.String())
	}
	var rows []struct {
		AssetID string `json:"asset_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected explorer rows")
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets?media_kind=photo&limit=1&sort=name", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("X-Total-Count") != "2" {
		t.Fatalf("filtered assets status %d total %s body %s", rec.Code, rec.Header().Get("X-Total-Count"), rec.Body.String())
	}
	var filteredAssets []catalog.Asset
	if err := json.Unmarshal(rec.Body.Bytes(), &filteredAssets); err != nil {
		t.Fatal(err)
	}
	if len(filteredAssets) != 1 || filteredAssets[0].MediaKind != "photo" {
		t.Fatalf("unexpected filtered assets: %#v", filteredAssets)
	}
	rec = httptest.NewRecorder()
	headOriginal := httptest.NewRequest(http.MethodHead, "/api/v1/media/"+filteredAssets[0].ID+"/original", nil)
	srv.ServeHTTP(rec, headOriginal)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") == "" {
		t.Fatalf("head original status %d headers %#v", rec.Code, rec.Header())
	}
	rec = httptest.NewRecorder()
	rangeOriginal := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+filteredAssets[0].ID+"/original", nil)
	rangeOriginal.Header.Set("Range", "bytes=0-3")
	srv.ServeHTTP(rec, rangeOriginal)
	if rec.Code != http.StatusPartialContent || !strings.HasPrefix(rec.Header().Get("Content-Range"), "bytes 0-3/") {
		t.Fatalf("range original status %d content-range %q body %q", rec.Code, rec.Header().Get("Content-Range"), rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/explorer?media_kind=video&limit=1", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("X-Total-Count") != "1" || !strings.Contains(rec.Body.String(), `"media_kind":"video"`) {
		t.Fatalf("filtered explorer status %d total %s body %s", rec.Code, rec.Header().Get("X-Total-Count"), rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/explorer?view=folders", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"path":"photos"`) {
		t.Fatalf("folder explorer status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+rows[0].AssetID, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"original_url"`) {
		t.Fatalf("asset detail status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/missing", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("asset 404 status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tracks", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"point_count":3`) {
		t.Fatalf("tracks status %d body %s", rec.Code, rec.Body.String())
	}
	var tracks []catalog.TrackSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &tracks); err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected one track, got %#v", tracks)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tracks/"+tracks[0].TrackAssetID, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"points"`) {
		t.Fatalf("track detail status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/gps/tracks/"+tracks[0].TrackAssetID+"/point-info?lat=40.001&lon=44.001", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"distance_m"`) {
		t.Fatalf("track point info status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/gps/tracks/"+tracks[0].TrackAssetID+"/profile?metric=altitude&max_points=100", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"metric":"altitude"`) || !strings.Contains(rec.Body.String(), `"series"`) {
		t.Fatalf("track profile status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/gps/tracks/"+tracks[0].TrackAssetID+"/nearby-assets?distance_m=1000", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"assets"`) {
		t.Fatalf("track nearby assets status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/media/"+tracks[0].TrackAssetID+"/track-preview", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"FeatureCollection"`) {
		t.Fatalf("track preview status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/media/"+tracks[0].TrackAssetID+"/track-thumbnail?width=240&height=120", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("track thumbnail status %d content-type %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=photo_001", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"explanation"`) || !strings.Contains(rec.Body.String(), "photo_001") {
		t.Fatalf("search status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/map?bbox=43.9,39.9,44.1,40.1&zoom=10", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"FeatureCollection"`) {
		t.Fatalf("map status %d body %s", rec.Code, rec.Body.String())
	}
	assets, err := store.ListAssets(httptest.NewRequest(http.MethodGet, "/", nil).Context())
	if err != nil {
		t.Fatal(err)
	}
	var videoID string
	for _, asset := range assets {
		if asset.MediaKind == "video" {
			videoID = asset.ID
			break
		}
	}
	if videoID == "" {
		t.Fatal("expected video asset")
	}
	takenAt := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	if err := store.UpdateAssetMetadata(httptest.NewRequest(http.MethodGet, "/", nil).Context(), videoID, &takenAt, map[string]any{"duration_seconds": 600}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/gps/tracks/"+tracks[0].TrackAssetID+"/assets?exclude_track_assets=true", nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"media_kind":"track"`) {
		t.Fatalf("track media status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sync/candidates?asset_id="+videoID, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tracks[0].TrackAssetID) {
		t.Fatalf("sync candidates status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	body := strings.NewReader(`{"asset_id":"` + videoID + `","track_asset_id":"` + tracks[0].TrackAssetID + `","time_offset_ms":1200}`)
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/sync/links", body))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"time_offset_ms":1200`) {
		t.Fatalf("sync link status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ai/workers", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ai-cpu"`) {
		t.Fatalf("ai workers status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ai/jobs/classify", strings.NewReader(`{"scope":"selected"}`)))
	if rec.Code != http.StatusAccepted || !(strings.Contains(rec.Body.String(), `"not_configured"`) || strings.Contains(rec.Body.String(), `"completed"`)) {
		t.Fatalf("ai classify status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/transcoding/presets", strings.NewReader(`{"id":"original","name":"bad","hardware":"cpu","codec":"h264","ffmpeg_encoder":"libx264","mode":"quality","parameter_value":"28","container":"hls"}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "built-in") {
		t.Fatalf("preset collision status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestMapTrackOverlayPagesBeyondFirstTrackPage(t *testing.T) {
	ctx := context.Background()
	store := catalog.NewMemoryStore()
	cfg := config.Defaults()
	cfg.Cache.Dir = t.TempDir()
	srv := New(Dependencies{
		Version:      "test",
		Config:       cfg,
		Plugins:      plugins.BuiltIns(),
		Store:        store,
		StoreBackend: "memory",
	})
	var lastTrackID string
	for i := 0; i < 650; i++ {
		name := fmt.Sprintf("track-%03d.gpx", i)
		result, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
			StorageName:  "fixture",
			StorageURL:   "fs://fixture/" + name,
			RelativePath: name,
			Name:         name,
			Extension:    "gpx",
			MIME:         "application/gpx+xml",
			MediaKind:    "track",
			SizeBytes:    100,
			MTime:        time.Date(2026, 1, 1, 12, 0, i%60, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		trackID := result.Asset.ID
		lastTrackID = trackID
		lat := 40.0 + float64(i)/10000
		points := []catalog.TrackPoint{
			{TrackAssetID: trackID, RecordedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), Lat: lat, Lon: 44.0, Source: "test"},
			{TrackAssetID: trackID, RecordedAt: time.Date(2026, 1, 1, 12, 1, 0, 0, time.UTC), Lat: lat + 0.001, Lon: 44.001, Source: "test"},
		}
		if err := store.UpsertTrackPoints(ctx, trackID, points); err != nil {
			t.Fatal(err)
		}
		distance := 120.0
		if err := store.UpsertGPSTrackSummary(ctx, catalog.TrackSummary{
			TrackAssetID: trackID,
			Name:         name,
			PointCount:   len(points),
			StartTime:    &points[0].RecordedAt,
			EndTime:      &points[1].RecordedAt,
			MinLat:       &points[0].Lat,
			MinLon:       &points[0].Lon,
			MaxLat:       &points[1].Lat,
			MaxLon:       &points[1].Lon,
			DistanceM:    distance,
			SourceFormat: "gpx",
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/map/tracks?limit=650&zoom=8&track_point_budget=20000", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("map tracks status %d body %s", rec.Code, rec.Body.String())
	}
	var collection struct {
		Features     []map[string]any `json:"features"`
		TrackOverlay struct {
			Returned float64 `json:"returned"`
			Matched  float64 `json:"matched"`
		} `json:"track_overlay"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &collection); err != nil {
		t.Fatal(err)
	}
	if len(collection.Features) != 650 || collection.TrackOverlay.Returned != 650 || collection.TrackOverlay.Matched != 650 {
		t.Fatalf("expected all 650 tracks, got features=%d overlay=%#v", len(collection.Features), collection.TrackOverlay)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/map?track_id="+url.QueryEscape(lastTrackID)+"&zoom=8", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("map selected track status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), lastTrackID) || !strings.Contains(rec.Body.String(), `"returned":1`) {
		t.Fatalf("selected last-page track was not rendered: %s", rec.Body.String())
	}
}

func TestMapHeatmapIncludesAssetAndTrackPoints(t *testing.T) {
	ctx := context.Background()
	store := catalog.NewMemoryStore()
	cfg := config.Defaults()
	cfg.Cache.Dir = t.TempDir()
	srv := New(Dependencies{
		Version:      "test",
		Config:       cfg,
		Plugins:      plugins.BuiltIns(),
		Store:        store,
		StoreBackend: "memory",
	})
	photo, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/photo.jpg",
		RelativePath: "photo.jpg",
		Name:         "photo.jpg",
		Extension:    "jpg",
		MIME:         "image/jpeg",
		MediaKind:    "photo",
		SizeBytes:    10,
		MTime:        time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAssetGeo(ctx, catalog.AssetGeo{AssetID: photo.Asset.ID, Lat: 40.18, Lon: 44.51, Source: "exif"}, true); err != nil {
		t.Fatal(err)
	}
	track, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/track.gpx",
		RelativePath: "track.gpx",
		Name:         "track.gpx",
		Extension:    "gpx",
		MIME:         "application/gpx+xml",
		MediaKind:    "track",
		SizeBytes:    10,
		MTime:        time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	points := []catalog.TrackPoint{
		{TrackAssetID: track.Asset.ID, RecordedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), Lat: 40.10, Lon: 44.10, Source: "test"},
		{TrackAssetID: track.Asset.ID, RecordedAt: time.Date(2026, 1, 1, 12, 1, 0, 0, time.UTC), Lat: 40.11, Lon: 44.11, Source: "test"},
		{TrackAssetID: track.Asset.ID, RecordedAt: time.Date(2026, 1, 1, 12, 2, 0, 0, time.UTC), Lat: 40.12, Lon: 44.12, Source: "test"},
	}
	if err := store.UpsertTrackPoints(ctx, track.Asset.ID, points); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertGPSTrackSummary(ctx, catalog.TrackSummary{
		TrackAssetID: track.Asset.ID,
		Name:         "track.gpx",
		PointCount:   len(points),
		StartTime:    &points[0].RecordedAt,
		EndTime:      &points[2].RecordedAt,
		MinLat:       &points[0].Lat,
		MinLon:       &points[0].Lon,
		MaxLat:       &points[2].Lat,
		MaxLon:       &points[2].Lon,
		DistanceM:    1000,
		SourceFormat: "gpx",
	}, nil); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/map/heatmap?zoom=12&track_limit=10&track_point_budget=100&asset_limit=10", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("heatmap status %d body %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Features []map[string]any `json:"features"`
		Heatmap  struct {
			AssetPoints float64 `json:"asset_points"`
			TrackPoints float64 `json:"track_points"`
			TotalPoints float64 `json:"total_points"`
		} `json:"heatmap"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Heatmap.AssetPoints != 1 || payload.Heatmap.TrackPoints < 3 || payload.Heatmap.TotalPoints != float64(len(payload.Features)) {
		t.Fatalf("unexpected heatmap counts: %#v features=%d", payload.Heatmap, len(payload.Features))
	}
}

func TestTrackJumpFilterSplitsTeleportSegments(t *testing.T) {
	points := []catalog.TrackPoint{
		{Lat: 40.000, Lon: 44.000},
		{Lat: 40.001, Lon: 44.001},
		{Lat: 42.000, Lon: 46.000},
		{Lat: 40.002, Lon: 44.002},
		{Lat: 40.003, Lon: 44.003},
	}
	geometry, segmentCount, hidden := trackGeometryFromPoints(points, true, 10000)
	if hidden != 2 {
		t.Fatalf("expected two hidden jumps, got %d", hidden)
	}
	if segmentCount != 2 {
		t.Fatalf("expected two drawable segments, got %d", segmentCount)
	}
	if geometry["type"] != "MultiLineString" {
		t.Fatalf("expected multiline geometry, got %#v", geometry)
	}
	coordinates, ok := geometry["coordinates"].([][][]float64)
	if !ok || len(coordinates) != 2 || len(coordinates[0]) != 2 || len(coordinates[1]) != 2 {
		t.Fatalf("expected two 2-point line segments, got %#v", geometry["coordinates"])
	}
}

func TestComponentManagerAPIsAndSafeOperatorInputs(t *testing.T) {
	store := catalog.NewMemoryStore()
	cfg := config.Defaults()
	cfg.Cache.Dir = t.TempDir()
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: t.TempDir(), Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	srv := New(Dependencies{Version: "test", Config: cfg, Plugins: plugins.BuiltIns(), Registry: registry, Store: store, StoreBackend: "memory"})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/components", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ffmpeg"`) || !strings.Contains(rec.Body.String(), `"tesseract"`) {
		t.Fatalf("components list status %d body %s", rec.Code, rec.Body.String())
	}

	toolDir := t.TempDir()
	toolPath := filepath.Join(toolDir, "ffmpeg")
	if err := os.WriteFile(toolPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/components/ffmpeg/provide-path", strings.NewReader(`{"path":"`+filepath.ToSlash(toolPath)+`"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"user_provided"`) || !strings.Contains(rec.Body.String(), toolPath) {
		t.Fatalf("provide path status %d body %s", rec.Code, rec.Body.String())
	}

	archivePath := filepath.Join(t.TempDir(), "bad.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(archive)
	entry, err := zw.Create("../escape/ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/components/ffmpeg/provide-archive", strings.NewReader(`{"path":"`+filepath.ToSlash(archivePath)+`"}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "archive traversal rejected") {
		t.Fatalf("traversal archive status %d body %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/components/ffmpeg/check", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"job_id"`) {
		t.Fatalf("component check status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/components/ffmpeg/events", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"events"`) {
		t.Fatalf("component events status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestTranscribeAIPersistenceAndTranscriptAPIs(t *testing.T) {
	ctx := context.Background()
	store := catalog.NewMemoryStore()
	cfg := config.Defaults()
	cfg.Cache.Dir = t.TempDir()
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: t.TempDir(), Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	srv := New(Dependencies{Version: "test", Config: cfg, Plugins: plugins.BuiltIns(), Registry: registry, Store: store, StoreBackend: "memory"})
	now := time.Now().UTC()
	audio, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/audio/sample.wav",
		RelativePath: "audio/sample.wav",
		Name:         "sample.wav",
		Extension:    ".wav",
		MIME:         "audio/wav",
		MediaKind:    "audio",
		SizeBytes:    128,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, unsafe := srv.persistAIResponse(ctx, "ai-local", "transcribe_audio", audio.Asset.ID, aiInferencePayload{
		Status:   "ok",
		Endpoint: "transcribe-audio",
		Metadata: map[string]any{
			"engine":   "faster-whisper",
			"provider": "faster-whisper",
			"model":    "small",
			"language": "en",
			"text":     "Hello laboratory.\nStation announcement.",
			"segments": []any{
				map[string]any{"start_ms": float64(0), "end_ms": float64(900), "text": "Hello laboratory.", "confidence": 0.94},
				map[string]any{"start_ms": float64(900), "end_ms": float64(1700), "text": "Station announcement.", "confidence": 0.91},
			},
		},
	}, aiJobRequest{})
	if unsafe || stored != 3 {
		t.Fatalf("stored=%d unsafe=%v", stored, unsafe)
	}
	transcripts, err := store.ListTranscripts(ctx, audio.Asset.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(transcripts) != 1 || transcripts[0].SourceKind != "audio" || len(transcripts[0].Segments) != 2 {
		t.Fatalf("unexpected transcripts %#v", transcripts)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/transcripts?limit=10", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Hello laboratory") {
		t.Fatalf("transcript list status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/transcripts/"+transcripts[0].ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete transcript status %d body %s", rec.Code, rec.Body.String())
	}
	transcripts, _ = store.ListTranscripts(ctx, audio.Asset.ID, 10)
	if len(transcripts) != 0 {
		t.Fatalf("transcript was not deleted: %#v", transcripts)
	}
}

func TestAudioAnalyzeAIPersistence(t *testing.T) {
	ctx := context.Background()
	store := catalog.NewMemoryStore()
	cfg := config.Defaults()
	cfg.Cache.Dir = t.TempDir()
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: t.TempDir(), Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	srv := New(Dependencies{Version: "test", Config: cfg, Plugins: plugins.BuiltIns(), Registry: registry, Store: store, StoreBackend: "memory"})
	now := time.Now().UTC()
	audio, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/audio/features.wav",
		RelativePath: "audio/features.wav",
		Name:         "features.wav",
		Extension:    ".wav",
		MIME:         "audio/wav",
		MediaKind:    "audio",
		SizeBytes:    128,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, unsafe := srv.persistAIResponse(ctx, "ai-local", "analyze_audio", audio.Asset.ID, aiInferencePayload{
		Status:   "ok",
		Endpoint: "analyze-audio",
		Metadata: map[string]any{
			"model":              "librosa_audio_features",
			"duration_seconds":   12.5,
			"tempo_bpm":          121.5,
			"key":                "A",
			"mode":               "minor",
			"loudness":           -18.2,
			"speech_music_ratio": 0.32,
			"genre_labels":       []any{"music-like", "mid-tempo"},
			"genre_status":       "heuristic_labels_model_missing",
		},
	}, aiJobRequest{})
	if unsafe || stored != 1 {
		t.Fatalf("stored=%d unsafe=%v", stored, unsafe)
	}
	features, err := store.GetAudioFeatures(ctx, audio.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if features.TempoBPM == nil || *features.TempoBPM < 121 || features.Key != "A" || features.Mode != "minor" || len(features.GenreLabels) != 2 {
		t.Fatalf("unexpected audio features %#v", features)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+audio.Asset.ID+"/audio-features", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"music-like"`) {
		t.Fatalf("asset audio features status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestFaceClusterGeoAlignAndVideoTrackWorkflows(t *testing.T) {
	ctx := context.Background()
	store := catalog.NewMemoryStore()
	cfg := config.Defaults()
	cfg.Cache.Dir = t.TempDir()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "photos"), 0o755); err != nil {
		t.Fatal(err)
	}
	photoFile, err := os.Create(filepath.Join(root, "photos", "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 80, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{R: uint8(120 + x), G: uint8(80 + y), B: 180, A: 255})
		}
	}
	if err := jpeg.Encode(photoFile, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := photoFile.Close(); err != nil {
		t.Fatal(err)
	}
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	srv := New(Dependencies{Version: "test", Config: cfg, Plugins: plugins.BuiltIns(), Registry: registry, Store: store, StoreBackend: "memory"})
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	photo, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/photos/photo.jpg",
		RelativePath: "photos/photo.jpg",
		Name:         "photo.jpg",
		Extension:    ".jpg",
		MIME:         "image/jpeg",
		MediaKind:    "photo",
		SizeBytes:    12,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	video, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/videos/video.mp4",
		RelativePath: "videos/video.mp4",
		Name:         "video.mp4",
		Extension:    ".mp4",
		MIME:         "video/mp4",
		MediaKind:    "video",
		SizeBytes:    24,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	audio, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/audio/clip.wav",
		RelativePath: "audio/clip.wav",
		Name:         "clip.wav",
		Extension:    ".wav",
		MIME:         "audio/wav",
		MediaKind:    "audio",
		SizeBytes:    48,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/docs/invoice.pdf",
		RelativePath: "docs/invoice.pdf",
		Name:         "invoice.pdf",
		Extension:    ".pdf",
		MIME:         "application/pdf",
		MediaKind:    "document",
		SizeBytes:    64,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAssetMetadata(ctx, photo.Asset.ID, &now, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAssetMetadata(ctx, video.Asset.ID, &now, map[string]any{"duration_seconds": 7}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertTranscript(ctx, catalog.Transcript{
		AssetID:    audio.Asset.ID,
		SourceKind: "audio",
		Language:   "eng",
		Model:      "fixture-asr",
		FullText:   "Station announcement for the laboratory train.",
	}, []catalog.TranscriptSegment{{StartMS: 0, EndMS: 1200, Text: "Station announcement"}}); err != nil {
		t.Fatal(err)
	}
	tempo := 120.0
	if _, err := store.UpsertAudioFeatures(ctx, catalog.AudioFeatures{
		AssetID:         audio.Asset.ID,
		DurationSeconds: floatPtr(3.5),
		TempoBPM:        &tempo,
		Key:             "Am",
		GenreLabels:     []string{"ambient", "field-recording"},
		Model:           "fixture-audio",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertVideoFrameCaption(ctx, catalog.VideoFrameCaption{
		AssetID:     video.Asset.ID,
		TimestampMS: 3000,
		Fraction:    0.5,
		Caption:     "Train platform with people",
		Model:       "fixture-frame-caption",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertDocumentText(ctx, catalog.DocumentText{
		AssetID:   doc.Asset.ID,
		PageCount: 1,
		Title:     "Fixture invoice",
		Text:      "Invoice number laboratory-123",
		Markdown:  "# Fixture invoice\n\nInvoice number laboratory-123",
		Engine:    "fixture-document",
	}); err != nil {
		t.Fatal(err)
	}
	confidence := 0.9
	face, err := store.CreateFaceDetection(ctx, catalog.FaceDetection{
		AssetID:    photo.Asset.ID,
		X:          10,
		Y:          20,
		Width:      30,
		Height:     40,
		Confidence: &confidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/faces/detections/"+face.ID+"/thumbnail", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("face thumbnail status %d content-type %s body %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/faces/clusters", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"face_count":1`) {
		t.Fatalf("clusters status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/faces/detections/"+face.ID+"/ignore", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ignored":true`) {
		t.Fatalf("ignore status %d body %s", rec.Code, rec.Body.String())
	}

	if _, err := store.UpsertAssetGeo(ctx, catalog.AssetGeo{AssetID: photo.Asset.ID, Lat: 40.18, Lon: 44.51, Source: "exif"}, true); err != nil {
		t.Fatal(err)
	}
	ocrConfidence := 0.82
	ocrPrediction, err := store.CreateAIPrediction(ctx, catalog.AIPrediction{
		AssetID:    photo.Asset.ID,
		Task:       "ocr_image",
		Label:      "Laboratory sample label",
		Confidence: &ocrConfidence,
		ModelName:  "tesseract-test",
		Metadata: map[string]any{
			"language": "eng",
			"engine":   "tesseract",
			"x":        10,
			"y":        12,
			"width":    140,
			"height":   24,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=Yerevan", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"local place bbox"`) || !strings.Contains(rec.Body.String(), `"places"`) || !strings.Contains(rec.Body.String(), `"backend":"postgres_local"`) {
		t.Fatalf("Yerevan search status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=laboratory", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"AI prediction/caption/class"`) {
		t.Fatalf("OCR search status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=ext:mp4&limit=1", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"extension"`) || !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("extension search status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=ocr:laboratory", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"matched OCR text"`) {
		t.Fatalf("OCR prefix search status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=transcript:station", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"matched transcript"`) {
		t.Fatalf("transcript search status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=genre:ambient", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"matched audio features"`) {
		t.Fatalf("audio feature search status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=tempo:100..140", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"matched audio features"`) {
		t.Fatalf("audio tempo range search status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=caption:train", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"matched video frame caption"`) {
		t.Fatalf("frame caption search status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=document:invoice", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"matched document text"`) {
		t.Fatalf("document search status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search/places", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cache_only"`) || !strings.Contains(rec.Body.String(), `"Yerevan"`) || !strings.Contains(rec.Body.String(), `"Vanadzor"`) {
		t.Fatalf("place cache status %d body %s", rec.Code, rec.Body.String())
	}
	placePayload := `{"name":"Fixture Lab","display_name":"Fixture Lab, Armenia","aliases":["fixture road"],"provider":"local","country":"Armenia","city":"Yerevan","road":"Fixture Road","lat":40.18,"lon":44.51,"bbox":{"min_lon":44.50,"min_lat":40.17,"max_lon":44.52,"max_lat":40.19},"source":"test"}`
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/places", strings.NewReader(placePayload)))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"Fixture Lab"`) {
		t.Fatalf("create place status %d body %s", rec.Code, rec.Body.String())
	}
	var createdPlace catalog.PlaceCacheEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &createdPlace); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/places?q=fixture", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Fixture Lab"`) || !strings.Contains(rec.Body.String(), `"Fixture Road"`) {
		t.Fatalf("list places status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/places/hierarchy?q=fixture", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"local_place_cache"`) || !strings.Contains(rec.Body.String(), `"Fixture Lab"`) || !strings.Contains(rec.Body.String(), `"tree"`) {
		t.Fatalf("place hierarchy status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/places/"+createdPlace.ID, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Fixture Lab"`) || !strings.Contains(rec.Body.String(), `"stats"`) || !strings.Contains(rec.Body.String(), `"search_queries"`) {
		t.Fatalf("place detail status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/places/reverse-geocode/start", strings.NewReader(`{"limit":10,"batch_size":5,"online":false}`)))
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"kind":"reverse_geocode"`) || !strings.Contains(rec.Body.String(), `"online":false`) {
		t.Fatalf("reverse geocode start status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/places/"+createdPlace.ID, strings.NewReader(`{"display_name":"Fixture Lab Cache"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Fixture Lab Cache"`) {
		t.Fatalf("patch place status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/places/"+createdPlace.ID, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deleted"`) {
		t.Fatalf("delete place status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+photo.Asset.ID, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"places"`) || !strings.Contains(rec.Body.String(), `"Yerevan, Armenia"`) {
		t.Fatalf("asset places status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+photo.Asset.ID+"/ocr", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Laboratory sample label"`) || !strings.Contains(rec.Body.String(), `"full_text"`) || !strings.Contains(rec.Body.String(), `"blocks"`) {
		t.Fatalf("asset OCR status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+audio.Asset.ID+"/transcripts", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Station announcement"`) {
		t.Fatalf("asset transcripts status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+audio.Asset.ID+"/audio-features", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ambient"`) {
		t.Fatalf("asset audio features status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+video.Asset.ID+"/frame-captions", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `Train platform`) {
		t.Fatalf("asset frame captions status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+doc.Asset.ID+"/document", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Fixture invoice"`) {
		t.Fatalf("asset document status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+photo.Asset.ID+"/ai", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ocr_blocks"`) || !strings.Contains(rec.Body.String(), `"faces"`) || !strings.Contains(rec.Body.String(), `"generated_truth"`) {
		t.Fatalf("asset AI aggregate status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+photo.Asset.ID+"/faces", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("asset faces status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+photo.Asset.ID+"/captions", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"captions"`) {
		t.Fatalf("asset captions status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+photo.Asset.ID+"/classification", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"predictions"`) {
		t.Fatalf("asset classification status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+photo.Asset.ID+"/safety", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"safety"`) {
		t.Fatalf("asset safety status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/assets/"+photo.Asset.ID+"/ocr/"+ocrPrediction.ID, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deleted"`) {
		t.Fatalf("delete OCR status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+photo.Asset.ID+"/ocr", nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"Laboratory sample label"`) {
		t.Fatalf("asset OCR after delete status %d body %s", rec.Code, rec.Body.String())
	}
	ocrPrediction, err = store.CreateAIPrediction(ctx, catalog.AIPrediction{
		AssetID:    photo.Asset.ID,
		Task:       "ocr_image",
		Label:      "Laboratory sample label",
		Confidence: &ocrConfidence,
		ModelName:  "tesseract-test",
		Metadata: map[string]any{
			"language": "eng",
			"engine":   "tesseract",
			"x":        10,
			"y":        12,
			"width":    140,
			"height":   24,
		},
	})
	if err != nil || ocrPrediction.ID == "" {
		t.Fatalf("recreate OCR prediction: %v", err)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ocr/runs", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored_blocks":1`) || !strings.Contains(rec.Body.String(), `"eng"`) {
		t.Fatalf("OCR runs status %d body %s", rec.Code, rec.Body.String())
	}
	loriPhoto, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/photos/vanadzor.jpg",
		RelativePath: "photos/vanadzor.jpg",
		Name:         "vanadzor.jpg",
		Extension:    ".jpg",
		MIME:         "image/jpeg",
		MediaKind:    "photo",
		SizeBytes:    13,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAssetGeo(ctx, catalog.AssetGeo{AssetID: loriPhoto.Asset.ID, Lat: 40.8128, Lon: 44.4883, Source: "exif"}, true); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"Vanadzor", "Lori province"} {
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q="+url.QueryEscape(query), nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"local place bbox"`) || !strings.Contains(rec.Body.String(), `"vanadzor.jpg"`) {
			t.Fatalf("%s search status %d body %s", query, rec.Code, rec.Body.String())
		}
	}
	trackAsset, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/tracks/test.gpx",
		RelativePath: "tracks/test.gpx",
		Name:         "test.gpx",
		Extension:    ".gpx",
		MIME:         "application/gpx+xml",
		MediaKind:    "track",
		SizeBytes:    32,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	trackID := trackAsset.Asset.ID
	points := []catalog.TrackPoint{
		{TrackAssetID: trackID, RecordedAt: now.Add(-time.Second), Lat: 40.17, Lon: 44.50, Source: "test"},
		{TrackAssetID: trackID, RecordedAt: now.Add(time.Second), Lat: 40.19, Lon: 44.52, Source: "test"},
	}
	if err := store.UpsertTrackPoints(ctx, trackID, points); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertGPSTrackSummary(ctx, catalog.TrackSummary{TrackAssetID: trackID, Name: "test track", PointCount: len(points), SourceFormat: "gpx"}, nil); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/geo-align/session", strings.NewReader(`{"limit":10,"track_ids":["`+trackID+`"]}`)))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"markers"`) {
		t.Fatalf("geo session status %d body %s", rec.Code, rec.Body.String())
	}
	var geoSession geoAlignSession
	if err := json.Unmarshal(rec.Body.Bytes(), &geoSession); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/geo-align/sessions/"+geoSession.ID+"/marker/"+photo.Asset.ID, strings.NewReader(`{"lat":40.3,"lon":44.3}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"modified":true`) {
		t.Fatalf("geo marker status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/geo-align/sessions/"+geoSession.ID+"/marker/"+photo.Asset.ID, strings.NewReader(`{"reset":true}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"modified":false`) || !strings.Contains(rec.Body.String(), `"staged_lon":44.51`) {
		t.Fatalf("geo marker reset status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/geo-align/sessions/"+geoSession.ID+"/marker/"+photo.Asset.ID, strings.NewReader(`{"lat":40.3,"lon":44.3}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"modified":true`) {
		t.Fatalf("geo marker second move status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/geo-align/sessions/"+geoSession.ID+"/apply", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"updated":1`) {
		t.Fatalf("geo apply status %d body %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/video-track-player/session", strings.NewReader(`{"video_asset_id":"`+video.Asset.ID+`","track_ids":["`+trackID+`"]}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("video session status %d body %s", rec.Code, rec.Body.String())
	}
	var videoSession videoTrackPlayerSession
	if err := json.Unmarshal(rec.Body.Bytes(), &videoSession); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/video-track-player/sessions/"+videoSession.ID+"/position?time_ms=500", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"positions"`) {
		t.Fatalf("video position status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestScreenDistanceClusteringSplitsWithZoom(t *testing.T) {
	features := []map[string]any{
		testPointFeature("a", 44.10000, 40.10000),
		testPointFeature("b", 44.10005, 40.10005),
		testPointFeature("c", 44.30000, 40.30000),
	}
	low := clusterGeoJSONPoints(features, bbox{}, false, 10, 48)
	if len(low) >= len(features) {
		t.Fatalf("expected low zoom to cluster nearby points, got %d features", len(low))
	}
	high := clusterGeoJSONPoints(features, bbox{}, false, 21, 48)
	if len(high) != len(features) {
		t.Fatalf("expected high zoom to split isolated nearby points, got %#v", high)
	}
	same := []map[string]any{testPointFeature("a", 44.1, 40.1), testPointFeature("b", 44.1, 40.1)}
	clusteredSame := clusterGeoJSONPoints(same, bbox{}, false, 22, 48)
	if len(clusteredSame) != 1 {
		t.Fatalf("same coordinate points should remain one cluster, got %#v", clusteredSame)
	}
	props, _ := clusteredSame[0]["properties"].(map[string]any)
	if props["count"] != 2 {
		t.Fatalf("expected cluster count 2, got %#v", props)
	}
}

func TestVideoTrackPositionInterpolatesSpeedAndElevation(t *testing.T) {
	ctx := context.Background()
	store := catalog.NewMemoryStore()
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: t.TempDir(), Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Auth.Mode = "local"
	cfg.Cache.Dir = t.TempDir()
	srv := New(Dependencies{
		Version:      "test",
		Config:       cfg,
		Plugins:      plugins.BuiltIns(),
		Registry:     registry,
		Store:        store,
		StoreBackend: "memory",
	})
	videoFile, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/videos/PXL_20260512_072546131.mp4",
		RelativePath: "videos/PXL_20260512_072546131.mp4",
		Name:         "PXL_20260512_072546131.mp4",
		Extension:    "mp4",
		MIME:         "video/mp4",
		MediaKind:    "video",
		SizeBytes:    1024,
		MTime:        time.Date(2026, 5, 12, 7, 25, 46, 0, time.FixedZone("AMT", 4*60*60)),
	})
	if err != nil {
		t.Fatal(err)
	}
	videoTaken := time.Date(2026, 5, 12, 7, 25, 46, 0, time.FixedZone("AMT", 4*60*60))
	if err := store.UpdateAssetMetadata(ctx, videoFile.Asset.ID, &videoTaken, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	trackFile, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/tracks/20260512-072610.gpx",
		RelativePath: "tracks/20260512-072610.gpx",
		Name:         "20260512-072610.gpx",
		Extension:    "gpx",
		MIME:         "application/gpx+xml",
		MediaKind:    "track",
		SizeBytes:    1024,
		MTime:        videoTaken,
	})
	if err != nil {
		t.Fatal(err)
	}
	speedA := 1.5
	speedB := 2.5
	elevA := 10.0
	elevB := 20.0
	if err := store.UpsertTrackPoints(ctx, trackFile.Asset.ID, []catalog.TrackPoint{
		{RecordedAt: videoTaken, Lat: 61.0, Lon: 30.0, SpeedMPS: &speedA, ElevationM: &elevA},
		{RecordedAt: videoTaken.Add(10 * time.Second), Lat: 61.1, Lon: 30.1, SpeedMPS: &speedB, ElevationM: &elevB},
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/video-track-player/session", strings.NewReader(`{"video_asset_id":"`+videoFile.Asset.ID+`","track_ids":["`+trackFile.Asset.ID+`"]}`))
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("session status %d body %s", rec.Code, rec.Body.String())
	}
	var session videoTrackPlayerSession
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/video-track-player/sessions/"+session.ID+"/position?time_ms=5000", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("position status %d body %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Positions []struct {
			Lat        float64  `json:"lat"`
			Lon        float64  `json:"lon"`
			SpeedMPS   *float64 `json:"speed_mps"`
			ElevationM *float64 `json:"elevation_m"`
			Mode       string   `json:"mode"`
			Time       string   `json:"time"`
		} `json:"positions"`
		TargetTime string `json:"target_time"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Positions) != 1 {
		t.Fatalf("expected one interpolated position, got %#v", payload.Positions)
	}
	if payload.Positions[0].SpeedMPS == nil || payload.Positions[0].ElevationM == nil {
		t.Fatalf("expected interpolated speed/elevation, got %#v", payload.Positions[0])
	}
	if payload.Positions[0].Mode == "" || payload.TargetTime == "" {
		t.Fatalf("expected moving marker metadata, got %#v", payload)
	}
}

func testPointFeature(id string, lon, lat float64) map[string]any {
	return map[string]any{
		"type": "Feature",
		"geometry": map[string]any{
			"type":        "Point",
			"coordinates": []float64{lon, lat},
		},
		"properties": map[string]any{
			"id":   id,
			"kind": "photo",
		},
	}
}

func TestLocalAuthEndpoints(t *testing.T) {
	authStore := auth.NewMemoryStore()
	authService := auth.NewLocalService(authStore, auth.Config{
		AdminEmail:       "admin@example.local",
		AdminDisplayName: "Admin",
		SessionTTL:       time.Hour,
		APITokenTTL:      time.Hour,
		CookieName:       "test_session",
	})
	if _, _, err := authService.Bootstrap(context.Background(), "password"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "public.txt"), []byte("public fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	publicAsset, err := store.UpsertDiscoveredFile(context.Background(), storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/public.txt",
		RelativePath: "public.txt",
		Name:         "public.txt",
		Extension:    "txt",
		MIME:         "text/plain",
		MediaKind:    "document",
		SizeBytes:    int64(len("public fixture")),
		MTime:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Auth.Mode = "local"
	srv := New(Dependencies{
		Version:       "test",
		Config:        cfg,
		Plugins:       plugins.BuiltIns(),
		Registry:      registry,
		Store:         store,
		StoreBackend:  "memory",
		Authenticator: authService,
		Authorizer:    authService,
		AuthService:   authService,
	})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"principal":null`) {
		t.Fatalf("expected public auth status with null principal, got %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected protected stats without login, got %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{\"email\":\" admin@example.local \",\"password\":\"password\\n\"}"))
	srv.ServeHTTP(rec, login)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status %d body %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}
	rec = httptest.NewRecorder()
	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.AddCookie(cookies[0])
	srv.ServeHTTP(rec, me)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "admin@example.local") {
		t.Fatalf("me status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	csrfReq.AddCookie(cookies[0])
	srv.ServeHTTP(rec, csrfReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("csrf status %d body %s", rec.Code, rec.Body.String())
	}
	var csrf struct {
		Header string `json:"header"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &csrf); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	privateOriginal := httptest.NewRequest(http.MethodHead, "/api/v1/media/"+publicAsset.Asset.ID+"/original", nil)
	srv.ServeHTTP(rec, privateOriginal)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected private media to require auth, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	publicToggle := httptest.NewRequest(http.MethodPatch, "/api/v1/assets/"+publicAsset.Asset.ID+"/visibility", strings.NewReader(`{"public":true}`))
	publicToggle.AddCookie(cookies[0])
	publicToggle.Header.Set(csrf.Header, csrf.Token)
	srv.ServeHTTP(rec, publicToggle)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"public":true`) {
		t.Fatalf("public toggle status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/public/assets?limit=10", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), publicAsset.Asset.ID) {
		t.Fatalf("public list status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/public/assets/"+publicAsset.Asset.ID, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"original_url"`) {
		t.Fatalf("public detail status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	publicOriginal := httptest.NewRequest(http.MethodHead, "/api/v1/media/"+publicAsset.Asset.ID+"/original", nil)
	srv.ServeHTTP(rec, publicOriginal)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("public original status %d content-type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	rec = httptest.NewRecorder()
	privateToggle := httptest.NewRequest(http.MethodPatch, "/api/v1/assets/"+publicAsset.Asset.ID+"/visibility", strings.NewReader(`{"public":false}`))
	privateToggle.AddCookie(cookies[0])
	privateToggle.Header.Set(csrf.Header, csrf.Token)
	srv.ServeHTTP(rec, privateToggle)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"public":false`) {
		t.Fatalf("private toggle status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/api/v1/media/"+publicAsset.Asset.ID+"/original", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unmarked media to require auth again, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	protectedNoCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/discovery/start", nil)
	protectedNoCSRF.AddCookie(cookies[0])
	srv.ServeHTTP(rec, protectedNoCSRF)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected csrf failure, got %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	protectedWithCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/discovery/start", nil)
	protectedWithCSRF.AddCookie(cookies[0])
	protectedWithCSRF.Header.Set(csrf.Header, csrf.Token)
	srv.ServeHTTP(rec, protectedWithCSRF)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected csrf-protected discovery accepted, got %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	createToken := httptest.NewRequest(http.MethodPost, "/api/v1/auth/tokens", strings.NewReader(`{"name":"test","scopes":["jobs:write"]}`))
	createToken.AddCookie(cookies[0])
	createToken.Header.Set(csrf.Header, csrf.Token)
	srv.ServeHTTP(rec, createToken)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"secret"`) {
		t.Fatalf("token status %d body %s", rec.Code, rec.Body.String())
	}
	var tokenResult struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tokenResult); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	pluginRescan := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/rescan", nil)
	pluginRescan.Header.Set("Authorization", "Bearer "+tokenResult.Secret)
	srv.ServeHTTP(rec, pluginRescan)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected token scope denial, got %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	changePassword := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", strings.NewReader(`{"old_password":"password","new_password":"new-password"}`))
	changePassword.AddCookie(cookies[0])
	changePassword.Header.Set(csrf.Header, csrf.Token)
	srv.ServeHTTP(rec, changePassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("password status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestJobOperationsEndpoints(t *testing.T) {
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: t.TempDir(), Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	job := jobs.New("metadata_enrich", nil)
	jobs.AddLog(&job, "error", "transient failure")
	job.Status = jobs.StatusRunning
	if err := jobs.Fail(&job, jobs.Transient(errTestFailure{})); err != nil {
		t.Fatal(err)
	}
	queued, err := store.EnqueueJob(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(Dependencies{
		Version:      "test",
		Config:       config.Defaults(),
		Plugins:      plugins.BuiltIns(),
		Registry:     registry,
		Store:        store,
		StoreBackend: "memory",
	})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/stats", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"failed":1`) {
		t.Fatalf("stats status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+queued.ID, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), queued.ID) {
		t.Fatalf("detail status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+queued.ID+"/logs?limit=1", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"logs"`) {
		t.Fatalf("logs status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+queued.ID+"/retry", strings.NewReader(`{}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"queued"`) {
		t.Fatalf("retry status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestJobsActiveSortPinsRunningAndQueued(t *testing.T) {
	store := catalog.NewMemoryStore()
	oldSucceeded, err := store.EnqueueJob(context.Background(), jobs.New("old_succeeded", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Start(&oldSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Complete(&oldSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateJob(context.Background(), oldSucceeded); err != nil {
		t.Fatal(err)
	}
	running, err := store.EnqueueJob(context.Background(), jobs.New("long_discovery", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Start(&running); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateJob(context.Background(), running); err != nil {
		t.Fatal(err)
	}
	queued, err := store.EnqueueJob(context.Background(), jobs.New("queued_hash", nil))
	if err != nil {
		t.Fatal(err)
	}
	newSucceeded, err := store.EnqueueJob(context.Background(), jobs.New("new_succeeded_ai", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Start(&newSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Complete(&newSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateJob(context.Background(), newSucceeded); err != nil {
		t.Fatal(err)
	}
	srv := New(Dependencies{
		Version:      "test",
		Config:       config.Defaults(),
		Plugins:      plugins.BuiltIns(),
		Store:        store,
		StoreBackend: "memory",
	})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/jobs?sort=active&limit=3", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("jobs status %d body %s", rec.Code, rec.Body.String())
	}
	var rows []jobs.Job
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected three jobs, got %#v", rows)
	}
	if rows[0].ID != running.ID || rows[1].ID != queued.ID || rows[2].ID != newSucceeded.ID {
		t.Fatalf("active sort should pin running then queued before newest history; got %#v", []string{rows[0].Kind, rows[1].Kind, rows[2].Kind})
	}
}

type errTestFailure struct{}

func (errTestFailure) Error() string { return "boom" }

func TestBuildMonthBuckets(t *testing.T) {
	jan := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 2, 3, 0, 0, 0, time.UTC)
	assets := []catalog.Asset{
		{
			ID: "a", MediaKind: "photo", DisplayName: "a.jpg",
			Locations: []catalog.Location{{StorageName: "fixture", Extension: "jpg", MediaKind: "photo", SizeBytes: 10, MTime: jan, HashStatus: catalog.HashStatusHashed}},
		},
		{
			ID: "b", MediaKind: "video", DisplayName: "b.mp4",
			Locations: []catalog.Location{{StorageName: "fixture", Extension: "mp4", MediaKind: "video", SizeBytes: 20, MTime: feb, HashStatus: catalog.HashStatusUnhashed}},
		},
	}
	buckets := buildMonthBuckets(assets, url.Values{})
	if len(buckets) != 2 || buckets[0].Month != "2026-02" || buckets[1].Month != "2026-01" {
		t.Fatalf("unexpected buckets %#v", buckets)
	}
	filtered := buildMonthBuckets(assets, url.Values{"hash_status": []string{catalog.HashStatusHashed}})
	if len(filtered) != 1 || filtered[0].Month != "2026-01" || filtered[0].Photos != 1 {
		t.Fatalf("unexpected filtered buckets %#v", filtered)
	}
}

func TestResolveAIAssetsFiltersSupportedKindsForScopedJobs(t *testing.T) {
	ctx := context.Background()
	store := catalog.NewMemoryStore()
	cfg := config.Defaults()
	cfg.Cache.Dir = t.TempDir()
	srv := New(Dependencies{Version: "test", Config: cfg, Plugins: plugins.BuiltIns(), Store: store, StoreBackend: "memory", SyncJobs: true})
	now := time.Now().UTC()
	files := []storage.FileInfo{
		{StorageName: "fixture", StorageURL: "fs://fixture/tracks/aaa.gpx", RelativePath: "tracks/aaa.gpx", Name: "aaa.gpx", Extension: "gpx", MediaKind: "track", SizeBytes: 1, MTime: now},
		{StorageName: "fixture", StorageURL: "fs://fixture/audio/ddd.mp3", RelativePath: "audio/ddd.mp3", Name: "ddd.mp3", Extension: "mp3", MediaKind: "audio", SizeBytes: 1, MTime: now},
		{StorageName: "fixture", StorageURL: "fs://fixture/videos/eee.mp4", RelativePath: "videos/eee.mp4", Name: "eee.mp4", Extension: "mp4", MediaKind: "video", SizeBytes: 1, MTime: now},
	}
	for _, file := range files {
		if _, err := store.UpsertDiscoveredFile(ctx, file); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 610; i++ {
		name := fmt.Sprintf("photo-%03d.jpg", i)
		if _, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
			StorageName:  "fixture",
			StorageURL:   "fs://fixture/photos/" + name,
			RelativePath: "photos/" + name,
			Name:         name,
			Extension:    "jpg",
			MediaKind:    "photo",
			SizeBytes:    1,
			MTime:        now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	imageAssets, err := srv.resolveAIAssets(ctx, "ocr_image", aiJobRequest{Scope: "current_indexed", Limit: 600})
	if err != nil {
		t.Fatal(err)
	}
	if len(imageAssets) != 600 {
		t.Fatalf("expected 600 image assets, got %d", len(imageAssets))
	}
	for _, asset := range imageAssets {
		if asset.MediaKind != "photo" {
			t.Fatalf("image AI resolver returned unsupported asset: %#v", asset)
		}
	}

	avAssets, err := srv.resolveAIAssets(ctx, "transcribe_audio", aiJobRequest{Scope: "current_indexed", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(avAssets) != 2 {
		t.Fatalf("expected audio/video assets, got %#v", avAssets)
	}
	for _, asset := range avAssets {
		if asset.MediaKind != "audio" && asset.MediaKind != "video" {
			t.Fatalf("transcription resolver returned unsupported asset: %#v", asset)
		}
	}
}

func TestSettingsAndDBExportStayInCache(t *testing.T) {
	cfg := config.Defaults()
	cfg.Cache.Dir = t.TempDir()
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: t.TempDir(), Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	srv := New(Dependencies{
		Version:      "test",
		Config:       cfg,
		Plugins:      plugins.BuiltIns(),
		Registry:     registry,
		Store:        catalog.NewMemoryStore(),
		StoreBackend: "memory",
	})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "runtime_settings") || !strings.Contains(rec.Body.String(), "restart_required") || !strings.Contains(rec.Body.String(), "Search/Places") {
		t.Fatalf("settings status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings/schema", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "search.geocoder_mode") {
		t.Fatalf("settings schema status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/settings/runtime", strings.NewReader(`{"indexing.default_max_files":25,"unknown":"ignored"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "indexing.default_max_files") || strings.Contains(rec.Body.String(), "unknown") {
		t.Fatalf("runtime settings status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/storages", strings.NewReader(`{"name":"synthetic","kind":"fs","root":"`+filepath.ToSlash(t.TempDir())+`","mode":"strict_read_only"}`)))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"synthetic"`) {
		t.Fatalf("storage add status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/storages/synthetic/validate", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"valid":true`) {
		t.Fatalf("storage validate status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/storages", strings.NewReader(`{"name":"realbad","kind":"fs","root":"/mnt/Models/rclone","mode":"read_only"}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "strict_read_only") {
		t.Fatalf("real archive mode guard status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/db/export", strings.NewReader(`{}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("export status %d body %s", rec.Code, rec.Body.String())
	}
	var exported struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if err := ensurePathInside(cfg.Cache.Dir, exported.Path); err != nil {
		t.Fatalf("export escaped cache: %v", err)
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/db/import-plan", strings.NewReader(`{"path":"`+exported.ID+`","confirmation_phrase":"PLAN ONLY"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "PLAN ONLY") && !strings.Contains(rec.Body.String(), "Validated") {
		t.Fatalf("import plan status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestReverseGeocoderProvidersAndLocale(t *testing.T) {
	cfg := config.Defaults()
	cfg.Cache.Dir = t.TempDir()
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: t.TempDir(), Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	srv := New(Dependencies{
		Version:      "test",
		Config:       cfg,
		Plugins:      plugins.BuiltIns(),
		Registry:     registry,
		Store:        catalog.NewMemoryStore(),
		StoreBackend: "memory",
	})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/places/providers", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"google_secret_source"`) || strings.Contains(rec.Body.String(), "secret-key") {
		t.Fatalf("providers status %d body %s", rec.Code, rec.Body.String())
	}
	withRuntimeSettings(t, map[string]any{
		"search.online_geocoding":         true,
		"search.geocoder_provider":        "nominatim_compatible",
		"search.geocoder_provider_url":    "http://127.0.0.1:9999/geocoder",
		"search.geocoder_locale":          "ru,en",
		"search.geocoder_min_interval_ms": 0,
	})
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/places/providers", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"active_provider":"nominatim_compatible"`) || !strings.Contains(rec.Body.String(), `"locale":"ru,en"`) {
		t.Fatalf("providers with locale status %d body %s", rec.Code, rec.Body.String())
	}
	place, err := normalizeFeaturePlace("photon", "http://127.0.0.1:9999/geocoder", "ru,en", 59.93428, 30.33510, map[string]any{
		"name":    "Nevsky Prospect",
		"country": "Russia",
		"state":   "Saint Petersburg",
		"city":    "Saint Petersburg",
		"street":  "Nevsky Prospect",
		"label":   "Nevsky Prospect, Saint Petersburg, Russia",
	}, []float64{30.33, 59.93, 30.34, 59.94}, []float64{30.33510, 59.93428})
	if err != nil {
		t.Fatal(err)
	}
	if place.Provider != "photon:ru,en" || place.Road != "Nevsky Prospect" || place.Country != "Russia" {
		t.Fatalf("unexpected normalized provider place: %#v", place)
	}
}

func TestReverseGeocodeCacheSatisfiedOnlyByProviderBackedEntries(t *testing.T) {
	localSeed := catalog.PlaceCacheEntry{
		Provider: "local",
		Source:   "built_in_seed",
		Name:     "Armenia",
	}
	if reverseGeocodeCacheSatisfied([]catalog.PlaceCacheEntry{localSeed}) {
		t.Fatalf("coarse local seed should not satisfy missing reverse geocode")
	}
	nominatim := catalog.PlaceCacheEntry{
		Provider: "nominatim:ru,en",
		Source:   "online_user_triggered_cache",
		Name:     "Kalinina Street",
	}
	if !reverseGeocodeCacheSatisfied([]catalog.PlaceCacheEntry{localSeed, nominatim}) {
		t.Fatalf("provider-backed reverse geocode should satisfy missing reverse geocode")
	}
}

func withRuntimeSettings(t *testing.T, values map[string]any) {
	t.Helper()
	runtimeSettings.Lock()
	previous := map[string]any{}
	for key, value := range values {
		previous[key] = runtimeSettings.values[key]
		runtimeSettings.values[key] = value
	}
	runtimeSettings.Unlock()
	t.Cleanup(func() {
		runtimeSettings.Lock()
		defer runtimeSettings.Unlock()
		for key, value := range previous {
			runtimeSettings.values[key] = value
		}
	})
}

func TestHLSArgsProfilesAndPathSafety(t *testing.T) {
	dir := t.TempDir()
	args, err := hlsArgs("h264-low", "/tmp/source.mp4", dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "master.m3u8") || !strings.Contains(joined, "segment_%05d.ts") || !strings.Contains(joined, "-f hls") {
		t.Fatalf("unexpected hls args %q", joined)
	}
	if _, err := hlsArgs("av1-preview", "/tmp/source.mp4", dir); err == nil {
		t.Fatal("expected unsupported profile error")
	}
	if err := ensurePathInside(dir, filepath.Join(dir, "child")); err != nil {
		t.Fatal(err)
	}
	if err := ensurePathInside(dir, filepath.Join(dir, "..", "escape")); err == nil {
		t.Fatal("expected escape to be rejected")
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
