package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigPasswordFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "password.txt"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, "client.yaml")
	data := []byte(`server_addr: 127.0.0.1:9443
username: alice
password_file: password.txt
tls:
  insecure_skip_verify: true
forwards:
  - name: echo
    listen_addr: 127.0.0.1:0
    server_target: 127.0.0.1:80
`)
	if err := os.WriteFile(cfgFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "secret" {
		t.Fatalf("unexpected password: %q", cfg.Password)
	}
}

func TestLoadConfigPasswordEnv(t *testing.T) {
	t.Setenv("LSYL_TUNNEL_TEST_PASSWORD", "from-env")
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "client.yaml")
	data := []byte(`server_addr: 127.0.0.1:9443
username: alice
password_env: LSYL_TUNNEL_TEST_PASSWORD
tls:
  insecure_skip_verify: true
forwards:
  - name: echo
    listen_addr: 127.0.0.1:0
    server_target: 127.0.0.1:80
`)
	if err := os.WriteFile(cfgFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password != "from-env" {
		t.Fatalf("unexpected password: %q", cfg.Password)
	}
}

func TestCheckConfigUpgradeCompatibleRejectsNewerConfigVersion(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "client.yaml")
	data := []byte(`config_version: 99
server_addr: 127.0.0.1:9443
username: alice
tls:
  insecure_skip_verify: true
forwards:
  - name: echo
    listen_addr: 127.0.0.1:0
    server_target: 127.0.0.1:80
`)
	if err := os.WriteFile(cfgFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	err := CheckConfigUpgradeCompatible(cfgFile)
	if err == nil {
		t.Fatal("expected newer config_version error")
	}
}

func TestCheckConfigUpgradeCompatibleRejectsReverseListenerFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	data := []byte(`config_version: 1
server_addr: 127.0.0.1:9443
forwards:
  - name: mysql
    direction: server_to_client
    listen_port: 3306
    server_target: 127.0.0.1:3307
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckConfigUpgradeCompatible(path); err == nil || !strings.Contains(err.Error(), `forward "mysql"`) || !strings.Contains(err.Error(), "must not include listen_port") {
		t.Fatalf("CheckConfigUpgradeCompatible() error = %v, want forbidden reverse listener field", err)
	}
}

func TestCheckConfigUpgradeCompatibleAllowsMultipleReverseRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	data := []byte(`config_version: 1
server_addr: 127.0.0.1:9443
forwards:
  - direction: server_to_client
    listen_addr: 127.0.0.1:13306
    server_target: 127.0.0.1:3307
  - direction: server_to_client
    listen_addr: 127.0.0.1:18080
    server_target: 127.0.0.1:8080
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckConfigUpgradeCompatible(path); err != nil {
		t.Fatalf("CheckConfigUpgradeCompatible() rejected multiple reverse rules: %v", err)
	}
}

func TestLoadConfigRejectsLegacyReverseListenerFields(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "client.yaml")
	data := []byte(`server_addr: 127.0.0.1:9443
username: alice
password: secret
tls:
  insecure_skip_verify: true
forwards:
  - name: reverse-web
    direction: server_to_client
    listen_port: 18080
    listen_addr: localhost:18081
    server_target: 127.0.0.1:8080
`)
	if err := os.WriteFile(cfgFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(cfgFile)
	if err == nil || !strings.Contains(err.Error(), `forward "reverse-web"`) || !strings.Contains(err.Error(), "must not include listen_port") {
		t.Fatalf("LoadConfig() error = %v, want rule-specific forbidden listen_port error", err)
	}
	dataAfter, readErr := os.ReadFile(cfgFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(dataAfter) != string(data) {
		t.Fatalf("LoadConfig() modified invalid config:\n%s", dataAfter)
	}
}

func TestLoadConfigAcceptsReverseListenAddressWithoutRewriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	data := []byte(`server_addr: 127.0.0.1:9443
username: alice
password: secret
tls:
  insecure_skip_verify: true
forwards:
  - name: mysql
    direction: server_to_client
    listen_addr: 127.0.0.1:3307
    server_target: 127.0.0.1:3307
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() rejected reverse listen_addr: %v", err)
	}
	if got := cfg.Forwards[0].ListenAddr; got != "127.0.0.1:3307" {
		t.Fatalf("LoadConfig() listen_addr = %q, want 127.0.0.1:3307", got)
	}
	dataAfter, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(dataAfter) != string(data) {
		t.Fatalf("LoadConfig() modified invalid config:\n%s", dataAfter)
	}
}

func TestLoadConfigRejectsEmptyReverseListenerFieldWithoutRewriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	data := []byte(`server_addr: 127.0.0.1:9443
username: alice
password: secret
tls:
  insecure_skip_verify: true
forwards:
  - name: mysql
    direction: server_to_client
    listen_addr: ""
    server_target: 127.0.0.1:3307
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), `forward "mysql"`) || !strings.Contains(err.Error(), "server_to_client requires listen_addr") {
		t.Fatalf("LoadConfig() error = %v, want required reverse listen_addr error", err)
	}
	dataAfter, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(dataAfter) != string(data) {
		t.Fatalf("LoadConfig() modified invalid config:\n%s", dataAfter)
	}
}

