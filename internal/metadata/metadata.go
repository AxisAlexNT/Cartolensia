package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/gpx"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/media"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

type Payload struct {
	AssetIDs      []string `json:"asset_ids,omitempty"`
	Storage       string   `json:"storage,omitempty"`
	MediaKind     string   `json:"media_kind,omitempty"`
	OnlyMissing   bool     `json:"only_missing,omitempty"`
	MaxFiles      int      `json:"max_files,omitempty"`
	IncludeVideo  bool     `json:"include_video"`
	IncludeImages bool     `json:"include_images"`
	IncludeTracks bool     `json:"include_tracks"`
}

type Runner struct {
	Registry      *storage.Registry
	Store         catalog.Store
	WorkerID      string
	LeaseDuration time.Duration
}

func NewPayload() Payload {
	return Payload{IncludeVideo: true, IncludeImages: true, IncludeTracks: true}
}

func DecodePayload(raw any) Payload {
	payload := NewPayload()
	if raw == nil {
		return payload
	}
	data, err := json.Marshal(raw)
	if err == nil {
		_ = json.Unmarshal(data, &payload)
	}
	return payload
}

func (r Runner) Enrich(ctx context.Context, job *jobs.Job) error {
	if r.Registry == nil || r.Store == nil {
		return jobs.Permanent(fmt.Errorf("metadata runner is not configured"))
	}
	payload := DecodePayload(job.Payload)
	if err := jobs.Start(job); err != nil {
		return err
	}
	jobs.AddLog(job, "info", "metadata enrichment started")
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	assets, err := r.Store.ListAssets(ctx)
	if err != nil {
		_ = r.failJob(ctx, *job, err)
		return err
	}
	targets := selectTargets(assets, payload)
	total := int64(len(targets))
	job.ProgressTotal = &total
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	for _, asset := range targets {
		if err := r.checkCanceled(ctx, job); err != nil {
			return err
		}
		if err := r.enrichAsset(ctx, asset); err != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "warn", fmt.Sprintf("%s: metadata skipped: %v", asset.DisplayName, err))
		} else {
			job.Counters.Updated++
		}
		job.ProgressCurrent++
		if err := r.updateJob(ctx, *job); err != nil {
			return err
		}
	}
	jobs.AddLog(job, "info", "metadata enrichment completed")
	return r.completeJob(ctx, *job)
}

func selectTargets(assets []catalog.Asset, payload Payload) []catalog.Asset {
	ids := map[string]struct{}{}
	for _, id := range payload.AssetIDs {
		ids[id] = struct{}{}
	}
	var out []catalog.Asset
	for _, asset := range assets {
		loc, ok := catalog.FirstLocation(asset)
		if !ok {
			continue
		}
		if len(ids) > 0 {
			if _, ok := ids[asset.ID]; !ok {
				continue
			}
		}
		if payload.Storage != "" && loc.StorageName != payload.Storage {
			continue
		}
		if payload.MediaKind != "" && asset.MediaKind != payload.MediaKind {
			continue
		}
		if !payload.IncludeImages && asset.MediaKind == "photo" {
			continue
		}
		if !payload.IncludeVideo && asset.MediaKind == "video" {
			continue
		}
		if !payload.IncludeTracks && asset.MediaKind == "track" {
			continue
		}
		if payload.OnlyMissing && !metadataMissing(asset) {
			continue
		}
		out = append(out, asset)
		if payload.MaxFiles > 0 && len(out) >= payload.MaxFiles {
			break
		}
	}
	return out
}

func metadataMissing(asset catalog.Asset) bool {
	switch asset.MediaKind {
	case "photo":
		return asset.Metadata["width"] == nil || asset.Metadata["height"] == nil
	case "video":
		return asset.Metadata["duration_seconds"] == nil || asset.Metadata["ffprobe_available"] == nil
	case "track":
		return asset.Metadata["track_point_count"] == nil
	default:
		return false
	}
}

func (r Runner) enrichAsset(ctx context.Context, asset catalog.Asset) error {
	loc, ok := catalog.FirstLocation(asset)
	if !ok {
		return catalog.ErrNotFound
	}
	switch loc.MediaKind {
	case "photo":
		return r.enrichImage(ctx, asset, loc)
	case "video":
		return r.enrichVideo(ctx, asset, loc)
	case "track":
		if strings.EqualFold(loc.Extension, "gpx") {
			return r.enrichGPX(ctx, asset, loc)
		}
	}
	return nil
}

func (r Runner) enrichImage(ctx context.Context, asset catalog.Asset, loc catalog.Location) error {
	file, _, err := r.Registry.OpenByURL(loc.StorageURL)
	if err != nil {
		return err
	}
	defer file.Close()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil
	}
	return r.Store.UpdateAssetMetadata(ctx, asset.ID, nil, map[string]any{
		"width":                 cfg.Width,
		"height":                cfg.Height,
		"metadata_extracted_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (r Runner) enrichVideo(ctx context.Context, asset catalog.Asset, loc catalog.Location) error {
	file, _, err := r.Registry.OpenByURL(loc.StorageURL)
	if err != nil {
		return err
	}
	path := file.Name()
	_ = file.Close()
	probe, err := media.ProbeVideo(ctx, path)
	metadata := map[string]any{
		"ffprobe_available":     probe.Available,
		"metadata_extracted_at": time.Now().UTC().Format(time.RFC3339),
	}
	if err != nil {
		return r.Store.UpdateAssetMetadata(ctx, asset.ID, nil, metadata)
	}
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
	if probe.Container != "" {
		metadata["container"] = probe.Container
	}
	if probe.BitrateBPS != nil {
		metadata["bitrate_bps"] = *probe.BitrateBPS
	}
	if probe.FrameRate != nil {
		metadata["frame_rate"] = *probe.FrameRate
	}
	return r.Store.UpdateAssetMetadata(ctx, asset.ID, nil, metadata)
}

func (r Runner) enrichGPX(ctx context.Context, asset catalog.Asset, loc catalog.Location) error {
	file, _, err := r.Registry.OpenByURL(loc.StorageURL)
	if err != nil {
		return err
	}
	defer file.Close()
	points, err := gpx.Parse(file)
	if err != nil {
		return err
	}
	if err := r.Store.UpsertTrackPoints(ctx, asset.ID, points); err != nil {
		return err
	}
	analysis := gpx.Analyze(points)
	metadata := map[string]any{
		"track_point_count":     analysis.PointCount,
		"distance_m":            analysis.DistanceM,
		"metadata_extracted_at": time.Now().UTC().Format(time.RFC3339),
	}
	if analysis.StartTime != nil {
		metadata["track_start_at"] = analysis.StartTime.Format(time.RFC3339)
	}
	if analysis.EndTime != nil {
		metadata["track_end_at"] = analysis.EndTime.Format(time.RFC3339)
	}
	if analysis.MinLat != nil {
		metadata["min_lat"] = *analysis.MinLat
		metadata["min_lon"] = *analysis.MinLon
		metadata["max_lat"] = *analysis.MaxLat
		metadata["max_lon"] = *analysis.MaxLon
	}
	if analysis.DurationSeconds != nil {
		metadata["duration_seconds"] = *analysis.DurationSeconds
	}
	if analysis.ElevationMinM != nil {
		metadata["elevation_min_m"] = *analysis.ElevationMinM
		metadata["elevation_max_m"] = *analysis.ElevationMaxM
	}
	var takenAt *time.Time
	if analysis.StartTime != nil {
		t := analysis.StartTime.UTC()
		takenAt = &t
	}
	return r.Store.UpdateAssetMetadata(ctx, asset.ID, takenAt, metadata)
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
