package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/auth"
	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/config"
	"github.com/AxisAlexNT/Cartolensia/internal/database"
	"github.com/AxisAlexNT/Cartolensia/internal/discovery"
	"github.com/AxisAlexNT/Cartolensia/internal/id"
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

type monthBucket struct {
	Month      string     `json:"month"`
	Count      int        `json:"count"`
	Photos     int        `json:"photos"`
	Videos     int        `json:"videos"`
	Tracks     int        `json:"tracks"`
	TotalBytes int64      `json:"total_bytes"`
	FirstAt    *time.Time `json:"first_at,omitempty"`
	LastAt     *time.Time `json:"last_at,omitempty"`
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
	s.mux.HandleFunc("/api/v1/settings", s.handleSettings)
	s.mux.HandleFunc("/api/v1/settings/effective", s.handleSettingsEffective)
	s.mux.HandleFunc("/api/v1/settings/runtime", s.handleSettingsRuntime)
	s.mux.HandleFunc("/api/v1/settings/pending/download", s.handleSettingsPendingDownload)
	s.mux.HandleFunc("/api/v1/settings/pending", s.handleSettingsPending)
	s.mux.HandleFunc("/api/v1/settings/restart-required", s.handleSettingsRestartRequired)
	s.mux.HandleFunc("/api/v1/admin/db/export", s.handleDBExport)
	s.mux.HandleFunc("/api/v1/admin/db/exports", s.handleDBExports)
	s.mux.HandleFunc("/api/v1/admin/db/exports/", s.handleDBExportByID)
	s.mux.HandleFunc("/api/v1/admin/db/import-plan", s.handleDBImportPlan)
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
	s.mux.HandleFunc("/api/v1/indexing/start", s.handleIndexingStart)
	s.mux.HandleFunc("/api/v1/indexing/latest", s.handleIndexingLatest)
	s.mux.HandleFunc("/api/v1/indexing/", s.handleIndexingByID)
	s.mux.HandleFunc("/api/v1/discovery/dry-run", s.handleDiscoveryDryRun)
	s.mux.HandleFunc("/api/v1/discovery/dry-run/", s.handleDiscoveryDryRunByJob)
	s.mux.HandleFunc("/api/v1/hash/start", s.handleHashStart)
	s.mux.HandleFunc("/api/v1/metadata/enrich/start", s.handleMetadataEnrichStart)
	s.mux.HandleFunc("/api/v1/previews/start", s.handlePreviewsStart)
	s.mux.HandleFunc("/api/v1/previews/status", s.handlePreviewStatus)
	s.mux.HandleFunc("/api/v1/previews/cache", s.handlePreviewCache)
	s.mux.HandleFunc("/api/v1/previews/cleanup", s.handlePreviewCleanup)
	s.mux.HandleFunc("/api/v1/assets/months", s.handleAssetMonths)
	s.mux.HandleFunc("/api/v1/assets", s.handleAssets)
	s.mux.HandleFunc("/api/v1/assets/", s.handleAssetByID)
	s.mux.HandleFunc("/api/v1/duplicates", s.handleDuplicates)
	s.mux.HandleFunc("/api/v1/albums", s.handleAlbums)
	s.mux.HandleFunc("/api/v1/albums/", s.handleAlbumByID)
	s.mux.HandleFunc("/api/v1/explorer", s.handleExplorer)
	s.mux.HandleFunc("/api/v1/tracks", s.handleTracks)
	s.mux.HandleFunc("/api/v1/tracks/", s.handleTrackByID)
	s.mux.HandleFunc("/api/v1/gps/tracks", s.handleGPSTracks)
	s.mux.HandleFunc("/api/v1/gps/tracks/", s.handleGPSTrackByID)
	s.mux.HandleFunc("/api/v1/sync/candidates", s.handleSyncCandidates)
	s.mux.HandleFunc("/api/v1/sync/links", s.handleSyncLinks)
	s.mux.HandleFunc("/api/v1/sync/links/", s.handleSyncLinkByID)
	s.mux.HandleFunc("/api/v1/videos/", s.handleVideoByID)
	s.mux.HandleFunc("/api/v1/map", s.handleMap)
	s.mux.HandleFunc("/api/v1/map/", s.handleMapSubroute)
	s.mux.HandleFunc("/api/v1/transcoding/status", s.handleTranscodingStatus)
	s.mux.HandleFunc("/api/v1/transcoding/capabilities", s.handleTranscodingCapabilities)
	s.mux.HandleFunc("/api/v1/transcoding/presets", s.handleTranscodingPresets)
	s.mux.HandleFunc("/api/v1/ai/status", s.handleAIStatus)
	s.mux.HandleFunc("/api/v1/ai/accelerators", s.handleAIAccelerators)
	s.mux.HandleFunc("/api/v1/vector/status", s.handleVectorStatus)
	s.mux.HandleFunc("/api/v1/stats", s.handleStats)
	s.mux.HandleFunc("/api/v1/backend/status", s.handleBackendStatus)
	s.mux.HandleFunc("/api/v1/tiles/", s.handleTiles)
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
	if len(parts) == 2 && parts[1] == "settings" {
		s.handlePluginSettings(w, r, manifest.ID)
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
	payload := discovery.ScanPayload{}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if payload.MarkMissing {
		writeError(w, http.StatusBadRequest, fmt.Errorf("mark_missing is not supported for discovery/start"))
		return
	}
	payload = discovery.DecodeScanPayload(payload)
	var jobPayload any = map[string]any{"storage": "all"}
	if payload.Storage != "" || payload.Prefix != "" || len(payload.Prefixes) > 0 || payload.MaxFiles > 0 || payload.MaxBytes > 0 {
		jobPayload = payload
	}
	if err := discovery.ValidateScanSafety(s.deps.Registry, discovery.DecodeScanPayload(jobPayload)); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, err := s.deps.Store.EnqueueJob(r.Context(), jobs.New("discovery", jobPayload))
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

type indexingStartRequest struct {
	Storage           string   `json:"storage"`
	Prefix            string   `json:"prefix,omitempty"`
	Prefixes          []string `json:"prefixes,omitempty"`
	MaxFiles          int      `json:"max_files,omitempty"`
	MaxBytes          int64    `json:"max_bytes,omitempty"`
	IncludeExtensions []string `json:"include_extensions,omitempty"`
	ExcludePatterns   []string `json:"exclude_patterns,omitempty"`
	IndexFiles        *bool    `json:"index_files,omitempty"`
	Hash              *bool    `json:"hash,omitempty"`
	Metadata          *bool    `json:"metadata,omitempty"`
	Previews          *bool    `json:"previews,omitempty"`
	ParseTracks       *bool    `json:"parse_tracks,omitempty"`
	GeotagEXIF        *bool    `json:"geotag_exif,omitempty"`
	SnapToTracks      *bool    `json:"snap_to_tracks,omitempty"`
	RefreshMap        *bool    `json:"refresh_map,omitempty"`
}

func (s *Server) handleIndexingStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "discovery.start") {
		return
	}
	var req indexingStartRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	pipelineID := id.NewUUID()
	prefixes := compactStrings(req.Prefixes)
	if strings.TrimSpace(req.Prefix) != "" {
		prefixes = append([]string{strings.TrimSpace(req.Prefix)}, prefixes...)
	}
	scanPayload := discovery.DecodeScanPayload(discovery.ScanPayload{
		Storage:           strings.TrimSpace(req.Storage),
		Prefixes:          prefixes,
		MaxFiles:          req.MaxFiles,
		MaxBytes:          req.MaxBytes,
		IncludeExtensions: req.IncludeExtensions,
		ExcludePatterns:   req.ExcludePatterns,
		MarkMissing:       false,
	})
	if err := discovery.ValidateScanSafety(s.deps.Registry, scanPayload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	indexFiles := boolDefault(req.IndexFiles, true)
	summary, err := s.indexingScopeSummary(r.Context(), scanPayload.Storage, scanPayload.Prefixes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var queued []jobs.Job
	if indexFiles {
		jobPayload := map[string]any{
			"pipeline_id":        pipelineID,
			"pipeline_stage":     "discovery",
			"storage":            scanPayload.Storage,
			"prefixes":           scanPayload.Prefixes,
			"max_files":          scanPayload.MaxFiles,
			"max_bytes":          scanPayload.MaxBytes,
			"include_extensions": scanPayload.IncludeExtensions,
			"exclude_patterns":   scanPayload.ExcludePatterns,
			"mark_missing":       false,
		}
		job, err := s.deps.Store.EnqueueJob(r.Context(), jobs.New("discovery", jobPayload))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		queued = append(queued, job)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"pipeline_id": pipelineID,
		"scope":       summary,
		"queued_jobs": queued,
		"options": map[string]bool{
			"index_files":    indexFiles,
			"hash":           boolDefault(req.Hash, true),
			"metadata":       boolDefault(req.Metadata, true),
			"previews":       boolDefault(req.Previews, true),
			"parse_tracks":   boolDefault(req.ParseTracks, true),
			"geotag_exif":    boolDefault(req.GeotagEXIF, true),
			"snap_to_tracks": boolDefault(req.SnapToTracks, true),
			"refresh_map":    boolDefault(req.RefreshMap, true),
		},
		"note": "Discovery is queued first. The WebUI runs following stages sequentially after the current scope is known.",
	})
}

func (s *Server) handleIndexingLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	storageName := r.URL.Query().Get("storage")
	prefixes := compactStrings(r.URL.Query()["prefixes"])
	if prefix := strings.TrimSpace(r.URL.Query().Get("prefix")); prefix != "" {
		prefixes = append([]string{prefix}, prefixes...)
	}
	summary, err := s.indexingScopeSummary(r.Context(), storageName, prefixes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	jobsList, err := s.deps.Store.ListJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"scope":       summary,
		"latest_jobs": latestIndexingJobs(jobsList, storageName, prefixes),
	})
}

