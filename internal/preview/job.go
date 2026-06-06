package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

type GeneratePayload struct {
	AssetIDs    []string `json:"asset_ids,omitempty"`
	MediaKind   string   `json:"media_kind,omitempty"`
	OnlyMissing bool     `json:"only_missing,omitempty"`
	MaxFiles    int      `json:"max_files,omitempty"`
	Variant     string   `json:"variant,omitempty"`
}

type Runner struct {
	Registry      *storage.Registry
	Store         catalog.Store
	CacheDir      string
	WorkerID      string
	LeaseDuration time.Duration
}

func DecodeGeneratePayload(raw any) GeneratePayload {
	var payload GeneratePayload
	if raw == nil {
		return payload
	}
	data, err := json.Marshal(raw)
	if err == nil {
		_ = json.Unmarshal(data, &payload)
	}
	return payload
}

func (r Runner) Generate(ctx context.Context, job *jobs.Job) error {
	if r.Registry == nil || r.Store == nil || r.CacheDir == "" {
		return jobs.Permanent(fmt.Errorf("preview runner is not configured"))
	}
	payload := DecodeGeneratePayload(job.Payload)
	if err := jobs.Start(job); err != nil {
		return err
	}
	jobs.AddLog(job, "info", "preview generation started")
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	assets, err := r.Store.ListAssets(ctx)
	if err != nil {
		_ = r.failJob(ctx, *job, err)
		return err
	}
	targets := selectPreviewTargets(assets, payload, r.CacheDir)
	total := int64(len(targets))
	job.ProgressTotal = &total
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	for _, asset := range targets {
		if err := r.checkCanceled(ctx, job); err != nil {
			return err
		}
		loc, _ := catalog.FirstLocation(asset)
		file, _, err := r.Registry.OpenByURL(loc.StorageURL)
		if err != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "warn", fmt.Sprintf("%s: preview skipped: %v", asset.DisplayName, err))
			continue
		}
		info, genErr := GenerateImage(ctx, r.CacheDir, asset, file)
		closeErr := file.Close()
		if genErr != nil || closeErr != nil {
			job.Counters.Errors++
			if genErr == nil {
				genErr = closeErr
			}
			if info.Status == "" {
				info.Status = StatusFailed
				info.Message = genErr.Error()
			}
			jobs.AddLog(job, "warn", fmt.Sprintf("%s: preview failed: %v", asset.DisplayName, genErr))
		} else if info.Status == StatusReady {
			job.Counters.Created++
		} else {
			job.Counters.Updated++
			jobs.AddLog(job, "info", fmt.Sprintf("%s: preview %s", asset.DisplayName, info.Status))
		}
		if _, err := r.Store.UpsertPreviewCacheEntry(ctx, IndexEntry(asset, info, payload.Variant)); err != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "warn", fmt.Sprintf("%s: preview cache index update failed: %v", asset.DisplayName, err))
		}
		job.ProgressCurrent++
		if err := r.updateJob(ctx, *job); err != nil {
			return err
		}
	}
	jobs.AddLog(job, "info", "preview generation completed")
	return r.completeJob(ctx, *job)
}

func selectPreviewTargets(assets []catalog.Asset, payload GeneratePayload, cacheDir string) []catalog.Asset {
	ids := map[string]struct{}{}
	for _, id := range payload.AssetIDs {
		ids[id] = struct{}{}
	}
	if payload.MediaKind == "" {
		payload.MediaKind = "photo"
	}
	var out []catalog.Asset
	for _, asset := range assets {
		loc, ok := catalog.FirstLocation(asset)
		if !ok || loc.MediaKind != "photo" {
			continue
		}
		if len(ids) > 0 {
			if _, ok := ids[asset.ID]; !ok {
				continue
			}
		}
		if payload.MediaKind != "" && asset.MediaKind != payload.MediaKind {
			continue
		}
		if payload.OnlyMissing && InfoForAsset(cacheDir, asset).Status == StatusReady {
			continue
		}
		out = append(out, asset)
		if payload.MaxFiles > 0 && len(out) >= payload.MaxFiles {
			break
		}
	}
	return out
}

func (r Runner) updateJob(ctx context.Context, job jobs.Job) error {
	if r.WorkerID != "" {
		return r.Store.UpdateLeasedJob(ctx, job, r.WorkerID)
	}
	return r.Store.UpdateJob(ctx, job)
}

func (r Runner) completeJob(ctx context.Context, job jobs.Job) error {
	if r.WorkerID != "" {
		return r.Store.CompleteLeasedJob(ctx, job, r.WorkerID)
	}
	if err := jobs.Complete(&job); err != nil {
		return err
	}
	return r.Store.UpdateJob(ctx, job)
}

func (r Runner) failJob(ctx context.Context, job jobs.Job, cause error) error {
	if r.WorkerID != "" {
		return r.Store.FailLeasedJob(ctx, job, r.WorkerID, cause)
	}
	if err := jobs.Fail(&job, cause); err != nil {
		return err
	}
	return r.Store.UpdateJob(ctx, job)
}

func (r Runner) checkCanceled(ctx context.Context, job *jobs.Job) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	latest, err := r.Store.GetJob(ctx, job.ID)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			return nil
		}
		return err
	}
	if latest.Status == jobs.StatusCancelRequested || latest.Status == jobs.StatusCanceled || latest.CancelRequestedAt != nil {
		jobs.AddLog(job, "info", "job canceled")
		if r.WorkerID != "" {
			if err := r.Store.CancelLeasedJob(ctx, *job, r.WorkerID); err != nil {
				return err
			}
			return jobs.ErrCanceled
		}
		if err := jobs.Cancel(job); err != nil {
			return err
		}
		if err := r.Store.UpdateJob(ctx, *job); err != nil {
			return err
		}
		return jobs.ErrCanceled
	}
	return nil
}
