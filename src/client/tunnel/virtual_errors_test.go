package tunnel

import (
	"errors"
	"testing"
)

func TestVirtualForwardErrorClassification(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		want      string
		permanent bool
	}{
		{
			name:      "invalid listen format",
			raw:       "forward ssh: virtual listen_addr must use IPv4:port",
			want:      "虚拟入口格式不正确，请填写完整的 IPv4:端口",
			permanent: true,
		},
		{
			name:      "implicit virtual IP",
			raw:       "virtual listen_addr requires an explicit IPv4 address; use IPv4:port",
			want:      "虚拟入口不能只填端口，请填写完整的 IPv4:端口",
			permanent: true,
		},
		{
			name:      "invalid listen port",
			raw:       "virtual listen_addr port must be between 1 and 65535",
			want:      "虚拟入口端口必须在 1 到 65535 之间",
			permanent: true,
		},
		{
			name:      "domain name",
			raw:       "virtual listen_addr does not support domain names; use IPv4:port",
			want:      "虚拟入口不支持域名，请填写完整的 IPv4:端口",
			permanent: true,
		},
		{
			name:      "IPv6 address",
			raw:       "virtual listen_addr host must be an IPv4 address",
			want:      "虚拟入口不支持 IPv6，请填写完整的 IPv4:端口",
			permanent: true,
		},
		{
			name:      "local address",
			raw:       "virtual listen_addr host must be a usable non-local IPv4 address",
			want:      "虚拟入口不能使用链路本地或保留地址；回环入口请填写 127.0.0.1",
			permanent: true,
		},
		{
			name:      "invalid server port",
			raw:       "server_addr must include a valid port before virtual forwarding is configured",
			want:      "服务端地址必须包含有效端口，配置虚拟转发前请先修正 server_addr",
			permanent: true,
		},
		{
			name:      "tunnel port conflict",
			raw:       "forward ssh virtual listen_addr cannot use the server_addr port",
			want:      "虚拟入口端口不能与客户端连接服务端的端口相同，请更换业务端口",
			permanent: true,
		},
		{
			name:      "duplicate endpoint",
			raw:       "forward ssh-2 virtual endpoint 203.0.113.10:22 duplicates forward ssh-1",
			want:      "存在重复的虚拟入口 IP:端口，请为每条虚拟转发配置不同端点",
			permanent: true,
		},
		{
			name:      "endpoint limit",
			raw:       "virtual forwarding supports at most 48 endpoints",
			want:      "虚拟转发最多支持 48 个端点，请减少配置数量",
			permanent: true,
		},
		{
			name:      "unsupported platform",
			raw:       "virtual forwarding is only supported by the standard 64-bit Windows client",
			want:      "虚拟 IP 接管仅支持标准 64 位 Windows 客户端",
			permanent: true,
		},
		{
			name:      "authorization cancelled",
			raw:       "administrator authorization for virtual forwarding was cancelled",
			want:      "IP 接管需要管理员授权，本次授权已取消",
			permanent: false,
		},
		{
			name:      "component missing",
			raw:       "WinDivert.dll is missing; repair the standard 64-bit Windows client installation",
			want:      "虚拟端点接管组件缺失，请修复或重新安装标准客户端",
			permanent: true,
		},
		{
			name:      "component integrity failed",
			raw:       "WinDivert64.sys integrity check failed: SHA-256 mismatch",
			want:      "虚拟端点接管组件完整性校验失败，请修复或重新安装标准客户端",
			permanent: true,
		},
		{
			name:      "component load failed",
			raw:       "load WinDivert.dll: The specified module could not be found",
			want:      "虚拟端点接管组件加载失败，请修复或重新安装标准客户端",
			permanent: true,
		},
		{
			name:      "driver open failed",
			raw:       "WinDivertOpen failed: access is denied",
			want:      "无法启动虚拟端点接管，请检查管理员权限、安全软件和 WinDivert 驱动状态",
			permanent: false,
		},
		{
			name:      "traffic redirect interrupted",
			raw:       "WinDivert checksum update failed: invalid data",
			want:      "虚拟流量接管已中断，请重新连接；若持续出现请检查 WinDivert 驱动状态",
			permanent: false,
		},
		{
			name:      "local bridge unavailable",
			raw:       "allocate virtual local redirect listener: bind: address unavailable",
			want:      "无法分配虚拟转发的本地桥接端口，请关闭占用大量端口的程序后重新连接",
			permanent: false,
		},
		{
			name:      "duplicate forward name",
			raw:       "virtual forward ssh listener already exists",
			want:      "虚拟转发名称重复，请为每条转发配置不同的 name",
			permanent: true,
		},
		{
			name:      "invalid redirect rule",
			raw:       "duplicate virtual redirect endpoint 203.0.113.10:22",
			want:      "虚拟端点接管规则无效，请检查虚拟转发配置并重新连接",
			permanent: true,
		},
		{
			name:      "helper stopped",
			raw:       "virtual redirect helper stopped unexpectedly",
			want:      "虚拟端点接管进程异常，请重新连接；若持续出现请检查管理员权限和 WinDivert 驱动状态",
			permanent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, ok := VirtualForwardErrorText(tt.raw)
			if !ok {
				t.Fatal("VirtualForwardErrorText() did not recognize error")
			}
			if message != tt.want {
				t.Fatalf("VirtualForwardErrorText() = %q, want %q", message, tt.want)
			}
			err := errors.New(tt.raw)
			if got := ForwardErrorMessage(err); got != tt.want {
				t.Fatalf("ForwardErrorMessage() = %q, want %q", got, tt.want)
			}
			if got := IsPermanentForwardError(err); got != tt.permanent {
				t.Fatalf("IsPermanentForwardError() = %v, want %v", got, tt.permanent)
			}
		})
	}
}

func TestVirtualForwardErrorTextIgnoresUnrelatedErrors(t *testing.T) {
	if message, ok := VirtualForwardErrorText("connection refused"); ok || message != "" {
		t.Fatalf("VirtualForwardErrorText() = %q, %v, want unrelated", message, ok)
	}
}
