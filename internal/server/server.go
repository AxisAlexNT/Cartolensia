package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "image/png"

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
	deps                     Dependencies
	mux                      *http.ServeMux
	sessionMu                sync.RWMutex
	geoAlignSessions         map[string]*geoAlignSession
	videoTrackSessions       map[string]*videoTrackPlayerSession
	environmentUsageMu       sync.Mutex
	environmentUsageCache    map[string]any
	environmentUsageCachedAt time.Time
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

type geoAlignMarker struct {
	AssetID         string           `json:"asset_id"`
	Name            string           `json:"name"`
	MediaKind       string           `json:"media_kind"`
	ThumbnailURL    string           `json:"thumbnail_url,omitempty"`
	OriginalLat     *float64         `json:"original_lat,omitempty"`
	OriginalLon     *float64         `json:"original_lon,omitempty"`
	ManualLat       *float64         `json:"manual_lat,omitempty"`
	ManualLon       *float64         `json:"manual_lon,omitempty"`
	StagedLat       float64          `json:"staged_lat"`
	StagedLon       float64          `json:"staged_lon"`
	Status          string           `json:"status"`
	TrackCandidates []map[string]any `json:"track_candidates"`
	Modified        bool             `json:"modified"`
	Metadata        map[string]any   `json:"metadata,omitempty"`
}

type geoAlignSession struct {
	ID        string           `json:"id"`
	AssetIDs  []string         `json:"asset_ids"`
	TrackIDs  []string         `json:"track_ids"`
	Markers   []geoAlignMarker `json:"markers"`
	BBox      catalog.BBox     `json:"bbox"`
	ReadOnly  bool             `json:"read_only"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type videoTrackPlayerSession struct {
	ID            string         `json:"id"`
	VideoAssetID  string         `json:"video_asset_id"`
	TrackIDs      []string       `json:"track_ids"`
	TimestampMode string         `json:"timestamp_mode"`
	OffsetSeconds float64        `json:"offset_seconds"`
	VideoStartAt  *time.Time     `json:"video_start_at,omitempty"`
	TimeSource    string         `json:"time_source,omitempty"`
	Warnings      []string       `json:"warnings,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

func New(deps Dependencies) *Server {
	if deps.Authenticator == nil {
		deps.Authenticator = auth.DevNoAuth{}
	}
	if deps.Authorizer == nil {
		deps.Authorizer = auth.DevNoAuth{}
	}
	s := &Server{
		deps:               deps,
		mux:                http.NewServeMux(),
		geoAlignSessions:   map[string]*geoAlignSession{},
		videoTrackSessions: map[string]*videoTrackPlayerSession{},
	}
	s.seedDefaultPlaces()
	s.seedDefaultComponents()
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.publicRequest(r) {
		if _, err := s.deps.Authenticator.Authenticate(r); err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) publicRequest(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}
	if s.deps.AuthService == nil {
		return true
	}
	switch {
	case r.URL.Path == "/api/v1/health",
		r.URL.Path == "/api/v1/version",
		r.URL.Path == "/api/v1/diagnostics/readiness",
		r.URL.Path == "/api/v1/auth/me",
		r.URL.Path == "/api/v1/auth/csrf",
		r.URL.Path == "/api/v1/auth/login":
		return true
	case strings.HasPrefix(r.URL.Path, "/api/v1/public/"):
		return true
	case strings.HasPrefix(r.URL.Path, "/api/v1/ai-media/") && s.aiMediaRequestAuthorized(r):
		return true
	case s.publicMediaRequest(r):
		return true
	default:
		return false
	}
}

func (s *Server) publicMediaRequest(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/media/")
	if rest == r.URL.Path {
		return false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return false
	}
	switch parts[1] {
	case "original", "preview", "track-preview", "track-thumbnail":
	default:
		return false
	}
	asset, err := s.deps.Store.GetAsset(r.Context(), parts[0])
	return err == nil && assetIsPublic(asset)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/v1/health", s.handleHealth)
	s.mux.HandleFunc("/api/v1/version", s.handleVersion)
	s.mux.HandleFunc("/api/v1/diagnostics/readiness", s.handleReadiness)
	s.mux.HandleFunc("/api/v1/config/effective", s.handleConfig)
	s.mux.HandleFunc("/api/v1/settings", s.handleSettings)
	s.mux.HandleFunc("/api/v1/settings/schema", s.handleSettingsSchema)
	s.mux.HandleFunc("/api/v1/settings/effective", s.handleSettingsEffective)
	s.mux.HandleFunc("/api/v1/settings/runtime", s.handleSettingsRuntime)
	s.mux.HandleFunc("/api/v1/settings/pending/download", s.handleSettingsPendingDownload)
	s.mux.HandleFunc("/api/v1/settings/pending", s.handleSettingsPending)
	s.mux.HandleFunc("/api/v1/settings/restart-required", s.handleSettingsRestartRequired)
	s.mux.HandleFunc("/api/v1/files/browse", s.handleFileBrowse)
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
	s.mux.HandleFunc("/api/v1/ai-media/", s.handleAIMedia)
	s.mux.HandleFunc("/api/v1/storages", s.handleStorages)
	s.mux.HandleFunc("/api/v1/storages/", s.handleStorageByName)
	s.mux.HandleFunc("/api/v1/components/status", s.handleComponentsStatus)
	s.mux.HandleFunc("/api/v1/components", s.handleComponents)
	s.mux.HandleFunc("/api/v1/components/", s.handleComponentByKey)
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
	s.mux.HandleFunc("/api/v1/public/assets", s.handlePublicAssets)
	s.mux.HandleFunc("/api/v1/public/assets/", s.handlePublicAssetByID)
	s.mux.HandleFunc("/api/v1/search/parse", s.handleSearchParse)
	s.mux.HandleFunc("/api/v1/search/plan", s.handleSearchPlan)
	s.mux.HandleFunc("/api/v1/search/sql", s.handleSearchSQL)
	s.mux.HandleFunc("/api/v1/search", s.handleSearch)
	s.mux.HandleFunc("/api/v1/knowledge/facts", s.handleKnowledgeFacts)
	s.mux.HandleFunc("/api/v1/knowledge/relations", s.handleKnowledgeRelations)
	s.mux.HandleFunc("/api/v1/knowledge/extract", s.handleKnowledgeExtract)
	s.mux.HandleFunc("/api/v1/knowledge/chat", s.handleKnowledgeChat)
	s.mux.HandleFunc("/api/v1/knowledge/chat/stream", s.handleKnowledgeChatStream)
	s.mux.HandleFunc("/api/v1/knowledge/llm/status", s.handleKnowledgeLLMStatus)
	s.mux.HandleFunc("/api/v1/search/places", s.handleSearchPlaces)
	s.mux.HandleFunc("/api/v1/places/hierarchy", s.handlePlaceHierarchy)
	s.mux.HandleFunc("/api/v1/places/providers", s.handlePlaceProviders)
	s.mux.HandleFunc("/api/v1/places/reverse-geocode/start", s.handleReverseGeocodeStart)
	s.mux.HandleFunc("/api/v1/places", s.handlePlaces)
	s.mux.HandleFunc("/api/v1/places/reverse", s.handlePlaceReverse)
	s.mux.HandleFunc("/api/v1/places/", s.handlePlaceByID)
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
	s.mux.HandleFunc("/api/v1/audio/analyze/start", s.handleAudioAnalyzeStart)
	s.mux.HandleFunc("/api/v1/audio/", s.handleAudioByID)
	s.mux.HandleFunc("/api/v1/transcripts", s.handleTranscripts)
	s.mux.HandleFunc("/api/v1/transcripts/", s.handleTranscriptByID)
	s.mux.HandleFunc("/api/v1/geo-align/session", s.handleGeoAlignSessionCreate)
	s.mux.HandleFunc("/api/v1/geo-align/sessions/", s.handleGeoAlignSessionByID)
	s.mux.HandleFunc("/api/v1/video-track-player/session", s.handleVideoTrackPlayerSessionCreate)
	s.mux.HandleFunc("/api/v1/video-track-player/sessions/", s.handleVideoTrackPlayerSessionByID)
	s.mux.HandleFunc("/api/v1/ocr/runs", s.handleOCRRuns)
	s.mux.HandleFunc("/api/v1/map/tracks/render-cache/status", s.handleMapTrackRenderCacheStatus)
	s.mux.HandleFunc("/api/v1/map/tracks/render-cache/refresh", s.handleMapTrackRenderCacheRefresh)
	s.mux.HandleFunc("/api/v1/map", s.handleMap)
	s.mux.HandleFunc("/api/v1/map/", s.handleMapSubroute)
	s.mux.HandleFunc("/api/v1/transcoding/status", s.handleTranscodingStatus)
	s.mux.HandleFunc("/api/v1/transcoding/capabilities", s.handleTranscodingCapabilities)
	s.mux.HandleFunc("/api/v1/transcoding/metrics/status", s.handleTranscodingMetricsStatus)
	s.mux.HandleFunc("/api/v1/transcoding/hardware-test", s.handleTranscodingHardwareTest)
	s.mux.HandleFunc("/api/v1/transcoding/presets", s.handleTranscodingPresets)
	s.mux.HandleFunc("/api/v1/transcoding/presets/", s.handleTranscodingPresetByID)
	s.mux.HandleFunc("/api/v1/ai/status", s.handleAIStatus)
	s.mux.HandleFunc("/api/v1/ai/accelerators", s.handleAIAccelerators)
	s.mux.HandleFunc("/api/v1/ai/workers", s.handleAIWorkers)
	s.mux.HandleFunc("/api/v1/ai/workers/", s.handleAIWorkerByID)
	s.mux.HandleFunc("/api/v1/ai/summary", s.handleAISummary)
	s.mux.HandleFunc("/api/v1/ai/tags", s.handleAITags)
	s.mux.HandleFunc("/api/v1/ai/faces", s.handleAIFaces)
	s.mux.HandleFunc("/api/v1/ai/safety", s.handleAISafety)
	s.mux.HandleFunc("/api/v1/ai/backfill/start", s.handleAIBackfillStart)
	s.mux.HandleFunc("/api/v1/ai/jobs/classify", s.handleAIJobRequest("classify_image"))
	s.mux.HandleFunc("/api/v1/ai/jobs/faces", s.handleAIJobRequest("detect_faces"))
	s.mux.HandleFunc("/api/v1/ai/jobs/safety", s.handleAIJobRequest("safety_nsfw"))
	s.mux.HandleFunc("/api/v1/ai/jobs/embed", s.handleAIJobRequest("embed_image"))
	s.mux.HandleFunc("/api/v1/ai/jobs/describe", s.handleAIJobRequest("describe_image"))
	s.mux.HandleFunc("/api/v1/ai/jobs/ocr", s.handleAIJobRequest("ocr_image"))
	s.mux.HandleFunc("/api/v1/ai/jobs/transcribe", s.handleAIJobRequest("transcribe_audio"))
	s.mux.HandleFunc("/api/v1/ai/jobs/audio-analyze", s.handleAIJobRequest("analyze_audio"))
	s.mux.HandleFunc("/api/v1/ai/predictions", s.handleAIPredictions)
	s.mux.HandleFunc("/api/v1/ai/safety/", s.handleAISafetyByAsset)
	s.mux.HandleFunc("/api/v1/faces/clusters", s.handleFaceClusters)
	s.mux.HandleFunc("/api/v1/faces/clusters/", s.handleFaceClusterByID)
	s.mux.HandleFunc("/api/v1/faces/detections", s.handleFaceDetections)
	s.mux.HandleFunc("/api/v1/faces/detections/", s.handleFaceDetectionByID)
	s.mux.HandleFunc("/api/v1/vector/status", s.handleVectorStatus)
	s.mux.HandleFunc("/api/v1/search/vector", s.handleVectorSearch)
	s.mux.HandleFunc("/api/v1/stats", s.handleStats)
	s.mux.HandleFunc("/api/v1/environment/usage", s.handleEnvironmentUsage)
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
		if errors.Is(err, auth.ErrUnauthenticated) {
			writeJSON(w, http.StatusOK, map[string]any{"principal": nil, "auth": s.authStatus()})
			return
		}
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
	req.Email = strings.TrimSpace(req.Email)
	req.Password = strings.TrimRight(req.Password, "\r\n")
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
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.storageStatuses())
	case http.MethodPost:
		if !s.requireWrite(w, r, "storages.create") {
			return
		}
		var req storageMutationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cfg, warnings, err := validateStorageMutation(req.Config(), "")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.ValidateOnly {
			writeJSON(w, http.StatusOK, map[string]any{"valid": true, "storage": cfg, "warnings": warnings, "diagnostic": diagnoseStorageRoot(cfg, 750*time.Millisecond)})
			return
		}
		added, err := s.deps.Registry.AddStorage(cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"storage": added, "warnings": warnings, "active": true})
	default:
		methodNotAllowed(w)
	}
}

type storageStatus struct {
	storage.Config
	Health        string         `json:"health"`
	HealthCode    string         `json:"health_code,omitempty"`
	HealthMessage string         `json:"health_message,omitempty"`
	LastCheckedAt string         `json:"last_checked_at"`
	Details       map[string]any `json:"details,omitempty"`
}

func (s *Server) storageStatuses() []storageStatus {
	configs := s.deps.Registry.ListStorages()
	out := make([]storageStatus, 0, len(configs))
	now := time.Now().UTC().Format(time.RFC3339)
	for _, cfg := range configs {
		diag := diagnoseStorageRoot(cfg, 750*time.Millisecond)
		status := storageStatus{
			Config:        cfg,
			Health:        diag.Health,
			HealthCode:    diag.Code,
			HealthMessage: diag.Message,
			LastCheckedAt: now,
			Details:       diag.Details,
		}
		out = append(out, status)
	}
	return out
}

type storageDiagnostic struct {
	Health  string
	Code    string
	Message string
	Details map[string]any
}

func diagnoseStorageRoot(cfg storage.Config, timeout time.Duration) storageDiagnostic {
	smb := effectiveSMBConfig(cfg)
	details := map[string]any{
		"name":       cfg.Name,
		"root":       cfg.Root,
		"kind":       cfg.Kind,
		"mode":       cfg.Mode,
		"source_url": cfg.SourceURL,
	}
	if smb != nil {
		details["smb_host"] = smb.Host
		details["smb_share"] = smb.Share
		details["smb_path"] = smb.Path
		details["smb_credentials_configured"] = smb.CredentialsFile != "" || smb.PasswordEnv != ""
		if smb.CredentialsFile != "" {
			details["smb_credentials_file"] = smb.CredentialsFile
		}
	}

	rootErr := probeReadableDirBounded(cfg.Root, timeout)
	if rootErr == nil {
		return storageDiagnostic{Health: "available", Code: "available", Message: "storage root is readable", Details: details}
	}
	details["root_error"] = rootErr.Error()

	if smb == nil || smb.Host == "" {
		return classifyFilesystemStorageError(rootErr, details)
	}

	hostCode, hostMsg := probeSMBHost(smb.Host, timeout)
	details["smb_host_status"] = hostCode
	if hostCode != "host_reachable" {
		return storageDiagnostic{Health: "unavailable", Code: hostCode, Message: hostMsg, Details: details}
	}

	if shareDiag, ok := probeSMBShareWithClient(*smb, 2*time.Second); ok {
		for key, value := range shareDiag.Details {
			details[key] = value
		}
		if shareDiag.Code != "share_available" {
			shareDiag.Details = details
			return shareDiag
		}
	}

	code, message := classifyMountBackedStorageError(rootErr)
	return storageDiagnostic{Health: "unavailable", Code: code, Message: message, Details: details}
}

func classifyFilesystemStorageError(err error, details map[string]any) storageDiagnostic {
	switch {
	case errors.Is(err, os.ErrPermission):
		return storageDiagnostic{Health: "unavailable", Code: "permission_denied", Message: "storage root exists but Cartolensia cannot read it", Details: details}
	case errors.Is(err, os.ErrNotExist):
		return storageDiagnostic{Health: "unavailable", Code: "path_missing", Message: "storage root path does not exist", Details: details}
	case strings.Contains(strings.ToLower(err.Error()), "timed out"):
		return storageDiagnostic{Health: "unavailable", Code: "probe_timeout", Message: "storage root probe timed out", Details: details}
	default:
		return storageDiagnostic{Health: "unavailable", Code: "unavailable", Message: "storage root is unavailable: " + err.Error(), Details: details}
	}
}

func classifyMountBackedStorageError(err error) (string, string) {
	lower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, os.ErrPermission) || strings.Contains(lower, "permission denied"):
		return "credentials_or_permission_denied", "SMB host is reachable, but the mounted export is not readable; check Samba credentials and share ACLs"
	case errors.Is(err, os.ErrNotExist):
		return "export_or_path_unavailable", "SMB host is reachable, but the configured export/path is not mounted or does not exist"
	case strings.Contains(lower, "no such device"), strings.Contains(lower, "not connected"), strings.Contains(lower, "host is down"):
		return "export_or_mount_unavailable", "SMB host is reachable, but the local CIFS mount/export is unavailable"
	case strings.Contains(lower, "timed out"):
		return "export_or_mount_timeout", "SMB host is reachable, but the local CIFS mount/export did not become available before the timeout"
	default:
		return "export_or_mount_unavailable", "SMB host is reachable, but the local storage root is unavailable: " + err.Error()
	}
}

func effectiveSMBConfig(cfg storage.Config) *storage.SMBConfig {
	if cfg.SMB != nil && cfg.SMB.Host != "" {
		out := *cfg.SMB
		return &out
	}
	if cfg.SourceURL == "" {
		return cfg.SMB
	}
	u, err := url.Parse(cfg.SourceURL)
	if err != nil || (u.Scheme != "smb" && u.Scheme != "cifs") {
		return cfg.SMB
	}
	out := storage.SMBConfig{Host: u.Hostname()}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		if share, err := url.PathUnescape(parts[0]); err == nil {
			out.Share = share
		}
	}
	if len(parts) > 1 {
		decoded := make([]string, 0, len(parts)-1)
		for _, part := range parts[1:] {
			if value, err := url.PathUnescape(part); err == nil {
				decoded = append(decoded, value)
			}
		}
		out.Path = strings.Join(decoded, "/")
	}
	if cfg.SMB != nil {
		if cfg.SMB.Share != "" {
			out.Share = cfg.SMB.Share
		}
		if cfg.SMB.Path != "" {
			out.Path = cfg.SMB.Path
		}
		out.Domain = cfg.SMB.Domain
		out.Username = cfg.SMB.Username
		out.CredentialsFile = cfg.SMB.CredentialsFile
		out.PasswordEnv = cfg.SMB.PasswordEnv
	}
	return &out
}

func probeSMBHost(host string, timeout time.Duration) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, "445"))
	if err == nil {
		_ = conn.Close()
		return "host_reachable", "SMB host is reachable on TCP 445"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "host_unresolved", "SMB host cannot be resolved: " + err.Error()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "host_offline", "SMB host did not respond on TCP 445 before timeout"
	}
	return "host_offline", "SMB host is not reachable on TCP 445: " + err.Error()
}

func probeSMBShareWithClient(smb storage.SMBConfig, timeout time.Duration) (storageDiagnostic, bool) {
	if smb.Host == "" || smb.Share == "" || smb.CredentialsFile == "" {
		return storageDiagnostic{}, false
	}
	if info, err := os.Stat(smb.CredentialsFile); err != nil {
		code := "credentials_file_unreadable"
		message := "SMB credentials file is configured, but Cartolensia cannot read it"
		if errors.Is(err, os.ErrNotExist) {
			code = "credentials_file_missing"
			message = "SMB credentials file is configured, but the file does not exist"
		} else if errors.Is(err, os.ErrPermission) {
			message = "SMB credentials file exists, but Cartolensia does not have permission to read it"
		}
		return storageDiagnostic{
			Health:  "unavailable",
			Code:    code,
			Message: message,
			Details: map[string]any{
				"smbclient_checked":        false,
				"smb_credentials_file":     smb.CredentialsFile,
				"smb_credentials_file_err": err.Error(),
			},
		}, true
	} else if info.IsDir() {
		return storageDiagnostic{
			Health:  "unavailable",
			Code:    "credentials_file_unreadable",
			Message: "SMB credentials path points to a directory, not a credentials file",
			Details: map[string]any{
				"smbclient_checked":    false,
				"smb_credentials_file": smb.CredentialsFile,
			},
		}, true
	}
	bin, err := exec.LookPath("smbclient")
	if err != nil {
		return storageDiagnostic{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	unc := "//" + smb.Host + "/" + smb.Share
	args := []string{unc, "-A", smb.CredentialsFile, "-m", "SMB3", "-t", strconv.Itoa(max(1, int(timeout.Seconds()))), "-c", "pwd"}
	cmd := exec.CommandContext(ctx, bin, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	details := map[string]any{"smbclient_checked": true}
	if text != "" {
		details["smbclient_output"] = redactSMBProbeOutput(text)
	}
	if err == nil {
		return storageDiagnostic{Health: "unavailable", Code: "share_available", Message: "SMB share accepted the configured credentials; local mount/path is still unavailable", Details: details}, true
	}
	lower := strings.ToLower(text + " " + err.Error())
	switch {
	case strings.Contains(lower, "logon_failure"), strings.Contains(lower, "access_denied"), strings.Contains(lower, "session setup failed"):
		return storageDiagnostic{Health: "unavailable", Code: "credentials_invalid", Message: "SMB host is reachable, but configured credentials were rejected", Details: details}, true
	case strings.Contains(lower, "bad_network_name"), strings.Contains(lower, "tree connect failed"), strings.Contains(lower, "object_name_not_found"):
		return storageDiagnostic{Health: "unavailable", Code: "export_unavailable", Message: "SMB host is reachable, but the configured share/export is not available", Details: details}, true
	case strings.Contains(lower, "could not resolve"), strings.Contains(lower, "name or service not known"):
		return storageDiagnostic{Health: "unavailable", Code: "host_unresolved", Message: "SMB host cannot be resolved by smbclient", Details: details}, true
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "connection refused"), strings.Contains(lower, "no route to host"):
		return storageDiagnostic{Health: "unavailable", Code: "host_offline", Message: "SMB host is not reachable by smbclient", Details: details}, true
	default:
		return storageDiagnostic{Health: "unavailable", Code: "smb_probe_failed", Message: "SMB probe failed: " + firstNonEmpty(text, err.Error()), Details: details}, true
	}
}

func redactSMBProbeOutput(value string) string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "password") {
			out = append(out, "[redacted password line]")
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func (s *Server) handleStorageByName(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/storages/"), "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	name, err := url.PathUnescape(parts[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(parts) == 2 && parts[1] == "validate" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		cfg, err := s.deps.Registry.GetStorage(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		validated, warnings, err := validateStorageMutation(cfg, name)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"valid": false, "storage": cfg, "error": err.Error(), "warnings": warnings})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"valid": true, "storage": validated, "warnings": warnings, "diagnostic": diagnoseStorageRoot(validated, 750*time.Millisecond)})
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.deps.Registry.GetStorage(name)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPatch:
		if !s.requireWrite(w, r, "storages.update") {
			return
		}
		var req storageMutationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cfg := req.Config()
		cfg.Name = name
		updated, warnings, err := validateStorageMutation(cfg, name)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.ValidateOnly {
			writeJSON(w, http.StatusOK, map[string]any{"valid": true, "storage": updated, "warnings": warnings, "diagnostic": diagnoseStorageRoot(updated, 750*time.Millisecond)})
			return
		}
		applied, err := s.deps.Registry.UpdateStorage(name, updated)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"storage": applied, "warnings": warnings, "active": true})
	default:
		methodNotAllowed(w)
	}
}

type storageMutationRequest struct {
	Name         string             `json:"name"`
	Kind         string             `json:"kind"`
	Root         string             `json:"root"`
	Mode         string             `json:"mode"`
	SourceURL    string             `json:"source_url"`
	SMB          *storage.SMBConfig `json:"smb"`
	ValidateOnly bool               `json:"validate_only"`
}

func (r storageMutationRequest) Config() storage.Config {
	return storage.Config{Name: r.Name, Kind: r.Kind, Root: r.Root, Mode: r.Mode, SourceURL: r.SourceURL, SMB: r.SMB}
}

func validateStorageMutation(cfg storage.Config, currentName string) (storage.Config, []string, error) {
	warnings := []string{}
	if strings.TrimSpace(cfg.Name) == "" && currentName != "" {
		cfg.Name = currentName
	}
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = "strict_read_only"
	}
	if cfg.Mode == "journaled_deferred" || cfg.Mode == "read_write" {
		return storage.Config{}, warnings, fmt.Errorf("storage mode %q is not enabled; originals remain immutable in this build", cfg.Mode)
	}
	normalized, err := storage.ValidateConfig(cfg)
	if err != nil {
		return storage.Config{}, warnings, err
	}
	if storagePathIsRealArchive(normalized.Root) && normalized.Mode != "strict_read_only" {
		return storage.Config{}, warnings, fmt.Errorf("real archive storage rooted at /mnt/Models/rclone must remain strict_read_only")
	}
	if normalized.Mode == "read_only" {
		warnings = append(warnings, "read_only mode still exposes no write/delete/move operations through Cartolensia")
	}
	if storagePathIsRealArchive(normalized.Root) {
		warnings = append(warnings, "real archive storage is locked to strict_read_only")
	}
	return normalized, warnings, nil
}

func storagePathIsRealArchive(root string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	archive := "/mnt/Models/rclone"
	absArchive, err := filepath.Abs(archive)
	if err == nil {
		archive = absArchive
	}
	rel, err := filepath.Rel(archive, absRoot)
	if err != nil {
		return absRoot == archive
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
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
	if len(parts) == 3 && parts[1] == "settings" && parts[2] == "schema" {
		s.handlePluginSettingsSchema(w, r, manifest.ID)
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
	if !truthyQuery(r.URL.Query().Get("full_payload")) {
		jobsList = summarizeJobsForList(jobsList)
	}
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
		runner := s.discoveryRunner()
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

func (s *Server) discoveryRunner() discovery.Runner {
	leaseDuration, err := time.ParseDuration(s.deps.Config.Workers.LeaseDuration)
	if err != nil || leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	return discovery.Runner{
		Registry:         s.deps.Registry,
		Store:            s.deps.Store,
		WorkerID:         s.deps.Config.Workers.WorkerID,
		LeaseDuration:    leaseDuration,
		MaxFolderWorkers: runtimeIntSetting("discovery.max_folder_workers", 4),
		MaxFileWorkers:   runtimeIntSetting("discovery.max_file_workers", 8),
		FolderQueueDepth: runtimeIntSetting("discovery.folder_queue_depth", 64),
	}
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
		runner := s.discoveryRunner()
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

func (s *Server) handleAudioAnalyzeStart(w http.ResponseWriter, r *http.Request) {
	s.handleAIJobRequestWithDefault("analyze_audio", func(req *aiJobRequest) {
		if req.Scope == "" && req.AssetID == "" && len(req.AssetIDs) == 0 {
			req.Scope = "current"
		}
		if req.Limit <= 0 {
			req.Limit = 50
		}
	})(w, r)
}

func (s *Server) handleAudioByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/audio/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[1] != "metadata" {
		http.NotFound(w, r)
		return
	}
	asset, err := s.deps.Store.GetAsset(r.Context(), parts[0])
	if errors.Is(err, catalog.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if asset.MediaKind != "audio" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("asset %s is %s, not audio", asset.ID, asset.MediaKind))
		return
	}
	features, err := s.deps.Store.GetAudioFeatures(r.Context(), asset.ID)
	if errors.Is(err, catalog.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"asset": asset, "features": nil, "status": "features_missing"})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"asset": asset, "features": features, "status": "ok"})
}

func (s *Server) handleTranscripts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := intQuery(r.URL.Query(), "limit", 50, 1, 200)
	offset := intQuery(r.URL.Query(), "offset", 0, 0, 100000)
	transcripts, err := s.deps.Store.ListAllTranscripts(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"transcripts": transcripts,
		"limit":       limit,
		"offset":      offset,
		"total":       len(transcripts),
		"note":        "Transcripts are Cartolensia metadata only; no ASR sidecars are written beside originals.",
	})
}

