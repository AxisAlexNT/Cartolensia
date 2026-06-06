package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHLSArgsProfiles(t *testing.T) {
	dir := t.TempDir()
	args, err := hlsArgs("h264_low_bitrate", "/tmp/input.mp4", dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, arg := range args {
		joined += arg + " "
	}
	if !strings.Contains(joined, "libx264") || !strings.Contains(joined, "master.m3u8") || !strings.Contains(joined, "hls") {
		t.Fatalf("unexpected hls args: %s", joined)
	}
	if _, err := hlsArgs("unknown", "/tmp/input.mp4", dir); err == nil {
		t.Fatal("expected unsupported profile error")
	}
}

func TestHLSReadyRequiresPlaylistAndSegment(t *testing.T) {
	dir := t.TempDir()
	if hlsReady(dir) {
		t.Fatal("empty directory should not be ready")
	}
	if err := os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte("#EXTM3U\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if hlsReady(dir) {
		t.Fatal("playlist without segment should not be ready")
	}
	if err := os.WriteFile(filepath.Join(dir, "segment_00000.ts"), []byte("segment"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hlsReady(dir) {
		t.Fatal("playlist and segment should be ready")
	}
}
