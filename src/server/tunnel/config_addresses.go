package tunnel

import (
	"fmt"
	"strings"

	"lsyltunnel/src/internal/netaddr"
)

func validateLoopbackAddresses(cfg Config) error {
	if err := netaddr.CheckLoopbackEndpoint(cfg.ListenAddr); err != nil {
		return fmt.Errorf("listen_addr: %w", err)
	}
	if err := netaddr.CheckLoopbackEndpoint(cfg.MonitorAddr); err != nil {
		return fmt.Errorf("monitor_addr: %w", err)
	}
	for _, fwd := range cfg.Forwards {
		if err := netaddr.CheckLoopbackEndpoint(fwd.ListenAddr); err != nil {
			return fmt.Errorf("forward %q listen_addr: %w", fwd.Name, err)
		}
		if err := netaddr.CheckLoopbackEndpoint(fwd.ServerTarget); err != nil {
			return fmt.Errorf("forward %q server_target: %w", fwd.Name, err)
		}
	}
	return nil
}

func validateReverseListenFields(fwd ForwardConfig) error {
	if strings.TrimSpace(fwd.Direction) != DirectionServerToClient {
		return nil
	}
	if fwd.ListenPort < 0 || fwd.ListenPort > 65535 {
		return fmt.Errorf("forward %q listen_port must be between 1 and 65535", fwd.Name)
	}
	if strings.TrimSpace(fwd.ListenAddr) == "" {
		return nil
	}
	host, port, err := splitHostPort(fwd.ListenAddr)
	if err != nil || host != netaddr.LoopbackIPv4 {
		return fmt.Errorf("forward %q listen_addr must be a loopback address (127.0.0.1:port)", fwd.Name)
	}
	if fwd.ListenPort != 0 && fwd.ListenPort != port {
		return fmt.Errorf("forward %q listen_port conflicts with legacy listen_addr; keep only listen_port or use the same port", fwd.Name)
	}
	return nil
}
