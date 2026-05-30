//go:build windows

package gui

import (
	"errors"
	"path/filepath"
	"testing"

	"lsyltunnel/src/client/tunnel"
	"lsyltunnel/src/internal/protocol"
)

func TestRuntimeStatusTextReconnectsInsteadOfConnected(t *testing.T) {
	got := runtimeStatusText(tunnelStatsForTest("server_unavailable"))
	if got != "正在重连" {
		t.Fatalf("runtimeStatusText() = %q, want 正在重连", got)
	}
}

func TestFriendlyErrorText(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "missing server cert",
			raw:  `open C:\Program Files\LSYL Tunnel Client\cert\server.crt: The system cannot find the file specified.`,
			want: "缺少服务端信任证书 server.crt，请联系管理员重新下发客户端安装包。",
		},
		{
			name: "invalid cert file",
			raw:  `no server TLS trust data found in C:\Program Files\LSYL Tunnel Client\cert\server.crt`,
			want: "服务端信任证书无效，请联系管理员重新下发。",
		},
		{
			name: "cert name mismatch",
			raw:  `x509: certificate is valid for localhost, not vpn.example.com`,
			want: "服务端证书和当前地址不匹配，请检查服务端地址或重新下发证书。",
		},
		{
			name: "missing credentials",
			raw:  "username and password are required",
			want: "请输入用户名和密码。",
		},
		{
			name: "wrong password",
			raw:  "username or password is incorrect",
			want: "用户名或密码不正确。",
		},
		{
			name: "missing server address",
			raw:  "server_addr is required",
			want: "请输入服务端地址。",
		},
		{
			name: "server refused",
			raw:  "dial tcp 127.0.0.1:9443: connectex: No connection could be made because the target machine actively refused it.",
			want: "连接不上服务端，请检查服务端是否启动或地址端口是否正确。",
		},
		{
			name: "local port busy",
			raw:  "listen 127.0.0.1:18080: bind: Only one usage of each socket address is normally permitted.",
			want: "本地端口已被占用，请关闭占用程序或调整端口。",
		},
		{
			name: "target denied",
			raw:  "target_denied: user is not allowed to access this target",
			want: "当前账号没有访问该目标的权限。",
		},
		{
			name: "target unreachable",
			raw:  "target_unreachable: target service is unreachable",
			want: "服务端无法访问目标服务，请联系管理员检查目标服务或防火墙。",
		},
		{
			name: "uac cancelled",
			raw:  "管理员授权已取消",
			want: "管理员授权已取消",
		},
		{
			name: "unknown error",
			raw:  "some obscure low level error",
			want: "连接失败，请检查服务端地址、账号密码和网络后重试。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := friendlyErrorText(tt.raw); got != tt.want {
				t.Fatalf("friendlyErrorText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShouldClearSavedPasswordState(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "wrong password", err: errors.New("auth_failed: username or password is incorrect"), want: true},
		{name: "expired credential", err: errors.New("credential_expired: saved login has expired"), want: true},
		{name: "server down", err: errors.New("connectex: actively refused"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldClearSavedPasswordState(tt.err); got != tt.want {
				t.Fatalf("shouldClearSavedPasswordState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClearSavedPasswordState(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "conf", "client.yaml")
	app := &App{configPath: configPath}
	cfg := tunnel.Config{
		ServerAddr: "127.0.0.1:3443",
		Username:   "alice",
		Password:   "stale-password",
		SavedCredential: protocol.SealedCredential{
			Type:       "server_sealed",
			KeyID:      "login-key-old",
			ExpiresAt:  "2026-08-20T00:00:00+08:00",
			Ciphertext: "sealed",
		},
		TLS:        tunnel.TLSConfig{CACertFile: "../cert/server.crt"},
		Connection: tunnel.ConnectionConfig{DialTimeoutSec: 5},
		Forwards: []tunnel.ForwardConfig{{
			Name:         "web",
			ListenAddr:   "127.0.0.1:18080",
			ServerTarget: "127.0.0.1:80",
		}},
	}
	if err := app.saveClientConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := app.clearSavedPasswordState(); err != nil {
		t.Fatal(err)
	}
	got, err := readClientConfigRaw(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.Password != "" {
		t.Fatalf("password was not cleared: %q", got.Password)
	}
	if got.SavedCredential.Ciphertext != "" {
		t.Fatalf("saved credential was not cleared: %+v", got.SavedCredential)
	}
	if got.Username != "alice" || got.ServerAddr != "127.0.0.1:3443" {
		t.Fatalf("unexpected config change: username=%q server=%q", got.Username, got.ServerAddr)
	}
}

func tunnelStatsForTest(health string) tunnel.ClientStats {
	return tunnel.ClientStats{Health: tunnel.HealthStatus{State: health}}
}
