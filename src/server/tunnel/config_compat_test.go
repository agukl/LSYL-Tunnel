package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lsyltunnel/src/internal/protocol"
	appversion "lsyltunnel/src/internal/version"
)

func TestCheckConfigUpgradeCompatibleAcceptsLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	current := writeTestConfig(t, dir, "server.yaml", `
tls:
  cert_file: ../certs/server.crt
  key_file: ../certs/server.key
auth:
  users: []
`)

	if err := CheckConfigUpgradeCompatible(current); err != nil {
		t.Fatalf("expected legacy config to be upgrade-compatible, got %v", err)
	}
}

func TestCheckConfigUpgradeCompatibleRejectsNewerConfigVersion(t *testing.T) {
	dir := t.TempDir()
	current := writeTestConfig(t, dir, "server.yaml", `
config_version: 99
tls:
  cert_file: ../certs/server.crt
  key_file: ../certs/server.key
`)

	err := CheckConfigUpgradeCompatible(current)
	if err == nil || !strings.Contains(err.Error(), "requires a newer server") {
		t.Fatalf("expected newer config_version error, got %v", err)
	}
}

func TestCheckConfigUpgradeCompatibleRejectsInvalidClientRange(t *testing.T) {
	dir := t.TempDir()
	current := writeTestConfig(t, dir, "server.yaml", `
compatibility:
  min_client_version: "1.2.0"
  max_client_version: "1.1.0"
tls:
  cert_file: ../certs/server.crt
  key_file: ../certs/server.key
`)

	err := CheckConfigUpgradeCompatible(current)
	if err == nil || !strings.Contains(err.Error(), "min_client_version must be <= max_client_version") {
		t.Fatalf("expected invalid client range error, got %v", err)
	}
}

func TestCheckRequestCompatibilityRejectsLegacyClientBelowMinimum(t *testing.T) {
	srv := &Server{cfg: Config{Compatibility: CompatibilityConfig{
		MinClientVersion: appversion.MinCompatibleClientVersion,
		ProtocolVersion:  appversion.ProtocolVersion,
	}}}

	resp, ok := srv.checkRequestCompatibility(protocol.OpenRequest{
		Type:            "login",
		ClientVersion:   appversion.LegacyClientVersion,
		ProtocolVersion: appversion.ProtocolVersion,
	})
	if ok {
		t.Fatal("expected legacy client to be rejected")
	}
	if resp.Code != "client_version_unsupported" {
		t.Fatalf("Code = %q, want client_version_unsupported", resp.Code)
	}
}

func TestCheckRequestCompatibilityRejectsProtocolMismatch(t *testing.T) {
	srv := &Server{cfg: Config{Compatibility: CompatibilityConfig{
		MinClientVersion: appversion.MinCompatibleClientVersion,
		ProtocolVersion:  appversion.ProtocolVersion,
	}}}

	resp, ok := srv.checkRequestCompatibility(protocol.OpenRequest{
		Type:            "login",
		ClientVersion:   appversion.AppVersion,
		ProtocolVersion: appversion.LegacyProtocolVersion,
	})
	if ok {
		t.Fatal("expected protocol mismatch to be rejected")
	}
	if resp.Code != "protocol_version_unsupported" {
		t.Fatalf("Code = %q, want protocol_version_unsupported", resp.Code)
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
