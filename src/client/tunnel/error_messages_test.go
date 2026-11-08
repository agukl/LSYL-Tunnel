package tunnel

import (
	"errors"
	"testing"
	"time"
)

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
			name:      "temporary auth block",
			err:       errors.New("auth_blocked: too many login failures, temporarily blocked; try later"),
			want:      "登录失败次数过多，当前来源 IP 已被临时封禁，请稍后再试",
			permanent: false,
		},
		{
			name:      "account stream limit",
			err:       errors.New("user_stream_limit: too many concurrent streams for account"),
			want:      "当前账号并发连接数已达到上限，请关闭部分连接后重试",
			permanent: false,
		},
		{
			name:      "certificate",
			err:       errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
			want:      "服务端证书不受信任，请联系管理员重新下发证书",
			permanent: true,
		},
		{
			name:      "expired certificate",
			err:       errors.New("x509: certificate has expired or is not yet valid"),
			want:      "服务端证书已过期或尚未生效，请联系管理员重新下发有效证书",
			permanent: true,
		},
		{
			name:      "client version",
			err:       errors.New("client_version_unsupported: maximum client_version is 2.0.1"),
			want:      "客户端版本不在服务端允许范围内，请联系管理员确认两端版本",
			permanent: true,
		},
		{
			name:      "protocol version",
			err:       errors.New("protocol_version_unsupported: required protocol_version is 2"),
			want:      "客户端与服务端协议版本不一致，请联系管理员升级对应一端",
			permanent: true,
		},
		{
			name:      "network reset",
			err:       errors.New("wsarecv: An existing connection was forcibly closed by the remote host."),
			want:      "连接被重置，客户端会自动重试",
			permanent: false,
		},
		{
			name:      "windows read timeout",
			err:       errors.New("read tcp 10.42.159.244:62380->36.27.120.130:55443: wsarecv: A connection attempt failed because the connected party did not properly respond after a period of time, or established connection failed because connected host has failed to respond."),
			want:      "连接超时，客户端会自动重试",
			permanent: false,
		},
		{
			name:      "certificate gate eof",
			err:       errors.New("read certificate possession gate challenge: EOF"),
			want:      "连接意外结束，客户端会自动重试",
			permanent: false,
		},
		{
			name:      "tls handshake timeout after gate",
			err:       errors.New("TLS handshake after certificate possession gate: read tcp 127.0.0.1:50000->127.0.0.1:55443: i/o timeout"),
			want:      "连接超时，客户端会自动重试",
			permanent: false,
		},
		{
			name:      "tls server alert after gate",
			err:       errors.New("TLS handshake after certificate possession gate: remote error: tls: internal error"),
			want:      "TLS 握手失败，客户端会自动重试；若持续出现请确认两端版本",
			permanent: false,
		},
		{
			name:      "server passive port unavailable",
			err:       errors.New("reverse_failed: server passive port is unavailable: listen reverse 127.0.0.1:18080: bind: Only one usage of each socket address is normally permitted."),
			want:      "服务端被动端口不可用，客户端会自动重试，请联系管理员检查服务端端口占用",
			permanent: false,
		},
		{
			name:      "reverse listener is not configured",
			err:       errors.New("reverse_failed: reverse listen address is not configured on server"),
			want:      "反向监听地址未在服务端配置，请联系管理员检查保留端口",
			permanent: true,
		},
		{
			name:      "reverse listener is not authorized",
			err:       errors.New("reverse_failed: user is not allowed to activate this reverse listen address"),
			want:      "当前账号无权使用该反向端口，请联系管理员检查端口授权",
			permanent: true,
		},
		{
			name:      "client target unavailable",
			err:       errors.New("connect client target 127.0.0.1:22: connectex: actively refused"),
			want:      "客户端目标服务不可达，请确认对应服务已启动并监听配置端口",
			permanent: false,
		},
		{
			name:      "server target unavailable",
			err:       errors.New("target_unreachable: target service is unreachable"),
			want:      "服务端无法访问目标服务，请联系管理员检查目标服务或防火墙",
			permanent: false,
		},
		{
			name:      "virtual domain name",
			err:       errors.New("virtual listen_addr does not support domain names; use an IPv4 address from the server certificate SAN"),
			want:      "虚拟入口不支持域名，请填写 :端口 或 IPv4:端口；公网 IP 必须在证书中",
			permanent: true,
		},
		{
			name:      "ambiguous virtual IP",
			err:       errors.New("server certificate has multiple usable IPv4 SANs; specify virtual listen_addr as IPv4:port"),
			want:      "服务端证书包含多个可用 IPv4，请将虚拟 listen_addr 填写为完整的 IPv4:端口",
			permanent: true,
		},
		{
			name:      "no usable virtual IP",
			err:       errors.New("server certificate has no usable non-local IPv4 SAN for automatic virtual forwarding"),
			want:      "服务端证书没有可用于自动选择的 IPv4，请显式填写内网 IPv4 或 127.0.0.1 加端口，或重新下发证书",
			permanent: true,
		},
		{
			name:      "virtual IP outside certificate SAN",
			err:       errors.New("virtual listen_addr 192.0.2.22:22 is not authorized by the server certificate IPv4 SAN"),
			want:      "填写的公网虚拟 IP 不在服务端证书 IPv4 SAN 中，请修正 listen_addr 或重新下发证书",
			permanent: true,
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
	if got := ReverseRetryDelay(errors.New("user_stream_limit: too many concurrent streams for account"), 1); got != 2*time.Second {
		t.Fatalf("ReverseRetryDelay(user_stream_limit) = %s, want 2s", got)
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
			name:      "temporary auth block",
			err:       errors.New("auth_blocked: too many login failures, temporarily blocked; try later"),
			wantState: HealthAuthError,
			wantMsg:   "登录失败次数过多，当前来源 IP 已被临时封禁",
		},
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
			name:      "expired certificate",
			err:       errors.New("x509: certificate has expired or is not yet valid"),
			wantState: HealthAuthError,
			wantMsg:   "服务端证书已过期或尚未生效，请联系管理员重新下发有效证书",
		},
		{
			name:      "client version",
			err:       errors.New("client_version_unsupported: maximum client_version is 2.0.1"),
			wantState: HealthAuthError,
			wantMsg:   "客户端版本不在服务端允许范围内，请联系管理员确认两端版本",
		},
		{
			name:      "protocol version",
			err:       errors.New("protocol_version_unsupported: required protocol_version is 2"),
			wantState: HealthAuthError,
			wantMsg:   "客户端与服务端协议版本不一致，请联系管理员升级对应一端",
		},
		{
			name:      "invalid server address",
			err:       errors.New("dial tcp: address vpn.example.com: missing port in address"),
			wantState: HealthAuthError,
			wantMsg:   "服务端地址格式不正确，请填写 地址:端口",
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
		{
			name:      "certificate gate eof",
			err:       errors.New("read certificate possession gate challenge: EOF"),
			wantState: HealthServerUnavailable,
			wantMsg:   "连接意外结束，请检查网络链路；若持续出现请查看两端日志",
		},
		{
			name:      "windows read timeout",
			err:       errors.New("read tcp 10.42.159.244:62380->36.27.120.130:55443: wsarecv: A connection attempt failed because the connected party did not properly respond after a period of time"),
			wantState: HealthServerUnavailable,
			wantMsg:   "连接超时，请检查网络或服务端防火墙",
		},
		{
			name:      "connection reset",
			err:       errors.New("wsarecv: An existing connection was forcibly closed by the remote host"),
			wantState: HealthServerUnavailable,
			wantMsg:   "连接被重置，请检查网络链路或服务端状态",
		},
		{
			name:      "tls handshake timeout after gate",
			err:       errors.New("TLS handshake after certificate possession gate: read tcp 127.0.0.1:50000->127.0.0.1:55443: i/o timeout"),
			wantState: HealthServerUnavailable,
			wantMsg:   "连接超时，请检查网络或服务端防火墙",
		},
		{
			name:      "tls server alert after gate",
			err:       errors.New("TLS handshake after certificate possession gate: remote error: tls: internal error"),
			wantState: HealthServerUnavailable,
			wantMsg:   "TLS 握手失败，请检查服务端状态及两端版本",
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
