package tunnel

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestStartLogsConfiguredForwardAvailabilityFailuresWithoutFailing(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	unreachable := freeTCPAddr(t)

	logs := make(chan string, 32)
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Forwards: []ForwardConfig{
			{Direction: DirectionClientToServer, ServerTarget: unreachable, AllowedUsers: []string{"alice"}},
			{Direction: DirectionServerToClient, ListenAddr: occupied.Addr().String(), AllowedUsers: []string{"alice"}},
		},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 1, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, func(format string, args ...any) {
		select {
		case logs <- fmt.Sprintf(format, args...):
		default:
		}
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer server.Close()

	waitForLogContains(t, logs, "target_unreachable", 2*time.Second)
	waitForLogContains(t, logs, "passive_port_unavailable", 2*time.Second)
}

func waitForLogContains(t *testing.T, logs <-chan string, needle string, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line := <-logs:
			if strings.Contains(line, needle) {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for log containing %q", needle)
		}
	}
}
