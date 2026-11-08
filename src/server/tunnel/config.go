package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type RequiresConfig struct {
	MinServerVersion string `yaml:"min_server_version"`
}

type CompatibilityConfig struct {
	MinClientVersion string `yaml:"min_client_version"`
	MaxClientVersion string `yaml:"max_client_version"`
	ProtocolVersion  int    `yaml:"protocol_version"`
}

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
	MaxTrackedConnectionIPs       int `yaml:"max_tracked_connection_ips"`
	ConnectionLimiterCleanupSec   int `yaml:"connection_limiter_cleanup_sec"`
	MaxTrackedFailureIPs          int `yaml:"max_tracked_failure_ips"`
	FailureTrackerCleanupSec      int `yaml:"failure_tracker_cleanup_sec"`
	EntryTrafficLogQueueSize      int `yaml:"entry_traffic_log_queue_size"`
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
	// FlowTrafficLogFile is accepted from older configs; flow events now use BusinessLogFile.
	FlowTrafficLogFile string `yaml:"flow_traffic_log_file,omitempty"`
	RecentEvents       int    `yaml:"recent_events"`
}

type ForwardConfig struct {
	Name         string   `yaml:"name"`
	Direction    string   `yaml:"direction"`
	ListenPort   int      `yaml:"listen_port,omitempty"`
	ListenAddr   string   `yaml:"listen_addr,omitempty"`
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
	ConfigPath    string              `yaml:"-"`
	ConfigVersion int                 `yaml:"config_version"`
	Requires      RequiresConfig      `yaml:"requires"`
	Compatibility CompatibilityConfig `yaml:"compatibility"`
	ListenAddr    string              `yaml:"listen_addr"`
	MonitorAddr   string              `yaml:"monitor_addr"`
	// LogLevel is accepted from older configs but has no runtime effect.
	LogLevel       string               `yaml:"log_level,omitempty"`
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
	cfg.TLS.CertFile = resolveConfigPath(base, cfg.TLS.CertFile)
	cfg.TLS.KeyFile = resolveConfigPath(base, cfg.TLS.KeyFile)
	for i := range cfg.CredentialSeal.Keys {
		cfg.CredentialSeal.Keys[i].PrivateKeyFile = resolveConfigPath(base, cfg.CredentialSeal.Keys[i].PrivateKeyFile)
		cfg.CredentialSeal.Keys[i].PublicKeyFile = resolveConfigPath(base, cfg.CredentialSeal.Keys[i].PublicKeyFile)
	}
	return cfg, ValidateConfig(cfg)
}

func SaveConfig(path string, cfg Config) error {
	if err := validateLoopbackAddresses(cfg); err != nil {
		return err
	}
	for _, fwd := range cfg.Forwards {
		if err := validateReverseListenFields(fwd); err != nil {
			return err
		}
	}
	cfg.Forwards = append([]ForwardConfig(nil), cfg.Forwards...)
	ApplyDefaults(&cfg)
	data, err := yaml.Marshal(configForSave(cfg))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// PrepareFreshInstallConfig replaces machine-local credential sealing metadata
// without generating or packaging private key material.
func PrepareFreshInstallConfig(path string, now time.Time) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	ApplyDefaults(&cfg)
	template := rawCredentialPathTemplate(cfg.CredentialSeal.Keys)
	privateDir := filepath.Dir(strings.TrimSpace(template.PrivateKeyFile))
	publicDir := filepath.Dir(strings.TrimSpace(template.PublicKeyFile))
	if privateDir == "" || privateDir == "." {
		privateDir = "../certs"
	}
	if publicDir == "" || publicDir == "." {
		publicDir = privateDir
	}
	keyID := "login-key-" + now.Format("2006-01")
	cfg.CredentialSeal.Keys = []CredentialSealKeyConfig{{
		KeyID:          keyID,
		PrivateKeyFile: filepath.Join(privateDir, keyID+".key"),
		PublicKeyFile:  filepath.Join(publicDir, keyID+".pub"),
		ExpiresAt:      now.AddDate(0, 3, 0).Format(time.RFC3339),
		Active:         true,
	}}
	if err := ValidateConfig(cfg); err != nil {
		return err
	}
	return SaveConfig(path, cfg)
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
