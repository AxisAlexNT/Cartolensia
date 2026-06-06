package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	srv := New(Dependencies{
		Version:      "test",
		Config:       config.Defaults(),
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
