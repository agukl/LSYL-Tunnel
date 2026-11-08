package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"lsyltunnel/src/internal/protocol"
	appversion "lsyltunnel/src/internal/version"

	"gopkg.in/yaml.v3"
)

type RequiresConfig struct {
	MinClientVersion string `yaml:"min_client_version"`
}

type TLSConfig struct {
	CACertFile         string `yaml:"ca_cert_file"`
	ServerName         string `yaml:"server_name"`
	MinVersion         string `yaml:"min_version"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type ConnectionConfig struct {
	DialTimeoutSec int `yaml:"dial_timeout_sec"`
}

type Config struct {
	ConfigVersion   int                       `yaml:"config_version"`
	Requires        RequiresConfig            `yaml:"requires"`
	ServerAddr      string                    `yaml:"server_addr"`
	Remark          string                    `yaml:"remark,omitempty"`
	Username        string                    `yaml:"username"`
	Password        string                    `yaml:"password"`
	PasswordEnv     string                    `yaml:"password_env"`
	PasswordFile    string                    `yaml:"password_file"`
	SavedCredential protocol.SealedCredential `yaml:"saved_credential"`
	ClientID        string                    `yaml:"client_id"`
	LogLevel        string                    `yaml:"log_level"`
	TLS             TLSConfig                 `yaml:"tls"`
	Connection      ConnectionConfig          `yaml:"connection"`
	Forwards        []ForwardConfig           `yaml:"forwards"`
}

func LoadConfig(path string) (Config, error) {
	cfg, err := LoadConfigRaw(path)
	if err != nil {
		return cfg, err
	}
	base := filepath.Dir(path)
	cfg.TLS.CACertFile = resolveConfigPath(base, cfg.TLS.CACertFile)
	cfg.PasswordFile = resolveConfigPath(base, cfg.PasswordFile)
	if cfg.Password == "" && strings.TrimSpace(cfg.PasswordEnv) != "" {
		cfg.Password = os.Getenv(strings.TrimSpace(cfg.PasswordEnv))
	}
	if cfg.Password == "" && strings.TrimSpace(cfg.PasswordFile) != "" {
		data, err := os.ReadFile(cfg.PasswordFile)
		if err != nil {
			return cfg, fmt.Errorf("read password_file: %w", err)
		}
		cfg.Password = strings.TrimRight(string(data), "\r\n")
	}
	return cfg, ValidateConfig(cfg)
}

// LoadConfigRaw strictly decodes config without resolving secrets or validating
// runtime-only values. GUI and packaging tools use it before login is available.
func LoadConfigRaw(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	cfg, err = decodeClientConfig(data)
	if err != nil {
		return cfg, err
	}
	ApplyDefaults(&cfg)
	return cfg, validateLoopbackAddresses(cfg)
}

func SaveConfig(path string, cfg Config) error {
	if err := validateLoopbackAddresses(cfg); err != nil {
		return err
	}
	ApplyDefaults(&cfg)
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
	if cfg.ConfigVersion == 0 {
		cfg.ConfigVersion = appversion.ClientConfigVersion
	}
	if strings.TrimSpace(cfg.Requires.MinClientVersion) == "" {
		cfg.Requires.MinClientVersion = appversion.ClientConfigRequiresClientVersion
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		if hostname, err := os.Hostname(); err == nil && strings.TrimSpace(hostname) != "" {
			cfg.ClientID = strings.TrimSpace(hostname)
		}
	}
	if cfg.TLS.MinVersion == "" {
		cfg.TLS.MinVersion = "1.3"
	}
	if cfg.Connection.DialTimeoutSec <= 0 {
		cfg.Connection.DialTimeoutSec = 5
	}
	for i := range cfg.Forwards {
		cfg.Forwards[i].Direction = strings.TrimSpace(cfg.Forwards[i].Direction)
		if cfg.Forwards[i].Direction == "" {
			cfg.Forwards[i].Direction = DirectionClientToServer
		}
	}
}

func resolveConfigPath(base, p string) string {
	p = strings.TrimSpace(p)
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(base, p))
}
