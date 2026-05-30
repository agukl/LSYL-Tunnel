package tunnel

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestStartSoftFailsOccupiedForward(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS:        TLSConfig{InsecureSkipVerify: true},
		Forwards: []ForwardConfig{
			{Name: "bad", ListenAddr: occupied.Addr().String(), ServerTarget: "127.0.0.1:80"},
			{Name: "good", ListenAddr: "127.0.0.1:0", ServerTarget: "127.0.0.1:80"},
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.ForwardAddr("good") == "" {
		t.Fatal("good forward should be listening")
	}
	stats := client.Stats()
	if got := forwardStateForTest(stats, "bad"); got != ForwardListenFailed {
		t.Fatalf("bad forward state = %q, want %q", got, ForwardListenFailed)
	}
	if got := forwardStateForTest(stats, "good"); got != ForwardListening {
		t.Fatalf("good forward state = %q, want %q", got, ForwardListening)
	}
}

func TestStartFailsWhenNoForwardUsable(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS:        TLSConfig{InsecureSkipVerify: true},
		Forwards: []ForwardConfig{{
			Name:         "bad",
			ListenAddr:   occupied.Addr().String(),
			ServerTarget: "127.0.0.1:80",
		}},
	}, t.Logf)
	if err == nil {
		_ = client.Close()
		t.Fatal("expected no usable forward error")
	}
}

func TestForwardErrorMessageAndRetryPolicy(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		want      string
		permanent bool
	}{
		{
			name:      "permission denied",
			err:       errors.New("target_denied: user is not allowed to access this target"),
			want:      "当前账号没有访问该端口的权限，请联系管理员检查端口授权",
			permanent: true,
		},
		{
			name:      "certificate",
			err:       errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
			want:      "服务端证书校验失败，请联系管理员检查证书",
			permanent: true,
		},
		{
			name:      "network reset",
			err:       errors.New("wsarecv: An existing connection was forcibly closed by the remote host."),
			want:      "连接被断开，客户端会自动重试",
			permanent: false,
		},
		{
			name:      "server passive port unavailable",
			err:       errors.New("reverse_failed: server passive port is unavailable: listen reverse 127.0.0.1:18080: bind: Only one usage of each socket address is normally permitted."),
			want:      "服务端被动端口不可用，客户端会自动重试，请联系管理员检查服务端端口占用",
			permanent: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForwardErrorMessage(tt.err); got != tt.want {
				t.Fatalf("ForwardErrorMessage() = %q, want %q", got, tt.want)
			}
			if got := IsPermanentForwardError(tt.err); got != tt.permanent {
				t.Fatalf("IsPermanentForwardError() = %v, want %v", got, tt.permanent)
			}
		})
	}
	if got := ReverseRetryDelay(errors.New("auth_blocked: too many login failures"), 1); got != 5*time.Minute {
		t.Fatalf("ReverseRetryDelay(auth_blocked) = %s, want 5m", got)
	}
	if got := ReverseRetryDelay(errors.New("connect server failed"), 100); got != 30*time.Second {
		t.Fatalf("ReverseRetryDelay(max) = %s, want 30s", got)
	}
	if got := ReconnectDelay(1); got != 2*time.Second {
		t.Fatalf("ReconnectDelay(1) = %s, want 2s", got)
	}
	if got := ReconnectDelay(4); got != 16*time.Second {
		t.Fatalf("ReconnectDelay(4) = %s, want 16s", got)
	}
	if got := ReconnectDelay(10); got != 30*time.Second {
		t.Fatalf("ReconnectDelay(10) = %s, want 30s", got)
	}
}

func TestClassifyHealthErrorMessages(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantState string
		wantMsg   string
	}{
		{
			name:      "missing server cert",
			err:       errors.New(`open C:\Program Files\LSYL Tunnel Client\cert\server.crt: The system cannot find the file specified.`),
			wantState: HealthAuthError,
			wantMsg:   "缺少服务端信任证书 server.crt，请联系管理员重新下发客户端安装包",
		},
		{
			name:      "invalid cert file",
			err:       errors.New(`no server TLS trust data found in C:\Program Files\LSYL Tunnel Client\cert\server.crt`),
			wantState: HealthAuthError,
			wantMsg:   "服务端信任证书无效，请联系管理员重新下发",
		},
		{
			name:      "name mismatch",
			err:       errors.New("x509: certificate is valid for localhost, not vpn.example.com"),
			wantState: HealthAuthError,
			wantMsg:   "服务端证书和当前地址不匹配，请检查服务端地址或重新下发证书",
		},
		{
			name:      "dns",
			err:       errors.New("lookup vpn.example.com: no such host"),
			wantState: HealthServerUnavailable,
			wantMsg:   "服务端地址无法解析，请检查域名或网络",
		},
		{
			name:      "refused",
			err:       errors.New("dial tcp 127.0.0.1:3443: connectex: No connection could be made because the target machine actively refused it."),
			wantState: HealthServerUnavailable,
			wantMsg:   "连接不上服务端，请检查服务端是否启动或地址端口是否正确",
		},
		{
			name:      "timeout",
			err:       errors.New("i/o timeout"),
			wantState: HealthServerUnavailable,
			wantMsg:   "连接超时，请检查网络或服务端防火墙",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, msg := classifyHealthError(tt.err)
			if state != tt.wantState || msg != tt.wantMsg {
				t.Fatalf("classifyHealthError() = (%q, %q), want (%q, %q)", state, msg, tt.wantState, tt.wantMsg)
			}
		})
	}
}

func TestFinalizeHealthStatusCancelsAfterReconnectLimit(t *testing.T) {
	client := &Client{}
	status := client.finalizeHealthStatus(HealthStatus{
		State:               HealthServerUnavailable,
		Message:             "连接不上服务端，请检查服务端是否启动或地址端口是否正确",
		ConsecutiveFailures: healthMaxReconnectFailures - 1,
	})
	if status.Terminal {
		t.Fatal("status below reconnect limit should not be terminal")
	}

	status = client.finalizeHealthStatus(HealthStatus{
		State:               HealthServerUnavailable,
		Message:             "连接不上服务端，请检查服务端是否启动或地址端口是否正确",
		ConsecutiveFailures: healthMaxReconnectFailures,
	})
	if !status.Terminal {
		t.Fatal("status at reconnect limit should be terminal")
	}
	if status.Message != "多次重连失败，已取消连接状态，请确认服务端恢复后重新连接" {
		t.Fatalf("terminal message = %q", status.Message)
	}
	if got := client.Stats().Health; !got.Terminal || got.Message != status.Message {
		t.Fatalf("stored health = %+v, want terminal message %q", got, status.Message)
	}
}

func forwardStateForTest(stats ClientStats, name string) string {
	for _, item := range stats.Items {
		if item.Name == name {
			return item.State
		}
	}
	return ""
}
