package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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
	srv := New(Dependencies{
		Version:      "test",
		Config:       config.Defaults(),
		Plugins:      plugins.BuiltIns(),
		Registry:     registry,
		Store:        catalog.NewMemoryStore(),
		StoreBackend: "memory",
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
}
