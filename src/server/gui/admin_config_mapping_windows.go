//go:build windows

package gui

import (
	"fmt"
	"strings"
	"time"

	"lsyltunnel/src/server/tunnel"
)

type adminConfig struct {
	ListenAddr     string          `json:"listen_addr"`
	MonitorAddr    string          `json:"monitor_addr"`
	LogLevel       string          `json:"log_level"`
	TLS            adminTLS        `json:"tls"`
	Users          []adminUser     `json:"users"`
	Forwards       []adminForward  `json:"forwards"`
	Security       adminSecurity   `json:"security"`
	CredentialSeal adminCredential `json:"credential_seal"`
}

type adminTLS struct {
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	MinVersion string `json:"min_version"`
}

type adminUser struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Disabled     bool   `json:"disabled"`
}

type adminSecurity struct {
	HandshakeTimeoutSec           int `json:"handshake_timeout_sec"`
	DialTimeoutSec                int `json:"dial_timeout_sec"`
	MaxHandshakeBytes             int `json:"max_handshake_bytes"`
	MaxConcurrentConnections      int `json:"max_concurrent_connections"`
	MaxConcurrentConnectionsPerIP int `json:"max_concurrent_connections_per_ip"`
	ConnectionRateWindowSec       int `json:"connection_rate_window_sec"`
	MaxNewConnectionsPerIPWindow  int `json:"max_new_connections_per_ip_window"`
	MaxConnectionsPerIPPerWindow  int `json:"max_connections_per_ip_per_window,omitempty"`
	MaxTrackedConnectionIPs       int `json:"max_tracked_connection_ips"`
	ConnectionLimiterCleanupSec   int `json:"connection_limiter_cleanup_sec"`
	MaxTrackedFailureIPs          int `json:"max_tracked_failure_ips"`
	FailureTrackerCleanupSec      int `json:"failure_tracker_cleanup_sec"`
	EntryTrafficLogQueueSize      int `json:"entry_traffic_log_queue_size"`
	MaxConcurrentStreamsPerUser   int `json:"max_concurrent_streams_per_user"`
	StreamRateLimitBytesPerSec    int `json:"stream_rate_limit_bytes_per_sec"`
	AuthFailWindowSec             int `json:"auth_fail_window_sec"`
	AuthFailThreshold             int `json:"auth_fail_threshold"`
	AuthFailBlockSec              int `json:"auth_fail_block_sec"`
}

type adminCredential struct {
	KeyID          string `json:"key_id"`
	PrivateKeyFile string `json:"private_key_file"`
	PublicKeyFile  string `json:"public_key_file"`
	ExpiresAt      string `json:"expires_at"`
	Active         bool   `json:"active"`
}

