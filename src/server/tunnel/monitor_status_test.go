package tunnel

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestMonitorStatusJSONContract(t *testing.T) {
	s := &Server{started: time.Now()}
	status := s.status()
	status.UptimeSec = 0
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{
  "service": "lsyl-tunnel-server",
  "listen_addr": "",
  "uptime_sec": 0,
  "active_connections": 0,
  "tracked_remote_ips": 0,
  "connections_rejected": 0,
  "connection_limits": {
    "active": 0,
    "tracked_remote_ips": 0,
    "max_tracked_remote_ips": 0,
    "tracked_remote_ip_evictions": 0,
    "tracked_remote_ip_capacity_rejected": 0,
    "max_concurrent_connections": 0,
    "max_concurrent_connections_per_ip": 0,
    "new_connection_rate_window_sec": 0,
    "max_new_connections_per_ip_window": 0
  },
  "connection_rejections": {
    "total": 0,
    "global_concurrent": 0,
    "per_ip_concurrent": 0,
    "per_ip_new_connection_rate": 0,
    "tracked_remote_ip_capacity": 0
  },
  "entry_security": {
    "protocol_rejected_total": 0,
    "http_probe_rejected": 0,
    "non_tls_rejected": 0,
    "unsupported_tls_rejected": 0,
    "slow_handshake_rejected": 0,
    "closed_handshake_rejected": 0,
    "oversized_handshake_rejected": 0,
    "unsupported_request_rejected": 0,
    "invalid_handshake_rejected": 0,
    "permanent_blocks_created": 0,
    "permanent_block_hits_observed": 0
  },
  "failure_tracker": {
    "tracked_ips": 0,
    "max_tracked_ips": 0,
    "evicted_ips": 0,
    "capacity_drops": 0
  },
  "entry_traffic_log": {
    "queue_capacity": 0,
    "queue_pending": 0,
    "accepted": 0,
    "written": 0,
    "dropped": 0
  },
  "user_stream_limits": {
    "active": 0,
    "tracked_users": 0,
    "active_by_user": {},
    "max_concurrent_streams_per_user": 0,
    "stream_rate_limit_bytes_per_sec": 0,
    "user_stream_limit_rejections_total": 0
  },
  "user_stream_limit_rejected": 0,
  "active_streams": 0,
  "total_streams": 0,
  "auth_ok": 0,
  "auth_failed": 0,
  "policy_rejected": 0,
  "dial_failed": 0,
  "bytes_up": 0,
  "bytes_down": 0,
  "blocked_ips": null,
  "recent_events": []
}`
	var gotValue, wantValue any
	if err := json.Unmarshal(data, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("monitor contract changed: %s", data)
	}
	var decoded MonitorStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(status, decoded) {
		t.Fatal("monitor status does not round trip")
	}
}

func TestMonitorStatusReportsLiveCounters(t *testing.T) {
	cfg := SecurityConfig{MaxConcurrentConnections: 10, MaxConcurrentConnectionsPerIP: 2,
		MaxNewConnectionsPerIPWindow: 3, ConnectionRateWindowSec: 7, MaxTrackedConnectionIPs: 20,
		MaxTrackedFailureIPs: 30, MaxConcurrentStreamsPerUser: 4, StreamRateLimitBytesPerSec: 1024}
	s := &Server{cfg: Config{Security: cfg}, started: time.Now(), connLimiter: newConnectionLimiter(cfg),
		fails: newFailTracker(cfg, "", ""), userStreams: newUserStreamLimiter(cfg)}
	release, ok, _ := s.connLimiter.acquire("192.0.2.1")
	if !ok {
		t.Fatal("connection rejected")
	}
	defer release()
	releaseStream, ok := s.userStreams.acquire("alice")
	if !ok {
		t.Fatal("stream rejected")
	}
	defer releaseStream()
	s.connectionsRejected.Add(11)
	s.connectionsRejectedIPCapacity.Add(12)
	s.entryHTTPProbeRejected.Add(13)
	s.authOK.Add(14)
	s.userStreamLimitRejected.Add(15)
	s.bytesUp.Add(16)
	s.fails.evicted.Add(17)
	status := s.status()
	if status.ActiveConnections != 1 || status.TrackedRemoteIPs != 1 ||
		status.ConnectionLimits.MaxTrackedRemoteIPs != 20 || status.ConnectionLimits.MaxConcurrentConnections != 10 ||
		status.ConnectionLimits.MaxConcurrentConnectionsPerIP != 2 || status.ConnectionLimits.NewConnectionRateWindowSec != 7 ||
		status.ConnectionLimits.MaxNewConnectionsPerIPWindow != 3 {
		t.Fatalf("connection snapshot = %+v", status.ConnectionLimits)
	}
	if status.ConnectionsRejected != 11 || status.ConnectionRejections.Total != 11 ||
		status.ConnectionRejections.TrackedRemoteIPCapacity != 12 || status.EntrySecurity.HTTPProbeRejected != 13 ||
		status.AuthOK != 14 || status.UserStreamLimitRejected != 15 || status.BytesUp != 16 ||
		status.FailureTracker.EvictedIPs != 17 || status.FailureTracker.MaxTrackedIPs != 30 {
		t.Fatalf("counters = %+v", status)
	}
	if status.UserStreamLimits.Active != 1 || status.UserStreamLimits.ActiveByUser["alice"] != 1 ||
		status.UserStreamLimits.MaxConcurrentStreamsPerUser != 4 || status.UserStreamLimits.StreamRateLimitBytesPerSec != 1024 ||
		status.UserStreamLimits.UserStreamLimitRejectionsTotal != 15 {
		t.Fatalf("user limits = %+v", status.UserStreamLimits)
	}
	status.UserStreamLimits.ActiveByUser["alice"] = 99
	if s.status().UserStreamLimits.ActiveByUser["alice"] != 1 {
		t.Fatal("snapshot aliases live limiter")
	}
}
