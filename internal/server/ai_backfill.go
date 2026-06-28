package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
)

type aiBackfillRequest struct {
	Tasks              []string `json:"tasks"`
	LimitPerTask       int      `json:"limit_per_task"`
	BatchSize          int      `json:"batch_size"`
	MaxAudioSeconds    float64  `json:"max_audio_seconds"`
	MaxVideoSeconds    float64  `json:"max_video_seconds"`
	SafetyThreshold    *float64 `json:"safety_threshold"`
	Language           string   `json:"language"`
	Model              string   `json:"model"`
	SplitByTask        *bool    `json:"split_by_task"`
	ProductionBackfill bool     `json:"production_backfill"`
}

type aiBackfillJobPayload struct {
	Task               string   `json:"task"`
	Kind               string   `json:"kind"`
	MediaKind          string   `json:"media_kind"`
	Limit              int      `json:"limit"`
	BatchSize          int      `json:"batch_size"`
	MaxDurationSeconds float64  `json:"max_duration_seconds"`
	SafetyThreshold    *float64 `json:"safety_threshold,omitempty"`
	Language           string   `json:"language,omitempty"`
	Model              string   `json:"model,omitempty"`
	ProductionBackfill bool     `json:"production_backfill,omitempty"`
	Processed          int      `json:"processed,omitempty"`
	Stored             int      `json:"stored,omitempty"`
	Failed             int      `json:"failed,omitempty"`
	StartedAt          string   `json:"started_at,omitempty"`
	UpdatedAt          string   `json:"updated_at,omitempty"`
}

type aiBackfillTaskSpec struct {
	Task      string
	Kind      string
	MediaKind string
}

func aiBackfillTaskSpecs() []aiBackfillTaskSpec {
	return []aiBackfillTaskSpec{
		{Task: "classify", Kind: "classify_image", MediaKind: "photo"},
		{Task: "safety", Kind: "safety_nsfw", MediaKind: "photo"},
		{Task: "describe", Kind: "describe_image", MediaKind: "photo"},
		{Task: "embed", Kind: "embed_image", MediaKind: "photo"},
		{Task: "faces", Kind: "detect_faces", MediaKind: "photo"},
		{Task: "ocr", Kind: "ocr_image", MediaKind: "photo"},
		{Task: "audio_features", Kind: "analyze_audio", MediaKind: "audio"},
		{Task: "audio_transcript", Kind: "transcribe_audio", MediaKind: "audio"},
		{Task: "video_transcript", Kind: "transcribe_audio", MediaKind: "video"},
	}
}

func (s *Server) handleAIBackfillStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "ai.jobs.write") {
		return
	}
	var req aiBackfillRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	specs, err := selectAIBackfillSpecs(req.Tasks)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.LimitPerTask == 0 {
		req.LimitPerTask = 1000
	}
	if req.BatchSize <= 0 {
		req.BatchSize = 32
	}
	if req.MaxAudioSeconds <= 0 {
		req.MaxAudioSeconds = 45 * 60
	}
	if req.MaxVideoSeconds <= 0 {
		req.MaxVideoSeconds = 15 * 60
	}
	queued := make([]jobs.Job, 0, len(specs))
	now := time.Now().UTC().Format(time.RFC3339)
	for _, spec := range specs {
		payload := aiBackfillJobPayload{
			Task:               spec.Task,
			Kind:               spec.Kind,
			MediaKind:          spec.MediaKind,
			Limit:              req.LimitPerTask,
			BatchSize:          req.BatchSize,
			SafetyThreshold:    req.SafetyThreshold,
			Language:           req.Language,
			Model:              req.Model,
			ProductionBackfill: req.ProductionBackfill,
			StartedAt:          now,
		}
		switch spec.Task {
		case "audio_transcript":
			payload.MaxDurationSeconds = req.MaxAudioSeconds
		case "video_transcript":
			payload.MaxDurationSeconds = req.MaxVideoSeconds
		}
		job, err := s.deps.Store.EnqueueJob(r.Context(), jobs.New("ai_backfill", payload))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		queued = append(queued, job)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"jobs":       queued,
		"task_count": len(queued),
		"safe_note":  "AI backfill jobs are durable worker-owned jobs; originals are read through Cartolensia read-only media URLs and outputs are metadata only.",
	})
}

