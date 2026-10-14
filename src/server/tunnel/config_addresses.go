package tunnel

import (
	"fmt"

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
