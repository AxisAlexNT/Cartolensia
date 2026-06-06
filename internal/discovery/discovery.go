package discovery

import (
	"context"
	"fmt"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/media"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

type Runner struct {
	Registry *storage.Registry
	Store    catalog.Store
}

func (r Runner) Scan(ctx context.Context, job *jobs.Job) error {
	if r.Registry == nil || r.Store == nil {
		return fmt.Errorf("discovery runner is not configured")
	}
	if err := jobs.Start(job); err != nil {
		return err
	}
	jobs.AddLog(job, "info", "discovery scan started")
	if err := r.Store.UpdateJob(ctx, *job); err != nil {
		return err
	}
	for _, storageConfig := range r.Registry.ListStorages() {
		adapter, err := r.Registry.Adapter(storageConfig.Name)
		if err != nil {
			return err
		}
		files, err := adapter.ListRecursive(ctx)
		if err != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "error", err.Error())
			_ = jobs.Fail(job, err)
			_ = r.Store.UpdateJob(ctx, *job)
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
			if err := r.Store.UpdateJob(ctx, *job); err != nil {
				return err
			}
		}
	}
	if err := jobs.Complete(job); err != nil {
		return err
	}
	jobs.AddLog(job, "info", "discovery scan completed")
	return r.Store.UpdateJob(ctx, *job)
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
		_ = jobs.Fail(job, err)
		_ = r.Store.UpdateJob(ctx, *job)
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
	if err := r.Store.UpdateJob(ctx, *job); err != nil {
		return err
	}
	for _, asset := range targets {
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
		if err := r.Store.UpdateJob(ctx, *job); err != nil {
			return err
		}
	}
	if err := jobs.Complete(job); err != nil {
		return err
	}
	jobs.AddLog(job, "info", "hash job completed")
	return r.Store.UpdateJob(ctx, *job)
}
