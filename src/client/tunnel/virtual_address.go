package tunnel

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"lsyltunnel/src/internal/netaddr"
)

// VirtualForwardAddr validates a client-side endpoint without changing the config.
func VirtualForwardAddr(fwd ForwardConfig) (string, error) {
	host, port, err := parseVirtualListenAddr(fwd.ListenAddr)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func parseVirtualListenAddr(value string) (string, int, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", 0, fmt.Errorf("virtual listen_addr must use IPv4:port")
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return "", 0, fmt.Errorf("virtual listen_addr requires an explicit IPv4 address; use IPv4:port")
	}
	if err := netaddr.CheckLoopbackHost(host); err != nil {
		return "", 0, err
	}
	if host != netaddr.LoopbackIPv4 {
		virtualIP, err := normalizeVirtualIPv4(host)
		if err != nil {
			return "", 0, err
		}
		host = virtualIP.String()
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", 0, fmt.Errorf("virtual listen_addr port must be between 1 and 65535")
	}
	return host, portNumber, nil
}

// Redirect bridges exclude loopback; explicit 127.0.0.1 binds directly.
func normalizeVirtualIPv4(value string) (net.IP, error) {
	virtualIP := net.ParseIP(strings.TrimSpace(value))
	if virtualIP == nil {
		return nil, fmt.Errorf("virtual listen_addr does not support domain names; use IPv4:port")
	}
	if virtualIP.To4() == nil || strings.Contains(value, ":") {
		return nil, fmt.Errorf("virtual listen_addr host must be an IPv4 address")
	}
	virtualIP = virtualIP.To4()
	if virtualIP[0] == 0 || virtualIP[0] >= 224 || virtualIP.IsLoopback() || virtualIP.IsLinkLocalUnicast() {
		return nil, fmt.Errorf("virtual listen_addr host must be a usable non-local IPv4 address")
	}
	return virtualIP, nil
}