func (s *Server) handleTranscriptByID(w http.ResponseWriter, r *http.Request) {
	transcriptID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/transcripts/"), "/")
	if transcriptID == "" {
		writeError(w, http.StatusNotFound, errors.New("transcript id is required"))
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if !s.requireWrite(w, r, "transcripts.write") {
			return
		}
		if err := s.deps.Store.DeleteTranscript(r.Context(), transcriptID); errors.Is(err, catalog.ErrNotFound) {
			http.NotFound(w, r)
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": transcriptID})
	default:
		methodNotAllowed(w)
	}
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

func assetIsPublic(asset catalog.Asset) bool {
	for _, key := range []string{"public", "is_public", "visibility_public"} {
		if value, ok := asset.Metadata[key].(bool); ok {
			return value
		}
	}
	if value, ok := asset.Metadata["visibility"].(string); ok {
		return strings.EqualFold(value, "public")
	}
	return false
}

func (s *Server) handlePublicAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 60, 200)
	offset := queryInt(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	page, err := s.deps.Store.QueryAssets(r.Context(), catalog.AssetQuery{
		PublicOnly: true,
		Limit:      limit,
		Offset:     offset,
		Sort:       firstNonEmpty(r.URL.Query().Get("sort"), "taken_at"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(page.Page.Total))
	writeJSON(w, http.StatusOK, page.Assets)
}

func (s *Server) handlePublicAssetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	assetID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/public/assets/"), "/")
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
	if !assetIsPublic(asset) {
		writeError(w, http.StatusNotFound, catalog.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, s.assetDetail(r.Context(), asset))
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	plan := s.buildSearchPlan(raw)
	tokens := plan.Tokens
	if fastPage, ok, err := s.queryFastSearchAssets(r.Context(), tokens, limit, offset); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	} else if ok {
		searchCtx := assetSearchContext{}
		backend := s.searchBackend()
		type result struct {
			Asset       catalog.Asset `json:"asset"`
			Matched     []string      `json:"matched"`
			Explanation string        `json:"explanation"`
		}
		results := make([]result, 0, len(fastPage.Assets))
		for _, asset := range fastPage.Assets {
			matched := assetSearchMatches(asset, tokens, searchCtx)
			if len(tokens) > 0 && len(matched) == 0 {
				matched = []string{"asset filter"}
			}
			results = append(results, result{Asset: asset, Matched: matched, Explanation: searchExplanation(matched)})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"query":        raw,
			"tokens":       tokens,
			"plan":         plan,
			"plan_preview": searchPlanPreview(plan),
			"backend":      backend.ID(),
			"backend_mode": backend.Mode(),
			"results":      results,
			"tracks":       []any{},
			"places":       []any{},
			"warnings":     searchWarnings(tokens),
			"page":         fastPage.Page,
		})
		return
	}
	page, err := s.queryAllSearchAssets(r.Context(), tokens)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	searchCtx := s.buildSearchContext(r.Context(), tokens)
	backend := s.searchBackend()
	type result struct {
		Asset       catalog.Asset `json:"asset"`
		Matched     []string      `json:"matched"`
		Explanation string        `json:"explanation"`
	}
	type trackResult struct {
		Track       catalog.TrackSummary `json:"track"`
		Matched     []string             `json:"matched"`
		Explanation string               `json:"explanation"`
	}
	all := make([]result, 0, len(page.Assets))
	for _, asset := range page.Assets {
		matched := assetSearchMatches(asset, tokens, searchCtx)
		if len(tokens) > 0 && len(matched) == 0 {
			continue
		}
		all = append(all, result{Asset: asset, Matched: matched, Explanation: searchExplanation(matched)})
	}
	trackMatches := []trackResult{}
	if tracks, err := s.deps.Store.ListGPSTracks(r.Context(), catalog.GPSTrackQuery{Limit: 1000}); err == nil {
		for _, track := range tracks {
			matched := trackSearchMatches(track, tokens, searchCtx.placeEntries)
			if len(tokens) > 0 && len(matched) == 0 {
				continue
			}
			trackMatches = append(trackMatches, trackResult{Track: track, Matched: matched, Explanation: searchExplanation(matched)})
		}
	}
	total := len(all)
	if offset >= len(all) {
		all = []result{}
	} else {
		end := offset + limit
		if end > len(all) {
			end = len(all)
		}
		all = all[offset:end]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"query":        raw,
		"tokens":       tokens,
		"plan":         plan,
		"plan_preview": searchPlanPreview(plan),
		"backend":      backend.ID(),
		"backend_mode": backend.Mode(),
		"results":      all,
		"tracks":       trackMatches,
		"places":       searchCtx.places,
		"warnings":     searchWarnings(tokens),
		"page":         catalog.Page{Limit: limit, Offset: offset, Total: total},
	})
}

func (s *Server) queryFastSearchAssets(ctx context.Context, tokens []string, limit, offset int) (catalog.AssetPage, bool, error) {
	if len(tokens) != 1 {
		return catalog.AssetPage{}, false, nil
	}
	prefix, plain := splitSearchToken(tokens[0])
	plain = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(plain), "."))
	if plain == "" {
		return catalog.AssetPage{}, false, nil
	}
	query := catalog.AssetQuery{Limit: limit, Offset: offset, Sort: "taken_at"}
	switch prefix {
	case "ext", "extension":
		query.Extension = plain
	case "kind", "type", "media", "media_kind":
		query.MediaKind = plain
	case "":
		if !isSupportedExtensionSearchToken(plain) {
			return catalog.AssetPage{}, false, nil
		}
		query.Extension = plain
	default:
		return catalog.AssetPage{}, false, nil
	}
	page, err := s.deps.Store.QueryAssets(ctx, query)
	return page, true, err
}

func isSupportedExtensionSearchToken(token string) bool {
	token = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(token), "."))
	for _, ext := range storage.SupportedExtensions() {
		if token == ext {
			return true
		}
	}
	return false
}

func (s *Server) queryAllSearchAssets(ctx context.Context, tokens []string) (catalog.AssetPage, error) {
	base := catalog.AssetQuery{Limit: 500, Sort: "taken_at"}
	if len(tokens) == 1 {
		prefix, plain := splitSearchToken(tokens[0])
		switch prefix {
		case "ext", "extension":
			base.Extension = plain
		case "kind", "type", "media":
			base.MediaKind = plain
		case "filename", "path":
			base.Q = wildcardToLikeText(plain)
		}
	}
	all := []catalog.Asset{}
	total := 0
	for offset := 0; ; offset += 500 {
		query := base
		query.Offset = offset
		page, err := s.deps.Store.QueryAssets(ctx, query)
		if err != nil {
			return catalog.AssetPage{}, err
		}
		total = page.Page.Total
		all = append(all, page.Assets...)
		if len(page.Assets) == 0 || offset+len(page.Assets) >= page.Page.Total || offset > 100000 {
			break
		}
	}
	return catalog.AssetPage{Assets: all, Page: catalog.Page{Limit: len(all), Offset: 0, Total: total}}, nil
}

func splitSearchToken(token string) (string, string) {
	token = strings.TrimSpace(strings.ToLower(token))
	if before, after, ok := strings.Cut(token, ":"); ok {
		return before, strings.TrimSpace(after)
	}
	return "", token
}

func wildcardToLikeText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	value = strings.ReplaceAll(value, "*", "")
	value = strings.ReplaceAll(value, "?", "")
	return value
}

func (s *Server) handleSearchPlaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	entries := s.placeEntries(r.Context())
	places := make([]searchPlaceMatch, 0, len(entries))
	for _, place := range entries {
		matchedAssets := 0
		geos, err := s.deps.Store.QueryAssetGeo(r.Context(), catalog.GeoQuery{BBox: &place.BBox, Limit: 10000})
		if err == nil {
			seen := map[string]struct{}{}
			for _, geo := range geos {
				seen[geo.Asset.ID] = struct{}{}
			}
			matchedAssets = len(seen)
		}
		places = append(places, searchPlaceMatch{
			Query:         place.NormalizedName,
			Name:          place.Name,
			DisplayName:   place.DisplayName,
			Provider:      place.Provider,
			Source:        place.Source,
			Lat:           place.Lat,
			Lon:           place.Lon,
			BBox:          place.BBox,
			MatchedAssets: matchedAssets,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"backend":        "postgres_local",
		"mode":           "cache_only",
		"online_enabled": false,
		"provider":       "local_place_cache",
		"places":         places,
		"note":           "Place search is cache-only. No online geocoder is called by this endpoint.",
	})
}

func (s *Server) assetRelated(ctx context.Context, asset catalog.Asset) (map[string]any, error) {
	page, err := s.queryAllSearchAssets(ctx, nil)
	if err != nil {
		return nil, err
	}
	type relatedItem struct {
		Asset   catalog.Asset  `json:"asset"`
		Reason  string         `json:"reason"`
		Score   float64        `json:"score"`
		Details map[string]any `json:"details,omitempty"`
	}
	groups := map[string][]relatedItem{
		"same_folder": {},
		"same_device": {},
		"same_day":    {},
		"time_window": {},
		"same_track":  {},
	}
	add := func(group string, other catalog.Asset, reason string, score float64, details map[string]any) {
		if other.ID == "" || other.ID == asset.ID {
			return
		}
		for _, existing := range groups[group] {
			if existing.Asset.ID == other.ID {
				return
			}
		}
		groups[group] = append(groups[group], relatedItem{Asset: other, Reason: reason, Score: score, Details: details})
	}
	assetFolder := firstAssetFolder(asset)
	assetDevice := deviceSignature(asset)
	assetTime, hasAssetTime := catalog.AssetPrimaryTimestamp(asset, time.Local)
	for _, other := range page.Assets {
		if assetFolder != "" && firstAssetFolder(other) == assetFolder {
			add("same_folder", other, "same storage folder", 0.65, map[string]any{"folder": assetFolder})
		}
		if assetDevice != "" && deviceSignature(other) == assetDevice {
			add("same_device", other, "same camera/device metadata", 0.7, map[string]any{"device": assetDevice})
		}
		if hasAssetTime {
			if otherTime, ok := catalog.AssetPrimaryTimestamp(other, time.Local); ok {
				if sameLocalDay(assetTime.Time, otherTime.Time) {
					add("same_day", other, "same local calendar day", 0.55, map[string]any{"time_source": otherTime.Source})
				}
				diff := otherTime.Time.Sub(assetTime.Time)
				if diff < 0 {
					diff = -diff
				}
				if diff <= 30*time.Minute {
					add("time_window", other, "created within 30 minutes", 0.85, map[string]any{"time_source": otherTime.Source, "seconds": diff.Seconds()})
				}
			}
		}
	}
	if hasAssetTime {
		if tracks, err := s.deps.Store.ListGPSTracks(ctx, catalog.GPSTrackQuery{Limit: 1000}); err == nil {
			for _, track := range tracks {
				if track.StartTime == nil || track.EndTime == nil {
					continue
				}
				if assetTime.Time.Before(track.StartTime.Add(-30*time.Minute)) || assetTime.Time.After(track.EndTime.Add(30*time.Minute)) {
					continue
				}
				trackAsset, err := s.deps.Store.GetAsset(ctx, track.TrackAssetID)
				if err == nil {
					add("same_track", trackAsset, "timestamp candidate overlaps GPS track", 0.75, map[string]any{"track": track.Name, "time_source": assetTime.Source})
				}
			}
		}
	}
	for key := range groups {
		sort.Slice(groups[key], func(i, j int) bool {
			if groups[key][i].Score == groups[key][j].Score {
				return groups[key][i].Asset.DisplayName < groups[key][j].Asset.DisplayName
			}
			return groups[key][i].Score > groups[key][j].Score
		})
		if len(groups[key]) > 24 {
			groups[key] = groups[key][:24]
		}
	}
	return map[string]any{
		"asset_id":             asset.ID,
		"timestamp_candidates": catalog.AssetTimestampCandidates(asset, time.Local),
		"device":               assetDevice,
		"folder":               assetFolder,
		"groups":               groups,
		"note":                 "Related assets are metadata-only suggestions based on folder, device, local timestamp candidates, and track overlap.",
	}, nil
}

func firstAssetFolder(asset catalog.Asset) string {
	if len(asset.Locations) == 0 {
		return ""
	}
	path := strings.TrimSpace(asset.Locations[0].RelativePath)
	if path == "" {
		return ""
	}
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[:idx]
	}
	return ""
}

func deviceSignature(asset catalog.Asset) string {
	makeValue := strings.TrimSpace(fmt.Sprint(asset.Metadata["camera_make"]))
	modelValue := strings.TrimSpace(fmt.Sprint(asset.Metadata["camera_model"]))
	if makeValue == "<nil>" {
		makeValue = ""
	}
	if modelValue == "<nil>" {
		modelValue = ""
	}
	return strings.TrimSpace(strings.Join(compactStrings([]string{makeValue, modelValue}), " "))
}

func sameLocalDay(a, b time.Time) bool {
	ay, am, ad := a.In(time.Local).Date()
	by, bm, bd := b.In(time.Local).Date()
	return ay == by && am == bm && ad == bd
}

type placeHierarchyStore interface {
	QueryPlaceHierarchy(context.Context, catalog.PlaceHierarchyQuery) (catalog.PlaceHierarchyPage, error)
}

func (s *Server) handlePlaceHierarchy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	query := catalog.PlaceHierarchyQuery{
		Q:      r.URL.Query().Get("q"),
		Limit:  limit,
		Offset: offset,
	}
	var page catalog.PlaceHierarchyPage
	var err error
	if store, ok := s.deps.Store.(placeHierarchyStore); ok {
		page, err = store.QueryPlaceHierarchy(r.Context(), query)
	} else {
		places, listErr := s.deps.Store.ListPlaces(r.Context(), catalog.PlaceQuery{Q: query.Q, Limit: query.Limit, Offset: query.Offset})
		if listErr != nil {
			err = listErr
		} else {
			page.Page = catalog.Page{Limit: query.Limit, Offset: query.Offset, Total: len(places)}
			if page.Page.Limit <= 0 {
				page.Page.Limit = len(places)
			}
			page.Entries = make([]catalog.PlaceHierarchyEntry, 0, len(places))
			for _, place := range places {
				page.Entries = append(page.Entries, catalog.PlaceHierarchyEntry{
					Place:     place,
					Level:     placeCacheLevel(place),
					Label:     placeCacheLabel(place),
					Hierarchy: placeCacheHierarchy(place),
				})
			}
		}
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":           "local_place_cache",
		"offline":        true,
		"provider":       runtimeStringSetting("search.geocoder_provider", "local_place_cache"),
		"online_enabled": runtimeBoolSetting("search.online_geocoding", true),
		"radius_m":       runtimeIntSetting("search.reverse_geocode_radius_m", 100),
		"entries":        page.Entries,
		"tree":           buildPlaceHierarchyTree(page.Entries),
		"page":           page.Page,
		"note":           "Places are read from Cartolensia's local cache. Online geocoders are never called from this page.",
	})
}

func placeCacheHierarchy(place catalog.PlaceCacheEntry) []string {
	return compactStrings([]string{place.Country, place.Region, place.City, place.Road, place.Name})
}

func placeCacheLevel(place catalog.PlaceCacheEntry) string {
	switch {
	case strings.TrimSpace(place.Road) != "":
		return "road"
	case strings.TrimSpace(place.City) != "":
		return "city"
	case strings.TrimSpace(place.Region) != "":
		return "region"
	case strings.TrimSpace(place.Country) != "":
		return "country"
	default:
		return "place"
	}
}

func placeCacheLabel(place catalog.PlaceCacheEntry) string {
	if strings.TrimSpace(place.DisplayName) != "" {
		return place.DisplayName
	}
	return strings.Join(placeCacheHierarchy(place), " / ")
}

func buildPlaceHierarchyTree(entries []catalog.PlaceHierarchyEntry) []map[string]any {
	type node struct {
		Key         string
		Name        string
		Level       string
		AssetCount  int
		TrackCount  int
		PlacesCount int
		Children    map[string]*node
	}
	levels := []string{"country", "region", "city", "road", "place"}
	root := &node{Children: map[string]*node{}}
	for _, entry := range entries {
		parent := root
		parts := entry.Hierarchy
		if len(parts) == 0 {
			parts = []string{entry.Place.Name}
		}
		for idx, part := range parts {
			level := "place"
			if idx < len(levels) {
				level = levels[idx]
			}
			key := level + ":" + strings.ToLower(part)
			child := parent.Children[key]
			if child == nil {
				child = &node{Key: key, Name: part, Level: level, Children: map[string]*node{}}
				parent.Children[key] = child
			}
			child.AssetCount += entry.AssetCount
			child.TrackCount += entry.TrackCount
			child.PlacesCount++
			parent = child
		}
	}
	var flatten func(map[string]*node) []map[string]any
	flatten = func(nodes map[string]*node) []map[string]any {
		keys := make([]string, 0, len(nodes))
		for key := range nodes {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			return strings.ToLower(nodes[keys[i]].Name) < strings.ToLower(nodes[keys[j]].Name)
		})
		out := make([]map[string]any, 0, len(keys))
		for _, key := range keys {
			item := nodes[key]
			row := map[string]any{
				"key":          item.Key,
				"name":         item.Name,
				"level":        item.Level,
				"asset_count":  item.AssetCount,
				"track_count":  item.TrackCount,
				"places_count": item.PlacesCount,
			}
			if len(item.Children) > 0 {
				row["children"] = flatten(item.Children)
			}
			out = append(out, row)
		}
		return out
	}
	return flatten(root.Children)
}

var geocoderRateLimiter = struct {
	sync.Mutex
	lastCall map[string]time.Time
}{lastCall: map[string]time.Time{}}