func adminConfigFromTunnel(cfg tunnel.Config) adminConfig {
	tunnel.ApplyDefaults(&cfg)
	users := make([]adminUser, 0, len(cfg.Auth.Users))
	for _, user := range cfg.Auth.Users {
		users = append(users, adminUser{
			Username:     user.Username,
			PasswordHash: user.PasswordHash,
			Disabled:     user.Disabled,
		})
	}
	forwards := make([]adminForward, 0, len(cfg.Forwards))
	for _, fwd := range cfg.Forwards {
		allowedUsers := cleanAllowedUsers(fwd.AllowedUsers)
		forwards = append(forwards, adminForward{
			Name:         fwd.Name,
			Direction:    forwardDirectionOrDefault(fwd.Direction),
			Port:         forwardPortText(fwd),
			ListenAddr:   fwd.ListenAddr,
			ServerTarget: fwd.ServerTarget,
			Owner:        firstAllowedUser(allowedUsers),
			AllowedUsers: allowedUsers,
		})
	}
	return adminConfig{
		ListenAddr:  cfg.ListenAddr,
		MonitorAddr: cfg.MonitorAddr,
		LogLevel:    cfg.LogLevel,
		TLS: adminTLS{
			CertFile:   cfg.TLS.CertFile,
			KeyFile:    cfg.TLS.KeyFile,
			MinVersion: cfg.TLS.MinVersion,
		},
		Users:    users,
		Forwards: forwards,
		Security: adminSecurity{
			HandshakeTimeoutSec:           cfg.Security.HandshakeTimeoutSec,
			DialTimeoutSec:                cfg.Security.DialTimeoutSec,
			MaxHandshakeBytes:             cfg.Security.MaxHandshakeBytes,
			MaxConcurrentConnections:      cfg.Security.MaxConcurrentConnections,
			MaxConcurrentConnectionsPerIP: cfg.Security.MaxConcurrentConnectionsPerIP,
			ConnectionRateWindowSec:       cfg.Security.ConnectionRateWindowSec,
			MaxNewConnectionsPerIPWindow:  cfg.Security.MaxNewConnectionsPerIPWindow,
			MaxTrackedConnectionIPs:       cfg.Security.MaxTrackedConnectionIPs,
			ConnectionLimiterCleanupSec:   cfg.Security.ConnectionLimiterCleanupSec,
			MaxTrackedFailureIPs:          cfg.Security.MaxTrackedFailureIPs,
			FailureTrackerCleanupSec:      cfg.Security.FailureTrackerCleanupSec,
			EntryTrafficLogQueueSize:      cfg.Security.EntryTrafficLogQueueSize,
			MaxConcurrentStreamsPerUser:   cfg.Security.MaxConcurrentStreamsPerUser,
			StreamRateLimitBytesPerSec:    cfg.Security.StreamRateLimitBytesPerSec,
			AuthFailWindowSec:             cfg.Security.AuthFailWindowSec,
			AuthFailThreshold:             cfg.Security.AuthFailThreshold,
			AuthFailBlockSec:              cfg.Security.AuthFailBlockSec,
		},
		CredentialSeal: activeCredential(cfg.CredentialSeal),
	}
}

func adminConfigToTunnel(form adminConfig, existing tunnel.Config) (tunnel.Config, error) {
	cfg := existing
	cfg.ListenAddr = strings.TrimSpace(form.ListenAddr)
	cfg.MonitorAddr = strings.TrimSpace(form.MonitorAddr)
	cfg.LogLevel = strings.TrimSpace(form.LogLevel)
	cfg.TLS = tunnel.TLSConfig{
		CertFile:   strings.TrimSpace(form.TLS.CertFile),
		KeyFile:    strings.TrimSpace(form.TLS.KeyFile),
		MinVersion: strings.TrimSpace(form.TLS.MinVersion),
	}
	cfg.Auth.Users = make([]tunnel.UserConfig, 0, len(form.Users))
	for _, item := range form.Users {
		username := strings.TrimSpace(item.Username)
		if username == "" && strings.TrimSpace(item.PasswordHash) == "" {
			continue
		}
		cfg.Auth.Users = append(cfg.Auth.Users, tunnel.UserConfig{
			Username:     username,
			PasswordHash: strings.TrimSpace(item.PasswordHash),
			Disabled:     item.Disabled,
		})
	}
	cfg.Forwards = make([]tunnel.ForwardConfig, 0, len(form.Forwards))
	for _, item := range form.Forwards {
		fwd, empty, err := adminForwardToTunnel(item)
		if err != nil {
			return cfg, err
		}
		if empty {
			continue
		}
		fwd.AllowedUsers = cleanAllowedUsers(append(item.AllowedUsers, item.Owner))
		cfg.Forwards = append(cfg.Forwards, fwd)
	}
	cfg.Security = tunnel.SecurityConfig{
		HandshakeTimeoutSec:           form.Security.HandshakeTimeoutSec,
		DialTimeoutSec:                form.Security.DialTimeoutSec,
		MaxHandshakeBytes:             form.Security.MaxHandshakeBytes,
		MaxConcurrentConnections:      form.Security.MaxConcurrentConnections,
		MaxConcurrentConnectionsPerIP: form.Security.MaxConcurrentConnectionsPerIP,
		ConnectionRateWindowSec:       form.Security.ConnectionRateWindowSec,
		MaxNewConnectionsPerIPWindow:  form.Security.MaxNewConnectionsPerIPWindow,
		MaxConnectionsPerIPPerWindow:  form.Security.MaxConnectionsPerIPPerWindow,
		MaxTrackedConnectionIPs:       form.Security.MaxTrackedConnectionIPs,
		ConnectionLimiterCleanupSec:   form.Security.ConnectionLimiterCleanupSec,
		MaxTrackedFailureIPs:          form.Security.MaxTrackedFailureIPs,
		FailureTrackerCleanupSec:      form.Security.FailureTrackerCleanupSec,
		EntryTrafficLogQueueSize:      form.Security.EntryTrafficLogQueueSize,
		MaxConcurrentStreamsPerUser:   form.Security.MaxConcurrentStreamsPerUser,
		StreamRateLimitBytesPerSec:    form.Security.StreamRateLimitBytesPerSec,
		AuthFailWindowSec:             form.Security.AuthFailWindowSec,
		AuthFailThreshold:             form.Security.AuthFailThreshold,
		AuthFailBlockSec:              form.Security.AuthFailBlockSec,
	}
	credentialSeal, err := credentialSealFromAdmin(form.CredentialSeal, existing.CredentialSeal)
	if err != nil {
		return cfg, err
	}
	cfg.CredentialSeal = credentialSeal
	tunnel.ApplyDefaults(&cfg)
	return cfg, nil
}

