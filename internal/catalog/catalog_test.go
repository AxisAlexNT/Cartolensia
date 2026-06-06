package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

func TestMemoryJobLeaseOwnershipAndExpiry(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	queued, err := store.EnqueueJob(ctx, jobs.New("hash", nil))
	if err != nil {
		t.Fatal(err)
	}
	leased, err := store.LeaseNextJob(ctx, "worker-a", []string{"hash"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if leased.ID != queued.ID || leased.Status != jobs.StatusRunning || leased.WorkerID != "worker-a" {
		t.Fatalf("unexpected lease: %#v", leased)
	}
	if err := store.CompleteLeasedJob(ctx, leased, "worker-b"); !errors.Is(err, ErrJobLeaseLost) {
		t.Fatalf("expected lease lost, got %v", err)
	}
	if _, err := store.ReleaseExpiredLeases(ctx, time.Now().UTC().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	released, err := store.GetJob(ctx, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if released.Status != jobs.StatusQueued {
		t.Fatalf("expected queued after expired lease, got %#v", released)
	}
	released.NextRunAt = nil
	if err := store.UpdateJob(ctx, released); err != nil {
		t.Fatal(err)
	}
	leasedAgain, err := store.LeaseNextJob(ctx, "worker-b", []string{"hash"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteLeasedJob(ctx, leased, "worker-a"); !errors.Is(err, ErrJobLeaseLost) {
		t.Fatalf("stale worker completed job: %v", err)
	}
	if err := store.CompleteLeasedJob(ctx, leasedAgain, "worker-b"); err != nil {
		t.Fatal(err)
	}
	done, err := store.GetJob(ctx, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != jobs.StatusSucceeded {
		t.Fatalf("expected success, got %#v", done)
	}
}

func TestMemoryJobCancellation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	queued, _ := store.EnqueueJob(ctx, jobs.New("discovery", nil))
	leased, err := store.LeaseNextJob(ctx, "worker-a", nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.RequestCancelJob(ctx, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != jobs.StatusCancelRequested {
		t.Fatalf("expected cancel_requested, got %#v", cancelled)
	}
	if err := store.CancelLeasedJob(ctx, leased, "worker-a"); err != nil {
		t.Fatal(err)
	}
	final, err := store.GetJob(ctx, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != jobs.StatusCanceled {
		t.Fatalf("expected canceled, got %#v", final)
	}
}

func TestBuildExplorerViewGroupsFolders(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	files := []storage.FileInfo{
		{StorageName: "fixture", StorageURL: "fs://fixture/photos/one.jpg", RelativePath: "photos/one.jpg", Name: "one.jpg", MediaKind: "photo", SizeBytes: 10, MTime: time.Unix(10, 0).UTC()},
		{StorageName: "fixture", StorageURL: "fs://fixture/videos/clip.mp4", RelativePath: "videos/clip.mp4", Name: "clip.mp4", MediaKind: "video", SizeBytes: 20, MTime: time.Unix(20, 0).UTC()},
	}
	for _, file := range files {
		if _, err := store.UpsertDiscoveredFile(ctx, file); err != nil {
			t.Fatal(err)
		}
	}
	assets, err := store.ListAssets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root, err := BuildExplorerView(assets, ExplorerOptions{Path: ""})
	if err != nil {
		t.Fatal(err)
	}
	if root.FolderCount != 2 || root.FileCount != 0 {
		t.Fatalf("unexpected root view: %#v", root)
	}
	photos, err := BuildExplorerView(assets, ExplorerOptions{Path: "photos"})
	if err != nil {
		t.Fatal(err)
	}
	if photos.FileCount != 1 || photos.Files[0].Name != "one.jpg" {
		t.Fatalf("unexpected photos view: %#v", photos)
	}
}