func selectAIBackfillSpecs(tasks []string) ([]aiBackfillTaskSpec, error) {
	all := aiBackfillTaskSpecs()
	if len(tasks) == 0 {
		return all, nil
	}
	byName := map[string]aiBackfillTaskSpec{}
	for _, spec := range all {
		byName[spec.Task] = spec
		byName[spec.Kind] = spec
	}
	out := make([]aiBackfillTaskSpec, 0, len(tasks))
	seen := map[string]struct{}{}
	for _, raw := range tasks {
		key := strings.ToLower(strings.TrimSpace(raw))
		spec, ok := byName[key]
		if !ok {
			return nil, fmt.Errorf("unsupported AI backfill task %q", raw)
		}
		if _, ok := seen[spec.Task]; ok {
			continue
		}
		seen[spec.Task] = struct{}{}
		out = append(out, spec)
	}
	return out, nil
}

func (s *Server) RunAIBackfillJob(ctx context.Context, job *jobs.Job, workerID string, leaseDuration time.Duration) error {
	payload, err := decodeAIBackfillPayload(job.Payload)
	if err != nil {
		return jobs.Permanent(err)
	}
	if payload.BatchSize <= 0 {
		payload.BatchSize = 32
	}
	if payload.BatchSize > 128 {
		payload.BatchSize = 128
	}
	if payload.Limit == 0 {
		payload.Limit = 1000
	}
	if payload.Kind == "" || payload.MediaKind == "" {
		specs, err := selectAIBackfillSpecs([]string{payload.Task})
		if err != nil {
			return jobs.Permanent(err)
		}
		payload.Kind = specs[0].Kind
		payload.MediaKind = specs[0].MediaKind
		payload.Task = specs[0].Task
	}
	endpoint, _, ok := aiConfiguredWorkerEndpoint(ctx)
	if !ok {
		jobs.AddLog(job, "error", "AI sidecar is not reachable at "+endpoint)
		return jobs.Transient(fmt.Errorf("AI sidecar is not reachable at %s", endpoint))
	}
	total, err := s.countAIMissing(ctx, payload)
	if err != nil {
		return err
	}
	if payload.Limit > 0 && total > payload.Limit {
		total = payload.Limit
	}
	job.ProgressTotal = int64Ptr(total)
	job.ProgressCurrent = 0
	job.Counters.Scanned = int64(total)
	jobs.AddLog(job, "info", fmt.Sprintf("AI backfill %s started: media=%s missing=%d batch=%d limit=%d", payload.Task, payload.MediaKind, total, payload.BatchSize, payload.Limit))
	if err := s.deps.Store.UpdateLeasedJob(ctx, *job, workerID); err != nil {
		return err
	}
	processed := 0
	stored := 0
	failed := 0
	failedIDs := map[string]struct{}{}
	for payload.Limit < 0 || processed+failed < payload.Limit {
		if err := s.cancelAIBackfillIfRequested(ctx, job, workerID); err != nil {
			return err
		}
		remaining := payload.BatchSize
		if payload.Limit > 0 {
			left := payload.Limit - processed - failed
			if left <= 0 {
				break
			}
			if left < remaining {
				remaining = left
			}
		}
		page, err := s.deps.Store.QueryAIMissingAssets(ctx, catalog.AIMissingQuery{
			Task:               payload.Kind,
			MediaKind:          payload.MediaKind,
			Limit:              remaining,
			Offset:             len(failedIDs),
			MaxDurationSeconds: payload.MaxDurationSeconds,
		})
		if err != nil {
			return err
		}
		if len(page.Assets) == 0 {
			break
		}
		ids := make([]string, 0, len(page.Assets))
		for _, asset := range page.Assets {
			if _, skip := failedIDs[asset.ID]; skip {
				continue
			}
			ids = append(ids, asset.ID)
		}
		if len(ids) == 0 {
			break
		}
		req := aiJobRequest{
			AssetIDs:        ids,
			Scope:           "selected",
			Limit:           len(ids),
			SafetyThreshold: payload.SafetyThreshold,
			Language:        payload.Language,
			Model:           payload.Model,
		}
		var latest aiJobResult
		result, err := s.runAIJobWithMediaURL(ctx, payload.Kind, req, func(asset catalog.Asset) string {
			return s.aiWorkerMediaURL(asset.ID)
		}, func(result aiJobResult) {
			latest = result
		})
		if err != nil {
			return err
		}
		if latest.Targets > 0 {
			result = latest
		}
		for _, msg := range result.Errors {
			if id := strings.TrimSpace(strings.SplitN(msg, ":", 2)[0]); id != "" {
				failedIDs[id] = struct{}{}
			}
		}
		processed += result.Processed + result.Skipped
		failed = len(failedIDs)
		stored += result.Stored
		payload.Processed = processed
		payload.Stored = stored
		payload.Failed = failed
		payload.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		job.Payload = payload
		job.ProgressCurrent = int64(processed + failed)
		job.Counters.Updated = int64(stored)
		job.Counters.Errors = int64(failed)
		if len(result.Errors) > 0 {
			jobs.AddLog(job, "warn", fmt.Sprintf("%s batch completed with %d errors; processed=%d stored=%d", payload.Task, len(result.Errors), result.Processed, result.Stored))
		} else {
			jobs.AddLog(job, "info", fmt.Sprintf("%s batch completed: processed=%d stored=%d", payload.Task, result.Processed, result.Stored))
		}
		if err := s.deps.Store.UpdateLeasedJob(ctx, *job, workerID); err != nil {
			return err
		}
		if result.Processed == 0 && result.Skipped == 0 && len(result.Errors) > 0 {
			break
		}
	}
	if err := s.cancelAIBackfillIfRequested(ctx, job, workerID); err != nil {
		return err
	}
	jobs.AddLog(job, "info", fmt.Sprintf("AI backfill %s finished: processed=%d stored=%d failed=%d", payload.Task, processed, stored, failed))
	job.Payload = payload
	job.ProgressCurrent = int64(processed + failed)
	job.Counters.Updated = int64(stored)
	job.Counters.Errors = int64(failed)
	return s.deps.Store.CompleteLeasedJob(ctx, *job, workerID)
}

