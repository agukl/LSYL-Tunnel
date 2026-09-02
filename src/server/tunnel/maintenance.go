package tunnel

import "time"

func (s *Server) failureTrackerCleanupLoop(done <-chan struct{}) {
	if s == nil || s.fails == nil {
		return
	}
	ticker := time.NewTicker(s.fails.cleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := s.fails.cleanup(); err != nil {
				s.log("failure tracker cleanup failed: %v", err)
			}
		}
	}
}

func (s *Server) connectionLimiterCleanupLoop(done <-chan struct{}) {
	if s == nil || s.connLimiter == nil {
		return
	}
	ticker := time.NewTicker(s.connLimiter.cleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s.connLimiter.cleanup()
		}
	}
}

func (s *Server) recordConnectionRejected(reason string) {
	s.connectionsRejected.Add(1)
	switch reason {
	case "global_concurrent_connections":
		s.connectionsRejectedGlobal.Add(1)
	case "per_ip_concurrent_connections":
		s.connectionsRejectedPerIPActive.Add(1)
	case "per_ip_new_connection_rate":
		s.connectionsRejectedPerIPNewRate.Add(1)
	case "tracked_remote_ip_capacity":
		s.connectionsRejectedIPCapacity.Add(1)
	}
}

func (s *Server) connectionRejectLimit(reason string) (int, int) {
	switch reason {
	case "global_concurrent_connections":
		return s.cfg.Security.MaxConcurrentConnections, 0
	case "per_ip_concurrent_connections":
		return s.cfg.Security.MaxConcurrentConnectionsPerIP, 0
	case "per_ip_new_connection_rate":
		return s.cfg.Security.MaxNewConnectionsPerIPWindow, s.cfg.Security.ConnectionRateWindowSec
	case "tracked_remote_ip_capacity":
		return s.cfg.Security.MaxTrackedConnectionIPs, 0
	default:
		return 0, 0
	}
}

func (s *Server) log(format string, args ...any) {
	if s.logf != nil {
		s.logf(format, args...)
	}
}
