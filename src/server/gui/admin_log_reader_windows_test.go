//go:build windows

package gui

import (
	"testing"

	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/server/tunnel"
)

func TestBuildBusinessFlowMergesConnectionLifecycle(t *testing.T) {
	rows := buildBusinessFlow([]tunnel.BusinessLogEntry{
		{
			Time:        "2026-05-29T10:05:00+08:00",
			RequestID:   "req-1",
			Kind:        "open",
			Result:      "closed",
			RemoteIP:    "203.0.113.10",
			Username:    "alice",
			ClientID:    "client-a",
			ForwardName: "web",
			Direction:   tunnel.DirectionClientToServer,
			Target:      "127.0.0.1:8080",
			Code:        "closed",
			DurationMS:  5 * 60 * 1000,
			BytesUp:     1024,
			BytesDown:   2048,
		},
		{
			Time:        "2026-05-29T10:00:00+08:00",
			RequestID:   "req-1",
			Kind:        "open",
			Result:      "connected",
			RemoteIP:    "203.0.113.10",
			Username:    "alice",
			ClientID:    "client-a",
			ForwardName: "web",
			Direction:   tunnel.DirectionClientToServer,
			Target:      "127.0.0.1:8080",
			Code:        "ok",
		},
	}, nil, 10)

	if len(rows) != 1 {
		t.Fatalf("rows length = %d, want 1: %#v", len(rows), rows)
	}
	row := rows[0]
	if row.Result != "closed" {
		t.Fatalf("Result = %q, want closed", row.Result)
	}
	if row.StartedAt != "2026-05-29T10:00:00+08:00" || row.EndedAt != "2026-05-29T10:05:00+08:00" {
		t.Fatalf("period = %q - %q", row.StartedAt, row.EndedAt)
	}
	if row.Username != "alice" || row.RemoteIP != "203.0.113.10" {
		t.Fatalf("actor = %q / %q", row.Username, row.RemoteIP)
	}
	if row.BytesUp != 1024 || row.BytesDown != 2048 || row.DurationMS != 5*60*1000 {
		t.Fatalf("traffic/duration not preserved: %#v", row)
	}
}

func TestBuildBusinessFlowSupplementsFailedRequestLogs(t *testing.T) {
	rows := buildBusinessFlow(nil, []tunnel.RequestLogEntry{
		{
			Time:      "2026-05-29T11:00:00+08:00",
			RequestID: "req-bad",
			RemoteIP:  "203.0.113.20",
			Request: protocol.OpenRequest{
				Type:     "bad_type",
				Username: "bob",
			},
			Response: protocol.OpenResponse{OK: false, Code: "bad_request", Message: "unsupported request"},
			Result:   "failed",
		},
		{
			Time:      "2026-05-29T11:01:00+08:00",
			RequestID: "req-health",
			RemoteIP:  "203.0.113.20",
			Request:   protocol.OpenRequest{Type: "health", Username: "bob"},
			Response:  protocol.OpenResponse{OK: true, Code: "ok"},
			Result:    "ok",
		},
	}, 10)

	if len(rows) != 1 {
		t.Fatalf("rows length = %d, want 1: %#v", len(rows), rows)
	}
	row := rows[0]
	if row.Kind != "request" || row.Result != "failed" || row.Code != "bad_request" {
		t.Fatalf("unexpected supplemented row: %#v", row)
	}
	if row.Username != "bob" || row.RemoteIP != "203.0.113.20" {
		t.Fatalf("actor = %q / %q", row.Username, row.RemoteIP)
	}
}
