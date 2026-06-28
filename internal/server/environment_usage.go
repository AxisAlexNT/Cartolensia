package server

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/database"
)

type databaseUsageProvider interface {
	DatabaseUsage(context.Context) (database.Usage, error)
}

type directoryUsage struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Path      string   `json:"path"`
	Status    string   `json:"status"`
	Bytes     int64    `json:"bytes"`
	Files     int64    `json:"files"`
	Dirs      int64    `json:"dirs"`
	Errors    []string `json:"errors,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
}

func (s *Server) handleEnvironmentUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if r.URL.Query().Get("refresh") != "1" {
		s.environmentUsageMu.Lock()
		if s.environmentUsageCache != nil && time.Since(s.environmentUsageCachedAt) < 5*time.Minute {
			cached := s.environmentUsageCache
			s.environmentUsageMu.Unlock()
			writeJSON(w, http.StatusOK, cached)
			return
		}
		s.environmentUsageMu.Unlock()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	usage := map[string]any{
		"generated_at": time.Now().UTC(),
		"scope":        "Cartolensia-owned database/cache/component/model/export directories only; original storages are intentionally not scanned.",
		"directories":  s.environmentDirectoryUsage(ctx),
	}
	if provider, ok := s.deps.Store.(databaseUsageProvider); ok {
		dbUsage, err := provider.DatabaseUsage(ctx)
		if err != nil {
			usage["database_error"] = err.Error()
		} else {
			usage["database"] = dbUsage
		}
	} else {
		usage["database"] = map[string]any{"backend": s.deps.StoreBackend, "available": false}
	}
	s.environmentUsageMu.Lock()
	s.environmentUsageCache = usage
	s.environmentUsageCachedAt = time.Now()
	s.environmentUsageMu.Unlock()
	writeJSON(w, http.StatusOK, usage)
}

func (s *Server) environmentDirectoryUsage(ctx context.Context) []directoryUsage {
	roots := []directoryUsage{
		{Key: "cache", Label: "Cache", Path: s.deps.Config.Cache.Dir},
		{Key: "exports", Label: "Exports", Path: s.exportDir()},
		{Key: "components", Label: "Components", Path: componentRoot()},
		{Key: "models", Label: "AI models", Path: modelRoot()},
		{Key: "ai_venv", Label: "AI Python environment", Path: firstExistingPath(os.Getenv("CARTOLENSIA_AI_VENV"), filepath.Join(filepath.Dir(s.deps.Config.Cache.Dir), "ai-venv"), filepath.Join(".cartolensia", "ai-venv"))},
	}
	for i := range roots {
		roots[i] = collectDirectoryUsage(ctx, roots[i])
	}
	return roots
}

func collectDirectoryUsage(ctx context.Context, usage directoryUsage) directoryUsage {
	path := strings.TrimSpace(usage.Path)
	if path == "" {
		usage.Status = "not_configured"
		return usage
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		usage.Path = abs
	}
	info, err := os.Stat(usage.Path)
	if err != nil {
		usage.Status = "missing"
		usage.Errors = []string{err.Error()}
		return usage
	}
	if !info.IsDir() {
		usage.Status = "not_directory"
		usage.Bytes = info.Size()
		return usage
	}
	const maxEntries = 250000
	usage.Status = "ok"
	walkErr := filepath.WalkDir(usage.Path, func(path string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			usage.Truncated = true
			return ctx.Err()
		}
		if err != nil {
			if len(usage.Errors) < 8 {
				usage.Errors = append(usage.Errors, err.Error())
			}
			return nil
		}
		if usage.Files+usage.Dirs >= maxEntries {
			usage.Truncated = true
			return fs.SkipAll
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			usage.Dirs++
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			if len(usage.Errors) < 8 {
				usage.Errors = append(usage.Errors, err.Error())
			}
			return nil
		}
		usage.Files++
		usage.Bytes += info.Size()
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.SkipAll) && !errors.Is(walkErr, context.Canceled) && !errors.Is(walkErr, context.DeadlineExceeded) {
		usage.Status = "partial"
		if len(usage.Errors) < 8 {
			usage.Errors = append(usage.Errors, walkErr.Error())
		}
	}
	if ctx.Err() != nil {
		usage.Status = "partial"
	}
	return usage
}

func modelRoot() string {
	configured := strings.TrimSpace(os.Getenv("CARTOLENSIA_AI_MODEL_DIR"))
	if configured == "" {
		configured = strings.TrimSpace(os.Getenv("CARTOLENSIA_MODEL_DIR"))
	}
	if configured == "" {
		configured = filepath.Join(".cartolensia", "models")
	}
	root, err := filepath.Abs(configured)
	if err != nil {
		return configured
	}
	return root
}

func firstExistingPath(paths ...string) string {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			return path
		}
	}
	return ""
}
