package tunnel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lsyltunnel/src/internal/protocol"
)

func TestSuccessfulRequestForBlockProtection(t *testing.T) {
	supported := []string{"login", "health", "open", "forward_check", "reverse", "reverse_listen", "reverse_stream"}
	for _, requestType := range supported {
		entry := successfulRequestEntry(time.Now(), "203.0.113.10", requestType)
		if !isSuccessfulRequestForBlockProtection(entry) {
			t.Fatalf("request type %q was not accepted", requestType)
		}
	}

	tests := []struct {
		name  string
		entry RequestLogEntry
	}{
		{name: "failed result", entry: RequestLogEntry{RemoteIP: "203.0.113.10", Result: "failed", AuthResult: "ok", Response: protocol.OpenResponse{OK: true}, Request: protocol.OpenRequest{Type: "login"}}},
		{name: "failed auth", entry: RequestLogEntry{RemoteIP: "203.0.113.10", Result: "ok", AuthResult: "failed", Response: protocol.OpenResponse{OK: true}, Request: protocol.OpenRequest{Type: "login"}}},
		{name: "failed response", entry: RequestLogEntry{RemoteIP: "203.0.113.10", Result: "ok", AuthResult: "ok", Response: protocol.OpenResponse{OK: false}, Request: protocol.OpenRequest{Type: "login"}}},
		{name: "unsupported request", entry: RequestLogEntry{RemoteIP: "203.0.113.10", Result: "ok", AuthResult: "ok", Response: protocol.OpenResponse{OK: true}, Request: protocol.OpenRequest{Type: "probe"}}},
		{name: "invalid IP", entry: RequestLogEntry{RemoteIP: "not-an-ip", Result: "ok", AuthResult: "ok", Response: protocol.OpenResponse{OK: true}, Request: protocol.OpenRequest{Type: "login"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if isSuccessfulRequestForBlockProtection(tt.entry) {
				t.Fatalf("entry unexpectedly qualified: %+v", tt.entry)
			}
		})
	}
}

func TestScanRecentSuccessfulRequestIPsUsesSevenLocalCalendarDays(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.Local)
	basePath := filepath.Join(t.TempDir(), "request.jsonl")

	writeSuccessfulIPHistoryLog(t, datedJSONLPath(basePath, "2026-08-04"), []RequestLogEntry{
		successfulRequestEntry(time.Date(2026, time.August, 4, 8, 0, 0, 0, time.Local), "203.0.113.10", "login"),
		successfulRequestEntry(time.Date(2026, time.July, 28, 23, 59, 0, 0, time.Local), "203.0.113.99", "open"),
		{Time: time.Date(2026, time.August, 4, 9, 0, 0, 0, time.Local).Format(time.RFC3339), RemoteIP: "not-an-ip", Result: "ok", AuthResult: "ok", Response: protocol.OpenResponse{OK: true}, Request: protocol.OpenRequest{Type: "login"}},
	}, "{broken-json")
	writeSuccessfulIPHistoryLog(t, datedJSONLPath(basePath, "2026-07-29"), []RequestLogEntry{
		successfulRequestEntry(time.Date(2026, time.July, 29, 0, 1, 0, 0, time.Local), "2001:0db8:0:0:0:0:0:1", "health"),
	}, "")
	writeSuccessfulIPHistoryLog(t, datedJSONLPath(basePath, "2026-07-28"), []RequestLogEntry{
		successfulRequestEntry(time.Date(2026, time.July, 28, 12, 0, 0, 0, time.Local), "203.0.113.20", "open"),
	}, "")
	writeSuccessfulIPHistoryLog(t, basePath, []RequestLogEntry{
		successfulRequestEntry(time.Date(2026, time.August, 4, 10, 0, 0, 0, time.Local), "203.0.113.30", "reverse_stream"),
		{Time: time.Date(2026, time.August, 4, 10, 1, 0, 0, time.Local).Format(time.RFC3339), RemoteIP: "203.0.113.40", Result: "denied", AuthResult: "ok", Response: protocol.OpenResponse{OK: false}, Request: protocol.OpenRequest{Type: "forward_check"}},
	}, "")

	visited := map[string]time.Time{}
	err := scanRecentSuccessfulRequestIPs(basePath, now, 7, func(ip string, occurredAt time.Time) {
		visited[ip] = occurredAt
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"203.0.113.10", "2001:db8::1", "203.0.113.30"} {
		if _, ok := visited[ip]; !ok {
			t.Fatalf("visited IPs = %#v, missing %s", visited, ip)
		}
	}
	for _, ip := range []string{"203.0.113.20", "203.0.113.40", "203.0.113.99"} {
		if _, ok := visited[ip]; ok {
			t.Fatalf("visited IPs = %#v, unexpectedly included %s", visited, ip)
		}
	}
}

func TestSuccessfulIPSkipsPermanentProtocolBlock(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.Local)
	tracker := newFailTracker(SecurityConfig{AuthFailWindowSec: 60, AuthFailThreshold: 2, AuthFailBlockSec: 60, MaxTrackedFailureIPs: 8}, "", filepath.Join(t.TempDir(), "permanent.txt"))
	tracker.now = func() time.Time { return now }

	if !tracker.markSuccessful("203.0.113.10") {
		t.Fatal("successful IP was not tracked")
	}
	if tracker.addProtocolFailure("203.0.113.10") || tracker.addProtocolFailure("203.0.113.10") {
		t.Fatal("successful IP reached permanent block threshold")
	}
	if tracker.hasPermanent("203.0.113.10") {
		t.Fatal("successful IP was permanently blocked")
	}
}

