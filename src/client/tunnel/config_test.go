package tunnel

import (
	"os"
	"path/filepath"
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
