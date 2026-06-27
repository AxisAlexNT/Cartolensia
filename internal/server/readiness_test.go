package server

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProbeReadableDirBoundedReportsUnavailableRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "offline")
	started := time.Now()
	if err := probeReadableDirBounded(missing, time.Second); err == nil {
		t.Fatal("expected missing storage root to be reported unavailable")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("missing storage root probe took too long: %s", elapsed)
	}
	if err := probeReadableDirBounded(t.TempDir(), time.Second); err != nil {
		t.Fatalf("expected readable directory to pass readiness probe: %v", err)
	}
}

func TestProbeReadableDirBoundedTimeout(t *testing.T) {
	if err := probeReadableDirBounded(t.TempDir(), 0); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected bounded probe timeout, got %v", err)
	}
}
