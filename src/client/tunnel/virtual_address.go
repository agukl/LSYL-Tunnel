package tunnel

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"

	"lsyltunnel/src/internal/netaddr"
)

// VirtualForwardAddr resolves a client-side endpoint without changing the config.
func VirtualForwardAddr(cfg Config, fwd ForwardConfig) (string, error) {
	host, port, err := parseVirtualListenAddr(fwd.ListenAddr)
	if err != nil {
		return "", err
	}
	resolver, err := newVirtualAddressResolver(cfg)
	if err != nil {
		return "", err
	}
	return resolver.resolve(host, port)
}

func parseVirtualListenAddr(value string) (string, int, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", 0, fmt.Errorf("virtual listen_addr must use :port or IPv4:port")
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if err := netaddr.CheckLoopbackHost(host); err != nil {
		return "", 0, err
	}
	if host != "" && host != netaddr.LoopbackIPv4 {
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

// Redirect bridges and automatic selection exclude loopback; explicit 127.0.0.1 binds directly.
func normalizeVirtualIPv4(value string) (net.IP, error) {
	virtualIP := net.ParseIP(strings.TrimSpace(value))
	if virtualIP == nil {
		return nil, fmt.Errorf("virtual listen_addr does not support domain names; use :port or IPv4:port")
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

// virtualAddressResolver applies endpoint policy, not TLS server verification.
type virtualAddressResolver struct {
	serverAddr     string
	certificateIPs map[string]struct{}
	candidates     []string
}

func certificateIPv4SANs(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("tls.ca_cert_file is required to authorize virtual forwarding addresses")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read virtual forwarding certificate policy: %w", err)
	}

	seen := make(map[string]struct{})
	certificates := 0
	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse virtual forwarding certificate policy: %w", err)
		}
		certificates++
		for _, value := range cert.IPAddresses {
			if ip := value.To4(); ip != nil {
				seen[ip.String()] = struct{}{}
			}
		}
	}
	if certificates == 0 {
		return nil, fmt.Errorf("no certificate found in tls.ca_cert_file for virtual forwarding")
	}

	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func newVirtualAddressResolver(cfg Config) (*virtualAddressResolver, error) {
	certificateIPs, err := certificateIPv4SANs(cfg.TLS.CACertFile)
	if err != nil {
		return nil, err
	}
	resolver := &virtualAddressResolver{
		serverAddr:     strings.TrimSpace(cfg.ServerAddr),
		certificateIPs: make(map[string]struct{}, len(certificateIPs)),
		candidates:     make([]string, 0, len(certificateIPs)),
	}
	for _, value := range certificateIPs {
		resolver.certificateIPs[value] = struct{}{}
		if _, err := normalizeVirtualIPv4(value); err == nil {
			resolver.candidates = append(resolver.candidates, value)
		}
	}
	return resolver, nil
}

func (r *virtualAddressResolver) resolve(host string, port int) (string, error) {
	if r == nil {
		return "", fmt.Errorf("virtual address resolver is unavailable")
	}
	if host == "" {
		// Private and loopback exemptions do not change certificate-based automatic selection.
		var err error
		host, err = r.automaticIP()
		if err != nil {
			return "", err
		}
	} else if !netaddr.IsForwardTargetHost(host) {
		if _, ok := r.certificateIPs[host]; !ok {
			return "", fmt.Errorf("virtual listen_addr %s is not authorized by the server certificate IPv4 SAN", net.JoinHostPort(host, strconv.Itoa(port)))
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func (r *virtualAddressResolver) automaticIP() (string, error) {
	if len(r.certificateIPs) == 0 {
		return "", fmt.Errorf("server certificate has no IPv4 SAN for virtual forwarding")
	}
	if host, _, err := net.SplitHostPort(r.serverAddr); err == nil {
		if ip, normalizeErr := normalizeVirtualIPv4(strings.Trim(strings.TrimSpace(host), "[]")); normalizeErr == nil {
			value := ip.String()
			if _, ok := r.certificateIPs[value]; ok {
				return value, nil
			}
		}
	}
	switch len(r.candidates) {
	case 0:
		return "", fmt.Errorf("server certificate has no usable non-local IPv4 SAN for automatic virtual forwarding")
	case 1:
		return r.candidates[0], nil
	default:
		return "", fmt.Errorf("server certificate has multiple usable IPv4 SANs; specify virtual listen_addr as IPv4:port")
	}
}
