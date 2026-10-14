package tunnel

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestServerConfigRejectsLoopbackAliasesWithoutRewriting(t *testing.T) {
	for _, host := range []string{"localhost", "LOCALHOST.", "::1", "::ffff:127.0.0.1", "127.0.0.2"} {
		for _, field := range []string{"listen_addr", "monitor_addr", "server_target", "reverse_legacy", "reverse_with_port"} {
			t.Run(host+"/"+field, func(t *testing.T) {
				cfg := Config{
					ListenAddr: "0.0.0.0:3443", MonitorAddr: "127.0.0.1:19111",
					TLS:  TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
					Auth: AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
					Forwards: []ForwardConfig{{
						Name: "rdp", Direction: DirectionClientToServer,
						ServerTarget: "127.0.0.1:3389", AllowedUsers: []string{"alice"},
					}},
				}
				switch field {
				case "listen_addr":
					cfg.ListenAddr = net.JoinHostPort(host, "3443")
				case "monitor_addr":
					cfg.MonitorAddr = net.JoinHostPort(host, "19111")
				case "server_target":
					cfg.Forwards[0].ServerTarget = net.JoinHostPort(host, "3389")
				default:
					cfg.Forwards[0].Direction = DirectionServerToClient
					cfg.Forwards[0].ServerTarget = ""
					cfg.Forwards[0].ListenAddr = net.JoinHostPort(host, "13389")
					if field == "reverse_with_port" {
						cfg.Forwards[0].ListenPort = 13389
					}
				}
				ApplyDefaults(&cfg)
				if strings.HasPrefix(field, "reverse") && cfg.Forwards[0].ListenAddr != net.JoinHostPort(host, "13389") {
					t.Fatal("defaults rewrote an unsupported loopback alias")
				}
				before, err := yaml.Marshal(cfg)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(t.TempDir(), "server.yaml")
				if err := os.WriteFile(path, before, 0o600); err != nil {
					t.Fatal(err)
				}
				for _, check := range []func() error{
					func() error { return ValidateConfig(cfg) },
					func() error { _, err := LoadConfig(path); return err },
					func() error { return CheckConfigUpgradeCompatible(path) },
					func() error { return SaveConfig(path, cfg) },
				} {
					if err := check(); err == nil || !strings.Contains(err.Error(), "use 127.0.0.1") {
						t.Fatalf("alias accepted or unclear error: %v", err)
					}
				}
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("invalid config was rewritten")
				}
			})
		}
	}
}
