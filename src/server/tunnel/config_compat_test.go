package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckConfigShapeCompatibleIgnoresOrderAndValues(t *testing.T) {
	dir := t.TempDir()
	reference := writeTestConfig(t, dir, "reference.yaml", `
listen_addr: 0.0.0.0:3443
tls:
  cert_file: ../certs/server.crt
  key_file: ../certs/server.key
auth:
  users: []
security:
  max_handshake_bytes: 32768
  auth_fail_threshold: 8
`)
	current := writeTestConfig(t, dir, "current.yaml", `
security:
  auth_fail_threshold: 12
  max_handshake_bytes: 65536
auth:
  users:
    - username: admin
      password_hash: hash
tls:
  key_file: ../certs/custom.key
  cert_file: ../certs/custom.crt
listen_addr: 127.0.0.1:3443
`)

	if err := CheckConfigShapeCompatible(current, reference); err != nil {
		t.Fatalf("expected compatible config shape, got %v", err)
	}
}

func TestCheckConfigShapeCompatibleRejectsMissingKey(t *testing.T) {
	dir := t.TempDir()
	reference := writeTestConfig(t, dir, "reference.yaml", `
listen_addr: 0.0.0.0:3443
tls:
  cert_file: ../certs/server.crt
  key_file: ../certs/server.key
`)
	current := writeTestConfig(t, dir, "current.yaml", `
listen_addr: 0.0.0.0:3443
tls:
  cert_file: ../certs/server.crt
`)

	err := CheckConfigShapeCompatible(current, reference)
	if err == nil || !strings.Contains(err.Error(), "missing tls.key_file") {
		t.Fatalf("expected missing key error, got %v", err)
	}
}

func TestCheckConfigShapeCompatibleRejectsExtraKey(t *testing.T) {
	dir := t.TempDir()
	reference := writeTestConfig(t, dir, "reference.yaml", `
listen_addr: 0.0.0.0:3443
tls:
  cert_file: ../certs/server.crt
`)
	current := writeTestConfig(t, dir, "current.yaml", `
listen_addr: 0.0.0.0:3443
tls:
  cert_file: ../certs/server.crt
  key_file: ../certs/server.key
`)

	err := CheckConfigShapeCompatible(current, reference)
	if err == nil || !strings.Contains(err.Error(), "extra tls.key_file") {
		t.Fatalf("expected extra key error, got %v", err)
	}
}

func TestCheckConfigShapeCompatibleChecksReferenceSequenceItems(t *testing.T) {
	dir := t.TempDir()
	reference := writeTestConfig(t, dir, "reference.yaml", `
credential_seal:
  keys:
    - key_id: login-key
      private_key_file: ../certs/login.key
      public_key_file: ../certs/login.pub
`)
	current := writeTestConfig(t, dir, "current.yaml", `
credential_seal:
  keys:
    - key_id: login-key
      private_key_file: ../certs/login.key
      extra_key_file: ../certs/extra.key
`)

	err := CheckConfigShapeCompatible(current, reference)
	if err == nil || !strings.Contains(err.Error(), "missing credential_seal.keys[].public_key_file") || !strings.Contains(err.Error(), "extra credential_seal.keys[].extra_key_file") {
		t.Fatalf("expected sequence item structure error, got %v", err)
	}
}

func writeTestConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}