func TestSaveConfigWritesReverseAddresses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	cfg := Config{
		ServerAddr: "127.0.0.1:9443",
		Username:   "alice",
		Password:   "secret",
		TLS:        TLSConfig{InsecureSkipVerify: true},
		Forwards: []ForwardConfig{{
			Name:         "reverse-web",
			Direction:    DirectionServerToClient,
			ListenAddr:   "127.0.0.1:18080",
			ServerTarget: "127.0.0.1:8080",
		}},
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "listen_port:") || !strings.Contains(text, "listen_addr: 127.0.0.1:18080") || !strings.Contains(text, "server_target: 127.0.0.1:8080") {
		t.Fatalf("unexpected saved reverse config: %s", text)
	}
}

func TestValidateConfigAllowsMultipleReverseRules(t *testing.T) {
	cfg := Config{
		ServerAddr: "127.0.0.1:9443",
		Username:   "alice",
		Password:   "secret",
		TLS:        TLSConfig{InsecureSkipVerify: true},
		Forwards: []ForwardConfig{{
			Direction:    DirectionServerToClient,
			ListenAddr:   "127.0.0.1:13306",
			ServerTarget: "127.0.0.1:3306",
		}},
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() rejected one unnamed reverse rule: %v", err)
	}
	cfg.Forwards = append(cfg.Forwards, ForwardConfig{
		Name:         "web",
		Direction:    DirectionServerToClient,
		ListenAddr:   "127.0.0.1:18080",
		ServerTarget: "127.0.0.1:8080",
	})
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() rejected multiple reverse rules: %v", err)
	}
}

func TestForwardNameUsesReverseListenAddress(t *testing.T) {
	fwd := ForwardConfig{
		Direction:    DirectionServerToClient,
		ListenAddr:   "127.0.0.1:13306",
		ServerTarget: "127.0.0.1:3306",
	}
	if got := forwardName(fwd); got != fwd.ListenAddr {
		t.Fatalf("forwardName() = %q, want reverse listen_addr %q", got, fwd.ListenAddr)
	}
}

func TestValidateConfigRequiresLoopbackReverseTarget(t *testing.T) {
	cfg := Config{
		ServerAddr: "127.0.0.1:9443",
		Username:   "alice",
		Password:   "secret",
		TLS:        TLSConfig{InsecureSkipVerify: true},
		Forwards: []ForwardConfig{{
			Name:         "mysql",
			Direction:    DirectionServerToClient,
			ListenAddr:   "127.0.0.1:13306",
			ServerTarget: "192.168.1.10:3306",
		}},
	}
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "reverse server_target must use a loopback address") {
		t.Fatalf("ValidateConfig() error = %v, want loopback reverse target", err)
	}
}

