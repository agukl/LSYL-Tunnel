package tunnel

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientCachesTLSConfig(t *testing.T) {
	certFile := writeTestServerCertificate(t, "192.0.2.22")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         certFile,
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{{
			Name:         "cached-tls",
			ListenAddr:   "127.0.0.1:0",
			ServerTarget: "127.0.0.1:80",
		}},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := os.Remove(certFile); err != nil {
		t.Fatal(err)
	}

	first, err := client.clientTLSConfig()
	if err != nil {
		t.Fatalf("cached TLS config read the removed CA file: %v", err)
	}
	second, err := client.clientTLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("client returned different TLS config instances")
	}
}

func TestStartRejectsInvalidTLSBeforeListening(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenAddr := reserved.Addr().String()
	_ = reserved.Close()
	certFile := writeTestServerCertificate(t, "192.0.2.22")
	if err := os.WriteFile(certFile, []byte("invalid certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	client, err := Start(context.Background(), Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         certFile,
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{{
			Name:         "invalid-tls",
			ListenAddr:   listenAddr,
			ServerTarget: "127.0.0.1:80",
		}},
	}, t.Logf)
	if err == nil {
		_ = client.Close()
		t.Fatal("Start accepted invalid TLS trust data")
	}
	probe, listenErr := net.Listen("tcp", listenAddr)
	if listenErr != nil {
		t.Fatalf("failed start retained local listener: %v", listenErr)
	}
	_ = probe.Close()
}

func TestStartVerifiedDelaysInitialHealthCheck(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var accepted atomic.Int32
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			accepted.Add(1)
			_ = conn.Close()
		}
	}()

	originalInterval := healthOKInterval
	healthOKInterval = 150 * time.Millisecond
	t.Cleanup(func() { healthOKInterval = originalInterval })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := StartVerified(ctx, Config{
		ServerAddr: listener.Addr().String(),
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         writeTestServerCertificate(t, "192.0.2.22"),
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{{
			Name:         "verified",
			ListenAddr:   "127.0.0.1:0",
			ServerTarget: "127.0.0.1:80",
		}},
	}, "2.0.1", t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	time.Sleep(50 * time.Millisecond)
	if got := accepted.Load(); got != 0 {
		t.Fatalf("verified startup made %d immediate health connections", got)
	}
	deadline := time.Now().Add(time.Second)
	for accepted.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := accepted.Load(); got == 0 {
		t.Fatal("verified startup did not resume periodic health checks")
	}
	if stats := client.Stats(); stats.ServerVersion != "2.0.1" {
		t.Fatalf("server version = %q, want 2.0.1", stats.ServerVersion)
	}

	_ = listener.Close()
	<-acceptDone
}

func TestStartSoftFailsOccupiedForward(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         writeTestServerCertificate(t, "192.0.2.22"),
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{
			{Name: "bad", ListenAddr: occupied.Addr().String(), ServerTarget: "127.0.0.1:80"},
			{Name: "good", ListenAddr: "127.0.0.1:0", ServerTarget: "127.0.0.1:80"},
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.ForwardAddr("good") == "" {
		t.Fatal("good forward should be listening")
	}
	stats := client.Stats()
	if got := forwardStateForTest(stats, "bad"); got != ForwardListenFailed {
		t.Fatalf("bad forward state = %q, want %q", got, ForwardListenFailed)
	}
	if got := forwardStateForTest(stats, "good"); got != ForwardListening {
		t.Fatalf("good forward state = %q, want %q", got, ForwardListening)
	}
}

func TestStartFailsWhenNoForwardUsable(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         writeTestServerCertificate(t, "192.0.2.22"),
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{{
			Name:         "bad",
			ListenAddr:   occupied.Addr().String(),
			ServerTarget: "127.0.0.1:80",
		}},
	}, t.Logf)
	if err == nil {
		_ = client.Close()
		t.Fatal("expected no usable forward error")
	}
	if !strings.Contains(err.Error(), "no usable forward is available") {
		t.Fatalf("Start() error = %v, want no usable forward error", err)
	}
}

func TestSetForwardStateClearsRecoveredError(t *testing.T) {
	client := &Client{forwards: map[string]*forwardRuntime{}}
	fwd := ForwardConfig{Name: "web", ListenAddr: "127.0.0.1:8080", ServerTarget: "127.0.0.1:80"}
	client.initForward("web", fwd, ForwardStarting, "")
	client.recordForwardError("web", errors.New("connectex: actively refused"))
	client.setForwardState("web", ForwardListening, "本地端口监听中")

	status, ok := forwardStatusForTest(client.Stats(), "web")
	if !ok {
		t.Fatal("forward status is missing")
	}
	if status.LastError != "" {
		t.Fatalf("recovered forward LastError = %q, want empty", status.LastError)
	}
	if status.State != ForwardListening {
		t.Fatalf("recovered forward state = %q, want %q", status.State, ForwardListening)
	}
}

func TestFinalizeHealthStatusCancelsAfterReconnectLimit(t *testing.T) {
	client := &Client{}
	status := client.finalizeHealthStatus(HealthStatus{
		State:               HealthServerUnavailable,
		Message:             "连接不上服务端，请检查服务端是否启动或地址端口是否正确",
		ConsecutiveFailures: healthMaxReconnectFailures - 1,
	})
	if status.Terminal {
		t.Fatal("status below reconnect limit should not be terminal")
	}

	status = client.finalizeHealthStatus(HealthStatus{
		State:               HealthServerUnavailable,
		Message:             "连接不上服务端，请检查服务端是否启动或地址端口是否正确",
		ConsecutiveFailures: healthMaxReconnectFailures,
	})
	if !status.Terminal {
		t.Fatal("status at reconnect limit should be terminal")
	}
	if status.Message != "多次重连失败，已取消连接状态，请确认服务端恢复后重新连接" {
		t.Fatalf("terminal message = %q", status.Message)
	}
	if got := client.Stats().Health; !got.Terminal || got.Message != status.Message {
		t.Fatalf("stored health = %+v, want terminal message %q", got, status.Message)
	}
}

func forwardStateForTest(stats ClientStats, name string) string {
	item, _ := forwardStatusForTest(stats, name)
	return item.State
}

func forwardStatusForTest(stats ClientStats, name string) (ForwardStatus, bool) {
	for _, item := range stats.Items {
		if item.Name == name {
			return item, true
		}
	}
	return ForwardStatus{}, false
}
