package preview

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
)

func TestGenerateImagePreviewWritesInsideCache(t *testing.T) {
	cacheDir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	asset := catalog.Asset{
		ID:          "asset-1",
		MediaKind:   "photo",
		DisplayName: "synthetic.png",
		FirstSeenAt: time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		Locations: []catalog.Location{{
			ID:         "loc-1",
			AssetID:    "asset-1",
			MediaKind:  "photo",
			FileName:   "synthetic.png",
			StorageURL: "fs://fixture/synthetic.png",
		}},
	}
	info, err := GenerateImage(context.Background(), cacheDir, asset, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != StatusReady {
		t.Fatalf("expected ready preview, got %#v", info)
	}
	if _, err := os.Stat(info.CachePath); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(cacheDir, info.CachePath)
	if err != nil {
		t.Fatal(err)
	}
	if rel == ".." || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("preview escaped cache dir: %s", info.CachePath)
	}
	status := InfoForAsset(cacheDir, asset)
	if status.Status != StatusReady {
		t.Fatalf("expected cached preview status, got %#v", status)
	}
}

func TestGenerateImagePreviewUnsupportedFormat(t *testing.T) {
	asset := catalog.Asset{ID: "asset-1", MediaKind: "photo", Locations: []catalog.Location{{MediaKind: "photo"}}}
	info, err := GenerateImage(context.Background(), t.TempDir(), asset, bytes.NewReader([]byte("not an image")))
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != StatusUnsupported {
		t.Fatalf("expected unsupported, got %#v", info)
	}
}

func TestCleanupStaysInsideCache(t *testing.T) {
	cacheDir := t.TempDir()
	previewDir := filepath.Join(cacheDir, "previews", "aa")
	if err := os.MkdirAll(previewDir, 0o700); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(previewDir, "old.jpg")
	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	removed, err := Cleanup(cacheDir, 0, time.Hour, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 {
		t.Fatalf("expected one removed preview, got %#v", removed)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected old preview removed, stat err=%v", err)
	}
}
