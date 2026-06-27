package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/gpx"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/media"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

type Runner struct {
	Registry         *storage.Registry
	Store            catalog.Store
	WorkerID         string
	LeaseDuration    time.Duration
	MaxFolderWorkers int
	MaxFileWorkers   int
	FolderQueueDepth int
}

type ScanPayload struct {
	Storage           string   `json:"storage,omitempty"`
	Prefix            string   `json:"prefix,omitempty"`
	Prefixes          []string `json:"prefixes,omitempty"`
	MaxFiles          int      `json:"max_files,omitempty"`
	MaxBytes          int64    `json:"max_bytes,omitempty"`
	IncludeExtensions []string `json:"include_extensions,omitempty"`
	ExcludePatterns   []string `json:"exclude_patterns,omitempty"`
	MarkMissing       bool     `json:"mark_missing,omitempty"`
	Hash              bool     `json:"hash,omitempty"`
	Metadata          bool     `json:"metadata,omitempty"`
	Previews          bool     `json:"previews,omitempty"`
}

type HashPayload struct {
	Scope    string   `json:"scope,omitempty"`
	AssetID  string   `json:"asset_id,omitempty"`
	AssetIDs []string `json:"asset_ids,omitempty"`
	Storage  string   `json:"storage,omitempty"`
	Prefix   string   `json:"prefix,omitempty"`
	Prefixes []string `json:"prefixes,omitempty"`
	AlbumID  string   `json:"album_id,omitempty"`
	MaxFiles int      `json:"max_files,omitempty"`
}

func DecodeScanPayload(raw any) ScanPayload {
	var payload ScanPayload
	if raw == nil {
		return payload
	}
	data, err := json.Marshal(raw)
	if err == nil {
		_ = json.Unmarshal(data, &payload)
	}
	payload.Storage = strings.TrimSpace(payload.Storage)
	payload.Prefix = strings.TrimSpace(payload.Prefix)
	payload.Prefixes = compactPayloadStrings(payload.Prefixes)
	if payload.Prefix != "" {
		payload.Prefixes = append([]string{payload.Prefix}, payload.Prefixes...)
	}
	payload.IncludeExtensions = normalizeExtensions(payload.IncludeExtensions)
	payload.ExcludePatterns = compactPayloadStrings(payload.ExcludePatterns)
	return payload
}

func DecodeHashPayload(raw any) HashPayload {
	var payload HashPayload
	if raw == nil {
		return payload
	}
	data, err := json.Marshal(raw)
	if err == nil {
		_ = json.Unmarshal(data, &payload)
	}
	payload.Scope = strings.TrimSpace(payload.Scope)
	payload.Storage = strings.TrimSpace(payload.Storage)
	payload.Prefix = strings.TrimSpace(payload.Prefix)
	payload.Prefixes = compactPayloadStrings(payload.Prefixes)
	if payload.Prefix != "" {
		payload.Prefixes = append([]string{payload.Prefix}, payload.Prefixes...)
	}
	payload.AssetID = strings.TrimSpace(payload.AssetID)
	payload.AssetIDs = compactPayloadStrings(payload.AssetIDs)
	if payload.AssetID != "" {
		payload.AssetIDs = append([]string{payload.AssetID}, payload.AssetIDs...)
	}
	payload.AlbumID = strings.TrimSpace(payload.AlbumID)
	return payload
}

func (p ScanPayload) bounded() bool {
	return p.Storage != "" || len(p.Prefixes) > 0 || p.MaxFiles > 0 || p.MaxBytes > 0 ||
		len(p.IncludeExtensions) > 0 || len(p.ExcludePatterns) > 0
}

