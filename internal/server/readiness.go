package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/config"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

type readinessCheck struct {
	ID       string         `json:"id"`
	Category string         `json:"category"`
	Label    string         `json:"label"`
	Status   string         `json:"status"`
	Summary  string         `json:"summary"`
	Details  map[string]any `json:"details,omitempty"`
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	checks := s.readinessChecks(r.Context())
	overall := "ok"
	counts := map[string]int{"ok": 0, "warn": 0, "error": 0}
	for _, check := range checks {
		counts[check.Status]++
		if check.Status == "error" {
			overall = "error"
		} else if check.Status == "warn" && overall == "ok" {
			overall = "warn"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       overall,
		"generated_at": time.Now().UTC(),
		"counts":       counts,
		"checks":       checks,
		"note":         "Readiness checks are bounded deployment probes; original media is not modified.",
	})
}

func (s *Server) readinessChecks(ctx context.Context) []readinessCheck {
	checks := []readinessCheck{}
	add := func(id, category, label, status, summary string, details map[string]any) {
		checks = append(checks, readinessCheck{ID: id, Category: category, Label: label, Status: status, Summary: summary, Details: details})
	}

	if strings.EqualFold(s.deps.Config.Auth.Mode, "local") {
		add("auth", "security", "Authentication", "ok", "local authentication is enabled", map[string]any{"mode": s.deps.Config.Auth.Mode})
	} else {
		add("auth", "security", "Authentication", "warn", "development authentication mode is active", map[string]any{"mode": s.deps.Config.Auth.Mode})
	}
	if strings.Contains(strings.ToLower(s.deps.StoreBackend), "postgres") {
		add("database", "database", "Durable database", "ok", "PostgreSQL-backed store is active", map[string]any{"backend": s.deps.StoreBackend})
	} else {
		add("database", "database", "Durable database", "warn", "production deployments should use PostgreSQL", map[string]any{"backend": s.deps.StoreBackend})
	}
	for _, check := range s.storageReadinessChecks() {
		checks = append(checks, check)
	}

	cacheStatus, cacheSummary := writableDirStatus(s.deps.Config.Cache.Dir)
	add("cache", "filesystem", "Cache directory", cacheStatus, cacheSummary, map[string]any{"path": s.deps.Config.Cache.Dir})
	componentStatus, componentSummary := writableDirStatus(componentRoot())
	add("components", "filesystem", "Component directory", componentStatus, componentSummary, map[string]any{"path": componentRoot()})
	modelDir := strings.TrimSpace(os.Getenv("CARTOLENSIA_AI_MODEL_DIR"))
	if modelDir == "" {
		modelDir = filepath.Join(".cartolensia", "models")
	}
	modelStatus, modelSummary := writableDirStatus(modelDir)
	add("models", "filesystem", "Model directory", modelStatus, modelSummary, map[string]any{"path": modelDir})

	ffmpegStatus, ffmpegSummary, ffmpegDetails := executableStatus("ffmpeg")
	add("ffmpeg", "tools", "FFmpeg", ffmpegStatus, ffmpegSummary, ffmpegDetails)
	ffprobeStatus, ffprobeSummary, ffprobeDetails := executableStatus("ffprobe")
	add("ffprobe", "tools", "FFprobe", ffprobeStatus, ffprobeSummary, ffprobeDetails)
	tesseractStatus, tesseractSummary, tesseractDetails := executableStatus("tesseract")
	add("tesseract", "tools", "Tesseract OCR", tesseractStatus, tesseractSummary, tesseractDetails)

	endpoint := aiWorkerEndpoint()
	aiStatus, aiHealth := aiWorkerHealth(ctx, endpoint+"/health")
	if aiStatus == "ok" {
		add("ai.worker", "ai", "AI worker", "ok", "AI sidecar is reachable", map[string]any{"endpoint": endpoint, "health": aiHealth})
	} else {
		add("ai.worker", "ai", "AI worker", "warn", "AI sidecar is not reachable; AI/OCR/ASR actions remain optional", map[string]any{"endpoint": endpoint, "health": aiHealth})
	}

	components, err := s.deps.Store.ListComponents(ctx, catalog.ComponentQuery{Limit: 500})
	if err != nil {
		add("component.registry", "components", "Component registry", "warn", "component registry could not be queried: "+err.Error(), map[string]any{"error": err.Error()})
	} else {
		counts := map[string]int{}
		for _, component := range components {
			counts[component.Status]++
		}
		status := "ok"
		summary := fmt.Sprintf("%d components tracked", len(components))
		if counts["failed"] > 0 {
			status = "warn"
			summary = fmt.Sprintf("%d failed component checks need attention", counts["failed"])
		}
		add("component.registry", "components", "Component registry", status, summary, map[string]any{"counts": counts, "total": len(components)})
	}

	if s.deps.Config.HTTP.TLSCertFile != "" || s.deps.Config.HTTP.TLSAutoSelfSigned {
		add("http.tls", "network", "HTTP/TLS", "ok", "TLS is configured", map[string]any{"addr": s.deps.Config.HTTP.Addr, "auto_self_signed": s.deps.Config.HTTP.TLSAutoSelfSigned})
	} else {
		add("http.tls", "network", "HTTP/TLS", "warn", "plain HTTP is active; use localhost, a trusted reverse proxy, or configured TLS for production", map[string]any{"addr": s.deps.Config.HTTP.Addr})
	}
	return checks
}

