package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRuntimePathUsesRuntimeForSourceConfig(t *testing.T) {
	configDir := filepath.Join("repo", "src", "server", "conf")
	got := defaultRuntimePath(configDir, "data", "server-state.json")
	want := filepath.Join("repo", "runtime", "data", "server-state.json")
	if got != want {
		t.Fatalf("defaultRuntimePath() = %q, want %q", got, want)
	}
}

func TestDefaultRuntimePathKeepsPackageLocalLayout(t *testing.T) {
	configDir := filepath.Join("package", "conf")
	got := defaultRuntimePath(configDir, "logs", filepath.Join("request", "request.jsonl"))
	want := filepath.Join("package", "conf", "..", "logs", "request", "request.jsonl")
	if got != want {
		t.Fatalf("defaultRuntimePath() = %q, want %q", got, want)
	}
}

func TestApplyDefaultsMigratesLegacyConnectionRateField(t *testing.T) {
	cfg := Config{
		Security: SecurityConfig{
			MaxConnectionsPerIPPerWindow: 42,
		},
	}
	ApplyDefaults(&cfg)
	if got := cfg.Security.MaxNewConnectionsPerIPWindow; got != 42 {
		t.Fatalf("MaxNewConnectionsPerIPWindow = %d, want 42", got)
	}
	if got := cfg.Security.MaxConnectionsPerIPPerWindow; got != 0 {
		t.Fatalf("legacy MaxConnectionsPerIPPerWindow = %d, want 0", got)
	}
}

func TestLoadConfigDefaultsTrafficLogPaths(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(confDir, "server.yaml")
	data := strings.Join([]string{
		"tls:",
		"  cert_file: " + filepath.ToSlash(certFile),
		"  key_file: " + filepath.ToSlash(keyFile),
		"auth:",
		"  users:",
		"    - username: alice",
		"      password_hash: plain:secret",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Runtime.RequestLogFile, filepath.Join(confDir, "..", "logs", "request", "request.jsonl"); got != want {
		t.Fatalf("RequestLogFile = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.BusinessLogFile, filepath.Join(confDir, "..", "logs", "business", "business.jsonl"); got != want {
		t.Fatalf("BusinessLogFile = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.EntryTrafficLogFile, filepath.Join(confDir, "..", "logs", "entry-traffic", "entry-traffic.jsonl"); got != want {
		t.Fatalf("EntryTrafficLogFile = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.FlowTrafficLogFile, filepath.Join(confDir, "..", "logs", "flow-traffic", "flow-traffic.jsonl"); got != want {
		t.Fatalf("FlowTrafficLogFile = %q, want %q", got, want)
	}
}
