package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

func TestQueryAIMissingAssetsUsesOutputsAndTaskStatus(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	photo1 := mustInsertTestAsset(t, store, storage.FileInfo{StorageName: "test", StorageURL: "fs://test/a.jpg", RelativePath: "a.jpg", Name: "a.jpg", Extension: "jpg", MediaKind: "photo", MTime: time.Now()})
	photo2 := mustInsertTestAsset(t, store, storage.FileInfo{StorageName: "test", StorageURL: "fs://test/b.jpg", RelativePath: "b.jpg", Name: "b.jpg", Extension: "jpg", MediaKind: "photo", MTime: time.Now()})
	photo3 := mustInsertTestAsset(t, store, storage.FileInfo{StorageName: "test", StorageURL: "fs://test/c.jpg", RelativePath: "c.jpg", Name: "c.jpg", Extension: "jpg", MediaKind: "photo", MTime: time.Now()})
	audio := mustInsertTestAsset(t, store, storage.FileInfo{StorageName: "test", StorageURL: "fs://test/a.mp3", RelativePath: "a.mp3", Name: "a.mp3", Extension: "mp3", MediaKind: "audio", MTime: time.Now()})

	if _, err := store.CreateAIPrediction(ctx, AIPrediction{AssetID: photo1.Asset.ID, Task: "ocr_image", Label: "text"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAIAssetTaskStatus(ctx, AIAssetTaskStatus{AssetID: photo2.Asset.ID, Task: "ocr_image", Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertAudioFeatures(ctx, AudioFeatures{AssetID: audio.Asset.ID}); err != nil {
		t.Fatal(err)
	}

	page, err := store.QueryAIMissingAssets(ctx, AIMissingQuery{Task: "ocr_image", MediaKind: "photo", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Page.Total != 1 || len(page.Assets) != 1 || page.Assets[0].ID != photo3.Asset.ID {
		t.Fatalf("expected only unprocessed photo, got total=%d assets=%v", page.Page.Total, page.Assets)
	}

	excludedPage, err := store.QueryAIMissingAssets(ctx, AIMissingQuery{Task: "ocr_image", MediaKind: "photo", Limit: 10, ExcludeAssetIDs: []string{photo3.Asset.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if excludedPage.Page.Total != 0 || len(excludedPage.Assets) != 0 {
		t.Fatalf("expected excluded missing photo to be skipped, got total=%d assets=%v", excludedPage.Page.Total, excludedPage.Assets)
	}

	audioPage, err := store.QueryAIMissingAssets(ctx, AIMissingQuery{Task: "analyze_audio", MediaKind: "audio", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if audioPage.Page.Total != 0 {
		t.Fatalf("expected completed audio to be excluded, got total=%d", audioPage.Page.Total)
	}
}

func mustInsertTestAsset(t *testing.T, store *MemoryStore, info storage.FileInfo) UpsertResult {
	t.Helper()
	if info.MTime.IsZero() {
		info.MTime = time.Now()
	}
	res, err := store.UpsertDiscoveredFile(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
