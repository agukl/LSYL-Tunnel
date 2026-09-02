package tunnel

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func splitHostPort(target string) (string, int, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(target))
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port")
	}
	return strings.Trim(host, "[]"), port, nil
}

func normalizeTarget(target string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(target))
	if err != nil {
		return strings.ToLower(strings.TrimSpace(target))
	}
	return net.JoinHostPort(strings.ToLower(strings.Trim(host, "[]")), port)
}

func isLoopbackName(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isPrivateTargetIP(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsPrivate()
}

func remoteHost(addr net.Addr) string {
	if addr == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

func addrString(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	return addr.String()
}
