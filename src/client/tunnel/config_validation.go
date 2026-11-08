package tunnel

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"lsyltunnel/src/internal/netaddr"
	appversion "lsyltunnel/src/internal/version"
)

func ValidateConfig(cfg Config) error {
	if err := ValidateConfigVersion(cfg); err != nil {
		return err
	}
	if err := validateLoopbackAddresses(cfg); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.ServerAddr) == "" {
		return fmt.Errorf("server_addr is required")
	}
	if strings.TrimSpace(cfg.Username) == "" || (cfg.Password == "" && strings.TrimSpace(cfg.SavedCredential.Ciphertext) == "") {
		return fmt.Errorf("username and password are required")
	}
	if len(cfg.Forwards) == 0 {
		return fmt.Errorf("at least one forward is required")
	}
	if err := validateReverseForwardFields(cfg.Forwards); err != nil {
		return err
	}
	serverPort := 0
	if _, portText, err := net.SplitHostPort(strings.TrimSpace(cfg.ServerAddr)); err == nil {
		serverPort, _ = strconv.Atoi(portText)
	}
	seenVirtualEndpoints := map[string]string{}
	seenNames := map[string]bool{}
	virtualEndpoints := make([]string, 0)
	var virtualResolver *virtualAddressResolver
	for _, fwd := range cfg.Forwards {
		direction := strings.TrimSpace(fwd.Direction)
		name := forwardName(fwd)
		if seenNames[name] {
			return fmt.Errorf("forward %q has duplicate runtime name; use unique names within this client", name)
		}
		seenNames[name] = true
		switch direction {
		case DirectionClientToServer, DirectionServerToClient, DirectionVirtual:
		default:
			return fmt.Errorf("forward %q has unsupported direction", fwd.Name)
		}
		if strings.TrimSpace(fwd.ServerTarget) == "" {
			return fmt.Errorf("forward %q requires server_target", fwd.Name)
		}
		targetHost, targetPort, err := net.SplitHostPort(strings.TrimSpace(fwd.ServerTarget))
		if err != nil || strings.Trim(strings.TrimSpace(targetHost), "[]") == "" {
			return fmt.Errorf("forward %q server_target must include a host and port", fwd.Name)
		}
		if port, err := strconv.Atoi(targetPort); err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("forward %q server_target port must be between 1 and 65535", fwd.Name)
		}
		if direction == DirectionServerToClient {
			if targetHost != netaddr.LoopbackIPv4 {
				return fmt.Errorf("forward %q reverse server_target must use a loopback address (127.0.0.1)", fwd.Name)
			}
			continue
		}
		if !netaddr.IsForwardTargetHost(targetHost) {
			return fmt.Errorf("forward %q server_target must use 127.0.0.1 or a private IP address", fwd.Name)
		}
		if direction == DirectionVirtual {
			virtualHost, virtualPort, err := parseVirtualListenAddr(fwd.ListenAddr)
			if err != nil {
				return fmt.Errorf("forward %q: %w", fwd.Name, err)
			}
			if virtualResolver == nil {
				virtualResolver, err = newVirtualAddressResolver(cfg)
				if err != nil {
					return fmt.Errorf("forward %q: %w", fwd.Name, err)
				}
			}
			virtualAddr, err := virtualResolver.resolve(virtualHost, virtualPort)
			if err != nil {
				return fmt.Errorf("forward %q: %w", fwd.Name, err)
			}
			if serverPort <= 0 || serverPort > 65535 {
				return fmt.Errorf("server_addr must include a valid port before virtual forwarding is configured")
			}
			if virtualPort == serverPort {
				return fmt.Errorf("forward %q virtual listen_addr cannot use the server_addr port", fwd.Name)
			}
			key := strings.ToLower(virtualAddr)
			if existing, ok := seenVirtualEndpoints[key]; ok {
				return fmt.Errorf("forward %q virtual endpoint %s duplicates forward %q", fwd.Name, virtualAddr, existing)
			}
			seenVirtualEndpoints[key] = fwd.Name
			virtualEndpoints = append(virtualEndpoints, virtualAddr)
			continue
		}
		if strings.TrimSpace(fwd.ListenAddr) == "" {
			return fmt.Errorf("forward %q requires listen_addr and server_target", fwd.Name)
		}
		_, portText, err := net.SplitHostPort(strings.TrimSpace(fwd.ListenAddr))
		if err != nil {
			return fmt.Errorf("forward %q listen_addr must include a host and port", fwd.Name)
		}
		if port, err := strconv.Atoi(portText); err != nil || port < 0 || port > 65535 {
			return fmt.Errorf("forward %q listen_addr port must be between 0 and 65535", fwd.Name)
		}
	}
	if len(virtualEndpoints) > maxVirtualRedirectEndpoints {
		return fmt.Errorf("virtual forwarding supports at most %d endpoints", maxVirtualRedirectEndpoints)
	}
	if strings.TrimSpace(cfg.TLS.CACertFile) == "" {
		return fmt.Errorf("tls.ca_cert_file is required for certificate possession gate and server verification")
	}
	return nil
}

func validateLoopbackAddresses(cfg Config) error {
	if err := netaddr.CheckLoopbackEndpoint(cfg.ServerAddr); err != nil {
		return fmt.Errorf("server_addr: %w", err)
	}
	if err := netaddr.CheckLoopbackHost(cfg.TLS.ServerName); err != nil {
		return fmt.Errorf("tls.server_name: %w", err)
	}
	for _, fwd := range cfg.Forwards {
		if strings.TrimSpace(fwd.Direction) != DirectionVirtual {
			if err := netaddr.CheckLoopbackEndpoint(fwd.ListenAddr); err != nil {
				return fmt.Errorf("forward %q listen_addr: %w", fwd.Name, err)
			}
		}
		if err := netaddr.CheckLoopbackEndpoint(fwd.ServerTarget); err != nil {
			return fmt.Errorf("forward %q server_target: %w", fwd.Name, err)
		}
	}
	return nil
}

func ValidateConfigVersion(cfg Config) error {
	if cfg.ConfigVersion < 0 {
		return fmt.Errorf("client config_version must be >= 0")
	}
	if cfg.ConfigVersion > appversion.ClientConfigVersion {
		return fmt.Errorf("client config_version %d requires a newer client; current client supports config_version %d", cfg.ConfigVersion, appversion.ClientConfigVersion)
	}
	if cfg.ConfigVersion >= 2 {
		minimum := strings.TrimSpace(cfg.Requires.MinClientVersion)
		if minimum == "" {
			return fmt.Errorf("client config_version %d requires requires.min_client_version", cfg.ConfigVersion)
		}
		less, err := appversion.Less(minimum, appversion.ClientConfigRequiresClientVersion)
		if err != nil {
			return fmt.Errorf("client config requires invalid minimum version %q: %w", minimum, err)
		}
		if less {
			return fmt.Errorf("client config_version %d requires min_client_version >= %s", cfg.ConfigVersion, appversion.ClientConfigRequiresClientVersion)
		}
	}
	if err := appversion.CheckMin(appversion.AppVersion, cfg.Requires.MinClientVersion, "client config"); err != nil {
		return err
	}
	return nil
}
