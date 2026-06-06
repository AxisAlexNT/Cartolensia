package catalog

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/id"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

const (
	HashStatusUnhashed = "unhashed"
	HashStatusHashed   = "hashed"
)

type Asset struct {
	ID          string     `json:"id"`
	MediaKind   string     `json:"media_kind"`
	DisplayName string     `json:"display_name"`
	FirstSeenAt time.Time  `json:"first_seen_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Locations   []Location `json:"locations"`
}

type Location struct {
	ID           string    `json:"id"`
	AssetID      string    `json:"asset_id"`
	StorageName  string    `json:"storage_name"`
	StorageURL   string    `json:"storage_url"`
	RelativePath string    `json:"relative_path"`
	FileName     string    `json:"file_name"`
	Extension    string    `json:"extension"`
	MIME         string    `json:"mime"`
	MediaKind    string    `json:"media_kind"`
	SizeBytes    int64     `json:"size_bytes"`
	MTime        time.Time `json:"mtime"`
	HashStatus   string    `json:"hash_status"`
	SHA512Hex    string    `json:"sha512_hex,omitempty"`
	ContentID    string    `json:"content_id,omitempty"`
	LastSeenAt   time.Time `json:"last_seen_at"`
}

type Stats struct {
	Assets     int   `json:"assets"`
	Locations  int   `json:"locations"`
	Photos     int   `json:"photos"`
	Videos     int   `json:"videos"`
	Tracks     int   `json:"tracks"`
	Unhashed   int   `json:"unhashed"`
	Hashed     int   `json:"hashed"`
	TotalBytes int64 `json:"total_bytes"`
}

type UpsertResult struct {
	Asset   Asset `json:"asset"`
	Created bool  `json:"created"`
}

type Store interface {
	UpsertDiscoveredFile(context.Context, storage.FileInfo) (UpsertResult, error)
	ListAssets(context.Context) ([]Asset, error)
	GetAsset(context.Context, string) (Asset, error)
	UpdateLocationHash(context.Context, string, string, int64) error
	Stats(context.Context) (Stats, error)
	EnqueueJob(context.Context, jobs.Job) (jobs.Job, error)
	UpdateJob(context.Context, jobs.Job) error
	ListJobs(context.Context) ([]jobs.Job, error)
	GetJob(context.Context, string) (jobs.Job, error)
	LeaseNextJob(context.Context, string, []string, time.Duration) (jobs.Job, error)
	HeartbeatJob(context.Context, string, string, time.Duration) error
	UpdateLeasedJob(context.Context, jobs.Job, string) error
	CompleteLeasedJob(context.Context, jobs.Job, string) error
	FailLeasedJob(context.Context, jobs.Job, string, error) error
	CancelLeasedJob(context.Context, jobs.Job, string) error
	RequestCancelJob(context.Context, string) (jobs.Job, error)
	ReleaseExpiredLeases(context.Context, time.Time) (int64, error)
}

type MemoryStore struct {
	mu              sync.RWMutex
	assets          map[string]Asset
	byURL           map[string]string
	locationByAsset map[string]string
	jobs            map[string]jobs.Job
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		assets:          make(map[string]Asset),
		byURL:           make(map[string]string),
		locationByAsset: make(map[string]string),
		jobs:            make(map[string]jobs.Job),
	}
}

func (s *MemoryStore) UpsertDiscoveredFile(_ context.Context, info storage.FileInfo) (UpsertResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if assetID, ok := s.byURL[info.StorageURL]; ok {
		asset := s.assets[assetID]
		if len(asset.Locations) > 0 {
			asset.Locations[0].SizeBytes = info.SizeBytes
			asset.Locations[0].MTime = info.MTime
			asset.Locations[0].MIME = info.MIME
			asset.Locations[0].LastSeenAt = now
			asset.Locations[0].MediaKind = info.MediaKind
			asset.Locations[0].Extension = info.Extension
		}
		asset.MediaKind = info.MediaKind
		asset.DisplayName = info.Name
		asset.UpdatedAt = now
		s.assets[assetID] = asset
		return UpsertResult{Asset: asset}, nil
	}
	assetID := id.NewUUID()
	locationID := id.NewUUID()
	asset := Asset{
		ID:          assetID,
		MediaKind:   info.MediaKind,
		DisplayName: info.Name,
		FirstSeenAt: now,
		UpdatedAt:   now,
		Locations: []Location{{
			ID:           locationID,
			AssetID:      assetID,
			StorageName:  info.StorageName,
			StorageURL:   info.StorageURL,
			RelativePath: info.RelativePath,
			FileName:     info.Name,
			Extension:    info.Extension,
			MIME:         info.MIME,
			MediaKind:    info.MediaKind,
			SizeBytes:    info.SizeBytes,
			MTime:        info.MTime,
			HashStatus:   HashStatusUnhashed,
			LastSeenAt:   now,
		}},
	}
	s.assets[assetID] = asset
	s.byURL[info.StorageURL] = assetID
	s.locationByAsset[assetID] = locationID
	return UpsertResult{Asset: asset, Created: true}, nil
}

func (s *MemoryStore) ListAssets(_ context.Context) ([]Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Asset, 0, len(s.assets))
	for _, asset := range s.assets {
		out = append(out, cloneAsset(asset))
	}
	sort.Slice(out, func(i, j int) bool {
		left := ""
		right := ""
		if len(out[i].Locations) > 0 {
			left = out[i].Locations[0].StorageURL
		}
		if len(out[j].Locations) > 0 {
			right = out[j].Locations[0].StorageURL
		}
		return left < right
	})
	return out, nil
}

func (s *MemoryStore) GetAsset(_ context.Context, assetID string) (Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	asset, ok := s.assets[assetID]
	if !ok {
		return Asset{}, ErrNotFound
	}
	return cloneAsset(asset), nil
}

func (s *MemoryStore) UpdateLocationHash(_ context.Context, assetID, sha512Hex string, bytes int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	asset, ok := s.assets[assetID]
	if !ok {
		return ErrNotFound
	}
	contentID := id.NewUUID()
	for i := range asset.Locations {
		asset.Locations[i].SHA512Hex = sha512Hex
		asset.Locations[i].ContentID = contentID
		asset.Locations[i].HashStatus = HashStatusHashed
		if bytes >= 0 {
			asset.Locations[i].SizeBytes = bytes
		}
	}
	asset.UpdatedAt = time.Now().UTC()
	s.assets[assetID] = asset
	return nil
}

func (s *MemoryStore) Stats(_ context.Context) (Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var stats Stats
	stats.Assets = len(s.assets)
	for _, asset := range s.assets {
		for _, loc := range asset.Locations {
			stats.Locations++
			stats.TotalBytes += loc.SizeBytes
			switch loc.MediaKind {
			case "photo":
				stats.Photos++
			case "video":
				stats.Videos++
			case "track":
				stats.Tracks++
			}
			switch loc.HashStatus {
			case HashStatusHashed:
				stats.Hashed++
			default:
				stats.Unhashed++
			}
		}
	}
	return stats, nil
}

func (s *MemoryStore) EnqueueJob(_ context.Context, job jobs.Job) (jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 3
	}
	s.jobs[job.ID] = job
	return cloneJob(job), nil
}

func (s *MemoryStore) UpdateJob(_ context.Context, job jobs.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[job.ID]; !ok {
		return ErrNotFound
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *MemoryStore) ListJobs(_ context.Context) ([]jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]jobs.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		out = append(out, cloneJob(job))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryStore) GetJob(_ context.Context, jobID string) (jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return jobs.Job{}, ErrNotFound
	}
	return cloneJob(job), nil
}

func (s *MemoryStore) LeaseNextJob(_ context.Context, workerID string, kinds []string, leaseDuration time.Duration) (jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	_, _ = s.releaseExpiredLeasesLocked(now)
	var selected *jobs.Job
	for _, job := range s.jobs {
		job := job
		if job.Status != jobs.StatusQueued {
			continue
		}
		if job.NextRunAt != nil && job.NextRunAt.After(now) {
			continue
		}
		if !jobKindAllowed(job.Kind, kinds) {
			continue
		}
		if selected == nil || job.CreatedAt.Before(selected.CreatedAt) || (job.CreatedAt.Equal(selected.CreatedAt) && job.ID < selected.ID) {
			selected = &job
		}
	}
	if selected == nil {
		return jobs.Job{}, ErrNotFound
	}
	job := *selected
	job.Status = jobs.StatusRunning
	job.WorkerID = workerID
	leaseUntil := now.Add(leaseDuration)
	job.LeaseExpiresAt = &leaseUntil
	if job.StartedAt == nil {
		started := now
		job.StartedAt = &started
	}
	job.Attempts++
	job.NextRunAt = nil
	job.Error = ""
	s.jobs[job.ID] = cloneJob(job)
	return cloneJob(job), nil
}

func (s *MemoryStore) HeartbeatJob(_ context.Context, jobID, workerID string, leaseDuration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return ErrNotFound
	}
	if job.WorkerID != workerID || (job.Status != jobs.StatusRunning && job.Status != jobs.StatusCancelRequested) {
		return ErrJobLeaseLost
	}
	leaseUntil := time.Now().UTC().Add(leaseDuration)
	job.LeaseExpiresAt = &leaseUntil
	s.jobs[jobID] = job
	return nil
}

func (s *MemoryStore) UpdateLeasedJob(_ context.Context, job jobs.Job, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.jobs[job.ID]
	if !ok {
		return ErrNotFound
	}
	if current.WorkerID != workerID || (current.Status != jobs.StatusRunning && current.Status != jobs.StatusCancelRequested) {
		return ErrJobLeaseLost
	}
	if current.Status == jobs.StatusCancelRequested && job.Status == jobs.StatusRunning {
		job.Status = jobs.StatusCancelRequested
		job.CancelRequestedAt = current.CancelRequestedAt
	}
	job.WorkerID = current.WorkerID
	job.LeaseExpiresAt = current.LeaseExpiresAt
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *MemoryStore) CompleteLeasedJob(_ context.Context, job jobs.Job, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.jobs[job.ID]
	if !ok {
		return ErrNotFound
	}
	if current.WorkerID != workerID || current.Status != jobs.StatusRunning {
		return ErrJobLeaseLost
	}
	job.WorkerID = current.WorkerID
	job.LeaseExpiresAt = current.LeaseExpiresAt
	if err := jobs.Complete(&job); err != nil {
		return err
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *MemoryStore) FailLeasedJob(_ context.Context, job jobs.Job, workerID string, cause error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.jobs[job.ID]
	if !ok {
		return ErrNotFound
	}
	if current.WorkerID != workerID || (current.Status != jobs.StatusRunning && current.Status != jobs.StatusCancelRequested) {
		return ErrJobLeaseLost
	}
	job.WorkerID = current.WorkerID
	job.LeaseExpiresAt = current.LeaseExpiresAt
	return s.failLeasedLocked(job, cause)
}

func (s *MemoryStore) CancelLeasedJob(_ context.Context, job jobs.Job, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.jobs[job.ID]
	if !ok {
		return ErrNotFound
	}
	if current.WorkerID != workerID || (current.Status != jobs.StatusRunning && current.Status != jobs.StatusCancelRequested) {
		return ErrJobLeaseLost
	}
	job.WorkerID = current.WorkerID
	job.LeaseExpiresAt = current.LeaseExpiresAt
	if err := jobs.Cancel(&job); err != nil {
		return err
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *MemoryStore) RequestCancelJob(_ context.Context, jobID string) (jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return jobs.Job{}, ErrNotFound
	}
	if err := jobs.RequestCancel(&job); err != nil {
		return jobs.Job{}, err
	}
	jobs.AddLog(&job, "info", "cancellation requested")
	s.jobs[jobID] = cloneJob(job)
	return cloneJob(job), nil
}

func (s *MemoryStore) ReleaseExpiredLeases(_ context.Context, now time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.releaseExpiredLeasesLocked(now)
}

func (s *MemoryStore) releaseExpiredLeasesLocked(now time.Time) (int64, error) {
	var released int64
	for id, job := range s.jobs {
		if job.LeaseExpiresAt == nil || !job.LeaseExpiresAt.Before(now) {
			continue
		}
		if job.Status != jobs.StatusRunning && job.Status != jobs.StatusCancelRequested {
			continue
		}
		released++
		if job.Status == jobs.StatusCancelRequested {
			_ = jobs.Cancel(&job)
			jobs.AddLog(&job, "warn", "expired lease cancelled after cancellation request")
			s.jobs[id] = cloneJob(job)
			continue
		}
		_ = s.failLeasedLocked(job, fmt.Errorf("job lease expired"))
	}
	return released, nil
}

func (s *MemoryStore) failLeasedLocked(job jobs.Job, cause error) error {
	if cause == nil {
		cause = fmt.Errorf("job failed")
	}
	maxAttempts := job.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if job.Attempts < maxAttempts && job.Status != jobs.StatusCancelRequested {
		delay := retryDelay(job.Attempts)
		if err := jobs.Retry(&job, delay, cause); err != nil {
			return err
		}
		jobs.AddLog(&job, "warn", fmt.Sprintf("will retry after %s: %v", delay, cause))
	} else {
		if err := jobs.Fail(&job, cause); err != nil {
			return err
		}
		jobs.AddLog(&job, "error", cause.Error())
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

var ErrNotFound = &notFoundError{}

var ErrJobLeaseLost = errors.New("job lease is not owned by worker")

type notFoundError struct{}

func (*notFoundError) Error() string { return "not found" }

type ExplorerOptions struct {
	Storage   string
	Path      string
	MediaKind string
	Limit     int
	Offset    int
	Sort      string
}

type ExplorerView struct {
	CurrentPath string           `json:"current_path"`
	ParentPath  string           `json:"parent_path,omitempty"`
	Folders     []ExplorerFolder `json:"folders"`
	Files       []ExplorerFile   `json:"files"`
	FileCount   int              `json:"file_count"`
	FolderCount int              `json:"folder_count"`
	TotalBytes  int64            `json:"total_bytes"`
	Offset      int              `json:"offset"`
	Limit       int              `json:"limit"`
}

type ExplorerFolder struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	FileCount   int       `json:"file_count"`
	TotalBytes  int64     `json:"total_bytes"`
	LatestMTime time.Time `json:"latest_mtime"`
}

type ExplorerFile struct {
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

func BuildExplorerView(assets []Asset, opts ExplorerOptions) (ExplorerView, error) {
	current, err := normalizeExplorerPath(opts.Path)
	if err != nil {
		return ExplorerView{}, err
	}
	if opts.Limit <= 0 || opts.Limit > 500 {
		opts.Limit = 200
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	view := ExplorerView{
		CurrentPath: current,
		ParentPath:  parentExplorerPath(current),
		Limit:       opts.Limit,
		Offset:      opts.Offset,
	}
	folderIndex := map[string]int{}
	for _, asset := range assets {
		for _, loc := range asset.Locations {
			if opts.Storage != "" && loc.StorageName != opts.Storage {
				continue
			}
			if opts.MediaKind != "" && loc.MediaKind != opts.MediaKind {
				continue
			}
			remaining, ok := pathRemainder(loc.RelativePath, current)
			if !ok || remaining == "" {
				continue
			}
			view.TotalBytes += loc.SizeBytes
			if slash := strings.IndexByte(remaining, '/'); slash >= 0 {
				name := remaining[:slash]
				folderPath := joinExplorerPath(current, name)
				idx, ok := folderIndex[folderPath]
				if !ok {
					view.Folders = append(view.Folders, ExplorerFolder{Name: name, Path: folderPath})
					idx = len(view.Folders) - 1
					folderIndex[folderPath] = idx
				}
				view.Folders[idx].FileCount++
				view.Folders[idx].TotalBytes += loc.SizeBytes
				if loc.MTime.After(view.Folders[idx].LatestMTime) {
					view.Folders[idx].LatestMTime = loc.MTime
				}
				continue
			}
			view.Files = append(view.Files, ExplorerFile{
				AssetID:      asset.ID,
				Name:         loc.FileName,
				MediaKind:    loc.MediaKind,
				StorageURL:   loc.StorageURL,
				RelativePath: loc.RelativePath,
				SizeBytes:    loc.SizeBytes,
				MTime:        loc.MTime,
				HashStatus:   loc.HashStatus,
				SHA512Hex:    loc.SHA512Hex,
			})
		}
	}
	sort.Slice(view.Folders, func(i, j int) bool {
		return view.Folders[i].Name < view.Folders[j].Name
	})
	sortExplorerFiles(view.Files, opts.Sort)
	view.FileCount = len(view.Files)
	view.FolderCount = len(view.Folders)
	if opts.Offset >= len(view.Files) {
		view.Files = nil
		return view, nil
	}
	end := opts.Offset + opts.Limit
	if end > len(view.Files) {
		end = len(view.Files)
	}
	view.Files = view.Files[opts.Offset:end]
	return view, nil
}

func cloneAsset(asset Asset) Asset {
	asset.Locations = append([]Location(nil), asset.Locations...)
	return asset
}

func cloneJob(job jobs.Job) jobs.Job {
	job.Logs = append([]jobs.LogLine(nil), job.Logs...)
	return job
}

func FirstLocation(asset Asset) (Location, bool) {
	if len(asset.Locations) == 0 {
		return Location{}, false
	}
	return asset.Locations[0], true
}

func SearchAssets(assets []Asset, query string) []Asset {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return assets
	}
	var out []Asset
	for _, asset := range assets {
		if strings.Contains(strings.ToLower(asset.DisplayName), query) {
			out = append(out, asset)
			continue
		}
		for _, loc := range asset.Locations {
			if strings.Contains(strings.ToLower(loc.RelativePath), query) {
				out = append(out, asset)
				break
			}
		}
	}
	return out
}

func jobKindAllowed(kind string, kinds []string) bool {
	if len(kinds) == 0 {
		return true
	}
	for _, allowed := range kinds {
		if kind == allowed {
			return true
		}
	}
	return false
}

func retryDelay(attempts int) time.Duration {
	if attempts <= 0 {
		return time.Second
	}
	if attempts > 5 {
		attempts = 5
	}
	return time.Duration(attempts) * time.Second
}

func normalizeExplorerPath(input string) (string, error) {
	input = strings.TrimSpace(strings.ReplaceAll(input, "\\", "/"))
	if input == "" || input == "/" {
		return "", nil
	}
	if strings.HasPrefix(input, "/") {
		return "", fmt.Errorf("explorer path must be relative")
	}
	clean := path.Clean(input)
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("explorer path escapes root")
	}
	return clean, nil
}

func parentExplorerPath(current string) string {
	if current == "" {
		return ""
	}
	parent := path.Dir(current)
	if parent == "." {
		return ""
	}
	return parent
}

func joinExplorerPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

func pathRemainder(relativePath, current string) (string, bool) {
	if current == "" {
		return relativePath, true
	}
	if relativePath == current {
		return "", false
	}
	prefix := current + "/"
	if !strings.HasPrefix(relativePath, prefix) {
		return "", false
	}
	return strings.TrimPrefix(relativePath, prefix), true
}

func sortExplorerFiles(files []ExplorerFile, key string) {
	switch key {
	case "size":
		sort.Slice(files, func(i, j int) bool {
			if files[i].SizeBytes == files[j].SizeBytes {
				return files[i].Name < files[j].Name
			}
			return files[i].SizeBytes < files[j].SizeBytes
		})
	case "mtime":
		sort.Slice(files, func(i, j int) bool {
			if files[i].MTime.Equal(files[j].MTime) {
				return files[i].Name < files[j].Name
			}
			return files[i].MTime.After(files[j].MTime)
		})
	case "media_kind":
		sort.Slice(files, func(i, j int) bool {
			if files[i].MediaKind == files[j].MediaKind {
				return files[i].Name < files[j].Name
			}
			return files[i].MediaKind < files[j].MediaKind
		})
	default:
		sort.Slice(files, func(i, j int) bool {
			return files[i].Name < files[j].Name
		})
	}
}
