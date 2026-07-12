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

func TestCheckConfigUpgradeCompatibleAcceptsCompleteV1Config(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, "server.yaml", validServerConfigV1YAML(""))

	if err := CheckConfigUpgradeCompatible(path); err != nil {
		t.Fatalf("expected complete V1 config to be upgrade-compatible, got %v", err)
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
	current := writeTestConfig(t, dir, "server.yaml", validServerConfigV1YAML(`
compatibility:
  min_client_version: "2.1.0"
  max_client_version: "2.0.0"
  protocol_version: 2
`))

	err := CheckConfigUpgradeCompatible(current)
	if err == nil || !strings.Contains(err.Error(), "min_client_version must be <= max_client_version") {
		t.Fatalf("expected invalid client range error, got %v", err)
	}
}

func TestCheckConfigUpgradeCompatibleRejectsUnknownLegacyField(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, "server.yaml", `
tls:
  cert_file: ../certs/server.crt
  key_file: ../certs/server.key
auth:
  users: []
unknown_setting: true
`)

	err := CheckConfigUpgradeCompatible(path)
	if err == nil || !strings.Contains(err.Error(), "field unknown_setting not found") {
		t.Fatalf("expected unknown legacy field error, got %v", err)
	}
}

func TestCheckConfigUpgradeCompatibleRejectsV1MissingRequiredStructure(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, "server.yaml", strings.Replace(validServerConfigV1YAML(""), "runtime:\n  state_file: data/server-state.json\n  permanent_block_file: data/server-permanent-block.txt\n  request_log_file: logs/request/request.jsonl\n  business_log_file: logs/business/business.jsonl\n  entry_traffic_log_file: logs/entry-traffic/entry-traffic.jsonl\n  flow_traffic_log_file: logs/flow-traffic/flow-traffic.jsonl\n  recent_events: 500\n", "", 1))

	err := CheckConfigUpgradeCompatible(path)
	if err == nil || !strings.Contains(err.Error(), "missing required field runtime") {
		t.Fatalf("expected missing V1 structure error, got %v", err)
	}
}

func TestCheckConfigUpgradeCompatibleRejectsV1UnknownNestedField(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, "server.yaml", strings.Replace(validServerConfigV1YAML(""), "  min_version: \"1.3\"", "  min_version: \"1.3\"\n  extra: true", 1))

	err := CheckConfigUpgradeCompatible(path)
	if err == nil || !strings.Contains(err.Error(), "field extra not found") {
		t.Fatalf("expected unknown nested field error, got %v", err)
	}
}

func TestCheckConfigUpgradeCompatibleRejectsV1WrongStructure(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, "server.yaml", strings.Replace(validServerConfigV1YAML(""), "auth:\n  users: []", "auth: []", 1))

	err := CheckConfigUpgradeCompatible(path)
	if err == nil || !strings.Contains(err.Error(), "cannot unmarshal") {
		t.Fatalf("expected wrong V1 structure error, got %v", err)
	}
}

func TestCheckConfigUpgradeCompatibleRejectsV1InvalidForwardAuthorization(t *testing.T) {
	dir := t.TempDir()
	config := validServerConfigV1YAML("")
	config = strings.Replace(config, "auth:\n  users: []", "auth:\n  users:\n    - username: alice\n      password_hash: plain:secret\n      disabled: false", 1)
	config = strings.Replace(config, "forwards: []", "forwards:\n  - name: rdp\n    direction: client_to_server\n    server_target: 127.0.0.1:3389\n    allowed_users:\n      - bob", 1)
	path := writeTestConfig(t, dir, "server.yaml", config)

	err := CheckConfigUpgradeCompatible(path)
	if err == nil || !strings.Contains(err.Error(), "allowed user bob does not exist") {
		t.Fatalf("expected invalid forward authorization error, got %v", err)
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

func validServerConfigV1YAML(override string) string {
	config := `
config_version: 1
requires:
  min_server_version: "2.0.0"
compatibility:
  min_client_version: "2.0.0"
  max_client_version: ""
  protocol_version: 2
listen_addr: 0.0.0.0:3443
monitor_addr: 127.0.0.1:19111
log_level: info
tls:
  cert_file: ../certs/server.crt
  key_file: ../certs/server.key
  min_version: "1.3"
auth:
  users: []
forwards: []
security:
  handshake_timeout_sec: 8
  dial_timeout_sec: 5
  max_handshake_bytes: 32768
  max_concurrent_connections: 2048
  max_concurrent_connections_per_ip: 128
  connection_rate_window_sec: 1
  max_new_connections_per_ip_window: 120
  max_concurrent_streams_per_user: 0
  stream_rate_limit_bytes_per_sec: 0
  auth_fail_window_sec: 300
  auth_fail_threshold: 8
  auth_fail_block_sec: 1800
credential_seal:
  keys: []
runtime:
  state_file: data/server-state.json
  permanent_block_file: data/server-permanent-block.txt
  request_log_file: logs/request/request.jsonl
  business_log_file: logs/business/business.jsonl
  entry_traffic_log_file: logs/entry-traffic/entry-traffic.jsonl
  flow_traffic_log_file: logs/flow-traffic/flow-traffic.jsonl
  recent_events: 500
`
	if strings.TrimSpace(override) == "" {
		return config
	}
	start := strings.Index(config, "compatibility:\n")
	end := strings.Index(config[start:], "listen_addr:")
	if start < 0 || end < 0 {
		return config
	}
	end += start
	return config[:start] + strings.TrimSpace(override) + "\n" + config[end:]
}
