//go:build windows

package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"lsyltunnel/src/server/tunnel"
)

func TestMonitorGUIProjectionPreservesJSON(t *testing.T) {
	wire := tunnel.MonitorStatus{
		Service: "lsyl-tunnel-server", ListenAddr: "127.0.0.1:3443", UptimeSec: 42,
		ActiveConnections: 2, TrackedRemoteIPs: 1, ConnectionsRejected: 3,
		ConnectionLimits:        &tunnel.ConnectionLimitsStatus{Active: 2, MaxConcurrentConnections: 100},
		ConnectionRejections:    &tunnel.ConnectionRejectionsStatus{Total: 3},
		EntrySecurity:           &tunnel.EntrySecurityStatus{HTTPProbeRejected: 4},
		FailureTracker:          &tunnel.FailureTrackerStatus{EvictedIPs: 9007199254740993},
		EntryTrafficLog:         &tunnel.EntryTrafficLogStatus{Written: 5},
		UserStreamLimits:        &tunnel.UserStreamLimitsStatus{Active: 6, ActiveByUser: map[string]int{"alice": 6}},
		UserStreamLimitRejected: 7, ActiveStreams: 8, TotalStreams: 9,
		AuthOK: 10, AuthFailed: 11, PolicyRejected: 12, DialFailed: 13, BytesUp: 14, BytesDown: 15,
		BlockedIPs: []tunnel.BlockedIPState{}, RecentEvents: []tunnel.RuntimeEvent{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(wire)
	}))
	defer srv.Close()
	got, err := loadMonitorStatus(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var wantFields, gotFields map[string]json.RawMessage
	if err := json.Unmarshal(wantJSON, &wantFields); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(gotJSON, &gotFields); err != nil {
		t.Fatal(err)
	}
	delete(wantFields, "tracked_remote_ips")
	delete(wantFields, "blocked_ips")
	delete(wantFields, "recent_events")
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("GUI projection changed: %s", gotJSON)
	}
}

func TestMonitorGUIFallbackOmitsUnavailableSections(t *testing.T) {
	data, err := json.Marshal(&monitorStatus{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tracked_remote_ips", "connection_limits", "connection_rejections", "entry_security", "failure_tracker", "entry_traffic_log", "user_stream_limits", "blocked_ips", "recent_events"} {
		if _, ok := fields[key]; ok {
			t.Errorf("fallback unexpectedly emitted %s", key)
		}
	}
}