type placeProviderStatus struct {
	Key              string `json:"key"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Configured       bool   `json:"configured"`
	Enabled          bool   `json:"enabled"`
	URL              string `json:"url,omitempty"`
	Locale           string `json:"locale,omitempty"`
	License          string `json:"license,omitempty"`
	Policy           string `json:"policy"`
	Recommended      bool   `json:"recommended,omitempty"`
	SecretConfigured bool   `json:"secret_configured,omitempty"`
	Note             string `json:"note,omitempty"`
}

func (s *Server) handlePlaceProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	provider := normalizeGeocoderProvider(runtimeStringSetting("search.geocoder_provider", "nominatim"))
	baseURL := strings.TrimRight(runtimeStringSetting("search.geocoder_provider_url", defaultGeocoderURL(provider)), "/")
	locale := normalizedGeocoderLocale()
	onlineEnabled := runtimeBoolSetting("search.online_geocoding", true)
	userAgent := strings.TrimSpace(runtimeStringSetting("search.geocoder_user_agent", "Cartolensia/1.0 self-hosted online-cache reverse geocoder"))
	minInterval := runtimeIntSetting("search.geocoder_min_interval_ms", 1100)
	googleConfigured := strings.TrimSpace(os.Getenv("CARTOLENSIA_GOOGLE_GEOCODING_API_KEY")) != ""
	googleCacheAcknowledged := googleGeocodingCacheAcknowledged()
	providers := []placeProviderStatus{
		{
			Key:         "local_place_cache",
			Name:        "Local Cartolensia place cache",
			Kind:        "local_cache",
			Configured:  true,
			Enabled:     true,
			Locale:      locale,
			Policy:      "Always available offline. No network calls.",
			Recommended: true,
			Note:        "Always checked first. Online providers only fill missing cache rows and never replace local cache matches.",
		},
		{
			Key:         "nominatim",
			Name:        "OpenStreetMap Nominatim",
			Kind:        "osm_nominatim",
			Configured:  provider == "nominatim" && baseURL != "",
			Enabled:     onlineEnabled && provider == "nominatim",
			URL:         baseURL,
			Locale:      locale,
			License:     "OpenStreetMap/Open Database License data; provider terms apply",
			Policy:      "Enabled by default for missing cache entries, cached, and rate-limited. For bulk enrichment, use a self-hosted endpoint.",
			Recommended: true,
			Note:        fmt.Sprintf("User-Agent configured: %t; minimum interval: %d ms.", userAgent != "", minInterval),
		},
		{
			Key:         "nominatim_compatible",
			Name:        "Self-hosted Nominatim-compatible endpoint",
			Kind:        "osm_nominatim",
			Configured:  provider == "nominatim_compatible" && baseURL != "",
			Enabled:     onlineEnabled && provider == "nominatim_compatible",
			URL:         baseURL,
			Locale:      locale,
			License:     "Depends on imported geodata and operator deployment",
			Policy:      "Recommended for large archives. Cartolensia caches every result and can safely run broad cache-fill jobs against operator-controlled endpoints.",
			Recommended: true,
		},
		{
			Key:         "photon",
			Name:        "Photon-compatible endpoint",
			Kind:        "osm_photon",
			Configured:  provider == "photon" && baseURL != "",
			Enabled:     onlineEnabled && provider == "photon",
			URL:         baseURL,
			Locale:      locale,
			License:     "Depends on endpoint and OSM-derived data",
			Policy:      "Use a self-hosted or operator-approved endpoint for large batches. Results are cached for offline reuse.",
			Recommended: true,
		},
		{
			Key:         "pelias",
			Name:        "Pelias-compatible endpoint",
			Kind:        "pelias",
			Configured:  provider == "pelias" && baseURL != "",
			Enabled:     onlineEnabled && provider == "pelias",
			URL:         baseURL,
			Locale:      locale,
			License:     "Depends on endpoint and imported geodata",
			Policy:      "Use a self-hosted or operator-approved endpoint for large archives. Results are cached for offline reuse.",
			Recommended: true,
		},
		{
			Key:              "google",
			Name:             "Google Geocoding API",
			Kind:             "google_geocoding",
			Configured:       provider == "google" && googleConfigured,
			Enabled:          onlineEnabled && provider == "google" && googleConfigured && googleCacheAcknowledged,
			URL:              "https://maps.googleapis.com/maps/api/geocode/json",
			Locale:           locale,
			License:          "Google Maps Platform terms apply",
			Policy:           "Opt-in only. API key is read from CARTOLENSIA_GOOGLE_GEOCODING_API_KEY and never returned by the API. Because Google restricts caching/storage, Cartolensia requires CARTOLENSIA_GOOGLE_GEOCODING_CACHE_ACK=I_ACCEPT_GOOGLE_TERMS before caching Google reverse-geocode rows.",
			SecretConfigured: googleConfigured,
			Note:             "Prefer self-hosted OSM providers for large offline archives. Google place IDs may be stored indefinitely; broader result storage is terms-bound.",
		},
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":                 runtimeStringSetting("search.geocoder_mode", "online_cache"),
		"online_enabled":       onlineEnabled,
		"active_provider":      provider,
		"active_provider_url":  baseURL,
		"locale":               locale,
		"min_interval_ms":      minInterval,
		"providers":            providers,
		"cache_policy":         "Cartolensia always checks local place_cache first. Missing coordinates use the configured online provider when online geocoding is enabled, and every result is persisted for offline reuse.",
		"bulk_policy":          "Public providers remain rate-limited. For large libraries, use imported local geodata or a self-hosted Nominatim/Pelias/Photon endpoint.",
		"google_secret_source": "CARTOLENSIA_GOOGLE_GEOCODING_API_KEY",
		"google_cache_ack":     googleCacheAcknowledged,
	})
}

func (s *Server) handlePlaceReverse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
		Online  *bool   `json:"online,omitempty"`
		RadiusM float64 `json:"radius_m,omitempty"`
	}
	onlineRequested := runtimeBoolSetting("search.online_geocoding", true)
	if r.Method == http.MethodGet {
		req.Lat, _ = strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("lat")), 64)
		req.Lon, _ = strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("lon")), 64)
		if _, ok := r.URL.Query()["online"]; ok {
			onlineRequested = boolQuery(r.URL.Query().Get("online"))
		}
		req.RadiusM, _ = strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("radius_m")), 64)
	} else if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.Online != nil {
			onlineRequested = *req.Online
		}
	}
	if req.Lat < -90 || req.Lat > 90 || req.Lon < -180 || req.Lon > 180 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("lat/lon are required and must be valid"))
		return
	}
	if req.RadiusM <= 0 {
		req.RadiusM = float64(runtimeIntSetting("search.reverse_geocode_radius_m", 100))
	}
	if req.RadiusM < 0 {
		req.RadiusM = 0
	}
	if req.RadiusM > 5000 {
		req.RadiusM = 5000
	}
	placeEntries := s.placeEntries(r.Context())
	places := mergePlaceMatches(
		placesForPoint(req.Lat, req.Lon, placeEntries),
		nearbyPlacesForPoint(req.Lat, req.Lon, placeEntries, req.RadiusM),
	)
	onlineEnabled := onlineRequested && runtimeBoolSetting("search.online_geocoding", true)
	if !onlineEnabled || reverseGeocodeCacheSatisfied(places) {
		note := "Matched existing cached place bbox and/or nearby cached place centroid; no online geocoder was called."
		if len(places) == 0 {
			note = "No local cached place contains this coordinate. Online reverse geocoding is disabled for this request or runtime."
		}
		if onlineEnabled && reverseGeocodeCacheSatisfied(places) {
			note = "Matched an existing provider-backed reverse-geocode cache entry; no duplicate online geocoder call was made."
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"lat":      req.Lat,
			"lon":      req.Lon,
			"source":   "local_place_cache",
			"cached":   len(places) > 0,
			"places":   places,
			"radius_m": req.RadiusM,
			"note":     note,
			"provider": "local_place_cache",
		})
		return
	}
	place, err := s.reverseGeocodeOnline(r.Context(), req.Lat, req.Lon)
	if err != nil {
		if len(places) > 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"lat":          req.Lat,
				"lon":          req.Lon,
				"source":       "local_place_cache",
				"cached":       true,
				"places":       places,
				"radius_m":     req.RadiusM,
				"provider":     runtimeStringSetting("search.geocoder_provider", "local_place_cache"),
				"online_error": err.Error(),
				"note":         "Local cache matched this coordinate, but the configured online geocoder failed while trying to enrich it.",
			})
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}
	created, err := s.deps.Store.UpsertPlace(r.Context(), place)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lat":      req.Lat,
		"lon":      req.Lon,
		"source":   created.Source,
		"cached":   true,
		"places":   mergePlaceMatches(places, []catalog.PlaceCacheEntry{created}),
		"radius_m": req.RadiusM,
		"provider": created.Provider,
		"note":     "Cache-first reverse geocode appended a unique provider-backed place_cache entry when missing.",
	})
}

func (s *Server) handlePlaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		places, err := s.deps.Store.ListPlaces(r.Context(), catalog.PlaceQuery{
			Q:      r.URL.Query().Get("q"),
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"places":         places,
			"mode":           "cache_only",
			"online_enabled": false,
			"note":           "Place cache is operator-managed. Cartolensia does not call online geocoders automatically.",
		})
	case http.MethodPost:
		place, err := decodePlacePayload(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		created, err := s.deps.Store.UpsertPlace(r.Context(), place)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handlePlaceByID(w http.ResponseWriter, r *http.Request) {
	placeID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/places/"), "/")
	if placeID == "" || strings.Contains(placeID, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		place, err := s.lookupPlaceByID(r.Context(), placeID)
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		detail, err := s.placeDetail(r.Context(), place)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	case http.MethodPatch:
		existing, err := s.lookupPlaceByID(r.Context(), placeID)
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		var patch catalog.PlaceCacheEntry
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&patch); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		merged := mergePlacePatch(existing, patch)
		merged.ID = placeID
		updated, err := s.deps.Store.UpsertPlace(r.Context(), merged)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := s.deps.Store.DeletePlace(r.Context(), placeID); err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "id": placeID})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) lookupPlaceByID(ctx context.Context, placeID string) (catalog.PlaceCacheEntry, error) {
	const pageSize = 10000
	for offset := 0; ; offset += pageSize {
		places, err := s.deps.Store.ListPlaces(ctx, catalog.PlaceQuery{Limit: pageSize, Offset: offset})
		if err != nil {
			return catalog.PlaceCacheEntry{}, err
		}
		for _, place := range places {
			if place.ID == placeID {
				return place, nil
			}
		}
		if len(places) < pageSize {
			break
		}
	}
	return catalog.PlaceCacheEntry{}, catalog.ErrNotFound
}

func (s *Server) placeDetail(ctx context.Context, place catalog.PlaceCacheEntry) (map[string]any, error) {
	const statsLimit = 10000
	radiusM := runtimeIntSetting("search.reverse_geocode_radius_m", 100)
	bbox := normalizedPlaceDetailBBox(place, radiusM)
	geos, err := s.deps.Store.QueryAssetGeo(ctx, catalog.GeoQuery{BBox: &bbox, Limit: statsLimit + 1})
	if err != nil {
		return nil, err
	}
	capped := len(geos) > statsLimit
	if capped {
		geos = geos[:statsLimit]
	}
	stats := map[string]any{
		"total_assets": len(geos),
		"photos":       0,
		"videos":       0,
		"audio":        0,
		"documents":    0,
		"tracks":       0,
		"other":        0,
		"capped":       capped,
	}
	for _, geo := range geos {
		switch geo.Asset.MediaKind {
		case "photo":
			stats["photos"] = stats["photos"].(int) + 1
		case "video":
			stats["videos"] = stats["videos"].(int) + 1
		case "audio":
			stats["audio"] = stats["audio"].(int) + 1
		case "document":
			stats["documents"] = stats["documents"].(int) + 1
		case "track":
			stats["tracks"] = stats["tracks"].(int) + 1
		default:
			stats["other"] = stats["other"].(int) + 1
		}
	}
	return map[string]any{
		"place":          place,
		"level":          placeCacheLevel(place),
		"label":          placeCacheLabel(place),
		"hierarchy":      placeCacheHierarchy(place),
		"radius_m":       radiusM,
		"stats":          stats,
		"search_queries": placeDetailSearchQueries(place),
		"note":           "Statistics are computed from cached Cartolensia geotags inside the place bbox/radius. Capped results avoid expensive full-archive scans.",
	}, nil
}

func normalizedPlaceDetailBBox(place catalog.PlaceCacheEntry, radiusM int) catalog.BBox {
	bbox := place.BBox
	if bbox.MaxLat > bbox.MinLat && bbox.MaxLon > bbox.MinLon {
		return bbox
	}
	if radiusM <= 0 {
		radiusM = 100
	}
	latDelta := float64(radiusM) / 111320.0
	cosLat := math.Cos(place.Lat * math.Pi / 180)
	if math.Abs(cosLat) < 0.01 {
		cosLat = 0.01
	}
	lonDelta := float64(radiusM) / (111320.0 * math.Abs(cosLat))
	return catalog.BBox{
		MinLon: clampFloat(place.Lon-lonDelta, -180, 180),
		MinLat: clampFloat(place.Lat-latDelta, -90, 90),
		MaxLon: clampFloat(place.Lon+lonDelta, -180, 180),
		MaxLat: clampFloat(place.Lat+latDelta, -90, 90),
	}
}

func placeDetailSearchQueries(place catalog.PlaceCacheEntry) map[string]string {
	base := placeSearchToken(place)
	return map[string]string{
		"all":       base,
		"photos":    base + " kind:photo",
		"videos":    base + " kind:video",
		"audio":     base + " kind:audio",
		"documents": base + " kind:document",
		"tracks":    base + " kind:track",
	}
}

func placeSearchToken(place catalog.PlaceCacheEntry) string {
	name := firstNonEmpty(place.DisplayName, place.Name, place.City, place.Region, place.Country)
	if name == "" {
		return "place:*"
	}
	return "place:" + strconv.Quote(name)
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func decodePlacePayload(r *http.Request) (catalog.PlaceCacheEntry, error) {
	var payload catalog.PlaceCacheEntry
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&payload); err != nil {
		return payload, err
	}
	return normalizePlacePayload(payload)
}

func normalizePlacePayload(place catalog.PlaceCacheEntry) (catalog.PlaceCacheEntry, error) {
	place.Name = strings.TrimSpace(place.Name)
	place.NormalizedName = strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(firstNonEmpty(place.NormalizedName, place.Name)))), " ")
	place.Provider = firstNonEmpty(strings.TrimSpace(place.Provider), "local")
	place.DisplayName = firstNonEmpty(strings.TrimSpace(place.DisplayName), place.Name)
	place.Source = firstNonEmpty(strings.TrimSpace(place.Source), "operator_cache")
	place.Country = strings.TrimSpace(place.Country)
	place.Region = strings.TrimSpace(place.Region)
	place.City = strings.TrimSpace(place.City)
	place.Road = strings.TrimSpace(place.Road)
	place.Aliases = compactStrings(place.Aliases)
	if place.Metadata == nil {
		place.Metadata = map[string]any{}
	}
	if place.Name == "" {
		return place, fmt.Errorf("place name is required")
	}
	if place.Lat < -90 || place.Lat > 90 || place.Lon < -180 || place.Lon > 180 {
		return place, fmt.Errorf("place coordinates are invalid")
	}
	if place.BBox.MinLon == 0 && place.BBox.MaxLon == 0 && place.BBox.MinLat == 0 && place.BBox.MaxLat == 0 {
		place.BBox = catalog.BBox{
			MinLon: place.Lon - 0.01,
			MinLat: place.Lat - 0.01,
			MaxLon: place.Lon + 0.01,
			MaxLat: place.Lat + 0.01,
		}
	}
	if place.BBox.MinLon > place.BBox.MaxLon || place.BBox.MinLat > place.BBox.MaxLat {
		return place, fmt.Errorf("place bbox is invalid")
	}
	return place, nil
}

func mergePlacePatch(existing, patch catalog.PlaceCacheEntry) catalog.PlaceCacheEntry {
	if strings.TrimSpace(patch.Name) != "" {
		existing.Name = patch.Name
	}
	if strings.TrimSpace(patch.NormalizedName) != "" {
		existing.NormalizedName = patch.NormalizedName
	}
	if len(patch.Aliases) > 0 {
		existing.Aliases = patch.Aliases
	}
	if strings.TrimSpace(patch.Provider) != "" {
		existing.Provider = patch.Provider
	}
	if strings.TrimSpace(patch.DisplayName) != "" {
		existing.DisplayName = patch.DisplayName
	}
	if strings.TrimSpace(patch.Country) != "" {
		existing.Country = patch.Country
	}
	if strings.TrimSpace(patch.Region) != "" {
		existing.Region = patch.Region
	}
	if strings.TrimSpace(patch.City) != "" {
		existing.City = patch.City
	}
	if strings.TrimSpace(patch.Road) != "" {
		existing.Road = patch.Road
	}
	if patch.Lat != 0 || patch.Lon != 0 {
		existing.Lat = patch.Lat
		existing.Lon = patch.Lon
	}
	if patch.BBox.MinLon != 0 || patch.BBox.MinLat != 0 || patch.BBox.MaxLon != 0 || patch.BBox.MaxLat != 0 {
		existing.BBox = patch.BBox
	}
	if strings.TrimSpace(patch.Source) != "" {
		existing.Source = patch.Source
	}
	if patch.Metadata != nil {
		existing.Metadata = patch.Metadata
	}
	normalized, err := normalizePlacePayload(existing)
	if err == nil {
		return normalized
	}
	return existing
}

func (s *Server) reverseGeocodeOnline(ctx context.Context, lat, lon float64) (catalog.PlaceCacheEntry, error) {
	provider := normalizeGeocoderProvider(runtimeStringSetting("search.geocoder_provider", "nominatim"))
	if provider == "local_place_cache" || provider == "none" {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("select an online reverse-geocoding provider before requesting online lookup")
	}
	baseURL := strings.TrimRight(runtimeStringSetting("search.geocoder_provider_url", defaultGeocoderURL(provider)), "/")
	if baseURL == "" {
		baseURL = defaultGeocoderURL(provider)
	}
	if baseURL == "" && provider != "google" {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("geocoder provider URL is empty")
	}
	locale := normalizedGeocoderLocale()
	minInterval := runtimeIntSetting("search.geocoder_min_interval_ms", 1100)
	if minInterval < 0 {
		minInterval = 0
	}
	switch provider {
	case "nominatim", "nominatim_compatible":
		if err := waitGeocoderRateLimit(ctx, provider+"|"+baseURL, time.Duration(minInterval)*time.Millisecond); err != nil {
			return catalog.PlaceCacheEntry{}, err
		}
		return s.reverseGeocodeNominatim(ctx, provider, baseURL, lat, lon, locale)
	case "photon":
		if err := waitGeocoderRateLimit(ctx, provider+"|"+baseURL, time.Duration(minInterval)*time.Millisecond); err != nil {
			return catalog.PlaceCacheEntry{}, err
		}
		return s.reverseGeocodePhoton(ctx, provider, baseURL, lat, lon, locale)
	case "pelias":
		if err := waitGeocoderRateLimit(ctx, provider+"|"+baseURL, time.Duration(minInterval)*time.Millisecond); err != nil {
			return catalog.PlaceCacheEntry{}, err
		}
		return s.reverseGeocodePelias(ctx, provider, baseURL, lat, lon, locale)
	case "google":
		if err := waitGeocoderRateLimit(ctx, provider, time.Duration(minInterval)*time.Millisecond); err != nil {
			return catalog.PlaceCacheEntry{}, err
		}
		return s.reverseGeocodeGoogle(ctx, lat, lon, locale)
	default:
		return catalog.PlaceCacheEntry{}, fmt.Errorf("unsupported reverse-geocoding provider %q", provider)
	}
}

func normalizeGeocoderProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "osm", "openstreetmap", "nominatim":
		return "nominatim"
	case "local", "cache", "local_place_cache", "none":
		return "local_place_cache"
	case "nominatim-compatible", "nominatim_compatible", "self_hosted_nominatim":
		return "nominatim_compatible"
	case "photon", "photon_compatible", "komoot_photon":
		return "photon"
	case "pelias", "pelias_compatible":
		return "pelias"
	case "google", "google_geocoding", "google_maps":
		return "google"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func defaultGeocoderURL(provider string) string {
	switch normalizeGeocoderProvider(provider) {
	case "nominatim", "nominatim_compatible":
		return "https://nominatim.openstreetmap.org"
	case "google":
		return "https://maps.googleapis.com/maps/api/geocode/json"
	default:
		return ""
	}
}

func normalizedGeocoderLocale() string {
	locale := strings.TrimSpace(runtimeStringSetting("search.geocoder_locale", ""))
	if locale == "" || strings.EqualFold(locale, "auto") {
		return ""
	}
	return locale
}

func geocoderProviderWithLocale(provider, locale string) string {
	provider = normalizeGeocoderProvider(provider)
	if locale == "" {
		return provider
	}
	return provider + ":" + locale
}

func waitGeocoderRateLimit(ctx context.Context, key string, interval time.Duration) error {
	if interval <= 0 {
		return nil
	}
	geocoderRateLimiter.Lock()
	defer geocoderRateLimiter.Unlock()
	last := geocoderRateLimiter.lastCall[key]
	delay := interval - time.Since(last)
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		geocoderRateLimiter.Unlock()
		select {
		case <-ctx.Done():
			geocoderRateLimiter.Lock()
			return ctx.Err()
		case <-timer.C:
		}
		geocoderRateLimiter.Lock()
	}
	geocoderRateLimiter.lastCall[key] = time.Now()
	return nil
}

func (s *Server) reverseGeocodeNominatim(ctx context.Context, provider, baseURL string, lat, lon float64, locale string) (catalog.PlaceCacheEntry, error) {
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/reverse")
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	query := endpoint.Query()
	query.Set("format", "jsonv2")
	query.Set("lat", strconv.FormatFloat(lat, 'f', 7, 64))
	query.Set("lon", strconv.FormatFloat(lon, 'f', 7, 64))
	query.Set("addressdetails", "1")
	query.Set("extratags", "1")
	if locale != "" {
		query.Set("accept-language", locale)
	}
	if email := strings.TrimSpace(runtimeStringSetting("search.geocoder_contact_email", "")); email != "" {
		query.Set("email", email)
	}
	endpoint.RawQuery = query.Encode()
	req, cancel, err := geocoderRequest(ctx, endpoint.String(), locale)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	defer cancel()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("geocoder returned HTTP %d", resp.StatusCode)
	}
	var data struct {
		DisplayName string            `json:"display_name"`
		Name        string            `json:"name"`
		Lat         string            `json:"lat"`
		Lon         string            `json:"lon"`
		BoundingBox []string          `json:"boundingbox"`
		Address     map[string]string `json:"address"`
		PlaceID     int64             `json:"place_id"`
		OSMType     string            `json:"osm_type"`
		OSMID       int64             `json:"osm_id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&data); err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	if strings.TrimSpace(data.DisplayName) == "" {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("geocoder returned no display_name")
	}
	country := firstNonEmpty(data.Address["country"], data.Address["country_code"])
	region := firstNonEmpty(data.Address["state"], data.Address["region"], data.Address["province"], data.Address["county"])
	city := firstNonEmpty(data.Address["city"], data.Address["town"], data.Address["village"], data.Address["municipality"])
	road := firstNonEmpty(data.Address["road"], data.Address["pedestrian"], data.Address["footway"], data.Address["path"])
	name := firstNonEmpty(data.Name, road, city, region, country, data.DisplayName)
	centerLat := lat
	centerLon := lon
	if parsed, err := strconv.ParseFloat(data.Lat, 64); err == nil {
		centerLat = parsed
	}
	if parsed, err := strconv.ParseFloat(data.Lon, 64); err == nil {
		centerLon = parsed
	}
	bbox := catalog.BBox{MinLon: lon - 0.01, MinLat: lat - 0.01, MaxLon: lon + 0.01, MaxLat: lat + 0.01}
	if len(data.BoundingBox) >= 4 {
		minLat, err1 := strconv.ParseFloat(data.BoundingBox[0], 64)
		maxLat, err2 := strconv.ParseFloat(data.BoundingBox[1], 64)
		minLon, err3 := strconv.ParseFloat(data.BoundingBox[2], 64)
		maxLon, err4 := strconv.ParseFloat(data.BoundingBox[3], 64)
		if err1 == nil && err2 == nil && err3 == nil && err4 == nil {
			bbox = catalog.BBox{MinLon: minLon, MinLat: minLat, MaxLon: maxLon, MaxLat: maxLat}
		}
	}
	aliases := uniqueStrings(compactStrings([]string{name, data.DisplayName, country, region, city, road}))
	return normalizePlacePayload(catalog.PlaceCacheEntry{
		Name:           name,
		NormalizedName: strings.Join(strings.Fields(strings.ToLower(name)), " "),
		Aliases:        aliases,
		Provider:       geocoderProviderWithLocale(provider, locale),
		DisplayName:    data.DisplayName,
		Country:        country,
		Region:         region,
		City:           city,
		Road:           road,
		Lat:            centerLat,
		Lon:            centerLon,
		BBox:           bbox,
		Source:         "online_user_triggered_cache",
		Metadata: map[string]any{
			"provider":   provider,
			"locale":     locale,
			"place_id":   data.PlaceID,
			"osm_type":   data.OSMType,
			"osm_id":     data.OSMID,
			"policy":     "online cache-fill reverse geocode; cached for offline reuse; use self-hosted/operator-approved endpoints for large archives",
			"source_url": strings.TrimRight(baseURL, "/"),
		},
	})
}

func (s *Server) reverseGeocodePhoton(ctx context.Context, provider, baseURL string, lat, lon float64, locale string) (catalog.PlaceCacheEntry, error) {
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/reverse")
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	query := endpoint.Query()
	query.Set("lat", strconv.FormatFloat(lat, 'f', 7, 64))
	query.Set("lon", strconv.FormatFloat(lon, 'f', 7, 64))
	if locale != "" {
		query.Set("lang", firstLocaleToken(locale))
	}
	endpoint.RawQuery = query.Encode()
	req, cancel, err := geocoderRequest(ctx, endpoint.String(), locale)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	defer cancel()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("geocoder returned HTTP %d", resp.StatusCode)
	}
	var data struct {
		Features []struct {
			Properties map[string]any `json:"properties"`
			BBox       []float64      `json:"bbox"`
			Geometry   struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&data); err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	if len(data.Features) == 0 {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("geocoder returned no features")
	}
	props := data.Features[0].Properties
	return normalizeFeaturePlace(provider, strings.TrimRight(baseURL, "/"), locale, lat, lon, props, data.Features[0].BBox, data.Features[0].Geometry.Coordinates)
}

func (s *Server) reverseGeocodePelias(ctx context.Context, provider, baseURL string, lat, lon float64, locale string) (catalog.PlaceCacheEntry, error) {
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/v1/reverse")
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	query := endpoint.Query()
	query.Set("point.lat", strconv.FormatFloat(lat, 'f', 7, 64))
	query.Set("point.lon", strconv.FormatFloat(lon, 'f', 7, 64))
	if locale != "" {
		query.Set("lang", firstLocaleToken(locale))
	}
	endpoint.RawQuery = query.Encode()
	req, cancel, err := geocoderRequest(ctx, endpoint.String(), locale)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	defer cancel()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("geocoder returned HTTP %d", resp.StatusCode)
	}
	var data struct {
		Features []struct {
			Properties map[string]any `json:"properties"`
			BBox       []float64      `json:"bbox"`
			Geometry   struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&data); err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	if len(data.Features) == 0 {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("geocoder returned no features")
	}
	props := data.Features[0].Properties
	return normalizeFeaturePlace(provider, strings.TrimRight(baseURL, "/"), locale, lat, lon, props, data.Features[0].BBox, data.Features[0].Geometry.Coordinates)
}

