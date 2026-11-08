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
	"strings"
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

func TestCertificateIPv4SANsAllowsDNSOnlyCertificate(t *testing.T) {
	path := writeTestServerCertificate(t, "vpn.example.test")
	if values, err := certificateIPv4SANs(path); err != nil || len(values) != 0 {
		t.Fatalf("certificateIPv4SANs() = %v, %v", values, err)
	}
}

func TestVirtualCertificatePolicy(t *testing.T) {
	certificate := writeTestServerCertificate(t, "vpn.example.test", "203.0.113.10")
	for _, host := range []string{"10.0.0.1", "10.255.255.254", "172.16.0.1", "172.31.255.254", "192.168.0.1", "192.168.255.254", "127.0.0.1", "203.0.113.10"} {
		t.Run(host, func(t *testing.T) {
			cfg := validVirtualForwardConfig(t)
			cfg.TLS.CACertFile = certificate
			cfg.Forwards[0].ListenAddr = net.JoinHostPort(host, "3389")
			if err := ValidateConfig(cfg); err != nil {
				t.Fatal(err)
			}
			if got, err := VirtualForwardAddr(cfg, cfg.Forwards[0]); err != nil || got != cfg.Forwards[0].ListenAddr {
				t.Fatalf("VirtualForwardAddr() = %q, %v", got, err)
			}
		})
	}
	for _, host := range []string{"9.255.255.255", "11.0.0.1", "172.15.255.255", "172.32.0.1", "192.167.255.255", "192.169.0.1", "100.64.0.1", "203.0.113.11"} {
		t.Run("SAN required "+host, func(t *testing.T) {
			cfg := validVirtualForwardConfig(t)
			cfg.TLS.CACertFile = certificate
			cfg.Forwards[0].ListenAddr = net.JoinHostPort(host, "3389")
			if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "not authorized") {
				t.Fatalf("expected certificate policy rejection, got %v", err)
			}
		})
	}
}

func TestVirtualPrivateAndLoopbackWithDNSOnlyCertificate(t *testing.T) {
	for _, host := range []string{"10.0.0.7", "172.16.0.7", "192.168.1.7", "127.0.0.1"} {
		t.Run(host, func(t *testing.T) {
			cfg := validVirtualForwardConfig(t)
			cfg.ServerAddr = "vpn.example.test:9443"
			cfg.TLS.CACertFile = writeTestServerCertificate(t, "vpn.example.test")
			cfg.Forwards[0].ListenAddr = net.JoinHostPort(host, "3389")
			if err := ValidateConfig(cfg); err != nil {
				t.Fatal(err)
			}
			cfg.Forwards[0].ListenAddr = net.JoinHostPort(host, "9443")
			if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "cannot use the server_addr port") {
				t.Fatalf("tunnel port protection was lost: %v", err)
			}
			cfg.Forwards[0].ListenAddr = ":3389"
			if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "no IPv4 SAN") {
				t.Fatalf("automatic selection must still require certificate SANs: %v", err)
			}
			cfg.Forwards[0].ListenAddr = net.JoinHostPort(host, "3389")
			cfg.TLS.CACertFile = ""
			if err := ValidateConfig(cfg); err == nil {
				t.Fatal("private and loopback endpoints must not remove the certificate requirement")
			}
		})
	}
}

func TestVirtualAddressSpellingAndSpecialAddressesRemainRejected(t *testing.T) {
	for _, addr := range []string{"localhost:3389", "127.0.0.2:3389", "[::1]:3389", "[::ffff:127.0.0.1]:3389", "0.0.0.0:3389", "169.254.1.2:3389", "224.0.0.1:3389", "255.255.255.255:3389"} {
		t.Run(addr, func(t *testing.T) {
			if _, _, err := parseVirtualListenAddr(addr); err == nil {
				t.Fatalf("accepted invalid endpoint %s", addr)
			}
		})
	}
}
