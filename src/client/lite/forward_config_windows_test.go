//go:build windows

package lite

import (
	"errors"
	"strings"
	"testing"
)

func TestForwardConfigErrorsAreNotGenericFailures(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`tls.server_name: loopback host "localhost" is not supported; use 127.0.0.1`, "不再支持，请使用 127.0.0.1"},
		{`forward "rdp" server_target must use 127.0.0.1 or a private IP address`, "不支持域名或公网 IP"},
		{`forward "rdp" has duplicate runtime name; use unique names within this client`, "规则标识重复"},
	} {
		if got := friendlyLiteError(errors.New(tc.raw)); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %s", tc.raw, got)
		}
	}
}
