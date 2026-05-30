package tunnel

import (
	"context"
	"net"
	"strings"
	"time"
)

const configuredForwardProbeTimeout = 800 * time.Millisecond

func (s *Server) prepareConfiguredForwards() error {
	for _, fwd := range s.cfg.Forwards {
		name := forwardDisplayName(fwd)
		switch strings.TrimSpace(fwd.Direction) {
		case DirectionClientToServer:
			target := strings.TrimSpace(fwd.ServerTarget)
			if target == "" {
				s.log("forward target skipped name=%s reason=empty_target", name)
				continue
			}
			if _, _, err := splitHostPort(target); err != nil {
				s.log("forward target skipped name=%s target=%s reason=invalid_target error=%v", name, target, err)
				continue
			}
			s.log("forward target configured name=%s target=%s", name, target)
		case DirectionServerToClient:
			listenAddr := strings.TrimSpace(fwd.ListenAddr)
			if listenAddr == "" {
				s.log("reverse forward skipped name=%s reason=empty_listen", name)
				continue
			}
			host, _, err := splitHostPort(listenAddr)
			if err != nil {
				s.log("reverse forward skipped name=%s listen=%s reason=invalid_listen error=%v", name, listenAddr, err)
				continue
			}
			if !isLoopbackName(host) {
				s.log("reverse forward skipped name=%s listen=%s reason=non_loopback_listen", name, listenAddr)
				continue
			}
			s.markConfiguredReverseListener(listenAddr)
			s.log("reverse forward configured name=%s listen=%s", name, listenAddr)
		default:
			s.log("forward skipped name=%s direction=%s reason=unsupported_direction", name, strings.TrimSpace(fwd.Direction))
		}
	}
	return nil
}

func (s *Server) auditConfiguredForwardAvailability(ctx context.Context) {
	for _, fwd := range s.cfg.Forwards {
		select {
		case <-ctx.Done():
			return
		default:
		}

		name := forwardDisplayName(fwd)
		direction := strings.TrimSpace(fwd.Direction)
		switch direction {
		case DirectionClientToServer:
			s.auditClientToServerForward(ctx, name, fwd)
		case DirectionServerToClient:
			s.auditServerToClientForward(ctx, name, fwd)
		}
	}
}

func (s *Server) auditClientToServerForward(ctx context.Context, name string, fwd ForwardConfig) {
	target := strings.TrimSpace(fwd.ServerTarget)
	if target == "" {
		s.recordEvent(RuntimeEvent{Kind: "forward_check", Result: "failed", ForwardName: name, Direction: DirectionClientToServer, Code: "empty_target", Message: "forward target is empty"})
		return
	}
	if _, _, err := splitHostPort(target); err != nil {
		s.recordEvent(RuntimeEvent{Kind: "forward_check", Result: "failed", ForwardName: name, Direction: DirectionClientToServer, Target: target, Code: "invalid_target", Message: err.Error()})
		return
	}
	dialer := net.Dialer{Timeout: configuredForwardProbeTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		s.recordEvent(RuntimeEvent{Kind: "forward_check", Result: "failed", ForwardName: name, Direction: DirectionClientToServer, Target: target, Code: "target_unreachable", Message: err.Error()})
		return
	}
	_ = conn.Close()
	s.log("forward target available name=%s target=%s", name, target)
}

func (s *Server) auditServerToClientForward(ctx context.Context, name string, fwd ForwardConfig) {
	listenAddr := strings.TrimSpace(fwd.ListenAddr)
	if listenAddr == "" {
		s.recordEvent(RuntimeEvent{Kind: "forward_check", Result: "failed", ForwardName: name, Direction: DirectionServerToClient, Code: "empty_listen", Message: "reverse listen address is empty"})
		return
	}
	host, _, err := splitHostPort(listenAddr)
	if err != nil {
		s.recordEvent(RuntimeEvent{Kind: "forward_check", Result: "failed", ForwardName: name, Direction: DirectionServerToClient, ListenAddr: listenAddr, Code: "invalid_listen", Message: err.Error()})
		return
	}
	if !isLoopbackName(host) {
		s.recordEvent(RuntimeEvent{Kind: "forward_check", Result: "failed", ForwardName: name, Direction: DirectionServerToClient, ListenAddr: listenAddr, Code: "non_loopback_listen", Message: "reverse listen address must be loopback"})
		return
	}
	if s.reverseListener(listenAddr) != nil {
		s.log("reverse passive port already active name=%s listen=%s", name, listenAddr)
		return
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", listenAddr)
	if err != nil {
		if s.reverseListener(listenAddr) != nil {
			s.log("reverse passive port already active name=%s listen=%s", name, listenAddr)
			return
		}
		s.recordEvent(RuntimeEvent{Kind: "forward_check", Result: "failed", ForwardName: name, Direction: DirectionServerToClient, ListenAddr: listenAddr, Code: "passive_port_unavailable", Message: err.Error()})
		return
	}
	_ = ln.Close()
	s.log("reverse passive port available name=%s listen=%s", name, listenAddr)
}

func forwardDisplayName(fwd ForwardConfig) string {
	if name := strings.TrimSpace(fwd.Name); name != "" {
		return name
	}
	if addr := strings.TrimSpace(fwd.ListenAddr); addr != "" {
		return addr
	}
	return strings.TrimSpace(fwd.ServerTarget)
}

func (s *Server) markConfiguredReverseListener(addr string) {
	key := normalizeTarget(addr)
	s.reverseMu.Lock()
	defer s.reverseMu.Unlock()
	if s.configuredReverse == nil {
		s.configuredReverse = map[string]bool{}
	}
	s.configuredReverse[key] = true
}

func (s *Server) isConfiguredReverseListener(addr string) bool {
	key := normalizeTarget(addr)
	s.reverseMu.Lock()
	defer s.reverseMu.Unlock()
	return s.configuredReverse[key]
}
