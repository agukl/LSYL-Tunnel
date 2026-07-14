package tunnel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestServerCertificate(t *testing.T, values ...string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "LSYL Tunnel test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, value := range values {
		if ip := net.ParseIP(value); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, value)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "server.crt")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCertificateIPv4SANs(t *testing.T) {
	path := writeTestServerCertificate(t, "vpn.example.test", "192.0.2.22", "198.51.100.8")
	values, err := certificateIPv4SANs(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != "192.0.2.22" || values[1] != "198.51.100.8" {
		t.Fatalf("certificateIPv4SANs() = %v", values)
	}
}

func TestCertificateIPv4SANsRequiresIPAddress(t *testing.T) {
	path := writeTestServerCertificate(t, "vpn.example.test")
	if _, err := certificateIPv4SANs(path); err == nil {
		t.Fatal("certificateIPv4SANs() accepted a certificate without an IPv4 SAN")
	}
}
