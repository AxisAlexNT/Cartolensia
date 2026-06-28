package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func SelfSignedCertificate(hosts []string) (tls.Certificate, error) {
	hosts = normalizeHosts(hosts)
	certPEM, keyPEM, err := selfSignedPEM(hosts, time.Now().UTC())
	if err != nil {
		return tls.Certificate{}, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load generated tls key pair: %w", err)
	}
	return cert, nil
}

func LoadOrCreateSelfSignedCertificate(cacheDir string, hosts []string) (tls.Certificate, string, error) {
	hosts = normalizeHosts(hosts)
	if strings.TrimSpace(cacheDir) == "" {
		cert, err := SelfSignedCertificate(hosts)
		return cert, "", err
	}
	dir := filepath.Join(cacheDir, "tls")
	hash := sha256.Sum256([]byte(strings.Join(hosts, "\x00")))
	stem := fmt.Sprintf("self-signed-%x", hash[:8])
	certPath := filepath.Join(dir, stem+".crt")
	keyPath := filepath.Join(dir, stem+".key")
	if cert, ok := loadReusableCertificate(certPath, keyPath, hosts, time.Now().UTC()); ok {
		return cert, certPath, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, "", fmt.Errorf("create tls cache directory: %w", err)
	}
	certPEM, keyPEM, err := selfSignedPEM(hosts, time.Now().UTC())
	if err != nil {
		return tls.Certificate{}, "", err
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return tls.Certificate{}, "", fmt.Errorf("write tls certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return tls.Certificate{}, "", fmt.Errorf("write tls private key: %w", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, "", fmt.Errorf("load generated tls key pair: %w", err)
	}
	return cert, certPath, nil
}

func selfSignedPEM(hosts []string, now time.Time) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate tls private key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("generate tls serial: %w", err)
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: hosts[0],
		},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}
		template.DNSNames = append(template.DNSNames, host)
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, key.Public(), key)
	if err != nil {
		return nil, nil, fmt.Errorf("create tls certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal tls private key: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func loadReusableCertificate(certPath, keyPath string, hosts []string, now time.Time) (tls.Certificate, bool) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, false
	}
	if len(cert.Certificate) == 0 {
		return tls.Certificate{}, false
	}
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, false
	}
	if now.Before(parsed.NotBefore) || now.After(parsed.NotAfter.Add(-30*24*time.Hour)) {
		return tls.Certificate{}, false
	}
	for _, host := range hosts {
		if err := parsed.VerifyHostname(host); err != nil {
			return tls.Certificate{}, false
		}
	}
	return cert, true
}

func normalizeHosts(hosts []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(hosts)+2)
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		host = strings.Trim(host, "[]")
		if host == "" || host == ":" {
			continue
		}
		if strings.Contains(host, ":") {
			if parsedHost, _, err := net.SplitHostPort(host); err == nil {
				host = strings.Trim(parsedHost, "[]")
			}
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	for _, fallback := range []string{"localhost", "127.0.0.1"} {
		if _, ok := seen[fallback]; ok {
			continue
		}
		out = append(out, fallback)
	}
	return out
}
