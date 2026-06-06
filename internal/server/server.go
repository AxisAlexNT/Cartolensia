package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/auth"
	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/config"
	"github.com/AxisAlexNT/Cartolensia/internal/database"
	"github.com/AxisAlexNT/Cartolensia/internal/discovery"
	"github.com/AxisAlexNT/Cartolensia/internal/gpx"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/media"
	"github.com/AxisAlexNT/Cartolensia/internal/metadata"
	"github.com/AxisAlexNT/Cartolensia/internal/plugins"
	"github.com/AxisAlexNT/Cartolensia/internal/preview"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
	"github.com/AxisAlexNT/Cartolensia/internal/transcoding"
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

type explorerRow struct {
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
	s.mux.HandleFunc("/api/v1/auth/csrf", s.handleAuthCSRF)
	s.mux.HandleFunc("/api/v1/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("/api/v1/auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("/api/v1/auth/password", s.handleAuthPassword)
	s.mux.HandleFunc("/api/v1/auth/tokens", s.handleAuthTokens)
	s.mux.HandleFunc("/api/v1/auth/tokens/", s.handleAuthTokenByID)
	s.mux.HandleFunc("/api/v1/storages", s.handleStorages)
	s.mux.HandleFunc("/api/v1/plugins", s.handlePlugins)
	s.mux.HandleFunc("/api/v1/plugins/", s.handlePluginByID)
	s.mux.HandleFunc("/api/v1/plugins/rescan", s.handlePluginsRescan)
	s.mux.HandleFunc("/api/v1/jobs", s.handleJobs)
	s.mux.HandleFunc("/api/v1/jobs/stats", s.handleJobStats)
	s.mux.HandleFunc("/api/v1/jobs/", s.handleJobByID)
	s.mux.HandleFunc("/api/v1/discovery/start", s.handleDiscoveryStart)
	s.mux.HandleFunc("/api/v1/hash/start", s.handleHashStart)
	s.mux.HandleFunc("/api/v1/metadata/enrich/start", s.handleMetadataEnrichStart)
	s.mux.HandleFunc("/api/v1/previews/start", s.handlePreviewsStart)
	s.mux.HandleFunc("/api/v1/assets", s.handleAssets)
	s.mux.HandleFunc("/api/v1/assets/", s.handleAssetByID)
	s.mux.HandleFunc("/api/v1/explorer", s.handleExplorer)
	s.mux.HandleFunc("/api/v1/tracks", s.handleTracks)
	s.mux.HandleFunc("/api/v1/tracks/", s.handleTrackByID)
	s.mux.HandleFunc("/api/v1/sync/candidates", s.handleSyncCandidates)
	s.mux.HandleFunc("/api/v1/sync/links", s.handleSyncLinks)
	s.mux.HandleFunc("/api/v1/sync/links/", s.handleSyncLinkByID)
	s.mux.HandleFunc("/api/v1/videos/", s.handleVideoByID)
	s.mux.HandleFunc("/api/v1/map", s.handleMap)
	s.mux.HandleFunc("/api/v1/transcoding/status", s.handleTranscodingStatus)
	s.mux.HandleFunc("/api/v1/transcoding/capabilities", s.handleTranscodingCapabilities)
	s.mux.HandleFunc("/api/v1/transcoding/presets", s.handleTranscodingPresets)
	s.mux.HandleFunc("/api/v1/ai/status", s.handleAIStatus)
	s.mux.HandleFunc("/api/v1/ai/accelerators", s.handleAIAccelerators)
	s.mux.HandleFunc("/api/v1/vector/status", s.handleVectorStatus)
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
	writeJSON(w, http.StatusOK, map[string]any{"principal": principal, "auth": s.authStatus()})
}

func (s *Server) handleAuthCSRF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.deps.AuthService == nil {
		writeJSON(w, http.StatusOK, map[string]any{"required": false, "header": "", "token": ""})
		return
	}
	credential, ok := s.deps.AuthService.CredentialFromRequest(r)
	if !ok || credential.Method != auth.AuthMethodSession {
		writeError(w, http.StatusUnauthorized, auth.ErrUnauthenticated)
		return
	}
	if _, err := s.deps.Authenticator.Authenticate(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"required": true,
		"header":   s.deps.AuthService.CSRFHeader(),
		"token":    s.deps.AuthService.CSRFToken(credential.Secret),
	})
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
		Secure:   s.deps.AuthService.CookieSecure(),
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
		if err := s.deps.AuthService.ValidateCSRF(r); err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
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
			Secure:   s.deps.AuthService.CookieSecure(),
			SameSite: http.SameSiteLaxMode,
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthPassword(w http.ResponseWriter, r *http.Request) {
	if s.deps.AuthService == nil {
		writeError(w, http.StatusNotImplemented, fmt.Errorf("password changes require local auth mode"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	principal, err := s.deps.Authenticator.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := s.deps.Authorizer.Authorize(principal, "auth.password.change"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if err := s.deps.AuthService.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.deps.AuthService.ChangePassword(r.Context(), principal, req.OldPassword, req.NewPassword); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrUnauthenticated) {
			status = http.StatusUnauthorized
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
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
		if err := s.deps.AuthService.ValidateCSRF(r); err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
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
	if err := s.deps.AuthService.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusForbidden, err)
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

func (s *Server) handlePluginByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/plugins/"), "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	manifest, ok := s.plugin(parts[0])
	if !ok {
		writeError(w, http.StatusNotFound, catalog.ErrNotFound)
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, manifest)
		return
	}
	if len(parts) == 2 && parts[1] == "health" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, s.pluginHealth(manifest))
		return
	}
	http.NotFound(w, r)
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
	jobsList = filterJobs(jobsList, r)
	writeJSON(w, http.StatusOK, jobsList)
}

