package main

import (
	"archive/zip"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testConfig = `server_addr: 127.0.0.1:3443
username: alice
password: ""
saved_credential: {}
tls:
  ca_cert_file: ../cert/server.crt
  min_version: "1.3"
forwards:
  - name: web
    direction: client_to_server
    listen_addr: 127.0.0.1:3388
    server_target: 127.0.0.1:65398
`

const testMobileConfig = `server_addr: 127.0.0.1:3443
username: alice
password: should-not-export
password_env: LSYL_PASSWORD
password_file: password.txt
saved_credential:
  type: server_sealed
  key_id: login-key-test
  expires_at: "2099-01-01T00:00:00Z"
  ciphertext: sealed-mobile-credential
client_id: desktop-client-1
tls:
  ca_cert_file: ../cert/server.crt
  server_name: localhost
  min_version: "1.3"
  insecure_skip_verify: false
connection:
  dial_timeout_sec: 7
forwards:
  - name: web
    direction: client_to_server
    listen_addr: localhost:13388
    server_target: 127.0.0.1:65398
`

func TestImportCurrentUseAndDeleteProfile(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction behavior is validated on Windows")
	}
	dir := t.TempDir()
	install := filepath.Join(dir, "install")
	profiles := filepath.Join(dir, "profiles")
	writeFile(t, filepath.Join(install, "conf", "client.yaml"), testConfig)
	writeFile(t, filepath.Join(install, "cert", "server.crt"), validTestCert(t))
	opts := options{installDir: install, profilesDir: profiles}

	var out bytes.Buffer
	if err := commandImportCurrent(&out, opts, []string{"dev-a"}); err != nil {
		t.Fatalf("import-current failed: %v", err)
	}
	if !fileExists(filepath.Join(profiles, "dev-a", "conf", "client.yaml")) {
		t.Fatal("imported config missing")
	}
	if !fileExists(filepath.Join(profiles, "dev-a", "cert", "server.crt")) {
		t.Fatal("imported cert missing")
	}

	writeFile(t, filepath.Join(install, "conf", "local.txt"), "keep")
	if err := commandUse(&out, opts, []string{"dev-a"}); err != nil {
		t.Fatalf("use failed: %v", err)
	}
	if name, ok := activeProfileName(opts); !ok || name != "dev-a" {
		t.Fatalf("active profile = %q/%v, want dev-a/true", name, ok)
	}
	if !fileExists(filepath.Join(profiles, "dev-a", "conf", "client.yaml")) {
		t.Fatal("profile target was removed while switching")
	}
	backups, err := filepath.Glob(filepath.Join(install, "conf.profile-backup-*"))
	if err != nil || len(backups) == 0 {
		t.Fatalf("expected backup of original conf dir, got %v, %v", backups, err)
	}

	if err := commandDelete(&out, opts, []string{"dev-a", "-yes"}); err == nil {
		t.Fatal("delete active profile without force unexpectedly succeeded")
	}
	if err := commandDelete(&out, opts, []string{"dev-a", "-yes", "-force"}); err != nil {
		t.Fatalf("delete active profile with force failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(profiles, "dev-a")); !os.IsNotExist(err) {
		t.Fatalf("profile still exists or stat failed: %v", err)
	}
}

func TestExportMobileProfileCommand(t *testing.T) {
	dir := t.TempDir()
	install := filepath.Join(dir, "install")
	writeFile(t, filepath.Join(install, "conf", "client.yaml"), testMobileConfig)
	testCert := validTestCert(t)
	writeFile(t, filepath.Join(install, "cert", "server.crt"), testCert)
	target := filepath.Join(dir, "alice.lsylprofile")
	opts := options{installDir: install, profilesDir: filepath.Join(dir, "profiles")}

	var out bytes.Buffer
	if err := commandExportMobile(&out, opts, []string{"-out", target}); err != nil {
		t.Fatalf("export-mobile failed: %v", err)
	}
	if !strings.Contains(out.String(), "Exported mobile profile") {
		t.Fatalf("unexpected output: %s", out.String())
	}

	profileJSON := readZipEntry(t, target, "profile.json")
	cert := readZipEntry(t, target, "server.crt")
	if string(cert) != testCert {
		t.Fatal("exported server.crt did not match client cert")
	}
	var profile map[string]any
	if err := json.Unmarshal(profileJSON, &profile); err != nil {
		t.Fatalf("profile json invalid: %v", err)
	}
	for _, key := range []string{"password", "password_env", "password_file"} {
		if _, ok := profile[key]; ok {
			t.Fatalf("mobile profile leaked %s", key)
		}
	}
	if profile["version"].(float64) != 1 {
		t.Fatalf("unexpected version: %v", profile["version"])
	}
	if profile["username"] != "alice" {
		t.Fatalf("unexpected username: %v", profile["username"])
	}
	if profile["client_id"] != "desktop-client-1" {
		t.Fatalf("unexpected client_id: %v", profile["client_id"])
	}
	credential := profile["saved_credential"].(map[string]any)
	if credential["ciphertext"] != "sealed-mobile-credential" {
		t.Fatalf("unexpected credential: %v", credential)
	}
	tls := profile["tls"].(map[string]any)
	if tls["ca_cert_file"] != "server.crt" || tls["min_version"] != "1.3" || tls["insecure_skip_verify"] != false {
		t.Fatalf("unexpected tls config: %v", tls)
	}
	forwards := profile["forwards"].([]any)
	if len(forwards) != 1 {
		t.Fatalf("unexpected forward count: %d", len(forwards))
	}
	forward := forwards[0].(map[string]any)
	if forward["listen_addr"] != "127.0.0.1:13388" {
		t.Fatalf("listen_addr was not normalized for mobile: %v", forward["listen_addr"])
	}
	if forward["server_target"] != "127.0.0.1:65398" {
		t.Fatalf("unexpected server_target: %v", forward["server_target"])
	}
}

func TestExportMobileProfileRequiresSavedCredential(t *testing.T) {
	dir := t.TempDir()
	install := filepath.Join(dir, "install")
	writeFile(t, filepath.Join(install, "conf", "client.yaml"), testConfig)
	writeFile(t, filepath.Join(install, "cert", "server.crt"), validTestCert(t))
	target := filepath.Join(dir, "alice.lsylprofile")
	opts := options{installDir: install, profilesDir: filepath.Join(dir, "profiles")}

	var out bytes.Buffer
	err := commandExportMobile(&out, opts, []string{"-out", target})
	if err == nil || !strings.Contains(err.Error(), "saved_credential") {
		t.Fatalf("expected saved_credential error, got %v", err)
	}
}

func TestImportProfileRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	opts := options{installDir: filepath.Join(dir, "install"), profilesDir: filepath.Join(dir, "profiles")}
	conf := filepath.Join(dir, "client.yaml")
	cert := filepath.Join(dir, "server.crt")
	writeFile(t, conf, testConfig)
	writeFile(t, cert, validTestCert(t))
	if err := importProfile(opts, "..\\escape", conf, cert, false); err == nil || !strings.Contains(err.Error(), "invalid profile") {
		t.Fatalf("expected invalid profile error, got %v", err)
	}
}

func readZipEntry(t *testing.T, path, name string) []byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatalf("zip entry %s not found", name)
	return nil
}

func validTestCert(t *testing.T) string {
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
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