func TestSuccessfulIPProtectionExpiresOnNextLocalDay(t *testing.T) {
	now := time.Date(2026, time.August, 4, 23, 59, 0, 0, time.Local)
	tracker := newFailTracker(SecurityConfig{AuthFailWindowSec: 60, AuthFailThreshold: 2, AuthFailBlockSec: 60, MaxTrackedFailureIPs: 8}, "", filepath.Join(t.TempDir(), "permanent.txt"))
	tracker.now = func() time.Time { return now }
	if !tracker.markSuccessful("203.0.113.10") {
		t.Fatal("successful IP was not tracked")
	}

	now = time.Date(2026, time.August, 5, 0, 1, 0, 0, time.Local)
	if tracker.addProtocolFailure("203.0.113.10") {
		t.Fatal("first protocol failure created permanent block")
	}
	if !tracker.addProtocolFailure("203.0.113.10") {
		t.Fatal("next-day protocol failures did not create permanent block")
	}
}

func TestSuccessfulIPStillReceivesTemporaryAuthBlock(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.Local)
	tracker := newFailTracker(SecurityConfig{AuthFailWindowSec: 60, AuthFailThreshold: 2, AuthFailBlockSec: 60, MaxTrackedFailureIPs: 8}, filepath.Join(t.TempDir(), "state.json"), filepath.Join(t.TempDir(), "permanent.txt"))
	tracker.now = func() time.Time { return now }
	tracker.markSuccessful("203.0.113.10")
	tracker.addFailure("203.0.113.10")
	tracker.addFailure("203.0.113.10")
	if got := tracker.blockKind("203.0.113.10"); got != blockedTemporary {
		t.Fatalf("block kind = %v, want temporary", got)
	}
}

func TestSuccessfulIPClearsOnlyProtocolFailures(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.Local)
	tracker := newFailTracker(SecurityConfig{AuthFailWindowSec: 60, AuthFailThreshold: 5, AuthFailBlockSec: 60, MaxTrackedFailureIPs: 8}, "", "")
	tracker.now = func() time.Time { return now }
	blockedUntil := now.Add(time.Minute)
	tracker.items["203.0.113.10"] = &failState{
		authFailures:     []time.Time{now},
		protocolFailures: []time.Time{now},
		blockedUntil:     blockedUntil,
		lastSeen:         now,
	}

	tracker.markSuccessful("203.0.113.10")
	state := tracker.items["203.0.113.10"]
	if len(state.protocolFailures) != 0 {
		t.Fatalf("protocol failures = %v, want empty", state.protocolFailures)
	}
	if len(state.authFailures) != 1 || !state.blockedUntil.Equal(blockedUntil) {
		t.Fatalf("authentication state changed: %+v", state)
	}
}

func TestSuccessfulIPSetIsBounded(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.Local)
	tracker := newFailTracker(SecurityConfig{MaxTrackedFailureIPs: 2}, "", "")
	tracker.now = func() time.Time { return now }
	if !tracker.markSuccessful("203.0.113.10") || !tracker.markSuccessful("203.0.113.20") {
		t.Fatal("success set rejected an IP below capacity")
	}
	if tracker.markSuccessful("203.0.113.30") {
		t.Fatal("success set accepted an IP above capacity")
	}
	if got := len(tracker.successfulIPs); got != 2 {
		t.Fatalf("successful IP count = %d, want 2", got)
	}
}

