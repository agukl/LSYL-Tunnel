package tunnel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clienttunnel "lsyltunnel/src/client/tunnel"
)

func TestRequestAndBusinessLogsWritten(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	dir := t.TempDir()
	requestLog := filepath.Join(dir, "request.jsonl")
	businessLog := filepath.Join(dir, "business.jsonl")
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
			HandshakeTimeoutSec: 3,
			DialTimeoutSec:      1,
			MaxHandshakeBytes:   32768,
			AuthFailThreshold:   3,
			AuthFailWindowSec:   60,
			AuthFailBlockSec:    60,
		},
		Runtime: RuntimeConfig{
			RequestLogFile:  requestLog,
			BusinessLogFile: businessLog,
			RecentEvents:    500,
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	_, err = clienttunnel.CheckLoginResponse(ctx, clienttunnel.Config{
		ServerAddr: server.Addr(),
		Username:   "alice",
		Password:   "secret",
		TLS:        clienttunnel.TLSConfig{CACertFile: certFile, ServerName: "localhost"},
		Connection: clienttunnel.ConnectionConfig{DialTimeoutSec: 3},
	})
	if err != nil {
		t.Fatal(err)
	}

	waitForFileContains(t, datedJSONLPath(requestLog, time.Now().Format("2006-01-02")), `"password":"secret"`, 2*time.Second)
	waitForFileContains(t, datedJSONLPath(businessLog, time.Now().Format("2006-01-02")), `"kind":"login"`, 2*time.Second)
}

func TestHealthLogDoesNotWriteBusinessEvent(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	dir := t.TempDir()
	requestLog := filepath.Join(dir, "request.jsonl")
	businessLog := filepath.Join(dir, "business.jsonl")
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
			HandshakeTimeoutSec: 3,
			DialTimeoutSec:      1,
			MaxHandshakeBytes:   32768,
			AuthFailThreshold:   3,
			AuthFailWindowSec:   60,
			AuthFailBlockSec:    60,
		},
		Runtime: RuntimeConfig{
			RequestLogFile:  requestLog,
			BusinessLogFile: businessLog,
			RecentEvents:    500,
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	if _, err = clienttunnel.CheckHealthResponse(ctx, clienttunnel.Config{
		ServerAddr: server.Addr(),
		Username:   "alice",
		Password:   "secret",
		TLS:        clienttunnel.TLSConfig{CACertFile: certFile, ServerName: "localhost"},
		Connection: clienttunnel.ConnectionConfig{DialTimeoutSec: 3},
	}); err != nil {
		t.Fatal(err)
	}

	requestPath := datedJSONLPath(requestLog, time.Now().Format("2006-01-02"))
	businessPath := datedJSONLPath(businessLog, time.Now().Format("2006-01-02"))
	waitForFileContains(t, requestPath, `"type":"health"`, 2*time.Second)
	if data, err := os.ReadFile(businessPath); err == nil && strings.Contains(string(data), `"kind"`) {
		t.Fatalf("health should not write business log, got %s", string(data))
	}
}

func TestPermanentBlockedHitsAreAggregatedWithoutRequestLogSpam(t *testing.T) {
	dir := t.TempDir()
	requestLog := filepath.Join(dir, "request.jsonl")
	businessLog := filepath.Join(dir, "business.jsonl")
	entryTrafficLog := filepath.Join(dir, "entry-traffic.jsonl")
	server := &Server{
		requestLog:         newJSONLLog(requestLog),
		businessLog:        newJSONLLog(businessLog),
		entryTrafficLog:    newJSONLLog(entryTrafficLog),
		maxRecentEvents:    500,
		permanentBlockHits: newPermanentBlockHitAggregator(time.Second),
	}
	server.entryTrafficWriter = newAsyncEntryTrafficLog(8, server.writeEntryTrafficLog)
	defer server.Close()

	server.recordPermanentBlockedHit("203.0.113.10")
	server.recordPermanentBlockedHit("203.0.113.10")
	server.recordPermanentBlockedHit("203.0.113.10")
	server.flushPermanentBlockHitLogs()

	entryPath := datedJSONLPath(entryTrafficLog, time.Now().Format("2006-01-02"))
	waitForFileContains(t, entryPath, `"code":"ip_permanently_blocked"`, 2*time.Second)
	data, err := os.ReadFile(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"code":"ip_permanently_blocked"`) != 1 {
		t.Fatalf("expected one aggregated entry log, got %s", string(data))
	}
	if !strings.Contains(string(data), `hit 3 times in last 1s`) {
		t.Fatalf("expected aggregated count in entry log, got %s", string(data))
	}
	if _, err := os.Stat(datedJSONLPath(businessLog, time.Now().Format("2006-01-02"))); !os.IsNotExist(err) {
		t.Fatalf("expected no business log file for permanent blocked hits, got err=%v", err)
	}

	requestPath := datedJSONLPath(requestLog, time.Now().Format("2006-01-02"))
	if _, err := os.Stat(requestPath); !os.IsNotExist(err) {
		t.Fatalf("expected no request log file for permanent blocked hits, got err=%v", err)
	}
}

func TestAsyncEntryTrafficLogDropsWhenQueueIsFull(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	writer := newAsyncEntryTrafficLog(1, func(EntryTrafficLogEntry) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
	})

	writer.Enqueue(EntryTrafficLogEntry{Event: "protocol_rejected"})
	<-started
	writer.Enqueue(EntryTrafficLogEntry{Event: "protocol_rejected"})
	writer.Enqueue(EntryTrafficLogEntry{Event: "protocol_rejected"})
	if got := writer.stats().Dropped; got != 1 {
		t.Fatalf("dropped entry logs = %d, want 1", got)
	}

	close(release)
	writer.Close()
	stats := writer.stats()
	if stats.Accepted != 2 || stats.Written != 2 {
		t.Fatalf("writer stats = %#v, want two accepted and written entries", stats)
	}
}

func waitForFileContains(t *testing.T, path, needle string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), needle) {
			return
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to contain %q: %v", path, needle, lastErr)
}
