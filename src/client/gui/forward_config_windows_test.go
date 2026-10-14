//go:build windows

package gui

import (
	"errors"
	"strings"
	"testing"
)

func TestForwardConfigErrorMessagesKeepRuleAndField(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`forward "rdp" has duplicate runtime name; use unique names within this client`, "规则标识重复"},
		{`forward "rdp" server_target must use 127.0.0.1 or a private IP address`, "不支持域名或公网 IP"},
		{`forward "rdp" listen_addr must include a host and port`, "正向 listen_addr"},
		{`forward "rdp" listen_addr port must be between 0 and 65535`, "随机监听端口"},
	} {
		got := friendlyErrorText(tc.raw)
		if !strings.Contains(got, "rdp") || !strings.Contains(got, tc.want) {
			t.Errorf("%s: %s", tc.raw, got)
		}
	}
	raw := `tls.server_name: loopback host "localhost" is not supported; use 127.0.0.1`
	if got := mobileExportFriendlyError(errors.New(raw)); !strings.Contains(got, "tls.server_name") || !strings.Contains(got, "127.0.0.1") {
		t.Fatal(got)
	}
}
