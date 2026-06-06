package discovery

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

func TestDiscoveryScansFixture(t *testing.T) {
	root, err := filepath.Abs("../../testdata/media_fixture")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	job, err := store.EnqueueJob(context.Background(), jobs.New("discovery", nil))
	if err != nil {
		t.Fatal(err)
	}
	runner := Runner{Registry: registry, Store: store}
	if err := runner.Scan(context.Background(), &job); err != nil {
		t.Fatal(err)
	}
	assets, err := store.ListAssets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 4 {
		t.Fatalf("expected 4 media assets, got %d: %#v", len(assets), assets)
	}
	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Unhashed != 4 || stats.Hashed != 0 {
		t.Fatalf("unexpected hash stats %#v", stats)
	}
}

func TestHashUnhashedFixture(t *testing.T) {
	root, err := filepath.Abs("../../testdata/media_fixture")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	runner := Runner{Registry: registry, Store: store}
	discoveryJob, _ := store.EnqueueJob(context.Background(), jobs.New("discovery", nil))
	if err := runner.Scan(context.Background(), &discoveryJob); err != nil {
		t.Fatal(err)
	}
	hashJob, _ := store.EnqueueJob(context.Background(), jobs.New("hash", nil))
	if err := runner.HashUnhashed(context.Background(), &hashJob); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Hashed != 4 || stats.Unhashed != 0 {
		t.Fatalf("unexpected hash stats %#v", stats)
	}
}
