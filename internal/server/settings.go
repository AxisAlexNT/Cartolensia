package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/id"
	"gopkg.in/yaml.v3"
)

var runtimeSettings = struct {
	sync.RWMutex
	values map[string]any
}{values: map[string]any{
	"indexing.default_max_files":    50,
	"indexing.hash_after_index":     true,
	"indexing.metadata_after_index": true,
	"indexing.previews_after_index": false,
	"map.cluster_radius_px":         64,
	"map.tiles_enabled":             true,
	"preview.cache_max_bytes":       int64(10 * 1024 * 1024 * 1024),
	"gallery.default_view":          "tile",
	"transcode.session_ttl":         "2h",
}}

var pluginSettings = struct {
	sync.RWMutex
	values map[string]map[string]any
}{values: map[string]map[string]any{}}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, s.settingsPayload())
}

func (s *Server) handleSettingsSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tabs":             s.settingsPayload()["tabs"],
		"runtime_settings": runtimeSettingsSchema(),
		"pending_settings": pendingSettingsSchema(),
		"plugin_settings":  "see /api/v1/plugins/{id}/settings/schema",
	})
}

func (s *Server) handleSettingsEffective(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"config":            s.deps.Config,
		"runtime_settings":  runtimeSettingsSnapshot(),
		"restart_required":  restartRequiredSettings(),
		"yaml_bound_fields": yamlBoundSettings(),
	})
}

func (s *Server) handleSettingsPending(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writePendingSettings(w)
	case http.MethodPatch:
		if !s.requireWrite(w, r, "settings.pending.update") {
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if len(req) == 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("pending settings payload is empty"))
			return
		}
		data, err := yaml.Marshal(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		target := s.pendingConfigPath()
		if err := ensurePathInside(s.deps.Config.Cache.Dir, target); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		s.writePendingSettings(w)
	case http.MethodDelete:
		if !s.requireWrite(w, r, "settings.pending.delete") {
			return
		}
		if err := ensurePathInside(s.deps.Config.Cache.Dir, s.pendingConfigPath()); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		_ = os.Remove(s.pendingConfigPath())
		writeJSON(w, http.StatusOK, map[string]any{"exists": false, "restart_required": false})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleSettingsPendingDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	target := s.pendingConfigPath()
	if err := ensurePathInside(s.deps.Config.Cache.Dir, target); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := os.Stat(target); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml")
	http.ServeFile(w, r, target)
}

func (s *Server) writePendingSettings(w http.ResponseWriter) {
	target := s.pendingConfigPath()
	payload := map[string]any{
		"exists":           false,
		"path":             target,
		"restart_required": false,
		"download_url":     "/api/v1/settings/pending/download",
	}
	if err := ensurePathInside(s.deps.Config.Cache.Dir, target); err != nil {
		payload["error"] = err.Error()
		writeJSON(w, http.StatusOK, payload)
		return
	}
	data, err := os.ReadFile(target)
	if err != nil {
		writeJSON(w, http.StatusOK, payload)
		return
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		payload["error"] = err.Error()
	} else {
		payload["pending"] = parsed
	}
	payload["exists"] = true
	payload["restart_required"] = true
	payload["bytes"] = len(data)
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) pendingConfigPath() string {
	return filepath.Join(s.deps.Config.Cache.Dir, "pending-config.yaml")
}

func (s *Server) handleSettingsRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "settings.runtime.update") {
		return
	}
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	accepted := map[string]any{}
	runtimeSettings.Lock()
	for key, value := range req {
		if _, ok := runtimeSettings.values[key]; !ok {
			continue
		}
		runtimeSettings.values[key] = value
		accepted[key] = value
	}
	snapshot := cloneMap(runtimeSettings.values)
	runtimeSettings.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"updated": accepted, "runtime_settings": snapshot})
}

func (s *Server) handleSettingsRestartRequired(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, restartRequiredSettings())
}

