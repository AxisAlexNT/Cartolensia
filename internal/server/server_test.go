package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"assets":4`) || !strings.Contains(rec.Body.String(), `"hashed":4`) {
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
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"not_configured"`) {
		t.Fatalf("ai classify status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/transcoding/presets", strings.NewReader(`{"id":"original","name":"bad","hardware":"cpu","codec":"h264","ffmpeg_encoder":"libx264","mode":"quality","parameter_value":"28","container":"hls"}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "built-in") {
		t.Fatalf("preset collision status %d body %s", rec.Code, rec.Body.String())
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
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: t.TempDir(), Mode: "strict_read_only"}})
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
		Store:         catalog.NewMemoryStore(),
		StoreBackend:  "memory",
		Authenticator: authService,
		Authorizer:    authService,
		AuthService:   authService,
	})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated me, got %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@example.local","password":"password"}`))
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
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "runtime_settings") || !strings.Contains(rec.Body.String(), "restart_required") {
		t.Fatalf("settings status %d body %s", rec.Code, rec.Body.String())
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
