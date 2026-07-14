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

func TestApplyDefaultsSetsEntryProtectionLimits(t *testing.T) {
	var cfg Config
	ApplyDefaults(&cfg)
	if got := cfg.Security.MaxTrackedConnectionIPs; got != 8192 {
		t.Fatalf("MaxTrackedConnectionIPs = %d, want 8192", got)
	}
	if got := cfg.Security.ConnectionLimiterCleanupSec; got != 60 {
		t.Fatalf("ConnectionLimiterCleanupSec = %d, want 60", got)
	}
	if got := cfg.Security.MaxTrackedFailureIPs; got != 8192 {
		t.Fatalf("MaxTrackedFailureIPs = %d, want 8192", got)
	}
	if got := cfg.Security.FailureTrackerCleanupSec; got != 60 {
		t.Fatalf("FailureTrackerCleanupSec = %d, want 60", got)
	}
	if got := cfg.Security.EntryTrafficLogQueueSize; got != 2048 {
		t.Fatalf("EntryTrafficLogQueueSize = %d, want 2048", got)
	}
}

func TestValidateConfigRejectsNonLoopbackMonitor(t *testing.T) {
	err := ValidateConfig(Config{
		ListenAddr:  "0.0.0.0:3443",
		MonitorAddr: "0.0.0.0:19111",
		TLS:         TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
	})
	if err == nil || !strings.Contains(err.Error(), "monitor_addr must use a loopback address") {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

func TestValidateConfigAllowsLoopbackMonitor(t *testing.T) {
	err := ValidateConfig(Config{
		ListenAddr:  "0.0.0.0:3443",
		MonitorAddr: "127.0.0.1:19111",
		TLS:         TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
	})
	if err != nil {
		t.Fatalf("ValidateConfig() returned error: %v", err)
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

func TestValidateConfigAllowsPrivateForwardTarget(t *testing.T) {
	err := ValidateConfig(Config{
		TLS:  TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Auth: AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
		Forwards: []ForwardConfig{{
			Name:         "lan-rdp",
			Direction:    DirectionClientToServer,
			ServerTarget: "10.20.30.40:3389",
			AllowedUsers: []string{"alice"},
		}},
	})
	if err != nil {
		t.Fatalf("ValidateConfig returned error: %v", err)
	}
}

func TestValidateConfigRejectsPublicForwardTarget(t *testing.T) {
	err := ValidateConfig(Config{
		TLS:  TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Auth: AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
		Forwards: []ForwardConfig{{
			Name:         "public-rdp",
			Direction:    DirectionClientToServer,
			ServerTarget: "203.0.113.20:3389",
			AllowedUsers: []string{"alice"},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "loopback or private IP") {
		t.Fatalf("expected public target validation error, got %v", err)
	}
}

func TestSaveConfigWritesReverseListenPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yaml")
	cfg := Config{
		TLS:  TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Auth: AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
		Forwards: []ForwardConfig{{
			Name:         "reverse-web",
			Direction:    DirectionServerToClient,
			ListenPort:   18080,
			AllowedUsers: []string{"alice"},
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
	if !strings.Contains(text, "listen_port: 18080") || strings.Contains(text, "\n              listen_addr:") {
		t.Fatalf("unexpected saved reverse config: %s", text)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Forwards[0].ListenAddr; got != "127.0.0.1:18080" {
		t.Fatalf("ListenAddr = %q, want loopback runtime address", got)
	}
}

func TestValidateConfigRequiresKnownAllowedUsers(t *testing.T) {
	base := Config{
		TLS:  TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Auth: AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
		Forwards: []ForwardConfig{{
			Name:         "rdp",
			Direction:    DirectionClientToServer,
			ServerTarget: "127.0.0.1:3389",
		}},
	}
	if err := ValidateConfig(base); err == nil || !strings.Contains(err.Error(), "at least one allowed user") {
		t.Fatalf("expected missing allowed user error, got %v", err)
	}
	base.Forwards[0].AllowedUsers = []string{"bob"}
	if err := ValidateConfig(base); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected unknown allowed user error, got %v", err)
	}
}

func TestValidateConfigRequiresUniqueSingleOwnerReversePorts(t *testing.T) {
	base := Config{
		TLS: TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Auth: AuthConfig{Users: []UserConfig{
			{Username: "alice", PasswordHash: "plain:secret"},
			{Username: "bob", PasswordHash: "plain:secret"},
		}},
		Forwards: []ForwardConfig{{
			Name:         "reverse-a",
			Direction:    DirectionServerToClient,
			ListenPort:   18080,
			AllowedUsers: []string{"alice", "bob"},
		}},
	}
	if err := ValidateConfig(base); err == nil || !strings.Contains(err.Error(), "exactly one allowed user") {
		t.Fatalf("expected reverse single owner error, got %v", err)
	}
	base.Forwards[0].AllowedUsers = []string{"alice"}
	base.Forwards = append(base.Forwards, ForwardConfig{
		Name:         "reverse-b",
		Direction:    DirectionServerToClient,
		ListenPort:   18080,
		AllowedUsers: []string{"bob"},
	})
	if err := ValidateConfig(base); err == nil || !strings.Contains(err.Error(), "duplicates forward") {
		t.Fatalf("expected duplicate reverse port error, got %v", err)
	}
}

func TestValidateConfigAllowsMultipleReversePortsPerOwner(t *testing.T) {
	base := Config{
		TLS:  TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Auth: AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
		Forwards: []ForwardConfig{
			{Direction: DirectionServerToClient, ListenPort: 18080, AllowedUsers: []string{"alice"}},
			{Name: "web", Direction: DirectionServerToClient, ListenPort: 18081, AllowedUsers: []string{"alice"}},
		},
	}
	if err := ValidateConfig(base); err != nil {
		t.Fatalf("ValidateConfig() rejected multiple reverse ports for one owner: %v", err)
	}
}

func TestLoadConfigRejectsNonLoopbackLegacyReverseAddress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yaml")
	data := []byte(`tls:
  cert_file: server.crt
  key_file: server.key
forwards:
  - name: reverse-web
    direction: server_to_client
    listen_port: 18080
    listen_addr: 0.0.0.0:18080
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "listen_addr must be a loopback address") {
		t.Fatalf("expected non-loopback legacy address error, got %v", err)
	}
}
