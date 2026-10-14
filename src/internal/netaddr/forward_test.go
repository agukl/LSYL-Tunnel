package netaddr

import "testing"

func TestForwardTargetHostPolicy(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "172.31.255.254", "192.168.1.7", "fd00::7", "::ffff:192.168.1.7"} {
		if !IsForwardTargetHost(host) {
			t.Errorf("allowed host rejected: %s", host)
		}
	}
	for _, host := range []string{"", "localhost", "::1", "127.0.0.2", "::ffff:127.0.0.1", "db.internal", "203.0.113.10", "172.15.0.1", "172.32.0.1", "169.254.1.1", "fe80::1", "fec0::1", "2001:db8::1", "0.0.0.0"} {
		if IsForwardTargetHost(host) {
			t.Errorf("forbidden host accepted: %s", host)
		}
	}
}
