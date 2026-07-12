package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigPasswordFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "password.txt"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, "client.yaml")
	data := []byte(`server_addr: 127.0.0.1:9443
username: alice
password_file: password.txt
tls:
  insecure_skip_verify: true
forwards:
  - name: echo
    listen_addr: 127.0.0.1:0
    server_target: 127.0.0.1:80
`)
	if err := os.WriteFile(cfgFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "secret" {
		t.Fatalf("unexpected password: %q", cfg.Password)
	}
}

func TestLoadConfigPasswordEnv(t *testing.T) {
	t.Setenv("LSYL_TUNNEL_TEST_PASSWORD", "from-env")
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "client.yaml")
	data := []byte(`server_addr: 127.0.0.1:9443
username: alice
password_env: LSYL_TUNNEL_TEST_PASSWORD
tls:
  insecure_skip_verify: true
forwards:
  - name: echo
    listen_addr: 127.0.0.1:0
    server_target: 127.0.0.1:80
`)
	if err := os.WriteFile(cfgFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "from-env" {
		t.Fatalf("unexpected password: %q", cfg.Password)
	}
}

func TestCheckConfigUpgradeCompatibleRejectsNewerConfigVersion(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "client.yaml")
	data := []byte(`config_version: 99
server_addr: 127.0.0.1:9443
username: alice
tls:
  insecure_skip_verify: true
forwards:
  - name: echo
    listen_addr: 127.0.0.1:0
    server_target: 127.0.0.1:80
`)
	if err := os.WriteFile(cfgFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	err := CheckConfigUpgradeCompatible(cfgFile)
	if err == nil {
		t.Fatal("expected newer config_version error")
	}
}

func TestLoadConfigNormalizesLegacyReverseListenAddress(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "client.yaml")
	data := []byte(`server_addr: 127.0.0.1:9443
username: alice
password: secret
tls:
  insecure_skip_verify: true
forwards:
  - name: reverse-web
    direction: server_to_client
    listen_addr: localhost:18080
    server_target: 127.0.0.1:8080
`)
	if err := os.WriteFile(cfgFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Forwards[0].ListenPort; got != 18080 {
		t.Fatalf("ListenPort = %d, want 18080", got)
	}
	if got := cfg.Forwards[0].ListenAddr; got != "127.0.0.1:18080" {
		t.Fatalf("ListenAddr = %q, want canonical loopback address", got)
	}
}

func TestSaveConfigWritesReverseListenPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	cfg := Config{
		ServerAddr: "127.0.0.1:9443",
		Username:   "alice",
		Password:   "secret",
		TLS:        TLSConfig{InsecureSkipVerify: true},
		Forwards: []ForwardConfig{{
			Name:         "reverse-web",
			Direction:    DirectionServerToClient,
			ListenPort:   18080,
			ServerTarget: "127.0.0.1:8080",
		}},
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "listen_port: 18080") || strings.Contains(text, "listen_addr:") {
		t.Fatalf("unexpected saved reverse config: %s", text)
	}
}
