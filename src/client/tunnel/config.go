package tunnel

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
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

type ForwardConfig struct {
	Name         string `yaml:"name"`
	Direction    string `yaml:"direction"`
	ListenAddr   string `yaml:"listen_addr,omitempty"`
	ServerTarget string `yaml:"server_target"`
}

func (f *ForwardConfig) UnmarshalYAML(node *yaml.Node) error {
	type rawForwardConfig ForwardConfig
	var raw rawForwardConfig
	if err := node.Decode(&raw); err != nil {
		return err
	}
	direction := strings.TrimSpace(raw.Direction)
	if direction == DirectionServerToClient {
		name := strings.TrimSpace(raw.Name)
		if name == "" {
			name = "<unnamed>"
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "listen_port" {
				return fmt.Errorf("forward %q server_to_client client config must not include listen_port; use listen_addr and server_target", name)
			}
		}
	}
	*f = ForwardConfig(raw)
	return nil
}

const (
	DirectionClientToServer = "client_to_server"
	DirectionServerToClient = "server_to_client"
	DirectionVirtual        = "virtual"
)

type Config struct {
	ConfigVersion   int                       `yaml:"config_version"`
	Requires        RequiresConfig            `yaml:"requires"`
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

func ValidateConfig(cfg Config) error {
	if err := ValidateConfigVersion(cfg); err != nil {
		return err
	}
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
	if err := validateReverseForwardFields(cfg.Forwards); err != nil {
		return err
	}
	serverPort := 0
	if _, portText, err := net.SplitHostPort(strings.TrimSpace(cfg.ServerAddr)); err == nil {
		serverPort, _ = strconv.Atoi(portText)
	}
	seenVirtualEndpoints := map[string]string{}
	virtualEndpoints := make([]string, 0)
	var virtualResolver *virtualAddressResolver
	for _, fwd := range cfg.Forwards {
		direction := strings.TrimSpace(fwd.Direction)
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
			if !isLoopbackName(targetHost) {
				return fmt.Errorf("forward %q reverse server_target must use a loopback address", fwd.Name)
			}
			continue
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
	}
	if len(virtualEndpoints) > maxVirtualRedirectEndpoints {
		return fmt.Errorf("virtual forwarding supports at most %d endpoints", maxVirtualRedirectEndpoints)
	}
	return nil
}

func validateReverseForwardFields(forwards []ForwardConfig) error {
	for _, fwd := range forwards {
		if strings.TrimSpace(fwd.Direction) != DirectionServerToClient {
			continue
		}
		listenAddr := strings.TrimSpace(fwd.ListenAddr)
		if listenAddr == "" {
			return fmt.Errorf("forward %q server_to_client requires listen_addr", fwd.Name)
		}
		host, portText, err := net.SplitHostPort(listenAddr)
		if err != nil || strings.Trim(strings.TrimSpace(host), "[]") == "" {
			return fmt.Errorf("forward %q reverse listen_addr must include a loopback host and port", fwd.Name)
		}
		if port, err := strconv.Atoi(portText); err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("forward %q reverse listen_addr port must be between 1 and 65535", fwd.Name)
		}
		if !isLoopbackName(host) {
			return fmt.Errorf("forward %q reverse listen_addr must use a loopback address", fwd.Name)
		}
	}
	return nil
}

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
	if host != "" {
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

func normalizeVirtualIPv4(value string) (net.IP, error) {
	virtualIP := net.ParseIP(strings.TrimSpace(value))
	if virtualIP == nil {
		return nil, fmt.Errorf("virtual listen_addr does not support domain names; use an IPv4 address from the server certificate SAN")
	}
	if virtualIP.To4() == nil {
		return nil, fmt.Errorf("virtual listen_addr host must be an IPv4 address")
	}
	virtualIP = virtualIP.To4()
	if virtualIP[0] == 0 || virtualIP[0] >= 224 || virtualIP.IsLoopback() || virtualIP.IsLinkLocalUnicast() {
		return nil, fmt.Errorf("virtual listen_addr host must be a usable non-local IPv4 address")
	}
	return virtualIP, nil
}

func isLoopbackName(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func ValidateConfigVersion(cfg Config) error {
	if cfg.ConfigVersion < 0 {
		return fmt.Errorf("client config_version must be >= 0")
	}
	if cfg.ConfigVersion > appversion.ClientConfigVersion {
		return fmt.Errorf("client config_version %d requires a newer client; current client supports config_version %d", cfg.ConfigVersion, appversion.ClientConfigVersion)
	}
	if err := appversion.CheckMin(appversion.AppVersion, cfg.Requires.MinClientVersion, "client config"); err != nil {
		return err
	}
	return nil
}

func CheckConfigUpgradeCompatible(path string) error {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	ApplyDefaults(&cfg)
	if err := ValidateConfigVersion(cfg); err != nil {
		return err
	}
	return validateReverseForwardFields(cfg.Forwards)
}

func resolveConfigPath(base, p string) string {
	p = strings.TrimSpace(p)
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(base, p))
}