func (s *Server) settingsPayload() map[string]any {
	return map[string]any{
		"tabs": []map[string]any{
			{"id": "general", "label": "General", "runtime": true},
			{"id": "server", "label": "Server/HTTP/HTTPS", "runtime": false},
			{"id": "storage", "label": "Storage", "runtime": false},
			{"id": "indexing", "label": "Indexing/Discovery", "runtime": true},
			{"id": "metadata", "label": "Metadata/EXIF", "runtime": true},
			{"id": "preview", "label": "Preview Cache", "runtime": true},
			{"id": "map", "label": "Map/Tiles", "runtime": true},
			{"id": "gps", "label": "GPS/KML Tracks", "runtime": true},
			{"id": "transcoding", "label": "Transcoding", "runtime": true},
			{"id": "ai", "label": "AI/Vector", "runtime": false},
			{"id": "auth", "label": "Auth/Security", "runtime": false},
			{"id": "backups", "label": "Backups/DB Export", "runtime": true},
			{"id": "plugins", "label": "Plugins", "runtime": true},
			{"id": "raw", "label": "Raw YAML / Effective Config", "runtime": false},
		},
		"runtime_settings":  runtimeSettingsSnapshot(),
		"pending_settings":  s.pendingSettingsSnapshot(),
		"restart_required":  restartRequiredSettings(),
		"yaml_bound_fields": yamlBoundSettings(),
		"effective": map[string]any{
			"http":     s.deps.Config.HTTP,
			"cache":    s.deps.Config.Cache,
			"storages": s.deps.Config.Storages,
			"workers":  s.deps.Config.Workers,
			"auth":     s.deps.Config.Auth,
			"plugins":  s.deps.Config.Plugins,
		},
	}
}

func (s *Server) pendingSettingsSnapshot() map[string]any {
	target := s.pendingConfigPath()
	out := map[string]any{
		"exists":           false,
		"path":             target,
		"restart_required": false,
		"download_url":     "/api/v1/settings/pending/download",
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return out
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err == nil {
		out["pending"] = parsed
	}
	out["exists"] = true
	out["restart_required"] = true
	out["bytes"] = len(data)
	return out
}

func (s *Server) handlePluginSettings(w http.ResponseWriter, r *http.Request, pluginID string) {
	switch r.Method {
	case http.MethodGet:
		pluginSettings.RLock()
		settings := cloneMap(pluginSettings.values[pluginID])
		pluginSettings.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"plugin_id": pluginID, "settings": settings})
	case http.MethodPatch:
		if !s.requireWrite(w, r, "plugins.settings.update") {
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		pluginSettings.Lock()
		if pluginSettings.values[pluginID] == nil {
			pluginSettings.values[pluginID] = map[string]any{}
		}
		for key, value := range req {
			pluginSettings.values[pluginID][key] = value
		}
		settings := cloneMap(pluginSettings.values[pluginID])
		pluginSettings.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"plugin_id": pluginID, "settings": settings})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handlePluginSettingsSchema(w http.ResponseWriter, r *http.Request, pluginID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plugin_id": pluginID,
		"fields":    pluginSettingsSchema(pluginID),
		"modes":     []string{"ui", "yaml"},
	})
}

func runtimeSettingsSnapshot() map[string]any {
	runtimeSettings.RLock()
	defer runtimeSettings.RUnlock()
	return cloneMap(runtimeSettings.values)
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func yamlBoundSettings() []string {
	return []string{
		"http.addr",
		"http.tls_cert_file",
		"http.tls_key_file",
		"http.tls_auto_self_signed",
		"database.url",
		"cache.dir",
		"storages",
		"workers.enabled",
		"auth.mode",
	}
}

func restartRequiredSettings() map[string]any {
	return map[string]any{
		"required_for": yamlBoundSettings(),
		"note":         "These values come from YAML or environment configuration and require an app restart.",
	}
}

func runtimeSettingsSchema() []map[string]any {
	return []map[string]any{
		{"tab": "indexing", "key": "indexing.default_max_files", "type": "number", "label": "Default max files"},
		{"tab": "indexing", "key": "indexing.hash_after_index", "type": "boolean", "label": "Hash after indexing"},
		{"tab": "indexing", "key": "indexing.metadata_after_index", "type": "boolean", "label": "Extract metadata after indexing"},
		{"tab": "indexing", "key": "indexing.previews_after_index", "type": "boolean", "label": "Generate previews after indexing"},
		{"tab": "preview", "key": "preview.cache_max_bytes", "type": "number", "label": "Preview cache max bytes"},
		{"tab": "map", "key": "map.cluster_radius_px", "type": "number", "label": "Cluster radius px"},
		{"tab": "map", "key": "map.tiles_enabled", "type": "boolean", "label": "OSM tiles enabled"},
		{"tab": "transcoding", "key": "transcode.session_ttl", "type": "text", "label": "Transcode session TTL"},
	}
}

func pendingSettingsSchema() []map[string]any {
	return []map[string]any{
		{"tab": "metadata", "key": "metadata.exif_enabled", "type": "boolean", "restart_required": true},
		{"tab": "metadata", "key": "metadata.exif_gps_enabled", "type": "boolean", "restart_required": true},
		{"tab": "gps", "key": "gps.parse_gpx_enabled", "type": "boolean", "restart_required": true},
		{"tab": "gps", "key": "gps.parse_kml_enabled", "type": "boolean", "restart_required": true},
		{"tab": "gps", "key": "gps.parse_kmz_enabled", "type": "boolean", "restart_required": true},
		{"tab": "gps", "key": "gps.parse_gpz_enabled", "type": "boolean", "restart_required": true},
		{"tab": "preview", "key": "preview.cache_dir", "type": "text", "restart_required": true},
		{"tab": "map", "key": "map.tile_cache_dir", "type": "text", "restart_required": true},
		{"tab": "transcoding", "key": "transcoding.ffmpeg_path", "type": "text", "restart_required": true},
	}
}

func pluginSettingsSchema(pluginID string) []map[string]any {
	common := []map[string]any{
		{"key": "enabled", "type": "boolean", "label": "Enabled"},
		{"key": "notes", "type": "text", "label": "Operator notes"},
	}
	specific := map[string][]map[string]any{
		"albums": []map[string]any{
			{"key": "default_sort", "type": "text", "label": "Default album sort"},
			{"key": "show_virtual_warning", "type": "boolean", "label": "Show virtual album warning"},
		},
		"mapview": []map[string]any{
			{"key": "default_cluster_distance_px", "type": "number", "label": "Default cluster distance px"},
			{"key": "popup_gallery_limit", "type": "number", "label": "Popup gallery limit"},
		},
		"gpstracks": []map[string]any{
			{"key": "default_nearby_distance_m", "type": "number", "label": "Nearby media distance"},
			{"key": "thumbnail_osm_background", "type": "boolean", "label": "OSM track thumbnail background"},
		},
		"transcoding": []map[string]any{
			{"key": "default_preset", "type": "text", "label": "Default preset"},
			{"key": "max_concurrent_sessions", "type": "number", "label": "Max concurrent sessions"},
		},
	}
	return append(common, specific[pluginID]...)
}

func (s *Server) handleDBExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "admin.db.export") {
		return
	}
	export, err := s.createDBExport(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, export)
}