func (s *Server) reverseGeocodeGoogle(ctx context.Context, lat, lon float64, locale string) (catalog.PlaceCacheEntry, error) {
	apiKey := strings.TrimSpace(os.Getenv("CARTOLENSIA_GOOGLE_GEOCODING_API_KEY"))
	if apiKey == "" {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("google geocoding selected but CARTOLENSIA_GOOGLE_GEOCODING_API_KEY is not configured")
	}
	if !googleGeocodingCacheAcknowledged() {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("google geocoding results are not cached by default; review Google Maps Platform terms and set CARTOLENSIA_GOOGLE_GEOCODING_CACHE_ACK=I_ACCEPT_GOOGLE_TERMS to enable this provider")
	}
	baseURL := strings.TrimSpace(runtimeStringSetting("search.geocoder_provider_url", defaultGeocoderURL("google")))
	if baseURL == "" || strings.Contains(baseURL, "nominatim.openstreetmap.org") {
		baseURL = defaultGeocoderURL("google")
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	query := endpoint.Query()
	query.Set("latlng", strconv.FormatFloat(lat, 'f', 7, 64)+","+strconv.FormatFloat(lon, 'f', 7, 64))
	query.Set("key", apiKey)
	if locale != "" {
		query.Set("language", firstLocaleToken(locale))
	}
	endpoint.RawQuery = query.Encode()
	req, cancel, err := geocoderRequest(ctx, endpoint.String(), locale)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	defer cancel()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("google geocoder returned HTTP %d", resp.StatusCode)
	}
	var data struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message"`
		Results      []struct {
			FormattedAddress string   `json:"formatted_address"`
			PlaceID          string   `json:"place_id"`
			Types            []string `json:"types"`
			Geometry         struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
				Viewport struct {
					Northeast struct {
						Lat float64 `json:"lat"`
						Lng float64 `json:"lng"`
					} `json:"northeast"`
					Southwest struct {
						Lat float64 `json:"lat"`
						Lng float64 `json:"lng"`
					} `json:"southwest"`
				} `json:"viewport"`
			} `json:"geometry"`
			AddressComponents []struct {
				LongName  string   `json:"long_name"`
				ShortName string   `json:"short_name"`
				Types     []string `json:"types"`
			} `json:"address_components"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&data); err != nil {
		return catalog.PlaceCacheEntry{}, err
	}
	if data.Status != "" && data.Status != "OK" {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("google geocoder returned %s: %s", data.Status, data.ErrorMessage)
	}
	if len(data.Results) == 0 {
		return catalog.PlaceCacheEntry{}, fmt.Errorf("google geocoder returned no results")
	}
	result := data.Results[0]
	components := googleAddressComponents(result.AddressComponents)
	country := firstNonEmpty(components["country"], components["country_short"])
	region := firstNonEmpty(components["administrative_area_level_1"], components["administrative_area_level_2"])
	city := firstNonEmpty(components["locality"], components["postal_town"], components["administrative_area_level_3"])
	road := firstNonEmpty(components["route"], components["street_address"], components["premise"])
	name := firstNonEmpty(road, city, region, country, result.FormattedAddress)
	centerLat := firstNonZero(result.Geometry.Location.Lat, lat)
	centerLon := firstNonZero(result.Geometry.Location.Lng, lon)
	bbox := catalog.BBox{MinLon: lon - 0.01, MinLat: lat - 0.01, MaxLon: lon + 0.01, MaxLat: lat + 0.01}
	if result.Geometry.Viewport.Northeast.Lat != 0 || result.Geometry.Viewport.Southwest.Lat != 0 {
		bbox = catalog.BBox{
			MinLon: result.Geometry.Viewport.Southwest.Lng,
			MinLat: result.Geometry.Viewport.Southwest.Lat,
			MaxLon: result.Geometry.Viewport.Northeast.Lng,
			MaxLat: result.Geometry.Viewport.Northeast.Lat,
		}
	}
	return normalizePlacePayload(catalog.PlaceCacheEntry{
		Name:           name,
		NormalizedName: strings.Join(strings.Fields(strings.ToLower(name)), " "),
		Aliases:        uniqueStrings(compactStrings([]string{name, result.FormattedAddress, country, region, city, road})),
		Provider:       geocoderProviderWithLocale("google", locale),
		DisplayName:    result.FormattedAddress,
		Country:        country,
		Region:         region,
		City:           city,
		Road:           road,
		Lat:            centerLat,
		Lon:            centerLon,
		BBox:           bbox,
		Source:         "online_user_triggered_cache",
		Metadata: map[string]any{
			"provider":       "google",
			"locale":         locale,
			"place_id":       result.PlaceID,
			"types":          result.Types,
			"policy":         "Google Maps Platform terms apply; operator is responsible for API-key use, retention, attribution, and allowed caching.",
			"api_key_source": "CARTOLENSIA_GOOGLE_GEOCODING_API_KEY",
		},
	})
}

func googleGeocodingCacheAcknowledged() bool {
	return strings.TrimSpace(os.Getenv("CARTOLENSIA_GOOGLE_GEOCODING_CACHE_ACK")) == "I_ACCEPT_GOOGLE_TERMS"
}

func geocoderRequest(ctx context.Context, endpoint, locale string) (*http.Request, context.CancelFunc, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("User-Agent", firstNonEmpty(runtimeStringSetting("search.geocoder_user_agent", ""), "Cartolensia/1.0 self-hosted online-cache reverse geocoder"))
	if locale != "" {
		req.Header.Set("Accept-Language", locale)
	}
	return req, cancel, nil
}

func firstLocaleToken(locale string) string {
	token := strings.TrimSpace(strings.Split(locale, ",")[0])
	if idx := strings.Index(token, ";"); idx >= 0 {
		token = strings.TrimSpace(token[:idx])
	}
	return token
}

func normalizeFeaturePlace(provider, sourceURL, locale string, lat, lon float64, props map[string]any, bboxValues, coordinates []float64) (catalog.PlaceCacheEntry, error) {
	country := stringFromAny(firstExisting(props, "country", "country_a"))
	region := stringFromAny(firstExisting(props, "state", "region", "county", "macroregion"))
	city := stringFromAny(firstExisting(props, "city", "locality", "localadmin", "borough"))
	road := stringFromAny(firstExisting(props, "street", "road", "name"))
	displayName := stringFromAny(firstExisting(props, "label", "display_name", "name"))
	name := firstNonEmpty(stringFromAny(props["name"]), road, city, region, country, displayName)
	centerLat := lat
	centerLon := lon
	if len(coordinates) >= 2 {
		centerLon = coordinates[0]
		centerLat = coordinates[1]
	}
	bbox := catalog.BBox{MinLon: lon - 0.01, MinLat: lat - 0.01, MaxLon: lon + 0.01, MaxLat: lat + 0.01}
	if len(bboxValues) >= 4 {
		bbox = catalog.BBox{MinLon: bboxValues[0], MinLat: bboxValues[1], MaxLon: bboxValues[2], MaxLat: bboxValues[3]}
	}
	return normalizePlacePayload(catalog.PlaceCacheEntry{
		Name:           name,
		NormalizedName: strings.Join(strings.Fields(strings.ToLower(name)), " "),
		Aliases:        uniqueStrings(compactStrings([]string{name, displayName, country, region, city, road})),
		Provider:       geocoderProviderWithLocale(provider, locale),
		DisplayName:    firstNonEmpty(displayName, strings.Join(compactStrings([]string{road, city, region, country}), ", ")),
		Country:        country,
		Region:         region,
		City:           city,
		Road:           road,
		Lat:            centerLat,
		Lon:            centerLon,
		BBox:           bbox,
		Source:         "online_user_triggered_cache",
		Metadata: map[string]any{
			"provider":   provider,
			"locale":     locale,
			"source_url": sourceURL,
			"source_id":  firstExisting(props, "id", "gid", "osm_id"),
			"layer":      firstExisting(props, "layer", "osm_key", "type"),
			"policy":     "online cache-fill reverse geocode; cached for offline reuse; use self-hosted/operator-approved endpoints for large archives",
		},
	})
}

func firstExisting(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if strings.TrimSpace(stringFromAny(value)) != "" {
				return value
			}
		}
	}
	return nil
}

func googleAddressComponents(components []struct {
	LongName  string   `json:"long_name"`
	ShortName string   `json:"short_name"`
	Types     []string `json:"types"`
}) map[string]string {
	out := map[string]string{}
	for _, component := range components {
		for _, typ := range component.Types {
			if strings.TrimSpace(component.LongName) != "" {
				out[typ] = component.LongName
			}
			if strings.TrimSpace(component.ShortName) != "" {
				out[typ+"_short"] = component.ShortName
			}
		}
	}
	return out
}

func firstNonZero(value, fallback float64) float64 {
	if value != 0 {
		return value
	}
	return fallback
}

func runtimeStringSetting(key, fallback string) string {
	runtimeSettings.RLock()
	defer runtimeSettings.RUnlock()
	if value, ok := runtimeSettings.values[key]; ok {
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
			return text
		}
	}
	return fallback
}

func runtimeBoolSetting(key string, fallback bool) bool {
	runtimeSettings.RLock()
	defer runtimeSettings.RUnlock()
	if value, ok := runtimeSettings.values[key]; ok {
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			return boolQuery(typed)
		}
	}
	return fallback
}

type assetSearchContext struct {
	albumAssetMatches map[string]map[string]struct{}
	trackNameMatches  map[string]map[string]struct{}
	tagMatches        map[string]map[string]struct{}
	predictionMatches map[string]map[string]struct{}
	faceMatches       map[string]map[string]struct{}
	placeMatches      map[string]map[string]struct{}
	transcriptMatches map[string]map[string]struct{}
	documentMatches   map[string]map[string]struct{}
	audioMatches      map[string]map[string]struct{}
	frameMatches      map[string]map[string]struct{}
	places            []searchPlaceMatch
	placeEntries      []catalog.PlaceCacheEntry
}

type SearchBackend interface {
	ID() string
	Mode() string
}

type postgresLocalSearchBackend struct{}

func (postgresLocalSearchBackend) ID() string {
	return "postgres_local"
}

func (postgresLocalSearchBackend) Mode() string {
	return "fts_trigram_ready_metadata_place_ai_ocr"
}

func (s *Server) searchBackend() SearchBackend {
	return postgresLocalSearchBackend{}
}

type searchPlaceMatch struct {
	Query         string       `json:"query"`
	Name          string       `json:"name"`
	DisplayName   string       `json:"display_name"`
	Provider      string       `json:"provider"`
	Source        string       `json:"source"`
	Lat           float64      `json:"lat"`
	Lon           float64      `json:"lon"`
	BBox          catalog.BBox `json:"bbox"`
	MatchedAssets int          `json:"matched_assets"`
}

type assetPlaceRecord struct {
	CoordinateSource string         `json:"coordinate_source"`
	GeoSource        string         `json:"geo_source,omitempty"`
	Lat              float64        `json:"lat"`
	Lon              float64        `json:"lon"`
	PlaceName        string         `json:"place_name"`
	DisplayName      string         `json:"display_name"`
	Provider         string         `json:"provider"`
	Source           string         `json:"source"`
	Match            string         `json:"match"`
	BBox             catalog.BBox   `json:"bbox"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type ocrBlockRecord struct {
	ID         string         `json:"id"`
	AssetID    string         `json:"asset_id"`
	Text       string         `json:"text"`
	Language   string         `json:"language,omitempty"`
	Engine     string         `json:"engine,omitempty"`
	Confidence *float64       `json:"confidence,omitempty"`
	X          float64        `json:"x"`
	Y          float64        `json:"y"`
	Width      float64        `json:"width"`
	Height     float64        `json:"height"`
	ModelName  string         `json:"model_name,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ocrSummaryRecord struct {
	FullText   string    `json:"full_text"`
	Languages  []string  `json:"languages"`
	Engines    []string  `json:"engines"`
	ModelName  string    `json:"model_name,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	BlockCount int       `json:"block_count"`
}

func (s *Server) buildSearchContext(ctx context.Context, tokens []string) assetSearchContext {
	out := assetSearchContext{
		albumAssetMatches: map[string]map[string]struct{}{},
		trackNameMatches:  map[string]map[string]struct{}{},
		tagMatches:        map[string]map[string]struct{}{},
		predictionMatches: map[string]map[string]struct{}{},
		faceMatches:       map[string]map[string]struct{}{},
		placeMatches:      map[string]map[string]struct{}{},
		transcriptMatches: map[string]map[string]struct{}{},
		documentMatches:   map[string]map[string]struct{}{},
		audioMatches:      map[string]map[string]struct{}{},
		frameMatches:      map[string]map[string]struct{}{},
	}
	out.placeEntries = s.placeEntries(ctx)
	var indexedAssets []catalog.Asset
	for _, token := range tokens {
		prefix, plain, ok := strings.Cut(token, ":")
		if !ok {
			prefix = ""
			plain = token
		}
		plain = strings.TrimSpace(strings.ToLower(plain))
		if plain == "" {
			continue
		}
		if prefix == "" || prefix == "place" {
			if place, ok := placeForQuery(plain, out.placeEntries); ok {
				matches := map[string]struct{}{}
				geos, err := s.deps.Store.QueryAssetGeo(ctx, catalog.GeoQuery{BBox: &place.BBox, Limit: 10000})
				if err == nil {
					for _, geo := range geos {
						matches[geo.Asset.ID] = struct{}{}
					}
				}
				out.placeMatches[token] = matches
				out.places = append(out.places, searchPlaceMatch{
					Query:         plain,
					Name:          place.Name,
					DisplayName:   place.DisplayName,
					Provider:      place.Provider,
					Source:        place.Source,
					Lat:           place.Lat,
					Lon:           place.Lon,
					BBox:          place.BBox,
					MatchedAssets: len(matches),
				})
				if prefix == "place" {
					continue
				}
			}
		}
		switch prefix {
		case "", "album":
			matches := map[string]struct{}{}
			albums, err := s.deps.Store.ListAlbums(ctx, catalog.AlbumQuery{Tree: true, Limit: 1000})
			if err == nil {
				for _, album := range albums {
					if !strings.Contains(strings.ToLower(album.Title+" "+album.Slug+" "+album.Description), plain) {
						continue
					}
					items, err := s.deps.Store.ListAlbumItems(ctx, catalog.AlbumItemQuery{AlbumID: album.ID, Limit: 10000})
					if err != nil {
						continue
					}
					for _, item := range items.Items {
						matches[item.Asset.ID] = struct{}{}
					}
				}
			}
			if len(matches) > 0 || prefix == "album" {
				out.albumAssetMatches[token] = matches
			}
			if prefix != "" {
				break
			}
			fallthrough
		case "track":
			matches := map[string]struct{}{}
			tracks, err := s.deps.Store.ListGPSTracks(ctx, catalog.GPSTrackQuery{Limit: 1000})
			if err == nil {
				for _, track := range tracks {
					if strings.Contains(strings.ToLower(track.Name+" "+track.SourceFormat), plain) {
						matches[track.TrackAssetID] = struct{}{}
					}
				}
			}
			if len(matches) > 0 || prefix == "track" {
				out.trackNameMatches[token] = matches
			}
			if prefix != "" {
				break
			}
			fallthrough
		case "tag", "category", "safety", "caption", "ocr", "face", "transcript", "document", "genre", "key", "tempo", "frame", "video-caption":
			if indexedAssets == nil {
				page, err := s.deps.Store.QueryAssets(ctx, catalog.AssetQuery{Limit: 1000})
				if err == nil {
					indexedAssets = page.Assets
				}
			}
			matches := map[string]struct{}{}
			faceMatches := map[string]struct{}{}
			transcriptMatches := map[string]struct{}{}
			documentMatches := map[string]struct{}{}
			audioMatches := map[string]struct{}{}
			frameMatches := map[string]struct{}{}
			for _, asset := range indexedAssets {
				if prefix == "face" || prefix == "" {
					faces, err := s.deps.Store.ListFaceDetections(ctx, asset.ID)
					if err == nil && len(faces) > 0 {
						for _, face := range faces {
							if metadataBool(face.Metadata, "ignored") || metadataBool(face.Metadata, "deleted") {
								continue
							}
							label := strings.ToLower(strings.Join([]string{
								"face detected yes unassigned",
								face.ClusterID,
								stringFromMap(face.Metadata, "label"),
								stringFromMap(face.Metadata, "name"),
								stringFromMap(face.Metadata, "review_status"),
							}, " "))
							if plain == "" || strings.Contains(label, plain) {
								faceMatches[asset.ID] = struct{}{}
							}
						}
					}
					if prefix == "face" {
						continue
					}
				}
				if prefix == "" || prefix == "tag" || prefix == "category" || prefix == "safety" || prefix == "caption" || prefix == "ocr" {
					tags, _ := s.deps.Store.ListAssetTags(ctx, asset.ID)
					for _, tag := range tags {
						text := strings.ToLower(tag.Tag + " " + tag.Source)
						if strings.Contains(text, plain) {
							matches[asset.ID] = struct{}{}
						}
					}
					predictions, _ := s.deps.Store.ListAIPredictions(ctx, asset.ID)
					for _, prediction := range predictions {
						if prefix == "ocr" && prediction.Task != "ocr_image" && prediction.Task != "ocr" && prediction.Task != "ocr_text" {
							continue
						}
						text := strings.ToLower(prediction.Label + " " + prediction.Task + " " + prediction.ModelName)
						if strings.Contains(text, plain) {
							matches[asset.ID] = struct{}{}
						}
					}
				}
				if prefix == "" || prefix == "transcript" {
					transcripts, _ := s.deps.Store.ListTranscripts(ctx, asset.ID, 20)
					for _, transcript := range transcripts {
						text := strings.ToLower(transcript.FullText + " " + transcript.Language + " " + transcript.Model)
						for _, segment := range transcript.Segments {
							text += " " + strings.ToLower(segment.Text)
						}
						if strings.Contains(text, plain) {
							transcriptMatches[asset.ID] = struct{}{}
						}
					}
				}
				if prefix == "" || prefix == "document" {
					doc, err := s.deps.Store.GetDocumentText(ctx, asset.ID)
					if err == nil && strings.Contains(strings.ToLower(doc.Title+" "+doc.Author+" "+doc.Text+" "+doc.Markdown+" "+doc.Engine), plain) {
						documentMatches[asset.ID] = struct{}{}
					}
				}
				if prefix == "" || prefix == "genre" || prefix == "key" || prefix == "tempo" {
					features, err := s.deps.Store.GetAudioFeatures(ctx, asset.ID)
					if err == nil && audioFeatureMatchesToken(features, prefix, plain) {
						audioMatches[asset.ID] = struct{}{}
					}
				}
				if prefix == "" || prefix == "frame" || prefix == "video-caption" || prefix == "caption" {
					captions, _ := s.deps.Store.ListVideoFrameCaptions(ctx, asset.ID, 200)
					for _, caption := range captions {
						if strings.Contains(strings.ToLower(caption.Caption+" "+caption.Model), plain) {
							frameMatches[asset.ID] = struct{}{}
						}
					}
				}
			}
			if prefix == "" || prefix == "tag" || prefix == "category" || prefix == "safety" || prefix == "caption" || prefix == "ocr" {
				out.tagMatches[token] = matches
				out.predictionMatches[token] = matches
				out.faceMatches[token] = faceMatches
			} else {
				out.faceMatches[token] = faceMatches
			}
			if prefix == "" || prefix == "transcript" {
				out.transcriptMatches[token] = transcriptMatches
			}
			if prefix == "" || prefix == "document" {
				out.documentMatches[token] = documentMatches
			}
			if prefix == "" || prefix == "genre" || prefix == "key" || prefix == "tempo" {
				out.audioMatches[token] = audioMatches
			}
			if prefix == "" || prefix == "frame" || prefix == "video-caption" || prefix == "caption" {
				out.frameMatches[token] = frameMatches
			}
		}
	}
	return out
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
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/assets/"), "/")
	parts := strings.Split(rest, "/")
	assetID := parts[0]
	if assetID == "" || len(parts) > 3 {
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
	if len(parts) == 3 && parts[1] == "ocr" {
		if r.Method != http.MethodDelete {
			methodNotAllowed(w)
			return
		}
		if err := s.deps.Store.DeleteAIPrediction(r.Context(), asset.ID, parts[2]); err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "asset_id": asset.ID, "block_id": parts[2]})
		return
	}
	if len(parts) == 2 && parts[1] == "visibility" {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "public": assetIsPublic(asset)})
			return
		}
		if r.Method != http.MethodPatch {
			methodNotAllowed(w)
			return
		}
		var payload struct {
			Public *bool `json:"public"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if payload.Public == nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("public is required"))
			return
		}
		if err := s.deps.Store.UpdateAssetMetadata(r.Context(), asset.ID, nil, map[string]any{"public": *payload.Public}); err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		asset.Metadata["public"] = *payload.Public
		writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "public": *payload.Public})
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "albums":
			albums, err := s.deps.Store.ListAssetAlbums(r.Context(), asset.ID)
			if err != nil {
				writeStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "albums": albums, "total": len(albums)})
		case "ai":
			writeJSON(w, http.StatusOK, s.assetAIRecord(r.Context(), asset))
		case "faces":
			faces, err := s.deps.Store.ListFaceDetections(r.Context(), asset.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "faces": faces, "total": len(faces)})
		case "captions":
			writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "captions": s.assetCaptionRecords(r.Context(), asset.ID)})
		case "classification":
			writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "predictions": s.assetClassificationRecords(r.Context(), asset.ID)})
		case "safety":
			writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "safety": s.assetSafetyRecords(r.Context(), asset.ID)})
		case "ocr":
			blocks := s.assetOCRBlocks(r.Context(), asset.ID)
			summary := ocrSummaryFromBlocks(blocks)
			writeJSON(w, http.StatusOK, map[string]any{
				"asset_id":  asset.ID,
				"blocks":    blocks,
				"full_text": summary.FullText,
				"summary":   summary,
				"engine":    "tesseract_sidecar_contract",
				"note":      "OCR blocks are metadata records. Running OCR is explicit and never writes to originals.",
			})
		case "transcripts":
			transcripts, err := s.deps.Store.ListTranscripts(r.Context(), asset.ID, parsePositiveInt(r.URL.Query().Get("limit"), 20, 100))
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "transcripts": transcripts, "total": len(transcripts)})
		case "audio-features":
			features, err := s.deps.Store.GetAudioFeatures(r.Context(), asset.ID)
			if errors.Is(err, catalog.ErrNotFound) {
				writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "status": "missing", "features": nil})
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "features": features})
		case "frame-captions", "video-analysis":
			captions, err := s.deps.Store.ListVideoFrameCaptions(r.Context(), asset.ID, parsePositiveInt(r.URL.Query().Get("limit"), 200, 1000))
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "frame_captions": captions, "total": len(captions)})
		case "document":
			doc, err := s.deps.Store.GetDocumentText(r.Context(), asset.ID)
			if errors.Is(err, catalog.ErrNotFound) {
				writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "status": "missing", "document": nil})
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"asset_id": asset.ID, "document": doc})
		case "related", "context":
			payload, err := s.assetRelated(r.Context(), asset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, payload)
		default:
			http.NotFound(w, r)
		}
		return
	}
	writeJSON(w, http.StatusOK, s.assetDetail(r.Context(), asset))
}

func (s *Server) handleExplorer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query := r.URL.Query()
	if query.Get("view") == "folders" || query.Get("path") != "" || query.Get("storage") != "" || query.Get("storage_name") != "" {
		limit, _ := strconv.Atoi(query.Get("limit"))
		offset, _ := strconv.Atoi(query.Get("offset"))
		opts := catalog.ExplorerOptions{
			Storage:    firstNonEmpty(query.Get("storage"), query.Get("storage_name")),
			Path:       query.Get("path"),
			Q:          query.Get("q"),
			MediaKind:  query.Get("media_kind"),
			HashStatus: query.Get("hash_status"),
			Extension:  query.Get("extension"),
			Month:      query.Get("month"),
			Limit:      limit,
			Offset:     offset,
			Sort:       query.Get("sort"),
		}
		if store, ok := s.deps.Store.(interface {
			ExplorerView(context.Context, catalog.ExplorerOptions) (catalog.ExplorerView, error)
		}); ok {
			view, err := store.ExplorerView(r.Context(), opts)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, view)
			return
		}
		assets, err := s.deps.Store.ListAssets(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		view, err := catalog.BuildExplorerView(assets, opts)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
		return
	}
	assets, err := s.deps.Store.ListAssets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
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
	trackResult, err := s.mapTrackFeatures(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	assetFeatures, clustering, err := s.mapAssetFeatures(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	features := append(trackResult.Features, assetFeatures...)
	warnings := []string{}
	if trackResult.Truncated {
		warnings = append(warnings, fmt.Sprintf("Track overlay truncated at %d tracks; narrow the date/bbox filter or raise track_limit for this request.", trackResult.Limit))
	}
	if trackResult.HiddenJumps > 0 {
		warnings = append(warnings, fmt.Sprintf("Hidden %d GPS track jump segment(s) over %.0f m. Disable large-jump hiding to inspect raw track geometry.", trackResult.HiddenJumps, trackResult.JumpThresholdM))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type":        "FeatureCollection",
		"features":    features,
		"clustering":  clustering,
		"zoom":        zoom,
		"asset_count": len(assetFeatures),
		"track_overlay": map[string]any{
			"matched":          trackResult.Matched,
			"returned":         trackResult.Returned,
			"truncated":        trackResult.Truncated,
			"limit":            trackResult.Limit,
			"point_budget":     trackResult.PointBudget,
			"points_per_track": trackResult.PointsPerTrack,
			"hide_jumps":       trackResult.HideJumps,
			"jump_threshold_m": trackResult.JumpThresholdM,
			"hidden_jumps":     trackResult.HiddenJumps,
		},
		"warnings": warnings,
	})
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

func (s *Server) handleTranscodingMetricsStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, transcoding.DetectMetrics(r.Context()))
}

func (s *Server) handleTranscodingPresets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		presets := builtInTranscodingPresets(transcoding.Detect(r.Context()))
		custom, err := s.deps.Store.ListTranscodingPresets(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		presets = append(presets, custom...)
		writeJSON(w, http.StatusOK, presets)
	case http.MethodPost:
		if !s.requireWrite(w, r, "transcoding.presets.write") {
			return
		}
		var req catalog.TranscodingPreset
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		for _, preset := range builtInTranscodingPresets(transcoding.Detect(r.Context())) {
			if req.ID == preset.ID {
				writeError(w, http.StatusBadRequest, fmt.Errorf("custom preset cannot replace built-in preset %q", preset.ID))
				return
			}
		}
		req.BuiltIn = false
		if err := validateTranscodingPreset(req, transcoding.Detect(r.Context())); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		preset, err := s.deps.Store.UpsertTranscodingPreset(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, preset)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleTranscodingPresetByID(w http.ResponseWriter, r *http.Request) {
	presetID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/transcoding/presets/"), "/")
	if presetID == "" {
		http.NotFound(w, r)
		return
	}
	if presetID == "validate" {
		s.handleTranscodingPresetValidate(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "transcoding.presets.write") {
		return
	}
	for _, preset := range builtInTranscodingPresets(transcoding.Detect(r.Context())) {
		if preset.ID == presetID {
			writeError(w, http.StatusBadRequest, fmt.Errorf("built-in presets cannot be removed"))
			return
		}
	}
	if err := s.deps.Store.DeleteTranscodingPreset(r.Context(), presetID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	workers := aiWorkerProfiles(r.Context())
	hints := transcoding.AcceleratorHints()
	native := aiNativeRuntimeSummary(workers)
	stats, _ := s.deps.Store.Stats(r.Context())
	aiCounts := s.aiDataCounts(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":            aiWorkersConfigured(workers),
		"inference_running":  false,
		"vector_store":       s.vectorBackendName(),
		"embedding_jobs":     []string{"ai_embed"},
		"accelerator_hints":  hints,
		"native_worker":      native,
		"device_policy":      aiDevicePolicy(hints, native),
		"workers":            workers,
		"model_cache_dir":    ".cartolensia/models",
		"model_policy":       "model downloads are explicit and never use original storage",
		"planned_modalities": []string{"image", "video_frame", "audio_segment", "text_query"},
		"stored_assets":      stats.Assets,
		"ai_counts":          aiCounts,
	})
}

func (s *Server) handleAIAccelerators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, transcoding.AcceleratorHints())
}

func (s *Server) handleAIWorkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workers":       aiWorkerProfiles(r.Context()),
		"configured":    aiWorkersConfigured(aiWorkerProfiles(r.Context())),
		"protocol":      "http_json",
		"device_policy": aiDevicePolicy(transcoding.AcceleratorHints(), aiNativeRuntimeSummary(aiWorkerProfiles(r.Context()))),
		"dummy_worker":  "python -m cartolensia_ai.server --host 127.0.0.1 --port 19090",
	})
}

func (s *Server) handleAIWorkerByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	workerID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/ai/workers/"), "/")
	for _, worker := range aiWorkerProfiles(r.Context()) {
		if worker["id"] == workerID {
			writeJSON(w, http.StatusOK, worker)
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("AI worker %q is not configured", workerID))
}

func (s *Server) handleAIJobRequest(kind string) http.HandlerFunc {
	return s.handleAIJobRequestWithDefault(kind, nil)
}

func (s *Server) handleAIJobRequestWithDefault(kind string, applyDefaults func(*aiJobRequest)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if !s.requireWrite(w, r, "ai.jobs.write") {
			return
		}
		var req aiJobRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		if applyDefaults != nil {
			applyDefaults(&req)
		}
		jobKind := aiCartolensiaJobKind(kind)
		auditPayload := map[string]any{
			"kind":          kind,
			"scope":         req.Scope,
			"asset_id":      req.AssetID,
			"asset_ids":     req.AssetIDs,
			"limit":         req.Limit,
			"bounded_scope": true,
			"note":          "synchronous bounded AI action recorded for Jobs visibility",
		}
		jobCtx, cancelJob := context.WithTimeout(context.Background(), aiJobAuditTimeout(kind, req))
		defer cancelJob()
		auditJob, err := s.deps.Store.EnqueueJob(jobCtx, jobs.New(jobKind, auditPayload))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		_ = jobs.Start(&auditJob)
		auditJob.WorkerID = "api-ai"
		jobs.AddLog(&auditJob, "info", fmt.Sprintf("AI action %s started", kind))
		_ = s.deps.Store.UpdateJob(jobCtx, auditJob)

		updateAuditProgress := func(result aiJobResult) {
			auditJob.ProgressTotal = int64Ptr(result.Targets)
			auditJob.ProgressCurrent = int64(result.Processed + result.Skipped)
			auditJob.Counters.Scanned = int64(result.Targets)
			auditJob.Counters.Updated = int64(result.Stored)
			auditJob.Counters.Errors = int64(len(result.Errors))
			if result.Unsafe > 0 {
				auditJob.Counters.Created = int64(result.Unsafe)
			}
			_ = s.deps.Store.UpdateJob(jobCtx, auditJob)
		}

		result, err := s.runAIJob(jobCtx, r, kind, req, updateAuditProgress)
		if err != nil {
			auditJob.ProgressTotal = int64Ptr(result.Targets)
			auditJob.ProgressCurrent = int64(result.Processed + result.Skipped)
			auditJob.Counters.Scanned = int64(result.Targets)
			auditJob.Counters.Updated = int64(result.Stored)
			auditJob.Counters.Errors = int64(len(result.Errors) + 1)
			jobs.AddLog(&auditJob, "error", err.Error())
			_ = jobs.Fail(&auditJob, err)
			_ = s.deps.Store.UpdateJob(jobCtx, auditJob)
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result.JobID = auditJob.ID
		auditJob.Payload = map[string]any{
			"kind":   kind,
			"scope":  req,
			"result": result,
		}
		auditJob.ProgressTotal = int64Ptr(result.Targets)
		auditJob.ProgressCurrent = int64(result.Processed + result.Skipped)
		auditJob.Counters.Scanned = int64(result.Targets)
		auditJob.Counters.Updated = int64(result.Stored)
		auditJob.Counters.Errors = int64(len(result.Errors))
		if result.Unsafe > 0 {
			auditJob.Counters.Created = int64(result.Unsafe)
		}
		jobs.AddLog(&auditJob, "info", fmt.Sprintf("AI action %s finished: processed %d/%d, stored %d", kind, result.Processed, result.Targets, result.Stored))
		if result.Status == "failed" || result.Status == "not_configured" {
			cause := fmt.Errorf("AI action %s ended with status %s", kind, result.Status)
			if len(result.Errors) > 0 {
				cause = fmt.Errorf("%s", result.Errors[0])
			}
			_ = jobs.Fail(&auditJob, cause)
		} else {
			_ = jobs.Complete(&auditJob)
		}
		_ = s.deps.Store.UpdateJob(jobCtx, auditJob)
		writeJSON(w, http.StatusAccepted, result)
	}
}

func aiJobAuditTimeout(kind string, req aiJobRequest) time.Duration {
	if req.Limit < 0 {
		return 12 * time.Hour
	}
	switch kind {
	case "transcribe_audio":
		return 8 * time.Hour
	case "analyze_audio":
		return 4 * time.Hour
	default:
		return 2 * time.Hour
	}
}

type aiJobRequest struct {
	AssetID         string   `json:"asset_id"`
	AssetIDs        []string `json:"asset_ids"`
	Scope           string   `json:"scope"`
	Limit           int      `json:"limit"`
	SafetyThreshold *float64 `json:"safety_threshold"`
	Language        string   `json:"language"`
	Model           string   `json:"model"`
}

type aiJobResult struct {
	JobID        string           `json:"job_id,omitempty"`
	Kind         string           `json:"kind"`
	Status       string           `json:"status"`
	WorkerID     string           `json:"worker_id,omitempty"`
	Endpoint     string           `json:"endpoint,omitempty"`
	Targets      int              `json:"targets"`
	Processed    int              `json:"processed"`
	Skipped      int              `json:"skipped"`
	Stored       int              `json:"stored"`
	Unsafe       int              `json:"unsafe,omitempty"`
	Errors       []string         `json:"errors,omitempty"`
	Results      []map[string]any `json:"results,omitempty"`
	Scope        aiJobRequest     `json:"scope"`
	SafetyNote   string           `json:"safe_note"`
	SkippedKinds map[string]int   `json:"skipped_kinds,omitempty"`
}

type aiPredictionPayload struct {
	Label      string         `json:"label"`
	Confidence *float64       `json:"confidence"`
	Metadata   map[string]any `json:"metadata"`
}

type aiInferencePayload struct {
	Status      string                `json:"status"`
	Endpoint    string                `json:"endpoint"`
	Predictions []aiPredictionPayload `json:"predictions"`
	Reason      string                `json:"reason"`
	Metadata    map[string]any        `json:"metadata"`
}

func (s *Server) runAIJob(ctx context.Context, r *http.Request, kind string, req aiJobRequest, onProgress func(aiJobResult)) (aiJobResult, error) {
	return s.runAIJobWithMediaURL(ctx, kind, req, func(asset catalog.Asset) string {
		return s.aiMediaURL(r, asset.ID)
	}, onProgress)
}

func (s *Server) runAIJobWithMediaURL(ctx context.Context, kind string, req aiJobRequest, mediaURL func(catalog.Asset) string, onProgress func(aiJobResult)) (aiJobResult, error) {
	endpoint, workerID, ok := aiConfiguredWorkerEndpoint(ctx)
	result := aiJobResult{
		Kind:         kind,
		Status:       "not_configured",
		WorkerID:     workerID,
		Endpoint:     endpoint,
		Scope:        req,
		SafetyNote:   "AI reads scoped assets through Cartolensia read-only media URLs; outputs are stored in DB metadata only",
		SkippedKinds: map[string]int{},
	}
	if !ok {
		result.Errors = append(result.Errors, "no AI sidecar is reachable at "+endpoint)
		if onProgress != nil {
			onProgress(result)
		}
		return result, nil
	}
	assets, err := s.resolveAIAssets(ctx, kind, req)
	if err != nil {
		return result, err
	}
	result.Targets = len(assets)
	if onProgress != nil {
		onProgress(result)
	}
	apiPath := aiSidecarPath(kind)
	if apiPath == "" {
		return result, fmt.Errorf("unsupported AI job kind %q", kind)
	}
	if mediaURL == nil {
		mediaURL = func(asset catalog.Asset) string { return s.aiWorkerMediaURL(asset.ID) }
	}
	for _, asset := range assets {
		if !aiSupportsAsset(kind, asset) {
			result.Skipped++
			result.SkippedKinds[asset.MediaKind]++
			if onProgress != nil {
				onProgress(result)
			}
			continue
		}
		payload := map[string]any{
			"asset_id":  asset.ID,
			"media_url": mediaURL(asset),
			"options": map[string]any{
				"safety_threshold": req.SafetyThreshold,
				"language":         req.Language,
				"model":            req.Model,
				"suffix":           assetAISuffix(asset),
			},
		}
		timeout := 3 * time.Minute
		if kind == "transcribe_audio" {
			timeout = 45 * time.Minute
		} else if kind == "analyze_audio" {
			timeout = 10 * time.Minute
		}
		response, err := callAISidecarWithTimeout(ctx, endpoint+apiPath, payload, timeout)
		if err != nil {
			result.Errors = append(result.Errors, asset.ID+": "+err.Error())
			_ = s.markAIAssetTaskStatus(ctx, asset.ID, workerID, kind, "failed", 0, err.Error(), nil)
			if onProgress != nil {
				onProgress(result)
			}
			continue
		}
		if response.Status != "ok" {
			reason := strings.TrimSpace(response.Reason)
			if reason == "" {
				reason = "AI sidecar returned status " + response.Status
			}
			result.Errors = append(result.Errors, asset.ID+": "+reason)
			_ = s.markAIAssetTaskStatus(ctx, asset.ID, workerID, kind, "failed", 0, reason, summarizeAIMetadata(response.Metadata))
			if onProgress != nil {
				onProgress(result)
			}
			continue
		}
		stored, unsafe := s.persistAIResponse(ctx, workerID, kind, asset.ID, response, req)
		_ = s.markAIAssetTaskStatus(ctx, asset.ID, workerID, kind, "succeeded", stored, "", map[string]any{
			"model":  aiModelName(response.Metadata, kind),
			"reason": response.Reason,
		})
		result.Processed++
		result.Stored += stored
		if unsafe {
			result.Unsafe++
		}
		result.Results = append(result.Results, map[string]any{
			"asset_id":     asset.ID,
			"status":       response.Status,
			"reason":       response.Reason,
			"predictions":  response.Predictions,
			"metadata":     summarizeAIMetadata(response.Metadata),
			"stored_count": stored,
		})
		if onProgress != nil {
			onProgress(result)
		}
	}
	result.Status = "completed"
	if len(result.Errors) > 0 && result.Processed == 0 {
		result.Status = "failed"
	}
	return result, nil
}

func (s *Server) markAIAssetTaskStatus(ctx context.Context, assetID, workerID, kind, status string, stored int, errorText string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	_, err := s.deps.Store.UpsertAIAssetTaskStatus(ctx, catalog.AIAssetTaskStatus{
		AssetID:     assetID,
		Task:        kind,
		Status:      status,
		WorkerID:    workerID,
		ModelName:   stringFromAny(metadata["model"]),
		StoredCount: stored,
		Error:       errorText,
		Metadata:    metadata,
	})
	return err
}

func (s *Server) resolveAIAssets(ctx context.Context, kind string, req aiJobRequest) ([]catalog.Asset, error) {
	ids := compactStrings(req.AssetIDs)
	if req.AssetID != "" {
		ids = append(ids, req.AssetID)
	}
	if len(ids) > 0 {
		out := make([]catalog.Asset, 0, len(ids))
		for _, assetID := range uniqueStrings(ids) {
			asset, err := s.deps.Store.GetAsset(ctx, assetID)
			if err != nil {
				return out, err
			}
			out = append(out, asset)
		}
		return out, nil
	}
	scope := strings.TrimSpace(strings.ToLower(req.Scope))
	if scope == "" || scope == "selected" {
		return []catalog.Asset{}, nil
	}
	if scope != "current" && scope != "current_indexed" && scope != "indexed" {
		return nil, fmt.Errorf("unsupported AI scope %q", req.Scope)
	}
	limit := req.Limit
	if limit == 0 {
		limit = 250
	}
	mediaKinds := aiSupportedMediaKinds(kind)
	if len(mediaKinds) == 0 {
		return []catalog.Asset{}, nil
	}
	pageSize := 500
	if limit < 0 {
		var out []catalog.Asset
		seen := map[string]struct{}{}
		for _, mediaKind := range mediaKinds {
			for offset := 0; ; offset += pageSize {
				page, err := s.deps.Store.QueryAssets(ctx, catalog.AssetQuery{MediaKind: mediaKind, Limit: pageSize, Offset: offset, Sort: "taken_at"})
				if err != nil {
					return nil, err
				}
				for _, asset := range page.Assets {
					if _, ok := seen[asset.ID]; ok {
						continue
					}
					seen[asset.ID] = struct{}{}
					out = append(out, asset)
				}
				if offset+len(page.Assets) >= page.Page.Total || len(page.Assets) == 0 {
					break
				}
			}
		}
		return out, nil
	}
	out := make([]catalog.Asset, 0, limit)
	seen := map[string]struct{}{}
	remaining := limit
	for _, mediaKind := range mediaKinds {
		if remaining <= 0 {
			break
		}
		for offset := 0; remaining > 0; offset += pageSize {
			batchLimit := pageSize
			if remaining < batchLimit {
				batchLimit = remaining
			}
			page, err := s.deps.Store.QueryAssets(ctx, catalog.AssetQuery{MediaKind: mediaKind, Limit: batchLimit, Offset: offset, Sort: "taken_at"})
			if err != nil {
				return nil, err
			}
			for _, asset := range page.Assets {
				if _, ok := seen[asset.ID]; ok {
					continue
				}
				seen[asset.ID] = struct{}{}
				out = append(out, asset)
				remaining--
				if remaining <= 0 {
					break
				}
			}
			if offset+len(page.Assets) >= page.Page.Total || len(page.Assets) == 0 {
				break
			}
		}
	}
	return out, nil
}

func aiConfiguredWorkerEndpoint(ctx context.Context) (endpoint, workerID string, ok bool) {
	configuredEndpoint := aiWorkerEndpoint()
	for _, worker := range aiWorkerProfiles(ctx) {
		configured, _ := worker["configured"].(bool)
		rawEndpoint, _ := worker["endpoint"].(string)
		rawID, _ := worker["id"].(string)
		if configured && rawEndpoint != "" {
			return strings.TrimRight(rawEndpoint, "/"), rawID, true
		}
	}
	return strings.TrimRight(configuredEndpoint, "/"), "ai-configured", false
}

func aiWorkerEndpoint() string {
	if endpoint := strings.TrimSpace(os.Getenv("CARTOLENSIA_AI_WORKER_ENDPOINT")); endpoint != "" {
		return strings.TrimRight(endpoint, "/")
	}
	return strings.TrimRight(runtimeStringSetting("ai.worker_endpoint", "http://127.0.0.1:19090"), "/")
}

func aiSidecarPath(kind string) string {
	switch kind {
	case "classify_image":
		return "/classify-image"
	case "detect_faces":
		return "/detect-faces"
	case "safety_nsfw":
		return "/safety-nsfw"
	case "embed_image":
		return "/embed-image"
	case "describe_image":
		return "/describe-image"
	case "ocr_image":
		return "/ocr-image"
	case "transcribe_audio":
		return "/transcribe-audio"
	case "analyze_audio":
		return "/analyze-audio"
	default:
		return ""
	}
}

func aiSupportedMediaKinds(kind string) []string {
	switch kind {
	case "classify_image", "detect_faces", "safety_nsfw", "embed_image", "describe_image", "ocr_image":
		return []string{"photo"}
	case "transcribe_audio":
		return []string{"audio", "video"}
	case "analyze_audio":
		return []string{"audio"}
	default:
		return nil
	}
}

func aiSupportsAsset(kind string, asset catalog.Asset) bool {
	switch kind {
	case "classify_image", "detect_faces", "safety_nsfw", "embed_image", "describe_image", "ocr_image":
		return asset.MediaKind == "photo" && assetHasAISupportedExtension(kind, asset)
	case "transcribe_audio":
		return (asset.MediaKind == "audio" || asset.MediaKind == "video") && assetHasAISupportedExtension(kind, asset)
	case "analyze_audio":
		return asset.MediaKind == "audio" && assetHasAISupportedExtension(kind, asset)
	default:
		return false
	}
}

func assetHasAISupportedExtension(kind string, asset catalog.Asset) bool {
	allowed := aiSupportedAssetExtensions(kind, asset.MediaKind)
	if len(allowed) == 0 {
		return true
	}
	allowedSet := map[string]struct{}{}
	for _, ext := range allowed {
		allowedSet[ext] = struct{}{}
	}
	for _, loc := range asset.Locations {
		ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(loc.Extension)), ".")
		if _, ok := allowedSet[ext]; ok {
			return true
		}
	}
	return false
}

func aiSupportedAssetExtensions(kind, mediaKind string) []string {
	switch kind {
	case "classify_image", "safety_nsfw", "embed_image", "describe_image":
		return []string{"jpg", "jpeg", "png", "webp", "bmp", "gif", "tif", "tiff"}
	case "ocr_image", "detect_faces":
		return []string{"jpg", "jpeg", "png", "webp", "bmp", "tif", "tiff"}
	case "transcribe_audio":
		if mediaKind == "video" {
			return []string{"mp4", "mov", "m4v", "webm", "mkv", "avi", "3gp", "3gpp"}
		}
		return []string{"mp3", "wav", "flac", "ogg", "oga", "opus", "m4a", "aac", "amr", "3gp", "3gpp", "webm"}
	case "analyze_audio":
		return []string{"mp3", "wav", "flac", "ogg", "oga", "opus", "m4a", "aac", "amr", "3gp", "3gpp", "webm"}
	default:
		return nil
	}
}

func assetAISuffix(asset catalog.Asset) string {
	for _, loc := range asset.Locations {
		if loc.Extension != "" {
			return "." + strings.TrimPrefix(strings.ToLower(loc.Extension), ".")
		}
	}
	if asset.MediaKind != "" {
		return "." + asset.MediaKind
	}
	return ".media"
}

func (s *Server) aiMediaURL(r *http.Request, assetID string) string {
	if token := strings.TrimSpace(os.Getenv("CARTOLENSIA_AI_MEDIA_TOKEN")); token != "" {
		base := strings.TrimRight(strings.TrimSpace(os.Getenv("CARTOLENSIA_AI_MEDIA_BASE_URL")), "/")
		if base == "" {
			base = "http://127.0.0.1" + listenPortForURL(s.deps.Config.HTTP.Addr, ":18080")
		}
		return base + "/api/v1/ai-media/" + url.PathEscape(assetID) + "/original?token=" + url.QueryEscape(token)
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = strings.TrimPrefix(s.deps.Config.HTTP.Addr, "http://")
	}
	return scheme + "://" + host + "/api/v1/media/" + assetID + "/original"
}

func (s *Server) aiWorkerMediaURL(assetID string) string {
	if token := strings.TrimSpace(os.Getenv("CARTOLENSIA_AI_MEDIA_TOKEN")); token != "" {
		base := strings.TrimRight(strings.TrimSpace(os.Getenv("CARTOLENSIA_AI_MEDIA_BASE_URL")), "/")
		if base == "" {
			base = "http://127.0.0.1" + listenPortForURL(s.deps.Config.HTTP.Addr, ":18080")
		}
		return base + "/api/v1/ai-media/" + url.PathEscape(assetID) + "/original?token=" + url.QueryEscape(token)
	}
	base := "http://127.0.0.1" + listenPortForURL(s.deps.Config.HTTP.Addr, ":18080")
	return base + "/api/v1/media/" + url.PathEscape(assetID) + "/original"
}

func listenPortForURL(addr, fallback string) string {
	if addr == "" {
		return fallback
	}
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		return ":" + port
	}
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	return fallback
}

func callAISidecar(ctx context.Context, endpoint string, payload map[string]any) (aiInferencePayload, error) {
	return callAISidecarWithTimeout(ctx, endpoint, payload, 3*time.Minute)
}

func callAISidecarWithTimeout(ctx context.Context, endpoint string, payload map[string]any, timeout time.Duration) (aiInferencePayload, error) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return aiInferencePayload{}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, &body)
	if err != nil {
		return aiInferencePayload{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return aiInferencePayload{}, err
	}
	defer resp.Body.Close()
	var out aiInferencePayload
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<20)).Decode(&out); err != nil {
		return out, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("AI sidecar returned HTTP %d: %s", resp.StatusCode, out.Reason)
	}
	if out.Status != "ok" {
		return out, nil
	}
	return out, nil
}

func (s *Server) persistAIResponse(ctx context.Context, workerID, kind, assetID string, response aiInferencePayload, req aiJobRequest) (stored int, unsafe bool) {
	if response.Status != "ok" {
		return 0, false
	}
	modelName := aiModelName(response.Metadata, kind)
	for _, prediction := range response.Predictions {
		_, err := s.deps.Store.CreateAIPrediction(ctx, catalog.AIPrediction{
			AssetID:    assetID,
			WorkerID:   workerID,
			Task:       kind,
			Label:      prediction.Label,
			Confidence: prediction.Confidence,
			ModelName:  modelName,
			Metadata:   prediction.Metadata,
		})
		if err == nil {
			stored++
		}
	}
	switch kind {
	case "classify_image":
		if len(response.Predictions) > 0 {
			top := response.Predictions[0]
			if top.Confidence == nil || *top.Confidence >= 0.10 {
				if _, err := s.deps.Store.UpsertAssetTag(ctx, catalog.AssetTag{
					AssetID:    assetID,
					Tag:        top.Label,
					Source:     "ai_classify",
					Confidence: top.Confidence,
					Metadata:   map[string]any{"model": modelName, "category": top.Label},
				}); err == nil {
					stored++
				}
			}
		}
	case "safety_nsfw":
		unsafe = aiNeedsReview(response.Metadata, req)
		if len(response.Predictions) > 0 {
			top := response.Predictions[0]
			tag := "safety:" + strings.ToLower(strings.ReplaceAll(top.Label, " ", "_"))
			if unsafe {
				tag = "safety:needs_review"
			}
			if _, err := s.deps.Store.UpsertAssetTag(ctx, catalog.AssetTag{
				AssetID:    assetID,
				Tag:        tag,
				Source:     "ai_safety",
				Confidence: top.Confidence,
				Metadata:   map[string]any{"model": modelName, "needs_review": unsafe},
			}); err == nil {
				stored++
			}
		}
		if unsafe {
			if err := s.addAssetToPotentiallyUnsafeAlbum(ctx, assetID); err == nil {
				stored++
			}
		}
	case "detect_faces":
		if faces, ok := response.Metadata["faces"].([]any); ok {
			for _, item := range faces {
				face, ok := item.(map[string]any)
				if !ok {
					continue
				}
				conf := floatPtrFromAny(face["confidence"])
				_, err := s.deps.Store.CreateFaceDetection(ctx, catalog.FaceDetection{
					AssetID:    assetID,
					X:          floatFromAny(face["x"]),
					Y:          floatFromAny(face["y"]),
					Width:      floatFromAny(face["width"]),
					Height:     floatFromAny(face["height"]),
					Confidence: conf,
					Metadata:   map[string]any{"model": modelName, "local_cluster_only": true},
				})
				if err == nil {
					stored++
				}
			}
		}
	case "embed_image":
		embedding := floatSliceFromAny(response.Metadata["embedding"])
		if len(embedding) > 0 {
			modelID := "openclip-vit-b-32-laion2b-s34b-b79k"
			_, _ = s.deps.Store.UpsertEmbeddingModel(ctx, catalog.EmbeddingModel{
				ID:        modelID,
				Modality:  "image",
				ModelName: modelName,
				Version:   "local",
				Dimension: len(embedding),
				Metadata:  map[string]any{"backend": "local_json", "plugin_owner": "base-ai"},
			})
			if _, err := s.deps.Store.UpsertAssetEmbedding(ctx, catalog.AssetEmbedding{
				AssetID:   assetID,
				ModelID:   modelID,
				Modality:  "image",
				SourceRef: "asset",
				Vector:    embedding,
				Metadata:  map[string]any{"model": modelName},
			}); err == nil {
				stored++
			}
		}
	case "describe_image":
		if len(response.Predictions) > 0 {
			caption := response.Predictions[0].Label
			if caption != "" {
				if _, err := s.deps.Store.UpsertAssetTag(ctx, catalog.AssetTag{
					AssetID:  assetID,
					Tag:      "caption:" + caption,
					Source:   "ai_caption",
					Metadata: map[string]any{"model": modelName, "caption": caption},
				}); err == nil {
					stored++
				}
			}
		}
	case "ocr_image":
		if blocks, ok := response.Metadata["blocks"].([]any); ok {
			for _, item := range blocks {
				block, ok := item.(map[string]any)
				if !ok {
					continue
				}
				text := strings.TrimSpace(stringFromAny(block["text"]))
				if text == "" {
					continue
				}
				conf := floatPtrFromAny(block["confidence"])
				metadata := map[string]any{
					"engine":   stringFromAny(block["engine"]),
					"language": stringFromAny(block["language"]),
					"x":        floatFromAny(block["x"]),
					"y":        floatFromAny(block["y"]),
					"width":    floatFromAny(block["width"]),
					"height":   floatFromAny(block["height"]),
				}
				if metadata["engine"] == "" {
					metadata["engine"] = "tesseract_or_sidecar"
				}
				_, err := s.deps.Store.CreateAIPrediction(ctx, catalog.AIPrediction{
					AssetID:    assetID,
					WorkerID:   workerID,
					Task:       "ocr_image",
					Label:      text,
					Confidence: conf,
					ModelName:  modelName,
					Metadata:   metadata,
				})
				if err == nil {
					stored++
				}
			}
		}
	case "transcribe_audio":
		asset, err := s.deps.Store.GetAsset(ctx, assetID)
		sourceKind := "audio"
		if err == nil && asset.MediaKind == "video" {
			sourceKind = "video_audio"
		}
		fullText := strings.TrimSpace(stringFromAny(response.Metadata["text"]))
		segmentItems, _ := response.Metadata["segments"].([]any)
		segments := make([]catalog.TranscriptSegment, 0, len(segmentItems))
		for _, raw := range segmentItems {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(stringFromAny(item["text"]))
			if text == "" {
				continue
			}
			segments = append(segments, catalog.TranscriptSegment{
				StartMS:    int64FromAny(item["start_ms"]),
				EndMS:      int64FromAny(item["end_ms"]),
				Text:       text,
				Confidence: floatPtrFromAny(item["confidence"]),
				Metadata: map[string]any{
					"index":          item["index"],
					"avg_logprob":    item["avg_logprob"],
					"no_speech_prob": item["no_speech_prob"],
					"engine":         response.Metadata["engine"],
					"provider":       response.Metadata["provider"],
				},
			})
		}
		if fullText == "" && len(segments) > 0 {
			parts := make([]string, 0, len(segments))
			for _, segment := range segments {
				parts = append(parts, segment.Text)
			}
			fullText = strings.Join(parts, "\n")
		}
		metadata := summarizeAIMetadata(response.Metadata)
		delete(metadata, "segments")
		delete(metadata, "text")
		transcript, err := s.deps.Store.UpsertTranscript(ctx, catalog.Transcript{
			AssetID:    assetID,
			SourceKind: sourceKind,
			Language:   stringFromAny(response.Metadata["language"]),
			Model:      modelName,
			FullText:   fullText,
			Metadata:   metadata,
		}, segments)
		if err == nil {
			stored += 1 + len(transcript.Segments)
		}
	case "analyze_audio":
		metadata := summarizeAIMetadata(response.Metadata)
		features := catalog.AudioFeatures{
			AssetID:          assetID,
			DurationSeconds:  floatPtrFromOptionalAny(response.Metadata["duration_seconds"]),
			TempoBPM:         floatPtrFromOptionalAny(response.Metadata["tempo_bpm"]),
			Key:              stringFromAny(response.Metadata["key"]),
			Mode:             stringFromAny(response.Metadata["mode"]),
			Loudness:         floatPtrFromOptionalAny(response.Metadata["loudness"]),
			SpeechMusicRatio: floatPtrFromOptionalAny(response.Metadata["speech_music_ratio"]),
			GenreLabels:      stringSliceFromAny(response.Metadata["genre_labels"]),
			Model:            modelName,
			Metadata:         metadata,
		}
		if _, err := s.deps.Store.UpsertAudioFeatures(ctx, features); err == nil {
			stored++
		}
	}
	return stored, unsafe
}

func aiModelName(metadata map[string]any, fallback string) string {
	if text, ok := metadata["model"].(string); ok && text != "" {
		return text
	}
	return fallback
}

func aiNeedsReview(metadata map[string]any, req aiJobRequest) bool {
	if value, ok := metadata["needs_review"].(bool); ok {
		return value
	}
	score := floatFromAny(metadata["unsafe_score"])
	threshold := 0.75
	if req.SafetyThreshold != nil {
		threshold = *req.SafetyThreshold
	} else if raw := floatFromAny(metadata["threshold"]); raw > 0 {
		threshold = raw
	}
	return score >= threshold
}

func summarizeAIMetadata(metadata map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range metadata {
		if key == "embedding" {
			out["embedding_dimension"] = len(floatSliceFromAny(value))
			continue
		}
		out[key] = value
	}
	return out
}

func summarizeAssetEmbeddings(embeddings []catalog.AssetEmbedding) []map[string]any {
	out := make([]map[string]any, 0, len(embeddings))
	for _, embedding := range embeddings {
		metadata := embedding.Metadata
		if metadata == nil {
			metadata = map[string]any{}
		}
		out = append(out, map[string]any{
			"id":         embedding.ID,
			"asset_id":   embedding.AssetID,
			"model_id":   embedding.ModelID,
			"modality":   embedding.Modality,
			"source_ref": embedding.SourceRef,
			"dimension":  len(embedding.Vector),
			"metadata":   metadata,
			"created_at": embedding.CreatedAt,
		})
	}
	return out
}

func (s *Server) addAssetToPotentiallyUnsafeAlbum(ctx context.Context, assetID string) error {
	album, err := s.ensurePotentiallyUnsafeAlbum(ctx)
	if err != nil {
		return err
	}
	return s.deps.Store.AddAlbumItems(ctx, album.ID, []string{assetID})
}

func (s *Server) ensurePotentiallyUnsafeAlbum(ctx context.Context) (catalog.Album, error) {
	albums, err := s.deps.Store.ListAlbums(ctx, catalog.AlbumQuery{Tree: true, Limit: 1000})
	if err == nil {
		for _, album := range albums {
			if album.Slug == "potentially-unsafe" || strings.EqualFold(album.Title, "Potentially Unsafe") {
				return album, nil
			}
		}
	}
	return s.deps.Store.CreateAlbum(ctx, catalog.Album{
		Slug:        "potentially-unsafe",
		Title:       "Potentially Unsafe",
		Description: "Virtual local review album populated by the safety classifier. Original files are never moved or modified.",
		SortOrder:   9000,
	})
}

func (s *Server) handleAISummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"counts":       s.aiDataCounts(r.Context()),
		"workers":      aiWorkerProfiles(r.Context()),
		"vector_store": s.vectorStatusPayload(r.Context()),
		"review_album": s.potentiallyUnsafeAlbumSummary(r.Context()),
	})
}

func (s *Server) handleAITags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	tags, err := s.deps.Store.ListAssetTags(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	type group struct {
		Tag        string  `json:"tag"`
		Source     string  `json:"source"`
		Count      int     `json:"count"`
		Confidence float64 `json:"avg_confidence,omitempty"`
	}
	groups := map[string]*group{}
	confCounts := map[string]int{}
	for _, tag := range tags {
		key := tag.Source + "\x00" + tag.Tag
		item := groups[key]
		if item == nil {
			item = &group{Tag: tag.Tag, Source: tag.Source}
			groups[key] = item
		}
		item.Count++
		if tag.Confidence != nil {
			item.Confidence += *tag.Confidence
			confCounts[key]++
		}
	}
	out := make([]group, 0, len(groups))
	for key, item := range groups {
		if confCounts[key] > 0 {
			item.Confidence = item.Confidence / float64(confCounts[key])
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Tag < out[j].Tag
		}
		return out[i].Count > out[j].Count
	})
	writeJSON(w, http.StatusOK, map[string]any{"tags": out, "total": len(tags)})
}

func (s *Server) handleAIFaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	faces, err := s.deps.Store.ListFaceDetections(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if query != "" {
		filtered := faces[:0]
		for _, face := range faces {
			text := strings.ToLower(strings.Join([]string{
				face.ID,
				face.AssetID,
				face.ClusterID,
				face.PluginID,
				stringFromMetadata(face.Metadata),
			}, " "))
			if strings.Contains(text, query) {
				filtered = append(filtered, face)
			}
		}
		faces = filtered
	}
	total := len(faces)
	limit := intQuery(r.URL.Query(), "limit", 100, 1, 5000)
	offset := intQuery(r.URL.Query(), "offset", 0, 0, 1000000)
	faces = slicePage(faces, limit, offset)
	writeJSON(w, http.StatusOK, map[string]any{"faces": faces, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) handleFaceClusters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	clusters, assetsByCluster, err := s.faceClusterSummaries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"clusters":          clusters,
		"total":             len(clusters),
		"assets_by_cluster": assetsByCluster,
		"provisional_note":  "Unassigned detections are grouped provisionally by source asset until reviewed or named.",
	})
}

func (s *Server) handleFaceClusterByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/faces/clusters/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	clusterID := parts[0]
	if len(parts) == 2 && parts[1] == "assets" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		s.handleFaceClusterAssets(w, r, clusterID)
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "faces.cluster.write") {
		return
	}
	var req struct {
		Label string         `json:"label"`
		Note  string         `json:"note"`
		Meta  map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cluster, err := s.materializeFaceCluster(r.Context(), clusterID, strings.TrimSpace(req.Label), req.Meta)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, catalog.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, cluster)
}

func (s *Server) handleFaceDetections(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/faces/detections" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "faces.detection.write") {
		return
	}
	var req struct {
		AssetID    string  `json:"asset_id"`
		X          float64 `json:"x"`
		Y          float64 `json:"y"`
		Width      float64 `json:"width"`
		Height     float64 `json:"height"`
		Confidence float64 `json:"confidence"`
		Label      string  `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.AssetID = strings.TrimSpace(req.AssetID)
	if req.AssetID == "" || req.Width <= 0 || req.Height <= 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("asset_id and positive face rectangle dimensions are required"))
		return
	}
	if _, err := s.deps.Store.GetAsset(r.Context(), req.AssetID); err != nil {
		writeStoreError(w, err)
		return
	}
	confidence := req.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = 1
	}
	label := strings.TrimSpace(req.Label)
	clusterID := ""
	if label != "" {
		cluster, err := s.deps.Store.UpsertFaceCluster(r.Context(), catalog.FaceCluster{
			Label:    label,
			Metadata: map[string]any{"manual": true, "local_only": true},
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		clusterID = cluster.ID
	}
	confidencePtr := confidence
	face, err := s.deps.Store.CreateFaceDetection(r.Context(), catalog.FaceDetection{
		ID:         id.NewUUID(),
		AssetID:    req.AssetID,
		PluginID:   "manual_face",
		X:          req.X,
		Y:          req.Y,
		Width:      req.Width,
		Height:     req.Height,
		Confidence: &confidencePtr,
		ClusterID:  clusterID,
		Metadata: map[string]any{
			"manual":        true,
			"label":         label,
			"review_status": "manual",
			"local_only":    true,
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if clusterID != "" {
		_, _ = s.deps.Store.UpsertFaceCluster(r.Context(), catalog.FaceCluster{
			ID:                   clusterID,
			Label:                label,
			RepresentativeFaceID: face.ID,
			Metadata:             map[string]any{"manual": true, "local_only": true},
		})
	}
	writeJSON(w, http.StatusCreated, face)
}

func (s *Server) handleFaceDetectionByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/faces/detections/"), "/")
	parts := strings.Split(path, "/")
	if (len(parts) != 1 && len(parts) != 2) || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "thumbnail" && r.Method == http.MethodGet {
		s.handleFaceDetectionThumbnail(w, r, parts[0])
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if !s.requireWrite(w, r, "faces.detection.write") {
			return
		}
		face, err := s.updateFaceDetectionMetadata(r.Context(), parts[0], map[string]any{"ignored": true, "deleted": true, "review_status": "deleted"})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, catalog.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, face)
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "faces.detection.write") {
		return
	}
	switch parts[1] {
	case "ignore":
		face, err := s.updateFaceDetectionMetadata(r.Context(), parts[0], map[string]any{"ignored": true, "review_status": "ignored"})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, catalog.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, face)
	case "assign":
		var req struct {
			ClusterID string `json:"cluster_id"`
			Label     string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		face, err := s.assignFaceDetection(r.Context(), parts[0], req.ClusterID, req.Label)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, catalog.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, face)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleFaceDetectionThumbnail(w http.ResponseWriter, r *http.Request, detectionID string) {
	faces, err := s.deps.Store.ListFaceDetections(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var face catalog.FaceDetection
	for _, item := range faces {
		if item.ID == detectionID {
			face = item
			break
		}
	}
	if face.ID == "" {
		writeError(w, http.StatusNotFound, catalog.ErrNotFound)
		return
	}
	asset, err := s.deps.Store.GetAsset(r.Context(), face.AssetID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	loc, ok := catalog.FirstLocation(asset)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("asset has no storage location"))
		return
	}
	file, _, err := s.deps.Registry.OpenByURL(loc.StorageURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer file.Close()
	source, _, err := image.Decode(file)
	if err != nil {
		http.Redirect(w, r, "/api/v1/media/"+url.PathEscape(face.AssetID)+"/preview", http.StatusTemporaryRedirect)
		return
	}
	bounds := source.Bounds()
	x := int(math.Floor(face.X))
	y := int(math.Floor(face.Y))
	width := int(math.Ceil(face.Width))
	height := int(math.Ceil(face.Height))
	pad := int(math.Ceil(float64(maxInt(width, height)) * 0.35))
	crop := image.Rect(x-pad, y-pad, x+width+pad, y+height+pad).Intersect(bounds)
	if crop.Empty() {
		crop = bounds
	}
	out := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	draw.Draw(out, out.Bounds(), source, crop.Min, draw.Src)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=600")
	if err := jpeg.Encode(w, out, &jpeg.Options{Quality: 84}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
}

func (s *Server) handleFaceClusterAssets(w http.ResponseWriter, r *http.Request, clusterID string) {
	faces, err := s.faceDetectionsForCluster(r.Context(), clusterID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, catalog.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	assetIDs := map[string]struct{}{}
	for _, face := range faces {
		if metadataBool(face.Metadata, "ignored") {
			continue
		}
		assetIDs[face.AssetID] = struct{}{}
	}
	assets := make([]catalog.Asset, 0, len(assetIDs))
	for assetID := range assetIDs {
		asset, err := s.deps.Store.GetAsset(r.Context(), assetID)
		if err == nil {
			assets = append(assets, asset)
		}
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].DisplayName < assets[j].DisplayName })
	writeJSON(w, http.StatusOK, map[string]any{"cluster_id": clusterID, "faces": faces, "assets": assets, "total": len(assets)})
}

func (s *Server) handleAISafety(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	tags, err := s.deps.Store.ListAssetTags(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]catalog.AssetTag, 0)
	for _, tag := range tags {
		if strings.HasPrefix(strings.ToLower(tag.Tag), "safety:") {
			out = append(out, tag)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{
		"candidates":   out,
		"total":        len(out),
		"review_album": s.potentiallyUnsafeAlbumSummary(r.Context()),
	})
}

func (s *Server) faceClusterSummaries(ctx context.Context) ([]catalog.FaceCluster, map[string]int, error) {
	faces, err := s.deps.Store.ListFaceDetections(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	stored, err := s.deps.Store.ListFaceClusters(ctx)
	if err != nil {
		return nil, nil, err
	}
	storedByID := make(map[string]catalog.FaceCluster, len(stored))
	for _, cluster := range stored {
		if cluster.Metadata == nil {
			cluster.Metadata = map[string]any{}
		}
		storedByID[cluster.ID] = cluster
	}
	type aggregate struct {
		cluster               catalog.FaceCluster
		assetIDs              map[string]struct{}
		faceIDs               []string
		createdAt             time.Time
		representativeAssetID string
		representativeBox     map[string]any
	}
	groups := map[string]*aggregate{}
	for _, face := range faces {
		key := strings.TrimSpace(face.ClusterID)
		if key == "" {
			key = "asset:" + face.AssetID
		}
		group := groups[key]
		if group == nil {
			cluster := storedByID[key]
			if cluster.ID == "" {
				cluster.ID = key
				cluster.Metadata = map[string]any{"provisional": strings.HasPrefix(key, "asset:")}
				if strings.HasPrefix(key, "asset:") {
					cluster.Label = "Unassigned faces"
				}
			}
			group = &aggregate{cluster: cluster, assetIDs: map[string]struct{}{}, createdAt: face.CreatedAt}
			groups[key] = group
		}
		group.cluster.FaceCount++
		group.faceIDs = append(group.faceIDs, face.ID)
		group.assetIDs[face.AssetID] = struct{}{}
		if metadataBool(face.Metadata, "ignored") {
			group.cluster.IgnoredCount++
		}
		if group.cluster.RepresentativeFaceID == "" {
			group.cluster.RepresentativeFaceID = face.ID
		}
		if group.representativeAssetID == "" && !metadataBool(face.Metadata, "ignored") {
			group.representativeAssetID = face.AssetID
			group.representativeBox = map[string]any{"x": face.X, "y": face.Y, "width": face.Width, "height": face.Height, "confidence": face.Confidence}
		}
		if group.createdAt.IsZero() || face.CreatedAt.Before(group.createdAt) {
			group.createdAt = face.CreatedAt
		}
		if group.cluster.CreatedAt.IsZero() {
			group.cluster.CreatedAt = group.createdAt
		}
		if group.cluster.UpdatedAt.IsZero() || face.CreatedAt.After(group.cluster.UpdatedAt) {
			group.cluster.UpdatedAt = face.CreatedAt
		}
	}
	clusters := make([]catalog.FaceCluster, 0, len(groups))
	assetsByCluster := make(map[string]int, len(groups))
	for id, group := range groups {
		group.cluster.AssetCount = len(group.assetIDs)
		if group.cluster.Metadata == nil {
			group.cluster.Metadata = map[string]any{}
		}
		group.cluster.Metadata["face_ids"] = group.faceIDs
		group.cluster.Metadata["provisional"] = strings.HasPrefix(id, "asset:")
		if group.representativeAssetID != "" {
			group.cluster.Metadata["representative_asset_id"] = group.representativeAssetID
			group.cluster.Metadata["representative_box"] = group.representativeBox
			if asset, err := s.deps.Store.GetAsset(ctx, group.representativeAssetID); err == nil {
				group.cluster.Metadata["representative_asset_name"] = asset.DisplayName
			}
		}
		assetsByCluster[id] = len(group.assetIDs)
		clusters = append(clusters, group.cluster)
	}
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].FaceCount == clusters[j].FaceCount {
			return clusters[i].UpdatedAt.After(clusters[j].UpdatedAt)
		}
		return clusters[i].FaceCount > clusters[j].FaceCount
	})
	return clusters, assetsByCluster, nil
}

func (s *Server) faceDetectionsForCluster(ctx context.Context, clusterID string) ([]catalog.FaceDetection, error) {
	faces, err := s.deps.Store.ListFaceDetections(ctx, "")
	if err != nil {
		return nil, err
	}
	out := []catalog.FaceDetection{}
	for _, face := range faces {
		if face.ClusterID == clusterID || (face.ClusterID == "" && clusterID == "asset:"+face.AssetID) {
			out = append(out, face)
		}
	}
	if len(out) == 0 {
		return nil, catalog.ErrNotFound
	}
	return out, nil
}

func (s *Server) materializeFaceCluster(ctx context.Context, clusterID, label string, metadata map[string]any) (catalog.FaceCluster, error) {
	faces, err := s.faceDetectionsForCluster(ctx, clusterID)
	if err != nil {
		return catalog.FaceCluster{}, err
	}
	if strings.HasPrefix(clusterID, "asset:") {
		clusterID = id.NewUUID()
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["local_only"] = true
	metadata["reviewed_by_user"] = label != ""
	cluster, err := s.deps.Store.UpsertFaceCluster(ctx, catalog.FaceCluster{
		ID:                   clusterID,
		Label:                label,
		RepresentativeFaceID: faces[0].ID,
		Metadata:             metadata,
	})
	if err != nil {
		return catalog.FaceCluster{}, err
	}
	for _, face := range faces {
		face.ClusterID = cluster.ID
		if face.Metadata == nil {
			face.Metadata = map[string]any{}
		}
		face.Metadata["review_status"] = "clustered"
		_, _ = s.deps.Store.UpdateFaceDetection(ctx, face)
	}
	return cluster, nil
}

func (s *Server) updateFaceDetectionMetadata(ctx context.Context, detectionID string, patch map[string]any) (catalog.FaceDetection, error) {
	faces, err := s.deps.Store.ListFaceDetections(ctx, "")
	if err != nil {
		return catalog.FaceDetection{}, err
	}
	for _, face := range faces {
		if face.ID != detectionID {
			continue
		}
		if face.Metadata == nil {
			face.Metadata = map[string]any{}
		}
		for key, value := range patch {
			face.Metadata[key] = value
		}
		return s.deps.Store.UpdateFaceDetection(ctx, face)
	}
	return catalog.FaceDetection{}, catalog.ErrNotFound
}

func (s *Server) assignFaceDetection(ctx context.Context, detectionID, clusterID, label string) (catalog.FaceDetection, error) {
	faces, err := s.deps.Store.ListFaceDetections(ctx, "")
	if err != nil {
		return catalog.FaceDetection{}, err
	}
	for _, face := range faces {
		if face.ID != detectionID {
			continue
		}
		if clusterID == "" {
			cluster, err := s.deps.Store.UpsertFaceCluster(ctx, catalog.FaceCluster{
				Label:                label,
				RepresentativeFaceID: face.ID,
				Metadata:             map[string]any{"local_only": true},
			})
			if err != nil {
				return catalog.FaceDetection{}, err
			}
			clusterID = cluster.ID
		}
		face.ClusterID = clusterID
		if face.Metadata == nil {
			face.Metadata = map[string]any{}
		}
		face.Metadata["review_status"] = "assigned"
		return s.deps.Store.UpdateFaceDetection(ctx, face)
	}
	return catalog.FaceDetection{}, catalog.ErrNotFound
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	switch value := metadata[key].(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(value)
		return parsed
	default:
		return false
	}
}

func (s *Server) handleGeoAlignSessionCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "geo_align.session") {
		return
	}
	var req struct {
		AssetIDs []string `json:"asset_ids"`
		TrackIDs []string `json:"track_ids"`
		Limit    int      `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 54
	}
	assets := []catalog.Asset{}
	if len(req.AssetIDs) > 0 {
		for _, assetID := range req.AssetIDs {
			asset, err := s.deps.Store.GetAsset(r.Context(), strings.TrimSpace(assetID))
			if err == nil && (asset.MediaKind == "photo" || asset.MediaKind == "video") {
				assets = append(assets, asset)
			}
			if len(assets) >= limit {
				break
			}
		}
	} else {
		page, err := s.deps.Store.QueryAssets(r.Context(), catalog.AssetQuery{Limit: limit, Sort: "taken_at"})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		for _, asset := range page.Assets {
			if asset.MediaKind == "photo" || asset.MediaKind == "video" {
				assets = append(assets, asset)
			}
		}
	}
	session, err := s.buildGeoAlignSession(r.Context(), assets, req.TrackIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.sessionMu.Lock()
	s.geoAlignSessions[session.ID] = &session
	s.sessionMu.Unlock()
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleGeoAlignSessionByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/geo-align/sessions/"), "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	sessionID := parts[0]
	s.sessionMu.RLock()
	session := s.geoAlignSessions[sessionID]
	s.sessionMu.RUnlock()
	if session == nil {
		writeError(w, http.StatusNotFound, catalog.ErrNotFound)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, session)
		return
	}
	if !s.requireWrite(w, r, "geo_align.session.write") {
		return
	}
	if len(parts) == 2 && parts[1] == "reset" && r.Method == http.MethodPost {
		for i := range session.Markers {
			resetGeoAlignMarkerState(&session.Markers[i])
		}
		session.UpdatedAt = time.Now().UTC()
		writeJSON(w, http.StatusOK, session)
		return
	}
	if len(parts) == 2 && parts[1] == "apply" && r.Method == http.MethodPost {
		updated := 0
		for _, marker := range session.Markers {
			if !marker.Modified || marker.ManualLat == nil || marker.ManualLon == nil {
				continue
			}
			geo := catalog.AssetGeo{
				AssetID: marker.AssetID,
				Lat:     *marker.ManualLat,
				Lon:     *marker.ManualLon,
				Source:  "manual_user",
				Metadata: map[string]any{
					"geo_align_session_id": session.ID,
					"note":                 "DB-only user clarified geotag; originals were not modified",
				},
			}
			if _, err := s.deps.Store.UpsertAssetGeo(r.Context(), geo, true); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			updated++
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "applied", "updated": updated, "write_exif_enabled": false})
		return
	}
	if len(parts) == 2 && parts[1] == "write-exif" && r.Method == http.MethodPost {
		writeError(w, http.StatusConflict, errors.New("EXIF writeback is disabled for strict read-only storage; DB-only overrides remain available"))
		return
	}
	if len(parts) == 3 && parts[1] == "marker" && r.Method == http.MethodPatch {
		var req struct {
			Lat   float64 `json:"lat"`
			Lon   float64 `json:"lon"`
			Reset bool    `json:"reset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		for i := range session.Markers {
			if session.Markers[i].AssetID == parts[2] {
				if req.Reset {
					resetGeoAlignMarkerState(&session.Markers[i])
					session.UpdatedAt = time.Now().UTC()
					writeJSON(w, http.StatusOK, session.Markers[i])
					return
				}
				session.Markers[i].ManualLat = &req.Lat
				session.Markers[i].ManualLon = &req.Lon
				session.Markers[i].StagedLat = req.Lat
				session.Markers[i].StagedLon = req.Lon
				session.Markers[i].Modified = true
				session.UpdatedAt = time.Now().UTC()
				writeJSON(w, http.StatusOK, session.Markers[i])
				return
			}
		}
		writeError(w, http.StatusNotFound, catalog.ErrNotFound)
		return
	}
	http.NotFound(w, r)
}

