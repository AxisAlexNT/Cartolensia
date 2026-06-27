package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

type DryRunPayload struct {
	ScanRunID         string   `json:"scan_run_id,omitempty"`
	Storage           string   `json:"storage"`
	Prefixes          []string `json:"prefixes"`
	MaxFiles          int      `json:"max_files"`
	MaxBytes          int64    `json:"max_bytes"`
	IncludeExtensions []string `json:"include_extensions,omitempty"`
	ExcludePatterns   []string `json:"exclude_patterns,omitempty"`
	Hash              bool     `json:"hash"`
	Metadata          bool     `json:"metadata"`
	Previews          bool     `json:"previews"`
	MarkMissing       bool     `json:"mark_missing"`
	Mode              string   `json:"mode"`
	AllowOverLimit    bool     `json:"allow_over_limit,omitempty"`
}

func NormalizeDryRunPayload(payload DryRunPayload) DryRunPayload {
	payload.Storage = strings.TrimSpace(payload.Storage)
	payload.Prefixes = compactPayloadStrings(payload.Prefixes)
	payload.IncludeExtensions = normalizeExtensions(payload.IncludeExtensions)
	payload.ExcludePatterns = compactPayloadStrings(payload.ExcludePatterns)
	if payload.MaxFiles == 0 {
		payload.MaxFiles = 50
	}
	if payload.MaxFiles < 0 && !payload.AllowOverLimit {
		payload.MaxFiles = 50
	}
	if payload.MaxFiles > 50 && !payload.AllowOverLimit {
		payload.MaxFiles = 50
	}
	if payload.MaxBytes == 0 {
		payload.MaxBytes = 2 << 30
	}
	if payload.Mode == "" {
		payload.Mode = "dry_run"
	}
	payload.MarkMissing = false
	return payload
}

func DecodeDryRunPayload(raw any) DryRunPayload {
	var payload DryRunPayload
	if raw == nil {
		return NormalizeDryRunPayload(payload)
	}
	data, err := json.Marshal(raw)
	if err == nil {
		_ = json.Unmarshal(data, &payload)
	}
	return NormalizeDryRunPayload(payload)
}

func (p DryRunPayload) SafetySummary() map[string]any {
	return map[string]any{
		"strict_read_only_required": true,
		"prefixes_required":         false,
		"empty_prefix_scope":        "storage root",
		"default_max_files":         50,
		"unlimited_requires":        "normal indexing may use -1; dry-run previews stay capped unless allow_over_limit is explicit",
		"default_max_bytes":         int64(2 << 30),
		"mark_missing_allowed":      false,
		"hash_requested":            p.Hash,
		"metadata_requested":        p.Metadata,
		"previews_requested":        p.Previews,
	}
}

