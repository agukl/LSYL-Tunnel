package tunnel

import (
	"fmt"
	"strings"
	"time"

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
	if issue := ValidateForwardRules(cfg.Forwards, knownUsers); issue != nil {
		return issue
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
	if cfg.ConfigVersion > 0 && cfg.ConfigVersion < appversion.ServerConfigVersion {
		return fmt.Errorf("server config_version %d is no longer supported", cfg.ConfigVersion)
	}
	if cfg.ConfigVersion > appversion.ServerConfigVersion {
		return fmt.Errorf("server config_version %d requires a newer server; current server supports config_version %d", cfg.ConfigVersion, appversion.ServerConfigVersion)
	}
	if err := appversion.CheckMin(appversion.AppVersion, cfg.Requires.MinServerVersion, "server config"); err != nil {
		return err
	}
	return nil
}
