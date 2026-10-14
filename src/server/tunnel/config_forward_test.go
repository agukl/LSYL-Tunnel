package tunnel

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func reverseConfigForTest() Config {
	return Config{
		TLS:      TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Auth:     AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
		Forwards: []ForwardConfig{{Name: "reverse", Direction: DirectionServerToClient, ListenPort: 18080, ListenAddr: "127.0.0.1:18081", AllowedUsers: []string{"alice"}}},
	}
}

func TestReverseConflictingFieldsAreRejectedWithoutRewriting(t *testing.T) {
	for _, port := range []int{18080, -1, 65536} {
		cfg := reverseConfigForTest()
		cfg.Forwards[0].ListenPort = port
		before := cfg.Forwards[0]
		ApplyDefaults(&cfg)
		if !reflect.DeepEqual(before, cfg.Forwards[0]) {
			t.Fatal("defaults hid conflicting input")
		}
		body, err := yaml.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "server.yaml")
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		for _, check := range []func() error{
			func() error { return ValidateConfig(cfg) },
			func() error { _, err := LoadConfig(path); return err },
			func() error { return CheckConfigUpgradeCompatible(path) },
			func() error { return SaveConfig(path, cfg) },
		} {
			if err := check(); err == nil || !strings.Contains(err.Error(), "listen_port") {
				t.Fatalf("expected port-specific error, got %v", err)
			}
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(body) {
			t.Fatal("invalid file changed", err)
		}
	}
}

func TestReverseLegacyFieldsRemainCompatibleAndSaveDoesNotMutate(t *testing.T) {
	for _, port := range []int{0, 18081} {
		cfg := reverseConfigForTest()
		cfg.Forwards[0].ListenPort = port
		before := cfg.Forwards[0]
		path := filepath.Join(t.TempDir(), "server.yaml")
		if err := SaveConfig(path, cfg); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, cfg.Forwards[0]) {
			t.Fatal("save mutated the caller's rule")
		}
		loaded, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Forwards[0].ListenPort != 18081 || loaded.Forwards[0].ListenAddr != before.ListenAddr {
			t.Fatalf("legacy port changed: %+v", loaded.Forwards[0])
		}
	}
}

func TestRuntimeRejectsUnknownForwardFields(t *testing.T) {
	data := strings.Replace(validServerConfigV1YAML(""), "forwards: []", "forwards:\n  - name: bad\n    direction: client_to_server\n    server_target: 127.0.0.1:3389\n    virtual_ip: 192.0.2.10\n    allowed_users: []", 1)
	path := filepath.Join(t.TempDir(), "server.yaml")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	for _, check := range []func() error{
		func() error { _, err := LoadConfig(path); return err },
		func() error { return CheckConfigUpgradeCompatible(path) },
	} {
		if err := check(); err == nil || !strings.Contains(err.Error(), "unknown field") || !strings.Contains(err.Error(), "virtual_ip") {
			t.Fatalf("expected unknown rule field error, got %v", err)
		}
	}
}
