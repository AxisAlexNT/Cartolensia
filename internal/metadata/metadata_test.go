package metadata

import (
	"context"
	"path/filepath"
	"testing"

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