func ValidateScanSafety(registry *storage.Registry, payload ScanPayload) error {
	if registry == nil {
		return fmt.Errorf("storage registry is not configured")
	}
	storages := registry.ListStorages()
	if strings.EqualFold(payload.Storage, "all") {
		if containsRealArchiveStorage(storages) {
			return fmt.Errorf("storage=all is refused for real archive storage; choose one storage and a bounded adapter-relative prefix")
		}
		return nil
	}
	if payload.Storage != "" {
		var selected []storage.Config
		for _, storageConfig := range storages {
			if storageConfig.Name == payload.Storage {
				selected = append(selected, storageConfig)
				break
			}
		}
		if len(selected) == 0 {
			return fmt.Errorf("unknown storage %q", payload.Storage)
		}
		storages = selected
	}
	if !containsRealArchiveStorage(storages) {
		return nil
	}
	if payload.Storage == "" {
		return fmt.Errorf("real archive discovery requires an explicit storage name")
	}
	if err := validateRealArchiveScanPayload(payload); err != nil {
		return err
	}
	for _, storageConfig := range storages {
		if !isRealArchiveRoot(storageConfig.Root) {
			continue
		}
		adapter, err := registry.Adapter(storageConfig.Name)
		if err != nil {
			return err
		}
		if _, err := normalizeScanPrefixes(adapter, payload.Prefixes); err != nil {
			return err
		}
	}
	return nil
}

func ValidateHashSafety(registry *storage.Registry, payload HashPayload) error {
	if registry == nil || !containsRealArchiveStorage(registry.ListStorages()) {
		return nil
	}
	if len(payload.AssetIDs) > 0 {
		return nil
	}
	if payload.Storage == "" || strings.EqualFold(payload.Storage, "all") {
		return fmt.Errorf("hashing real archive assets requires explicit selected assets or storage plus prefix")
	}
	if len(payload.Prefixes) == 0 {
		return fmt.Errorf("hashing real archive assets requires an adapter-relative prefix")
	}
	if payload.MaxFiles == 0 {
		return fmt.Errorf("hashing real archive assets requires max_files")
	}
	adapter, err := registry.Adapter(payload.Storage)
	if err != nil {
		return err
	}
	if _, err := normalizeScanPrefixes(adapter, payload.Prefixes); err != nil {
		return err
	}
	return nil
}

