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
	"github.com/AxisAlexNT/Cartolensia/internal/media"
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
	AuthService   *auth.LocalService
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
	s.mux.HandleFunc("/api/v1/auth/me", s.handleAuthMe)
	s.mux.HandleFunc("/api/v1/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("/api/v1/auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("/api/v1/auth/tokens", s.handleAuthTokens)
	s.mux.HandleFunc("/api/v1/auth/tokens/", s.handleAuthTokenByID)
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
	s.mux.HandleFunc("/api/v1/tracks", s.handleTracks)
	s.mux.HandleFunc("/api/v1/tracks/", s.handleTrackByID)
	s.mux.HandleFunc("/api/v1/sync/candidates", s.handleSyncCandidates)
	s.mux.HandleFunc("/api/v1/sync/links", s.handleSyncLinks)
	s.mux.HandleFunc("/api/v1/map", s.handleMap)
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

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	principal, err := s.deps.Authenticator.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"principal": principal, "auth_mode": s.deps.Config.Auth.Mode})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.deps.AuthService == nil {
		principal, err := s.deps.Authenticator.Authenticate(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"principal": principal})
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, secret, err := s.deps.AuthService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.deps.AuthService.CookieName(),
		Value:    secret,
		Path:     "/",
		Expires:  result.Session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.deps.AuthService != nil {
		if token, ok := s.deps.AuthService.TokenFromRequest(r); ok {
			if err := s.deps.AuthService.Logout(r.Context(), token); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name:     s.deps.AuthService.CookieName(),
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthTokens(w http.ResponseWriter, r *http.Request) {
	if s.deps.AuthService == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("api tokens require local auth mode"))
		return
	}
	principal, err := s.deps.Authenticator.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		tokens, err := s.deps.AuthService.ListAPITokens(r.Context(), principal)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, tokens)
	case http.MethodPost:
		var req struct {
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		token, err := s.deps.AuthService.CreateAPIToken(r.Context(), principal, req.Name, req.Scopes, nil)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, token)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAuthTokenByID(w http.ResponseWriter, r *http.Request) {
	if s.deps.AuthService == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("api tokens require local auth mode"))
		return
	}
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	tokenID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/auth/tokens/"), "/")
	if tokenID == "" || strings.Contains(tokenID, "/") {
		http.NotFound(w, r)
		return
	}
	principal, err := s.deps.Authenticator.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := s.deps.AuthService.RevokeAPIToken(r.Context(), principal, tokenID); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
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

func (s *Server) handleTracks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	tracks, err := s.deps.Store.ListTracks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Server) handleTrackByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	trackID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/tracks/"), "/")
	if trackID == "" || strings.Contains(trackID, "/") {
		http.NotFound(w, r)
		return
	}
	track, err := s.deps.Store.GetTrack(r.Context(), trackID)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (s *Server) handleSyncCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	assetID := r.URL.Query().Get("asset_id")
	if assetID == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("asset_id is required"))
		return
	}
	candidates, err := s.deps.Store.TrackCandidates(r.Context(), assetID)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, candidates)
}

