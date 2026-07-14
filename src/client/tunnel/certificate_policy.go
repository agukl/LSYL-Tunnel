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
)

type virtualAddressResolver struct {
	serverAddr string
	allowed    map[string]struct{}
	candidates []string
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
	if len(seen) == 0 {
		return nil, fmt.Errorf("server certificate has no IPv4 SAN for virtual forwarding")
	}

	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func newVirtualAddressResolver(cfg Config) (*virtualAddressResolver, error) {
	allowedValues, err := certificateIPv4SANs(cfg.TLS.CACertFile)
	if err != nil {
		return nil, err
	}
	resolver := &virtualAddressResolver{
		serverAddr: strings.TrimSpace(cfg.ServerAddr),
		allowed:    make(map[string]struct{}, len(allowedValues)),
		candidates: make([]string, 0, len(allowedValues)),
	}
	for _, value := range allowedValues {
		resolver.allowed[value] = struct{}{}
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
		var err error
		host, err = r.automaticIP()
		if err != nil {
			return "", err
		}
	} else if _, ok := r.allowed[host]; !ok {
		return "", fmt.Errorf("virtual listen_addr %s is not authorized by the server certificate IPv4 SAN", net.JoinHostPort(host, strconv.Itoa(port)))
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func (r *virtualAddressResolver) automaticIP() (string, error) {
	if host, _, err := net.SplitHostPort(r.serverAddr); err == nil {
		if ip, normalizeErr := normalizeVirtualIPv4(strings.Trim(strings.TrimSpace(host), "[]")); normalizeErr == nil {
			value := ip.String()
			if _, ok := r.allowed[value]; ok {
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
