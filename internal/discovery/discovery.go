package discovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/gpx"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/media"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

type Runner struct {
	Registry      *storage.Registry
	Store         catalog.Store
	WorkerID      string
	LeaseDuration time.Duration
}

func (r Runner) Scan(ctx context.Context, job *jobs.Job) error {
	if r.Registry == nil || r.Store == nil {
		return jobs.Permanent(fmt.Errorf("discovery runner is not configured"))
	}
	if err := jobs.Start(job); err != nil {
		return err
	}
	jobs.AddLog(job, "info", "discovery scan started")
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	for _, storageConfig := range r.Registry.ListStorages() {
		if err := r.checkCanceled(ctx, job); err != nil {
			return err
		}
		adapter, err := r.Registry.Adapter(storageConfig.Name)
		if err != nil {
			return jobs.Permanent(err)
		}
		files, err := adapter.ListRecursive(ctx)
		if err != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "error", err.Error())
			cause := classifyStorageFailure(err)
			_ = r.failJob(ctx, *job, cause)
			return cause
		}
		total := int64(0)
		for _, file := range files {
			if file.MediaKind != "other" {
				total++
			}
		}
		job.ProgressTotal = &total
		jobs.AddLog(job, "info", fmt.Sprintf("scanning storage %s: %d indexable files", storageConfig.Name, total))
		for _, file := range files {
			if err := r.checkCanceled(ctx, job); err != nil {
				return err
			}
			if file.MediaKind == "other" {
				continue
			}
			result, err := r.Store.UpsertDiscoveredFile(ctx, file)
			if err != nil {
				job.Counters.Errors++
				jobs.AddLog(job, "error", fmt.Sprintf("%s: %v", file.StorageURL, err))
				continue
			}
			job.Counters.Scanned++
			job.Counters.Bytes += file.SizeBytes
			if result.Created {
				job.Counters.Created++
			} else {
				job.Counters.Updated++
			}
			if err := r.enrichDiscoveredFile(ctx, result.Asset.ID, file); err != nil {
				job.Counters.Errors++
				jobs.AddLog(job, "warn", fmt.Sprintf("%s: metadata enrichment skipped: %v", file.StorageURL, err))
			}
			job.ProgressCurrent++
			if err := r.updateJob(ctx, *job); err != nil {
				return err
			}
		}
	}
	jobs.AddLog(job, "info", "discovery scan completed")
	return r.completeJob(ctx, *job)
}

func (r Runner) HashUnhashed(ctx context.Context, job *jobs.Job) error {
	if r.Registry == nil || r.Store == nil {
		return jobs.Permanent(fmt.Errorf("hash runner is not configured"))
	}
	if err := jobs.Start(job); err != nil {
		return err
	}
	jobs.AddLog(job, "info", "hash job started")
	assets, err := r.Store.ListAssets(ctx)
	if err != nil {
		_ = r.failJob(ctx, *job, err)
		return err
	}
	var targets []catalog.Asset
	for _, asset := range assets {
		loc, ok := catalog.FirstLocation(asset)
		if ok && loc.HashStatus != catalog.HashStatusHashed {
			targets = append(targets, asset)
		}
	}
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
			jobs.AddLog(job, "error", fmt.Sprintf("%s: %v", loc.StorageURL, err))
			if isStorageSafetyError(err) {
				cause := jobs.Permanent(err)
				_ = r.failJob(ctx, *job, cause)
				return cause
			}
			continue
		}
		result, hashErr := media.HashReader(file)
		closeErr := file.Close()
		if hashErr != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "error", fmt.Sprintf("%s: %v", loc.StorageURL, hashErr))
			continue
		}
		if closeErr != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "error", fmt.Sprintf("%s: %v", loc.StorageURL, closeErr))
			continue
		}
		if err := r.Store.UpdateLocationHash(ctx, asset.ID, result.SHA512Hex, result.Bytes); err != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "error", fmt.Sprintf("%s: %v", loc.StorageURL, err))
			continue
		}
		job.Counters.Hashed++
		job.Counters.Bytes += result.Bytes
		job.ProgressCurrent++
		if err := r.updateJob(ctx, *job); err != nil {
			return err
		}
	}
	jobs.AddLog(job, "info", "hash job completed")
	return r.completeJob(ctx, *job)
}