func TestReconcileSuccessfulIPHistoryRemovesRecentPermanentBlocks(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.Local)
	dir := t.TempDir()
	permanentFile := filepath.Join(dir, "server-permanent-block.txt")
	requestLog := filepath.Join(dir, "request", "request.jsonl")
	input := "# keep comment\n203.0.113.10\n203.0.113.20\n203.0.113.30\n\n"
	if err := os.WriteFile(permanentFile, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(requestLog), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSuccessfulIPHistoryLog(t, datedJSONLPath(requestLog, "2026-08-04"), []RequestLogEntry{
		successfulRequestEntry(time.Date(2026, time.August, 4, 8, 0, 0, 0, time.Local), "203.0.113.10", "login"),
		successfulRequestEntry(time.Date(2026, time.August, 4, 9, 0, 0, 0, time.Local), "203.0.113.40", "health"),
	}, "")
	writeSuccessfulIPHistoryLog(t, datedJSONLPath(requestLog, "2026-07-29"), []RequestLogEntry{
		successfulRequestEntry(time.Date(2026, time.July, 29, 8, 0, 0, 0, time.Local), "203.0.113.20", "open"),
	}, "")
	writeSuccessfulIPHistoryLog(t, datedJSONLPath(requestLog, "2026-07-28"), []RequestLogEntry{
		successfulRequestEntry(time.Date(2026, time.July, 28, 8, 0, 0, 0, time.Local), "203.0.113.30", "open"),
	}, "")

	tracker := newFailTracker(SecurityConfig{MaxTrackedFailureIPs: 8}, "", permanentFile)
	tracker.now = func() time.Time { return now }
	if err := tracker.load(); err != nil {
		t.Fatal(err)
	}
	if err := reconcileSuccessfulIPHistory(tracker, requestLog, now); err != nil {
		t.Fatal(err)
	}

	for _, ip := range []string{"203.0.113.10", "203.0.113.20"} {
		if tracker.hasPermanent(ip) {
			t.Fatalf("recent successful IP %s remained permanently blocked", ip)
		}
	}
	if !tracker.hasPermanent("203.0.113.30") {
		t.Fatal("seven-day-old success removed a permanent block")
	}

	tracker.mu.Lock()
	todayProtected := tracker.successfulTodayLocked("203.0.113.10", now) && tracker.successfulTodayLocked("203.0.113.40", now)
	oldProtected := tracker.successfulTodayLocked("203.0.113.20", now)
	tracker.mu.Unlock()
	if !todayProtected {
		t.Fatal("today's successful IPs were not seeded")
	}
	if oldProtected {
		t.Fatal("an older successful IP was seeded into today's set")
	}

	data, err := os.ReadFile(permanentFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "203.0.113.10") || strings.Contains(text, "203.0.113.20") {
		t.Fatalf("recent successful IP remained in file: %q", text)
	}
	if !strings.Contains(text, "# keep comment") || !strings.Contains(text, "203.0.113.30") || !strings.HasSuffix(text, "\n\n") {
		t.Fatalf("unrelated permanent block content changed: %q", text)
	}
}

func TestRecordRequestLogMarksSuccessfulIP(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.Local)
	tracker := newFailTracker(SecurityConfig{AuthFailWindowSec: 60, AuthFailThreshold: 2, AuthFailBlockSec: 60, MaxTrackedFailureIPs: 8}, "", filepath.Join(t.TempDir(), "permanent.txt"))
	tracker.now = func() time.Time { return now }
	server := &Server{fails: tracker}

	server.recordRequestLog(successfulRequestEntry(now, "203.0.113.50", "login"))
	tracker.addProtocolFailure("203.0.113.50")
	tracker.addProtocolFailure("203.0.113.50")
	if tracker.hasPermanent("203.0.113.50") {
		t.Fatal("recorded successful request did not protect its IP")
	}

	server.recordRequestLog(RequestLogEntry{
		Time:       now.Format(time.RFC3339),
		RemoteIP:   "203.0.113.60",
		Request:    protocol.OpenRequest{Type: "forward_check"},
		AuthResult: "ok",
		Response:   protocol.OpenResponse{OK: false, Code: "target_denied"},
		Result:     "denied",
	})
	tracker.addProtocolFailure("203.0.113.60")
	tracker.addProtocolFailure("203.0.113.60")
	if !tracker.hasPermanent("203.0.113.60") {
		t.Fatal("denied request unexpectedly protected its IP")
	}
}

func successfulRequestEntry(at time.Time, ip, requestType string) RequestLogEntry {
	return RequestLogEntry{
		Time:       at.Format(time.RFC3339),
		RemoteIP:   ip,
		Request:    protocol.OpenRequest{Type: requestType},
		AuthResult: "ok",
		Response:   protocol.OpenResponse{OK: true, Code: "ok"},
		Result:     "ok",
	}
}

func writeSuccessfulIPHistoryLog(t *testing.T, path string, entries []RequestLogEntry, rawLine string) {
	t.Helper()
	lines := make([]string, 0, len(entries)+1)
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(data))
	}
	if rawLine != "" {
		lines = append(lines, rawLine)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
