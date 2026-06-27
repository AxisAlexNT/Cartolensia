package server

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/storage"
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

func TestEffectiveSMBConfigParsesSourceURL(t *testing.T) {
	cfg := storage.Config{SourceURL: "smb://nas.local/share/path/to/photos"}
	smb := effectiveSMBConfig(cfg)
	if smb == nil || smb.Host != "nas.local" || smb.Share != "share" || smb.Path != "path/to/photos" {
		t.Fatalf("unexpected SMB config %#v", smb)
	}
}

func TestClassifyMountBackedStorageError(t *testing.T) {
	if code, _ := classifyMountBackedStorageError(os.ErrPermission); code != "credentials_or_permission_denied" {
		t.Fatalf("expected credentials/permission code, got %s", code)
	}
	if code, _ := classifyMountBackedStorageError(os.ErrNotExist); code != "export_or_path_unavailable" {
		t.Fatalf("expected export/path code, got %s", code)
	}
	if code, _ := classifyMountBackedStorageError(errors.New("stat /mnt/share: no such device")); code != "export_or_mount_unavailable" {
		t.Fatalf("expected mount unavailable code, got %s", code)
	}
	if code, _ := classifyMountBackedStorageError(syscall.ENOTCONN); code != "export_or_mount_unavailable" {
		t.Fatalf("expected ENOTCONN to classify as mount unavailable, got %s", code)
	}
}

func TestProbeSMBShareWithClientReportsMissingCredentialsFile(t *testing.T) {
	diag, ok := probeSMBShareWithClient(storage.SMBConfig{
		Host:            "nas.local",
		Share:           "media",
		CredentialsFile: filepath.Join(t.TempDir(), "missing.credentials"),
	}, time.Second)
	if !ok {
		t.Fatal("expected missing credentials file to be classified without smbclient")
	}
	if diag.Code != "credentials_file_missing" {
		t.Fatalf("expected credentials_file_missing, got %#v", diag)
	}
}
