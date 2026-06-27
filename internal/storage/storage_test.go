package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func TestFSAdapterAllowsUnavailableRootAtStartup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-mounted")
	reg, err := NewRegistry([]Config{{Name: "offline", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatalf("offline storage root should not prevent startup: %v", err)
	}
	cfg, err := reg.GetStorage("offline")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Root != filepath.Clean(root) {
		t.Fatalf("expected clean configured root to be preserved, got %q", cfg.Root)
	}
	adapter, err := reg.Adapter("offline")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Stat("photo.jpg"); err == nil {
		t.Fatal("expected file access against unavailable root to fail")
	}
}

func TestNonFatalSymlinkResolutionErrorIncludesOfflineMountErrors(t *testing.T) {
	for _, err := range []error{
		fs.ErrNotExist,
		syscall.ENODEV,
		syscall.ENOTCONN,
		syscall.EHOSTDOWN,
		syscall.EHOSTUNREACH,
		syscall.ETIMEDOUT,
		syscall.EIO,
	} {
		if !nonFatalSymlinkResolutionError(err) {
			t.Fatalf("expected %v to be non-fatal during storage initialization", err)
		}
	}
	if nonFatalSymlinkResolutionError(syscall.EPERM) {
		t.Fatal("permission errors should remain fatal during symlink resolution")
	}
}

func TestFSAdapterWalkRecursiveBoundedStreamsAndCancels(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album", "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"album/one.jpg", "album/two.jpg", "album/nested/three.jpg"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte("dummy"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	adapter, err := NewFSAdapter("fixture", root)
	if err != nil {
		t.Fatal(err)
	}
	stopErr := errors.New("stop after first streamed file")
	visited := 0
	report, err := adapter.WalkRecursiveBounded(context.Background(), WalkOptions{
		Prefixes:          []string{"album"},
		MaxFiles:          -1,
		MaxBytes:          -1,
		MaxFolderWorkers:  2,
		MaxFileWorkers:    2,
		FolderQueueDepth:  4,
		IncludeExtensions: []string{"jpg"},
	}, func(FileInfo) error {
		visited++
		return stopErr
	})
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected callback error, got %v", err)
	}
	if visited == 0 {
		t.Fatal("expected streaming callback to be called")
	}
	if report.Complete {
		t.Fatal("expected incomplete report after callback cancellation")
	}
}

func TestFSAdapterWalkRecursiveBoundedHandlesSaturatedFolderQueue(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 12; i++ {
		dir := filepath.Join(root, "wide", "child-"+string(rune('a'+i)))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("dummy"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	adapter, err := NewFSAdapter("fixture", root)
	if err != nil {
		t.Fatal(err)
	}
	visited := 0
	report, err := adapter.WalkRecursiveBounded(context.Background(), WalkOptions{
		Prefixes:          []string{"wide"},
		MaxFiles:          -1,
		MaxBytes:          -1,
		MaxFolderWorkers:  1,
		MaxFileWorkers:    1,
		FolderQueueDepth:  1,
		IncludeExtensions: []string{"jpg"},
	}, func(FileInfo) error {
		visited++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if visited != 12 {
		t.Fatalf("expected all files to be visited, got %d", visited)
	}
	if report.FilesReturned != 12 || !report.Complete {
		t.Fatalf("unexpected walk report: %#v", report)
	}
}

func TestFSAdapterWalkRecursiveBoundedPrunesExcludedSubtrees(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"keep/a.jpg",
		"skip/b.jpg",
		"skip/nested/c.jpg",
		"also-skip/d.jpg",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, filepath.FromSlash(rel))), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte("dummy"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	adapter, err := NewFSAdapter("fixture", root)
	if err != nil {
		t.Fatal(err)
	}
	var visited []string
	report, err := adapter.WalkRecursiveBounded(context.Background(), WalkOptions{
		MaxFiles:          -1,
		MaxBytes:          -1,
		MaxFolderWorkers:  2,
		MaxFileWorkers:    2,
		FolderQueueDepth:  4,
		IncludeExtensions: []string{"jpg"},
		ExcludePatterns:   []string{"skip/**", "also-skip"},
	}, func(info FileInfo) error {
		visited = append(visited, info.RelativePath)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(visited) != 1 || visited[0] != "keep/a.jpg" {
		t.Fatalf("expected only keep/a.jpg, got %#v", visited)
	}
	if report.SkippedReasons["pattern"] == 0 {
		t.Fatalf("expected pattern skips in report: %#v", report)
	}
}

func TestParseURLRejectsEncodedTraversal(t *testing.T) {
	if _, err := ParseURL("fs://fixture/photos/%2e%2e/secret.jpg"); !errors.Is(err, ErrTraversal) {
		t.Fatalf("expected traversal error, got %v", err)
	}
}

func TestRegistryAddUpdateAndValidateStorage(t *testing.T) {
	root := t.TempDir()
	reg, err := NewRegistry([]Config{{Name: "fixture", Kind: "fs", Root: root, Mode: "strict_read_only"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.ListStorages()[0]; got.Mode != "strict_read_only" || got.Root == "" {
		t.Fatalf("unexpected initial storage %#v", got)
	}

	second := t.TempDir()
	added, err := reg.AddStorage(Config{Name: "synthetic", Kind: "fs", Root: second, Mode: "read_only"})
	if err != nil {
		t.Fatal(err)
	}
	if added.Mode != "read_only" {
		t.Fatalf("expected read_only mode, got %#v", added)
	}
	if _, err := reg.Adapter("synthetic"); err != nil {
		t.Fatalf("expected adapter for added storage: %v", err)
	}

	updated, err := reg.UpdateStorage("synthetic", Config{Kind: "fs", Root: second, Mode: "strict_read_only"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "synthetic" || updated.Mode != "strict_read_only" {
		t.Fatalf("unexpected update %#v", updated)
	}

	if _, err := ValidateConfig(Config{Name: "bad", Kind: "s3", Root: second, Mode: "strict_read_only"}); err == nil {
		t.Fatal("expected unsupported kind error")
	}
	if _, err := ValidateConfig(Config{Name: "bad", Kind: "fs", Root: second, Mode: "read_write"}); err == nil {
		t.Fatal("expected disabled write mode error")
	}
}
