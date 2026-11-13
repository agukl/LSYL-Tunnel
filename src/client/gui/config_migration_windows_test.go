//go:build windows

package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lsyltunnel/src/client/tunnel"
)

func TestClientMigrationPreflightsAllProfiles(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "conf")
	if err := os.Mkdir(conf, 0o755); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(conf, "client.yaml")
	archive := filepath.Join(conf, "client.example_3443.yaml")
	profile := `config_version: 2
server_addr: 127.0.0.1:3443
username: alice
tls:
  ca_cert_file: ../cert/server.crt
forwards:
  - name: rdp
    listen_addr: 127.0.0.1:3388
    server_target: 127.0.0.1:3389
`
	for _, path := range []string{active, archive} {
		if err := os.WriteFile(path, []byte(profile), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts := clientConfigCompatOptions{ConfigPath: active, SourceVersion: "3.2.0.0", BackupFile: filepath.Join(dir, "migration.json")}
	if err := runClientConfigCompatCheck(opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(opts.BackupFile); !os.IsNotExist(err) {
		t.Fatal("preflight wrote a backup or changed files")
	}
	if err := os.WriteFile(archive, []byte(profile+"unknown: value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runClientConfigCompatCheck(opts); err != nil {
		t.Fatalf("incompatible archived config blocked installation: %v", err)
	}
	opts.Apply = true
	if err := runClientConfigCompatCheck(opts); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(active); err != nil || !strings.Contains(string(data), "connection:") {
		t.Fatalf("compatible active config not migrated: %v\n%s", err, data)
	}
	if data, err := os.ReadFile(archive); err != nil || string(data) != profile+"unknown: value\n" {
		t.Fatalf("incompatible archived config changed: %v\n%s", err, data)
	}
	opts.Rollback = true
	if err := runClientConfigCompatCheck(opts); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(active); err != nil || string(data) != profile {
		t.Fatalf("active config not restored: %v\n%s", err, data)
	}
	if data, err := os.ReadFile(archive); err != nil || string(data) != profile+"unknown: value\n" {
		t.Fatalf("incompatible archive changed during rollback: %v\n%s", err, data)
	}
}

func TestClientMigrationSkipsIncompatibleActiveConfig(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "client.yaml")
	original := []byte("config_version: 1\nusername: alice\nunknown: true\n")
	if err := os.WriteFile(active, original, 0o644); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "migration.json")
	opts := clientConfigCompatOptions{ConfigPath: active, SourceVersion: "3.2.0.0", BackupFile: backup}
	if err := runClientConfigCompatCheck(opts); err != nil {
		t.Fatalf("incompatible active config blocked preflight: %v", err)
	}
	opts.Apply = true
	if err := runClientConfigCompatCheck(opts); err != nil {
		t.Fatalf("incompatible active config blocked apply: %v", err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("unexpected migration backup: %v", err)
	}
	if data, err := os.ReadFile(active); err != nil || string(data) != string(original) {
		t.Fatalf("incompatible active config was changed: %v\n%s", err, data)
	}
}

func TestClientInstallAccountCommand(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.yaml")
	newPath := filepath.Join(dir, "client.yaml")
	if err := os.WriteFile(oldPath, []byte("username: alice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	newConfig := `config_version: 2
server_addr: new.example:3443
username: packaged
client_id: ""
tls:
  ca_cert_file: ../cert/server.crt
forwards:
  - name: rdp
    listen_addr: 127.0.0.1:13389
    server_target: 127.0.0.1:3389
`
	if err := os.WriteFile(newPath, []byte(newConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	prepared, err := tunnel.PrepareConfigMigration(newPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, prepared.Data, 0o644); err != nil {
		t.Fatal(err)
	}
	opts, ok, err := parseClientConfigCompatArgs([]string{
		"-config-compat-check", "-preserve-account", "-config", newPath, "-source-config", oldPath,
	})
	if err != nil || !ok {
		t.Fatalf("parse install account command: ok=%v err=%v", ok, err)
	}
	if err := runClientConfigCompatCheck(opts); err != nil {
		t.Fatal(err)
	}
	cfg, err := tunnel.LoadConfigRaw(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Username != "alice" || cfg.ServerAddr != "new.example:3443" || len(cfg.Forwards) != 1 {
		t.Fatalf("installed config = %+v", cfg)
	}
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `client_id: ""`) {
		t.Fatalf("installer persisted a generated client ID: %s", data)
	}
	if err := runClientConfigCompatCheck(clientConfigCompatOptions{ConfigPath: newPath}); err != nil {
		t.Fatalf("valid installed package config was rejected: %v", err)
	}
	if err := os.WriteFile(newPath, append(data, []byte("unknown: true\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runClientConfigCompatCheck(clientConfigCompatOptions{ConfigPath: newPath}); err == nil {
		t.Fatal("invalid installed package config was accepted")
	}
}
