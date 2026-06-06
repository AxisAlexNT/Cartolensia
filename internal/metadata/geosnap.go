package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
)

type GeoSnapPayload struct {
	TrackAssetID     string `json:"track_asset_id"`
	OffsetSeconds    int64  `json:"offset_seconds"`
	MediaKind        string `json:"media_kind,omitempty"`
	MaxGapSeconds    int64  `json:"max_gap_seconds,omitempty"`
	MaxFiles         int    `json:"max_files,omitempty"`
	IncludeGeotagged bool   `json:"include_geotagged,omitempty"`
}

func DecodeGeoSnapPayload(raw any) GeoSnapPayload {
	var payload GeoSnapPayload
	if raw != nil {
		data, err := json.Marshal(raw)
		if err == nil {
			_ = json.Unmarshal(data, &payload)
		}
	}
	if payload.MaxGapSeconds <= 0 {
		payload.MaxGapSeconds = 300
	}
	if payload.MaxFiles <= 0 || payload.MaxFiles > 500 {
		payload.MaxFiles = 500
	}
	return payload
}

func (r Runner) SnapToTrack(ctx context.Context, job *jobs.Job) error {
	if r.Store == nil {
		return jobs.Permanent(fmt.Errorf("metadata runner is not configured"))
	}
	payload := DecodeGeoSnapPayload(job.Payload)
	if payload.TrackAssetID == "" {
		return jobs.Permanent(fmt.Errorf("track_asset_id is required"))
	}
	if err := jobs.Start(job); err != nil {
		return err
	}
	jobs.AddLog(job, "info", "track media snapping started")
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	detail, err := r.Store.GetTrack(ctx, payload.TrackAssetID)
	if err != nil {
		_ = r.failJob(ctx, *job, err)
		return err
	}
	points := timedPoints(detail.Points)
	if len(points) == 0 || detail.Summary.StartTime == nil || detail.Summary.EndTime == nil {
		jobs.AddLog(job, "warn", "track has no timed points")
		return r.completeJob(ctx, *job)
	}
	gap := time.Duration(payload.MaxGapSeconds) * time.Second
	from := detail.Summary.StartTime.Add(-gap)
	to := detail.Summary.EndTime.Add(gap)
	page, err := r.Store.QueryAssets(ctx, catalog.AssetQuery{
		MediaKind: payload.MediaKind,
		TakenFrom: &from,
		TakenTo:   &to,
		Limit:     payload.MaxFiles,
		Sort:      "taken_at",
		WithTotal: true,
	})
	if err != nil {
		_ = r.failJob(ctx, *job, err)
		return err
	}
	total := int64(len(page.Assets))
	job.ProgressTotal = &total
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	for _, asset := range page.Assets {
		if err := r.checkCanceled(ctx, job); err != nil {
			return err
		}
		if asset.ID == payload.TrackAssetID || asset.TakenAt == nil || asset.MediaKind == "track" {
			job.ProgressCurrent++
			continue
		}
		if !payload.IncludeGeotagged {
			if existing, err := r.Store.GetAssetGeo(ctx, asset.ID); err == nil && existing.Source != "estimated" && existing.Source != "track_snapped" {
				job.ProgressCurrent++
				continue
			}
		}
		target := asset.TakenAt.UTC().Add(time.Duration(payload.OffsetSeconds) * time.Second)
		lat, lon, nearestGap, ok := interpolateGeoPoint(points, target, gap)
		if !ok {
			job.ProgressCurrent++
			job.Counters.Updated++
			continue
		}
		confidence := 1 - math.Min(1, nearestGap.Seconds()/gap.Seconds())
		_, err := r.Store.UpsertAssetGeo(ctx, catalog.AssetGeo{
			AssetID:      asset.ID,
			Lat:          lat,
			Lon:          lon,
			Source:       "track_snapped",
			Confidence:   &confidence,
			TakenAt:      asset.TakenAt,
			TrackAssetID: payload.TrackAssetID,
			Metadata: map[string]any{
				"track_offset_seconds": payload.OffsetSeconds,
				"nearest_gap_seconds":  nearestGap.Seconds(),
				"source":               "track_snapped",
			},
		}, false)
		if err != nil {
			job.Counters.Errors++
			jobs.AddLog(job, "warn", fmt.Sprintf("%s: snap failed: %v", asset.DisplayName, err))
		} else {
			job.Counters.Created++
		}
		job.ProgressCurrent++
		if err := r.updateJob(ctx, *job); err != nil {
			return err
		}
	}
	jobs.AddLog(job, "info", "track media snapping completed")
	return r.completeJob(ctx, *job)
}

func timedPoints(points []catalog.TrackPoint) []catalog.TrackPoint {
	out := make([]catalog.TrackPoint, 0, len(points))
	for _, point := range points {
		if !point.RecordedAt.IsZero() {
			out = append(out, point)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordedAt.Before(out[j].RecordedAt) })
	return out
}

func interpolateGeoPoint(points []catalog.TrackPoint, target time.Time, maxGap time.Duration) (float64, float64, time.Duration, bool) {
	if len(points) == 0 {
		return 0, 0, 0, false
	}
	if target.Before(points[0].RecordedAt) {
		gap := points[0].RecordedAt.Sub(target)
		return points[0].Lat, points[0].Lon, gap, gap <= maxGap
	}
	last := points[len(points)-1]
	if target.After(last.RecordedAt) {
		gap := target.Sub(last.RecordedAt)
		return last.Lat, last.Lon, gap, gap <= maxGap
	}
	for i := 1; i < len(points); i++ {
		next := points[i]
		if target.After(next.RecordedAt) {
			continue
		}
		prev := points[i-1]
		gapPrev := target.Sub(prev.RecordedAt)
		gapNext := next.RecordedAt.Sub(target)
		nearest := gapPrev
		if gapNext < nearest {
			nearest = gapNext
		}
		if nearest > maxGap {
			return 0, 0, nearest, false
		}
		span := next.RecordedAt.Sub(prev.RecordedAt).Seconds()
		if span <= 0 {
			return prev.Lat, prev.Lon, nearest, true
		}
		ratio := target.Sub(prev.RecordedAt).Seconds() / span
		lat := prev.Lat + (next.Lat-prev.Lat)*ratio
		lon := prev.Lon + (next.Lon-prev.Lon)*ratio
		return lat, lon, nearest, true
	}
	return last.Lat, last.Lon, 0, true
}
