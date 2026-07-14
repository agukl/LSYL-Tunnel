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
			name: "expired certificate",
			raw:  `tls: failed to verify certificate: x509: certificate has expired or is not yet valid`,
			want: "服务端证书已过期或尚未生效，请联系管理员重新下发有效证书。",
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
			name: "temporary auth block",
			raw:  "auth_blocked: too many login failures, temporarily blocked; try later",
			want: "登录失败次数过多，当前来源 IP 已被临时封禁，请稍后再试。",
		},
		{
			name: "account stream limit",
			raw:  "user_stream_limit: too many concurrent streams for account",
			want: "当前账号并发连接数已达到上限，请关闭部分连接后重试。",
		},
		{
			name: "missing server address",
			raw:  "server_addr is required",
			want: "请输入服务端地址。",
		},
		{
			name: "invalid server address",
			raw:  "dial tcp: address vpn.example.com: missing port in address",
			want: "服务端地址格式不正确，请填写 地址:端口。",
		},
		{
			name: "newer client required by config",
			raw:  "client config_version 4 requires a newer client; current client supports config_version 3",
			want: "当前配置需要更高版本的客户端，请先升级客户端。",
		},
		{
			name: "invalid config version declaration",
			raw:  "client config_version 2 requires min_client_version >= 2.1.0",
			want: "客户端配置版本声明不完整或不匹配，请联系管理员重新下发配置。",
		},
		{
			name: "unsupported direction",
			raw:  `forward "ssh" has unsupported direction`,
			want: "转发规则 \"ssh\" 配置错误：direction 不受支持，请联系管理员检查客户端配置。",
		},
		{
			name: "missing server target",
			raw:  `forward "ssh" requires server_target`,
			want: "转发规则 \"ssh\" 配置错误：缺少业务目标 server_target，请联系管理员重新生成客户端配置。",
		},
		{
			name: "invalid server target",
			raw:  `forward "ssh" server_target must include a host and port`,
			want: "转发规则 \"ssh\" 配置错误：转发目标格式不正确，server_target 必须填写为 地址:端口。",
		},
		{
			name: "reverse client listen port is forbidden",
			raw:  `forward "mysql" server_to_client client config must not include listen_port; use listen_addr and server_target`,
			want: "转发规则 \"mysql\" 配置错误：客户端反向规则使用 listen_addr 选择服务端端口，请删除 listen_port。",
		},
		{
			name: "missing reverse listen address",
			raw:  `forward "mysql" server_to_client requires listen_addr`,
			want: "转发规则 \"mysql\" 配置错误：反向转发缺少服务端监听地址 listen_addr。",
		},
		{
			name: "invalid reverse listen address",
			raw:  `forward "mysql" reverse listen_addr must include a loopback host and port`,
			want: "转发规则 \"mysql\" 配置错误：反向 listen_addr 必须填写为服务端回环地址:端口。",
		},
		{
			name: "invalid reverse listen port",
			raw:  `forward "mysql" reverse listen_addr port must be between 1 and 65535`,
			want: "转发规则 \"mysql\" 配置错误：反向 listen_addr 端口必须在 1 到 65535 之间。",
		},
		{
			name: "non-loopback reverse listen address",
			raw:  `forward "mysql" reverse listen_addr must use a loopback address`,
			want: "转发规则 \"mysql\" 配置错误：反向 listen_addr 必须使用服务端回环地址。",
		},
		{
			name: "missing reverse listener in request",
			raw:  "reverse listen address is required",
			want: "反向转发缺少服务端监听地址 listen_addr，请检查客户端配置。",
		},
		{
			name: "reverse listener is not configured",
			raw:  "reverse listen address is not configured on server",
			want: "反向 listen_addr 未在服务端配置，请联系管理员检查服务端保留端口。",
		},
		{
			name: "reverse listener is not authorized",
			raw:  "user is not allowed to activate this reverse listen address",
			want: "当前账号无权使用该反向端口，请联系管理员检查端口授权。",
		},
		{
			name: "reverse target is not loopback",
			raw:  `forward "mysql" reverse server_target must use a loopback address`,
			want: "转发规则 \"mysql\" 配置错误：反向转发目标 server_target 必须使用客户端回环地址。",
		},
		{
			name: "missing ordinary forward fields",
			raw:  `forward "web" requires listen_addr and server_target`,
			want: "转发规则 \"web\" 配置错误：正向转发缺少客户端入口 listen_addr 或服务端目标 server_target。",
		},
		{
			name: "no forward configured",
			raw:  "at least one forward is required",
			want: "客户端未配置任何转发规则，请联系管理员检查配置。",
		},
		{
			name: "virtual domain name",
			raw:  "virtual listen_addr does not support domain names; use an IPv4 address from the server certificate SAN",
			want: "虚拟入口不支持域名，请填写 :端口，或填写服务端证书中的 IPv4:端口。",
		},
		{
			name: "ambiguous virtual IP",
			raw:  "server certificate has multiple usable IPv4 SANs; specify virtual listen_addr as IPv4:port",
			want: "服务端证书包含多个可用 IPv4，请将虚拟 listen_addr 填写为完整的 IPv4:端口。",
		},
		{
			name: "invalid virtual format",
			raw:  "no usable forward is available: virtual listen_addr must use :port or IPv4:port",
			want: "虚拟入口格式不正确，请填写 :端口 或 IPv4:端口。",
		},
		{
			name: "invalid virtual port",
			raw:  "virtual listen_addr port must be between 1 and 65535",
			want: "虚拟入口端口必须在 1 到 65535 之间。",
		},
		{
			name: "virtual IPv6",
			raw:  "virtual listen_addr host must be an IPv4 address",
			want: "虚拟入口不支持 IPv6，请填写 :端口，或填写服务端证书中的 IPv4:端口。",
		},
		{
			name: "virtual local address",
			raw:  "virtual listen_addr host must be a usable non-local IPv4 address",
			want: "虚拟入口必须使用可用的非本地 IPv4，不能使用回环、链路本地或保留地址。",
		},
		{
			name: "virtual tunnel port conflict",
			raw:  "forward ssh virtual listen_addr cannot use the server_addr port",
			want: "虚拟入口端口不能与客户端连接服务端的端口相同，请更换业务端口。",
		},
		{
			name: "duplicate virtual endpoint",
			raw:  "forward ssh-2 virtual endpoint 203.0.113.10:22 duplicates forward ssh-1",
			want: "存在重复的虚拟入口 IP:端口，请为每条虚拟转发配置不同端点。",
		},
		{
			name: "virtual endpoint limit",
			raw:  "virtual forwarding supports at most 48 endpoints",
			want: "虚拟转发最多支持 48 个端点，请减少配置数量。",
		},
		{
			name: "certificate has no IPv4 SAN",
			raw:  "server certificate has no IPv4 SAN for virtual forwarding",
			want: "服务端证书没有 IPv4 SAN，无法配置虚拟入口，请重新下发证书。",
		},
		{
			name: "virtual IP outside certificate",
			raw:  "virtual listen_addr 203.0.113.10:22 is not authorized by the server certificate IPv4 SAN",
			want: "填写的虚拟 IP 不在服务端证书 IPv4 SAN 中，请修正 listen_addr 或重新下发证书。",
		},
		{
			name: "WinDivert load failure",
			raw:  "load WinDivert.dll: The specified module could not be found",
			want: "虚拟端点接管组件加载失败，请修复或重新安装标准客户端。",
		},
		{
			name: "WinDivert runtime failure",
			raw:  "WinDivert checksum update failed: invalid data",
			want: "虚拟流量接管已中断，请重新连接；若持续出现请检查 WinDivert 驱动状态。",
		},
		{
			name: "virtual local bridge unavailable",
			raw:  "no usable forward is available: allocate virtual local redirect listener: bind failed",
			want: "无法分配虚拟转发的本地桥接端口，请关闭占用大量端口的程序后重新连接。",
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
			name: "client version rejected",
			raw:  "client_version_unsupported: maximum client_version is 2.0.1",
			want: "客户端版本不在服务端允许范围内，请联系管理员确认两端版本。",
		},
		{
			name: "protocol version rejected",
			raw:  "protocol_version_unsupported: required protocol_version is 2",
			want: "客户端与服务端协议版本不一致，请联系管理员升级对应一端。",
		},
		{
			name: "unknown config field",
			raw:  "yaml: unmarshal errors: field legacy_mode not found in type tunnel.Config",
			want: "配置文件结构与当前客户端不兼容，请联系管理员重新下发配置。",
		},
		{
			name: "unknown forward field",
			raw:  `forward "ssh" contains unknown field "virtual_ip"`,
			want: "转发规则 \"ssh\" 配置错误：包含当前客户端不支持的字段，请联系管理员重新生成配置。",
		},
		{
			name: "missing config structure",
			raw:  "client config is missing required field requires",
			want: "配置文件结构与当前客户端不兼容，请联系管理员重新下发配置。",
		},
		{
			name: "uac cancelled",
			raw:  "管理员授权已取消",
			want: "管理员授权已取消",
		},
		{
			name: "unknown error",
			raw:  "some obscure low level error",
			want: "连接失败，暂时无法识别具体原因；请重试，若持续出现请联系管理员查看客户端日志。",
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

func TestMobileExportFriendlyError(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "invalid credential structure", raw: "saved_credential.key_id is required", want: "当前登录凭据不可用于移动端导出，请重新输入密码连接成功后再试。"},
		{name: "invalid server address", raw: "server_addr is invalid: missing port in address", want: "客户端配置中的服务端地址格式不正确，应填写为 地址:端口。"},
		{name: "unsupported virtual rule", raw: `forward "ssh" is virtual; mobile export only supports client_to_server`, want: "移动端只支持正向转发，当前配置包含反向转发或 IP 接管规则，无法导出。"},
		{name: "tls version", raw: "mobile profile requires tls.min_version 1.3", want: "移动端要求使用 TLS 1.3，请联系管理员更新客户端配置。"},
		{name: "duplicate listen", raw: "duplicate mobile listen address: 127.0.0.1:8080", want: "存在重复的移动端监听端口，请联系管理员调整配置。"},
		{name: "invalid target", raw: `forward "web" server_target is invalid: missing port in address`, want: "转发目标地址格式不正确，应填写为 地址:端口。"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mobileExportFriendlyError(errors.New(tt.raw)); got != tt.want {
				t.Fatalf("mobileExportFriendlyError() = %q, want %q", got, tt.want)
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
