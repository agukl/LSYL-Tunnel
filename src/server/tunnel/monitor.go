package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

func (s *Server) startMonitor(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.status())
	})
	mux.HandleFunc("/security/unblock", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "method not allowed"})
			return
		}
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid request"})
			return
		}
		ip := strings.TrimSpace(req.IP)
		if net.ParseIP(ip) == nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid ip"})
			return
		}
		removed, err := s.fails.unblock(ip)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "unblocked", "removed": removed})
	})
	mux.HandleFunc("/security/permanent-block/delete", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "method not allowed"})
			return
		}
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid request"})
			return
		}
		ip := strings.TrimSpace(req.IP)
		if net.ParseIP(ip) == nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid ip"})
			return
		}
		removed, err := s.fails.unblockPermanent(ip)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "permanent block deleted", "removed": removed})
	})
	s.httpSrv = &http.Server{
		Addr:        s.cfg.MonitorAddr,
		BaseContext: func(net.Listener) context.Context { return ctx },
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.beginWorker() {
				http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
				return
			}
			defer s.lifecycle.workers.Done()
			mux.ServeHTTP(w, r)
		}),
	}
	s.startWorker(func() {
		err := s.httpSrv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log("monitor failed: %v", err)
		}
	})
}

func (s *Server) status() map[string]any {
	connectionLimits := s.connLimiter.snapshot()
	userStreamLimits := s.userStreams.snapshot()
	failureStats := s.fails.stats()
	entryTrafficStats := s.entryTrafficWriter.stats()
	return map[string]any{
		"service":              "lsyl-tunnel-server",
		"listen_addr":          s.Addr(),
		"uptime_sec":           int64(time.Since(s.started).Seconds()),
		"active_connections":   connectionLimits["active"],
		"tracked_remote_ips":   connectionLimits["tracked_ips"],
		"connections_rejected": s.connectionsRejected.Load(),
		"connection_limits": map[string]any{
			"active":                              connectionLimits["active"],
			"tracked_remote_ips":                  connectionLimits["tracked_ips"],
			"max_tracked_remote_ips":              connectionLimits["max_tracked_ips"],
			"tracked_remote_ip_evictions":         connectionLimits["evicted_ips"],
			"tracked_remote_ip_capacity_rejected": connectionLimits["capacity_rejected"],
			"max_concurrent_connections":          s.cfg.Security.MaxConcurrentConnections,
			"max_concurrent_connections_per_ip":   s.cfg.Security.MaxConcurrentConnectionsPerIP,
			"new_connection_rate_window_sec":      s.cfg.Security.ConnectionRateWindowSec,
			"max_new_connections_per_ip_window":   s.cfg.Security.MaxNewConnectionsPerIPWindow,
		},
		"connection_rejections": map[string]any{
			"total":                      s.connectionsRejected.Load(),
			"global_concurrent":          s.connectionsRejectedGlobal.Load(),
			"per_ip_concurrent":          s.connectionsRejectedPerIPActive.Load(),
			"per_ip_new_connection_rate": s.connectionsRejectedPerIPNewRate.Load(),
			"tracked_remote_ip_capacity": s.connectionsRejectedIPCapacity.Load(),
		},
		"entry_security": map[string]any{
			"protocol_rejected_total":       s.entryProtocolRejected.Load(),
			"http_probe_rejected":           s.entryHTTPProbeRejected.Load(),
			"non_tls_rejected":              s.entryNonTLSRejected.Load(),
			"unsupported_tls_rejected":      s.entryUnsupportedTLSRejected.Load(),
			"slow_handshake_rejected":       s.entrySlowHandshakeRejected.Load(),
			"closed_handshake_rejected":     s.entryClosedHandshakeRejected.Load(),
			"oversized_handshake_rejected":  s.entryOversizedHandshakeRejected.Load(),
			"unsupported_request_rejected":  s.entryUnsupportedRequestRejected.Load(),
			"invalid_handshake_rejected":    s.entryInvalidHandshakeRejected.Load(),
			"permanent_blocks_created":      s.entryPermanentBlocksCreated.Load(),
			"permanent_block_hits_observed": s.entryPermanentBlockHits.Load(),
		},
		"failure_tracker": map[string]any{
			"tracked_ips":     failureStats.TrackedIPs,
			"max_tracked_ips": failureStats.MaxTrackedIPs,
			"evicted_ips":     failureStats.EvictedIPs,
			"capacity_drops":  failureStats.CapacityDrops,
		},
		"entry_traffic_log": map[string]any{
			"queue_capacity": entryTrafficStats.Capacity,
			"queue_pending":  entryTrafficStats.Queued,
			"accepted":       entryTrafficStats.Accepted,
			"written":        entryTrafficStats.Written,
			"dropped":        entryTrafficStats.Dropped,
		},
		"user_stream_limits": map[string]any{
			"active":                             userStreamLimits.Active,
			"tracked_users":                      userStreamLimits.TrackedUsers,
			"active_by_user":                     userStreamLimits.ActiveByUser,
			"max_concurrent_streams_per_user":    userStreamLimits.MaxPerUser,
			"stream_rate_limit_bytes_per_sec":    s.cfg.Security.StreamRateLimitBytesPerSec,
			"user_stream_limit_rejections_total": s.userStreamLimitRejected.Load(),
		},
		"user_stream_limit_rejected": s.userStreamLimitRejected.Load(),
		"active_streams":             s.activeStreams.Load(),
		"total_streams":              s.totalStreams.Load(),
		"auth_ok":                    s.authOK.Load(),
		"auth_failed":                s.authFailed.Load(),
		"policy_rejected":            s.policyRejected.Load(),
		"dial_failed":                s.dialFailed.Load(),
		"bytes_up":                   s.bytesUp.Load(),
		"bytes_down":                 s.bytesDown.Load(),
		"blocked_ips":                s.fails.snapshotBlocked(),
		"recent_events":              s.recentEventSnapshot(),
	}
}
