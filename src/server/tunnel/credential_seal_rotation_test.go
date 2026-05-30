package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestExpiredCredentialSealKeyRotatesAndPersists(t *testing.T) {
	configPath := writeCredentialSealRotationConfig(t, []CredentialSealKeyConfig{{
		KeyID:          "login-key-old",
		PrivateKeyFile: "../certs/login-key-old.key",
		PublicKeyFile:  "../certs/login-key-old.pub",
		ExpiresAt:      time.Now().Add(-time.Hour).Format(time.RFC3339),
		Active:         true,
	}})
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{cfg: cfg, logf: t.Logf}
	if err := srv.loadCredentialSealKeys(); err != nil {
		t.Fatal(err)
	}
	if srv.activeCredentialKey == nil {
		t.Fatal("active credential key is nil")
	}
	if srv.activeCredentialKey.id == "login-key-old" {
		t.Fatalf("expired key is still active: %s", srv.activeCredentialKey.id)
	}
	if !strings.HasPrefix(srv.activeCredentialKey.id, "login-key-") {
		t.Fatalf("unexpected rotated key id: %s", srv.activeCredentialKey.id)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(configPath), "..", "certs", srv.activeCredentialKey.id+".key")); err != nil {
		t.Fatalf("rotated private key was not created: %v", err)
	}

	raw := readCredentialSealConfig(t, configPath)
	active := activeCredentialKeyForTest(t, raw.CredentialSeal.Keys)
	if active.KeyID != srv.activeCredentialKey.id {
		t.Fatalf("persisted active key = %q, want %q", active.KeyID, srv.activeCredentialKey.id)
	}
	if filepath.IsAbs(active.PrivateKeyFile) || filepath.IsAbs(active.PublicKeyFile) {
		t.Fatalf("persisted paths should stay package-relative: private=%q public=%q", active.PrivateKeyFile, active.PublicKeyFile)
	}
	if !strings.Contains(active.PrivateKeyFile, "../certs/") && !strings.Contains(active.PrivateKeyFile, `..\certs\`) {
		t.Fatalf("persisted private key path did not keep certs directory: %q", active.PrivateKeyFile)
	}
}

func TestExpiredCredentialSealKeyActivatesPreparedFutureKey(t *testing.T) {
	futureKey := CredentialSealKeyConfig{
		KeyID:          "login-key-prepared",
		PrivateKeyFile: "../certs/login-key-prepared.key",
		PublicKeyFile:  "../certs/login-key-prepared.pub",
		ExpiresAt:      time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339),
		Active:         false,
	}
	configPath := writeCredentialSealRotationConfig(t, []CredentialSealKeyConfig{
		{
			KeyID:          "login-key-old",
			PrivateKeyFile: "../certs/login-key-old.key",
			PublicKeyFile:  "../certs/login-key-old.pub",
			ExpiresAt:      time.Now().Add(-time.Hour).Format(time.RFC3339),
			Active:         true,
		},
		futureKey,
	})
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{cfg: cfg, logf: t.Logf}
	if err := srv.loadCredentialSealKeys(); err != nil {
		t.Fatal(err)
	}
	if srv.activeCredentialKey == nil || srv.activeCredentialKey.id != futureKey.KeyID {
		t.Fatalf("active key = %v, want %q", srv.activeCredentialKey, futureKey.KeyID)
	}
	raw := readCredentialSealConfig(t, configPath)
	if len(raw.CredentialSeal.Keys) != 2 {
		t.Fatalf("prepared key should be activated without appending a duplicate, got %d keys", len(raw.CredentialSeal.Keys))
	}
	active := activeCredentialKeyForTest(t, raw.CredentialSeal.Keys)
	if active.KeyID != futureKey.KeyID {
		t.Fatalf("persisted active key = %q, want %q", active.KeyID, futureKey.KeyID)
	}
}

func writeCredentialSealRotationConfig(t *testing.T, keys []CredentialSealKeyConfig) string {
	t.Helper()
	root := t.TempDir()
	confDir := filepath.Join(root, "conf")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(confDir, "server.yaml")
	data, err := yaml.Marshal(Config{
		ListenAddr:  "127.0.0.1:0",
		MonitorAddr: "",
		TLS: TLSConfig{
			CertFile:   "../certs/server.crt",
			KeyFile:    "../certs/server.key",
			MinVersion: "1.3",
		},
		CredentialSeal: CredentialSealConfig{Keys: keys},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The rotation logic should keep these paths relative when it writes back.
	data = []byte(strings.ReplaceAll(string(data), `..\certs\`, "../certs/"))
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func readCredentialSealConfig(t *testing.T, configPath string) Config {
	t.Helper()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func activeCredentialKeyForTest(t *testing.T, keys []CredentialSealKeyConfig) CredentialSealKeyConfig {
	t.Helper()
	var active []CredentialSealKeyConfig
	for _, key := range keys {
		if key.Active {
			active = append(active, key)
		}
	}
	if len(active) != 1 {
		t.Fatalf("active key count = %d, want 1; keys=%s", len(active), credentialKeyIDsForTest(keys))
	}
	return active[0]
}

func credentialKeyIDsForTest(keys []CredentialSealKeyConfig) string {
	ids := make([]string, 0, len(keys))
	for _, key := range keys {
		ids = append(ids, fmt.Sprintf("%s(active=%v)", key.KeyID, key.Active))
	}
	return strings.Join(ids, ", ")
}