func TestLoadConfigAcceptsVirtualDirectionWithListenAddress(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "client.yaml")
	certPath := filepath.ToSlash(writeTestServerCertificate(t, "192.0.2.22"))
	data := []byte(fmt.Sprintf(`server_addr: 198.51.100.10:9443
username: alice
password: secret
tls:
  ca_cert_file: %q
forwards:
  - name: ssh
    direction: virtual
    listen_addr: 192.0.2.22:2222
    server_target: 10.20.30.40:22
`, certPath))
	if err := os.WriteFile(cfgFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Forwards[0].Direction; got != DirectionVirtual {
		t.Fatalf("Direction = %q, want %q", got, DirectionVirtual)
	}
	if got := cfg.Forwards[0].ListenAddr; got != "192.0.2.22:2222" {
		t.Fatalf("ListenAddr = %q, want 192.0.2.22:2222", got)
	}
	if got, err := VirtualForwardAddr(cfg, cfg.Forwards[0]); err != nil || got != "192.0.2.22:2222" {
		t.Fatalf("VirtualForwardAddr = %q, error = %v", got, err)
	}
}

func TestLoadConfigAcceptsAutomaticVirtualAddress(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "client.yaml")
	certPath := filepath.ToSlash(writeTestServerCertificate(t, "127.0.0.1", "192.0.2.22"))
	data := []byte(fmt.Sprintf(`server_addr: vpn.example.test:9443
username: alice
password: secret
tls:
  ca_cert_file: %q
forwards:
  - name: ssh
    direction: virtual
    listen_addr: ":2222"
    server_target: 10.20.30.40:22
`, certPath))
	if err := os.WriteFile(cfgFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Forwards[0].ListenAddr; got != ":2222" {
		t.Fatalf("ListenAddr = %q, want :2222", got)
	}
	if got, err := VirtualForwardAddr(cfg, cfg.Forwards[0]); err != nil || got != "192.0.2.22:2222" {
		t.Fatalf("VirtualForwardAddr = %q, error = %v", got, err)
	}
}

func TestSaveConfigPreservesVirtualDirectionAndListenAddress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	cfg := validVirtualForwardConfig(t)
	cfg.Forwards[0].ListenAddr = ":22"
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "direction: virtual") || strings.Contains(text, "virtual_ip:") {
		t.Fatalf("unexpected saved virtual IP config: %s", text)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Forwards[0].ListenAddr; got != ":22" {
		t.Fatalf("saved automatic virtual listen_addr = %q, want :22", got)
	}
}