func (s *Server) handleSyncLinks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		links, err := s.deps.Store.ListTrackLinks(r.Context(), r.URL.Query().Get("asset_id"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, links)
	case http.MethodPost:
		if !s.requireWrite(w, r, "sync.links.save") {
			return
		}
		var link catalog.TrackLink
		if err := json.NewDecoder(r.Body).Decode(&link); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		saved, err := s.deps.Store.SaveTrackLink(r.Context(), link)
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	bbox, hasBBox, err := parseBBox(r.URL.Query().Get("bbox"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	features := make([]map[string]any, 0)
	tracks, err := s.deps.Store.ListTracks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, summary := range tracks {
		if hasBBox && !trackIntersectsBBox(summary, bbox) {
			continue
		}
		detail, err := s.deps.Store.GetTrack(r.Context(), summary.TrackAssetID)
		if err != nil {
			continue
		}
		coords := make([][]float64, 0, len(detail.Points))
		for _, point := range detail.Points {
			if hasBBox && !pointInBBox(point.Lon, point.Lat, bbox) {
				continue
			}
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
				"id":          summary.TrackAssetID,
				"name":        summary.Name,
				"kind":        "track",
				"point_count": summary.PointCount,
			},
		})
	}
	assets, err := s.deps.Store.ListAssets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, asset := range assets {
		lat, lon, ok := assetLatLon(asset)
		if !ok || (hasBBox && !pointInBBox(lon, lat, bbox)) {
			continue
		}
		features = append(features, map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []float64{lon, lat},
			},
			"properties": map[string]any{
				"id":         asset.ID,
				"name":       asset.DisplayName,
				"kind":       asset.MediaKind,
				"clustered":  false,
				"asset_type": "asset",
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"type": "FeatureCollection", "features": features, "clustering": "not_implemented"})
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
		"tools":         map[string]any{"ffprobe": media.DetectFFProbe()},
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
	loc, ok := catalog.FirstLocation(asset)
	if !ok {
		writeJSON(w, http.StatusUnsupportedMediaType, preview.ForAsset(asset))
		return
	}
	file, _, err := s.deps.Registry.OpenByURL(loc.StorageURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	info, err := preview.GenerateImage(r.Context(), s.deps.Config.Cache.Dir, asset, file)
	closeErr := file.Close()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, info)
		return
	}
	if closeErr != nil {
		writeError(w, http.StatusInternalServerError, closeErr)
		return
	}
	if info.Status == preview.StatusUnsupported {
		writeJSON(w, http.StatusUnsupportedMediaType, info)
		return
	}
	if info.Status != preview.StatusReady {
		writeJSON(w, http.StatusNotImplemented, info)
		return
	}
	previewFile, err := os.Open(info.CachePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer previewFile.Close()
	stat, err := previewFile.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Cartolensia-Preview-Status", string(info.Status))
	http.ServeContent(w, r, "preview.jpg", stat.ModTime(), previewFile)
}

func (s *Server) assetDetail(asset catalog.Asset) map[string]any {
	detail := map[string]any{
		"asset":        asset,
		"locations":    asset.Locations,
		"preview":      preview.InfoForAsset(s.deps.Config.Cache.Dir, asset),
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

type bbox struct {
	MinLon float64
	MinLat float64
	MaxLon float64
	MaxLat float64
}

func parseBBox(raw string) (bbox, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return bbox{}, false, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return bbox{}, false, fmt.Errorf("bbox must be minLon,minLat,maxLon,maxLat")
	}
	values := make([]float64, 4)
	for i, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return bbox{}, false, fmt.Errorf("parse bbox: %w", err)
		}
		values[i] = value
	}
	if values[0] > values[2] || values[1] > values[3] {
		return bbox{}, false, fmt.Errorf("bbox min values must not exceed max values")
	}
	return bbox{MinLon: values[0], MinLat: values[1], MaxLon: values[2], MaxLat: values[3]}, true, nil
}

func pointInBBox(lon, lat float64, b bbox) bool {
	return lon >= b.MinLon && lon <= b.MaxLon && lat >= b.MinLat && lat <= b.MaxLat
}

func trackIntersectsBBox(summary catalog.TrackSummary, b bbox) bool {
	if summary.MinLon == nil || summary.MaxLon == nil || summary.MinLat == nil || summary.MaxLat == nil {
		return true
	}
	return *summary.MaxLon >= b.MinLon && *summary.MinLon <= b.MaxLon && *summary.MaxLat >= b.MinLat && *summary.MinLat <= b.MaxLat
}

func assetLatLon(asset catalog.Asset) (float64, float64, bool) {
	if asset.Metadata == nil {
		return 0, 0, false
	}
	lat, okLat := metadataFloat(asset.Metadata["lat"])
	lon, okLon := metadataFloat(asset.Metadata["lon"])
	return lat, lon, okLat && okLon
}

func metadataFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
