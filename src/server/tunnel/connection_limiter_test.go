package tunnel

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConnectionLimiterRejectsPerIPConcurrency(t *testing.T) {
	limiter := newConnectionLimiter(SecurityConfig{
		MaxConcurrentConnections:      10,
		MaxConcurrentConnectionsPerIP: 1,
		ConnectionRateWindowSec:       1,
		MaxNewConnectionsPerIPWindow:  10,
	})

	release, ok, reason := limiter.acquire("203.0.113.10")
	if !ok {
		t.Fatalf("first acquire rejected: %s", reason)
	}
	defer release()

	if _, ok, reason := limiter.acquire("203.0.113.10"); ok || reason != "per_ip_concurrent_connections" {
		t.Fatalf("second acquire = %v, %q; want per-IP concurrency reject", ok, reason)
	}

	release()
	if release, ok, reason := limiter.acquire("203.0.113.10"); !ok {
		t.Fatalf("acquire after release rejected: %s", reason)
	} else {
		release()
	}
}

func TestConnectionLimiterRejectsConnectionRate(t *testing.T) {
	now := time.Unix(1000, 0)
	limiter := newConnectionLimiter(SecurityConfig{
		MaxConcurrentConnections:      10,
		MaxConcurrentConnectionsPerIP: 10,
		ConnectionRateWindowSec:       1,
		MaxNewConnectionsPerIPWindow:  2,
	})
	limiter.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		release, ok, reason := limiter.acquire("203.0.113.10")
		if !ok {
			t.Fatalf("acquire %d rejected: %s", i, reason)
		}
		release()
	}

	if _, ok, reason := limiter.acquire("203.0.113.10"); ok || reason != "per_ip_new_connection_rate" {
		t.Fatalf("third acquire = %v, %q; want rate reject", ok, reason)
	}

	now = now.Add(1100 * time.Millisecond)
	if release, ok, reason := limiter.acquire("203.0.113.10"); !ok {
		t.Fatalf("acquire after window rejected: %s", reason)
	} else {
		release()
	}
}

func TestServerRejectsConnectionsBeforeHandshakeWhenPerIPLimitReached(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Security: SecurityConfig{
			HandshakeTimeoutSec:           2,
			DialTimeoutSec:                1,
			MaxHandshakeBytes:             32768,
			MaxConcurrentConnections:      10,
			MaxConcurrentConnectionsPerIP: 1,
			ConnectionRateWindowSec:       1,
			MaxNewConnectionsPerIPWindow:  100,
			AuthFailWindowSec:             60,
			AuthFailThreshold:             3,
			AuthFailBlockSec:              60,
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	first, err := net.DialTimeout("tcp", server.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	waitForActiveConnections(t, server, 1, time.Second)

	second, err := net.DialTimeout("tcp", server.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := second.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	if n, err := second.Read(buf); err == nil || n != 0 {
		t.Fatalf("second connection read = %d, %v; want closed connection", n, err)
	}
	if got := server.connectionsRejected.Load(); got != 1 {
		t.Fatalf("connectionsRejected = %d, want 1", got)
	}
	if got := server.connectionsRejectedPerIPActive.Load(); got != 1 {
		t.Fatalf("connectionsRejectedPerIPActive = %d, want 1", got)
	}
}

func TestEntryLayerRejectsPermanentBlockBeforeConnectionLimiter(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Security: SecurityConfig{
			HandshakeTimeoutSec:           2,
			DialTimeoutSec:                1,
			MaxHandshakeBytes:             32768,
			MaxConcurrentConnections:      1,
			MaxConcurrentConnectionsPerIP: 1,
			ConnectionRateWindowSec:       1,
			MaxNewConnectionsPerIPWindow:  1,
			AuthFailWindowSec:             60,
			AuthFailThreshold:             3,
			AuthFailBlockSec:              60,
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	server.fails.permanent.Store("127.0.0.1", struct{}{})

	conn, err := net.DialTimeout("tcp", server.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = conn.Read(make([]byte, 1))
	_ = conn.Close()

	waitForCounter(t, func() int64 { return server.entryPermanentBlockHits.Load() }, 1, time.Second)
	if got := server.connLimiter.snapshot()["active"]; got != 0 {
		t.Fatalf("active connections = %d, want 0", got)
	}
}

func TestEntryLayerRejectsPlainHTTPProbeWithoutRequestLog(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	dir := t.TempDir()
	requestLog := filepath.Join(dir, "request.jsonl")
	entryLog := filepath.Join(dir, "entry-traffic.jsonl")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Security: SecurityConfig{
			HandshakeTimeoutSec: 2,
			DialTimeoutSec:      1,
			MaxHandshakeBytes:   32768,
			AuthFailWindowSec:   60,
			AuthFailThreshold:   3,
			AuthFailBlockSec:    60,
		},
		Runtime: RuntimeConfig{RequestLogFile: requestLog, EntryTrafficLogFile: entryLog},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	conn, err := net.DialTimeout("tcp", server.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: tunnel\r\n\r\n"))
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = conn.Read(make([]byte, 1))
	_ = conn.Close()

	waitForCounter(t, func() int64 { return server.entryHTTPProbeRejected.Load() }, 1, time.Second)
	waitForFileContains(t, datedJSONLPath(entryLog, time.Now().Format("2006-01-02")), `"code":"http_probe"`, 2*time.Second)
	requestPath := datedJSONLPath(requestLog, time.Now().Format("2006-01-02"))
	if _, err := os.Stat(requestPath); !os.IsNotExist(err) {
		t.Fatalf("expected no request log for entry HTTP probe, got err=%v", err)
	}
}

func TestClassifyEntryReadErrorDetectsHTTPProbe(t *testing.T) {
	code := classifyEntryReadError(protocolLengthErrorForTest("GET "))
	if code != entryCodeHTTPProbe {
		t.Fatalf("classifyEntryReadError(GET) = %q, want %q", code, entryCodeHTTPProbe)
	}
	code = classifyEntryReadError(protocolLengthErrorForTest("PROP"))
	if code != entryCodeHTTPProbe {
		t.Fatalf("classifyEntryReadError(PROP) = %q, want %q", code, entryCodeHTTPProbe)
	}
}

func protocolLengthErrorForTest(prefix string) error {
	value := int(prefix[0])<<24 | int(prefix[1])<<16 | int(prefix[2])<<8 | int(prefix[3])
	return fmt.Errorf("handshake too large: %d bytes", value)
}

func waitForCounter(t *testing.T, value func() int64, want int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := value(); got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("counter did not reach %d, got %d", want, value())
}

func waitForActiveConnections(t *testing.T, server *Server, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := server.connLimiter.snapshot()["active"]; got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("active connections did not reach %d", want)
}
