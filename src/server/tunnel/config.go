package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type TLSConfig struct {
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
	MinVersion string `yaml:"min_version"`
}

type SecurityConfig struct {
	HandshakeTimeoutSec           int `yaml:"handshake_timeout_sec"`
	DialTimeoutSec                int `yaml:"dial_timeout_sec"`
	MaxHandshakeBytes             int `yaml:"max_handshake_bytes"`
	MaxConcurrentConnections      int `yaml:"max_concurrent_connections"`
	MaxConcurrentConnectionsPerIP int `yaml:"max_concurrent_connections_per_ip"`
	ConnectionRateWindowSec       int `yaml:"connection_rate_window_sec"`
	MaxNewConnectionsPerIPWindow  int `yaml:"max_new_connections_per_ip_window"`
	MaxConnectionsPerIPPerWindow  int `yaml:"max_connections_per_ip_per_window,omitempty"`
	MaxConcurrentStreamsPerUser   int `yaml:"max_concurrent_streams_per_user"`
	StreamRateLimitBytesPerSec    int `yaml:"stream_rate_limit_bytes_per_sec"`
	AuthFailWindowSec             int `yaml:"auth_fail_window_sec"`
	AuthFailThreshold             int `yaml:"auth_fail_threshold"`
	AuthFailBlockSec              int `yaml:"auth_fail_block_sec"`
}

type CredentialSealKeyConfig struct {
	KeyID          string `yaml:"key_id"`
	PrivateKeyFile string `yaml:"private_key_file"`
	PublicKeyFile  string `yaml:"public_key_file"`
	ExpiresAt      string `yaml:"expires_at"`
	Active         bool   `yaml:"active"`
}

type CredentialSealConfig struct {
	Keys []CredentialSealKeyConfig `yaml:"keys"`
}

type RuntimeConfig struct {
	StateFile           string `yaml:"state_file"`
	PermanentBlockFile  string `yaml:"permanent_block_file"`
	RequestLogFile      string `yaml:"request_log_file"`
	BusinessLogFile     string `yaml:"business_log_file"`
	EntryTrafficLogFile string `yaml:"entry_traffic_log_file"`
	FlowTrafficLogFile  string `yaml:"flow_traffic_log_file"`
	RecentEvents        int    `yaml:"recent_events"`
}

type ForwardConfig struct {
	Name         string   `yaml:"name"`
	Direction    string   `yaml:"direction"`
	ListenAddr   string   `yaml:"listen_addr"`
	ServerTarget string   `yaml:"server_target"`
	AllowedUsers []string `yaml:"allowed_users,omitempty"`
}

const (
	DirectionClientToServer = "client_to_server"
	DirectionServerToClient = "server_to_client"
	DefaultListenAddr       = "0.0.0.0:3443"
)

type UserConfig struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
	Disabled     bool   `yaml:"disabled"`
}

type AuthConfig struct {
	Users []UserConfig `yaml:"users"`
}

type Config struct {
	ConfigPath     string               `yaml:"-"`
	ListenAddr     string               `yaml:"listen_addr"`
	MonitorAddr    string               `yaml:"monitor_addr"`
	LogLevel       string               `yaml:"log_level"`
	TLS            TLSConfig            `yaml:"tls"`
	Auth           AuthConfig           `yaml:"auth"`
	Forwards       []ForwardConfig      `yaml:"forwards"`
	Security       SecurityConfig       `yaml:"security"`
	CredentialSeal CredentialSealConfig `yaml:"credential_seal"`
	Runtime        RuntimeConfig        `yaml:"runtime"`
}

func LoadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if abs, err := filepath.Abs(path); err == nil {
		cfg.ConfigPath = abs
	} else {
		cfg.ConfigPath = path
	}
	base := filepath.Dir(path)
	ApplyDefaults(&cfg)
	if strings.TrimSpace(cfg.Runtime.StateFile) == "" {
		cfg.Runtime.StateFile = defaultRuntimePath(base, "data", "server-state.json")
	} else {
		cfg.Runtime.StateFile = resolveConfigPath(base, cfg.Runtime.StateFile)
	}
	if strings.TrimSpace(cfg.Runtime.PermanentBlockFile) == "" {
		cfg.Runtime.PermanentBlockFile = defaultRuntimePath(base, "data", "server-permanent-block.txt")
	} else {
		cfg.Runtime.PermanentBlockFile = resolveConfigPath(base, cfg.Runtime.PermanentBlockFile)
	}
	if strings.TrimSpace(cfg.Runtime.RequestLogFile) == "" {
		cfg.Runtime.RequestLogFile = defaultRuntimePath(base, "logs", filepath.Join("request", "request.jsonl"))
	} else {
		cfg.Runtime.RequestLogFile = resolveConfigPath(base, cfg.Runtime.RequestLogFile)
	}
	if strings.TrimSpace(cfg.Runtime.BusinessLogFile) == "" {
		cfg.Runtime.BusinessLogFile = defaultRuntimePath(base, "logs", filepath.Join("business", "business.jsonl"))
	} else {
		cfg.Runtime.BusinessLogFile = resolveConfigPath(base, cfg.Runtime.BusinessLogFile)
	}
	if strings.TrimSpace(cfg.Runtime.EntryTrafficLogFile) == "" {
		cfg.Runtime.EntryTrafficLogFile = defaultRuntimePath(base, "logs", filepath.Join("entry-traffic", "entry-traffic.jsonl"))
	} else {
		cfg.Runtime.EntryTrafficLogFile = resolveConfigPath(base, cfg.Runtime.EntryTrafficLogFile)
	}
	if strings.TrimSpace(cfg.Runtime.FlowTrafficLogFile) == "" {
		cfg.Runtime.FlowTrafficLogFile = defaultRuntimePath(base, "logs", filepath.Join("flow-traffic", "flow-traffic.jsonl"))
	} else {
		cfg.Runtime.FlowTrafficLogFile = resolveConfigPath(base, cfg.Runtime.FlowTrafficLogFile)
	}
	cfg.TLS.CertFile = resolveConfigPath(base, cfg.TLS.CertFile)
	cfg.TLS.KeyFile = resolveConfigPath(base, cfg.TLS.KeyFile)
	for i := range cfg.CredentialSeal.Keys {
		cfg.CredentialSeal.Keys[i].PrivateKeyFile = resolveConfigPath(base, cfg.CredentialSeal.Keys[i].PrivateKeyFile)
		cfg.CredentialSeal.Keys[i].PublicKeyFile = resolveConfigPath(base, cfg.CredentialSeal.Keys[i].PublicKeyFile)
	}
	return cfg, ValidateConfig(cfg)
}