func (r Runner) Scan(ctx context.Context, job *jobs.Job) error {
	if r.Registry == nil || r.Store == nil {
		return jobs.Permanent(fmt.Errorf("discovery runner is not configured"))
	}
	payload := DecodeScanPayload(job.Payload)
	if payload.MarkMissing {
		return jobs.Permanent(fmt.Errorf("discovery mark_missing is not implemented and is not safe for bounded scans"))
	}
	if err := jobs.Start(job); err != nil {
		return err
	}
	jobs.AddLog(job, "info", "discovery scan started")
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	storages := r.Registry.ListStorages()
	if strings.EqualFold(payload.Storage, "all") {
		if containsRealArchiveStorage(storages) {
			cause := jobs.Permanent(fmt.Errorf("storage=all is refused for real archive storage; choose one storage and a bounded adapter-relative prefix"))
			_ = r.failJob(ctx, *job, cause)
			return cause
		}
		payload.Storage = ""
	}
	if payload.Storage != "" {
		var selected []storage.Config
		for _, storageConfig := range storages {
			if storageConfig.Name == payload.Storage {
				selected = append(selected, storageConfig)
				break
			}
		}
		if len(selected) == 0 {
			cause := jobs.Permanent(fmt.Errorf("unknown storage %q", payload.Storage))
			_ = r.failJob(ctx, *job, cause)
			return cause
		}
		storages = selected
	}
	if !payload.bounded() && containsRealArchiveStorage(storages) {
		cause := jobs.Permanent(fmt.Errorf("unbounded discovery is refused for real archive storage; provide storage, adapter-relative prefixes, max_files, and max_bytes"))
		_ = r.failJob(ctx, *job, cause)
		return cause
	}
	if containsRealArchiveStorage(storages) && payload.Storage == "" {
		cause := jobs.Permanent(fmt.Errorf("real archive discovery requires an explicit storage name"))
		_ = r.failJob(ctx, *job, cause)
		return cause
	}
	for _, storageConfig := range storages {
		if err := r.checkCanceled(ctx, job); err != nil {
			return err
		}
		adapter, err := r.Registry.Adapter(storageConfig.Name)
		if err != nil {
			return jobs.Permanent(err)
		}
		var files []storage.FileInfo
		if payload.bounded() {
			if isRealArchiveRoot(storageConfig.Root) {
				if err := validateRealArchiveScanPayload(payload); err != nil {
					cause := jobs.Permanent(err)
					_ = r.failJob(ctx, *job, cause)
					return cause
				}
			}
			prefixes, err := normalizeScanPrefixes(adapter, payload.Prefixes)
			if err != nil {
				cause := jobs.Permanent(err)
				_ = r.failJob(ctx, *job, cause)
				return cause
			}
			if len(prefixes) == 0 {
				cause := jobs.Permanent(fmt.Errorf("bounded discovery prefixes are required"))
				_ = r.failJob(ctx, *job, cause)
				return cause
			}
			if err := r.scanBounded(ctx, job, adapter, storageConfig.Name, prefixes, payload); err != nil {
				return err
			}
			continue
		} else {
			if isRealArchiveRoot(storageConfig.Root) {
				cause := jobs.Permanent(fmt.Errorf("unbounded discovery is refused for real archive storage"))
				_ = r.failJob(ctx, *job, cause)
				return cause
			}
			files, err = adapter.ListRecursive(ctx)
			if err != nil {
				job.Counters.Errors++
				jobs.AddLog(job, "error", err.Error())
				cause := classifyStorageFailure(err)
				_ = r.failJob(ctx, *job, cause)
				return cause
			}
		}
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
			if err := r.applyTextHints(ctx, result.Asset.ID, file); err != nil {
				job.Counters.Errors++
				jobs.AddLog(job, "warn", fmt.Sprintf("%s: fixture hints skipped: %v", file.StorageURL, err))
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

func (r Runner) scanBounded(ctx context.Context, job *jobs.Job, adapter *storage.FSAdapter, storageName string, prefixes []string, payload ScanPayload) error {
	job.ProgressTotal = nil
	jobs.AddLog(job, "info", fmt.Sprintf("streaming bounded scan storage %s prefixes %v", storageName, prefixes))
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	var lastUpdate time.Time
	var lastProgressUpdate time.Time
	var lastProgressLog time.Time
	var scanMu sync.Mutex
	walkCtx, cancelWalk := context.WithCancel(ctx)
	defer cancelWalk()
	cancelPollDone := make(chan struct{})
	var cancelPollErr error
	go func() {
		defer close(cancelPollDone)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-walkCtx.Done():
				return
			case <-ticker.C:
				scanMu.Lock()
				err := r.checkCanceled(ctx, job)
				scanMu.Unlock()
				if err != nil {
					cancelPollErr = err
					cancelWalk()
					return
				}
			}
		}
	}()
	report, err := adapter.WalkRecursiveBounded(walkCtx, storage.WalkOptions{
		Prefixes:          prefixes,
		MaxFiles:          payload.MaxFiles,
		MaxBytes:          payload.MaxBytes,
		MaxFolderWorkers:  r.MaxFolderWorkers,
		MaxFileWorkers:    r.MaxFileWorkers,
		FolderQueueDepth:  r.FolderQueueDepth,
		IncludeExtensions: payload.IncludeExtensions,
		ExcludePatterns:   payload.ExcludePatterns,
		Progress: func(report storage.WalkReport) {
			scanMu.Lock()
			defer scanMu.Unlock()
			applyWalkReportCounters(job, report)
			if time.Since(lastProgressLog) > 30*time.Second {
				lastProgressLog = time.Now()
				jobs.AddLog(job, "info", fmt.Sprintf("walk progress: folders %d/%d, files seen %d, media returned %d, skipped %d", report.FoldersScanned, report.FoldersQueued, report.FilesSeen, report.FilesReturned, report.FilesSkipped))
			}
			if time.Since(lastProgressUpdate) > 5*time.Second {
				lastProgressUpdate = time.Now()
				_ = r.updateJob(ctx, *job)
			}
		},
	}, func(file storage.FileInfo) error {
		scanMu.Lock()
		defer scanMu.Unlock()
		if file.MediaKind == "other" {
			return nil
		}
		if job.ProgressCurrent%50 == 0 {
			if err := r.checkCanceled(ctx, job); err != nil {
				return err
			}
		}
		result, err := r.Store.UpsertDiscoveredFile(ctx, file)
		if err != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "error", fmt.Sprintf("%s: %v", file.StorageURL, err))
			return nil
		}
		job.Counters.Scanned++
		job.Counters.Bytes += file.SizeBytes
		if result.Created {
			job.Counters.Created++
		} else {
			job.Counters.Updated++
		}
		if err := r.applyTextHints(ctx, result.Asset.ID, file); err != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "warn", fmt.Sprintf("%s: fixture hints skipped: %v", file.StorageURL, err))
		}
		job.ProgressCurrent++
		if job.ProgressCurrent%250 == 0 || time.Since(lastUpdate) > 2*time.Second {
			lastUpdate = time.Now()
			if err := r.updateJob(ctx, *job); err != nil {
				return err
			}
		}
		return nil
	})
	cancelWalk()
	<-cancelPollDone
	if cancelPollErr != nil {
		return cancelPollErr
	}
	if errors.Is(err, jobs.ErrCanceled) {
		return err
	}
	if err != nil {
		job.Counters.Errors++
		jobs.AddLog(job, "error", err.Error())
		cause := classifyStorageFailure(err)
		_ = r.failJob(ctx, *job, cause)
		return cause
	}
	jobs.AddLog(job, "info", fmt.Sprintf("bounded scan storage %s prefixes %v: returned %d files, seen %d, folders %d/%d, complete=%t", storageName, prefixes, report.FilesReturned, report.FilesSeen, report.FoldersScanned, report.FoldersQueued, report.Complete))
	applyWalkReportCounters(job, report)
	if !report.Complete {
		jobs.AddLog(job, "warn", "bounded scan stopped at max_files or max_bytes limit; missing-file marking remains disabled")
	}
	return r.updateJob(ctx, *job)
}

