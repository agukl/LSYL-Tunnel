package tunnel

import (
	"net"
	"strings"
	"testing"
)

func TestForwardConfigRejectsDuplicateRuntimeNames(t *testing.T) {
	for _, names := range [][2]string{{"rdp", "rdp"}, {"", "127.0.0.1:13389"}} {
		cfg := loopbackTestConfig()
		cfg.Forwards[0].Name = names[0]
		second := cfg.Forwards[0]
		second.Name = names[1]
		second.ListenAddr = "127.0.0.1:13390"
		cfg.Forwards = append(cfg.Forwards, second)
		if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "duplicate runtime name") {
			t.Fatalf("expected duplicate rule key error, got %v", err)
		}
	}
}

func TestForwardConfigTargetsMatchServerPolicy(t *testing.T) {
	for _, host := range []string{"db.internal", "203.0.113.10", "0.0.0.0", "169.254.1.1"} {
		for _, direction := range []string{DirectionClientToServer, DirectionVirtual} {
			cfg := loopbackTestConfig()
			cfg.Forwards[0].Direction = direction
			cfg.Forwards[0].ServerTarget = net.JoinHostPort(host, "3389")
			if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "server_target must use") {
				t.Fatalf("target %s direction %s: %v", host, direction, err)
			}
		}
	}
	for _, host := range []string{"127.0.0.1", "192.168.1.7", "fd00::7"} {
		cfg := loopbackTestConfig()
		cfg.Forwards[0].ServerTarget = net.JoinHostPort(host, "3389")
		if err := ValidateConfig(cfg); err != nil {
			t.Fatalf("allowed target %s: %v", host, err)
		}
	}
}

func TestForwardListenSyntaxPreservesWildcardAndEphemeralPorts(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "127.0.0.1:http", "127.0.0.1:65536", "127.0.0.1:50000-50002"} {
		cfg := loopbackTestConfig()
		cfg.Forwards[0].ListenAddr = addr
		if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "listen_addr") {
			t.Fatalf("invalid listen address %s: %v", addr, err)
		}
	}
	for _, addr := range []string{"127.0.0.1:0", ":13389", "0.0.0.0:13389", "[::]:13389"} {
		cfg := loopbackTestConfig()
		cfg.Forwards[0].ListenAddr = addr
		if err := ValidateConfig(cfg); err != nil {
			t.Fatalf("existing listen mode %s rejected: %v", addr, err)
		}
	}
}

func TestVirtualListenDoesNotAcceptMappedIPv6(t *testing.T) {
	if _, _, err := parseVirtualListenAddr("[::ffff:192.0.2.22]:3389"); err == nil || !strings.Contains(err.Error(), "must be an IPv4") {
		t.Fatalf("mapped IPv6 accepted as virtual IPv4: %v", err)
	}
}
