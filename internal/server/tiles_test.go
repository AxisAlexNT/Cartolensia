package server

import (
	"path/filepath"
	"testing"
)

func TestParseTileRequestValidation(t *testing.T) {
	source, z, x, y, err := parseTileRequest([]string{"osm", "3", "4", "2.png"})
	if err != nil {
		t.Fatal(err)
	}
	if source != "osm" || z != 3 || x != 4 || y != 2 {
		t.Fatalf("unexpected parsed tile %s %d %d %d", source, z, x, y)
	}
	bad := [][]string{
		{"bad", "3", "4", "2.png"},
		{"osm", "-1", "0", "0.png"},
		{"osm", "20", "0", "0.png"},
		{"osm", "1", "2", "0.png"},
		{"osm", "1", "0", "2.png"},
		{"osm", "1", "0", "0.jpg"},
		{"osm", "1", "0", "../0.png"},
	}
	for _, parts := range bad {
		if _, _, _, _, err := parseTileRequest(parts); err == nil {
			t.Fatalf("expected invalid tile request for %#v", parts)
		}
	}
}

func TestSafeTileCachePath(t *testing.T) {
	root := t.TempDir()
	target, err := safeTileCachePath(root, "osm", 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "osm", "1", "0", "0.png")
	if target != want {
		t.Fatalf("unexpected cache path %q, want %q", target, want)
	}
}