func applyWalkReportCounters(job *jobs.Job, report storage.WalkReport) {
	job.Counters.FoldersQueued = int64(report.FoldersQueued)
	job.Counters.FoldersScanned = int64(report.FoldersScanned)
	job.Counters.FilesSeen = int64(report.FilesSeen)
	job.Counters.FilesReturned = int64(report.FilesReturned)
	job.Counters.FilesSkipped = int64(report.FilesSkipped)
}

func containsRealArchiveStorage(storages []storage.Config) bool {
	for _, storageConfig := range storages {
		if isRealArchiveRoot(storageConfig.Root) {
			return true
		}
	}
	return false
}

func isRealArchiveRoot(root string) bool {
	cleanRoot := filepath.Clean(root)
	realRoot := filepath.Clean("/mnt/Models/rclone")
	return cleanRoot == realRoot || strings.HasPrefix(cleanRoot, realRoot+string(filepath.Separator))
}

func validateRealArchiveScanPayload(payload ScanPayload) error {
	if strings.TrimSpace(payload.Storage) == "" || strings.EqualFold(payload.Storage, "all") {
		return fmt.Errorf("real archive discovery requires one explicit storage name")
	}
	if len(payload.Prefixes) == 0 {
		return fmt.Errorf("real archive discovery requires at least one adapter-relative prefix")
	}
	if payload.MaxFiles == 0 {
		return fmt.Errorf("real archive discovery requires explicit max_files; use -1 for no file-count limit")
	}
	if payload.MaxBytes == 0 {
		return fmt.Errorf("real archive discovery requires explicit max_bytes; use -1 for no byte-count limit")
	}
	return nil
}