func resetGeoAlignMarkerState(marker *geoAlignMarker) {
	marker.ManualLat = nil
	marker.ManualLon = nil
	if marker.OriginalLat != nil && marker.OriginalLon != nil {
		marker.StagedLat = *marker.OriginalLat
		marker.StagedLon = *marker.OriginalLon
	} else if len(marker.TrackCandidates) > 0 {
		candidate := marker.TrackCandidates[0]
		lat, lon := floatFromAny(candidate["lat"]), floatFromAny(candidate["lon"])
		if lat != 0 || lon != 0 {
			marker.StagedLat = lat
			marker.StagedLon = lon
		}
	}
	marker.Modified = false
}

func (s *Server) buildGeoAlignSession(ctx context.Context, assets []catalog.Asset, trackIDs []string) (geoAlignSession, error) {
	now := time.Now().UTC()
	session := geoAlignSession{
		ID:        id.NewUUID(),
		AssetIDs:  []string{},
		TrackIDs:  trackIDs,
		Markers:   []geoAlignMarker{},
		ReadOnly:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	minLat, minLon, maxLat, maxLon := math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64
	trackDetails := map[string]catalog.TrackDetail{}
	for _, trackID := range trackIDs {
		detail, err := s.deps.Store.GetTrack(ctx, trackID)
		if err == nil {
			trackDetails[trackID] = detail
			for _, point := range detail.Points {
				minLat = math.Min(minLat, point.Lat)
				minLon = math.Min(minLon, point.Lon)
				maxLat = math.Max(maxLat, point.Lat)
				maxLon = math.Max(maxLon, point.Lon)
			}
		}
	}
	stagingLat := 40.05
	stagingLon := 44.05
	if minLat != math.MaxFloat64 {
		stagingLat = minLat - math.Max(0.005, (maxLat-minLat)*0.15)
		stagingLon = minLon - math.Max(0.005, (maxLon-minLon)*0.15)
	}
	for index, asset := range assets {
		marker := geoAlignMarker{
			AssetID:         asset.ID,
			Name:            asset.DisplayName,
			MediaKind:       asset.MediaKind,
			ThumbnailURL:    fmt.Sprintf("/api/v1/media/%s/preview", asset.ID),
			Status:          "ungeotagged",
			TrackCandidates: []map[string]any{},
			Metadata:        map[string]any{},
		}
		if geo, err := s.deps.Store.GetAssetGeo(ctx, asset.ID); err == nil {
			marker.OriginalLat = &geo.Lat
			marker.OriginalLon = &geo.Lon
			marker.StagedLat = geo.Lat
			marker.StagedLon = geo.Lon
			marker.Status = "own_geotag"
			minLat = math.Min(minLat, geo.Lat)
			minLon = math.Min(minLon, geo.Lon)
			maxLat = math.Max(maxLat, geo.Lat)
			maxLon = math.Max(maxLon, geo.Lon)
		} else {
			marker.StagedLat = stagingLat
			marker.StagedLon = stagingLon + float64(index)*0.00015
		}
		if asset.TakenAt != nil {
			for trackID, detail := range trackDetails {
				if point, mode, err := interpolateTrackPoint(detail.Points, *asset.TakenAt); err == nil {
					marker.TrackCandidates = append(marker.TrackCandidates, map[string]any{
						"track_id": trackID,
						"lat":      point.Lat,
						"lon":      point.Lon,
						"mode":     mode,
						"time":     point.RecordedAt,
					})
					marker.Status = "track_candidate"
				}
			}
		}
		session.AssetIDs = append(session.AssetIDs, asset.ID)
		session.Markers = append(session.Markers, marker)
	}
	if minLat == math.MaxFloat64 {
		minLat, minLon, maxLat, maxLon = stagingLat, stagingLon, stagingLat+0.01, stagingLon+0.01
	}
	session.BBox = catalog.BBox{MinLat: minLat, MinLon: minLon, MaxLat: maxLat, MaxLon: maxLon}
	return session, nil
}

func (s *Server) handleVideoTrackPlayerSessionCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		VideoAssetID  string   `json:"video_asset_id"`
		TrackIDs      []string `json:"track_ids"`
		TimestampMode string   `json:"timestamp_mode"`
		OffsetSeconds float64  `json:"offset_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	asset, err := s.deps.Store.GetAsset(r.Context(), strings.TrimSpace(req.VideoAssetID))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if asset.MediaKind != "video" {
		writeError(w, http.StatusBadRequest, errors.New("video-track player requires a video asset"))
		return
	}
	mode := req.TimestampMode
	if mode != "video_end_time" {
		mode = "video_start_time"
	}
	trackIDs := compactStrings(req.TrackIDs)
	var autoSelectionNote string
	if len(trackIDs) == 0 && runtimeBoolSetting("video_track_player.auto_select_overlapping_tracks", true) {
		if selected, note, ok := s.autoSelectVideoTrackIDs(r.Context(), asset); ok {
			trackIDs = selected
			autoSelectionNote = note
		}
	}
	if len(trackIDs) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("select at least one GPS/KML track"))
		return
	}
	session := &videoTrackPlayerSession{
		ID:            id.NewUUID(),
		VideoAssetID:  asset.ID,
		TrackIDs:      trackIDs,
		TimestampMode: mode,
		OffsetSeconds: req.OffsetSeconds,
		CreatedAt:     time.Now().UTC(),
		Metadata:      map[string]any{"video_name": asset.DisplayName},
	}
	if candidate, ok := s.bestVideoTimestampCandidate(r.Context(), asset, trackIDs); ok {
		session.VideoStartAt = &candidate.Time
		session.TimeSource = candidate.Source
		session.Metadata["video_time_candidate"] = candidate
	} else {
		session.Warnings = append(session.Warnings, "Video has no usable timestamp candidate; map synchronization requires manual offset/time settings.")
	}
	if autoSelectionNote != "" {
		session.Warnings = append([]string{autoSelectionNote}, session.Warnings...)
	}
	s.sessionMu.Lock()
	s.videoTrackSessions[session.ID] = session
	s.sessionMu.Unlock()
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleVideoTrackPlayerSessionByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/video-track-player/sessions/"), "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	sessionID := parts[0]
	s.sessionMu.RLock()
	session := s.videoTrackSessions[sessionID]
	s.sessionMu.RUnlock()
	if session == nil {
		writeError(w, http.StatusNotFound, catalog.ErrNotFound)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, session)
		return
	}
	if len(parts) == 2 && parts[1] == "position" && r.Method == http.MethodGet {
		timeMS, _ := strconv.ParseInt(r.URL.Query().Get("time_ms"), 10, 64)
		payload, err := s.videoTrackPosition(r.Context(), session, timeMS)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) videoTrackPosition(ctx context.Context, session *videoTrackPlayerSession, timeMS int64) (map[string]any, error) {
	asset, err := s.deps.Store.GetAsset(ctx, session.VideoAssetID)
	if err != nil {
		return nil, err
	}
	videoStart := session.VideoStartAt
	if videoStart == nil {
		if candidate, ok := s.bestVideoTimestampCandidate(ctx, asset, session.TrackIDs); ok {
			videoStart = &candidate.Time
			session.VideoStartAt = &candidate.Time
			session.TimeSource = candidate.Source
		}
	}
	if videoStart == nil {
		return map[string]any{"session_id": session.ID, "positions": []map[string]any{}, "warning": "video timestamp unavailable"}, nil
	}
	target := videoStart.Add(time.Duration(timeMS)*time.Millisecond + time.Duration(session.OffsetSeconds*float64(time.Second)))
	if session.TimestampMode == "video_end_time" {
		duration := mediaDuration(asset.Metadata)
		if duration > 0 {
			target = videoStart.Add(-duration).Add(time.Duration(timeMS)*time.Millisecond + time.Duration(session.OffsetSeconds*float64(time.Second)))
		}
	}
	positions := []map[string]any{}
	warnings := []string{}
	for _, trackID := range session.TrackIDs {
		detail, err := s.deps.Store.GetTrack(ctx, trackID)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", trackID, err))
			continue
		}
		point, mode, err := interpolateTrackPointWithTolerance(detail.Points, target, 10*time.Minute)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: no point at %s", trackID, target.Format(time.RFC3339)))
			continue
		}
		positions = append(positions, map[string]any{
			"track_id":               trackID,
			"name":                   detail.Summary.Name,
			"lat":                    point.Lat,
			"lon":                    point.Lon,
			"time":                   point.RecordedAt,
			"mode":                   mode,
			"speed_mps":              point.SpeedMPS,
			"elevation_m":            point.ElevationM,
			"relative_time_seconds":  relativeTimeSeconds(detail.Summary.StartTime, point.RecordedAt),
			"track_distance_m":       detail.Summary.DistanceM,
			"track_duration_seconds": detail.Summary.DurationSec,
		})
	}
	return map[string]any{"session_id": session.ID, "target_time": target, "time_source": session.TimeSource, "positions": positions, "warnings": warnings}, nil
}

func relativeTimeSeconds(start *time.Time, current time.Time) float64 {
	if start == nil || start.IsZero() || current.IsZero() {
		return 0
	}
	return current.Sub(*start).Seconds()
}

func (s *Server) bestVideoTimestampCandidate(ctx context.Context, asset catalog.Asset, trackIDs []string) (catalog.TimestampCandidate, bool) {
	candidates := catalog.AssetTimestampCandidates(asset, time.Local)
	if len(candidates) == 0 {
		return catalog.TimestampCandidate{}, false
	}
	for _, candidate := range candidates {
		for _, trackID := range trackIDs {
			detail, err := s.deps.Store.GetTrack(ctx, trackID)
			if err != nil || detail.Summary.StartTime == nil || detail.Summary.EndTime == nil {
				continue
			}
			start := detail.Summary.StartTime.Add(-30 * time.Minute)
			end := detail.Summary.EndTime.Add(30 * time.Minute)
			if !candidate.Time.Before(start) && !candidate.Time.After(end) {
				return candidate, true
			}
		}
	}
	return candidates[0], true
}

func (s *Server) autoSelectVideoTrackIDs(ctx context.Context, asset catalog.Asset) ([]string, string, bool) {
	candidates, err := s.deps.Store.ListTracks(ctx)
	if err != nil || len(candidates) == 0 {
		return nil, "", false
	}
	best := catalog.AssetTimestampCandidates(asset, time.Local)
	if len(best) == 0 {
		return nil, "", false
	}
	for _, candidate := range best {
		var selected []string
		for _, track := range candidates {
			if track.StartTime == nil || track.EndTime == nil {
				continue
			}
			start := track.StartTime.Add(-30 * time.Minute)
			end := track.EndTime.Add(30 * time.Minute)
			if candidate.Time.Before(start) || candidate.Time.After(end) {
				continue
			}
			selected = append(selected, track.TrackAssetID)
		}
		if len(selected) > 0 {
			return uniqueStrings(selected), fmt.Sprintf("Auto-selected %d overlapping track(s) from timestamp candidates.", len(selected)), true
		}
	}
	return nil, "", false
}

func mediaDuration(metadata map[string]any) time.Duration {
	if metadata == nil {
		return 0
	}
	for _, key := range []string{"duration_seconds", "duration_sec", "duration"} {
		switch value := metadata[key].(type) {
		case float64:
			if value > 0 {
				return time.Duration(value * float64(time.Second))
			}
		case int:
			if value > 0 {
				return time.Duration(value) * time.Second
			}
		case string:
			parsed, err := strconv.ParseFloat(value, 64)
			if err == nil && parsed > 0 {
				return time.Duration(parsed * float64(time.Second))
			}
		}
	}
	return 0
}

func (s *Server) handleAIPredictions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	assetID := strings.TrimSpace(r.URL.Query().Get("asset_id"))
	limit := intQuery(r.URL.Query(), "limit", 100, 1, 5000)
	offset := intQuery(r.URL.Query(), "offset", 0, 0, 1000000)
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	taskQuery := strings.TrimSpace(r.URL.Query().Get("task"))
	tags, err := s.deps.Store.ListAssetTags(r.Context(), assetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var predictions []catalog.AIPrediction
	totalPredictions := 0
	if assetID == "" {
		if pager, ok := s.deps.Store.(interface {
			QueryAIPredictions(context.Context, catalog.AIPredictionQuery) (catalog.AIPredictionPage, error)
		}); ok {
			page, err := pager.QueryAIPredictions(r.Context(), catalog.AIPredictionQuery{
				Tasks:  aiPredictionTaskTokens(taskQuery),
				Q:      query,
				Limit:  limit,
				Offset: offset,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			predictions = page.Predictions
			totalPredictions = page.Total
		}
	}
	if predictions == nil {
		var err error
		predictions, err = s.deps.Store.ListAIPredictions(r.Context(), assetID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if taskQuery != "" {
			predictions = filterAIPredictionsByTask(predictions, taskQuery)
		}
		if query != "" {
			predictions = filterAIPredictions(predictions, query)
		}
		totalPredictions = len(predictions)
		if assetID == "" {
			predictions = slicePage(predictions, limit, offset)
		}
	}
	faces, err := s.deps.Store.ListFaceDetections(r.Context(), assetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	embeddings, err := s.deps.Store.ListAssetEmbeddings(r.Context(), assetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if query != "" {
		tags = filterAssetTags(tags, query)
		faces = filterFaceDetections(faces, query)
	}
	totalTags := len(tags)
	totalFaces := len(faces)
	totalEmbeddings := len(embeddings)
	if assetID == "" {
		tags = slicePage(tags, limit, offset)
		faces = slicePage(faces, limit, offset)
		embeddings = slicePage(embeddings, limit, offset)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"asset_id":          assetID,
		"tags":              tags,
		"predictions":       predictions,
		"faces":             faces,
		"embeddings":        summarizeAssetEmbeddings(embeddings),
		"limit":             limit,
		"offset":            offset,
		"total_tags":        totalTags,
		"total_predictions": totalPredictions,
		"total_faces":       totalFaces,
		"total_embeddings":  totalEmbeddings,
	})
}

func filterAIPredictionsByTask(predictions []catalog.AIPrediction, taskQuery string) []catalog.AIPrediction {
	allowed := map[string]struct{}{}
	for _, token := range aiPredictionTaskTokens(taskQuery) {
		allowed[token] = struct{}{}
	}
	if len(allowed) == 0 {
		return predictions
	}
	filtered := predictions[:0]
	for _, prediction := range predictions {
		if _, ok := allowed[strings.ToLower(prediction.Task)]; ok {
			filtered = append(filtered, prediction)
		}
	}
	return filtered
}

func aiPredictionTaskTokens(taskQuery string) []string {
	tokens := []string{}
	seen := map[string]struct{}{}
	for _, token := range strings.FieldsFunc(strings.ToLower(taskQuery), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	return tokens
}

func slicePage[T any](rows []T, limit, offset int) []T {
	if offset >= len(rows) {
		return []T{}
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end]
}

func stringFromMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Sprint(metadata)
	}
	return string(data)
}

func filterAssetTags(tags []catalog.AssetTag, query string) []catalog.AssetTag {
	filtered := tags[:0]
	for _, tag := range tags {
		text := strings.ToLower(strings.Join([]string{
			tag.AssetID,
			tag.Tag,
			tag.Source,
			tag.PluginID,
			stringFromMetadata(tag.Metadata),
		}, " "))
		if strings.Contains(text, query) {
			filtered = append(filtered, tag)
		}
	}
	return filtered
}

func filterAIPredictions(predictions []catalog.AIPrediction, query string) []catalog.AIPrediction {
	filtered := predictions[:0]
	for _, prediction := range predictions {
		text := strings.ToLower(strings.Join([]string{
			prediction.ID,
			prediction.AssetID,
			prediction.PluginID,
			prediction.WorkerID,
			prediction.Task,
			prediction.Label,
			prediction.ModelName,
			prediction.ModelVersion,
			stringFromMetadata(prediction.Metadata),
		}, " "))
		if strings.Contains(text, query) {
			filtered = append(filtered, prediction)
		}
	}
	return filtered
}

func filterFaceDetections(faces []catalog.FaceDetection, query string) []catalog.FaceDetection {
	filtered := faces[:0]
	for _, face := range faces {
		text := strings.ToLower(strings.Join([]string{
			face.ID,
			face.AssetID,
			face.ClusterID,
			face.PluginID,
			stringFromMetadata(face.Metadata),
		}, " "))
		if strings.Contains(text, query) {
			filtered = append(filtered, face)
		}
	}
	return filtered
}

func (s *Server) handleOCRRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	jobsList, err := s.deps.Store.ListJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	runs := []jobs.Job{}
	for _, job := range jobsList {
		if job.Kind == "ai_ocr" {
			runs = append(runs, job)
		}
	}
	runs = summarizeJobsForList(runs)
	predictions, err := s.deps.Store.ListAIPredictions(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	blocks := ocrBlocksFromPredictions(predictions)
	writeJSON(w, http.StatusOK, map[string]any{
		"runs":             runs,
		"total":            len(runs),
		"stored_blocks":    len(blocks),
		"supported_langs":  []string{"eng", "rus", "hye", "chi_sim", "chi_tra"},
		"engine_contract":  "tesseract_cli_or_ai_sidecar",
		"auto_run_enabled": false,
	})
}

func (s *Server) potentiallyUnsafeAlbumSummary(ctx context.Context) map[string]any {
	albums, err := s.deps.Store.ListAlbums(ctx, catalog.AlbumQuery{Tree: true, Limit: 1000})
	if err != nil {
		return map[string]any{"exists": false}
	}
	for _, album := range albums {
		if album.Slug == "potentially-unsafe" || strings.EqualFold(album.Title, "Potentially Unsafe") {
			return map[string]any{"exists": true, "id": album.ID, "title": album.Title, "item_count": album.ItemCount}
		}
	}
	return map[string]any{"exists": false}
}

func (s *Server) handleAISafetyByAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "ai.jobs.write") {
		return
	}
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/v1/ai/safety/"))
	if len(parts) != 2 || parts[1] != "allow" {
		http.NotFound(w, r)
		return
	}
	assetID := parts[0]
	album, err := s.ensurePotentiallyUnsafeAlbum(r.Context())
	if err == nil {
		_ = s.deps.Store.RemoveAlbumItem(r.Context(), album.ID, assetID)
	}
	_, _ = s.deps.Store.UpsertAssetTag(r.Context(), catalog.AssetTag{
		AssetID:  assetID,
		Tag:      "safety:reviewed_allowed",
		Source:   "ai_safety_review",
		Metadata: map[string]any{"reviewed_at": time.Now().UTC()},
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "reviewed_allowed", "asset_id": assetID})
}

func aiWorkerProfiles(ctx context.Context) []map[string]any {
	hints := transcoding.AcceleratorHints()
	endpoint := aiWorkerEndpoint()
	localStatus, localHealth := aiWorkerHealth(ctx, endpoint+"/health")
	localCapabilities := []string{"classify_image", "detect_faces", "safety_nsfw", "describe_image", "embed_image", "embed_text", "ocr_image", "transcribe_audio", "analyze_audio"}
	localDevice := ""
	localModels := map[string]any{}
	if caps, ok := localHealth["capabilities"].(map[string]any); ok {
		if raw, ok := caps["capabilities"].([]any); ok && len(raw) > 0 {
			localCapabilities = anySliceToStrings(raw)
		}
		if text, ok := caps["device"].(string); ok {
			localDevice = text
		}
		if models, ok := caps["models"].(map[string]any); ok {
			localModels = models
		}
	}
	return []map[string]any{
		{
			"id":                "ai-local",
			"profile":           "local",
			"configured":        localStatus == "ok",
			"status":            localStatus,
			"available":         true,
			"capabilities":      localCapabilities,
			"endpoint":          endpoint,
			"device":            localDevice,
			"cuda_available":    strings.HasPrefix(strings.ToLower(localDevice), "cuda"),
			"models":            localModels,
			"native_entrypoint": "python -m cartolensia_ai.server --host 0.0.0.0 --port 19090",
			"health":            localHealth,
			"note":              "local optional worker; inference only runs when a user starts a scoped AI job",
		},
		{
			"id":           "ai-cpu",
			"profile":      "cpu",
			"configured":   false,
			"status":       "not_configured",
			"capabilities": []string{"classify_image", "detect_faces", "describe_image", "embed_image", "ocr_image", "transcribe_audio", "analyze_audio"},
			"endpoint":     "",
		},
		{
			"id":             "ai-nvidia",
			"profile":        "nvidia",
			"configured":     false,
			"status":         "not_configured",
			"available":      hints["docker_nvidia_runtime"],
			"native_gpu":     hints["nvidia_smi"],
			"docker_runtime": hints["docker_nvidia_runtime"],
			"capabilities":   []string{"classify_image", "detect_faces", "describe_image", "embed_image", "ocr_image", "transcribe_audio", "analyze_audio"},
			"endpoint":       "",
			"note":           "optional Docker NVIDIA profile; native CUDA is represented by ai-local",
		},
		{
			"id":           "ai-rocm",
			"profile":      "rocm",
			"configured":   false,
			"status":       "not_configured",
			"available":    hints["dev_dri"],
			"capabilities": []string{"classify_image", "detect_faces", "describe_image", "embed_image", "ocr_image", "transcribe_audio", "analyze_audio"},
			"endpoint":     "",
		},
		{
			"id":           "ai-intel",
			"profile":      "intel",
			"configured":   false,
			"status":       "not_configured",
			"available":    hints["dev_dri"],
			"capabilities": []string{"classify_image", "detect_faces", "describe_image", "embed_image", "ocr_image", "transcribe_audio", "analyze_audio"},
			"endpoint":     "",
		},
	}
}

func aiWorkersConfigured(workers []map[string]any) bool {
	for _, worker := range workers {
		if configured, _ := worker["configured"].(bool); configured {
			return true
		}
	}
	return false
}

func aiNativeRuntimeSummary(workers []map[string]any) map[string]any {
	out := map[string]any{
		"configured": false,
		"status":     "not_configured",
		"device":     "",
		"cuda":       false,
	}
	for _, worker := range workers {
		if worker["id"] != "ai-local" {
			continue
		}
		device, _ := worker["device"].(string)
		configured, _ := worker["configured"].(bool)
		status, _ := worker["status"].(string)
		out["configured"] = configured
		out["status"] = status
		out["device"] = device
		out["cuda"] = strings.HasPrefix(strings.ToLower(device), "cuda")
		out["endpoint"] = worker["endpoint"]
		out["models"] = worker["models"]
		return out
	}
	return out
}

func aiDevicePolicy(hints map[string]any, native map[string]any) map[string]any {
	nativeCUDA, _ := native["cuda"].(bool)
	nvidia, _ := hints["nvidia_smi"].(bool)
	dockerNVIDIA, _ := hints["docker_nvidia_runtime"].(bool)
	devDRI, _ := hints["dev_dri"].(bool)
	device, _ := native["device"].(string)
	active := "cpu"
	if nativeCUDA && device != "" {
		active = device
	} else if nvidia {
		active = "nvidia_available_unselected"
	}
	return map[string]any{
		"preference":             "auto",
		"active_device":          active,
		"native_cuda_available":  nativeCUDA,
		"native_nvidia_present":  nvidia,
		"docker_nvidia_runtime":  dockerNVIDIA,
		"amd_or_intel_dri":       devDRI,
		"priority":               []string{"native NVIDIA CUDA", "Docker NVIDIA profile", "AMD/ROCm", "Intel/XPU", "CPU"},
		"cpu_fallback_available": true,
	}
}

func aiCartolensiaJobKind(kind string) string {
	switch kind {
	case "classify_image":
		return "ai_classify"
	case "detect_faces":
		return "ai_detect_faces"
	case "safety_nsfw":
		return "ai_safety_nsfw"
	case "embed_image":
		return "ai_embed"
	case "describe_image":
		return "ai_describe"
	case "ocr_image":
		return "ai_ocr"
	case "transcribe_audio":
		return "ai_transcribe"
	case "analyze_audio":
		return "audio_analyze"
	default:
		return "ai_action"
	}
}

func aiWorkerHealth(ctx context.Context, endpoint string) (string, map[string]any) {
	probeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "not_configured", map[string]any{"error": err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "not_configured", map[string]any{"error": err.Error()}
	}
	defer resp.Body.Close()
	var payload map[string]any
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "unhealthy", map[string]any{"status_code": resp.StatusCode, "response": payload}
	}
	return "ok", payload
}

func (s *Server) handleVectorStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, s.vectorStatusPayload(r.Context()))
}

func (s *Server) vectorStatusPayload(ctx context.Context) map[string]any {
	stats, _ := s.deps.Store.Stats(ctx)
	embeddings, _ := s.deps.Store.ListAssetEmbeddings(ctx, "")
	pgvectorInstalled := capabilityInstalled(s.deps.Capabilities, "vector")
	backend := "local_json_bruteforce"
	pgvectorNote := "pgvector extension is not installed; local JSON/bruteforce fallback is active."
	contract := "OpenCLIP embeddings are stored as JSON/float arrays and searched with bounded brute-force cosine similarity."
	if pgvectorInstalled {
		backend = "pgvector_ivfflat"
		pgvectorNote = "pgvector is installed; 512-dimensional image/text embeddings are also stored in an indexed vector column."
		contract = "OpenCLIP embeddings are stored in PostgreSQL JSON for portability and pgvector for indexed cosine search when dimensions match."
	}
	embeddedAssets := map[string]struct{}{}
	dimensions := 0
	for _, embedding := range embeddings {
		embeddedAssets[embedding.AssetID] = struct{}{}
		if dimensions == 0 && len(embedding.Vector) > 0 {
			dimensions = len(embedding.Vector)
		}
	}
	return map[string]any{
		"available":       true,
		"backend":         backend,
		"pgvector":        pgvectorInstalled,
		"pgvector_note":   pgvectorNote,
		"contract":        contract,
		"embedded_assets": len(embeddedAssets),
		"embedding_count": len(embeddings),
		"dimensions":      dimensions,
		"limits": map[string]any{
			"intended_collection_size": "small/local collections",
			"indexed_assets":           stats.Assets,
			"embedded_assets":          len(embeddedAssets),
		},
	}
}

func (s *Server) vectorBackendName() string {
	if capabilityInstalled(s.deps.Capabilities, "vector") {
		return "pgvector_ivfflat"
	}
	return "local_json_bruteforce"
}

func (s *Server) aiDataCounts(ctx context.Context) map[string]any {
	counts := map[string]any{
		"asset_tags":        0,
		"predictions":       0,
		"face_detections":   0,
		"asset_embeddings":  0,
		"embedded_assets":   0,
		"safety_candidates": 0,
	}
	if counter, ok := s.deps.Store.(interface {
		AIDataCounts(context.Context) (catalog.AIDataCounts, error)
	}); ok {
		if aggregate, err := counter.AIDataCounts(ctx); err == nil {
			counts["asset_tags"] = aggregate.AssetTags
			counts["predictions"] = aggregate.Predictions
			counts["face_detections"] = aggregate.FaceDetections
			counts["asset_embeddings"] = aggregate.AssetEmbeddings
			counts["embedded_assets"] = aggregate.EmbeddedAssets
			counts["safety_candidates"] = aggregate.SafetyCandidates
			return counts
		}
	}
	if tags, err := s.deps.Store.ListAssetTags(ctx, ""); err == nil {
		counts["asset_tags"] = len(tags)
	}
	if predictions, err := s.deps.Store.ListAIPredictions(ctx, ""); err == nil {
		counts["predictions"] = len(predictions)
		safetyCandidates := 0
		for _, prediction := range predictions {
			task := strings.ToLower(prediction.Task)
			label := strings.ToLower(prediction.Label)
			if !strings.Contains(task, "safety") && !strings.Contains(task, "nsfw") {
				continue
			}
			if !strings.Contains(label, "unsafe") && !strings.Contains(label, "nsfw") {
				continue
			}
			if prediction.Confidence == nil || *prediction.Confidence >= 0.75 {
				safetyCandidates++
			}
		}
		counts["safety_candidates"] = safetyCandidates
	}
	if faces, err := s.deps.Store.ListFaceDetections(ctx, ""); err == nil {
		counts["face_detections"] = len(faces)
	}
	if embeddings, err := s.deps.Store.ListAssetEmbeddings(ctx, ""); err == nil {
		embeddedAssets := map[string]struct{}{}
		for _, embedding := range embeddings {
			embeddedAssets[embedding.AssetID] = struct{}{}
		}
		counts["asset_embeddings"] = len(embeddings)
		counts["embedded_assets"] = len(embeddedAssets)
	}
	return counts
}

func (s *Server) handleVectorSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("q is required"))
		return
	}
	endpoint, _, ok := aiConfiguredWorkerEndpoint(r.Context())
	if !ok {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("no AI sidecar worker is reachable for text embeddings"))
		return
	}
	response, err := callAISidecar(r.Context(), endpoint+"/embed-text", map[string]any{"text": query})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if response.Status != "ok" {
		writeJSON(w, http.StatusOK, map[string]any{"query": query, "status": response.Status, "reason": response.Reason, "results": []any{}})
		return
	}
	embedding := floatSliceFromAny(response.Metadata["embedding"])
	if len(embedding) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"query": query, "status": "empty_embedding", "results": []any{}})
		return
	}
	limit := queryInt(r, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	results, err := s.deps.Store.VectorSearch(r.Context(), "openclip-vit-b-32-laion2b-s34b-b79k", embedding, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": query, "status": "ok", "results": results})
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

func (s *Server) handleFileBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	roots := s.fileBrowseRoots()
	rootID := strings.TrimSpace(r.URL.Query().Get("root"))
	if rootID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"roots": roots, "entries": []any{}, "note": "choose an allowlisted root before browsing"})
		return
	}
	root, ok := roots[rootID]
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("browse root %q is not allowlisted", rootID))
		return
	}
	requested := strings.TrimSpace(r.URL.Query().Get("path"))
	target, rel, err := safeBrowseTarget(root.Path, requested)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() == entries[j].IsDir() {
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		}
		return entries[i].IsDir()
	})
	type browseEntry struct {
		Name       string    `json:"name"`
		Path       string    `json:"path"`
		Kind       string    `json:"kind"`
		SizeBytes  int64     `json:"size_bytes,omitempty"`
		ModifiedAt time.Time `json:"modified_at,omitempty"`
		Selectable bool      `json:"selectable"`
		Readable   bool      `json:"readable"`
	}
	out := make([]browseEntry, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		kind := "file"
		if entry.IsDir() {
			kind = "folder"
		}
		childRel := entry.Name()
		if rel != "" {
			childRel = filepath.ToSlash(filepath.Join(rel, entry.Name()))
		}
		item := browseEntry{Name: entry.Name(), Path: childRel, Kind: kind, Selectable: true, Readable: infoErr == nil}
		if infoErr == nil {
			item.SizeBytes = info.Size()
			item.ModifiedAt = info.ModTime()
		}
		out = append(out, item)
		if len(out) >= 500 {
			break
		}
	}
	parent := ""
	if rel != "" {
		parent = filepath.ToSlash(filepath.Dir(rel))
		if parent == "." {
			parent = ""
		}
	}
	warnings := []string{}
	if strings.HasPrefix(filepath.Clean(root.Path), filepath.Clean("/mnt/Models/rclone")) {
		warnings = append(warnings, "real archive storage is browse-only and remains strict read-only")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"root":         root,
		"current_path": rel,
		"absolute":     target,
		"parent":       parent,
		"entries":      out,
		"warnings":     warnings,
		"limit":        500,
	})
}

type browseRoot struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	ReadOnly bool   `json:"read_only"`
	Warning  string `json:"warning,omitempty"`
}

func (s *Server) fileBrowseRoots() map[string]browseRoot {
	roots := map[string]browseRoot{}
	add := func(idValue, label, pathValue, kind, warning string, readOnly bool) {
		if strings.TrimSpace(pathValue) == "" {
			return
		}
		abs, err := filepath.Abs(pathValue)
		if err != nil {
			return
		}
		roots[idValue] = browseRoot{ID: idValue, Label: label, Path: abs, Kind: kind, ReadOnly: readOnly, Warning: warning}
	}
	add("cartolensia", "Cartolensia runtime", ".cartolensia", "runtime", "repo-local generated data only", false)
	add("tmp", "Temporary files", "/tmp", "system", "temporary cache/test location", false)
	add("mnt", "/mnt", "/mnt", "system", "mounted volumes; choose carefully", true)
	add("media", "/media", "/media", "system", "removable media roots", true)
	add("srv", "/srv", "/srv", "system", "service data roots", true)
	if home, err := os.UserHomeDir(); err == nil {
		add("home", "Home", home, "home", "manual path selection only", true)
	}
	for _, storageConfig := range s.deps.Registry.ListStorages() {
		warning := "configured storage; browsing is read-only"
		if strings.HasPrefix(filepath.Clean(storageConfig.Root), filepath.Clean("/mnt/Models/rclone")) {
			warning = "real archive storage: strict read-only, no writes or scans from the picker"
		}
		add("storage:"+storageConfig.Name, "Storage: "+storageConfig.Name, storageConfig.Root, "storage", warning, true)
	}
	return roots
}

func safeBrowseTarget(rootPath, requested string) (target, rel string, err error) {
	base, err := filepath.Abs(rootPath)
	if err != nil {
		return "", "", err
	}
	base = filepath.Clean(base)
	cleaned := filepath.Clean(strings.TrimSpace(requested))
	if cleaned == "." || cleaned == "/" {
		cleaned = ""
	}
	if filepath.IsAbs(cleaned) {
		abs := filepath.Clean(cleaned)
		relToBase, relErr := filepath.Rel(base, abs)
		if relErr != nil || relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(filepath.Separator)) {
			return "", "", fmt.Errorf("path escapes browse root")
		}
		cleaned = relToBase
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || strings.Contains(cleaned, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path traversal is not allowed")
	}
	if cleaned == "" {
		target = base
	} else {
		target = filepath.Join(base, cleaned)
	}
	target = filepath.Clean(target)
	relToBase, err := filepath.Rel(base, target)
	if err != nil || relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path escapes browse root")
	}
	if relToBase == "." {
		relToBase = ""
	}
	return target, filepath.ToSlash(relToBase), nil
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
	case "track-preview":
		s.handleTrackPreview(w, r, assetID)
	case "track-thumbnail":
		s.handleTrackThumbnail(w, r, assetID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleAIMedia(w http.ResponseWriter, r *http.Request) {
	if !s.aiMediaRequestAuthorized(r) {
		writeError(w, http.StatusUnauthorized, auth.ErrUnauthenticated)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/ai-media/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "original" {
		http.NotFound(w, r)
		return
	}
	s.handleOriginal(w, r, parts[0])
}

func (s *Server) aiMediaRequestAuthorized(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	token := strings.TrimSpace(os.Getenv("CARTOLENSIA_AI_MEDIA_TOKEN"))
	if token == "" {
		return false
	}
	got := strings.TrimSpace(r.URL.Query().Get("token"))
	if got == "" {
		got = strings.TrimSpace(r.Header.Get("X-Cartolensia-AI-Media-Token"))
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
	options := []map[string]any{{
		"id":          "original",
		"label":       "Original/direct",
		"available":   true,
		"url":         "/api/v1/media/" + asset.ID + "/original",
		"description": "Streams the immutable original through Cartolensia with HTTP Range support.",
	}}
	if asset.MediaKind == "video" {
		caps := transcoding.Detect(r.Context())
		presets := builtInTranscodingPresets(caps)
		custom, err := s.deps.Store.ListTranscodingPresets(r.Context())
		if err == nil {
			presets = append(presets, custom...)
		}
		for _, preset := range presets {
			if preset.ID == "original" {
				continue
			}
			options = append(options, map[string]any{
				"id":               preset.ID,
				"label":            preset.Name,
				"available":        preset.Available,
				"profile":          preset.ID,
				"built_in":         preset.BuiltIn,
				"hardware":         preset.Hardware,
				"codec":            preset.Codec,
				"mode":             preset.Mode,
				"parameter_value":  preset.ParameterValue,
				"session_endpoint": "/api/v1/media/" + asset.ID + "/transcode-session",
				"disabled_reason":  preset.DisabledReason,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"asset_id":     asset.ID,
		"media_kind":   asset.MediaKind,
		"direct_url":   "/api/v1/media/" + asset.ID + "/original",
		"range":        true,
		"storage":      loc.StorageName,
		"storage_mode": "strict_read_only",
		"options":      options,
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
		s.writeStorageOpenError(w, loc, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", info.MIME)
	w.Header().Set("X-Cartolensia-Storage-URL", info.StorageURL)
	http.ServeContent(w, r, info.Name, info.MTime, file)
}

func (s *Server) writeStorageOpenError(w http.ResponseWriter, loc catalog.Location, openErr error) {
	status := http.StatusServiceUnavailable
	code := "storage_unavailable"
	message := "original storage is unavailable; metadata and cached data remain available"
	details := map[string]any{
		"storage":       loc.StorageName,
		"relative_path": loc.RelativePath,
		"error":         openErr.Error(),
	}
	if cfg, err := s.deps.Registry.GetStorage(loc.StorageName); err == nil {
		diag := diagnoseStorageRoot(cfg, 750*time.Millisecond)
		details["storage_health"] = diag.Code
		details["storage_message"] = diag.Message
		for key, value := range diag.Details {
			details[key] = value
		}
		if diag.Code == "available" && errors.Is(openErr, os.ErrNotExist) {
			status = http.StatusNotFound
			code = "original_file_missing"
			message = "storage is available, but the original file is missing at the indexed path"
		} else if diag.Code == "credentials_invalid" || diag.Code == "credentials_or_permission_denied" || diag.Code == "credentials_file_missing" || diag.Code == "credentials_file_unreadable" || errors.Is(openErr, os.ErrPermission) {
			status = http.StatusForbidden
			code = "storage_credentials_or_permission_denied"
			message = "storage is reachable, but Cartolensia cannot read the original; check Samba credentials and share permissions"
		}
	} else if errors.Is(openErr, os.ErrNotExist) {
		status = http.StatusNotFound
		code = "original_file_missing"
		message = "original file is missing at the indexed path"
	} else if errors.Is(openErr, os.ErrPermission) {
		status = http.StatusForbidden
		code = "storage_credentials_or_permission_denied"
		message = "Cartolensia cannot read the original; check storage credentials and permissions"
	}
	writeJSON(w, status, map[string]any{
		"error":   message,
		"code":    code,
		"details": details,
	})
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
	if !s.deps.Config.Cache.PersistentPreviews {
		info, data, err := preview.GenerateImageBytes(r.Context(), asset, file)
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
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("X-Cartolensia-Preview-Status", string(info.Status))
		w.Header().Set("X-Cartolensia-Preview-Cache", "on-demand")
		http.ServeContent(w, r, "preview.jpg", asset.UpdatedAt, bytes.NewReader(data))
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

func (s *Server) assetDetail(ctx context.Context, asset catalog.Asset) map[string]any {
	previewInfo := preview.InfoForAsset("", asset)
	if s.deps.Config.Cache.PersistentPreviews {
		previewInfo = preview.InfoForAsset(s.deps.Config.Cache.Dir, asset)
	}
	detail := map[string]any{
		"asset":        asset,
		"locations":    asset.Locations,
		"preview":      previewInfo,
		"metadata":     asset.Metadata,
		"visibility":   map[string]any{"public": assetIsPublic(asset)},
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
	if tags, err := s.deps.Store.ListAssetTags(ctx, asset.ID); err == nil {
		detail["ai_tags"] = tags
	}
	if predictions, err := s.deps.Store.ListAIPredictions(ctx, asset.ID); err == nil {
		detail["ai_predictions"] = predictions
		ocrBlocks := ocrBlocksFromPredictions(predictions)
		ocrSummary := ocrSummaryFromBlocks(ocrBlocks)
		detail["ocr_blocks"] = ocrBlocks
		detail["ocr_summary"] = ocrSummary
		detail["ocr_full_text"] = ocrSummary.FullText
	}
	if faces, err := s.deps.Store.ListFaceDetections(ctx, asset.ID); err == nil {
		detail["face_detections"] = faces
	}
	if embeddings, err := s.deps.Store.ListAssetEmbeddings(ctx, asset.ID); err == nil {
		detail["embeddings"] = summarizeAssetEmbeddings(embeddings)
	}
	if transcripts, err := s.deps.Store.ListTranscripts(ctx, asset.ID, 20); err == nil {
		detail["transcripts"] = transcripts
	}
	if audioFeatures, err := s.deps.Store.GetAudioFeatures(ctx, asset.ID); err == nil {
		detail["audio_features"] = audioFeatures
	}
	if frameCaptions, err := s.deps.Store.ListVideoFrameCaptions(ctx, asset.ID, 200); err == nil {
		detail["frame_captions"] = frameCaptions
	}
	if doc, err := s.deps.Store.GetDocumentText(ctx, asset.ID); err == nil {
		detail["document"] = doc
	}
	detail["places"] = s.assetPlaceRecords(ctx, asset)
	return detail
}

func (s *Server) assetAIRecord(ctx context.Context, asset catalog.Asset) map[string]any {
	predictions, _ := s.deps.Store.ListAIPredictions(ctx, asset.ID)
	tags, _ := s.deps.Store.ListAssetTags(ctx, asset.ID)
	faces, _ := s.deps.Store.ListFaceDetections(ctx, asset.ID)
	embeddings, _ := s.deps.Store.ListAssetEmbeddings(ctx, asset.ID)
	ocrBlocks := ocrBlocksFromPredictions(predictions)
	return map[string]any{
		"asset_id":        asset.ID,
		"tags":            tags,
		"predictions":     predictions,
		"classification":  s.classificationRecordsFrom(predictions, tags),
		"captions":        s.captionRecordsFrom(predictions, tags),
		"ocr_blocks":      ocrBlocks,
		"ocr_summary":     ocrSummaryFromBlocks(ocrBlocks),
		"faces":           faces,
		"safety":          s.safetyRecordsFrom(predictions, tags),
		"embeddings":      summarizeAssetEmbeddings(embeddings),
		"generated_truth": "AI/OCR/caption outputs are local predictions or suggestions, not ground truth.",
	}
}

func (s *Server) assetCaptionRecords(ctx context.Context, assetID string) []map[string]any {
	predictions, _ := s.deps.Store.ListAIPredictions(ctx, assetID)
	tags, _ := s.deps.Store.ListAssetTags(ctx, assetID)
	return s.captionRecordsFrom(predictions, tags)
}

func (s *Server) captionRecordsFrom(predictions []catalog.AIPrediction, tags []catalog.AssetTag) []map[string]any {
	out := []map[string]any{}
	for _, prediction := range predictions {
		if prediction.Task != "describe_image" && prediction.Task != "caption_short" && prediction.Task != "caption_long" {
			continue
		}
		out = append(out, map[string]any{
			"id":         prediction.ID,
			"asset_id":   prediction.AssetID,
			"text":       prediction.Label,
			"task":       prediction.Task,
			"model":      prediction.ModelName,
			"confidence": prediction.Confidence,
			"created_at": prediction.CreatedAt,
			"metadata":   prediction.Metadata,
		})
	}
	for _, tag := range tags {
		if tag.Source != "ai_caption" && !strings.HasPrefix(tag.Tag, "caption:") {
			continue
		}
		text := strings.TrimPrefix(tag.Tag, "caption:")
		if metadataCaption := stringFromMap(tag.Metadata, "caption"); metadataCaption != "" {
			text = metadataCaption
		}
		out = append(out, map[string]any{
			"id":         fmt.Sprintf("%s:%s:%s", tag.AssetID, tag.Source, tag.Tag),
			"asset_id":   tag.AssetID,
			"text":       text,
			"task":       "caption",
			"model":      stringFromMap(tag.Metadata, "model"),
			"created_at": tag.CreatedAt,
			"metadata":   tag.Metadata,
		})
	}
	return out
}

func (s *Server) assetClassificationRecords(ctx context.Context, assetID string) []map[string]any {
	predictions, _ := s.deps.Store.ListAIPredictions(ctx, assetID)
	tags, _ := s.deps.Store.ListAssetTags(ctx, assetID)
	return s.classificationRecordsFrom(predictions, tags)
}

func (s *Server) classificationRecordsFrom(predictions []catalog.AIPrediction, tags []catalog.AssetTag) []map[string]any {
	out := []map[string]any{}
	for _, prediction := range predictions {
		switch prediction.Task {
		case "classify_image", "classification", "classify":
			out = append(out, map[string]any{
				"id":         prediction.ID,
				"asset_id":   prediction.AssetID,
				"label":      prediction.Label,
				"task":       prediction.Task,
				"model":      prediction.ModelName,
				"confidence": prediction.Confidence,
				"created_at": prediction.CreatedAt,
				"metadata":   prediction.Metadata,
			})
		}
	}
	for _, tag := range tags {
		if tag.Source != "ai_classification" && tag.Source != "ai_classifier" {
			continue
		}
		out = append(out, map[string]any{
			"id":         fmt.Sprintf("%s:%s:%s", tag.AssetID, tag.Source, tag.Tag),
			"asset_id":   tag.AssetID,
			"label":      tag.Tag,
			"task":       "tag",
			"source":     tag.Source,
			"confidence": tag.Confidence,
			"created_at": tag.CreatedAt,
			"metadata":   tag.Metadata,
		})
	}
	return out
}

func (s *Server) assetSafetyRecords(ctx context.Context, assetID string) []map[string]any {
	predictions, _ := s.deps.Store.ListAIPredictions(ctx, assetID)
	tags, _ := s.deps.Store.ListAssetTags(ctx, assetID)
	return s.safetyRecordsFrom(predictions, tags)
}

func (s *Server) safetyRecordsFrom(predictions []catalog.AIPrediction, tags []catalog.AssetTag) []map[string]any {
	out := []map[string]any{}
	for _, prediction := range predictions {
		if prediction.Task != "safety_nsfw" && prediction.Task != "nsfw" && prediction.Task != "safety" {
			continue
		}
		out = append(out, map[string]any{
			"id":         prediction.ID,
			"asset_id":   prediction.AssetID,
			"label":      prediction.Label,
			"task":       prediction.Task,
			"model":      prediction.ModelName,
			"score":      prediction.Confidence,
			"created_at": prediction.CreatedAt,
			"metadata":   prediction.Metadata,
		})
	}
	for _, tag := range tags {
		if tag.Source != "ai_safety" && tag.Source != "manual_safety_review" && !strings.HasPrefix(tag.Tag, "safety:") {
			continue
		}
		out = append(out, map[string]any{
			"id":         fmt.Sprintf("%s:%s:%s", tag.AssetID, tag.Source, tag.Tag),
			"asset_id":   tag.AssetID,
			"label":      tag.Tag,
			"source":     tag.Source,
			"score":      tag.Confidence,
			"created_at": tag.CreatedAt,
			"metadata":   tag.Metadata,
		})
	}
	return out
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

func intQuery(query url.Values, key string, fallback, min, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(query.Get(key)))
	if err != nil {
		value = fallback
	}
	if value < min {
		value = min
	}
	if value > max {
		value = max
	}
	return value
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
	case "active":
		sort.Slice(out, func(i, j int) bool {
			left, right := jobStatusPriority(out[i].Status), jobStatusPriority(out[j].Status)
			if left != right {
				return left < right
			}
			if out[i].Status == jobs.StatusQueued && out[j].Status == jobs.StatusQueued {
				return out[i].CreatedAt.Before(out[j].CreatedAt)
			}
			if isActiveJobStatus(out[i].Status) && isActiveJobStatus(out[j].Status) {
				return out[i].CreatedAt.Before(out[j].CreatedAt)
			}
			return out[i].CreatedAt.After(out[j].CreatedAt)
		})
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

func jobStatusPriority(status jobs.Status) int {
	switch status {
	case jobs.StatusRunning:
		return 0
	case jobs.StatusCancelRequested:
		return 1
	case jobs.StatusQueued:
		return 2
	case jobs.StatusFailed:
		return 3
	case jobs.StatusCanceled:
		return 4
	case jobs.StatusSucceeded:
		return 5
	default:
		return 6
	}
}

func isActiveJobStatus(status jobs.Status) bool {
	return status == jobs.StatusRunning || status == jobs.StatusCancelRequested
}

func truthyQuery(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func summarizeJobsForList(input []jobs.Job) []jobs.Job {
	out := make([]jobs.Job, len(input))
	for i, job := range input {
		out[i] = job
		out[i].Payload = summarizeJobPayload(job.Payload)
	}
	return out
}

func summarizeJobPayload(payload any) any {
	bytes, err := json.Marshal(payload)
	if err != nil || len(bytes) <= 16*1024 {
		return payload
	}
	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		return map[string]any{"summary": "large payload omitted from list response", "bytes": len(bytes)}
	}
	result, _ := decoded["result"].(map[string]any)
	scope := decoded["scope"]
	summary := map[string]any{
		"summary":       "large payload summarized for jobs list; request the job by id for full payload",
		"kind":          decoded["kind"],
		"scope":         scope,
		"result_status": result["status"],
		"processed":     result["processed"],
		"targets":       result["targets"],
		"skipped":       result["skipped"],
		"stored":        result["stored"],
		"worker_id":     result["worker_id"],
		"endpoint":      result["endpoint"],
		"bytes":         len(bytes),
	}
	if skippedKinds, ok := result["skipped_kinds"]; ok {
		summary["skipped_kinds"] = skippedKinds
	}
	return summary
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
			prev.ElevationM = interpolateOptionalFloat64(prev.ElevationM, next.ElevationM, ratio)
			prev.SpeedMPS = interpolateOptionalFloat64(prev.SpeedMPS, next.SpeedMPS, ratio)
			prev.RecordedAt = target
			return prev, "interpolated", nil
		}
	}
	return timed[len(timed)-1], "nearest", nil
}

func interpolateTrackPointWithTolerance(points []catalog.TrackPoint, target time.Time, tolerance time.Duration) (catalog.TrackPoint, string, error) {
	point, mode, err := interpolateTrackPoint(points, target)
	if err == nil {
		return point, mode, nil
	}
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
	if target.Before(timed[0].RecordedAt) && timed[0].RecordedAt.Sub(target) <= tolerance {
		return timed[0], "clamped_start", nil
	}
	last := timed[len(timed)-1]
	if target.After(last.RecordedAt) && target.Sub(last.RecordedAt) <= tolerance {
		return last, "clamped_end", nil
	}
	return catalog.TrackPoint{}, "", catalog.ErrNotFound
}

func interpolateOptionalFloat64(left, right *float64, ratio float64) *float64 {
	if left == nil && right == nil {
		return nil
	}
	if left == nil {
		value := *right
		return &value
	}
	if right == nil {
		value := *left
		return &value
	}
	value := *left + (*right-*left)*ratio
	return &value
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
	if strings.EqualFold(storageName, "all") {
		storageName = ""
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
	if strings.EqualFold(storageName, "all") {
		storageName = ""
	}
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
	if strings.EqualFold(storageName, "all") {
		storageName = ""
	}
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

func searchTokens(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var tokens []string
	var b strings.Builder
	inQuote := false
	for _, r := range raw {
		switch {
		case r == '"':
			inQuote = !inQuote
			if !inQuote && b.Len() > 0 {
				tokens = append(tokens, strings.ToLower(strings.TrimSpace(b.String())))
				b.Reset()
			}
		case !inQuote && (r == ' ' || r == '\t' || r == '\n'):
			if b.Len() > 0 {
				tokens = append(tokens, strings.ToLower(strings.TrimSpace(b.String())))
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		tokens = append(tokens, strings.ToLower(strings.TrimSpace(b.String())))
	}
	return compactStrings(tokens)
}

func assetSearchMatches(asset catalog.Asset, tokens []string, searchCtx assetSearchContext) []string {
	if len(tokens) == 0 {
		return []string{"all indexed assets"}
	}
	var matched []string
	for _, token := range tokens {
		fields := assetTokenOrMatches(asset, token, searchCtx)
		if len(fields) == 0 {
			return nil
		}
		matched = append(matched, fields...)
	}
	sort.Strings(matched)
	return uniqueStrings(matched)
}

func assetTokenOrMatches(asset catalog.Asset, token string, searchCtx assetSearchContext) []string {
	parts := strings.Split(token, ",")
	if len(parts) <= 1 {
		return assetTokenMatches(asset, token, searchCtx)
	}
	var out []string
	for _, part := range parts {
		fields := assetTokenMatches(asset, strings.TrimSpace(part), searchCtx)
		if len(fields) > 0 {
			out = append(out, fields...)
		}
	}
	return uniqueStrings(out)
}

func assetTokenMatches(asset catalog.Asset, token string, searchCtx assetSearchContext) []string {
	token = strings.TrimSpace(strings.ToLower(token))
	if token == "" {
		return nil
	}
	var out []string
	plain := token
	prefix := ""
	if before, after, ok := strings.Cut(token, ":"); ok {
		prefix = before
		plain = after
	}
	if prefix == "type" || prefix == "kind" || prefix == "media" {
		if strings.EqualFold(asset.MediaKind, plain) {
			return []string{"media kind"}
		}
		return nil
	}
	metadataText := ""
	if len(asset.Metadata) > 0 {
		if data, err := json.Marshal(asset.Metadata); err == nil {
			metadataText = strings.ToLower(string(data))
		}
	}
	if prefix == "tag" || prefix == "category" || prefix == "safety" || prefix == "caption" || prefix == "ocr" {
		if prefix == "ocr" {
			if _, ok := searchCtx.predictionMatches[token][asset.ID]; ok {
				return []string{"matched OCR text"}
			}
			return nil
		}
		if prefix == "caption" {
			fields := []string{}
			if _, ok := searchCtx.tagMatches[token][asset.ID]; ok {
				fields = append(fields, "AI tag/prediction")
			}
			if _, ok := searchCtx.predictionMatches[token][asset.ID]; ok {
				fields = append(fields, "AI prediction")
			}
			if _, ok := searchCtx.frameMatches[token][asset.ID]; ok {
				fields = append(fields, "matched video frame caption")
			}
			return uniqueStrings(fields)
		}
		if _, ok := searchCtx.tagMatches[token][asset.ID]; ok {
			return []string{"AI tag/prediction"}
		}
		if _, ok := searchCtx.predictionMatches[token][asset.ID]; ok {
			return []string{"AI prediction"}
		}
		return nil
	}
	if prefix == "face" {
		if _, ok := searchCtx.faceMatches[token][asset.ID]; ok {
			return []string{"face detection"}
		}
		return nil
	}
	if prefix == "transcript" {
		if _, ok := searchCtx.transcriptMatches[token][asset.ID]; ok {
			return []string{"matched transcript"}
		}
		return nil
	}
	if prefix == "document" {
		if _, ok := searchCtx.documentMatches[token][asset.ID]; ok {
			return []string{"matched document text"}
		}
		return nil
	}
	if prefix == "genre" || prefix == "key" || prefix == "tempo" {
		if _, ok := searchCtx.audioMatches[token][asset.ID]; ok {
			return []string{"matched audio features"}
		}
		return nil
	}
	if prefix == "frame" || prefix == "video-caption" {
		if _, ok := searchCtx.frameMatches[token][asset.ID]; ok {
			return []string{"matched video frame caption"}
		}
		return nil
	}
	if prefix == "camera" || prefix == "exif" {
		if textMatchesSearch(metadataText, plain) {
			return []string{"metadata/EXIF"}
		}
		return nil
	}
	if prefix == "filename" {
		if textMatchesSearch(strings.ToLower(asset.DisplayName), plain) {
			return []string{"filename"}
		}
		for _, loc := range asset.Locations {
			if textMatchesSearch(strings.ToLower(loc.FileName), plain) {
				return []string{"filename"}
			}
		}
		return nil
	}
	if prefix == "path" {
		for _, loc := range asset.Locations {
			if textMatchesSearch(strings.ToLower(loc.RelativePath), plain) {
				return []string{"path"}
			}
		}
		return nil
	}
	if prefix == "hash" {
		for _, loc := range asset.Locations {
			if strings.HasPrefix(strings.ToLower(loc.SHA512Hex), plain) {
				return []string{"SHA-512 prefix"}
			}
		}
		return nil
	}
	if prefix == "album" {
		if _, ok := searchCtx.albumAssetMatches[token][asset.ID]; ok {
			return []string{"album"}
		}
		return nil
	}
	if prefix == "place" {
		if _, ok := searchCtx.placeMatches[token][asset.ID]; ok {
			return []string{"local place bbox"}
		}
		return nil
	}
	if prefix == "track" {
		if _, ok := searchCtx.trackNameMatches[token][asset.ID]; ok {
			return []string{"track"}
		}
		return nil
	}
	if prefix == "ext" || prefix == "extension" {
		for _, loc := range asset.Locations {
			if strings.TrimPrefix(strings.ToLower(loc.Extension), ".") == strings.TrimPrefix(plain, ".") {
				return []string{"extension"}
			}
		}
		return nil
	}
	if strings.Contains(token, "..") {
		if assetMatchesDateRange(asset, token) {
			return []string{"date range"}
		}
		return nil
	}
	if textMatchesSearch(strings.ToLower(asset.DisplayName), plain) {
		out = append(out, "filename")
	}
	if _, ok := searchCtx.tagMatches[token][asset.ID]; ok {
		out = append(out, "AI tag")
	}
	if _, ok := searchCtx.predictionMatches[token][asset.ID]; ok {
		out = append(out, "AI prediction/caption/class")
	}
	if _, ok := searchCtx.albumAssetMatches[token][asset.ID]; ok {
		out = append(out, "album")
	}
	if _, ok := searchCtx.trackNameMatches[token][asset.ID]; ok {
		out = append(out, "track")
	}
	if _, ok := searchCtx.faceMatches[token][asset.ID]; ok {
		out = append(out, "face")
	}
	if _, ok := searchCtx.transcriptMatches[token][asset.ID]; ok {
		out = append(out, "matched transcript")
	}
	if _, ok := searchCtx.documentMatches[token][asset.ID]; ok {
		out = append(out, "matched document text")
	}
	if _, ok := searchCtx.audioMatches[token][asset.ID]; ok {
		out = append(out, "matched audio features")
	}
	if _, ok := searchCtx.frameMatches[token][asset.ID]; ok {
		out = append(out, "matched video frame caption")
	}
	if _, ok := searchCtx.placeMatches[token][asset.ID]; ok {
		out = append(out, "local place bbox")
	}
	if strings.Contains(strings.ToLower(asset.MediaKind), plain) {
		out = append(out, "media kind")
	}
	if asset.TakenAt != nil && strings.Contains(asset.TakenAt.Format("2006-01-02 2006-01 2006"), plain) {
		out = append(out, "date")
	}
	if textMatchesSearch(metadataText, plain) {
		out = append(out, "metadata/EXIF")
	}
	for _, loc := range asset.Locations {
		if textMatchesSearch(strings.ToLower(loc.RelativePath), plain) {
			out = append(out, "path")
		}
		if strings.TrimPrefix(strings.ToLower(loc.Extension), ".") == strings.TrimPrefix(plain, ".") {
			out = append(out, "extension")
		}
		if strings.HasPrefix(strings.ToLower(loc.SHA512Hex), plain) && plain != "" {
			out = append(out, "SHA-512 prefix")
		}
		if textMatchesSearch(strings.ToLower(loc.FileName), plain) {
			out = append(out, "filename")
		}
	}
	return uniqueStrings(out)
}

func textMatchesSearch(text, query string) bool {
	query = strings.TrimSpace(strings.ToLower(strings.Trim(query, `"`)))
	if query == "" {
		return false
	}
	if strings.ContainsAny(query, "*?") {
		needle := strings.ReplaceAll(query, "*", "")
		needle = strings.ReplaceAll(needle, "?", "")
		return needle != "" && strings.Contains(text, needle)
	}
	return strings.Contains(text, query)
}