func (s *Server) storageReadinessChecks() []readinessCheck {
	storages := s.deps.Config.Storages
	results := make([]readinessCheck, len(storages))
	var wg sync.WaitGroup
	for i, st := range storages {
		i, st := i, st
		wg.Add(1)
		go func() {
			defer wg.Done()
			diag := diagnoseStorageRoot(storage.Config{
				Name:      st.Name,
				Kind:      st.Kind,
				Root:      st.Root,
				Mode:      st.Mode,
				SourceURL: st.SourceURL,
				SMB:       storageSMBConfigFromRuntime(st.SMB),
			}, 750*time.Millisecond)
			status := "ok"
			summary := diag.Message
			details := diag.Details
			if st.Mode != "strict_read_only" {
				status = "warn"
				summary = "storage is not strict_read_only"
			}
			if diag.Code != "available" {
				status = "warn"
			}
			results[i] = readinessCheck{
				ID:       "storage." + st.Name,
				Category: "storage",
				Label:    "Storage " + st.Name,
				Status:   status,
				Summary:  summary,
				Details:  details,
			}
		}()
	}
	wg.Wait()
	return results
}

func storageSMBConfigFromRuntime(in *config.SMBStorageConfig) *storage.SMBConfig {
	if in == nil {
		return nil
	}
	return &storage.SMBConfig{
		Host:            in.Host,
		Share:           in.Share,
		Path:            in.Path,
		Domain:          in.Domain,
		Username:        in.Username,
		CredentialsFile: in.CredentialsFile,
		PasswordEnv:     in.PasswordEnv,
	}
}

func probeReadableDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return file.Close()
}

func probeReadableDirBounded(path string, timeout time.Duration) error {
	type result struct {
		err error
	}
	done := make(chan result, 1)
	go func() {
		done <- result{err: probeReadableDir(path)}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case res := <-done:
		return res.err
	case <-timer.C:
		return fmt.Errorf("storage probe timed out after %s", timeout)
	}
}

func writableDirStatus(path string) (string, string) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "error", "directory cannot be created: " + err.Error()
	}
	target := filepath.Join(path, ".cartolensia-readiness.tmp")
	if err := os.WriteFile(target, []byte("ok\n"), 0o600); err != nil {
		return "error", "directory is not writable: " + err.Error()
	}
	_ = os.Remove(target)
	return "ok", "directory is writable"
}

func executableStatus(name string) (string, string, map[string]any) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "warn", name + " is not on PATH", map[string]any{"available": false}
	}
	return "ok", name + " is available", map[string]any{"available": true, "path": path}
}
