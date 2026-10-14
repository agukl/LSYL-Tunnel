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

func loopbackTestConfig() Config {
	cfg := Config{
		ServerAddr: "vpn.example.test:3443",
		Username:   "alice", Password: "secret",
		TLS: TLSConfig{CACertFile: "server.crt", ServerName: "vpn.example.test"},
		Forwards: []ForwardConfig{{
			Name: "rdp", Direction: DirectionClientToServer,
			ListenAddr: "127.0.0.1:13389", ServerTarget: "192.168.1.7:3389",
		}},
	}
	ApplyDefaults(&cfg)
	return cfg
}

func TestConfigRejectsLoopbackAliasesWithoutRewriting(t *testing.T) {
	fields := []struct {
		name string
		set  func(*Config, string)
	}{
		{"server_addr", func(c *Config, h string) { c.ServerAddr = net.JoinHostPort(h, "3443") }},
		{"tls.server_name", func(c *Config, h string) { c.TLS.ServerName = h }},
		{"listen_addr", func(c *Config, h string) { c.Forwards[0].ListenAddr = net.JoinHostPort(h, "13389") }},
		{"server_target", func(c *Config, h string) { c.Forwards[0].ServerTarget = net.JoinHostPort(h, "3389") }},
		{"reverse listen_addr", func(c *Config, h string) {
			c.Forwards[0].Direction = DirectionServerToClient
			c.Forwards[0].ListenAddr = net.JoinHostPort(h, "13389")
			c.Forwards[0].ServerTarget = "127.0.0.1:3389"
		}},
	}
	for _, host := range []string{"localhost", "LOCALHOST.", "::1", "::ffff:127.0.0.1", "127.0.0.2"} {
		for _, field := range fields {
			t.Run(host+"/"+field.name, func(t *testing.T) {
				cfg := loopbackTestConfig()
				field.set(&cfg, host)
				before, err := yaml.Marshal(cfg)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(t.TempDir(), "client.yaml")
				if err := os.WriteFile(path, before, 0o600); err != nil {
					t.Fatal(err)
				}
				for _, check := range []func() error{
					func() error { return ValidateConfig(cfg) },
					func() error { _, err := LoadConfigRaw(path); return err },
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

func TestConfigAllowsCanonicalLoopbackDomainsAndWildcardListeners(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:13389", "0.0.0.0:13389", "[::]:13389"} {
		cfg := loopbackTestConfig()
		cfg.Forwards[0].ListenAddr = listen
		if err := ValidateConfig(cfg); err != nil {
			t.Fatal(err)
		}
	}
	cfg := loopbackTestConfig()
	cfg.ServerAddr = "127.0.0.1:3443"
	cfg.TLS.ServerName = "127.0.0.1"
	cfg.Forwards[0].Direction = DirectionServerToClient
	cfg.Forwards[0].ServerTarget = "127.0.0.1:3389"
	if err := ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
}