func (s *Server) handleIndexingByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/indexing/"), "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	pipelineID := parts[0]
	if len(parts) == 2 && parts[1] == "cancel" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if !s.requireWrite(w, r, "jobs.cancel") {
			return
		}
		jobsList, err := s.deps.Store.ListJobs(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		var canceled []jobs.Job
		for _, job := range jobsList {
			if !jobHasPayloadString(job, "pipeline_id", pipelineID) || !canCancelJob(job) {
				continue
			}
			next, err := s.deps.Store.RequestCancelJob(r.Context(), job.ID)
			if err == nil {
				canceled = append(canceled, next)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"pipeline_id": pipelineID, "canceled_jobs": canceled})
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	jobsList, err := s.deps.Store.ListJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var related []jobs.Job
	for _, job := range jobsList {
		if jobHasPayloadString(job, "pipeline_id", pipelineID) {
			related = append(related, job)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pipeline_id": pipelineID, "jobs": related})
}

func (s *Server) handleHashStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "hash.start") {
		return
	}
	payload := discovery.HashPayload{Scope: "unhashed"}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}
	payload = discovery.DecodeHashPayload(payload)
	if payload.Scope == "" {
		payload.Scope = "unhashed"
	}
	if err := discovery.ValidateHashSafety(s.deps.Registry, payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, err := s.deps.Store.EnqueueJob(r.Context(), jobs.New("hash", payload))
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
	query, err := assetQueryFromURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	page, err := s.deps.Store.QueryAssets(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(page.Page.Total))
	writeJSON(w, http.StatusOK, page.Assets)
}

