package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

func TestMemoryAlbumsDoNotDeleteAssets(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	result, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/photos/a.jpg",
		RelativePath: "photos/a.jpg",
		Name:         "a.jpg",
		Extension:    "jpg",
		MIME:         "image/jpeg",
		MediaKind:    "photo",
		SizeBytes:    10,
		MTime:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	album, err := store.CreateAlbum(ctx, Album{Title: "Trip"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddAlbumItems(ctx, album.ID, []string{result.Asset.ID}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAlbum(ctx, album.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAsset(ctx, result.Asset.ID); err != nil {
		t.Fatalf("album deletion removed asset: %v", err)
	}
}

func TestMemoryGeotagPriorityProtectsEXIF(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	result, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/photos/a.jpg",
		RelativePath: "photos/a.jpg",
		Name:         "a.jpg",
		Extension:    "jpg",
		MIME:         "image/jpeg",
		MediaKind:    "photo",
		SizeBytes:    10,
		MTime:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	confidence := 1.0
	if _, err := store.UpsertAssetGeo(ctx, AssetGeo{AssetID: result.Asset.ID, Lat: 40, Lon: 44, Source: "exif", Confidence: &confidence}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAssetGeo(ctx, AssetGeo{AssetID: result.Asset.ID, Lat: 1, Lon: 2, Source: "estimated"}, false); err != nil {
		t.Fatal(err)
	}
	geo, err := store.GetAssetGeo(ctx, result.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if geo.Source != "exif" || geo.Lat != 40 || geo.Lon != 44 {
		t.Fatalf("expected EXIF geotag to be preserved, got %#v", geo)
	}
}

func TestMemoryPreviewCacheIndexAndCleanup(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Now().UTC()
	entry, err := store.UpsertPreviewCacheEntry(ctx, PreviewCacheEntry{
		AssetID:   "asset-1",
		Variant:   "default",
		Width:     256,
		Height:    256,
		Format:    "jpg",
		CachePath: "/tmp/cartolensia-preview.jpg",
		Status:    "ready",
		SizeBytes: 50,
		CreatedAt: now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPreviewAccessed(ctx, entry.ID); err != nil {
		t.Fatal(err)
	}
	stats, err := store.PreviewCacheStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Entries != 1 || stats.Ready != 1 || stats.Bytes != 50 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	deleted, err := store.CleanupPreviewCacheEntries(ctx, now.Add(-time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 {
		t.Fatalf("expected one deleted cache entry, got %#v", deleted)
	}
}
