package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lsyltunnel/src/internal/protocol"
	appversion "lsyltunnel/src/internal/version"
)

func TestCheckConfigUpgradeCompatibleRejectsUnversionedConfig(t *testing.T) {
	dir := t.TempDir()
	current := writeTestConfig(t, dir, "server.yaml", `
tls:
  cert_file: ../certs/server.crt
  key_file: ../certs/server.key
auth:
  users: []
`)

	if err := CheckConfigUpgradeCompatible(current); err == nil || !strings.Contains(err.Error(), "config_version 0 is no longer supported") {
		t.Fatalf("expected unversioned config rejection, got %v", err)
	}
}

func TestCheckConfigUpgradeCompatibleAcceptsCompleteV1Config(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, "server.yaml", validServerConfigV1YAML(""))

	if err := CheckConfigUpgradeCompatible(path); err != nil {
		t.Fatalf("expected complete V1 config to be upgrade-compatible, got %v", err)
	}
}

func TestPrepareServerConfigMigration(t *testing.T) {
	path := writeTestConfig(t, t.TempDir(), "server.yaml", `
config_version: 1
tls:
  cert_file: ../certs/server.crt
  key_file: ../certs/server.key
auth:
  users: []
security:
  max_connections_per_ip_per_window: 27
runtime:
  flow_traffic_log_file: old-flow.jsonl
log_level: info
`)
	file, err := PrepareConfigMigration(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(file.Data), "max_connections_per_ip_per_window:") ||
		strings.Contains(string(file.Data), "flow_traffic_log_file:") ||
		strings.Contains(string(file.Data), "log_level:") {
		t.Fatalf("deprecated fields remain: %s", file.Data)
	}
	if !strings.Contains(string(file.Data), "max_new_connections_per_ip_window: 27") {
		t.Fatalf("known alias was not migrated: %s", file.Data)
	}
	if err := validateServerConfigV1Structure(file.Data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, file.Data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigRaw(path); err != nil {
		t.Fatalf("migrated server config cannot be loaded: %v", err)
	}
	if err := os.WriteFile(path, []byte("config_version: 1\nsecurty: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareConfigMigration(path); err == nil {
		t.Fatal("unknown field should prevent migration")
	}
	if err := os.WriteFile(path, []byte("config_version: 1\ntls: null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareConfigMigration(path); err == nil {
		t.Fatal("wrong mapping shape should prevent migration")
	}
}

func TestSaveConfigProducesCurrentServerStructure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yaml")
	if err := SaveConfig(path, Config{TLS: TLSConfig{
		CertFile: "server.crt",
		KeyFile:  "server.key",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := CheckConfigUpgradeCompatible(path); err != nil {
		t.Fatalf("saved config does not satisfy the current schema: %v", err)
	}
}

func TestServerConfigLoadAndUpgradeUseSameSchema(t *testing.T) {
	v1 := validServerConfigV1YAML("")
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "legacy", content: "tls:\n  cert_file: server.crt\n  key_file: server.key\nauth:\n  users: []\n", wantErr: "config_version 0 is no longer supported"},
		{name: "current", content: v1},
		{name: "unknown current field", content: strings.Replace(v1, "  min_version: \"1.3\"", "  min_version: \"1.3\"\n  extra: true", 1), wantErr: "field extra not found"},
		{name: "missing current structure", content: strings.Replace(v1, "auth:\n  users: []", "", 1), wantErr: "missing required field auth"},
		{name: "wrong current layer", content: strings.Replace(v1, "auth:\n  users: []", "auth: []", 1), wantErr: "cannot unmarshal"},
		{name: "future version", content: "config_version: 99\n", wantErr: "requires a newer server"},
		{name: "multiple documents", content: v1 + "\n---\nlisten_addr: 127.0.0.1:3443\n", wantErr: "exactly one YAML document"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, t.TempDir(), "server.yaml", tt.content)
			_, loadErr := LoadConfig(path)
			upgradeErr := CheckConfigUpgradeCompatible(path)
			if (loadErr == nil) != (upgradeErr == nil) {
				t.Fatalf("load error = %v, upgrade error = %v", loadErr, upgradeErr)
			}
			if tt.wantErr != "" && (loadErr == nil || !strings.Contains(loadErr.Error(), tt.wantErr) || !strings.Contains(upgradeErr.Error(), tt.wantErr)) {
				t.Fatalf("load error = %v, upgrade error = %v; want %q", loadErr, upgradeErr, tt.wantErr)
			}
		})
	}
}

func TestLoadConfigRawKeepsRelativePathsAndRejectsUnknownFields(t *testing.T) {
	path := writeTestConfig(t, t.TempDir(), "server.yaml", validServerConfigV1YAML(""))
	cfg, err := LoadConfigRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(cfg.TLS.CertFile) || filepath.IsAbs(cfg.Runtime.StateFile) {
		t.Fatalf("raw config resolved paths: cert=%q state=%q", cfg.TLS.CertFile, cfg.Runtime.StateFile)
	}
	path = writeTestConfig(t, t.TempDir(), "invalid.yaml", strings.Replace(validServerConfigV1YAML(""), "  min_version: \"1.3\"", "  min_version: \"1.3\"\n  extra: true", 1))
	if _, err := LoadConfigRaw(path); err == nil || !strings.Contains(err.Error(), "field extra not found") {
		t.Fatalf("unknown field was accepted: %v", err)
	}
}

func TestCheckConfigUpgradeCompatibleAcceptsV1WithoutDeprecatedLogLevel(t *testing.T) {
	path := writeTestConfig(t, t.TempDir(), "server.yaml", strings.Replace(validServerConfigV1YAML(""), "log_level: info\n", "", 1))
	if err := CheckConfigUpgradeCompatible(path); err != nil {
		t.Fatalf("expected V1 config without log_level to be upgrade-compatible, got %v", err)
	}
}

func TestCheckConfigUpgradeCompatibleAcceptsV1WithoutDeprecatedFlowLog(t *testing.T) {
	path := writeTestConfig(t, t.TempDir(), "server.yaml", strings.Replace(validServerConfigV1YAML(""), "  flow_traffic_log_file: logs/flow-traffic/flow-traffic.jsonl\n", "", 1))
	if err := CheckConfigUpgradeCompatible(path); err != nil {
		t.Fatalf("expected V1 config without flow_traffic_log_file to be upgrade-compatible, got %v", err)
	}
}

func TestSaveConfigOmitsDeprecatedLogLevel(t *testing.T) {
	path := writeTestConfig(t, t.TempDir(), "server.yaml", validServerConfigV1YAML(""))
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "log_level:") {
		t.Fatalf("saved config still contains deprecated log_level: %s", data)
	}
	if strings.Contains(string(data), "flow_traffic_log_file:") {
		t.Fatalf("saved config still contains deprecated flow_traffic_log_file: %s", data)
	}
	if err := CheckConfigUpgradeCompatible(path); err != nil {
		t.Fatalf("saved config is not upgrade-compatible: %v", err)
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
  protocol_version: 3
`))

	err := CheckConfigUpgradeCompatible(current)
	if err == nil || !strings.Contains(err.Error(), "min_client_version must be <= max_client_version") {
		t.Fatalf("expected invalid client range error, got %v", err)
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
  min_client_version: "3.0.0"
  max_client_version: ""
  protocol_version: 3
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
