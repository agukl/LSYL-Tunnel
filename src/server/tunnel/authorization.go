package tunnel

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"lsyltunnel/src/internal/netaddr"
	"lsyltunnel/src/internal/protocol"
)

func (s *Server) authorize(user UserConfig, target string) error {
	return s.authorizeTarget(user, target)
}

func (s *Server) authorizeOpen(user UserConfig, req protocol.OpenRequest) error {
	if direction := strings.TrimSpace(req.Direction); direction != "" && direction != DirectionClientToServer {
		return fmt.Errorf("request direction is invalid")
	}
	return s.authorizeTarget(user, req.Target)
}

func (s *Server) authorizeForwardCheck(user UserConfig, req protocol.OpenRequest) error {
	direction := strings.TrimSpace(req.Direction)
	if direction == "" {
		direction = DirectionClientToServer
	}
	switch direction {
	case DirectionClientToServer:
		return s.authorizeTarget(user, req.Target)
	case DirectionServerToClient:
		listenAddr := strings.TrimSpace(req.ListenAddr)
		if listenAddr == "" {
			return fmt.Errorf("reverse listen address is required")
		}
		if !s.isConfiguredReverseListener(listenAddr) {
			return fmt.Errorf("reverse listen address is not configured on server")
		}
		return s.authorizeReverse(user, listenAddr)
	default:
		return fmt.Errorf("request direction is invalid")
	}
}

func (s *Server) authorizeReverse(user UserConfig, listenAddr string) error {
	username := strings.TrimSpace(user.Username)
	normalizedListenAddr := normalizeTarget(listenAddr)
	for i := range s.cfg.Forwards {
		fwd := &s.cfg.Forwards[i]
		if strings.TrimSpace(fwd.Direction) != DirectionServerToClient {
			continue
		}
		configuredAddr := configuredReverseListenAddr(*fwd)
		if configuredAddr == "" || normalizeTarget(configuredAddr) != normalizedListenAddr {
			continue
		}
		if allowedUser(fwd.AllowedUsers, username) {
			return nil
		}
		break
	}
	return fmt.Errorf("user is not allowed to activate this reverse listen address")
}

func configuredReverseListenAddr(fwd ForwardConfig) string {
	if addr := strings.TrimSpace(fwd.ListenAddr); addr != "" {
		return addr
	}
	if fwd.ListenPort > 0 && fwd.ListenPort <= 65535 {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(fwd.ListenPort))
	}
	return ""
}

func (s *Server) authorizeTarget(user UserConfig, target string) error {
	host, _, err := splitHostPort(target)
	if err != nil {
		return fmt.Errorf("target address is invalid")
	}
	forward := s.allowedConfiguredForward(user.Username, target)
	if forward == nil {
		return fmt.Errorf("user is not allowed to access this target")
	}
	if netaddr.IsForwardTargetHost(host) {
		return nil
	}
	return fmt.Errorf("target is not allowed")
}

func (s *Server) allowedConfiguredForward(username, target string) *ForwardConfig {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	normalizedTarget := normalizeTarget(target)
	for i := range s.cfg.Forwards {
		fwd := &s.cfg.Forwards[i]
		configuredDirection := strings.TrimSpace(fwd.Direction)
		if configuredDirection == "" {
			configuredDirection = DirectionClientToServer
		}
		if configuredDirection != DirectionClientToServer {
			continue
		}
		if !allowedUser(fwd.AllowedUsers, username) {
			continue
		}
		forwardTarget := strings.TrimSpace(fwd.ServerTarget)
		if forwardTarget != "" && normalizeTarget(forwardTarget) == normalizedTarget {
			return fwd
		}
	}
	return nil
}

func allowedUser(users []string, username string) bool {
	for _, user := range users {
		if strings.TrimSpace(user) == username {
			return true
		}
	}
	return false
}
