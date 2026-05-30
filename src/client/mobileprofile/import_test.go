package mobileprofile

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func TestImportBytesConvertsProfileToClientConfig(t *testing.T) {
	profile := validImportProfile(t)
	zipBytes := profileZip(t, profile, validImportCert(t))

	imported, err := ImportBytes(zipBytes)
	if err != nil {
		t.Fatalf("ImportBytes failed: %v", err)
	}
	cfg := imported.Config
	if cfg.ServerAddr != "vpn.example.test:3443" {
		t.Fatalf("ServerAddr = %q", cfg.ServerAddr)
	}
	if cfg.Username != "alice" {
		t.Fatalf("Username = %q", cfg.Username)
	}
	if cfg.Password != "" || cfg.PasswordEnv != "" || cfg.PasswordFile != "" {
		t.Fatalf("secret fields leaked into config: %#v", cfg)
	}
	if cfg.SavedCredential.Ciphertext != "sealed-import-credential" {
		t.Fatalf("unexpected saved credential: %#v", cfg.SavedCredential)
	}
	if cfg.TLS.CACertFile != "server.crt" || cfg.TLS.MinVersion != "1.3" || cfg.TLS.InsecureSkipVerify {
		t.Fatalf("unexpected TLS config: %#v", cfg.TLS)
	}
	if len(cfg.Forwards) != 1 {
		t.Fatalf("forward count = %d", len(cfg.Forwards))
	}
	if cfg.Forwards[0].ListenAddr != "127.0.0.1:13388" {
		t.Fatalf("listen addr = %q", cfg.Forwards[0].ListenAddr)
	}
	if imported.ExpiresAt.IsZero() {
		t.Fatal("ExpiresAt was not set")
	}
}

func TestImportBytesRejectsSecretFields(t *testing.T) {
	profile := validImportProfile(t)
	profile["password"] = "do-not-import"
	_, err := ImportBytes(profileZip(t, profile, validImportCert(t)))
	if err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("expected password rejection, got %v", err)
	}
}

func TestImportBytesRejectsNonMobileForward(t *testing.T) {
	profile := validImportProfile(t)
	profile["forwards"] = []map[string]any{{
		"name":          "rdp",
		"direction":     "server_to_client",
		"listen_addr":   "127.0.0.1:13388",
		"server_target": "127.0.0.1:3389",
	}}
	_, err := ImportBytes(profileZip(t, profile, validImportCert(t)))
	if err == nil || !strings.Contains(err.Error(), "client_to_server") {
		t.Fatalf("expected direction rejection, got %v", err)
	}
}

func validImportProfile(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"version":     1,
		"profile_id":  "mobile-alice",
		"server_addr": "vpn.example.test:3443",
		"username":    "alice",
		"client_id":   "desktop-client-1",
		"saved_credential": map[string]any{
			"type":       "server_sealed",
			"key_id":     "login-key-test",
			"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			"ciphertext": "sealed-import-credential",
		},
		"tls": map[string]any{
			"ca_cert_file":         "server.crt",
			"server_name":          "vpn.example.test",
			"min_version":          "1.3",
			"insecure_skip_verify": false,
		},
		"connection": map[string]any{"dial_timeout_sec": 7},
		"forwards": []map[string]any{{
			"name":          "rdp",
			"direction":     "client_to_server",
			"listen_addr":   "127.0.0.1:13388",
			"server_target": "127.0.0.1:3389",
		}},
	}
}

func profileZip(t *testing.T, profile map[string]any, certPEM string) []byte {
	t.Helper()
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := writeZipTo(&buf, profileJSON, []byte(certPEM)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func validImportCert(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "lsyl-tunnel-server"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"vpn.example.test"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
