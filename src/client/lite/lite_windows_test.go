//go:build windows

package lite

import (
	"errors"
	"testing"
)

func TestFriendlyLiteError(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "temporary auth block", raw: "auth_blocked: too many login failures", want: "登录失败次数过多，当前来源 IP 已被临时封禁，请稍后再试。"},
		{name: "client version", raw: "client_version_unsupported: minimum client_version is 2.0.1", want: "客户端版本不在服务端允许范围内，请联系管理员确认两端版本。"},
		{name: "protocol version", raw: "protocol_version_unsupported: required protocol_version is 2", want: "客户端与服务端协议版本不一致，请联系管理员升级对应一端。"},
		{name: "missing username", raw: "username is required", want: "导入文件缺少用户名，请重新导出 .lsylprofile。"},
		{name: "missing certificate", raw: "invalid server certificate: The system cannot find the file specified", want: "服务端证书文件缺失，请重新导入 .lsylprofile。"},
		{name: "expired certificate", raw: "x509: certificate has expired or is not yet valid", want: "服务端证书已过期或尚未生效，请重新导出 .lsylprofile。"},
		{name: "target denied", raw: "target_denied: user is not allowed to access this target", want: "当前账号没有访问该目标的权限。"},
		{name: "profile version", raw: "unsupported profile version: 2", want: "导入文件格式或版本不受支持，请使用当前客户端重新导出 .lsylprofile。"},
		{name: "invalid listen", raw: `forward "web" listen_addr is invalid: missing port in address`, want: "本地监听地址格式不正确，应填写为 127.0.0.1:端口。"},
		{name: "unknown", raw: "obscure internal failure", want: "操作失败，暂时无法识别具体原因；请重试，若持续出现请联系管理员查看客户端日志。"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := friendlyLiteError(errors.New(tt.raw)); got != tt.want {
				t.Fatalf("friendlyLiteError() = %q, want %q", got, tt.want)
			}
		})
	}
}
