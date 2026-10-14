package netaddr

import (
	"net"
	"strings"
	"testing"
)

func TestCheckLoopbackHost(t *testing.T) {
	for _, host := range []string{"localhost", "LOCALHOST.", "app.localhost", "::1", "0:0:0:0:0:0:0:1", "::1%lo", "::ffff:127.0.0.1", "127.0.0.2", "127.255.255.254"} {
		t.Run(host, func(t *testing.T) {
			for _, err := range []error{CheckLoopbackHost(host), CheckLoopbackEndpoint(net.JoinHostPort(host, "3443"))} {
				if err == nil || !strings.Contains(err.Error(), "use 127.0.0.1") {
					t.Fatalf("loopback alias %q: %v", host, err)
				}
			}
		})
	}
	for _, host := range []string{"", "127.0.0.1", "0.0.0.0", "::", "192.168.1.7", "fd00::7", "2001:db8::1", "vpn.example.com", "localhost.example.com"} {
		if err := CheckLoopbackHost(host); err != nil {
			t.Errorf("non-alias %q rejected: %v", host, err)
		}
	}
}