func audioFeatureMatchesToken(features catalog.AudioFeatures, prefix, plain string) bool {
	text := strings.ToLower(strings.Join(features.GenreLabels, " ") + " " + features.Key + " " + features.Mode + " " + features.Model)
	switch prefix {
	case "genre":
		return strings.Contains(strings.ToLower(strings.Join(features.GenreLabels, " ")), plain)
	case "key":
		return strings.EqualFold(features.Key, plain) || strings.Contains(strings.ToLower(features.Key+" "+features.Mode), plain)
	case "tempo":
		if features.TempoBPM == nil {
			return false
		}
		if strings.Contains(plain, "..") {
			minRaw, maxRaw, _ := strings.Cut(plain, "..")
			minRaw = strings.TrimSpace(minRaw)
			maxRaw = strings.TrimSpace(maxRaw)
			if minRaw != "" {
				minValue, err := strconv.ParseFloat(minRaw, 64)
				if err != nil || *features.TempoBPM < minValue {
					return false
				}
			}
			if maxRaw != "" {
				maxValue, err := strconv.ParseFloat(maxRaw, 64)
				if err != nil || *features.TempoBPM > maxValue {
					return false
				}
			}
			return true
		}
		target, err := strconv.ParseFloat(plain, 64)
		if err != nil {
			return strings.Contains(strings.ToLower(fmt.Sprintf("%.0f %.1f bpm", *features.TempoBPM, *features.TempoBPM)), plain)
		}
		return math.Abs(*features.TempoBPM-target) <= 2
	default:
		if strings.Contains(text, plain) {
			return true
		}
		if features.TempoBPM != nil && strings.Contains(strings.ToLower(fmt.Sprintf("%.0f %.1f bpm", *features.TempoBPM, *features.TempoBPM)), plain) {
			return true
		}
		return false
	}
}