func (r Runner) DryRun(ctx context.Context, job *jobs.Job) error {
	if r.Registry == nil || r.Store == nil {
		return jobs.Permanent(fmt.Errorf("dry-run runner is not configured"))
	}
	payload := DecodeDryRunPayload(job.Payload)
	if payload.Storage == "" {
		return jobs.Permanent(fmt.Errorf("dry-run storage is required"))
	}
	if err := jobs.Start(job); err != nil {
		return err
	}
	started := time.Now().UTC()
	jobs.AddLog(job, "info", fmt.Sprintf("dry-run scan started for storage %s", payload.Storage))
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	adapter, err := r.Registry.Adapter(payload.Storage)
	if err != nil {
		cause := jobs.Permanent(err)
		_ = r.failJob(ctx, *job, cause)
		return cause
	}
	files, walkReport, err := adapter.ListRecursiveBounded(ctx, storage.WalkOptions{
		Prefixes:          payload.Prefixes,
		MaxFiles:          payload.MaxFiles,
		MaxBytes:          payload.MaxBytes,
		MaxFolderWorkers:  r.MaxFolderWorkers,
		MaxFileWorkers:    r.MaxFileWorkers,
		FolderQueueDepth:  r.FolderQueueDepth,
		IncludeExtensions: payload.IncludeExtensions,
		ExcludePatterns:   payload.ExcludePatterns,
	})
	if err != nil {
		cause := classifyStorageFailure(err)
		_ = r.failJob(ctx, *job, cause)
		return cause
	}
	total := int64(len(files))
	job.ProgressTotal = &total
	job.ProgressCurrent = total
	job.Counters.Scanned = int64(walkReport.FilesSeen)
	job.Counters.Bytes = walkReport.BytesSeen
	if err := r.checkCanceled(ctx, job); err != nil {
		return err
	}
	report := buildDryRunReport(payload, files, walkReport, started)
	if payload.ScanRunID != "" {
		if err := r.Store.FinishScanRun(ctx, payload.ScanRunID, report); err != nil {
			return err
		}
	} else if run, err := r.Store.GetScanRunByJob(ctx, job.ID); err == nil {
		if err := r.Store.FinishScanRun(ctx, run.ID, report); err != nil {
			return err
		}
	}
	jobs.AddLog(job, "info", fmt.Sprintf("dry-run scan completed: %d files would be considered", len(files)))
	if err := r.updateJob(ctx, *job); err != nil {
		return err
	}
	return r.completeJob(ctx, *job)
}

func buildDryRunReport(payload DryRunPayload, files []storage.FileInfo, walk storage.WalkReport, started time.Time) map[string]any {
	mediaKinds := map[string]int{}
	extensions := map[string]int{}
	var bytesIndexable int64
	for _, file := range files {
		mediaKinds[file.MediaKind]++
		extensions[file.Extension]++
		bytesIndexable += file.SizeBytes
	}
	return map[string]any{
		"storage":                           payload.Storage,
		"mode":                              payload.Mode,
		"prefixes":                          payload.Prefixes,
		"max_files":                         payload.MaxFiles,
		"max_bytes":                         payload.MaxBytes,
		"hash_requested":                    payload.Hash,
		"metadata_requested":                payload.Metadata,
		"previews_requested":                payload.Previews,
		"mark_missing":                      false,
		"dry_run":                           true,
		"files_seen":                        walk.FilesSeen,
		"files_indexed":                     0,
		"files_would_index":                 len(files),
		"files_skipped":                     walk.FilesSkipped,
		"bytes_scanned":                     walk.BytesSeen,
		"bytes_would_index":                 bytesIndexable,
		"media_kinds":                       mediaKinds,
		"top_extensions":                    topExtensionCounts(extensions, 20),
		"skipped_reasons":                   walk.SkippedReasons,
		"safety":                            payload.SafetySummary(),
		"scan_complete":                     walk.Complete,
		"missing_candidates":                0,
		"missing_marked":                    0,
		"skipped_missing_due_to_incomplete": !walk.Complete,
		"started_at":                        started.Format(time.RFC3339),
		"finished_at":                       time.Now().UTC().Format(time.RFC3339),
	}
}

func compactPayloadStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeExtensions(values []string) []string {
	values = compactPayloadStrings(values)
	for i := range values {
		values[i] = strings.TrimPrefix(strings.ToLower(values[i]), ".")
	}
	sort.Strings(values)
	return values
}

func topExtensionCounts(counts map[string]int, limit int) []map[string]any {
	type pair struct {
		Ext   string
		Count int
	}
	pairs := make([]pair, 0, len(counts))
	for ext, count := range counts {
		pairs = append(pairs, pair{Ext: ext, Count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Count == pairs[j].Count {
			return pairs[i].Ext < pairs[j].Ext
		}
		return pairs[i].Count > pairs[j].Count
	})
	if limit <= 0 || limit > len(pairs) {
		limit = len(pairs)
	}
	out := make([]map[string]any, 0, limit)
	for _, pair := range pairs[:limit] {
		out = append(out, map[string]any{"extension": pair.Ext, "count": pair.Count})
	}
	return out
}
