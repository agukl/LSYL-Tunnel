//go:build windows

package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if err := runClientConfigCompatCheck(opts); err == nil || !strings.Contains(err.Error(), archive) {
		t.Fatalf("bad archive was not identified: %v", err)
	}
	if err := os.WriteFile(archive, []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}
	opts.Apply = true
	if err := runClientConfigCompatCheck(opts); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{active, archive} {
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), "connection:") {
			t.Fatalf("%s not migrated: %v\n%s", path, err, data)
		}
	}
	opts.Rollback = true
	if err := runClientConfigCompatCheck(opts); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{active, archive} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != profile {
			t.Fatalf("%s not restored: %v\n%s", path, err, data)
		}
	}
}