func trackSearchMatches(track catalog.TrackSummary, tokens []string, places []catalog.PlaceCacheEntry) []string {
	if len(tokens) == 0 {
		return []string{"all parsed tracks"}
	}
	var matched []string
	text := strings.ToLower(strings.Join([]string{
		track.TrackAssetID,
		track.Name,
		track.SourceFormat,
		timePtrSearchString(track.StartTime),
		timePtrSearchString(track.EndTime),
	}, " "))
	for _, token := range tokens {
		plain := strings.ToLower(strings.TrimSpace(token))
		if before, after, ok := strings.Cut(plain, ":"); ok {
			if before != "track" && before != "type" && before != "kind" && before != "media" && before != "format" && before != "date" && before != "place" {
				continue
			}
			plain = after
		}
		if plain == "" {
			continue
		}
		if strings.Contains(text, plain) {
			matched = append(matched, "track metadata")
		}
		if place, ok := placeForQuery(plain, places); ok && trackOverlapsPlace(track, place) {
			matched = append(matched, "local place bbox")
		}
		if track.PointCount > 0 && strings.Contains(strconv.Itoa(track.PointCount), plain) {
			matched = append(matched, "point count")
		}
		if track.DistanceM > 0 && strings.Contains(fmt.Sprintf("%.2f", track.DistanceM/1000), plain) {
			matched = append(matched, "distance")
		}
	}
	sort.Strings(matched)
	return uniqueStrings(matched)
}

