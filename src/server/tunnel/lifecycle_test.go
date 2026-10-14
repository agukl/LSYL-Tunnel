package tunnel

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
)

func TestEntryConnReleasesLeaseOnce(t *testing.T) {
	server := &Server{}
	left, right := net.Pipe()
	defer right.Close()
	var released atomic.Int64
	conn := server.trackEntryConn(left, func() { released.Add(1) })
	var callers sync.WaitGroup
	for i := 0; i < 8; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			_ = conn.Close()
		}()
	}
	callers.Wait()
	closeLifecycleServer(t, server)
	if got := released.Load(); got != 1 {
		t.Fatalf("lease releases = %d, want 1", got)
	}
	assertLifecycleStopped(t, server)
}

func TestServerCloseRejectsNewWorkAndWaitsForExistingWorker(t *testing.T) {
	server := &Server{}
	unblock := make(chan struct{})
	var release sync.Once
	defer release.Do(func() { close(unblock) })
	if !server.startWorker(func() { <-unblock }) {
		t.Fatal("worker was rejected before shutdown")
	}
	done := make(chan error, 1)
	go func() { done <- server.Close() }()
	waitForCounter(t, func() int64 {
		server.lifecycle.mu.Lock()
		defer server.lifecycle.mu.Unlock()
		if server.lifecycle.closing {
			return 1
		}
		return 0
	}, 1, time.Second)
	if server.startWorker(func() {}) {
		t.Fatal("worker was accepted during shutdown")
	}
	left, right := net.Pipe()
	defer right.Close()
	var released atomic.Int64
	if conn := server.trackEntryConn(left, func() { released.Add(1) }); conn != nil {
		t.Fatal("connection was accepted during shutdown")
	}
	if released.Load() != 1 {
		t.Fatal("rejected connection leaked its entry lease")
	}
	select {
	case <-done:
		t.Fatal("Close returned before the worker exited")
	default:
	}
	release.Do(func() { close(unblock) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not finish after worker exit")
	}
}

func TestServerCloseRacesWithConnectionRegistration(t *testing.T) {
	for i := 0; i < 50; i++ {
		server := &Server{}
		left, right := net.Pipe()
		var released atomic.Int64
		start := make(chan struct{})
		var callers sync.WaitGroup
		callers.Add(9)
		go func() {
			defer callers.Done()
			<-start
			conn := server.trackEntryConn(left, func() { released.Add(1) })
			if conn != nil && !server.startWorker(func() { _ = conn.Close() }) {
				_ = conn.Close()
			}
		}()
		for j := 0; j < 8; j++ {
			go func() {
				defer callers.Done()
				<-start
				_ = server.Close()
			}()
		}
		close(start)
		callers.Wait()
		_ = right.Close()
		if released.Load() != 1 {
			t.Fatalf("iteration %d: entry lease not released exactly once", i)
		}
		assertLifecycleStopped(t, server)
	}
}

func TestReverseConnectionsKeepEntryLeasesUntilClosed(t *testing.T) {
	for _, limit := range []string{"global", "per_ip"} {
		t.Run(limit, func(t *testing.T) {
			reverseAddr := freeTCPAddr(t)
			server, cert := startLifecycleServer(t, context.Background(), func(cfg *Config) {
				cfg.Forwards = []ForwardConfig{lifecycleReverseForward(reverseAddr)}
				if limit == "global" {
					cfg.Security.MaxConcurrentConnections = 2
				} else {
					cfg.Security.MaxConcurrentConnectionsPerIP = 2
				}
			})
			control := activateReverseControlForTest(t, server.Addr(), cert, "lifecycle", "reverse", reverseAddr, "127.0.0.1:1")
			defer control.Close()
			waitForActiveConnections(t, server, 1, time.Second)

			stream, inbound := openLifecycleReverseStream(t, server, cert, control, reverseAddr)
			waitForActiveConnections(t, server, 2, time.Second)
			waitForUserStreamActive(t, server, "alice", 1, time.Second)
			rejected, err := net.DialTimeout("tcp", server.Addr(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer rejected.Close()
			assertLifecycleConnClosed(t, rejected)
			waitForCounter(t, func() int64 { return server.connectionsRejected.Load() }, 1, time.Second)
			waitForActiveConnections(t, server, 2, time.Second)

			_ = stream.Close()
			_ = inbound.Close()
			waitForActiveConnections(t, server, 1, time.Second)
			waitForUserStreamActive(t, server, "alice", 0, time.Second)

			failed := dialTLSForTest(t, server.Addr(), cert)
			defer failed.Close()
			_ = failed.SetDeadline(time.Now().Add(3 * time.Second))
			if err := protocol.WriteJSON(failed, currentTestRequest(protocol.OpenRequest{
				Type: "reverse_stream", Username: "alice", Password: "secret",
				ListenAddr: reverseAddr, StreamID: "expired",
			})); err != nil {
				t.Fatal(err)
			}
			var resp protocol.OpenResponse
			if err := protocol.ReadJSON(failed, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
				t.Fatal(err)
			}
			if resp.OK || resp.Code != "reverse_failed" {
				t.Fatalf("unexpected failed handoff response: %+v", resp)
			}
			waitForActiveConnections(t, server, 1, time.Second)

			stream, inbound = openLifecycleReverseStream(t, server, cert, control, reverseAddr)
			waitForActiveConnections(t, server, 2, time.Second)
			_ = control.Close()
			waitForActiveConnections(t, server, 1, time.Second)
			_ = stream.Close()
			_ = inbound.Close()
			waitForActiveConnections(t, server, 0, time.Second)
		})
	}
}

func TestReversePendingTimeoutIsOwnedByWorker(t *testing.T) {
	reverseAddr := freeTCPAddr(t)
	server, cert := startLifecycleServer(t, context.Background(), func(cfg *Config) {
		cfg.Forwards = []ForwardConfig{lifecycleReverseForward(reverseAddr)}
		cfg.Security.DialTimeoutSec = 1
	})
	control := activateReverseControlForTest(t, server.Addr(), cert, "lifecycle", "reverse", reverseAddr, "127.0.0.1:1")
	defer control.Close()
	inbound, err := net.DialTimeout("tcp", reverseAddr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer inbound.Close()
	_ = control.SetReadDeadline(time.Now().Add(3 * time.Second))
	var request protocol.OpenResponse
	if err := protocol.ReadJSON(control, &request, protocol.DefaultMaxHandshakeBytes); err != nil {
		t.Fatal(err)
	}
	if request.Code != "reverse_connect" {
		t.Fatalf("unexpected pending stream request: %+v", request)
	}
	waitForCounter(t, func() int64 { return server.dialFailed.Load() }, 1, 3*time.Second)
	assertLifecycleConnClosed(t, inbound)
	listener := server.reverseListener(reverseAddr)
	listener.mu.Lock()
	pending := len(listener.pending)
	listener.mu.Unlock()
	if pending != 0 {
		t.Fatalf("expired pending connections = %d", pending)
	}
	closeLifecycleServer(t, server)
	path := datedJSONLPath(server.cfg.Runtime.FlowTrafficLogFile, time.Now().Format("2006-01-02"))
	data, err := os.ReadFile(path)
	if err != nil || strings.Count(string(data), "client_stream_timeout") != 1 {
		t.Fatalf("timeout log = %s, error = %v", data, err)
	}
}

func TestServerShutdownClosesStreamsPendingConnectionsAndLogs(t *testing.T) {
	for _, shutdown := range []string{"close", "cancel"} {
		t.Run(shutdown, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			echo := startEchoServer(t)
			reverseAddr := freeTCPAddr(t)
			server, cert := startLifecycleServer(t, ctx, func(cfg *Config) {
				cfg.Forwards = []ForwardConfig{
					lifecycleReverseForward(reverseAddr),
					{Name: "echo", Direction: DirectionClientToServer, ServerTarget: echo.Addr().String(), AllowedUsers: []string{"alice"}},
				}
			})
			control := activateReverseControlForTest(t, server.Addr(), cert, "lifecycle", "reverse", reverseAddr, "127.0.0.1:1")
			defer control.Close()
			stream, inbound := openLifecycleReverseStream(t, server, cert, control, reverseAddr)
			forward, resp := openTargetConnForTest(t, server.Addr(), cert, "alice", "secret", "echo", echo.Addr().String())
			defer forward.Close()
			if !resp.OK {
				t.Fatalf("forward failed: %+v", resp)
			}
			pending, err := net.DialTimeout("tcp", reverseAddr, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer pending.Close()
			_ = control.SetReadDeadline(time.Now().Add(3 * time.Second))
			var request protocol.OpenResponse
			if err := protocol.ReadJSON(control, &request, protocol.DefaultMaxHandshakeBytes); err != nil {
				t.Fatal(err)
			}
			if request.Code != "reverse_connect" {
				t.Fatalf("unexpected pending stream request: %+v", request)
			}
			waitForUserStreamActive(t, server, "alice", 2, time.Second)
			if shutdown == "cancel" {
				cancel()
				waitForActiveConnections(t, server, 0, 3*time.Second)
			}
			closeLifecycleServer(t, server)
			closeLifecycleServer(t, server)
			if shutdown == "close" && ctx.Err() != nil {
				t.Fatal("Close canceled the caller's context")
			}
			assertLifecycleStopped(t, server)
			for _, conn := range []net.Conn{control, stream, inbound, forward, pending} {
				assertLifecycleConnClosed(t, conn)
			}
			if got := server.userStreams.snapshot().Active; got != 0 {
				t.Fatalf("active account streams after shutdown = %d", got)
			}
			if got := server.activeStreams.Load(); got != 0 {
				t.Fatalf("active streams after shutdown = %d", got)
			}
			path := datedJSONLPath(server.cfg.Runtime.FlowTrafficLogFile, time.Now().Format("2006-01-02"))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Count(string(data), `"event":"stream_closed"`); got != 2 {
				t.Fatalf("closed stream log count = %d, want 2: %s", got, data)
			}
			if strings.Contains(string(data), "client_stream_timeout") {
				t.Fatalf("shutdown was logged as a reverse timeout: %s", data)
			}
			for _, log := range []*jsonlLog{server.requestLog, server.businessLog, server.entryTrafficLog, server.flowTrafficLog} {
				log.mu.Lock()
				open := log.file != nil
				log.mu.Unlock()
				if open {
					t.Fatal("log file remains open after Close")
				}
			}
		})
	}
}

func TestServerCloseInterruptsIncompleteHandshakes(t *testing.T) {
	for _, stage := range []string{"certificate_gate", "tls", "request"} {
		t.Run(stage, func(t *testing.T) {
			server, cert := startLifecycleServer(t, context.Background(), nil)
			var conn net.Conn
			if stage == "request" {
				conn = dialTLSForTest(t, server.Addr(), cert)
			} else {
				var err error
				conn, err = net.DialTimeout("tcp", server.Addr(), time.Second)
				if err != nil {
					t.Fatal(err)
				}
				if stage == "tls" {
					_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
					if err := transport.ClientCertificateGate(conn, server.certificateGateKey); err != nil {
						t.Fatal(err)
					}
				}
			}
			defer conn.Close()
			waitForActiveConnections(t, server, 1, time.Second)
			closeLifecycleServer(t, server)
			assertLifecycleConnClosed(t, conn)
			assertLifecycleStopped(t, server)
			if got := server.entryProtocolRejected.Load(); got != 0 {
				t.Fatalf("shutdown created %d entry rejections", got)
			}
		})
	}
}

func TestServerCloseStopsMonitor(t *testing.T) {
	monitor := freeTCPAddr(t)
	server, _ := startLifecycleServer(t, context.Background(), func(cfg *Config) {
		cfg.MonitorAddr = monitor
	})
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := client.Get("http://" + monitor + "/status")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("monitor status = %d", resp.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("monitor never started: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	stalled, err := net.DialTimeout("tcp", monitor, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer stalled.Close()
	_ = stalled.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := io.WriteString(stalled, "POST /security/unblock HTTP/1.1\r\nHost: 127.0.0.1\r\nContent-Length: 100\r\n\r\n{"); err != nil {
		t.Fatal(err)
	}
	closeLifecycleServer(t, server)
	assertLifecycleConnClosed(t, stalled)
	conn, err := net.DialTimeout("tcp", monitor, time.Second)
	if err == nil {
		_ = conn.Close()
		t.Fatal("monitor still accepts connections after Close")
	}
}

func startLifecycleServer(t *testing.T, ctx context.Context, configure func(*Config)) (*Server, string) {
	t.Helper()
	cert, key := writeTestCertificate(t)
	dir := t.TempDir()
	cfg := Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: cert, KeyFile: key},
		Auth:       AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
		Security:   SecurityConfig{HandshakeTimeoutSec: 30, DialTimeoutSec: 30, MaxConcurrentStreamsPerUser: 10},
		Runtime: RuntimeConfig{
			RequestLogFile:      filepath.Join(dir, "request.jsonl"),
			BusinessLogFile:     filepath.Join(dir, "business.jsonl"),
			EntryTrafficLogFile: filepath.Join(dir, "entry.jsonl"),
			FlowTrafficLogFile:  filepath.Join(dir, "flow.jsonl"),
		},
	}
	if configure != nil {
		configure(&cfg)
	}
	server, err := Start(ctx, cfg, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeLifecycleServer(t, server) })
	return server, cert
}

func lifecycleReverseForward(addr string) ForwardConfig {
	return ForwardConfig{Name: "reverse", Direction: DirectionServerToClient, ListenAddr: addr, AllowedUsers: []string{"alice"}}
}

func openLifecycleReverseStream(t *testing.T, server *Server, cert string, control *tls.Conn, addr string) (*tls.Conn, net.Conn) {
	t.Helper()
	inbound, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = inbound.Close() })
	_ = inbound.SetDeadline(time.Now().Add(3 * time.Second))
	_ = control.SetReadDeadline(time.Now().Add(3 * time.Second))
	var request protocol.OpenResponse
	if err := protocol.ReadJSON(control, &request, protocol.DefaultMaxHandshakeBytes); err != nil {
		t.Fatal(err)
	}
	if request.Code != "reverse_connect" || request.StreamID == "" {
		t.Fatalf("unexpected reverse request: %+v", request)
	}
	stream := dialTLSForTest(t, server.Addr(), cert)
	t.Cleanup(func() { _ = stream.Close() })
	_ = stream.SetDeadline(time.Now().Add(3 * time.Second))
	if err := protocol.WriteJSON(stream, currentTestRequest(protocol.OpenRequest{
		Type: "reverse_stream", Username: "alice", Password: "secret",
		Direction: DirectionServerToClient, ListenAddr: addr, StreamID: request.StreamID,
	})); err != nil {
		t.Fatal(err)
	}
	var response protocol.OpenResponse
	if err := protocol.ReadJSON(stream, &response, protocol.DefaultMaxHandshakeBytes); err != nil {
		t.Fatal(err)
	}
	if !response.OK {
		t.Fatalf("reverse stream failed: %+v", response)
	}
	if _, err := stream.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	var payload [4]byte
	if _, err := io.ReadFull(inbound, payload[:]); err != nil || string(payload[:]) != "ping" {
		t.Fatalf("reverse payload = %q, error = %v", payload, err)
	}
	return stream, inbound
}

func closeLifecycleServer(t *testing.T, server *Server) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- server.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("server Close did not finish")
	}
}

func assertLifecycleConnClosed(t *testing.T, conn net.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	// The certificate gate may already have sent a challenge before shutdown.
	_, err := io.Copy(io.Discard, conn)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("connection was not closed: %v", err)
	}
}

func assertLifecycleStopped(t *testing.T, server *Server) {
	t.Helper()
	if server.startWorker(func() {}) {
		t.Fatal("server accepted work after shutdown")
	}
	server.lifecycle.mu.Lock()
	count := len(server.lifecycle.conns)
	server.lifecycle.mu.Unlock()
	if count != 0 {
		t.Fatalf("tracked connections after shutdown = %d", count)
	}
	if got := server.connLimiter.snapshot()["active"]; got != 0 {
		t.Fatalf("active entry leases after shutdown = %d", got)
	}
}
