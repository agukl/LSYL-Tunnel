package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"lsyltunnel/src/internal/protocol"

	"gopkg.in/yaml.v3"
)

type TLSConfig struct {
	CACertFile         string `yaml:"ca_cert_file"`
	ServerName         string `yaml:"server_name"`
	MinVersion         string `yaml:"min_version"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type ConnectionConfig struct {
	DialTimeoutSec int `yaml:"dial_timeout_sec"`
}

type ForwardConfig struct {
	Name         string `yaml:"name"`
	Direction    string `yaml:"direction"`
	ListenAddr   string `yaml:"listen_addr"`
	ServerTarget string `yaml:"server_target"`
}

const (
	DirectionClientToServer = "client_to_server"
	DirectionServerToClient = "server_to_client"
)

type Config struct {
	ServerAddr      string                    `yaml:"server_addr"`
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
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	base := filepath.Dir(path)
	ApplyDefaults(&cfg)
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
		if cfg.Forwards[i].Direction == "" {
			cfg.Forwards[i].Direction = DirectionClientToServer
		}
	}
}

func ValidateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.ServerAddr) == "" {
		return fmt.Errorf("server_addr is required")
	}
	if strings.TrimSpace(cfg.Username) == "" || (cfg.Password == "" && strings.TrimSpace(cfg.SavedCredential.Ciphertext) == "") {
		return fmt.Errorf("username and password are required")
	}
	if !cfg.TLS.InsecureSkipVerify && strings.TrimSpace(cfg.TLS.CACertFile) == "" {
		return fmt.Errorf("tls.ca_cert_file is required for server verification unless insecure_skip_verify is true")
	}
	if len(cfg.Forwards) == 0 {
		return fmt.Errorf("at least one forward is required")
	}
	for _, fwd := range cfg.Forwards {
		switch strings.TrimSpace(fwd.Direction) {
		case DirectionClientToServer, DirectionServerToClient:
		default:
			return fmt.Errorf("forward %q has unsupported direction", fwd.Name)
		}
		if strings.TrimSpace(fwd.ListenAddr) == "" || strings.TrimSpace(fwd.ServerTarget) == "" {
			return fmt.Errorf("forward %q requires listen_addr and server_target", fwd.Name)
		}
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
