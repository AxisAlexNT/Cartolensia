package metadata

import (
	"archive/zip"
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/discovery"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

func TestMetadataEnrichmentParsesFixtureGPX(t *testing.T) {
	root, err := filepath.Abs("../../testdata/media_fixture")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	scanJob, err := store.EnqueueJob(context.Background(), jobs.New("discovery", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := (discovery.Runner{Registry: registry, Store: store}).Scan(context.Background(), &scanJob); err != nil {
		t.Fatal(err)
	}
	enrichJob, err := store.EnqueueJob(context.Background(), jobs.New("metadata_enrich", NewPayload()))
	if err != nil {
		t.Fatal(err)
	}
	if err := (Runner{Registry: registry, Store: store}).Enrich(context.Background(), &enrichJob); err != nil {
		t.Fatal(err)
	}
	tracks, err := store.ListTracks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].PointCount != 3 || tracks[0].DistanceM <= 0 {
		t.Fatalf("unexpected tracks: %#v", tracks)
	}
}

func TestMetadataEnrichmentScopesQueryByStoragePrefix(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	now := time.Now().UTC()
	keep, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/keep/a.md",
		RelativePath: "keep/a.md",
		Name:         "a.md",
		Extension:    "md",
		MIME:         "text/markdown",
		MediaKind:    "document",
		SizeBytes:    12,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	skip, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/skip/b.md",
		RelativePath: "skip/b.md",
		Name:         "b.md",
		Extension:    "md",
		MIME:         "text/markdown",
		MediaKind:    "document",
		SizeBytes:    12,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := NewPayload()
	payload.Storage = "fixture"
	payload.Prefixes = []string{"keep"}
	enrichJob, err := store.EnqueueJob(ctx, jobs.New("metadata_enrich", payload))
	if err != nil {
		t.Fatal(err)
	}
	if err := (Runner{Registry: registry, Store: store}).Enrich(ctx, &enrichJob); err != nil {
		t.Fatal(err)
	}
	kept, err := store.GetAsset(ctx, keep.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if kept.Metadata["document_metadata_extracted_at"] == nil {
		t.Fatalf("expected matching prefix document to be enriched: %#v", kept.Metadata)
	}
	skipped, err := store.GetAsset(ctx, skip.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if skipped.Metadata["document_metadata_extracted_at"] != nil {
		t.Fatalf("expected non-matching prefix document to remain untouched: %#v", skipped.Metadata)
	}
	if enrichJob.Counters.Updated != 1 || enrichJob.Counters.Scanned != 1 {
		t.Fatalf("expected one scoped update, got counters %#v", enrichJob.Counters)
	}
}

func TestParseGPZBytesWithKML(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("track/doc.kml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(`<kml><Placemark><LineString><coordinates>44,40 45,41</coordinates></LineString></Placemark></kml>`)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	points, err := parseGPZBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("expected gpz kml points, got %#v", points)
	}
}

func TestEnsureTrackPointTimesSynthesizesNoTimeGeometry(t *testing.T) {
	points, synthetic := ensureTrackPointTimes([]catalog.TrackPoint{{Lat: 40, Lon: 44}, {Lat: 41, Lon: 45}}, catalog.Asset{}.FirstSeenAt)
	if !synthetic || len(points) != 2 || points[0].RecordedAt.IsZero() || !points[1].RecordedAt.After(points[0].RecordedAt) {
		t.Fatalf("expected synthetic monotonic timestamps, got synthetic=%v points=%#v", synthetic, points)
	}
}
