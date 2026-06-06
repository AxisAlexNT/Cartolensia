package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/auth"
	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/config"
	"github.com/AxisAlexNT/Cartolensia/internal/database"
	"github.com/AxisAlexNT/Cartolensia/internal/discovery"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/plugins"
	"github.com/AxisAlexNT/Cartolensia/internal/preview"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

type Dependencies struct {
	Version       string
	Config        config.Config
	Plugins       []plugins.Manifest
	Registry      *storage.Registry
	Store         catalog.Store
	StoreBackend  string
	Capabilities  []database.Capability
	Authenticator auth.Authenticator
	Authorizer    auth.Authorizer
	SyncJobs      bool
}

type Server struct {
	deps Dependencies
	mux  *http.ServeMux
}

func New(deps Dependencies) *Server {
	if deps.Authenticator == nil {
		deps.Authenticator = auth.DevNoAuth{}
	}
	if deps.Authorizer == nil {
		deps.Authorizer = auth.DevNoAuth{}
	}
	s := &Server{deps: deps, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/v1/health", s.handleHealth)
	s.mux.HandleFunc("/api/v1/version", s.handleVersion)
	s.mux.HandleFunc("/api/v1/config/effective", s.handleConfig)
	s.mux.HandleFunc("/api/v1/storages", s.handleStorages)
	s.mux.HandleFunc("/api/v1/plugins", s.handlePlugins)
	s.mux.HandleFunc("/api/v1/plugins/rescan", s.handlePluginsRescan)
	s.mux.HandleFunc("/api/v1/jobs", s.handleJobs)
	s.mux.HandleFunc("/api/v1/jobs/", s.handleJobByID)
	s.mux.HandleFunc("/api/v1/discovery/start", s.handleDiscoveryStart)
	s.mux.HandleFunc("/api/v1/hash/start", s.handleHashStart)
	s.mux.HandleFunc("/api/v1/assets", s.handleAssets)
	s.mux.HandleFunc("/api/v1/assets/", s.handleAssetByID)
	s.mux.HandleFunc("/api/v1/explorer", s.handleExplorer)
	s.mux.HandleFunc("/api/v1/stats", s.handleStats)
	s.mux.HandleFunc("/api/v1/backend/status", s.handleBackendStatus)
	s.mux.HandleFunc("/api/v1/media/", s.handleMedia)
	s.mux.HandleFunc("/", s.handleIndex)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": s.deps.Version})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, s.deps.Config)
}

func (s *Server) handleStorages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, s.deps.Registry.ListStorages())
}

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, s.deps.Plugins)
}

func (s *Server) handlePluginsRescan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "plugins.rescan") {
		return
	}
	manifests, err := plugins.Load("plugins", true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.deps.Plugins = manifests
	writeJSON(w, http.StatusOK, manifests)
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	jobsList, err := s.deps.Store.ListJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, jobsList)
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[1] != "cancel" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "jobs.cancel") {
		return
	}
	job, err := s.deps.Store.RequestCancelJob(r.Context(), parts[0])
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleDiscoveryStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "discovery.start") {
		return
	}
	job, err := s.deps.Store.EnqueueJob(r.Context(), jobs.New("discovery", map[string]any{"storage": "all"}))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if s.deps.SyncJobs {
		runner := discovery.Runner{Registry: s.deps.Registry, Store: s.deps.Store}
		if err := runner.Scan(r.Context(), &job); err != nil && !errors.Is(err, jobs.ErrCanceled) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleHashStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "hash.start") {
		return
	}
	job, err := s.deps.Store.EnqueueJob(r.Context(), jobs.New("hash", map[string]any{"scope": "unhashed"}))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if s.deps.SyncJobs {
		runner := discovery.Runner{Registry: s.deps.Registry, Store: s.deps.Store}
		if err := runner.HashUnhashed(r.Context(), &job); err != nil && !errors.Is(err, jobs.ErrCanceled) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	assets, err := s.deps.Store.ListAssets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	assets = catalog.SearchAssets(assets, r.URL.Query().Get("q"))
	writeJSON(w, http.StatusOK, assets)
}

func (s *Server) handleAssetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	assetID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/assets/"), "/")
	if assetID == "" || strings.Contains(assetID, "/") {
		http.NotFound(w, r)
		return
	}
	asset, err := s.deps.Store.GetAsset(r.Context(), assetID)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.assetDetail(asset))
}

