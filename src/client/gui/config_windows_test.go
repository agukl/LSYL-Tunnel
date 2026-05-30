//go:build windows

package gui

import (
	"path/filepath"
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
