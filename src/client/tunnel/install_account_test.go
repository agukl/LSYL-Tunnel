package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreserveInstalledUsernameKeepsNewProfileSettings(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.yaml")
	newPath := filepath.Join(dir, "new.yaml")
	oldConfig := "username: alice\nserver_addr: old.example:3443\npassword: old-secret\n"
	newConfig := `config_version: 2
requires:
  min_client_version: "2.1.0"
server_addr: new.example:3443
username: packaged
password: ""
password_env: ""
password_file: ""
saved_credential: {}
client_id: ""
log_level: info
tls:
  ca_cert_file: ../cert/server.crt
  server_name: ""
  min_version: "1.3"
  insecure_skip_verify: false
connection:
  dial_timeout_sec: 5
forwards:
  - name: rdp
    direction: client_to_server
    listen_addr: 127.0.0.1:13389
    server_target: 127.0.0.1:3389
`
	for path, data := range map[string]string{oldPath: oldConfig, newPath: newConfig} {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := PreserveInstalledUsername(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := decodeClientConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Username != "alice" || cfg.ServerAddr != "new.example:3443" || cfg.ClientID != "" ||
		cfg.Password != "" || cfg.SavedCredential.Ciphertext != "" ||
		len(cfg.Forwards) != 1 || cfg.Forwards[0].ListenAddr != "127.0.0.1:13389" {
		t.Fatalf("installed profile changed unexpectedly: %+v", cfg)
	}
	if strings.Contains(string(data), "old-secret") || strings.Contains(string(data), "old.example") {
		t.Fatalf("old credentials or address were copied: %s", data)
	}
}

func TestPreserveInstalledUsernameSkipsEmptyAccount(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.yaml")
	newPath := filepath.Join(dir, "new.yaml")
	if err := os.WriteFile(oldPath, []byte("username: \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := []byte("username: packaged\n")
	if err := os.WriteFile(newPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PreserveInstalledUsername(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("empty old account replaced package value: %s", data)
	}
}

func TestPreserveInstalledUsernameSkipsIncompatibleOldConfig(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.yaml")
	newPath := filepath.Join(dir, "new.yaml")
	if err := os.WriteFile(oldPath, []byte("username: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := []byte("username: packaged\n")
	if err := os.WriteFile(newPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PreserveInstalledUsername(oldPath, newPath); err != nil {
		t.Fatalf("invalid old config blocked account migration: %v", err)
	}
	if data, err := os.ReadFile(newPath); err != nil || string(data) != string(original) {
		t.Fatalf("new config changed: %v\n%s", err, data)
	}
}
