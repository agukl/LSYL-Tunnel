package tunnel

import (
	"net"
	"net/netip"
	"sort"
	"strconv"
)

const maxAdvertisedBusinessEntries = 64

func listenIPv6TunnelEntry(main net.Listener, enabled bool, logf func(string, ...any)) (net.Listener, bool) {
	if !enabled {
		return nil, false
	}
	mainAddr := main.Addr().(*net.TCPAddr)
	if mainAddr.IP.IsLoopback() {
		return nil, false
	}
	if mainAddr.IP.To4() == nil {
		return nil, true
	}
	listener, err := net.Listen("tcp6", net.JoinHostPort("::", strconv.Itoa(mainAddr.Port)))
	if err != nil {
		if logf != nil {
			logf("listen IPv6 tunnel on port %d failed: %v", mainAddr.Port, err)
		}
		return nil, false
	}
	return listener, true
}

func globalIPv6Addresses() []netip.Addr {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	seen := make(map[netip.Addr]bool)
	result := make([]netip.Addr, 0)
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, value := range addrs {
			prefix, err := netip.ParsePrefix(value.String())
			if err != nil {
				continue
			}
			addr := prefix.Addr()
			if !advertisableIPv6(addr) || seen[addr] {
				continue
			}
			seen[addr] = true
			result = append(result, addr)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Compare(result[j]) < 0 })
	return result
}

func CurrentGlobalIPv6Addresses() []string {
	addresses := globalIPv6Addresses()
	result := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		result = append(result, addr.String())
	}
	return result
}

func advertisableIPv6(addr netip.Addr) bool {
	return addr.Is6() && addr.IsGlobalUnicast() && !addr.IsPrivate() && !addr.Is4In6() && addr.Zone() == ""
}

func (s *Server) advertisedBusinessEntries() []string {
	return s.advertisedBusinessEntriesForAddresses(globalIPv6Addresses())
}

func (s *Server) advertisedBusinessEntriesForAddresses(addresses []netip.Addr) []string {
	if !s.ipv6TunnelListening {
		return nil
	}
	mainAddr := s.listener.Addr().(*net.TCPAddr)
	result := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if !advertisableIPv6(addr) {
			continue
		}
		if mainAddr.IP.To4() == nil && !mainAddr.IP.IsUnspecified() && !mainAddr.IP.Equal(addr.AsSlice()) {
			continue
		}
		result = append(result, netip.AddrPortFrom(addr, uint16(mainAddr.Port)).String())
		if len(result) >= maxAdvertisedBusinessEntries {
			break
		}
	}
	return result
}
