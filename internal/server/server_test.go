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
	createToken := httptest.NewRequest(http.MethodPost, "/api/v1/auth/tokens", strings.NewReader(`{"name":"test","scopes":["jobs:write"]}`))
	createToken.AddCookie(cookies[0])
	srv.ServeHTTP(rec, createToken)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"secret"`) {
		t.Fatalf("token status %d body %s", rec.Code, rec.Body.String())
	}
}
