package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
)

type reverseGeocodeStartRequest struct {
	Limit       int     `json:"limit"`
	BatchSize   int     `json:"batch_size"`
	RadiusM     float64 `json:"radius_m"`
	Online      *bool   `json:"online"`
	OnlyMissing bool    `json:"only_missing"`
	MediaKind   string  `json:"media_kind"`
}

type reverseGeocodeJobPayload struct {
	Limit       int     `json:"limit"`
	BatchSize   int     `json:"batch_size"`
	RadiusM     float64 `json:"radius_m"`
	Online      bool    `json:"online"`
	OnlyMissing bool    `json:"only_missing"`
	MediaKind   string  `json:"media_kind"`
	Processed   int     `json:"processed,omitempty"`
	Matched     int     `json:"matched,omitempty"`
	Cached      int     `json:"cached,omitempty"`
	Failed      int     `json:"failed,omitempty"`
	StartedAt   string  `json:"started_at,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
}

func (s *Server) handleReverseGeocodeStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "places.write") {
		return
	}
	var req reverseGeocodeStartRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	}
	online := runtimeBoolSetting("search.online_geocoding", true)
	if req.Online != nil {
		online = *req.Online
	}
	payload := normalizeReverseGeocodePayload(reverseGeocodeJobPayload{
		Limit:       req.Limit,
		BatchSize:   req.BatchSize,
		RadiusM:     req.RadiusM,
		Online:      online,
		OnlyMissing: req.OnlyMissing,
		MediaKind:   req.MediaKind,
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
	})
	job, err := s.deps.Store.EnqueueJob(r.Context(), jobs.New("reverse_geocode", payload))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func normalizeReverseGeocodePayload(payload reverseGeocodeJobPayload) reverseGeocodeJobPayload {
	if payload.Limit == 0 {
		payload.Limit = -1
	}
	if payload.BatchSize <= 0 {
		payload.BatchSize = 500
	}
	if payload.BatchSize > 2000 {
		payload.BatchSize = 2000
	}
	if payload.RadiusM <= 0 {
		payload.RadiusM = float64(runtimeIntSetting("search.reverse_geocode_radius_m", 100))
	}
	if payload.RadiusM < 0 {
		payload.RadiusM = 0
	}
	if payload.RadiusM > 5000 {
		payload.RadiusM = 5000
	}
	if payload.StartedAt == "" {
		payload.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return payload
}

func decodeReverseGeocodePayload(raw any) (reverseGeocodeJobPayload, error) {
	var payload reverseGeocodeJobPayload
	if raw == nil {
		return normalizeReverseGeocodePayload(payload), nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return payload, err
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, err
	}
	return normalizeReverseGeocodePayload(payload), nil
}

func (s *Server) RunReverseGeocodeJob(ctx context.Context, job *jobs.Job, workerID string, leaseDuration time.Duration) error {
	payload, err := decodeReverseGeocodePayload(job.Payload)
	if err != nil {
		return jobs.Permanent(err)
	}
	if leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	ctx, stopHeartbeat, heartbeatErrs := s.startAIBackfillHeartbeat(ctx, job.ID, workerID, leaseDuration)
	defer stopHeartbeat()

	places := s.placeEntries(ctx)
	processed := maxInt(payload.Processed, 0)
	matched := maxInt(payload.Matched, 0)
	cached := maxInt(payload.Cached, 0)
	failed := maxInt(payload.Failed, 0)
	if payload.Limit > 0 {
		total := int64(payload.Limit)
		job.ProgressTotal = &total
	} else {
		job.ProgressTotal = nil
	}
	job.ProgressCurrent = int64(processed + failed)
	job.Counters.Scanned = int64(processed + failed)
	job.Counters.Updated = int64(matched + cached)
	job.Counters.Errors = int64(failed)
	jobs.AddLog(job, "info", fmt.Sprintf("reverse geocode started: limit=%d batch=%d radius=%.0fm online=%t media=%s", payload.Limit, payload.BatchSize, payload.RadiusM, payload.Online, payload.MediaKind))
	if err := s.deps.Store.UpdateLeasedJob(ctx, *job, workerID); err != nil {
		return err
	}

	offset := processed + failed
	lastFlush := time.Now().Add(-time.Minute)
	for payload.Limit < 0 || processed+failed < payload.Limit {
		if err := aiBackfillHeartbeatError(heartbeatErrs); err != nil {
			return err
		}
		if job.Status == jobs.StatusCancelRequested {
			return jobs.ErrCanceled
		}
		pageLimit := payload.BatchSize
		if payload.Limit > 0 {
			left := payload.Limit - processed - failed
			if left <= 0 {
				break
			}
			if left < pageLimit {
				pageLimit = left
			}
		}
		geos, err := s.deps.Store.QueryAssetGeo(ctx, catalog.GeoQuery{
			MediaKind: payload.MediaKind,
			Limit:     pageLimit,
			Offset:    offset,
		})
		if err != nil {
			return err
		}
		if len(geos) == 0 {
			break
		}
		for _, geo := range geos {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !validCoordinate(geo.Geo.Lat, geo.Geo.Lon) {
				failed++
				continue
			}
			local := mergePlaceMatches(
				placesForPoint(geo.Geo.Lat, geo.Geo.Lon, places),
				nearbyPlacesForPoint(geo.Geo.Lat, geo.Geo.Lon, places, payload.RadiusM),
			)
			if len(local) > 0 {
				matched++
				processed++
			} else if payload.Online && runtimeBoolSetting("search.online_geocoding", true) {
				place, err := s.reverseGeocodeOnline(ctx, geo.Geo.Lat, geo.Geo.Lon)
				if err != nil {
					failed++
					jobs.AddLog(job, "warn", fmt.Sprintf("online reverse geocode failed for %s %.6f,%.6f: %v", geo.Asset.ID, geo.Geo.Lat, geo.Geo.Lon, err))
				} else {
					created, err := s.deps.Store.UpsertPlace(ctx, place)
					if err != nil {
						failed++
						jobs.AddLog(job, "warn", fmt.Sprintf("cache place failed for %s %.6f,%.6f: %v", geo.Asset.ID, geo.Geo.Lat, geo.Geo.Lon, err))
					} else {
						places = append(places, created)
						cached++
						processed++
					}
				}
			} else {
				processed++
			}
			payload.Processed = processed
			payload.Matched = matched
			payload.Cached = cached
			payload.Failed = failed
			payload.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			job.Payload = payload
			job.ProgressCurrent = int64(processed + failed)
			job.Counters.Scanned = int64(processed + failed)
			job.Counters.Updated = int64(matched + cached)
			job.Counters.Errors = int64(failed)
			if time.Since(lastFlush) >= 5*time.Second {
				lastFlush = time.Now()
				if err := s.deps.Store.UpdateLeasedJob(ctx, *job, workerID); err != nil {
					return err
				}
				if job.Status == jobs.StatusCancelRequested {
					return jobs.ErrCanceled
				}
			}
		}
		offset += len(geos)
		if len(geos) < pageLimit {
			break
		}
	}
	payload.Processed = processed
	payload.Matched = matched
	payload.Cached = cached
	payload.Failed = failed
	payload.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	job.Payload = payload
	job.ProgressCurrent = int64(processed + failed)
	job.Counters.Scanned = int64(processed + failed)
	job.Counters.Updated = int64(matched + cached)
	job.Counters.Errors = int64(failed)
	jobs.AddLog(job, "info", fmt.Sprintf("reverse geocode finished: processed=%d matched=%d cached=%d failed=%d", processed, matched, cached, failed))
	return s.deps.Store.UpdateLeasedJob(ctx, *job, workerID)
}

func validCoordinate(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lon) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}
