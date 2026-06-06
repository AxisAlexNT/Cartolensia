package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStorageURLRoundTrip(t *testing.T) {
	u, err := ParseURL("fs://fixture/photos/2024-trip/photo_001.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if u.Storage != "fixture" || u.RelativePath != "photos/2024-trip/photo_001.jpg" {
		t.Fatalf("unexpected parsed url %#v", u)
	}
	if got := u.String(); got != "fs://fixture/photos/2024-trip/photo_001.jpg" {
		t.Fatalf("unexpected normalized url %q", got)
	}
}

func TestNormalizeRejectsTraversal(t *testing.T) {
	bad := []string{"../x", "/abs", "a/../../x", ""}
	for _, input := range bad {
		if _, err := NormalizeRelativePath(input); !errors.Is(err, ErrTraversal) {
			t.Fatalf("%q: expected traversal error, got %v", input, err)
		}
	}
}

func TestFSAdapterReadOnlyAndList(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter, err := NewFSAdapter("fixture", root)
	if err != nil {
		t.Fatal(err)
	}
	files, err := adapter.ListRecursive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].StorageURL != "fs://fixture/photo.jpg" {
		t.Fatalf("unexpected files %#v", files)
	}
	if err := adapter.Delete("photo.jpg"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only delete error, got %v", err)
	}
	if _, _, err := adapter.Open("../outside"); !errors.Is(err, ErrTraversal) {
		t.Fatalf("expected traversal error, got %v", err)
	}
}
