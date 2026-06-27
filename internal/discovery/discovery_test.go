package discovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	if len(assets) != 5 {
		t.Fatalf("expected 5 media assets, got %d: %#v", len(assets), assets)
	}
	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Unhashed != 5 || stats.Hashed != 0 || stats.Documents != 1 {
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
	if stats.Hashed != 5 || stats.Unhashed != 0 || stats.Documents != 1 {
		t.Fatalf("unexpected hash stats %#v", stats)
	}
}

func TestHashBoundedPrefix(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "photos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"photos/a.jpg", "photos/b.jpg", "other/c.jpg"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	runner := Runner{Registry: registry, Store: store}
	scanJob, _ := store.EnqueueJob(context.Background(), jobs.New("discovery", ScanPayload{
		Storage:  "fixture",
		Prefixes: []string{"photos"},
		MaxFiles: 2,
		MaxBytes: 1024,
	}))
	if err := runner.Scan(context.Background(), &scanJob); err != nil {
		t.Fatal(err)
	}
	hashJob, _ := store.EnqueueJob(context.Background(), jobs.New("hash", HashPayload{
		Storage:  "fixture",
		Prefixes: []string{"photos"},
		MaxFiles: 1,
	}))
	if err := runner.HashUnhashed(context.Background(), &hashJob); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Hashed != 1 || stats.Unhashed != 1 {
		t.Fatalf("expected one bounded hash and one remaining unhashed asset, got %#v", stats)
	}
}

func TestDiscoveryAllowsExplicitStorageRootScan(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "photos"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root.jpg", "photos/nested.jpg"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	runner := Runner{Registry: registry, Store: store}
	scanJob, _ := store.EnqueueJob(context.Background(), jobs.New("discovery", ScanPayload{
		Storage:  "fixture",
		MaxFiles: -1,
		MaxBytes: -1,
	}))
	if err := runner.Scan(context.Background(), &scanJob); err != nil {
		t.Fatal(err)
	}
	assets, err := store.ListAssets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected storage-root scan to index both files, got %d: %#v", len(assets), assets)
	}
}

func TestDiscoveryExcludesNestedStorageWhenScanningAll(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"root.jpg", "child/nested.jpg"} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(rel), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	registry, err := storage.NewRegistry([]storage.Config{
		{Name: "parent", Kind: "fs", Root: root, Mode: "strict_read_only"},
		{Name: "child", Kind: "fs", Root: filepath.Join(root, "child"), Mode: "strict_read_only"},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	runner := Runner{Registry: registry, Store: store}
	scanJob, _ := store.EnqueueJob(context.Background(), jobs.New("discovery", ScanPayload{
		Storage:  "all",
		MaxFiles: -1,
		MaxBytes: -1,
	}))
	if err := runner.Scan(context.Background(), &scanJob); err != nil {
		t.Fatal(err)
	}
	assets, err := store.ListAssets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected parent root file and child file once, got %d: %#v", len(assets), assets)
	}
	seen := map[string]bool{}
	for _, asset := range assets {
		loc, ok := catalog.FirstLocation(asset)
		if !ok {
			t.Fatalf("asset has no location: %#v", asset)
		}
		seen[loc.StorageURL] = true
	}
	if seen["fs://parent/child/nested.jpg"] {
		t.Fatalf("parent scan should exclude child storage subtree, got locations %#v", seen)
	}
	if !seen["fs://parent/root.jpg"] || !seen["fs://child/nested.jpg"] {
		t.Fatalf("expected parent root and child storage locations, got %#v", seen)
	}
}

func TestRealArchiveHashGuard(t *testing.T) {
	registry, err := storage.NewRegistry([]storage.Config{{
		Name: "rclone_peek",
		Kind: "fs",
		Root: "/mnt/Models/rclone",
		Mode: "strict_read_only",
	}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	job, err := store.EnqueueJob(context.Background(), jobs.New("hash", map[string]any{"scope": "unhashed"}))
	if err != nil {
		t.Fatal(err)
	}
	err = (Runner{Registry: registry, Store: store}).HashUnhashed(context.Background(), &job)
	if err == nil || !strings.Contains(err.Error(), "requires explicit selected assets") {
		t.Fatalf("expected real archive hash guard, got %v", err)
	}
	stored, err := store.GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != jobs.StatusFailed {
		t.Fatalf("unsafe hash should be persisted failed, got %s", stored.Status)
	}
}

func TestRealArchiveDiscoveryGuard(t *testing.T) {
	adapter, err := storage.NewFSAdapter("rclone_peek", "/mnt/Models/rclone")
	if err != nil {
		t.Fatal(err)
	}
	prefixes, err := normalizeScanPrefixes(adapter, []string{"/mnt/Models/rclone/Cartolensia-photos/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prefixes) != 1 || prefixes[0] != "Cartolensia-photos" {
		t.Fatalf("absolute prefix was not normalized safely: %#v", prefixes)
	}
	if _, err := normalizeScanPrefixes(adapter, []string{"/mnt/Models/rclone"}); !errors.Is(err, storage.ErrTraversal) {
		t.Fatalf("expected archive root prefix rejection, got %v", err)
	}
	for _, prefix := range []string{"", ".", "/", ".."} {
		if _, err := normalizeScanPrefixes(adapter, []string{prefix}); !errors.Is(err, storage.ErrTraversal) {
			t.Fatalf("%q: expected traversal rejection, got %v", prefix, err)
		}
	}
	if err := validateRealArchiveScanPayload(ScanPayload{
		Storage:  "rclone_peek",
		Prefixes: []string{"Cartolensia-photos"},
		MaxFiles: 50,
		MaxBytes: 2 << 30,
	}); err != nil {
		t.Fatalf("adapter-relative bounded payload should be accepted: %v", err)
	}
	for name, payload := range map[string]ScanPayload{
		"no_files": {Storage: "rclone_peek", Prefixes: []string{"Cartolensia-photos"}, MaxBytes: 2 << 30},
		"no_bytes": {Storage: "rclone_peek", Prefixes: []string{"Cartolensia-photos"}, MaxFiles: 50},
	} {
		if err := validateRealArchiveScanPayload(payload); err == nil {
			t.Fatalf("%s: expected guard rejection", name)
		}
	}
}

func TestRealArchiveStorageAllJobFailsBeforeWalk(t *testing.T) {
	registry, err := storage.NewRegistry([]storage.Config{{
		Name: "rclone_peek",
		Kind: "fs",
		Root: "/mnt/Models/rclone",
		Mode: "strict_read_only",
	}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	job, err := store.EnqueueJob(context.Background(), jobs.New("discovery", map[string]any{"storage": "all"}))
	if err != nil {
		t.Fatal(err)
	}
	runner := Runner{Registry: registry, Store: store}
	err = runner.Scan(context.Background(), &job)
	if err == nil || !strings.Contains(err.Error(), "requires explicit max_files") {
		t.Fatalf("expected real archive max_files guard failure, got %v", err)
	}
	assets, err := store.ListAssets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 {
		t.Fatalf("unsafe job should not index assets: %#v", assets)
	}
	stored, err := store.GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != jobs.StatusFailed {
		t.Fatalf("unsafe job should be persisted failed, got %s", stored.Status)
	}
}