func (s *Server) handleDBExports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	exports, err := s.listDBExports()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, exports)
}

func (s *Server) handleDBExportByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/db/exports/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "download" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	name := filepath.Base(parts[0])
	if name == "." || name == string(filepath.Separator) || strings.Contains(name, "..") {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid export id"))
		return
	}
	target := filepath.Join(s.exportDir(), name)
	if err := ensurePathInside(s.exportDir(), target); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	http.ServeFile(w, r, target)
}

func (s *Server) handleDBImportPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "admin.db.import.plan") {
		return
	}
	var req struct {
		Path               string `json:"path"`
		ConfirmationPhrase string `json:"confirmation_phrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.ConfirmationPhrase != "PLAN ONLY" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("confirmation_phrase must be PLAN ONLY"))
		return
	}
	target := filepath.Clean(req.Path)
	if !filepath.IsAbs(target) {
		target = filepath.Join(s.exportDir(), target)
	}
	if err := ensurePathInside(s.exportDir(), target); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":       target,
		"size_bytes": info.Size(),
		"plan":       "Validated export file only. Destructive restore is intentionally not implemented while the app is live.",
		"safe":       true,
	})
}

func (s *Server) createDBExport(r *http.Request) (map[string]any, error) {
	dir := s.exportDir()
	if err := ensurePathInside(s.deps.Config.Cache.Dir, dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	exportID := "metadata-" + id.NewUUID() + ".json"
	target := filepath.Join(dir, exportID)
	if err := ensurePathInside(dir, target); err != nil {
		return nil, err
	}
	stats, _ := s.deps.Store.Stats(r.Context())
	body := map[string]any{
		"exported_at":      time.Now().UTC(),
		"format":           "cartolensia-json-metadata-v1",
		"warning":          "This is a metadata/config export, not a destructive PostgreSQL restore script.",
		"store_backend":    s.deps.StoreBackend,
		"stats":            stats,
		"storages":         s.deps.Config.Storages,
		"plugins":          s.deps.Plugins,
		"runtime_settings": runtimeSettingsSnapshot(),
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return nil, err
	}
	return map[string]any{
		"id":           exportID,
		"path":         target,
		"size_bytes":   len(data),
		"download_url": "/api/v1/admin/db/exports/" + exportID + "/download",
		"created_at":   time.Now().UTC(),
	}, nil
}

func (s *Server) listDBExports() ([]map[string]any, error) {
	dir := s.exportDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id":           entry.Name(),
			"path":         filepath.Join(dir, entry.Name()),
			"size_bytes":   info.Size(),
			"created_at":   info.ModTime().UTC(),
			"download_url": "/api/v1/admin/db/exports/" + entry.Name() + "/download",
		})
	}
	return out, nil
}

func (s *Server) exportDir() string {
	return filepath.Join(s.deps.Config.Cache.Dir, "exports")
}
