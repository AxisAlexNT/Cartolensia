package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

func TestDryRunIsReportOnlyAndBounded(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "photos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "photos", "a.jpg"), []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "photos", "b.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture_dryrun", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	job, err := store.EnqueueJob(context.Background(), jobs.New("discovery_dry_run", DryRunPayload{
		Storage:           "fixture_dryrun",
		Prefixes:          []string{"photos"},
		MaxFiles:          1,
		MaxBytes:          1024,
		IncludeExtensions: []string{"jpg"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateScanRun(context.Background(), catalog.ScanRun{
		JobID:       job.ID,
		StorageName: "fixture_dryrun",
		Mode:        "dry_run",
		Prefixes:    []string{"photos"},
		MaxFiles:    1,
		MaxBytes:    1024,
		DryRun:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := DecodeDryRunPayload(job.Payload)
	payload.ScanRunID = run.ID
	job.Payload = payload
	if err := (Runner{Registry: registry, Store: store}).DryRun(context.Background(), &job); err != nil {
		t.Fatal(err)
	}
	assets, err := store.ListAssets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 {
		t.Fatalf("dry-run should not index assets, got %#v", assets)
	}
	report, err := store.GetScanRunByJob(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Report["files_would_index"] != float64(1) && report.Report["files_would_index"] != 1 {
		t.Fatalf("unexpected dry-run report: %#v", report.Report)
	}
	if report.MarkMissing {
		t.Fatalf("dry-run should not mark missing")
	}
}

func TestDryRunAllowsStorageRootPreview(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"a.jpg", "nested/b.jpg"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, filepath.FromSlash(rel))), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte("dummy"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	registry, err := storage.NewRegistry([]storage.Config{{Name: "fixture_dryrun", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewMemoryStore()
	job, err := store.EnqueueJob(context.Background(), jobs.New("discovery_dry_run", DryRunPayload{
		Storage:           "fixture_dryrun",
		MaxFiles:          50,
		MaxBytes:          1024,
		IncludeExtensions: []string{"jpg"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateScanRun(context.Background(), catalog.ScanRun{
		JobID:       job.ID,
		StorageName: "fixture_dryrun",
		Mode:        "dry_run",
		MaxFiles:    50,
		MaxBytes:    1024,
		DryRun:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := DecodeDryRunPayload(job.Payload)
	payload.ScanRunID = run.ID
	job.Payload = payload
	if err := (Runner{Registry: registry, Store: store}).DryRun(context.Background(), &job); err != nil {
		t.Fatal(err)
	}
	report, err := store.GetScanRunByJob(context.Background(), job.ID)
	if err == nil && report.Report["files_would_index"] != float64(2) && report.Report["files_would_index"] != 2 {
		t.Fatalf("unexpected dry-run report: %#v", report.Report)
	}
	assets, err := store.ListAssets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 {
		t.Fatalf("dry-run should not index assets, got %#v", assets)
	}
}
