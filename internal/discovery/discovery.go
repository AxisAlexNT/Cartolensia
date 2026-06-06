package discovery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
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
		return fmt.Errorf("discovery runner is not configured")
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
			return err
		}
		files, err := adapter.ListRecursive(ctx)
		if err != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "error", err.Error())
			_ = r.failJob(ctx, *job, err)
			return err
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
		return fmt.Errorf("hash runner is not configured")
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