func (s *Server) handleJobStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	jobsList, err := s.deps.Store.ListJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, buildJobStats(jobsList))
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	jobID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		job, err := s.deps.Store.GetJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "cancel":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if !s.requireWrite(w, r, "jobs.cancel") {
			return
		}
		job, err := s.deps.Store.RequestCancelJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, job)
	case "retry":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if !s.requireWrite(w, r, "jobs.retry") {
			return
		}
		var req struct {
			Force bool `json:"force"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		job, err := s.deps.Store.GetJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !retryAllowed(job, req.Force) {
			writeError(w, http.StatusConflict, fmt.Errorf("job retry is not allowed without force"))
			return
		}
		if err := jobs.RetryNow(&job, fmt.Errorf("manual retry requested")); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := s.deps.Store.UpdateJob(r.Context(), job); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, job)
	case "logs":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		job, err := s.deps.Store.GetJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, paginateLogs(job.Logs, r))
	default:
		http.NotFound(w, r)
	}
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

func (s *Server) handleMetadataEnrichStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "metadata.enrich") {
		return
	}
	payload := metadata.NewPayload()
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}
	job, err := s.deps.Store.EnqueueJob(r.Context(), jobs.New("metadata_enrich", payload))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if s.deps.SyncJobs {
		runner := metadata.Runner{Registry: s.deps.Registry, Store: s.deps.Store}
		if err := runner.Enrich(r.Context(), &job); err != nil && !errors.Is(err, jobs.ErrCanceled) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handlePreviewsStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "previews.generate") {
		return
	}
	var payload preview.GeneratePayload
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}
	job, err := s.deps.Store.EnqueueJob(r.Context(), jobs.New("preview_generate", payload))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if s.deps.SyncJobs {
		runner := preview.Runner{Registry: s.deps.Registry, Store: s.deps.Store, CacheDir: s.deps.Config.Cache.Dir}
		if err := runner.Generate(r.Context(), &job); err != nil && !errors.Is(err, jobs.ErrCanceled) {
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
	assets = filterAssets(assets, r.URL.Query())
	sortAssets(assets, r.URL.Query().Get("sort"))
	total := len(assets)
	assets = paginateAssets(assets, r.URL.Query())
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
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
	if query.Get("view") == "folders" || query.Get("path") != "" || query.Get("storage") != "" || query.Get("storage_name") != "" {
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
	rows := make([]explorerRow, 0, len(assets))
	for _, asset := range assets {
		loc, ok := catalog.FirstLocation(asset)
		if !ok {
			continue
		}
		rows = append(rows, explorerRow{
			AssetID: asset.ID, Name: loc.FileName, MediaKind: loc.MediaKind, StorageURL: loc.StorageURL,
			RelativePath: loc.RelativePath, SizeBytes: loc.SizeBytes, MTime: loc.MTime, HashStatus: loc.HashStatus, SHA512Hex: loc.SHA512Hex,
		})
	}
	rows = filterExplorerRows(rows, query)
	sortExplorerRows(rows, query.Get("sort"))
	total := len(rows)
	rows = paginateExplorerRows(rows, query)
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	writeJSON(w, http.StatusOK, rows)
}

func filterAssets(assets []catalog.Asset, query url.Values) []catalog.Asset {
	assets = catalog.SearchAssets(assets, query.Get("q"))
	mediaKind := strings.TrimSpace(query.Get("media_kind"))
	hashStatus := strings.TrimSpace(query.Get("hash_status"))
	storageName := firstNonEmpty(query.Get("storage"), query.Get("storage_name"))
	extension := normalizeExtension(query.Get("extension"))
	if mediaKind == "" && hashStatus == "" && storageName == "" && extension == "" {
		return assets
	}
	out := make([]catalog.Asset, 0, len(assets))
	for _, asset := range assets {
		for _, loc := range asset.Locations {
			if mediaKind != "" && loc.MediaKind != mediaKind {
				continue
			}
			if hashStatus != "" && loc.HashStatus != hashStatus {
				continue
			}
			if storageName != "" && loc.StorageName != storageName {
				continue
			}
			if extension != "" && normalizeExtension(loc.Extension) != extension {
				continue
			}
			out = append(out, asset)
			break
		}
	}
	return out
}

func sortAssets(assets []catalog.Asset, key string) {
	sort.SliceStable(assets, func(i, j int) bool {
		leftLoc, leftOK := catalog.FirstLocation(assets[i])
		rightLoc, rightOK := catalog.FirstLocation(assets[j])
		if !leftOK || !rightOK {
			return leftOK && !rightOK
		}
		switch key {
		case "size":
			if leftLoc.SizeBytes == rightLoc.SizeBytes {
				return assets[i].DisplayName < assets[j].DisplayName
			}
			return leftLoc.SizeBytes < rightLoc.SizeBytes
		case "mtime":
			if leftLoc.MTime.Equal(rightLoc.MTime) {
				return assets[i].DisplayName < assets[j].DisplayName
			}
			return leftLoc.MTime.After(rightLoc.MTime)
		case "media_kind":
			if assets[i].MediaKind == assets[j].MediaKind {
				return assets[i].DisplayName < assets[j].DisplayName
			}
			return assets[i].MediaKind < assets[j].MediaKind
		case "taken_at":
			leftTaken := assets[i].TakenAt
			rightTaken := assets[j].TakenAt
			if leftTaken == nil || rightTaken == nil {
				return leftTaken != nil && rightTaken == nil
			}
			if leftTaken.Equal(*rightTaken) {
				return assets[i].DisplayName < assets[j].DisplayName
			}
			return leftTaken.After(*rightTaken)
		default:
			return assets[i].DisplayName < assets[j].DisplayName
		}
	})
}

func paginateAssets(assets []catalog.Asset, query url.Values) []catalog.Asset {
	offset, _ := strconv.Atoi(query.Get("offset"))
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(assets) {
		return []catalog.Asset{}
	}
	end := offset + limit
	if end > len(assets) {
		end = len(assets)
	}
	return assets[offset:end]
}

func filterExplorerRows(rows []explorerRow, query url.Values) []explorerRow {
	text := strings.ToLower(strings.TrimSpace(query.Get("q")))
	mediaKind := strings.TrimSpace(query.Get("media_kind"))
	hashStatus := strings.TrimSpace(query.Get("hash_status"))
	storageName := firstNonEmpty(query.Get("storage"), query.Get("storage_name"))
	extension := normalizeExtension(query.Get("extension"))
	if text == "" && mediaKind == "" && hashStatus == "" && storageName == "" && extension == "" {
		return rows
	}
	out := make([]explorerRow, 0, len(rows))
	for _, row := range rows {
		if text != "" && !strings.Contains(strings.ToLower(row.Name), text) && !strings.Contains(strings.ToLower(row.RelativePath), text) {
			continue
		}
		if mediaKind != "" && row.MediaKind != mediaKind {
			continue
		}
		if hashStatus != "" && row.HashStatus != hashStatus {
			continue
		}
		if storageName != "" && !strings.HasPrefix(row.StorageURL, "fs://"+storageName+"/") {
			continue
		}
		if extension != "" && normalizeExtension(pathExtension(row.Name)) != extension {
			continue
		}
		out = append(out, row)
	}
	return out
}

func sortExplorerRows(rows []explorerRow, key string) {
	sort.SliceStable(rows, func(i, j int) bool {
		switch key {
		case "size":
			if rows[i].SizeBytes == rows[j].SizeBytes {
				return rows[i].Name < rows[j].Name
			}
			return rows[i].SizeBytes < rows[j].SizeBytes
		case "mtime":
			if rows[i].MTime.Equal(rows[j].MTime) {
				return rows[i].Name < rows[j].Name
			}
			return rows[i].MTime.After(rows[j].MTime)
		case "media_kind":
			if rows[i].MediaKind == rows[j].MediaKind {
				return rows[i].Name < rows[j].Name
			}
			return rows[i].MediaKind < rows[j].MediaKind
		default:
			return rows[i].Name < rows[j].Name
		}
	})
}

func paginateExplorerRows(rows []explorerRow, query url.Values) []explorerRow {
	offset, _ := strconv.Atoi(query.Get("offset"))
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(rows) {
		return []explorerRow{}
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeExtension(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, ".")
	return value
}

func pathExtension(name string) string {
	if idx := strings.LastIndexByte(name, '.'); idx >= 0 && idx+1 < len(name) {
		return name[idx+1:]
	}
	return ""
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

func (s *Server) handleSyncLinkByID(w http.ResponseWriter, r *http.Request) {
	linkID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/sync/links/"), "/")
	if linkID == "" || strings.Contains(linkID, "/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "sync.links.delete") {
		return
	}
	if err := s.deps.Store.DeleteTrackLink(r.Context(), linkID); err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleVideoByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/videos/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "track-sync" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	assetID := parts[0]
	timeMS, err := strconv.ParseInt(r.URL.Query().Get("time_ms"), 10, 64)
	if err != nil || timeMS < 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("time_ms must be a non-negative integer"))
		return
	}
	marker, err := s.trackMarker(r.Context(), assetID, timeMS, r.URL.Query().Get("track_asset_id"))
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, marker)
}

func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query := r.URL.Query()
	bbox, hasBBox, err := parseBBox(query.Get("bbox"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	zoom, _ := strconv.Atoi(query.Get("zoom"))
	if zoom <= 0 {
		zoom = 10
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	mediaKind := query.Get("media_kind")
	timeFrom, _ := time.Parse(time.RFC3339, query.Get("time_from"))
	timeTo, _ := time.Parse(time.RFC3339, query.Get("time_to"))
	selectedTrackIDs := csvSet(query.Get("track_ids"))
	selectedAssetIDs := csvSet(query.Get("asset_ids"))
	clusterPoints := query.Get("cluster") == "1" || strings.EqualFold(query.Get("cluster"), "true")
	features := make([]map[string]any, 0)
	tracks, err := s.deps.Store.ListTracks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, summary := range tracks {
		if len(selectedTrackIDs) > 0 {
			if _, ok := selectedTrackIDs[summary.TrackAssetID]; !ok {
				continue
			}
		}
		if !trackTimeMatches(summary, timeFrom, timeTo) {
			continue
		}
		if hasBBox && !trackIntersectsBBox(summary, bbox) {
			continue
		}
		detail, err := s.deps.Store.GetTrack(r.Context(), summary.TrackAssetID)
		if err != nil {
			continue
		}
		points := detail.Points
		if maxTrackPoints := maxTrackPointsForZoom(zoom); len(points) > maxTrackPoints {
			points = gpx.Simplify(points, maxTrackPoints)
		}
		coords := make([][]float64, 0, len(points))
		for _, point := range points {
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
				"simplified":  len(points) < summary.PointCount,
			},
		})
	}
	assets, err := s.deps.Store.ListAssets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var pointFeatures []map[string]any
	for _, asset := range assets {
		if mediaKind != "" && asset.MediaKind != mediaKind {
			continue
		}
		if len(selectedAssetIDs) > 0 {
			if _, ok := selectedAssetIDs[asset.ID]; !ok {
				continue
			}
		}
		if !assetTimeMatches(asset, timeFrom, timeTo) {
			continue
		}
		lat, lon, ok := assetLatLon(asset)
		if !ok || (hasBBox && !pointInBBox(lon, lat, bbox)) {
			continue
		}
		pointFeatures = append(pointFeatures, map[string]any{
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
		if len(pointFeatures) >= limit {
			break
		}
	}
	if clusterPoints {
		pointFeatures = clusterGeoJSONPoints(pointFeatures, bbox, hasBBox, zoom)
	}
	features = append(features, pointFeatures...)
	if len(features) > limit {
		features = features[:limit]
	}
	clustering := "raw"
	if clusterPoints {
		clustering = "grid"
	}
	writeJSON(w, http.StatusOK, map[string]any{"type": "FeatureCollection", "features": features, "clustering": clustering, "zoom": zoom, "limit": limit})
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

func (s *Server) handleTranscodingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, transcoding.Status(r.Context()))
}

func (s *Server) handleTranscodingCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, transcoding.Detect(r.Context()))
}

func (s *Server) handleTranscodingPresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, []map[string]any{
		{"id": "archive-h264", "name": "Archive H.264", "implemented": false},
		{"id": "archive-av1", "name": "Archive AV1", "implemented": false},
	})
}

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":            false,
		"inference_running":  false,
		"vector_store":       "not_configured",
		"embedding_jobs":     "not_implemented",
		"accelerator_hints":  transcoding.AcceleratorHints(),
		"planned_modalities": []string{"image", "video_frame", "audio_segment", "text_query"},
	})
}

func (s *Server) handleAIAccelerators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, transcoding.AcceleratorHints())
}

func (s *Server) handleVectorStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": false,
		"backend":   "none",
		"pgvector":  capabilityInstalled(s.deps.Capabilities, "vector"),
		"contract":  "VectorStore interface planned; no embeddings are generated in this build",
	})
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
		"auth":          s.authStatus(),
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
	if s.deps.AuthService != nil {
		if err := s.deps.AuthService.ValidateCSRF(r); err != nil {
			writeError(w, http.StatusForbidden, err)
			return false
		}
	}
	return true
}

func (s *Server) authStatus() map[string]any {
	status := map[string]any{
		"mode":          s.deps.Config.Auth.Mode,
		"oidc_enabled":  false,
		"oauth_enabled": false,
	}
	if s.deps.Config.Auth.Mode == "dev_no_auth" {
		status["warning"] = "authentication is disabled for development"
	}
	if s.deps.AuthService != nil {
		status["session_cookie_name"] = s.deps.AuthService.CookieName()
		status["session_cookie_secure"] = s.deps.AuthService.CookieSecure()
		status["csrf_required"] = true
		status["csrf_header"] = s.deps.AuthService.CSRFHeader()
	} else {
		status["csrf_required"] = false
	}
	return status
}

func filterJobs(input []jobs.Job, r *http.Request) []jobs.Job {
	query := r.URL.Query()
	status := jobs.Status(query.Get("status"))
	kind := query.Get("kind")
	failedOnly := query.Get("failed_only") == "1" || strings.EqualFold(query.Get("failed_only"), "true")
	runningOnly := query.Get("running_only") == "1" || strings.EqualFold(query.Get("running_only"), "true")
	createdAfter, _ := time.Parse(time.RFC3339, query.Get("created_after"))
	createdBefore, _ := time.Parse(time.RFC3339, query.Get("created_before"))
	out := make([]jobs.Job, 0, len(input))
	for _, job := range input {
		if status != "" && job.Status != status {
			continue
		}
		if kind != "" && job.Kind != kind {
			continue
		}
		if failedOnly && job.Status != jobs.StatusFailed {
			continue
		}
		if runningOnly && job.Status != jobs.StatusRunning && job.Status != jobs.StatusCancelRequested {
			continue
		}
		if !createdAfter.IsZero() && job.CreatedAt.Before(createdAfter) {
			continue
		}
		if !createdBefore.IsZero() && job.CreatedAt.After(createdBefore) {
			continue
		}
		out = append(out, job)
	}
	switch query.Get("sort") {
	case "created_at":
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	case "kind":
		sort.Slice(out, func(i, j int) bool {
			if out[i].Kind == out[j].Kind {
				return out[i].CreatedAt.After(out[j].CreatedAt)
			}
			return out[i].Kind < out[j].Kind
		})
	case "status":
		sort.Slice(out, func(i, j int) bool {
			if out[i].Status == out[j].Status {
				return out[i].CreatedAt.After(out[j].CreatedAt)
			}
			return out[i].Status < out[j].Status
		})
	default:
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	}
	offset, _ := strconv.Atoi(query.Get("offset"))
	limit, _ := strconv.Atoi(query.Get("limit"))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset >= len(out) {
		return []jobs.Job{}
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end]
}

func paginateLogs(logs []jobs.LogLine, r *http.Request) map[string]any {
	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	out := make([]jobs.LogLine, 0, len(logs))
	for _, line := range logs {
		if afterID > 0 && line.ID > 0 && line.ID <= afterID {
			continue
		}
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	var nextAfter int64
	if len(out) > 0 {
		nextAfter = out[len(out)-1].ID
	}
	return map[string]any{"logs": out, "next_after_id": nextAfter}
}

func buildJobStats(input []jobs.Job) map[string]any {
	counts := map[string]int{}
	workers := map[string]struct{}{}
	var oldestQueued *time.Time
	var lastErrors []map[string]any
	for _, job := range input {
		counts[string(job.Status)]++
		if job.WorkerID != "" && (job.Status == jobs.StatusRunning || job.Status == jobs.StatusCancelRequested) {
			workers[job.WorkerID] = struct{}{}
		}
		if job.Status == jobs.StatusQueued && (oldestQueued == nil || job.CreatedAt.Before(*oldestQueued)) {
			t := job.CreatedAt
			oldestQueued = &t
		}
		if job.Error != "" {
			lastErrors = append(lastErrors, map[string]any{"id": job.ID, "kind": job.Kind, "status": job.Status, "error": job.Error, "finished_at": job.FinishedAt})
			if len(lastErrors) > 10 {
				lastErrors = lastErrors[:10]
			}
		}
	}
	workerIDs := make([]string, 0, len(workers))
	for workerID := range workers {
		workerIDs = append(workerIDs, workerID)
	}
	sort.Strings(workerIDs)
	return map[string]any{
		"queued":            counts[string(jobs.StatusQueued)],
		"running":           counts[string(jobs.StatusRunning)],
		"cancel_requested":  counts[string(jobs.StatusCancelRequested)],
		"failed":            counts[string(jobs.StatusFailed)],
		"succeeded":         counts[string(jobs.StatusSucceeded)],
		"cancelled":         counts[string(jobs.StatusCanceled)],
		"oldest_queued_job": oldestQueued,
		"active_worker_ids": workerIDs,
		"last_errors":       lastErrors,
	}
}

func retryAllowed(job jobs.Job, force bool) bool {
	if job.Status != jobs.StatusFailed && job.Status != jobs.StatusCanceled {
		return false
	}
	if force {
		return true
	}
	return !looksPermanentJobError(job.Error)
}

func looksPermanentJobError(message string) bool {
	message = strings.ToLower(message)
	permanentHints := []string{
		"read-only",
		"read only",
		"traversal",
		"escapes",
		"unknown storage",
		"not configured",
		"unsupported storage",
		"permission denied",
	}
	for _, hint := range permanentHints {
		if strings.Contains(message, hint) {
			return true
		}
	}
	return false
}

func capabilityInstalled(caps []database.Capability, name string) bool {
	for _, cap := range caps {
		if cap.Name == name {
			return cap.Installed
		}
	}
	return false
}

func (s *Server) plugin(id string) (plugins.Manifest, bool) {
	for _, manifest := range s.deps.Plugins {
		if manifest.ID == id {
			return manifest, true
		}
	}
	return plugins.Manifest{}, false
}

func (s *Server) pluginHealth(manifest plugins.Manifest) map[string]any {
	status := manifest.Status
	if status == "" {
		status = "loaded"
	}
	health := map[string]any{
		"id":           manifest.ID,
		"name":         manifest.Name,
		"runtime":      manifest.Runtime,
		"status":       status,
		"version":      manifest.Version,
		"capabilities": manifest.Capabilities,
		"permissions":  manifest.Permissions,
		"last_error":   manifest.LastError,
	}
	if manifest.Runtime == "sidecar_http" {
		health["status"] = "unsupported"
		health["message"] = "sidecar HTTP plugins are user-managed future integrations; core does not auto-start sidecars"
		health["sidecar_http"] = manifest.SidecarHTTP
	}
	return health
}

func (s *Server) trackMarker(ctx context.Context, assetID string, timeMS int64, trackAssetID string) (map[string]any, error) {
	asset, err := s.deps.Store.GetAsset(ctx, assetID)
	if err != nil {
		return nil, err
	}
	if asset.TakenAt == nil {
		return nil, catalog.ErrNotFound
	}
	links, err := s.deps.Store.ListTrackLinks(ctx, assetID)
	if err != nil {
		return nil, err
	}
	var link catalog.TrackLink
	found := false
	for _, candidate := range links {
		if trackAssetID == "" || candidate.TrackAssetID == trackAssetID {
			link = candidate
			found = true
			break
		}
	}
	if !found {
		return nil, catalog.ErrNotFound
	}
	detail, err := s.deps.Store.GetTrack(ctx, link.TrackAssetID)
	if err != nil {
		return nil, err
	}
	target := asset.TakenAt.UTC().Add(time.Duration(timeMS+link.TimeOffsetMS) * time.Millisecond)
	point, mode, err := interpolateTrackPoint(detail.Points, target)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"asset_id":       assetID,
		"track_asset_id": link.TrackAssetID,
		"link_id":        link.ID,
		"playback_ms":    timeMS,
		"track_time":     target,
		"lat":            point.Lat,
		"lon":            point.Lon,
		"mode":           mode,
	}, nil
}

func interpolateTrackPoint(points []catalog.TrackPoint, target time.Time) (catalog.TrackPoint, string, error) {
	var timed []catalog.TrackPoint
	for _, point := range points {
		if !point.RecordedAt.IsZero() {
			timed = append(timed, point)
		}
	}
	if len(timed) == 0 {
		return catalog.TrackPoint{}, "", catalog.ErrNotFound
	}
	sort.Slice(timed, func(i, j int) bool { return timed[i].RecordedAt.Before(timed[j].RecordedAt) })
	if target.Before(timed[0].RecordedAt) || target.After(timed[len(timed)-1].RecordedAt) {
		return catalog.TrackPoint{}, "", catalog.ErrNotFound
	}
	for i, point := range timed {
		if point.RecordedAt.Equal(target) {
			return point, "exact", nil
		}
		if point.RecordedAt.After(target) && i > 0 {
			prev := timed[i-1]
			next := point
			total := next.RecordedAt.Sub(prev.RecordedAt).Seconds()
			if total <= 0 {
				return prev, "nearest", nil
			}
			ratio := target.Sub(prev.RecordedAt).Seconds() / total
			prev.Lat = prev.Lat + (next.Lat-prev.Lat)*ratio
			prev.Lon = prev.Lon + (next.Lon-prev.Lon)*ratio
			prev.RecordedAt = target
			return prev, "interpolated", nil
		}
	}
	return timed[len(timed)-1], "nearest", nil
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

func trackTimeMatches(summary catalog.TrackSummary, from, to time.Time) bool {
	if from.IsZero() && to.IsZero() {
		return true
	}
	if summary.StartTime == nil || summary.EndTime == nil {
		return false
	}
	if !from.IsZero() && summary.EndTime.Before(from) {
		return false
	}
	if !to.IsZero() && summary.StartTime.After(to) {
		return false
	}
	return true
}

func assetTimeMatches(asset catalog.Asset, from, to time.Time) bool {
	if from.IsZero() && to.IsZero() {
		return true
	}
	if asset.TakenAt == nil {
		return false
	}
	if !from.IsZero() && asset.TakenAt.Before(from) {
		return false
	}
	if !to.IsZero() && asset.TakenAt.After(to) {
		return false
	}
	return true
}

func maxTrackPointsForZoom(zoom int) int {
	switch {
	case zoom <= 5:
		return 200
	case zoom <= 9:
		return 600
	default:
		return 2000
	}
}

func csvSet(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = struct{}{}
		}
	}
	return out
}

func clusterGeoJSONPoints(features []map[string]any, b bbox, hasBBox bool, zoom int) []map[string]any {
	if len(features) <= 1 {
		return features
	}
	if !hasBBox {
		b = bbox{MinLon: -180, MinLat: -90, MaxLon: 180, MaxLat: 90}
	}
	divisions := 1 << minInt(maxInt(zoom, 1), 8)
	type bucket struct {
		count int
		lon   float64
		lat   float64
		first map[string]any
	}
	buckets := map[string]*bucket{}
	for _, feature := range features {
		lon, lat, ok := geoJSONPoint(feature)
		if !ok {
			continue
		}
		x := int((lon - b.MinLon) / (b.MaxLon - b.MinLon + 0.0000001) * float64(divisions))
		y := int((lat - b.MinLat) / (b.MaxLat - b.MinLat + 0.0000001) * float64(divisions))
		key := fmt.Sprintf("%d:%d", x, y)
		item := buckets[key]
		if item == nil {
			item = &bucket{first: feature}
			buckets[key] = item
		}
		item.count++
		item.lon += lon
		item.lat += lat
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		item := buckets[key]
		if item.count == 1 {
			out = append(out, item.first)
			continue
		}
		out = append(out, map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []float64{item.lon / float64(item.count), item.lat / float64(item.count)},
			},
			"properties": map[string]any{
				"kind":      "cluster",
				"clustered": true,
				"count":     item.count,
			},
		})
	}
	return out
}

func geoJSONPoint(feature map[string]any) (float64, float64, bool) {
	geometry, ok := feature["geometry"].(map[string]any)
	if !ok || geometry["type"] != "Point" {
		return 0, 0, false
	}
	coords, ok := geometry["coordinates"].([]float64)
	if ok && len(coords) == 2 {
		return coords[0], coords[1], true
	}
	values, ok := geometry["coordinates"].([]any)
	if !ok || len(values) != 2 {
		return 0, 0, false
	}
	lon, okLon := metadataFloat(values[0])
	lat, okLat := metadataFloat(values[1])
	return lon, lat, okLon && okLat
}

func assetLatLon(asset catalog.Asset) (float64, float64, bool) {
	if asset.Metadata == nil {
		return 0, 0, false
	}
	lat, okLat := metadataFloat(asset.Metadata["lat"])
	lon, okLon := metadataFloat(asset.Metadata["lon"])
	return lat, lon, okLat && okLon
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
