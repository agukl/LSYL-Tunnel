package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clienttunnel "lsyltunnel/src/client/tunnel"
)

func TestJSONLLogWriteJSONPreservesEncodedBytes(t *testing.T) {
	basePath := filepath.Join(t.TempDir(), "events.jsonl")
	logFile := newJSONLLog(basePath)
	defer logFile.Close()

	raw := []byte(`{"z":1, "a":2}`)
	if err := logFile.WriteJSON(raw); err != nil {
		t.Fatal(err)
	}
	path := datedJSONLPath(basePath, time.Now().Format("2006-01-02"))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), string(raw)+"\n"; got != want {
		t.Fatalf("JSONL bytes = %q, want %q", got, want)
	}
}

func TestStructuredLogsDoNotMirrorToServiceLog(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.jsonl")
	businessPath := filepath.Join(dir, "business.jsonl")
	entryPath := filepath.Join(dir, "entry.jsonl")
	var serviceLogs []string
	server := &Server{
		logf: func(format string, args ...any) {
			serviceLogs = append(serviceLogs, fmt.Sprintf(format, args...))
		},
		requestLog:      newJSONLLog(requestPath),
		businessLog:     newJSONLLog(businessPath),
		entryTrafficLog: newJSONLLog(entryPath),
	}
	defer server.Close()

	server.recordRequestLog(RequestLogEntry{RequestID: "request-1", Result: "ok"})
	server.recordBusinessLog(BusinessLogEntry{RequestID: "business-1", Kind: "open", Result: "connected"})
	server.recordEntryTrafficLog(EntryTrafficLogEntry{RequestID: "entry-1", Event: "connection_rejected", Result: "rejected"})
	server.log("service diagnostic: %s", "ready")

	date := time.Now().Format("2006-01-02")
	for _, entry := range []struct{ path, requestID string }{
		{datedJSONLPath(requestPath, date), "request-1"},
		{datedJSONLPath(businessPath, date), "business-1"},
		{datedJSONLPath(entryPath, date), "entry-1"},
	} {
		data, err := os.ReadFile(entry.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"request_id":"`+entry.requestID+`"`) {
			t.Fatalf("%s is missing request %q: %s", entry.path, entry.requestID, data)
		}
	}
	if len(serviceLogs) != 1 || serviceLogs[0] != "service diagnostic: ready" {
		t.Fatalf("service logs contain mirrored traffic: %#v", serviceLogs)
	}
}

func TestFlowMetricsAreIncludedInBusinessEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business.jsonl")
	server := &Server{
		cfg:         Config{Security: SecurityConfig{StreamRateLimitBytesPerSec: 1024, MaxConcurrentStreamsPerUser: 4}},
		businessLog: newJSONLLog(path),
	}
	defer server.Close()
	server.recordEvent(RuntimeEvent{Kind: "open", Result: "closed", DurationMS: 1000, BytesUp: 300, BytesDown: 200})
	server.recordEvent(RuntimeEvent{Kind: "reverse_inbound", Result: "failed", RemoteIP: "192.0.2.10", Code: "client_stream_timeout"})

	data, err := os.ReadFile(datedJSONLPath(path, time.Now().Format("2006-01-02")))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("business event count = %d, want 2: %s", len(lines), data)
	}
	var closed, failed BusinessLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &closed); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &failed); err != nil {
		t.Fatal(err)
	}
	if closed.BytesTotal != 500 || closed.AverageBytesPerSec != 500 || closed.RateLimitBytesPerSec != 1024 || closed.MaxConcurrentStreamsUser != 4 || closed.Abnormal {
		t.Fatalf("closed stream metrics = %+v", closed)
	}
	if failed.RemoteIP != "192.0.2.10" || failed.Code != "client_stream_timeout" || !failed.Abnormal {
		t.Fatalf("reverse inbound failure = %+v", failed)
	}
}

func TestRuntimeEventsOnlyLogOperationalDiagnostics(t *testing.T) {
	var serviceLogs []string
	server := &Server{
		logf: func(format string, args ...any) {
			serviceLogs = append(serviceLogs, fmt.Sprintf(format, args...))
		},
		businessLog: newJSONLLog(filepath.Join(t.TempDir(), "business.jsonl")),
	}
	defer server.Close()

	server.recordEvent(RuntimeEvent{Kind: "open", Result: "connected"})
	server.recordEvent(RuntimeEvent{Kind: "compatibility", Result: "rejected"})
	server.recordEvent(RuntimeEvent{Kind: "forward_check", Result: "failed", Code: "target_unreachable"})
	if len(serviceLogs) != 1 || !strings.Contains(serviceLogs[0], "event kind=forward_check result=failed") {
		t.Fatalf("service logs should retain only operational diagnostics: %#v", serviceLogs)
	}
}

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
		TLS:        clienttunnel.TLSConfig{CACertFile: certFile, ServerName: "127.0.0.1"},
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
		TLS:        clienttunnel.TLSConfig{CACertFile: certFile, ServerName: "127.0.0.1"},
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
		events:             newRuntimeEventRing(500),
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
