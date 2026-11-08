package tunnel

import (
	"fmt"
	"strings"
	"time"

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
	if strings.TrimSpace(cfg.TLS.CertFile) == "" || strings.TrimSpace(cfg.TLS.KeyFile) == "" {
		return fmt.Errorf("server TLS identity cert_file and key_file are required")
	}
	if monitorAddr := strings.TrimSpace(cfg.MonitorAddr); monitorAddr != "" {
		host, _, err := splitHostPort(monitorAddr)
		if err != nil || host != "127.0.0.1" {
			return fmt.Errorf("monitor_addr must use a loopback address (127.0.0.1)")
		}
	}
	knownUsers := map[string]bool{}
	for _, user := range cfg.Auth.Users {
		name := strings.TrimSpace(user.Username)
		if name == "" {
			return fmt.Errorf("auth user username is required")
		}
		if knownUsers[name] {
			return fmt.Errorf("duplicate auth user: %s", name)
		}
		knownUsers[name] = true
		if !user.Disabled && strings.TrimSpace(user.PasswordHash) == "" {
			return fmt.Errorf("password_hash is required for user %s", name)
		}
	}
	seenReversePorts := map[int]string{}
	for _, forward := range cfg.Forwards {
		direction := strings.TrimSpace(forward.Direction)
		if direction == "" {
			direction = DirectionClientToServer
		}
		reversePort := 0
		switch direction {
		case DirectionClientToServer:
			host, _, err := splitHostPort(forward.ServerTarget)
			if err != nil {
				return fmt.Errorf("forward %q server_target is invalid", forward.Name)
			}
			if !netaddr.IsForwardTargetHost(host) {
				return fmt.Errorf("forward %q server_target must be a loopback or private IP (127.0.0.1 or private IP)", forward.Name)
			}
		case DirectionServerToClient:
			if err := validateReverseListenFields(forward); err != nil {
				return err
			}
			port := forward.ListenPort
			if port <= 0 {
				host, legacyPort, err := splitHostPort(forward.ListenAddr)
				if err != nil || host != "127.0.0.1" {
					return fmt.Errorf("forward %q listen_port is required and legacy listen_addr must be a loopback address (127.0.0.1)", forward.Name)
				}
				port = legacyPort
			}
			if port < 1 || port > 65535 {
				return fmt.Errorf("forward %q listen_port must be between 1 and 65535", forward.Name)
			}
			reversePort = port
			if strings.TrimSpace(forward.ListenAddr) != "" {
				host, _, err := splitHostPort(forward.ListenAddr)
				if err != nil || host != "127.0.0.1" {
					return fmt.Errorf("forward %q listen_addr must be a loopback address (127.0.0.1)", forward.Name)
				}
			}
		default:
			return fmt.Errorf("forward %q has unsupported direction", forward.Name)
		}

		allowedUsers := map[string]bool{}
		for _, username := range forward.AllowedUsers {
			username = strings.TrimSpace(username)
			if username == "" {
				return fmt.Errorf("forward %q allowed_users contains an empty username", forward.Name)
			}
			if allowedUsers[username] {
				return fmt.Errorf("forward %q has duplicate allowed user %s", forward.Name, username)
			}
			if !knownUsers[username] {
				return fmt.Errorf("forward %q allowed user %s does not exist", forward.Name, username)
			}
			allowedUsers[username] = true
		}
		if len(allowedUsers) == 0 {
			return fmt.Errorf("forward %q requires at least one allowed user", forward.Name)
		}
		if direction == DirectionServerToClient {
			if len(allowedUsers) != 1 {
				return fmt.Errorf("forward %q reverse port must belong to exactly one allowed user", forward.Name)
			}
			if existing, ok := seenReversePorts[reversePort]; ok {
				return fmt.Errorf("forward %q reverse listen_port %d duplicates forward %q", forward.Name, reversePort, existing)
			}
			seenReversePorts[reversePort] = forward.Name
		}
	}
	activeKeys := 0
	keyIDs := map[string]bool{}
	for _, key := range cfg.CredentialSeal.Keys {
		id := strings.TrimSpace(key.KeyID)
		if id == "" {
			return fmt.Errorf("credential seal key_id is required")
		}
		if keyIDs[id] {
			return fmt.Errorf("duplicate credential seal key: %s", id)
		}
		keyIDs[id] = true
		if strings.TrimSpace(key.PrivateKeyFile) == "" || strings.TrimSpace(key.PublicKeyFile) == "" {
			return fmt.Errorf("credential seal key %s requires private_key_file and public_key_file", id)
		}
		if strings.TrimSpace(key.ExpiresAt) == "" {
			return fmt.Errorf("credential seal key %s requires expires_at", id)
		}
		if _, err := time.Parse(time.RFC3339, strings.TrimSpace(key.ExpiresAt)); err != nil {
			return fmt.Errorf("credential seal key %s expires_at must be RFC3339", id)
		}
		if key.Active {
			activeKeys++
		}
	}
	if activeKeys > 1 {
		return fmt.Errorf("only one credential seal key can be active")
	}
	return nil
}

func ValidateConfigVersion(cfg Config) error {
	if cfg.ConfigVersion < 0 {
		return fmt.Errorf("server config_version must be >= 0")
	}
	if cfg.ConfigVersion > appversion.ServerConfigVersion {
		return fmt.Errorf("server config_version %d requires a newer server; current server supports config_version %d", cfg.ConfigVersion, appversion.ServerConfigVersion)
	}
	if err := appversion.CheckMin(appversion.AppVersion, cfg.Requires.MinServerVersion, "server config"); err != nil {
		return err
	}
	protocolVersion := cfg.Compatibility.ProtocolVersion
	if protocolVersion == 0 {
		protocolVersion = appversion.ProtocolVersion
	}
	if protocolVersion < 0 {
		return fmt.Errorf("server compatibility.protocol_version must be >= 0")
	}
	if protocolVersion != appversion.ProtocolVersion {
		return fmt.Errorf("server compatibility.protocol_version %d is not supported by this server; current protocol_version is %d", protocolVersion, appversion.ProtocolVersion)
	}
	if strings.TrimSpace(cfg.Compatibility.MinClientVersion) != "" {
		if err := appversion.Validate(cfg.Compatibility.MinClientVersion); err != nil {
			return fmt.Errorf("server compatibility.min_client_version is invalid: %w", err)
		}
	}
	if strings.TrimSpace(cfg.Compatibility.MaxClientVersion) != "" {
		if err := appversion.Validate(cfg.Compatibility.MaxClientVersion); err != nil {
			return fmt.Errorf("server compatibility.max_client_version is invalid: %w", err)
		}
	}
	if strings.TrimSpace(cfg.Compatibility.MinClientVersion) != "" && strings.TrimSpace(cfg.Compatibility.MaxClientVersion) != "" {
		greater, err := appversion.Greater(cfg.Compatibility.MinClientVersion, cfg.Compatibility.MaxClientVersion)
		if err != nil {
			return fmt.Errorf("server compatibility client version range is invalid: %w", err)
		}
		if greater {
			return fmt.Errorf("server compatibility min_client_version must be <= max_client_version")
		}
	}
	return nil
}