func (s *Server) handleAssetMonths(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	assets, err := s.deps.Store.ListAssets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, buildMonthBuckets(assets, r.URL.Query()))
}

func (s *Server) handleDuplicates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	assets, err := s.deps.Store.ListAssets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	page := catalog.BuildDuplicateGroups(assets, limit, offset)
	w.Header().Set("X-Total-Count", strconv.Itoa(page.Page.Total))
	writeJSON(w, http.StatusOK, page)
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
			Storage:    firstNonEmpty(query.Get("storage"), query.Get("storage_name")),
			Path:       query.Get("path"),
			Q:          query.Get("q"),
			MediaKind:  query.Get("media_kind"),
			HashStatus: query.Get("hash_status"),
			Extension:  query.Get("extension"),
			Limit:      limit,
			Offset:     offset,
			Sort:       query.Get("sort"),
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

func buildMonthBuckets(assets []catalog.Asset, query url.Values) []monthBucket {
	buckets := map[string]*monthBucket{}
	mediaKind := strings.TrimSpace(query.Get("media_kind"))
	hashStatus := strings.TrimSpace(query.Get("hash_status"))
	storageName := firstNonEmpty(query.Get("storage"), query.Get("storage_name"))
	extension := normalizeExtension(query.Get("extension"))
	for _, asset := range assets {
		if mediaKind != "" && asset.MediaKind != mediaKind {
			continue
		}
		loc, ok := catalog.FirstLocation(asset)
		if !ok {
			continue
		}
		if storageName != "" && loc.StorageName != storageName {
			continue
		}
		if hashStatus != "" && loc.HashStatus != hashStatus {
			continue
		}
		if extension != "" && normalizeExtension(loc.Extension) != extension {
			continue
		}
		at := loc.MTime
		if asset.TakenAt != nil {
			at = *asset.TakenAt
		}
		if at.IsZero() {
			continue
		}
		month := at.UTC().Format("2006-01")
		bucket := buckets[month]
		if bucket == nil {
			bucket = &monthBucket{Month: month}
			buckets[month] = bucket
		}
		bucket.Count++
		bucket.TotalBytes += loc.SizeBytes
		switch asset.MediaKind {
		case "photo":
			bucket.Photos++
		case "video":
			bucket.Videos++
		case "track":
			bucket.Tracks++
		}
		t := at.UTC()
		if bucket.FirstAt == nil || t.Before(*bucket.FirstAt) {
			first := t
			bucket.FirstAt = &first
		}
		if bucket.LastAt == nil || t.After(*bucket.LastAt) {
			last := t
			bucket.LastAt = &last
		}
	}
	out := make([]monthBucket, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, *bucket)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Month > out[j].Month
	})
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
	if tracks == nil {
		tracks = []catalog.TrackSummary{}
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
	zoom, _ := strconv.Atoi(query.Get("zoom"))
	if zoom <= 0 {
		zoom = 10
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	trackFeatures, err := s.mapTrackFeatures(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	assetFeatures, clustering, err := s.mapAssetFeatures(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	features := append(trackFeatures, assetFeatures...)
	if len(features) > limit {
		features = features[:limit]
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
		"http": map[string]any{
			"addr":                 s.deps.Config.HTTP.Addr,
			"tls_enabled":          s.deps.Config.HTTP.TLSCertFile != "" || s.deps.Config.HTTP.TLSAutoSelfSigned,
			"tls_cert_configured":  s.deps.Config.HTTP.TLSCertFile != "",
			"tls_auto_self_signed": s.deps.Config.HTTP.TLSAutoSelfSigned,
		},
		"workers": s.deps.Config.Workers,
		"tools":   map[string]any{"ffprobe": media.DetectFFProbe()},
	})
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/media/") {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/media/")
	parts := strings.Split(rest, "/")
	if len(parts) >= 2 && parts[0] == "transcode-sessions" {
		s.handleTranscodeSession(w, r, parts)
		return
	}
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
	case "stream-options":
		s.handleStreamOptions(w, r, assetID)
	case "transcode-session":
		s.handleTranscodeSessionStart(w, r, assetID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleStreamOptions(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet {
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
	loc, _ := catalog.FirstLocation(asset)
	ffmpegAvailable := false
	if _, err := exec.LookPath("ffmpeg"); err == nil && asset.MediaKind == "video" {
		ffmpegAvailable = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"asset_id":     asset.ID,
		"media_kind":   asset.MediaKind,
		"direct_url":   "/api/v1/media/" + asset.ID + "/original",
		"range":        true,
		"storage":      loc.StorageName,
		"storage_mode": "strict_read_only",
		"options": []map[string]any{
			{
				"id":          "original",
				"label":       "Original/direct",
				"available":   true,
				"url":         "/api/v1/media/" + asset.ID + "/original",
				"description": "Streams the immutable original through Cartolensia with HTTP Range support.",
			},
			{
				"id":               "h264_720p_lan",
				"label":            "Browser-compatible H.264 720p",
				"available":        ffmpegAvailable,
				"profile":          "h264_720p_lan",
				"session_endpoint": "/api/v1/media/" + asset.ID + "/transcode-session",
				"disabled_reason":  disabledReason(!ffmpegAvailable, "ffmpeg is unavailable or asset is not a video"),
			},
			{
				"id":               "h264_low_bitrate",
				"label":            "Low bitrate LAN",
				"available":        ffmpegAvailable,
				"profile":          "h264_low_bitrate",
				"session_endpoint": "/api/v1/media/" + asset.ID + "/transcode-session",
				"disabled_reason":  disabledReason(!ffmpegAvailable, "ffmpeg is unavailable or asset is not a video"),
			},
			{
				"id":              "av1_low_bitrate",
				"label":           "AV1 preview/transcode",
				"available":       false,
				"disabled_reason": "Transcoding jobs are planned; original streaming is active.",
			},
		},
	})
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
	entry, indexErr := s.deps.Store.UpsertPreviewCacheEntry(r.Context(), preview.IndexEntry(asset, info, "default"))
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
	if indexErr == nil {
		_ = s.deps.Store.MarkPreviewAccessed(r.Context(), entry.ID)
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

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func disabledReason(disabled bool, reason string) string {
	if disabled {
		return reason
	}
	return ""
}

func (s *Server) indexingScopeSummary(ctx context.Context, storageName string, prefixes []string) (map[string]any, error) {
	assets, err := s.deps.Store.ListAssets(ctx)
	if err != nil {
		return nil, err
	}
	prefixes = compactStrings(prefixes)
	summary := map[string]any{
		"storage":          storageName,
		"prefixes":         prefixes,
		"assets":           0,
		"photos":           0,
		"videos":           0,
		"tracks":           0,
		"hashed":           0,
		"unhashed":         0,
		"geotagged":        0,
		"preview_ready":    0,
		"total_bytes":      int64(0),
		"track_like_files": 0,
	}
	for _, asset := range assets {
		loc, ok := catalog.FirstLocation(asset)
		if !ok || !assetInScope(loc, storageName, prefixes) {
			continue
		}
		summary["assets"] = summary["assets"].(int) + 1
		summary["total_bytes"] = summary["total_bytes"].(int64) + loc.SizeBytes
		switch asset.MediaKind {
		case "photo":
			summary["photos"] = summary["photos"].(int) + 1
		case "video":
			summary["videos"] = summary["videos"].(int) + 1
		case "track":
			summary["tracks"] = summary["tracks"].(int) + 1
			summary["track_like_files"] = summary["track_like_files"].(int) + 1
		}
		if loc.HashStatus == catalog.HashStatusHashed {
			summary["hashed"] = summary["hashed"].(int) + 1
		} else {
			summary["unhashed"] = summary["unhashed"].(int) + 1
		}
		if _, err := s.deps.Store.GetAssetGeo(ctx, asset.ID); err == nil {
			summary["geotagged"] = summary["geotagged"].(int) + 1
		}
		if asset.MediaKind == "photo" && preview.InfoForAsset(s.deps.Config.Cache.Dir, asset).Status == preview.StatusReady {
			summary["preview_ready"] = summary["preview_ready"].(int) + 1
		}
	}
	return summary, nil
}

func assetInScope(loc catalog.Location, storageName string, prefixes []string) bool {
	if storageName != "" && loc.StorageName != storageName {
		return false
	}
	if len(prefixes) == 0 {
		return true
	}
	relativePath := strings.Trim(strings.TrimSpace(loc.RelativePath), "/")
	for _, prefix := range prefixes {
		prefix = strings.Trim(strings.TrimSpace(prefix), "/")
		if prefix != "" && (relativePath == prefix || strings.HasPrefix(relativePath, prefix+"/")) {
			return true
		}
	}
	return false
}

func latestIndexingJobs(jobsList []jobs.Job, storageName string, prefixes []string) map[string]jobs.Job {
	out := map[string]jobs.Job{}
	kinds := map[string]struct{}{
		"discovery":        {},
		"hash":             {},
		"metadata_enrich":  {},
		"preview_generate": {},
		"geo_snap":         {},
	}
	for _, job := range jobsList {
		if _, ok := kinds[job.Kind]; !ok {
			continue
		}
		if storageName != "" && !jobPayloadMatchesScope(job, storageName, prefixes) {
			continue
		}
		current, exists := out[job.Kind]
		if !exists || job.CreatedAt.After(current.CreatedAt) {
			out[job.Kind] = job
		}
	}
	return out
}

func jobPayloadMatchesScope(job jobs.Job, storageName string, prefixes []string) bool {
	payload := jobPayloadMap(job)
	if payloadStorage, _ := payload["storage"].(string); storageName != "" && payloadStorage != "" && payloadStorage != storageName {
		return false
	}
	if len(prefixes) == 0 {
		return true
	}
	payloadPrefixes := payloadStringSlice(payload["prefixes"])
	if payloadPrefix, _ := payload["prefix"].(string); payloadPrefix != "" {
		payloadPrefixes = append([]string{payloadPrefix}, payloadPrefixes...)
	}
	payloadPrefixes = compactStrings(payloadPrefixes)
	if len(payloadPrefixes) == 0 {
		return false
	}
	for _, wanted := range prefixes {
		wanted = strings.Trim(wanted, "/")
		for _, got := range payloadPrefixes {
			got = strings.Trim(got, "/")
			if wanted == got {
				return true
			}
		}
	}
	return false
}

func jobHasPayloadString(job jobs.Job, key, value string) bool {
	got, _ := jobPayloadMap(job)[key].(string)
	return got == value
}

func jobPayloadMap(job jobs.Job) map[string]any {
	data, err := json.Marshal(job.Payload)
	if err != nil {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func payloadStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func canCancelJob(job jobs.Job) bool {
	return job.Status == jobs.StatusQueued || job.Status == jobs.StatusRunning || job.Status == jobs.StatusCancelRequested
}

type geoCluster struct {
	items  []map[string]any
	sumX   float64
	sumY   float64
	sumLon float64
	sumLat float64
	minLon float64
	minLat float64
	maxLon float64
	maxLat float64
}

func clusterGeoJSONPoints(features []map[string]any, _ bbox, _ bool, zoom int, clusterDistancePx int) []map[string]any {
	if len(features) <= 1 {
		return features
	}
	zoom = minInt(maxInt(zoom, 1), 22)
	if clusterDistancePx <= 0 {
		clusterDistancePx = 48
	}
	var clusters []*geoCluster
	for _, feature := range features {
		lon, lat, ok := geoJSONPoint(feature)
		if !ok {
			continue
		}
		x, y := webMercatorWorldPixel(lon, lat, zoom)
		var selected *geoCluster
		for _, existing := range clusters {
			if distance(x, y, existing.sumX/float64(len(existing.items)), existing.sumY/float64(len(existing.items))) <= float64(clusterDistancePx) {
				selected = existing
				break
			}
		}
		if selected == nil {
			selected = &geoCluster{minLon: lon, maxLon: lon, minLat: lat, maxLat: lat}
			clusters = append(clusters, selected)
		}
		addClusterFeature(selected, feature, lon, lat, x, y)
	}
	mergeClusters(clusters, float64(clusterDistancePx))
	out := make([]map[string]any, 0, len(clusters))
	for i, item := range clusters {
		if len(item.items) == 0 {
			continue
		}
		if len(item.items) == 1 {
			out = append(out, item.items[0])
			continue
		}
		out = append(out, clusterFeature(fmt.Sprintf("cluster:%d:%d", zoom, i), item))
	}
	return out
}

func addClusterFeature(item *geoCluster, feature map[string]any, lon, lat, x, y float64) {
	item.items = append(item.items, feature)
	item.sumX += x
	item.sumY += y
	item.sumLon += lon
	item.sumLat += lat
	item.minLon = minFloat(item.minLon, lon)
	item.maxLon = maxFloat(item.maxLon, lon)
	item.minLat = minFloat(item.minLat, lat)
	item.maxLat = maxFloat(item.maxLat, lat)
}

func mergeClusters(clusters []*geoCluster, threshold float64) {
	for {
		merged := false
		for i := 0; i < len(clusters) && !merged; i++ {
			if len(clusters[i].items) == 0 {
				continue
			}
			for j := i + 1; j < len(clusters); j++ {
				if len(clusters[j].items) == 0 {
					continue
				}
				leftX := clusters[i].sumX / float64(len(clusters[i].items))
				leftY := clusters[i].sumY / float64(len(clusters[i].items))
				rightX := clusters[j].sumX / float64(len(clusters[j].items))
				rightY := clusters[j].sumY / float64(len(clusters[j].items))
				if distance(leftX, leftY, rightX, rightY) > threshold {
					continue
				}
				clusters[i].items = append(clusters[i].items, clusters[j].items...)
				clusters[i].sumLon += clusters[j].sumLon
				clusters[i].sumLat += clusters[j].sumLat
				clusters[i].sumX += clusters[j].sumX
				clusters[i].sumY += clusters[j].sumY
				clusters[i].minLon = minFloat(clusters[i].minLon, clusters[j].minLon)
				clusters[i].maxLon = maxFloat(clusters[i].maxLon, clusters[j].maxLon)
				clusters[i].minLat = minFloat(clusters[i].minLat, clusters[j].minLat)
				clusters[i].maxLat = maxFloat(clusters[i].maxLat, clusters[j].maxLat)
				clusters[j].items = nil
				merged = true
				break
			}
		}
		if !merged {
			break
		}
	}
}

func clusterIdenticalPoints(features []map[string]any) []map[string]any {
	buckets := map[string][]map[string]any{}
	for _, feature := range features {
		lon, lat, ok := geoJSONPoint(feature)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%.7f:%.7f", lon, lat)
		buckets[key] = append(buckets[key], feature)
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		items := buckets[key]
		if len(items) == 1 {
			out = append(out, items[0])
			continue
		}
		lon, lat, _ := geoJSONPoint(items[0])
		out = append(out, map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []float64{lon, lat},
			},
			"properties": map[string]any{
				"kind":           "cluster",
				"cluster_id":     key,
				"clustered":      true,
				"count":          len(items),
				"photos_count":   countClusterKind(items, "photo"),
				"videos_count":   countClusterKind(items, "video"),
				"tracks_count":   countClusterKind(items, "track"),
				"centroid":       map[string]float64{"lon": lon, "lat": lat},
				"bbox":           map[string]float64{"min_lon": lon, "min_lat": lat, "max_lon": lon, "max_lat": lat},
				"sample_assets":  clusterSamples(items),
				"location_label": nil,
			},
		})
	}
	return out
}

func clusterFeature(id string, item *geoCluster) map[string]any {
	count := len(item.items)
	centerLon := item.sumLon / float64(count)
	centerLat := item.sumLat / float64(count)
	return map[string]any{
		"type": "Feature",
		"geometry": map[string]any{
			"type":        "Point",
			"coordinates": []float64{centerLon, centerLat},
		},
		"properties": map[string]any{
			"kind":           "cluster",
			"cluster_id":     id,
			"clustered":      true,
			"count":          count,
			"photos_count":   countClusterKind(item.items, "photo"),
			"videos_count":   countClusterKind(item.items, "video"),
			"tracks_count":   countClusterKind(item.items, "track"),
			"centroid":       map[string]float64{"lon": centerLon, "lat": centerLat},
			"bbox":           map[string]float64{"min_lon": item.minLon, "min_lat": item.minLat, "max_lon": item.maxLon, "max_lat": item.maxLat},
			"bbox_array":     []float64{item.minLon, item.minLat, item.maxLon, item.maxLat},
			"sample_assets":  clusterSamples(item.items),
			"location_label": nil,
		},
	}
}

func webMercatorWorldPixel(lon, lat float64, zoom int) (float64, float64) {
	lat = maxFloat(minFloat(lat, 85.05112878), -85.05112878)
	scale := 256 * math.Pow(2, float64(zoom))
	x := (lon + 180) / 360 * scale
	latRad := lat * math.Pi / 180
	y := (1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * scale
	return x, y
}

func distance(x1, y1, x2, y2 float64) float64 {
	return math.Hypot(x1-x2, y1-y2)
}

func featureExtent(features []map[string]any) bbox {
	out := bbox{MinLon: 180, MinLat: 90, MaxLon: -180, MaxLat: -90}
	for _, feature := range features {
		lon, lat, ok := geoJSONPoint(feature)
		if !ok {
			continue
		}
		out.MinLon = minFloat(out.MinLon, lon)
		out.MaxLon = maxFloat(out.MaxLon, lon)
		out.MinLat = minFloat(out.MinLat, lat)
		out.MaxLat = maxFloat(out.MaxLat, lat)
	}
	if out.MaxLon < out.MinLon || out.MaxLat < out.MinLat {
		return bbox{MinLon: -180, MinLat: -90, MaxLon: 180, MaxLat: 90}
	}
	paddingLon := maxFloat((out.MaxLon-out.MinLon)*0.05, 0.0001)
	paddingLat := maxFloat((out.MaxLat-out.MinLat)*0.05, 0.0001)
	out.MinLon -= paddingLon
	out.MaxLon += paddingLon
	out.MinLat -= paddingLat
	out.MaxLat += paddingLat
	return out
}

func countClusterKind(features []map[string]any, kind string) int {
	count := 0
	for _, feature := range features {
		props, _ := feature["properties"].(map[string]any)
		if props["kind"] == kind || props["asset_type"] == kind {
			count++
		}
	}
	return count
}

func clusterSamples(features []map[string]any) []map[string]any {
	samples := make([]map[string]any, 0, len(features))
	for i, feature := range features {
		if i >= 48 {
			break
		}
		props, _ := feature["properties"].(map[string]any)
		if props == nil {
			continue
		}
		id, _ := props["id"].(string)
		name, _ := props["name"].(string)
		kind, _ := props["kind"].(string)
		previewURL, _ := props["preview_url"].(string)
		detailURL, _ := props["detail_url"].(string)
		originalURL, _ := props["original_url"].(string)
		geometry, _ := feature["geometry"].(map[string]any)
		samples = append(samples, map[string]any{
			"asset_id":     id,
			"name":         name,
			"media_kind":   kind,
			"preview_url":  previewURL,
			"detail_url":   detailURL,
			"original_url": originalURL,
			"coordinates":  geometry["coordinates"],
		})
	}
	return samples
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

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
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
