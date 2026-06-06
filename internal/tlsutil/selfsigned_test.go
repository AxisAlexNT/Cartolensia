package tlsutil

import (
	"crypto/x509"
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
