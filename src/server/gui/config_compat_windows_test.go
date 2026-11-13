//go:build windows

package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lsyltunnel/src/server/tunnel"
)

func TestServerConfigInstallMigrationAndRollback(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "installed", "conf", "server.yaml")
	template := filepath.Join(dir, "package", "conf", "server.yaml")
	for _, path := range []string{installed, template} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := tunnel.SaveConfig(template, tunnel.Config{
		ListenAddr: "0.0.0.0:3443",
		TLS: tunnel.TLSConfig{
			CertFile: "../certs/server.crt",
			KeyFile:  "../certs/server.key",
		},
		Security: tunnel.SecurityConfig{AuthFailThreshold: 12},
	}); err != nil {
		t.Fatal(err)
	}
	original := []byte("listen_addr: 0.0.0.0:4555\nauth:\n  users:\n    - username: alice\n      password_hash: plain:secret\nforwards:\n  - name: rdp\n    direction: client_to_server\n    server_target: 127.0.0.1:3389\n    allowed_users: [alice]\nsecurity: []\n")
	if err := os.WriteFile(installed, original, 0o644); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "installed", "conf", ".install-rollback.json")
	opts := configCompatOptions{ConfigPath: installed, TemplatePath: template, SourceVersion: "3.4.0.0", BackupFile: backup}
	if err := runConfigCompatCheck(opts); err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if data, err := os.ReadFile(installed); err != nil || string(data) != string(original) {
		t.Fatalf("preflight changed installed config: %v", err)
	}
	opts.Apply = true
	if err := runConfigCompatCheck(opts); err != nil {
		t.Fatalf("apply: %v", err)
	}
	cfg, err := tunnel.LoadConfigRaw(installed)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "0.0.0.0:4555" || cfg.Security.AuthFailThreshold != 12 ||
		len(cfg.Auth.Users) != 1 || cfg.Auth.Users[0].Username != "alice" ||
		len(cfg.Forwards) != 1 || cfg.Forwards[0].ServerTarget != "127.0.0.1:3389" {
		t.Fatalf("migration did not retain business settings and new defaults: %+v", cfg)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup was not written: %v", err)
	}
	if err := runConfigCompatCheck(configCompatOptions{Rollback: true, BackupFile: backup}); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if data, err := os.ReadFile(installed); err != nil || string(data) != string(original) {
		t.Fatalf("rollback did not restore original config: %v", err)
	}
	if data, err := os.ReadFile(backup); err != nil || !strings.Contains(string(data), "server.yaml") {
		t.Fatalf("rollback backup should remain available for installer cleanup: %v", err)
	}
}