func credentialSealFromAdmin(form adminCredential, existing tunnel.CredentialSealConfig) (tunnel.CredentialSealConfig, error) {
	keyID := strings.TrimSpace(form.KeyID)
	privateFile := strings.TrimSpace(form.PrivateKeyFile)
	publicFile := strings.TrimSpace(form.PublicKeyFile)
	expiresAt := strings.TrimSpace(form.ExpiresAt)
	if keyID == "" && privateFile == "" && publicFile == "" && expiresAt == "" {
		return tunnel.CredentialSealConfig{}, nil
	}
	if keyID == "" || privateFile == "" || publicFile == "" || expiresAt == "" {
		return tunnel.CredentialSealConfig{}, fmt.Errorf("密封凭据配置不完整，请填写密钥ID、私钥、公钥和过期时间")
	}
	if _, err := time.Parse(time.RFC3339, expiresAt); err != nil {
		return tunnel.CredentialSealConfig{}, fmt.Errorf("密封凭据过期时间格式不正确，请使用 RFC3339")
	}
	keys := make([]tunnel.CredentialSealKeyConfig, 0, len(existing.Keys)+1)
	for _, key := range existing.Keys {
		if strings.TrimSpace(key.KeyID) == keyID {
			continue
		}
		key.Active = false
		keys = append(keys, key)
	}
	keys = append(keys, tunnel.CredentialSealKeyConfig{
		KeyID:          keyID,
		PrivateKeyFile: privateFile,
		PublicKeyFile:  publicFile,
		ExpiresAt:      expiresAt,
		Active:         true,
	})
	return tunnel.CredentialSealConfig{Keys: keys}, nil
}

func activeCredential(cfg tunnel.CredentialSealConfig) adminCredential {
	if len(cfg.Keys) == 0 {
		return adminCredential{}
	}
	selected := cfg.Keys[0]
	for _, item := range cfg.Keys {
		if item.Active {
			selected = item
			break
		}
	}
	return adminCredential{
		KeyID:          selected.KeyID,
		PrivateKeyFile: selected.PrivateKeyFile,
		PublicKeyFile:  selected.PublicKeyFile,
		ExpiresAt:      selected.ExpiresAt,
		Active:         selected.Active,
	}
}

func splitList(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		item := strings.TrimSpace(field)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func cleanAllowedUsers(items []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		for _, user := range splitList(item) {
			if seen[user] {
				continue
			}
			seen[user] = true
			out = append(out, user)
		}
	}
	return out
}

func firstAllowedUser(users []string) string {
	for _, user := range users {
		user = strings.TrimSpace(user)
		if user != "" {
			return user
		}
	}
	return ""
}