func SaveConfig(path string, cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ApplyDefaults(cfg *Config) {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = DefaultListenAddr
	}
	if cfg.TLS.MinVersion == "" {
		cfg.TLS.MinVersion = "1.3"
	}
	for i := range cfg.Forwards {
		if cfg.Forwards[i].Direction == "" {
			cfg.Forwards[i].Direction = DirectionClientToServer
		}
	}
	if cfg.Security.HandshakeTimeoutSec <= 0 {
		cfg.Security.HandshakeTimeoutSec = 8
	}
	if cfg.Security.DialTimeoutSec <= 0 {
		cfg.Security.DialTimeoutSec = 5
	}
	if cfg.Security.MaxHandshakeBytes <= 0 {
		cfg.Security.MaxHandshakeBytes = 32768
	}
	if cfg.Security.MaxConcurrentConnections <= 0 {
		cfg.Security.MaxConcurrentConnections = 2048
	}
	if cfg.Security.MaxConcurrentConnectionsPerIP <= 0 {
		cfg.Security.MaxConcurrentConnectionsPerIP = 128
	}
	if cfg.Security.ConnectionRateWindowSec <= 0 {
		cfg.Security.ConnectionRateWindowSec = 1
	}
	if cfg.Security.MaxNewConnectionsPerIPWindow <= 0 && cfg.Security.MaxConnectionsPerIPPerWindow > 0 {
		cfg.Security.MaxNewConnectionsPerIPWindow = cfg.Security.MaxConnectionsPerIPPerWindow
	}
	if cfg.Security.MaxNewConnectionsPerIPWindow <= 0 {
		cfg.Security.MaxNewConnectionsPerIPWindow = 120
	}
	cfg.Security.MaxConnectionsPerIPPerWindow = 0
	if cfg.Security.AuthFailWindowSec <= 0 {
		cfg.Security.AuthFailWindowSec = 300
	}
	if cfg.Security.AuthFailThreshold <= 0 {
		cfg.Security.AuthFailThreshold = 8
	}
	if cfg.Security.AuthFailBlockSec <= 0 {
		cfg.Security.AuthFailBlockSec = 1800
	}
	if cfg.Runtime.RecentEvents <= 0 {
		cfg.Runtime.RecentEvents = 500
	}
}

func ValidateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.TLS.CertFile) == "" || strings.TrimSpace(cfg.TLS.KeyFile) == "" {
		return fmt.Errorf("server TLS identity cert_file and key_file are required")
	}
	seen := map[string]bool{}
	for _, user := range cfg.Auth.Users {
		name := strings.TrimSpace(user.Username)
		if name == "" {
			return fmt.Errorf("auth user username is required")
		}
		if seen[name] {
			return fmt.Errorf("duplicate auth user: %s", name)
		}
		seen[name] = true
		if !user.Disabled && strings.TrimSpace(user.PasswordHash) == "" {
			return fmt.Errorf("password_hash is required for user %s", name)
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

func resolveConfigPath(base, p string) string {
	p = strings.TrimSpace(p)
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(base, p))
}

func defaultRuntimePath(configDir, category, name string) string {
	configDir = filepath.Clean(configDir)
	if isSourceServerConfigDir(configDir) {
		root := filepath.Dir(filepath.Dir(filepath.Dir(configDir)))
		return filepath.Join(root, "runtime", category, name)
	}
	return filepath.Join(configDir, "..", category, name)
}

func isSourceServerConfigDir(configDir string) bool {
	configDir = filepath.Clean(configDir)
	return strings.EqualFold(filepath.Base(configDir), "conf") &&
		strings.EqualFold(filepath.Base(filepath.Dir(configDir)), "server") &&
		strings.EqualFold(filepath.Base(filepath.Dir(filepath.Dir(configDir))), "src")
}
