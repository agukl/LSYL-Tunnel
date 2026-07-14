//go:build windows

package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lsyltunnel/src/client/tunnel"
	"lsyltunnel/src/internal/protocol"
)

func TestHasPasswordState(t *testing.T) {
	tests := []struct {
		name string
		cfg  tunnel.Config
		want bool
	}{
		{
			name: "empty",
			cfg:  tunnel.Config{},
			want: false,
		},
		{
			name: "plain password",
			cfg:  tunnel.Config{Password: "secret"},
			want: true,
		},
		{
			name: "password env",
			cfg:  tunnel.Config{PasswordEnv: "LSYL_PASSWORD"},
			want: true,
		},
		{
			name: "password file",
			cfg:  tunnel.Config{PasswordFile: "password.txt"},
			want: true,
		},
		{
			name: "saved credential",
			cfg: tunnel.Config{SavedCredential: protocol.SealedCredential{
				Type:       "server_sealed",
				KeyID:      "login-key-1",
				ExpiresAt:  "2026-08-20T00:00:00+08:00",
				Ciphertext: "sealed",
			}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{configPath: filepath.Join(t.TempDir(), "conf", "client.yaml")}
			if err := app.saveClientConfig(tt.cfg); err != nil {
				t.Fatal(err)
			}
			if got := app.hasPasswordState(); got != tt.want {
				t.Fatalf("hasPasswordState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrepareLoginConfigPreservesForwardTargets(t *testing.T) {
	app := &App{configPath: filepath.Join(t.TempDir(), "conf", "client.yaml")}
	original := tunnel.Config{
		ServerAddr: "old.example.com:3443",
		Username:   "old-user",
		TLS:        tunnel.TLSConfig{CACertFile: "../cert/server.crt"},
		Forwards: []tunnel.ForwardConfig{{
			Name:         "ssh",
			ListenAddr:   "127.0.0.1:2200",
			ServerTarget: "10.20.30.40:22",
		}},
	}
	if err := app.saveClientConfig(original); err != nil {
		t.Fatal(err)
	}

	got, err := app.prepareLoginConfig(loginForm{
		ServerAddr: "new.example.com:3443",
		Username:   "new-user",
		Password:   "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ServerAddr != "new.example.com:3443" || got.Username != "new-user" {
		t.Fatalf("login fields were not updated: server_addr=%q username=%q", got.ServerAddr, got.Username)
	}
	if len(got.Forwards) != 1 || got.Forwards[0].ServerTarget != "10.20.30.40:22" {
		t.Fatalf("forward target changed while preparing login: %+v", got.Forwards)
	}
}

func TestPrepareLoginConfigDoesNotReplaceInvalidExistingConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "conf", "client.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte(`server_addr: vpn.example.com:3443
username: alice
forwards:
  - name: mysql
    direction: server_to_client
    listen_addr: 127.0.0.1:3307
    listen_port: 3307
    server_target: 127.0.0.1:3307
`)
	if err := os.WriteFile(configPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{configPath: configPath}
	_, err := app.prepareLoginConfig(loginForm{
		ServerAddr: "new.example.com:3443",
		Username:   "bob",
		Password:   "secret",
	})
	if err == nil || !strings.Contains(err.Error(), `forward "mysql"`) || !strings.Contains(err.Error(), "must not include listen_port") {
		t.Fatalf("prepareLoginConfig() error = %v, want existing rule error", err)
	}
	after, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Fatalf("prepareLoginConfig() replaced invalid config:\n%s", after)
	}
}

func TestPrepareLoginConfigUsesDefaultsOnlyWhenConfigIsMissing(t *testing.T) {
	app := &App{configPath: filepath.Join(t.TempDir(), "conf", "client.yaml")}
	cfg, err := app.prepareLoginConfig(loginForm{
		ServerAddr: "vpn.example.com:3443",
		Username:   "alice",
		Password:   "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerAddr != "vpn.example.com:3443" || cfg.Username != "alice" || cfg.Password != "secret" {
		t.Fatalf("prepareLoginConfig() = %+v, want form values on a new config", cfg)
	}
}

func TestRuntimeConfigRejectsMissingForwardTarget(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "conf", "client.yaml")
	cfg := tunnel.Config{
		ServerAddr: "vpn.example.com:3443",
		Username:   "alice",
		Password:   "secret",
		TLS:        tunnel.TLSConfig{InsecureSkipVerify: true},
		Forwards: []tunnel.ForwardConfig{{
			Name:       "ssh",
			ListenAddr: "127.0.0.1:2200",
		}},
	}
	_, err := runtimeClientConfigFromRaw(configPath, cfg)
	if err == nil || !strings.Contains(err.Error(), "requires server_target") {
		t.Fatalf("runtimeClientConfigFromRaw() error = %v, want missing server_target", err)
	}
}

func TestRuntimeConfigAcceptsReverseListenAddress(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "conf", "client.yaml")
	cfg := tunnel.Config{
		ServerAddr: "vpn.example.com:3443",
		Username:   "alice",
		Password:   "secret",
		TLS:        tunnel.TLSConfig{InsecureSkipVerify: true},
		Forwards: []tunnel.ForwardConfig{{
			Name:         "mysql",
			Direction:    tunnel.DirectionServerToClient,
			ListenAddr:   "127.0.0.1:3307",
			ServerTarget: "127.0.0.1:3307",
		}},
	}
	got, err := runtimeClientConfigFromRaw(configPath, cfg)
	if err != nil {
		t.Fatalf("runtimeClientConfigFromRaw() rejected reverse listen_addr: %v", err)
	}
	if got.Forwards[0].ListenAddr != "127.0.0.1:3307" {
		t.Fatalf("runtimeClientConfigFromRaw() listen_addr = %q", got.Forwards[0].ListenAddr)
	}
}

func TestRouteSummaryKeepsClientEndpointOnLeft(t *testing.T) {
	app := &App{configPath: filepath.Join(t.TempDir(), "conf", "client.yaml")}
	if err := app.saveClientConfig(tunnel.Config{Forwards: []tunnel.ForwardConfig{
		{
			Name:         "SSH",
			ListenAddr:   "127.0.0.1:2200",
			ServerTarget: "10.20.30.40:22",
		},
		{
			Name:         "数据库",
			Direction:    tunnel.DirectionServerToClient,
			ListenAddr:   "127.0.0.1:13306",
			ServerTarget: "127.0.0.1:3306",
		},
	}}); err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"SSH: 2200 → 10.20.30.40:22",
		"数据库: 3306 ← 13306",
	}, "\n")
	if got := app.routeSummary(); got != want {
		t.Fatalf("routeSummary() = %q, want %q", got, want)
	}
}
