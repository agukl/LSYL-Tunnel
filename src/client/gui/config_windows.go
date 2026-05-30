//go:build windows

package gui

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"lsyltunnel/src/client/tunnel"
	"lsyltunnel/src/internal/protocol"
)

func (a *App) currentLoginForm() loginForm {
	cfg, err := readClientConfigRaw(a.configPath)
	if err != nil {
		a.appendLog("读取配置失败: " + err.Error())
		cfg = defaultClientConfig(a.configPath)
	}
	return loginForm{ServerAddr: cfg.ServerAddr, Username: cfg.Username, Password: ""}
}

func (a *App) saveLoginForm(form loginForm) error {
	cfg, err := a.prepareLoginConfig(form)
	if err != nil {
		return err
	}
	cfg.Password = ""
	return a.saveClientConfig(cfg)
}

func (a *App) prepareLoginConfig(form loginForm) (tunnel.Config, error) {
	cfg, err := readClientConfigRaw(a.configPath)
	if err != nil {
		cfg = defaultClientConfig(a.configPath)
	}
	cfg.ServerAddr = strings.TrimSpace(form.ServerAddr)
	cfg.Username = strings.TrimSpace(form.Username)
	if form.Password != "" {
		cfg.Password = form.Password
		cfg.PasswordEnv = ""
		cfg.PasswordFile = ""
		cfg.SavedCredential = protocol.SealedCredential{}
	}
	tunnel.ApplyDefaults(&cfg)
	return cfg, nil
}

func (a *App) saveClientConfig(cfg tunnel.Config) error {
	tunnel.ApplyDefaults(&cfg)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.configPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(a.configPath, data, 0o644)
}

func (a *App) clearSavedPasswordState() error {
	cfg, err := readClientConfigRaw(a.configPath)
	if err != nil {
		return err
	}
	changed := false
	if strings.TrimSpace(cfg.Password) != "" {
		cfg.Password = ""
		changed = true
	}
	if strings.TrimSpace(cfg.SavedCredential.Ciphertext) != "" {
		cfg.SavedCredential = protocol.SealedCredential{}
		changed = true
	}
	if !changed {
		return nil
	}
	return a.saveClientConfig(cfg)
}

func (a *App) routeSummary() string {
	cfg, err := readClientConfigRaw(a.configPath)
	if err != nil {
		return "配置未读取"
	}
	if len(cfg.Forwards) == 0 {
		return "未配置端口映射"
	}
	lines := make([]string, 0, len(cfg.Forwards))
	for _, fwd := range cfg.Forwards {
		name := strings.TrimSpace(fwd.Name)
		if name == "" {
			name = "转发"
		}
		if fwd.Direction == tunnel.DirectionServerToClient {
			lines = append(lines, fmt.Sprintf("%s: 服务端 %s -> 本机 %s", name, compactDisplayAddr(fwd.ListenAddr), compactDisplayAddr(fwd.ServerTarget)))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: 本机 %s -> 服务端 %s", name, compactDisplayAddr(fwd.ListenAddr), compactDisplayAddr(fwd.ServerTarget)))
	}
	return strings.Join(lines, "\n")
}

func (a *App) hasPasswordState() bool {
	cfg, err := readClientConfigRaw(a.configPath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(cfg.PasswordFile) != "" ||
		strings.TrimSpace(cfg.PasswordEnv) != "" ||
		strings.TrimSpace(cfg.SavedCredential.Ciphertext) != "" ||
		strings.TrimSpace(cfg.Password) != ""
}

func compactDisplayAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "-"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if isCompactLocalHost(host) {
		return port
	}
	return net.JoinHostPort(host, port)
}

func isCompactLocalHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return true
	}
	switch strings.ToLower(host) {
	case "localhost", "0.0.0.0", "::", "::1":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func readClientConfigRaw(path string) (tunnel.Config, error) {
	var cfg tunnel.Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	tunnel.ApplyDefaults(&cfg)
	return cfg, nil
}

func runtimeClientConfigFromRaw(configPath string, cfg tunnel.Config) (tunnel.Config, error) {
	base := filepath.Dir(configPath)
	tunnel.ApplyDefaults(&cfg)
	cfg.TLS.CACertFile = resolveLocalConfigPath(base, cfg.TLS.CACertFile)
	cfg.PasswordFile = resolveLocalConfigPath(base, cfg.PasswordFile)
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
	return cfg, tunnel.ValidateConfig(cfg)
}

func resolveLocalConfigPath(base, p string) string {
	p = strings.TrimSpace(p)
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(base, p))
}

func defaultClientConfig(configPath string) tunnel.Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "lsyl-tunnel-client"
	}
	return tunnel.Config{
		ServerAddr: "",
		Username:   "",
		Password:   "",
		ClientID:   hostname,
		LogLevel:   "info",
		TLS: tunnel.TLSConfig{
			CACertFile: defaultCACertFile(configPath),
			ServerName: "",
			MinVersion: "1.3",
		},
		Connection: tunnel.ConnectionConfig{DialTimeoutSec: 5},
		Forwards:   []tunnel.ForwardConfig{},
	}
}

func defaultCACertFile(configPath string) string {
	return "../cert/server.crt"
}
