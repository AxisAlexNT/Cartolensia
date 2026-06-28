package tlsutil

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSelfSignedCertificateIncludesHosts(t *testing.T) {
	cert, err := SelfSignedCertificate([]string{"127.0.0.1:18080", "cartolensia.local"})
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("expected certificate chain")
	}
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	if time.Now().After(parsed.NotAfter) {
		t.Fatal("generated certificate is expired")
	}
	if err := parsed.VerifyHostname("127.0.0.1"); err != nil {
		t.Fatalf("certificate should verify for 127.0.0.1: %v", err)
	}
	if err := parsed.VerifyHostname("cartolensia.local"); err != nil {
		t.Fatalf("certificate should verify for cartolensia.local: %v", err)
	}
	if err := parsed.VerifyHostname("example.com"); err == nil {
		t.Fatal("certificate should not verify for unrelated host")
	}
}

func TestLoadOrCreateSelfSignedCertificateReusesValidCache(t *testing.T) {
	dir := t.TempDir()
	hosts := []string{"127.0.0.1", "cartolensia.local"}
	first, certPath, err := LoadOrCreateSelfSignedCertificate(dir, hosts)
	if err != nil {
		t.Fatalf("load/create first certificate: %v", err)
	}
	if certPath == "" {
		t.Fatal("expected cached certificate path")
	}
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("expected certificate file: %v", err)
	}
	keyPath := strings.TrimSuffix(certPath, filepath.Ext(certPath)) + ".key"
	if info, err := os.Stat(keyPath); err != nil {
		t.Fatalf("expected key file: %v", err)
	} else if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("expected private key not to be group/world readable, got %v", info.Mode().Perm())
	}
	firstParsed, err := x509.ParseCertificate(first.Certificate[0])
	if err != nil {
		t.Fatalf("parse first certificate: %v", err)
	}
	second, secondPath, err := LoadOrCreateSelfSignedCertificate(dir, hosts)
	if err != nil {
		t.Fatalf("load/create second certificate: %v", err)
	}
	if secondPath != certPath {
		t.Fatalf("expected same cached path, got %q vs %q", secondPath, certPath)
	}
	secondParsed, err := x509.ParseCertificate(second.Certificate[0])
	if err != nil {
		t.Fatalf("parse second certificate: %v", err)
	}
	if firstParsed.SerialNumber.Cmp(secondParsed.SerialNumber) != 0 {
		t.Fatal("expected cached certificate to be reused")
	}
}

func TestLoadReusableCertificateRejectsNearlyExpired(t *testing.T) {
	dir := t.TempDir()
	hosts := normalizeHosts([]string{"127.0.0.1"})
	certPEM, keyPEM, err := selfSignedPEM(hosts, time.Now().UTC().Add(-340*24*time.Hour))
	if err != nil {
		t.Fatalf("create old certificate: %v", err)
	}
	certPath := filepath.Join(dir, "old.crt")
	keyPath := filepath.Join(dir, "old.key")
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if _, ok := loadReusableCertificate(certPath, keyPath, hosts, time.Now().UTC()); ok {
		t.Fatal("expected nearly expired certificate to be regenerated")
	}
}
