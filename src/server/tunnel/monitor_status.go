package tunnel

import "time"

// MonitorStatus is the /status wire contract shared by the server and its GUI.
// Fields are always emitted, including zero counters and empty collections.
type MonitorStatus struct {
	Service                 string                      `json:"service"`
	ListenAddr              string                      `json:"listen_addr"`
	UptimeSec               int64                       `json:"uptime_sec"`
	ActiveConnections       int                         `json:"active_connections"`
	TrackedRemoteIPs        int                         `json:"tracked_remote_ips"`
	ConnectionsRejected     int64                       `json:"connections_rejected"`
	ConnectionLimits        *ConnectionLimitsStatus     `json:"connection_limits"`
	ConnectionRejections    *ConnectionRejectionsStatus `json:"connection_rejections"`
	EntrySecurity           *EntrySecurityStatus        `json:"entry_security"`
	FailureTracker          *FailureTrackerStatus       `json:"failure_tracker"`
	EntryTrafficLog         *EntryTrafficLogStatus      `json:"entry_traffic_log"`
	UserStreamLimits        *UserStreamLimitsStatus     `json:"user_stream_limits"`
	UserStreamLimitRejected int64                       `json:"user_stream_limit_rejected"`
	ActiveStreams           int64                       `json:"active_streams"`
	TotalStreams            int64                       `json:"total_streams"`
	AuthOK                  int64                       `json:"auth_ok"`
	AuthFailed              int64                       `json:"auth_failed"`
	PolicyRejected          int64                       `json:"policy_rejected"`
	DialFailed              int64                       `json:"dial_failed"`
	BytesUp                 int64                       `json:"bytes_up"`
	BytesDown               int64                       `json:"bytes_down"`
	BlockedIPs              []BlockedIPState            `json:"blocked_ips"`
	RecentEvents            []RuntimeEvent              `json:"recent_events"`
}

type ConnectionLimitsStatus struct {
	Active                          int `json:"active"`
	TrackedRemoteIPs                int `json:"tracked_remote_ips"`
	MaxTrackedRemoteIPs             int `json:"max_tracked_remote_ips"`
	TrackedRemoteIPEvictions        int `json:"tracked_remote_ip_evictions"`
	TrackedRemoteIPCapacityRejected int `json:"tracked_remote_ip_capacity_rejected"`
	MaxConcurrentConnections        int `json:"max_concurrent_connections"`
	MaxConcurrentConnectionsPerIP   int `json:"max_concurrent_connections_per_ip"`
	NewConnectionRateWindowSec      int `json:"new_connection_rate_window_sec"`
	MaxNewConnectionsPerIPWindow    int `json:"max_new_connections_per_ip_window"`
}

type ConnectionRejectionsStatus struct {
	Total                   int64 `json:"total"`
	GlobalConcurrent        int64 `json:"global_concurrent"`
	PerIPConcurrent         int64 `json:"per_ip_concurrent"`
	PerIPNewConnectionRate  int64 `json:"per_ip_new_connection_rate"`
	TrackedRemoteIPCapacity int64 `json:"tracked_remote_ip_capacity"`
}

type EntrySecurityStatus struct {
	ProtocolRejectedTotal      int64 `json:"protocol_rejected_total"`
	HTTPProbeRejected          int64 `json:"http_probe_rejected"`
	NonTLSRejected             int64 `json:"non_tls_rejected"`
	UnsupportedTLSRejected     int64 `json:"unsupported_tls_rejected"`
	SlowHandshakeRejected      int64 `json:"slow_handshake_rejected"`
	ClosedHandshakeRejected    int64 `json:"closed_handshake_rejected"`
	OversizedHandshakeRejected int64 `json:"oversized_handshake_rejected"`
	UnsupportedRequestRejected int64 `json:"unsupported_request_rejected"`
	InvalidHandshakeRejected   int64 `json:"invalid_handshake_rejected"`
	PermanentBlocksCreated     int64 `json:"permanent_blocks_created"`
	PermanentBlockHitsObserved int64 `json:"permanent_block_hits_observed"`
}

type FailureTrackerStatus struct {
	TrackedIPs    int    `json:"tracked_ips"`
	MaxTrackedIPs int    `json:"max_tracked_ips"`
	EvictedIPs    uint64 `json:"evicted_ips"`
	CapacityDrops uint64 `json:"capacity_drops"`
}

type EntryTrafficLogStatus struct {
	QueueCapacity int    `json:"queue_capacity"`
	QueuePending  int    `json:"queue_pending"`
	Accepted      uint64 `json:"accepted"`
	Written       uint64 `json:"written"`
	Dropped       uint64 `json:"dropped"`
}

type UserStreamLimitsStatus struct {
	Active                         int            `json:"active"`
	TrackedUsers                   int            `json:"tracked_users"`
	ActiveByUser                   map[string]int `json:"active_by_user"`
	MaxConcurrentStreamsPerUser    int            `json:"max_concurrent_streams_per_user"`
	StreamRateLimitBytesPerSec     int            `json:"stream_rate_limit_bytes_per_sec"`
	UserStreamLimitRejectionsTotal int64          `json:"user_stream_limit_rejections_total"`
}

