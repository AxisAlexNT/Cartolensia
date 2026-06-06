package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"
)

func SelfSignedCertificate(hosts []string) (tls.Certificate, error) {
	hosts = normalizeHosts(hosts)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate tls private key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate tls serial: %w", err)
	}
	now := time.Now().UTC()
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
		return tls.Certificate{}, fmt.Errorf("create tls certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal tls private key: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load generated tls key pair: %w", err)
	}
	return cert, nil
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