func (s *Server) countAIMissing(ctx context.Context, payload aiBackfillJobPayload) (int, error) {
	page, err := s.deps.Store.QueryAIMissingAssets(ctx, catalog.AIMissingQuery{
		Task:               payload.Kind,
		MediaKind:          payload.MediaKind,
		Limit:              1,
		MaxDurationSeconds: payload.MaxDurationSeconds,
	})
	if err != nil {
		return 0, err
	}
	return page.Page.Total, nil
}

func (s *Server) cancelAIBackfillIfRequested(ctx context.Context, job *jobs.Job, workerID string) error {
	fresh, err := s.deps.Store.GetJob(ctx, job.ID)
	if err != nil {
		return err
	}
	if fresh.Status != jobs.StatusCancelRequested {
		return nil
	}
	jobs.AddLog(job, "info", "AI backfill cancel requested")
	if err := s.deps.Store.CancelLeasedJob(ctx, *job, workerID); err != nil {
		return err
	}
	return jobs.ErrCanceled
}

func decodeAIBackfillPayload(raw any) (aiBackfillJobPayload, error) {
	if raw == nil {
		return aiBackfillJobPayload{}, errors.New("AI backfill payload is required")
	}
	if payload, ok := raw.(aiBackfillJobPayload); ok {
		return payload, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return aiBackfillJobPayload{}, err
	}
	var payload aiBackfillJobPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return aiBackfillJobPayload{}, err
	}
	return payload, nil
}
