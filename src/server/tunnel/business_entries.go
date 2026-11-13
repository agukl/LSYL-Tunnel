package tunnel

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

const maxBusinessEntries = 16

func validateBusinessEntries(cfg Config) error {
	if len(cfg.BusinessEntries) > maxBusinessEntries {
		return fmt.Errorf("business_entries supports at most %d endpoints", maxBusinessEntries)
	}
	listens := make(map[netip.AddrPort]bool)
	advertised := make(map[netip.AddrPort]bool)
	for i, entry := range cfg.BusinessEntries {
		listen, err := parseBusinessEndpoint(entry.ListenAddr, true)
		if err != nil {
			return fmt.Errorf("business_entries[%d].listen_addr: %w", i, err)
		}
		public, err := parseBusinessEndpoint(entry.AdvertiseAddr, false)
		if err != nil {
			return fmt.Errorf("business_entries[%d].advertise_addr: %w", i, err)
		}
		if listen.Addr().Is4() != public.Addr().Is4() {
			return fmt.Errorf("business_entries[%d] listen and advertise address families differ", i)
		}
		if listens[listen] || advertised[public] {
			return fmt.Errorf("business_entries[%d] duplicates an entry endpoint", i)
		}
		listens[listen] = true
		advertised[public] = true
	}
	return nil
}

func parseBusinessEndpoint(value string, listen bool) (netip.AddrPort, error) {
	addr, err := netip.ParseAddrPort(strings.TrimSpace(value))
	if err != nil || addr.Port() == 0 || addr.Addr().Zone() != "" || addr.Addr().Is4In6() {
		return netip.AddrPort{}, fmt.Errorf("must be an IP:port with a nonzero port (IPv6: [address]:port)")
	}
	if addr.Addr().IsMulticast() || (!listen && addr.Addr().IsUnspecified()) {
		return netip.AddrPort{}, fmt.Errorf("must be a usable IP address")
	}
	if addr.Addr().IsLoopback() && !addr.Addr().Is4() {
		return netip.AddrPort{}, fmt.Errorf("loopback must use 127.0.0.1")
	}
	return addr, nil
}

func listenBusinessEntries(entries []BusinessEntryConfig) ([]net.Listener, error) {
	listeners := make([]net.Listener, 0, len(entries))
	for _, entry := range entries {
		addr, _ := parseBusinessEndpoint(entry.ListenAddr, true)
		network := "tcp6"
		if addr.Addr().Is4() {
			network = "tcp4"
		}
		ln, err := net.Listen(network, addr.String())
		if err != nil {
			for _, opened := range listeners {
				_ = opened.Close()
			}
			return nil, fmt.Errorf("listen business entry %s: %w", addr, err)
		}
		listeners = append(listeners, ln)
	}
	return listeners, nil
}

func (s *Server) advertisedBusinessEntries() []string {
	entries := make([]string, 0, len(s.cfg.BusinessEntries))
	for _, entry := range s.cfg.BusinessEntries {
		entries = append(entries, entry.AdvertiseAddr)
	}
	return entries
}
