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
	t.Setenv("CARTOLENSIA_HTTP_TLS_ADDR", ":9443")
	t.Setenv("CARTOLENSIA_HTTP_REDIRECT_HTTP_TO_HTTPS", "true")
	t.Setenv("CARTOLENSIA_HTTP_TLS_AUTO_SELF_SIGNED", "true")
	t.Setenv("CARTOLENSIA_HTTP_TLS_HOSTS", "cartolensia.local,127.0.0.1")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != ":9100" {
		t.Fatalf("env override did not apply: %q", cfg.HTTP.Addr)
	}
	if cfg.HTTP.TLSAddr != ":9443" || !cfg.HTTP.RedirectHTTPToHTTPS {
		t.Fatalf("tls env overrides did not apply: %#v", cfg.HTTP)
	}
	if !cfg.HTTP.TLSAutoSelfSigned || len(cfg.HTTP.TLSHosts) != 2 {
		t.Fatalf("tls self-signed env overrides did not apply: %#v", cfg.HTTP)
	}
	if cfg.Database.URL != "postgres://example" {
		t.Fatalf("unexpected db url %q", cfg.Database.URL)
	}
}

func TestStorageSMBSourceURLPopulatesDiagnosticFields(t *testing.T) {
	cfg := Defaults()
	cfg.Storages = []StorageConfig{{
		Name:      "nas",
		Kind:      "fs",
		Root:      "/mnt/nas/share",
		Mode:      ModeStrictReadOnly,
		SourceURL: "smb://nas.local/media/photos/2026",
		SMB: &SMBStorageConfig{
			CredentialsFile: "/etc/cartolensia/smb-nas.credentials",
		},
	}}
	if err := Validate(&cfg); err != nil {
		t.Fatal(err)
	}
	smb := cfg.Storages[0].SMB
	if smb == nil || smb.Host != "nas.local" || smb.Share != "media" || smb.Path != "photos/2026" {
		t.Fatalf("unexpected SMB metadata %#v", smb)
	}
	if smb.CredentialsFile != "/etc/cartolensia/smb-nas.credentials" {
		t.Fatalf("credentials file was not preserved: %#v", smb)
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

func TestValidateRequiresTLSCertAndKeyTogether(t *testing.T) {
	cfg := Defaults()
	cfg.HTTP.TLSCertFile = "cert.pem"
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected partial TLS configuration error")
	}
	cfg.HTTP.TLSKeyFile = "key.pem"
	if err := Validate(&cfg); err != nil {
		t.Fatalf("expected complete TLS configuration to pass: %v", err)
	}
}

func TestValidateAllowsAutoSelfSignedTLS(t *testing.T) {
	cfg := Defaults()
	cfg.HTTP.TLSAutoSelfSigned = true
	cfg.HTTP.TLSHosts = []string{"cartolensia.local", "127.0.0.1"}
	if err := Validate(&cfg); err != nil {
		t.Fatalf("expected self-signed TLS configuration to pass: %v", err)
	}
}

func TestValidateRequiresTLSConfigForSeparateTLSAddr(t *testing.T) {
	cfg := Defaults()
	cfg.HTTP.TLSAddr = ":9443"
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected tls_addr without TLS configuration to fail")
	}
	cfg.HTTP.TLSAutoSelfSigned = true
	if err := Validate(&cfg); err != nil {
		t.Fatalf("expected tls_addr with self-signed TLS to pass: %v", err)
	}
}
