// Package netaddr defines address spelling rules shared by configuration readers.
package netaddr

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

const LoopbackIPv4 = "127.0.0.1"

// CheckLoopbackHost rejects loopback aliases without resolving DNS or changing
// the input. Non-loopback hosts retain the caller's existing validation policy.
func CheckLoopbackHost(host string) error {
	host = strings.TrimSpace(host)
	if host == LoopbackIPv4 {
		return nil
	}
	name := strings.TrimSuffix(strings.ToLower(host), ".")
	ip, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if name == "localhost" || strings.HasSuffix(name, ".localhost") || (err == nil && ip.Unmap().IsLoopback()) {
		return fmt.Errorf("loopback host %q is not supported; use 127.0.0.1", host)
	}
	return nil
}

// CheckLoopbackEndpoint checks host spelling only; callers validate endpoint
// syntax, port ranges and which non-loopback hosts are allowed separately.
func CheckLoopbackEndpoint(addr string) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		host = addr
	}
	return CheckLoopbackHost(host)
}