func (s *Server) handleExplorer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	assets, err := s.deps.Store.ListAssets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	query := r.URL.Query()
	if query.Get("view") == "folders" || query.Get("path") != "" || query.Get("storage") != "" || query.Get("media_kind") != "" {
		limit, _ := strconv.Atoi(query.Get("limit"))
		offset, _ := strconv.Atoi(query.Get("offset"))
		view, err := catalog.BuildExplorerView(assets, catalog.ExplorerOptions{
			Storage:   query.Get("storage"),
			Path:      query.Get("path"),
			MediaKind: query.Get("media_kind"),
			Limit:     limit,
			Offset:    offset,
			Sort:      query.Get("sort"),
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
		return
	}
	type row struct {
		AssetID      string    `json:"asset_id"`
		Name         string    `json:"name"`
		MediaKind    string    `json:"media_kind"`
		StorageURL   string    `json:"storage_url"`
		RelativePath string    `json:"relative_path"`
		SizeBytes    int64     `json:"size_bytes"`
		MTime        time.Time `json:"mtime"`
		HashStatus   string    `json:"hash_status"`
		SHA512Hex    string    `json:"sha512_hex,omitempty"`
	}
	rows := make([]row, 0, len(assets))
	for _, asset := range assets {
		loc, ok := catalog.FirstLocation(asset)
		if !ok {
			continue
		}
		rows = append(rows, row{
			AssetID: asset.ID, Name: loc.FileName, MediaKind: loc.MediaKind, StorageURL: loc.StorageURL,
			RelativePath: loc.RelativePath, SizeBytes: loc.SizeBytes, MTime: loc.MTime, HashStatus: loc.HashStatus, SHA512Hex: loc.SHA512Hex,
		})
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	stats, err := s.deps.Store.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleBackendStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	stats, _ := s.deps.Store.Stats(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"store_backend": s.deps.StoreBackend,
		"storages":      s.deps.Registry.ListStorages(),
		"plugins":       len(s.deps.Plugins),
		"capabilities":  s.deps.Capabilities,
		"stats":         stats,
		"preview_cache": s.deps.Config.Cache.Dir,
		"auth_mode":     s.deps.Config.Auth.Mode,
		"workers":       s.deps.Config.Workers,
	})
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/media/") {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/media/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	assetID := parts[0]
	switch parts[1] {
	case "original":
		s.handleOriginal(w, r, assetID)
	case "preview":
		s.handlePreview(w, r, assetID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleOriginal(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w)
		return
	}
	asset, err := s.deps.Store.GetAsset(r.Context(), assetID)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	loc, ok := catalog.FirstLocation(asset)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("asset has no storage location"))
		return
	}
	file, info, err := s.deps.Registry.OpenByURL(loc.StorageURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", info.MIME)
	w.Header().Set("X-Cartolensia-Storage-URL", info.StorageURL)
	http.ServeContent(w, r, info.Name, info.MTime, file)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w)
		return
	}
	asset, err := s.deps.Store.GetAsset(r.Context(), assetID)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	info := preview.ForAsset(asset)
	body := map[string]any{
		"status":     info.Status,
		"message":    info.Message,
		"cache_key":  info.CacheKey,
		"cache_path": preview.CachePath(s.deps.Config.Cache.Dir, asset, "default"),
	}
	if info.Status == preview.StatusUnsupported {
		writeJSON(w, http.StatusUnsupportedMediaType, body)
		return
	}
	writeJSON(w, http.StatusNotImplemented, body)
}

func (s *Server) assetDetail(asset catalog.Asset) map[string]any {
	detail := map[string]any{
		"asset":        asset,
		"locations":    asset.Locations,
		"preview":      preview.ForAsset(asset),
		"metadata":     map[string]any{},
		"timestamps":   map[string]any{"first_seen_at": asset.FirstSeenAt, "updated_at": asset.UpdatedAt},
		"content":      map[string]any{"hash_status": "unhashed"},
		"original_url": "",
	}
	if loc, ok := catalog.FirstLocation(asset); ok {
		detail["original_url"] = "/api/v1/media/" + asset.ID + "/original"
		detail["preview_url"] = "/api/v1/media/" + asset.ID + "/preview"
		detail["content"] = map[string]any{
			"hash_status": loc.HashStatus,
			"sha512_hex":  loc.SHA512Hex,
			"content_id":  loc.ContentID,
			"size_bytes":  loc.SizeBytes,
		}
	}
	return detail
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat("webui/dist/index.html"); err == nil {
		http.FileServer(http.Dir("webui/dist")).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("Cartolensia API is running. Build webui/ to serve the frontend.\n"))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func (s *Server) requireWrite(w http.ResponseWriter, r *http.Request, action string) bool {
	principal, err := s.deps.Authenticator.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return false
	}
	if err := s.deps.Authorizer.Authorize(principal, action); err != nil {
		status := http.StatusForbidden
		if errors.Is(err, auth.ErrUnauthenticated) {
			status = http.StatusUnauthorized
		}
		writeError(w, status, err)
		return false
	}
	return true
}
