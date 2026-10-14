package netaddr

import "net"

// IsForwardTargetHost matches the server's literal-IP target policy. It never
// resolves a hostname, so DNS changes cannot expand a configured grant.
func IsForwardTargetHost(host string) bool {
	if host == LoopbackIPv4 {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsPrivate()
}
