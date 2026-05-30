//go:build windows

package gui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lsyltunnel/src/server/tunnel"
)

func TestBuildLogAnalysisAggregatesSuccessAndBlockedSources(t *testing.T) {
	dir := t.TempDir()
	businessBase := filepath.Join(dir, "business.jsonl")
	requestBase := filepath.Join(dir, "request.jsonl")
	date := "2026-05-29"
	writeAnalysisLogLines(t, datedAnalysisLogPath(businessBase, date), []string{
		`{"time":"2026-05-29T08:26:44+08:00","request_id":"req-open","kind":"open","result":"connected","remote_ip":"183.156.189.113","username":"hzls","client_id":"LAPTOP-LHJ","forward_name":"rdp","target":"127.0.0.1:3389","code":"ok"}`,
		`{"time":"2026-05-29T08:28:39+08:00","request_id":"req-open","kind":"open","result":"closed","remote_ip":"183.156.189.113","username":"hzls","client_id":"LAPTOP-LHJ","forward_name":"rdp","target":"127.0.0.1:3389","code":"closed","duration_ms":103233,"bytes_up":1024,"bytes_down":2048}`,
		`{"time":"2026-05-29T09:00:00+08:00","request_id":"req-blocked","kind":"auth","result":"blocked","remote_ip":"165.154.36.107","code":"auth_blocked","message":"too many login failures"}`,
		`{"time":"2026-05-29T09:10:00+08:00","request_id":"req-perm","kind":"auth","result":"blocked","remote_ip":"39.98.184.104","code":"ip_permanently_blocked","message":"too many invalid tunnel requests"}`,
	})
	writeAnalysisLogLines(t, datedAnalysisLogPath(requestBase, date), []string{
		`{"time":"2026-05-29T08:26:33+08:00","request_id":"req-health","remote_ip":"8.8.8.8","request":{"type":"health","username":"hzls","client_id":"LAPTOP-LHJ"},"auth_result":"ok","response":{"ok":true,"code":"ok"},"result":"ok","duration_ms":602}`,
		`{"time":"2026-05-29T08:26:44+08:00","request_id":"req-open","remote_ip":"183.156.189.113","request":{"type":"open","username":"hzls","client_id":"LAPTOP-LHJ","forward_name":"rdp","target":"127.0.0.1:3389"},"auth_result":"ok","response":{"ok":true,"code":"ok"},"result":"ok","duration_ms":641}`,
		`{"time":"2026-05-29T09:00:00+08:00","request_id":"req-blocked","remote_ip":"165.154.36.107","request":{"type":"","username":""},"auth_result":"blocked","response":{"ok":false,"code":"auth_blocked","message":"too many login failures, try later"},"result":"blocked","duration_ms":0}`,
		`{"time":"2026-05-29T09:10:00+08:00","request_id":"req-perm-probe","remote_ip":"39.98.184.104","request":{"type":"","username":""},"auth_result":"not_attempted","response":{"ok":false,"code":"bad_request","message":"invalid tunnel request"},"result":"failed","duration_ms":10}`,
	})

	start, _ := time.Parse(time.RFC3339, "2026-05-29T00:00:00+08:00")
	end, _ := time.Parse(time.RFC3339, "2026-05-29T23:59:59+08:00")
	app := &App{configPath: filepath.Join(dir, "conf", "server.yaml")}
	result, err := app.buildLogAnalysis(context.Background(), tunnel.Config{
		Runtime: tunnel.RuntimeConfig{
			BusinessLogFile: businessBase,
			RequestLogFile:  requestBase,
		},
	}, start, end, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(result.SuccessSources); got != 1 {
		t.Fatalf("success sources = %d, want 1: %#v", got, result.SuccessSources)
	}
	success := result.SuccessSources[0]
	if success.IP != "183.156.189.113" || success.SuccessEvents != 1 || success.BytesDown != 2048 {
		t.Fatalf("unexpected success source: %#v", success)
	}
	if len(result.BlockedSources) != 2 {
		t.Fatalf("blocked sources = %d, want 2: %#v", len(result.BlockedSources), result.BlockedSources)
	}
	blockedByIP := map[string]logAnalysisIPSource{}
	for _, source := range result.BlockedSources {
		blockedByIP[source.IP] = source
	}
	if blockedByIP["165.154.36.107"].AuthBlockedEvents != 1 {
		t.Fatalf("auth blocked source not aggregated: %#v", blockedByIP["165.154.36.107"])
	}
	if blockedByIP["39.98.184.104"].PermanentBlockedEvents != 1 {
		t.Fatalf("permanent blocked source not aggregated: %#v", blockedByIP["39.98.184.104"])
	}
	for _, source := range result.SuccessSources {
		if source.IP == "8.8.8.8" {
			t.Fatalf("health-only IP should not become a success source")
		}
	}
}

func writeAnalysisLogLines(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func datedAnalysisLogPath(basePath, date string) string {
	ext := filepath.Ext(basePath)
	name := strings.TrimSuffix(filepath.Base(basePath), ext)
	if ext == "" {
		ext = ".jsonl"
	}
	return filepath.Join(filepath.Dir(basePath), name+"-"+date+ext)
}