func classifyStorageFailure(err error) error {
	if isStorageSafetyError(err) {
		return jobs.Permanent(err)
	}
	return err
}

func isStorageSafetyError(err error) bool {
	return errors.Is(err, storage.ErrTraversal) || errors.Is(err, storage.ErrReadOnly) || errors.Is(err, storage.ErrUnknownStorage)
}

func (r Runner) enrichDiscoveredFile(ctx context.Context, assetID string, info storage.FileInfo) error {
	if err := r.applyTextHints(ctx, assetID, info); err != nil {
		return err
	}
	if info.MediaKind == "track" && strings.EqualFold(info.Extension, "gpx") {
		file, _, err := r.Registry.OpenByURL(info.StorageURL)
		if err != nil {
			return err
		}
		defer file.Close()
		points, err := gpx.Parse(file)
		if err != nil {
			return err
		}
		if len(points) == 0 {
			return nil
		}
		if err := r.Store.UpsertTrackPoints(ctx, assetID, points); err != nil {
			return err
		}
		metadata := map[string]any{
			"track_point_count": len(points),
			"track_start_at":    points[0].RecordedAt.Format(time.RFC3339),
			"track_end_at":      points[len(points)-1].RecordedAt.Format(time.RFC3339),
		}
		start := points[0].RecordedAt
		return r.Store.UpdateAssetMetadata(ctx, assetID, &start, metadata)
	}
	if info.MediaKind == "video" {
		r.applyFFProbe(ctx, assetID, info)
	}
	return nil
}

func (r Runner) applyTextHints(ctx context.Context, assetID string, info storage.FileInfo) error {
	file, _, err := r.Registry.OpenByURL(info.StorageURL)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 8192))
	if err != nil {
		return err
	}
	takenAt, metadata := parseFixtureHints(string(data))
	if takenAt == nil && len(metadata) == 0 {
		return nil
	}
	return r.Store.UpdateAssetMetadata(ctx, assetID, takenAt, metadata)
}

func parseFixtureHints(text string) (*time.Time, map[string]any) {
	metadata := map[string]any{}
	var takenAt *time.Time
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "taken_at_hint":
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				t := parsed.UTC()
				takenAt = &t
			}
		case "duration_hint_seconds":
			if parsed, err := strconv.ParseFloat(value, 64); err == nil {
				metadata["duration_seconds"] = parsed
			}
		}
	}
	return takenAt, metadata
}

func (r Runner) applyFFProbe(ctx context.Context, assetID string, info storage.FileInfo) {
	file, _, err := r.Registry.OpenByURL(info.StorageURL)
	if err != nil {
		return
	}
	path := file.Name()
	_ = file.Close()
	probe, err := media.ProbeVideo(ctx, path)
	if err != nil {
		metadata := map[string]any{"ffprobe_available": probe.Available}
		_ = r.Store.UpdateAssetMetadata(ctx, assetID, nil, metadata)
		return
	}
	metadata := map[string]any{"ffprobe_available": probe.Available}
	if probe.DurationSeconds != nil {
		metadata["duration_seconds"] = *probe.DurationSeconds
	}
	if probe.Width != nil {
		metadata["width"] = *probe.Width
	}
	if probe.Height != nil {
		metadata["height"] = *probe.Height
	}
	_ = r.Store.UpdateAssetMetadata(ctx, assetID, nil, metadata)
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

func (r Runner) cancelJob(ctx context.Context, job *jobs.Job) error {
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
		job.Status = latest.Status
		job.CancelRequestedAt = latest.CancelRequestedAt
		return r.cancelJob(ctx, job)
	}
	return nil
}
