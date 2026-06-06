package catalog

import (
	"context"
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
	s.jobs[job.ID] = job
	return job, nil
}

func (s *MemoryStore) UpdateJob(_ context.Context, job jobs.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[job.ID]; !ok {
		return ErrNotFound
	}
	s.jobs[job.ID] = job
	return nil
}

func (s *MemoryStore) ListJobs(_ context.Context) ([]jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]jobs.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		out = append(out, job)
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
	return job, nil
}

var ErrNotFound = &notFoundError{}

type notFoundError struct{}

func (*notFoundError) Error() string { return "not found" }

func cloneAsset(asset Asset) Asset {
	asset.Locations = append([]Location(nil), asset.Locations...)
	return asset
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