func (s *Server) status() MonitorStatus {
	connectionLimits := s.connLimiter.snapshot()
	userStreamLimits := s.userStreams.snapshot()
	failureStats := s.fails.stats()
	entryTrafficStats := s.entryTrafficWriter.stats()
	return MonitorStatus{
		Service:             "lsyl-tunnel-server",
		ListenAddr:          s.Addr(),
		UptimeSec:           int64(time.Since(s.started).Seconds()),
		ActiveConnections:   connectionLimits["active"],
		TrackedRemoteIPs:    connectionLimits["tracked_ips"],
		ConnectionsRejected: s.connectionsRejected.Load(),
		ConnectionLimits: &ConnectionLimitsStatus{
			Active:                          connectionLimits["active"],
			TrackedRemoteIPs:                connectionLimits["tracked_ips"],
			MaxTrackedRemoteIPs:             connectionLimits["max_tracked_ips"],
			TrackedRemoteIPEvictions:        connectionLimits["evicted_ips"],
			TrackedRemoteIPCapacityRejected: connectionLimits["capacity_rejected"],
			MaxConcurrentConnections:        s.cfg.Security.MaxConcurrentConnections,
			MaxConcurrentConnectionsPerIP:   s.cfg.Security.MaxConcurrentConnectionsPerIP,
			NewConnectionRateWindowSec:      s.cfg.Security.ConnectionRateWindowSec,
			MaxNewConnectionsPerIPWindow:    s.cfg.Security.MaxNewConnectionsPerIPWindow,
		},
		ConnectionRejections: &ConnectionRejectionsStatus{
			Total:                   s.connectionsRejected.Load(),
			GlobalConcurrent:        s.connectionsRejectedGlobal.Load(),
			PerIPConcurrent:         s.connectionsRejectedPerIPActive.Load(),
			PerIPNewConnectionRate:  s.connectionsRejectedPerIPNewRate.Load(),
			TrackedRemoteIPCapacity: s.connectionsRejectedIPCapacity.Load(),
		},
		EntrySecurity: &EntrySecurityStatus{
			ProtocolRejectedTotal:      s.entryProtocolRejected.Load(),
			HTTPProbeRejected:          s.entryHTTPProbeRejected.Load(),
			NonTLSRejected:             s.entryNonTLSRejected.Load(),
			UnsupportedTLSRejected:     s.entryUnsupportedTLSRejected.Load(),
			SlowHandshakeRejected:      s.entrySlowHandshakeRejected.Load(),
			ClosedHandshakeRejected:    s.entryClosedHandshakeRejected.Load(),
			OversizedHandshakeRejected: s.entryOversizedHandshakeRejected.Load(),
			UnsupportedRequestRejected: s.entryUnsupportedRequestRejected.Load(),
			InvalidHandshakeRejected:   s.entryInvalidHandshakeRejected.Load(),
			PermanentBlocksCreated:     s.entryPermanentBlocksCreated.Load(),
			PermanentBlockHitsObserved: s.entryPermanentBlockHits.Load(),
		},
		FailureTracker: &FailureTrackerStatus{
			TrackedIPs:    failureStats.TrackedIPs,
			MaxTrackedIPs: failureStats.MaxTrackedIPs,
			EvictedIPs:    failureStats.EvictedIPs,
			CapacityDrops: failureStats.CapacityDrops,
		},
		EntryTrafficLog: &EntryTrafficLogStatus{
			QueueCapacity: entryTrafficStats.Capacity,
			QueuePending:  entryTrafficStats.Queued,
			Accepted:      entryTrafficStats.Accepted,
			Written:       entryTrafficStats.Written,
			Dropped:       entryTrafficStats.Dropped,
		},
		UserStreamLimits: &UserStreamLimitsStatus{
			Active:                         userStreamLimits.Active,
			TrackedUsers:                   userStreamLimits.TrackedUsers,
			ActiveByUser:                   userStreamLimits.ActiveByUser,
			MaxConcurrentStreamsPerUser:    userStreamLimits.MaxPerUser,
			StreamRateLimitBytesPerSec:     s.cfg.Security.StreamRateLimitBytesPerSec,
			UserStreamLimitRejectionsTotal: s.userStreamLimitRejected.Load(),
		},
		UserStreamLimitRejected: s.userStreamLimitRejected.Load(),
		ActiveStreams:           s.activeStreams.Load(),
		TotalStreams:            s.totalStreams.Load(),
		AuthOK:                  s.authOK.Load(),
		AuthFailed:              s.authFailed.Load(),
		PolicyRejected:          s.policyRejected.Load(),
		DialFailed:              s.dialFailed.Load(),
		BytesUp:                 s.bytesUp.Load(),
		BytesDown:               s.bytesDown.Load(),
		BlockedIPs:              s.fails.snapshotBlocked(),
		RecentEvents:            s.recentEventSnapshot(),
	}
}
