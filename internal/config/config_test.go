package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenConfigMissing(t *testing.T) {
	t.Setenv("CARTOLENSIA_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Fatalf("unexpected addr %q", cfg.HTTP.Addr)
	}
	if len(cfg.Storages) != 1 || cfg.Storages[0].Name != "fixture" {
		t.Fatalf("unexpected storages %#v", cfg.Storages)
	}
}

func TestLoadYAMLAndEnvOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cartolensia.yaml")
	data := []byte("http:\n  addr: ':9000'\ndatabase:\n  url: postgres://example\nstorages:\n  - name: fixture\n    kind: fs\n    root: ./testdata/media_fixture\n    mode: strict_read_only\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CARTOLENSIA_HTTP_ADDR", ":9100")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != ":9100" {
		t.Fatalf("env override did not apply: %q", cfg.HTTP.Addr)
	}
	if cfg.Database.URL != "postgres://example" {
		t.Fatalf("unexpected db url %q", cfg.Database.URL)
	}
}

func TestValidateRejectsDuplicateStorage(t *testing.T) {
	cfg := Defaults()
	cfg.Storages = append(cfg.Storages, cfg.Storages[0])
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected duplicate storage error")
	}
}

func TestValidateRejectsWriteMode(t *testing.T) {
	cfg := Defaults()
	cfg.Storages[0].Mode = "full_access"
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected unsupported mode error")
	}
}