func normalizeScanPrefixes(adapter *storage.FSAdapter, prefixes []string) ([]string, error) {
	out := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" || prefix == "." || prefix == "/" || prefix == ".." {
			return nil, storage.ErrTraversal
		}
		if filepath.IsAbs(prefix) {
			abs := filepath.Clean(prefix)
			if evaluated, err := filepath.EvalSymlinks(abs); err == nil {
				abs = evaluated
			}
			rel, err := filepath.Rel(adapter.Root(), abs)
			if err != nil {
				return nil, err
			}
			rel = filepath.ToSlash(rel)
			if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || rel == "" {
				return nil, storage.ErrTraversal
			}
			prefix = rel
		}
		rel, err := storage.NormalizeRelativePath(prefix)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, nil
}

func (r Runner) HashUnhashed(ctx context.Context, job *jobs.Job) error {
	if r.Registry == nil || r.Store == nil {
		return jobs.Permanent(fmt.Errorf("hash runner is not configured"))
	}
	payload := DecodeHashPayload(job.Payload)
	if err := jobs.Start(job); err != nil {
		return err
	}
	if err := r.validateHashSafety(payload); err != nil {
		cause := jobs.Permanent(err)
		_ = r.failJob(ctx, *job, cause)
		return cause
	}
	jobs.AddLog(job, "info", "hash job started")
	assets, err := r.Store.ListAssets(ctx)
	if err != nil {
		_ = r.failJob(ctx, *job, err)
		return err
	}
	targets, err := r.filterHashTargets(ctx, assets, payload)
	if err != nil {
		cause := jobs.Permanent(err)
		_ = r.failJob(ctx, *job, cause)
		return cause
	}
	if payload.MaxFiles > 0 && len(targets) > payload.MaxFiles {
		targets = targets[:payload.MaxFiles]
		jobs.AddLog(job, "warn", fmt.Sprintf("hash target list was limited to %d assets", payload.MaxFiles))
	}
	jobs.AddLog(job, "info", fmt.Sprintf("hash target count: %d", len(targets)))
	if len(targets) == 0 {
		jobs.AddLog(job, "warn", r.hashNoTargetsReason(assets, payload))
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

func (r Runner) validateHashSafety(payload HashPayload) error {
	return ValidateHashSafety(r.Registry, payload)
}

func (r Runner) filterHashTargets(ctx context.Context, assets []catalog.Asset, payload HashPayload) ([]catalog.Asset, error) {
	assetFilter := map[string]struct{}{}
	for _, assetID := range payload.AssetIDs {
		assetFilter[assetID] = struct{}{}
	}
	if payload.AlbumID != "" {
		limit := payload.MaxFiles
		if limit <= 0 || limit > 500 {
			limit = 500
		}
		page, err := r.Store.ListAlbumItems(ctx, catalog.AlbumItemQuery{AlbumID: payload.AlbumID, Limit: limit})
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			assetFilter[item.Asset.ID] = struct{}{}
		}
	}
	normalizedPrefixes := map[string][]string{}
	if payload.Storage != "" && !strings.EqualFold(payload.Storage, "all") && len(payload.Prefixes) > 0 {
		adapter, err := r.Registry.Adapter(payload.Storage)
		if err != nil {
			return nil, err
		}
		prefixes, err := normalizeScanPrefixes(adapter, payload.Prefixes)
		if err != nil {
			return nil, err
		}
		normalizedPrefixes[payload.Storage] = prefixes
	}
	var targets []catalog.Asset
	for _, asset := range assets {
		loc, ok := catalog.FirstLocation(asset)
		if !ok || loc.HashStatus == catalog.HashStatusHashed {
			continue
		}
		if len(assetFilter) > 0 {
			if _, ok := assetFilter[asset.ID]; !ok {
				continue
			}
		}
		if payload.Storage != "" && !strings.EqualFold(payload.Storage, "all") && loc.StorageName != payload.Storage {
			continue
		}
		if prefixes := normalizedPrefixes[loc.StorageName]; len(prefixes) > 0 && !relativePathInPrefixes(loc.RelativePath, prefixes) {
			continue
		}
		targets = append(targets, asset)
	}
	return targets, nil
}

func (r Runner) hashNoTargetsReason(assets []catalog.Asset, payload HashPayload) string {
	scoped := 0
	hashed := 0
	normalizedPrefixes := map[string][]string{}
	if payload.Storage != "" && !strings.EqualFold(payload.Storage, "all") && len(payload.Prefixes) > 0 && r.Registry != nil {
		if adapter, err := r.Registry.Adapter(payload.Storage); err == nil {
			if prefixes, err := normalizeScanPrefixes(adapter, payload.Prefixes); err == nil {
				normalizedPrefixes[payload.Storage] = prefixes
			}
		}
	}
	assetFilter := map[string]struct{}{}
	for _, assetID := range payload.AssetIDs {
		assetFilter[assetID] = struct{}{}
	}
	for _, asset := range assets {
		loc, ok := catalog.FirstLocation(asset)
		if !ok {
			continue
		}
		if len(assetFilter) > 0 {
			if _, ok := assetFilter[asset.ID]; !ok {
				continue
			}
		}
		if payload.Storage != "" && !strings.EqualFold(payload.Storage, "all") && loc.StorageName != payload.Storage {
			continue
		}
		if prefixes := normalizedPrefixes[loc.StorageName]; len(prefixes) > 0 && !relativePathInPrefixes(loc.RelativePath, prefixes) {
			continue
		}
		scoped++
		if loc.HashStatus == catalog.HashStatusHashed {
			hashed++
		}
	}
	if scoped > 0 && scoped == hashed {
		return fmt.Sprintf("hash target count is 0: all %d assets in scope are already hashed", scoped)
	}
	if scoped == 0 {
		return "hash target count is 0: no assets match the selected scope"
	}
	return fmt.Sprintf("hash target count is 0: %d assets match scope but no unhashed files need work", scoped)
}

func relativePathInPrefixes(relativePath string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if relativePath == prefix || strings.HasPrefix(relativePath, prefix+"/") {
			return true
		}
	}
	return false
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
	if info.MediaKind == "video" || info.MediaKind == "audio" {
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
	probe, err := media.ProbeMedia(ctx, path)
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
	if probe.Codec != "" {
		metadata["codec"] = probe.Codec
	}
	if probe.AudioCodec != "" {
		metadata["audio_codec"] = probe.AudioCodec
	}
	if probe.Container != "" {
		metadata["container"] = probe.Container
	}
	if probe.BitrateBPS != nil {
		metadata["bitrate_bps"] = *probe.BitrateBPS
	}
	if probe.FrameRate != nil {
		metadata["frame_rate"] = *probe.FrameRate
	}
	if probe.SampleRateHz != nil {
		metadata["sample_rate_hz"] = *probe.SampleRateHz
	}
	if probe.Channels != nil {
		metadata["channels"] = *probe.Channels
	}
	metadata["has_video_stream"] = probe.HasVideo
	metadata["has_audio_stream"] = probe.HasAudio
	if info.MediaKind == "audio" {
		_, _ = r.Store.UpsertAudioFeatures(ctx, catalog.AudioFeatures{
			AssetID:         assetID,
			DurationSeconds: probe.DurationSeconds,
			Model:           "ffprobe_metadata",
			Metadata: map[string]any{
				"audio_codec":    probe.AudioCodec,
				"container":      probe.Container,
				"sample_rate_hz": metadata["sample_rate_hz"],
				"channels":       metadata["channels"],
				"analyzer":       "ffprobe",
				"genre_status":   "model_missing",
			},
		})
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