func placeForQuery(query string, places []catalog.PlaceCacheEntry) (catalog.PlaceCacheEntry, bool) {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return catalog.PlaceCacheEntry{}, false
	}
	for _, place := range places {
		if strings.EqualFold(query, strings.ToLower(place.Name)) {
			return place, true
		}
		if query == strings.ToLower(place.NormalizedName) {
			return place, true
		}
		for _, alias := range place.Aliases {
			if query == strings.ToLower(alias) {
				return place, true
			}
		}
		if strings.Contains(strings.ToLower(strings.Join([]string{place.DisplayName, place.Country, place.Region, place.City, place.Road}, " ")), query) {
			return place, true
		}
	}
	return catalog.PlaceCacheEntry{}, false
}

func defaultPlaceEntries() []catalog.PlaceCacheEntry {
	return []catalog.PlaceCacheEntry{
		{
			Name:           "Yerevan",
			NormalizedName: "yerevan",
			DisplayName:    "Yerevan, Armenia",
			Aliases:        []string{"yerevan", "erevan", "երեւան", "երևան"},
			Provider:       "local",
			Country:        "Armenia",
			Region:         "Yerevan",
			City:           "Yerevan",
			Lat:            40.1872,
			Lon:            44.5152,
			BBox:           catalog.BBox{MinLon: 44.35, MinLat: 40.05, MaxLon: 44.68, MaxLat: 40.28},
			Source:         "built_in_seed",
		},
		{
			Name:           "Vanadzor",
			NormalizedName: "vanadzor",
			DisplayName:    "Vanadzor, Lori Province, Armenia",
			Aliases:        []string{"vanadzor", "kirovakan", "վանաձոր"},
			Provider:       "local",
			Country:        "Armenia",
			Region:         "Lori Province",
			City:           "Vanadzor",
			Lat:            40.8128,
			Lon:            44.4883,
			BBox:           catalog.BBox{MinLon: 44.38, MinLat: 40.72, MaxLon: 44.62, MaxLat: 40.90},
			Source:         "built_in_seed",
		},
		{
			Name:           "Lori Province",
			NormalizedName: "lori province",
			DisplayName:    "Lori Province, Armenia",
			Aliases:        []string{"lori", "lori province", "lori marz", "լոռի", "լոռու մարզ"},
			Provider:       "local",
			Country:        "Armenia",
			Region:         "Lori Province",
			Lat:            40.9631,
			Lon:            44.4730,
			BBox:           catalog.BBox{MinLon: 43.78, MinLat: 40.68, MaxLon: 45.05, MaxLat: 41.32},
			Source:         "built_in_seed",
		},
		{
			Name:           "Armenia",
			NormalizedName: "armenia",
			DisplayName:    "Armenia",
			Aliases:        []string{"armenia", "hayastan", "հայաստան"},
			Provider:       "local",
			Country:        "Armenia",
			Lat:            40.0691,
			Lon:            45.0382,
			BBox:           catalog.BBox{MinLon: 43.45, MinLat: 38.80, MaxLon: 46.70, MaxLat: 41.35},
			Source:         "built_in_seed",
		},
	}
}

func (s *Server) assetPlaceRecords(ctx context.Context, asset catalog.Asset) []assetPlaceRecord {
	type coordinate struct {
		source    string
		geoSource string
		lat       float64
		lon       float64
		metadata  map[string]any
	}
	coordinates := []coordinate{}
	addCoordinate := func(source, geoSource string, lat, lon float64, metadata map[string]any) {
		if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			return
		}
		for _, existing := range coordinates {
			if existing.source == source && math.Abs(existing.lat-lat) < 0.000001 && math.Abs(existing.lon-lon) < 0.000001 {
				return
			}
		}
		coordinates = append(coordinates, coordinate{source: source, geoSource: geoSource, lat: lat, lon: lon, metadata: metadata})
	}
	if geo, err := s.deps.Store.GetAssetGeo(ctx, asset.ID); err == nil {
		addCoordinate("asset_geo", geo.Source, geo.Lat, geo.Lon, map[string]any{
			"confidence":     geo.Confidence,
			"track_asset_id": geo.TrackAssetID,
			"taken_at":       geo.TakenAt,
			"metadata":       geo.Metadata,
		})
	}
	if lat, lon, ok := metadataCoordinate(asset.Metadata, "gps_lat", "gps_lon"); ok {
		addCoordinate("exif", "metadata", lat, lon, map[string]any{"source": "asset.metadata.gps_lat/gps_lon"})
	} else if lat, lon, ok := metadataCoordinate(asset.Metadata, "lat", "lon"); ok {
		addCoordinate("metadata", "metadata", lat, lon, map[string]any{"source": "asset.metadata.lat/lon"})
	}
	out := []assetPlaceRecord{}
	seen := map[string]struct{}{}
	places := s.placeEntries(ctx)
	radiusM := float64(runtimeIntSetting("search.reverse_geocode_radius_m", 100))
	if radiusM < 0 {
		radiusM = 0
	}
	if radiusM > 5000 {
		radiusM = 5000
	}
	for _, coordinate := range coordinates {
		matches := mergePlaceMatches(
			placesForPoint(coordinate.lat, coordinate.lon, places),
			nearbyPlacesForPoint(coordinate.lat, coordinate.lon, places, radiusM),
		)
		for _, place := range matches {
			key := fmt.Sprintf("%s|%s|%.6f|%.6f", coordinate.source, strings.ToLower(place.Name), coordinate.lat, coordinate.lon)
			if _, ok := seen[key]; ok {
				continue
			}
			matchKind := firstNonEmpty(stringFromMap(place.Metadata, "match_kind"), "cached place match")
			if distance := floatFromAny(place.Metadata["distance_m"]); distance > 0 {
				matchKind = fmt.Sprintf("%s within %.0f m", matchKind, distance)
			}
			seen[key] = struct{}{}
			out = append(out, assetPlaceRecord{
				CoordinateSource: coordinate.source,
				GeoSource:        coordinate.geoSource,
				Lat:              coordinate.lat,
				Lon:              coordinate.lon,
				PlaceName:        place.Name,
				DisplayName:      place.DisplayName,
				Provider:         place.Provider,
				Source:           place.Source,
				Match:            matchKind,
				BBox:             place.BBox,
				Metadata:         coordinate.metadata,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CoordinateSource == out[j].CoordinateSource {
			return out[i].DisplayName < out[j].DisplayName
		}
		return out[i].CoordinateSource < out[j].CoordinateSource
	})
	return out
}

func (s *Server) assetOCRBlocks(ctx context.Context, assetID string) []ocrBlockRecord {
	predictions, err := s.deps.Store.ListAIPredictions(ctx, assetID)
	if err != nil {
		return []ocrBlockRecord{}
	}
	return ocrBlocksFromPredictions(predictions)
}

func ocrBlocksFromPredictions(predictions []catalog.AIPrediction) []ocrBlockRecord {
	out := []ocrBlockRecord{}
	for _, prediction := range predictions {
		if prediction.Task != "ocr_image" && prediction.Task != "ocr" && prediction.Task != "ocr_text" {
			continue
		}
		text := strings.TrimSpace(prediction.Label)
		if text == "" {
			continue
		}
		out = append(out, ocrBlockRecord{
			ID:         prediction.ID,
			AssetID:    prediction.AssetID,
			Text:       text,
			Language:   stringFromMap(prediction.Metadata, "language"),
			Engine:     stringFromMap(prediction.Metadata, "engine"),
			Confidence: prediction.Confidence,
			X:          floatFromAny(prediction.Metadata["x"]),
			Y:          floatFromAny(prediction.Metadata["y"]),
			Width:      floatFromAny(prediction.Metadata["width"]),
			Height:     floatFromAny(prediction.Metadata["height"]),
			ModelName:  prediction.ModelName,
			CreatedAt:  prediction.CreatedAt,
			Metadata:   prediction.Metadata,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y == out[j].Y {
			return out[i].X < out[j].X
		}
		return out[i].Y < out[j].Y
	})
	return out
}

func ocrSummaryFromBlocks(blocks []ocrBlockRecord) ocrSummaryRecord {
	languages := map[string]struct{}{}
	engines := map[string]struct{}{}
	lines := []string{}
	currentLine := []string{}
	var lastY float64
	var newest time.Time
	modelName := ""
	for i, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		if block.Language != "" {
			languages[block.Language] = struct{}{}
		}
		if block.Engine != "" {
			engines[block.Engine] = struct{}{}
		}
		if modelName == "" {
			modelName = block.ModelName
		}
		if block.CreatedAt.After(newest) {
			newest = block.CreatedAt
		}
		if i > 0 && block.Y > 0 && lastY > 0 && math.Abs(block.Y-lastY) > 0.025 {
			if len(currentLine) > 0 {
				lines = append(lines, strings.Join(currentLine, " "))
			}
			currentLine = []string{}
		}
		currentLine = append(currentLine, text)
		if block.Y > 0 {
			lastY = block.Y
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, strings.Join(currentLine, " "))
	}
	return ocrSummaryRecord{
		FullText:   strings.TrimSpace(strings.Join(lines, "\n")),
		Languages:  sortedMapKeys(languages),
		Engines:    sortedMapKeys(engines),
		ModelName:  modelName,
		CreatedAt:  newest,
		BlockCount: len(blocks),
	}
}

func sortedMapKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func parsePositiveInt(raw string, fallback, maxValue int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func (s *Server) seedDefaultPlaces() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, place := range defaultPlaceEntries() {
		_, _ = s.deps.Store.UpsertPlace(ctx, place)
	}
}

func (s *Server) placeEntries(ctx context.Context) []catalog.PlaceCacheEntry {
	places, err := s.deps.Store.ListPlaces(ctx, catalog.PlaceQuery{Limit: 100000})
	if err == nil && len(places) > 0 {
		return places
	}
	return defaultPlaceEntries()
}

func placesForPoint(lat, lon float64, places []catalog.PlaceCacheEntry) []catalog.PlaceCacheEntry {
	out := []catalog.PlaceCacheEntry{}
	for _, place := range places {
		if lon >= place.BBox.MinLon && lon <= place.BBox.MaxLon && lat >= place.BBox.MinLat && lat <= place.BBox.MaxLat {
			out = append(out, placeWithDistance(place, 0, "bbox_contains"))
		}
	}
	return out
}

func nearbyPlacesForPoint(lat, lon float64, places []catalog.PlaceCacheEntry, radiusM float64) []catalog.PlaceCacheEntry {
	if radiusM <= 0 {
		return nil
	}
	type match struct {
		place    catalog.PlaceCacheEntry
		distance float64
	}
	matches := []match{}
	for _, place := range places {
		if place.Lat == 0 && place.Lon == 0 {
			continue
		}
		distance := haversineMeters(lat, lon, place.Lat, place.Lon)
		if distance <= radiusM {
			matches = append(matches, match{place: placeWithDistance(place, distance, "nearby_centroid"), distance: distance})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].distance < matches[j].distance })
	out := make([]catalog.PlaceCacheEntry, 0, len(matches))
	for _, item := range matches {
		out = append(out, item.place)
	}
	return out
}

func mergePlaceMatches(groups ...[]catalog.PlaceCacheEntry) []catalog.PlaceCacheEntry {
	seen := map[string]struct{}{}
	out := []catalog.PlaceCacheEntry{}
	for _, group := range groups {
		for _, place := range group {
			key := firstNonEmpty(place.ID, place.Provider+":"+place.NormalizedName, place.DisplayName)
			if key == "" {
				key = fmt.Sprintf("%.7f,%.7f:%s", place.Lat, place.Lon, place.Name)
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, place)
		}
	}
	return out
}

func reverseGeocodeCacheSatisfied(places []catalog.PlaceCacheEntry) bool {
	for _, place := range places {
		if isProviderBackedReverseGeocode(place) {
			return true
		}
	}
	return false
}

func isProviderBackedReverseGeocode(place catalog.PlaceCacheEntry) bool {
	provider := strings.ToLower(strings.TrimSpace(place.Provider))
	if provider != "" {
		provider = strings.Split(provider, ":")[0]
		provider = normalizeGeocoderProvider(provider)
	}
	source := strings.ToLower(strings.TrimSpace(place.Source))
	if strings.Contains(source, "online") || strings.Contains(source, "geocoder") || strings.Contains(source, "nominatim") {
		return true
	}
	switch provider {
	case "nominatim", "nominatim_compatible", "photon", "pelias", "google":
		return true
	default:
		return false
	}
}

func placeWithDistance(place catalog.PlaceCacheEntry, distanceM float64, matchKind string) catalog.PlaceCacheEntry {
	metadata := map[string]any{}
	for key, value := range place.Metadata {
		metadata[key] = value
	}
	metadata["match_kind"] = matchKind
	metadata["distance_m"] = distanceM
	place.Metadata = metadata
	return place
}

func metadataCoordinate(metadata map[string]any, latKey, lonKey string) (float64, float64, bool) {
	if metadata == nil {
		return 0, 0, false
	}
	lat, latOK := metadataFloat(metadata[latKey])
	lon, lonOK := metadataFloat(metadata[lonKey])
	return lat, lon, latOK && lonOK
}

func metadataFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		f, err := typed.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func trackOverlapsPlace(track catalog.TrackSummary, place catalog.PlaceCacheEntry) bool {
	if track.MinLat == nil || track.MinLon == nil || track.MaxLat == nil || track.MaxLon == nil {
		return false
	}
	return *track.MaxLon >= place.BBox.MinLon &&
		*track.MinLon <= place.BBox.MaxLon &&
		*track.MaxLat >= place.BBox.MinLat &&
		*track.MinLat <= place.BBox.MaxLat
}

func timePtrSearchString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339 + " 2006-01-02 2006-01 2006")
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	return stringFromAny(values[key])
}

func stringFromAny(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}

func assetMatchesDateRange(asset catalog.Asset, token string) bool {
	startRaw, endRaw, ok := strings.Cut(token, "..")
	if !ok || asset.TakenAt == nil {
		return false
	}
	start, startOK := parseSearchDateBound(startRaw, false)
	end, endOK := parseSearchDateBound(endRaw, true)
	if !startOK && !endOK {
		return false
	}
	taken := *asset.TakenAt
	if startOK && taken.Before(start) {
		return false
	}
	if endOK && taken.After(end) {
		return false
	}
	return true
}

func parseSearchDateBound(raw string, end bool) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		value, err := time.ParseInLocation(layout, raw, time.UTC)
		if err != nil {
			continue
		}
		if !end {
			return value, true
		}
		switch layout {
		case "2006":
			return value.AddDate(1, 0, 0).Add(-time.Nanosecond), true
		case "2006-01":
			return value.AddDate(0, 1, 0).Add(-time.Nanosecond), true
		default:
			return value.AddDate(0, 0, 1).Add(-time.Nanosecond), true
		}
	}
	return time.Time{}, false
}

func searchExplanation(matched []string) string {
	if len(matched) == 0 {
		return "matched indexed asset"
	}
	return "matched by " + strings.Join(matched, ", ")
}

func searchWarnings(tokens []string) []string {
	var warnings []string
	for _, token := range tokens {
		if strings.HasPrefix(token, "near:") {
			warnings = append(warnings, token+" is recognized as a future structured filter; current search uses indexed asset fields.")
		}
	}
	return uniqueStrings(warnings)
}

func anySliceToStrings(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && text != "" {
			out = append(out, text)
		}
	}
	return out
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		out, _ := typed.Float64()
		return out
	case string:
		out, _ := strconv.ParseFloat(typed, 64)
		return out
	default:
		return 0
	}
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case json.Number:
		out, _ := typed.Int64()
		return out
	case string:
		out, _ := strconv.ParseInt(typed, 10, 64)
		return out
	default:
		return 0
	}
}

func floatPtrFromAny(value any) *float64 {
	if value == nil {
		return nil
	}
	out := floatFromAny(value)
	return &out
}

func floatPtrFromOptionalAny(value any) *float64 {
	if value == nil {
		return nil
	}
	if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
		return nil
	}
	out := floatFromAny(value)
	return &out
}

func int64Ptr(value int) *int64 {
	out := int64(value)
	return &out
}

func floatSliceFromAny(value any) []float64 {
	switch typed := value.(type) {
	case []float64:
		return typed
	case []any:
		out := make([]float64, 0, len(typed))
		for _, item := range typed {
			out = append(out, floatFromAny(item))
		}
		return out
	case []json.RawMessage:
		out := make([]float64, 0, len(typed))
		for _, item := range typed {
			var number float64
			if err := json.Unmarshal(item, &number); err == nil {
				out = append(out, number)
			}
		}
		return out
	default:
		return nil
	}
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return compactStrings(typed)
	case []any:
		return compactStrings(anySliceToStrings(typed))
	default:
		text := strings.TrimSpace(stringFromAny(value))
		if text == "" {
			return []string{}
		}
		parts := strings.Split(text, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if value := strings.TrimSpace(part); value != "" {
				out = append(out, value)
			}
		}
		return compactStrings(out)
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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
