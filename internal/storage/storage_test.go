package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	if err := adapter.Write("new.jpg", strings.NewReader("data")); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only write error, got %v", err)
	}
	if err := adapter.Move("photo.jpg", "moved.jpg"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only move error, got %v", err)
	}
	if err := adapter.Mkdir("folder"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only mkdir error, got %v", err)
	}
	if _, _, err := adapter.Open("../outside"); !errors.Is(err, ErrTraversal) {
		t.Fatalf("expected traversal error, got %v", err)
	}
}

func TestFSAdapterSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("dummy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.jpg"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.jpg"), filepath.Join(root, "linked.jpg")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	adapter, err := NewFSAdapter("fixture", root)
	if err != nil {
		t.Fatal(err)
	}
	files, err := adapter.ListRecursive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].RelativePath != "photo.jpg" {
		t.Fatalf("expected only real file, got %#v", files)
	}
	if _, _, err := adapter.Open("linked.jpg"); !errors.Is(err, ErrTraversal) {
		t.Fatalf("expected traversal through symlink, got %v", err)
	}
}

func TestParseURLRejectsEncodedTraversal(t *testing.T) {
	if _, err := ParseURL("fs://fixture/photos/%2e%2e/secret.jpg"); !errors.Is(err, ErrTraversal) {
		t.Fatalf("expected traversal error, got %v", err)
	}
}
