package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

func TestMemoryJobRetryClassification(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	transient := jobs.New("hash", nil)
	transient.MaxAttempts = 2
	transient, _ = store.EnqueueJob(ctx, transient)
	leased, err := store.LeaseNextJob(ctx, "worker", []string{"hash"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FailLeasedJob(ctx, leased, "worker", jobs.Transient(fmt.Errorf("temporary"))); err != nil {
		t.Fatal(err)
	}
	afterTransient, err := store.GetJob(ctx, transient.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterTransient.Status != jobs.StatusQueued || afterTransient.NextRunAt == nil {
		t.Fatalf("expected transient retry, got %#v", afterTransient)
	}

	permanent := jobs.New("hash", nil)
	permanent.MaxAttempts = 3
	permanent, _ = store.EnqueueJob(ctx, permanent)
	leasedPermanent, err := store.LeaseNextJob(ctx, "worker", []string{"hash"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FailLeasedJob(ctx, leasedPermanent, "worker", jobs.Permanent(fmt.Errorf("bad input"))); err != nil {
		t.Fatal(err)
	}
	afterPermanent, err := store.GetJob(ctx, permanent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterPermanent.Status != jobs.StatusFailed || afterPermanent.NextRunAt != nil {
		t.Fatalf("expected permanent failure, got %#v", afterPermanent)
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
	filtered, err := BuildExplorerView(assets, ExplorerOptions{Path: "photos", MediaKind: "video"})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.FileCount != 0 || filtered.FolderCount != 0 {
		t.Fatalf("video filter should hide photo folder entries, got %#v", filtered)
	}
}

func TestBuildDuplicateGroups(t *testing.T) {
	now := time.Now().UTC()
	assets := []Asset{
		{
			ID: "a1", DisplayName: "one.jpg", MediaKind: "photo",
			Locations: []Location{{StorageName: "fixture", RelativePath: "one.jpg", StorageURL: "fs://fixture/one.jpg", FileName: "one.jpg", SizeBytes: 10, SHA512Hex: "abc", HashStatus: HashStatusHashed, MTime: now}},
		},
		{
			ID: "a2", DisplayName: "two.jpg", MediaKind: "photo",
			Locations: []Location{{StorageName: "fixture", RelativePath: "two.jpg", StorageURL: "fs://fixture/two.jpg", FileName: "two.jpg", SizeBytes: 10, SHA512Hex: "abc", HashStatus: HashStatusHashed, MTime: now}},
		},
		{
			ID: "a3", DisplayName: "other.jpg", MediaKind: "photo",
			Locations: []Location{{StorageName: "fixture", RelativePath: "other.jpg", StorageURL: "fs://fixture/other.jpg", FileName: "other.jpg", SizeBytes: 11, SHA512Hex: "abc", HashStatus: HashStatusHashed, MTime: now}},
		},
		{
			ID: "a4", DisplayName: "unhashed.jpg", MediaKind: "photo",
			Locations: []Location{{StorageName: "fixture", RelativePath: "unhashed.jpg", StorageURL: "fs://fixture/unhashed.jpg", FileName: "unhashed.jpg", SizeBytes: 10, HashStatus: HashStatusUnhashed, MTime: now}},
		},
	}
	page := BuildDuplicateGroups(assets, 10, 0)
	if page.Page.Total != 1 || len(page.Groups) != 1 {
		t.Fatalf("expected one duplicate group, got %#v", page)
	}
	if page.Groups[0].AssetCount != 2 || page.Groups[0].TotalBytes != 20 {
		t.Fatalf("unexpected group counters %#v", page.Groups[0])
	}
}

func TestMemoryStoreMergesLocationsByHashAndSize(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Now().UTC()
	first, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName: "fixture", StorageURL: "fs://fixture/a/photo.jpg", RelativePath: "a/photo.jpg",
		Name: "photo.jpg", Extension: "jpg", MIME: "image/jpeg", MediaKind: "photo", SizeBytes: 12, MTime: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName: "fixture", StorageURL: "fs://fixture/b/photo.jpg", RelativePath: "b/photo.jpg",
		Name: "photo.jpg", Extension: "jpg", MIME: "image/jpeg", MediaKind: "photo", SizeBytes: 12, MTime: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Asset.ID == second.Asset.ID {
		t.Fatal("pre-hash URL discovery should create separate provisional assets")
	}
	if err := store.UpdateLocationHash(ctx, first.Asset.ID, strings.Repeat("a", 128), 12); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateLocationHash(ctx, second.Asset.ID, strings.Repeat("a", 128), 12); err != nil {
		t.Fatal(err)
	}
	assets, err := store.ListAssets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || len(assets[0].Locations) != 2 {
		t.Fatalf("expected one logical asset with two locations, got %#v", assets)
	}
}