func TestLoadConfigRejectsLegacyVirtualIPField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	data := []byte(`server_addr: 198.51.100.10:9443
username: alice
password: secret
tls:
  insecure_skip_verify: true
forwards:
  - direction: virtual
    virtual_ip: 192.0.2.22
    server_target: 10.20.30.40:22
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "virtual listen_addr") {
		t.Fatalf("LoadConfig() error = %v, want virtual listen_addr error", err)
	}
}

func TestValidateConfigRejectsInvalidVirtualForwardSettings(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		message string
	}{
		{
			name: "domain name",
			mutate: func(cfg *Config) {
				cfg.Forwards[0].ListenAddr = "virtual.example:22"
			},
			message: "virtual listen_addr does not support domain names",
		},
		{
			name: "IPv6 address",
			mutate: func(cfg *Config) {
				cfg.Forwards[0].ListenAddr = "[2001:db8::22]:22"
			},
			message: "virtual listen_addr host must be an IPv4 address",
		},
		{
			name: "loopback IP",
			mutate: func(cfg *Config) {
				cfg.Forwards[0].ListenAddr = "127.0.0.2:22"
			},
			message: "virtual listen_addr host must be a usable non-local IPv4 address",
		},
		{
			name: "missing port",
			mutate: func(cfg *Config) {
				cfg.Forwards[0].ListenAddr = "192.0.2.22"
			},
			message: "virtual listen_addr must use :port or IPv4:port",
		},
		{
			name: "invalid port",
			mutate: func(cfg *Config) {
				cfg.Forwards[0].ListenAddr = "192.0.2.22:70000"
			},
			message: "virtual listen_addr port must be between 1 and 65535",
		},
		{
			name: "server port conflict",
			mutate: func(cfg *Config) {
				cfg.Forwards[0].ListenAddr = "192.0.2.22:9443"
			},
			message: "virtual listen_addr cannot use the server_addr port",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validVirtualForwardConfig(t)
			tt.mutate(&cfg)
			err := ValidateConfig(cfg)
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("ValidateConfig() error = %v, want %q", err, tt.message)
			}
		})
	}
}

func TestValidateConfigRejectsDuplicateVirtualEndpoint(t *testing.T) {
	cfg := validVirtualForwardConfig(t)
	cfg.Forwards = append(cfg.Forwards, ForwardConfig{
		Name:         "duplicate",
		Direction:    DirectionVirtual,
		ListenAddr:   "192.0.2.22:22",
		ServerTarget: "10.20.30.41:2222",
	})
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "duplicates forward") {
		t.Fatalf("ValidateConfig() error = %v, want duplicate virtual endpoint error", err)
	}
}

func TestValidateConfigAcceptsSameVirtualIPOnDifferentPorts(t *testing.T) {
	cfg := validVirtualForwardConfig(t)
	cfg.Forwards = append(cfg.Forwards, ForwardConfig{
		Name:         "https",
		Direction:    DirectionVirtual,
		ListenAddr:   "192.0.2.22:443",
		ServerTarget: "10.20.30.41:8443",
	})
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

func TestValidateConfigAutomaticVirtualIPUsesServerAddress(t *testing.T) {
	cfg := validVirtualForwardConfig(t)
	cfg.ServerAddr = "198.51.100.10:9443"
	cfg.TLS.CACertFile = writeTestServerCertificate(t, "192.0.2.22", "198.51.100.10")
	cfg.Forwards[0].ListenAddr = ":22"
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if got, err := VirtualForwardAddr(cfg, cfg.Forwards[0]); err != nil || got != "198.51.100.10:22" {
		t.Fatalf("VirtualForwardAddr = %q, error = %v", got, err)
	}
}

func TestValidateConfigRejectsAmbiguousAutomaticVirtualIP(t *testing.T) {
	cfg := validVirtualForwardConfig(t)
	cfg.ServerAddr = "vpn.example.test:9443"
	cfg.TLS.CACertFile = writeTestServerCertificate(t, "192.0.2.22", "198.51.100.10")
	cfg.Forwards[0].ListenAddr = ":22"
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "multiple usable IPv4 SANs") {
		t.Fatalf("ValidateConfig() error = %v, want ambiguous IPv4 SAN error", err)
	}
}

func TestValidateConfigAcceptsExplicitVirtualIPWithMultipleSANs(t *testing.T) {
	cfg := validVirtualForwardConfig(t)
	cfg.ServerAddr = "vpn.example.test:9443"
	cfg.TLS.CACertFile = writeTestServerCertificate(t, "192.0.2.22", "198.51.100.10")
	cfg.Forwards[0].ListenAddr = "192.0.2.22:22"
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

func TestValidateConfigAcceptsCertificateIPMatchingServerAddressOnDifferentPort(t *testing.T) {
	cfg := validVirtualForwardConfig(t)
	cfg.ServerAddr = "192.0.2.22:9443"
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

func TestValidateConfigRejectsVirtualIPOutsideCertificateSAN(t *testing.T) {
	cfg := validVirtualForwardConfig(t)
	cfg.Forwards[0].ListenAddr = "198.51.100.22:22"
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "not authorized by the server certificate IPv4 SAN") {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

func TestValidateConfigRejectsTooManyVirtualEndpoints(t *testing.T) {
	cfg := validVirtualForwardConfig(t)
	cfg.Forwards = nil
	for i := 0; i <= maxVirtualRedirectEndpoints; i++ {
		port := 10000 + i
		cfg.Forwards = append(cfg.Forwards, ForwardConfig{
			Name:         fmt.Sprintf("virtual-%d", i),
			Direction:    DirectionVirtual,
			ListenAddr:   fmt.Sprintf("192.0.2.22:%d", port),
			ServerTarget: fmt.Sprintf("10.20.30.40:%d", port),
		})
	}
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "at most 48 endpoints") {
		t.Fatalf("ValidateConfig() error = %v, want endpoint limit", err)
	}
}

func validVirtualForwardConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		ServerAddr: "198.51.100.10:9443",
		Username:   "alice",
		Password:   "secret",
		TLS:        TLSConfig{CACertFile: writeTestServerCertificate(t, "192.0.2.22")},
		Forwards: []ForwardConfig{{
			Name:         "ssh",
			Direction:    DirectionVirtual,
			ListenAddr:   "192.0.2.22:22",
			ServerTarget: "10.20.30.40:22",
		}},
	}
}
