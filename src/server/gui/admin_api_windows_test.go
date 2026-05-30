//go:build windows

package gui

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"lsyltunnel/src/internal/passutil"
	"lsyltunnel/src/server/tunnel"
)

func TestAdminConfigToTunnelWritesForwardAllowedUsers(t *testing.T) {
	cfg, err := adminConfigToTunnel(adminConfig{
		ListenAddr:  "127.0.0.1:9443",
		MonitorAddr: "127.0.0.1:19111",
		TLS:         adminTLS{CertFile: "server.crt", KeyFile: "server.key", MinVersion: "1.3"},
		Users: []adminUser{
			{Username: "alice", PasswordHash: "plain:secret"},
			{Username: "bob", PasswordHash: "plain:secret"},
		},
		Forwards: []adminForward{{
			Name:         "rdp",
			Direction:    tunnel.DirectionClientToServer,
			Port:         "3389",
			AllowedUsers: []string{"alice", "bob"},
		}},
	}, tunnel.Config{})
	if err != nil {
		t.Fatalf("adminConfigToTunnel returned error: %v", err)
	}
	if got := cfg.Forwards[0].AllowedUsers; !reflect.DeepEqual(got, []string{"alice", "bob"}) {
		t.Fatalf("AllowedUsers = %#v, want alice,bob", got)
	}
	if got := cfg.Forwards[0].ServerTarget; got != "127.0.0.1:3389" {
		t.Fatalf("ServerTarget = %q, want 127.0.0.1:3389", got)
	}
}

func TestAdminConfigFromTunnelReadsForwardAllowedUsers(t *testing.T) {
	cfg := tunnel.Config{
		TLS: tunnel.TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Auth: tunnel.AuthConfig{Users: []tunnel.UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Forwards: []tunnel.ForwardConfig{{
			Name:         "rdp",
			Direction:    tunnel.DirectionClientToServer,
			ServerTarget: "127.0.0.1:3389",
			AllowedUsers: []string{"alice", "bob"},
		}},
	}
	form := adminConfigFromTunnel(cfg)
	if got := form.Forwards[0].Owner; got != "alice" {
		t.Fatalf("Owner = %q, want alice", got)
	}
	if got := form.Forwards[0].AllowedUsers; !reflect.DeepEqual(got, []string{"alice", "bob"}) {
		t.Fatalf("AllowedUsers = %#v, want alice,bob", got)
	}
	if got := form.Forwards[0].Port; got != "3389" {
		t.Fatalf("Port = %q, want 3389", got)
	}
}

func TestAdminConfigToTunnelNormalizesReversePort(t *testing.T) {
	cfg, err := adminConfigToTunnel(adminConfig{
		ListenAddr:  "127.0.0.1:9443",
		MonitorAddr: "127.0.0.1:19111",
		TLS:         adminTLS{CertFile: "server.crt", KeyFile: "server.key", MinVersion: "1.3"},
		Users: []adminUser{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}},
		Forwards: []adminForward{{
			Name:      "web",
			Direction: tunnel.DirectionServerToClient,
			Port:      "18080",
			Owner:     "alice",
		}},
	}, tunnel.Config{})
	if err != nil {
		t.Fatalf("adminConfigToTunnel returned error: %v", err)
	}
	if got := cfg.Forwards[0].ListenAddr; got != "127.0.0.1:18080" {
		t.Fatalf("ListenAddr = %q, want 127.0.0.1:18080", got)
	}
	if got := cfg.Forwards[0].ServerTarget; got != "" {
		t.Fatalf("ServerTarget = %q, want empty", got)
	}
	if got := cfg.Forwards[0].AllowedUsers; !reflect.DeepEqual(got, []string{"alice"}) {
		t.Fatalf("AllowedUsers = %#v, want alice", got)
	}
}

func TestAdminConfigToTunnelPreservesDuplicateReversePortAllowedUsers(t *testing.T) {
	cfg, err := adminConfigToTunnel(adminConfig{
		ListenAddr:  "127.0.0.1:9443",
		MonitorAddr: "127.0.0.1:19111",
		TLS:         adminTLS{CertFile: "server.crt", KeyFile: "server.key", MinVersion: "1.3"},
		Users: []adminUser{
			{Username: "alice", PasswordHash: "plain:secret"},
			{Username: "bob", PasswordHash: "plain:secret"},
		},
		Forwards: []adminForward{
			{Name: "web-a", Direction: tunnel.DirectionServerToClient, Port: "18080", Owner: "alice"},
			{Name: "web-b", Direction: tunnel.DirectionServerToClient, Port: "18080", Owner: "bob"},
		},
	}, tunnel.Config{})
	if err != nil {
		t.Fatalf("adminConfigToTunnel returned error: %v", err)
	}
	if got := cfg.Forwards[0].ListenAddr; got != "127.0.0.1:18080" {
		t.Fatalf("first ListenAddr = %q, want 127.0.0.1:18080", got)
	}
	if got := cfg.Forwards[1].ListenAddr; got != "127.0.0.1:18080" {
		t.Fatalf("second ListenAddr = %q, want 127.0.0.1:18080", got)
	}
	if got := cfg.Forwards[0].AllowedUsers; !reflect.DeepEqual(got, []string{"alice"}) {
		t.Fatalf("first AllowedUsers = %#v, want alice", got)
	}
	if got := cfg.Forwards[1].AllowedUsers; !reflect.DeepEqual(got, []string{"bob"}) {
		t.Fatalf("second AllowedUsers = %#v, want bob", got)
	}
}

func TestAdminConfigToTunnelRejectsInvalidForwardPort(t *testing.T) {
	_, err := adminConfigToTunnel(adminConfig{
		ListenAddr:  "127.0.0.1:9443",
		MonitorAddr: "127.0.0.1:19111",
		TLS:         adminTLS{CertFile: "server.crt", KeyFile: "server.key", MinVersion: "1.3"},
		Users: []adminUser{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}},
		Forwards: []adminForward{{
			Name:      "bad",
			Direction: tunnel.DirectionClientToServer,
			Port:      "70000",
			Owner:     "alice",
		}},
	}, tunnel.Config{})
	if err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestValidateAdminForwardsForSaveAcceptsConfiguredAllowedUsers(t *testing.T) {
	issues := validateAdminForwardsForSave(adminConfig{
		Users: []adminUser{
			{Username: "alice"},
			{Username: "bob"},
		},
		Forwards: []adminForward{{
			Direction:    tunnel.DirectionClientToServer,
			Port:         "3389",
			AllowedUsers: []string{"alice", "bob"},
		}},
	})
	if issues.hasErrors() {
		t.Fatalf("validateAdminForwardsForSave returned issues: %#v", issues)
	}
}

func TestValidateAdminForwardsForSaveRejectsUnknownAllowedUser(t *testing.T) {
	issues := validateAdminForwardsForSave(adminConfig{
		Users: []adminUser{{Username: "alice"}},
		Forwards: []adminForward{{
			Direction:    tunnel.DirectionClientToServer,
			Port:         "3389",
			AllowedUsers: []string{"bob"},
		}},
	})
	if !issues.hasErrors() {
		t.Fatal("expected validation error")
	}
	if got := issues[0].Field; got != "forwards[0].allowed_users" {
		t.Fatalf("issue field = %q, want forwards[0].allowed_users", got)
	}
}

func TestValidateAdminForwardsForSaveRejectsPassiveMultiUser(t *testing.T) {
	issues := validateAdminForwardsForSave(adminConfig{
		Users: []adminUser{{Username: "alice"}, {Username: "bob"}},
		Forwards: []adminForward{{
			Direction:    tunnel.DirectionServerToClient,
			Port:         "18080",
			AllowedUsers: []string{"alice", "bob"},
		}},
	})
	if !issues.hasErrors() {
		t.Fatal("expected passive single-owner validation error")
	}
	if got := issues[0].Field; got != "forwards[0].allowed_users" {
		t.Fatalf("issue field = %q, want forwards[0].allowed_users", got)
	}
}

func TestValidateAdminForwardsForSaveRejectsPassivePortAssignedToDifferentUsers(t *testing.T) {
	issues := validateAdminForwardsForSave(adminConfig{
		Users: []adminUser{{Username: "alice"}, {Username: "bob"}},
		Forwards: []adminForward{
			{Direction: tunnel.DirectionServerToClient, Port: "18080", Owner: "alice"},
			{Direction: tunnel.DirectionServerToClient, Port: "18080", Owner: "bob"},
		},
	})
	if !issues.hasErrors() {
		t.Fatal("expected duplicate passive owner validation error")
	}
}

func TestValidateAdminForwardsForSaveChecksPassivePortAvailability(t *testing.T) {
	ln, port := listenLocalTCP(t)
	defer ln.Close()

	issues := validateAdminForwardsForSaveWithOptions(adminConfig{
		Users: []adminUser{{Username: "alice"}},
		Forwards: []adminForward{{
			Direction: tunnel.DirectionServerToClient,
			Port:      port,
			Owner:     "alice",
		}},
	}, adminForwardValidationOptions{CheckPassivePortAvailability: true})
	if !issues.hasErrors() {
		t.Fatal("expected occupied passive port validation error")
	}
}

func TestValidateAdminForwardsForSaveSkipsCurrentPassivePortWhenServiceRunning(t *testing.T) {
	ln, port := listenLocalTCP(t)
	defer ln.Close()
	listenAddr := "127.0.0.1:" + port

	issues := validateAdminForwardsForSaveWithOptions(adminConfig{
		Users: []adminUser{{Username: "alice"}},
		Forwards: []adminForward{{
			Direction: tunnel.DirectionServerToClient,
			Port:      port,
			Owner:     "alice",
		}},
	}, adminForwardValidationOptions{
		Existing: tunnel.Config{Forwards: []tunnel.ForwardConfig{{
			Direction:  tunnel.DirectionServerToClient,
			ListenAddr: listenAddr,
		}}},
		ServiceRunning:               true,
		CheckPassivePortAvailability: true,
	})
	if issues.hasErrors() {
		t.Fatalf("validateAdminForwardsForSaveWithOptions returned issues: %#v", issues)
	}
}

func TestValidateAdminForwardsForSaveChecksForwardTargetReachability(t *testing.T) {
	ln, port := listenLocalTCP(t)
	defer ln.Close()
	go acceptOneTCP(ln)

	issues := validateAdminForwardsForSaveWithOptions(adminConfig{
		Users: []adminUser{{Username: "alice"}},
		Forwards: []adminForward{{
			Direction: tunnel.DirectionClientToServer,
			Port:      port,
			Owner:     "alice",
		}},
	}, adminForwardValidationOptions{
		CheckForwardTargetReachability: true,
		DialTimeout:                    500 * time.Millisecond,
	})
	if issues.hasErrors() {
		t.Fatalf("validateAdminForwardsForSaveWithOptions returned issues: %#v", issues)
	}
}

func TestHandleAdminConfigRejectsOccupiedPassivePort(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "server.yaml")
	if err := tunnel.SaveConfig(configPath, tunnel.Config{
		ListenAddr: "127.0.0.1:9443",
		TLS:        tunnel.TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
	}); err != nil {
		t.Fatal(err)
	}
	ln, port := listenLocalTCP(t)
	defer ln.Close()

	app := NewApp()
	app.configPath = configPath
	app.serviceName = "LSYL Tunnel Test Missing Service"
	form := adminConfig{
		ListenAddr: "127.0.0.1:9443",
		TLS:        adminTLS{CertFile: "server.crt", KeyFile: "server.key", MinVersion: "1.3"},
		Users:      []adminUser{{Username: "alice", PasswordHash: "plain:secret"}},
		Forwards: []adminForward{{
			Name:      "reverse-web",
			Direction: tunnel.DirectionServerToClient,
			Port:      port,
			Owner:     "alice",
		}},
	}
	res := postAdminConfig(t, app, form)
	if res.OK {
		t.Fatalf("expected save to fail for occupied passive port: %#v", res)
	}
	if len(res.Issues) == 0 || res.Issues[0].Field != "forwards[0].port" {
		t.Fatalf("unexpected validation issues: %#v", res.Issues)
	}
	cfg, err := tunnel.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Forwards) != 0 {
		t.Fatalf("config was saved despite validation failure: %#v", cfg.Forwards)
	}
}

func TestHandleAdminConfigSavesUnreachableForwardTarget(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "server.yaml")
	if err := tunnel.SaveConfig(configPath, tunnel.Config{
		ListenAddr: "127.0.0.1:9443",
		TLS:        tunnel.TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
	}); err != nil {
		t.Fatal(err)
	}
	_, port := closedLocalTCPPort(t)

	app := NewApp()
	app.configPath = configPath
	app.serviceName = "LSYL Tunnel Test Missing Service"
	form := adminConfig{
		ListenAddr: "127.0.0.1:9443",
		TLS:        adminTLS{CertFile: "server.crt", KeyFile: "server.key", MinVersion: "1.3"},
		Users:      []adminUser{{Username: "alice", PasswordHash: "plain:secret"}},
		Forwards: []adminForward{{
			Name:      "rdp",
			Direction: tunnel.DirectionClientToServer,
			Port:      port,
			Owner:     "alice",
		}},
	}
	res := postAdminConfig(t, app, form)
	if !res.OK {
		t.Fatalf("expected save to accept config-only forward target: %#v", res)
	}
	cfg, err := tunnel.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Forwards) != 1 || cfg.Forwards[0].ServerTarget != "127.0.0.1:"+port {
		t.Fatalf("config was not saved as expected: %#v", cfg.Forwards)
	}
}

func TestHandleAdminPasswordHashReturnsVerifiableHash(t *testing.T) {
	app := NewApp()
	body, err := json.Marshal(passwordHashRequest{Password: "new-secret"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/password/hash", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	app.handleAdminPasswordHash(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var res passwordHashResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.PasswordHash == "" {
		t.Fatalf("unexpected response: %#v", res)
	}
	if !passutil.VerifyPassword("new-secret", res.PasswordHash) {
		t.Fatalf("hash does not verify")
	}
	if passutil.VerifyPassword(res.PasswordHash, res.PasswordHash) {
		t.Fatalf("hash should not act as the plaintext password")
	}
}

func TestHandleAdminUnblockIPRemovesPersistedBlockedIP(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "server.yaml")
	stateFile := filepath.Join(dir, "server-state.json")
	cfg := tunnel.Config{
		ListenAddr:  "127.0.0.1:9443",
		MonitorAddr: "127.0.0.1:19111",
		TLS:         tunnel.TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Runtime:     tunnel.RuntimeConfig{StateFile: stateFile},
	}
	if err := tunnel.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	blockedUntil := time.Now().Add(time.Hour).Format(time.RFC3339)
	stateJSON := `{"blocked_ips":[{"ip":"203.0.113.10","blocked_until":"` + blockedUntil + `"}]}`
	if err := os.WriteFile(stateFile, []byte(stateJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	app.configPath = configPath
	app.serviceName = "LSYL Tunnel Test Missing Service"
	body, err := json.Marshal(unblockIPRequest{IP: "203.0.113.10"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/security/unblock", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	app.handleAdminUnblockIP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var res apiResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("unexpected response: %#v", res)
	}
	if got, err := tunnel.LoadBlockedIPs(stateFile); err != nil || len(got) != 0 {
		t.Fatalf("LoadBlockedIPs = %#v, %v", got, err)
	}
}

func TestBuildAdminStateIncludesPermanentBlockedIPs(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "conf", "server.yaml")
	permanentFile := filepath.Join(dir, "data", "server-permanent-block.txt")
	cfg := tunnel.Config{
		ListenAddr:  "127.0.0.1:9443",
		MonitorAddr: "127.0.0.1:19111",
		TLS:         tunnel.TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Runtime:     tunnel.RuntimeConfig{PermanentBlockFile: permanentFile},
	}
	if err := tunnel.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(permanentFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(permanentFile, []byte("# comment\n203.0.113.20\ninvalid\n203.0.113.10\n203.0.113.20\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := &App{configPath: configPath, serviceName: "LSYL Tunnel Test Missing Service"}

	state := app.buildAdminState("")

	if got := len(state.PermanentBlocks); got != 2 {
		t.Fatalf("PermanentBlocks length = %d, want 2: %#v", got, state.PermanentBlocks)
	}
	if state.PermanentBlocks[0].IP != "203.0.113.10" || state.PermanentBlocks[1].IP != "203.0.113.20" {
		t.Fatalf("PermanentBlocks not sorted/deduped: %#v", state.PermanentBlocks)
	}
	if state.PermanentBlocks[0].Source != permanentFile {
		t.Fatalf("PermanentBlocks source = %q, want %q", state.PermanentBlocks[0].Source, permanentFile)
	}
}

func listenLocalTCP(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		ln.Close()
		t.Fatal(err)
	}
	return ln, port
}

func closedLocalTCPPort(t *testing.T) (string, string) {
	t.Helper()
	ln, port := listenLocalTCP(t)
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr, port
}

func postAdminConfig(t *testing.T, app *App, form adminConfig) apiResult {
	t.Helper()
	body, err := json.Marshal(form)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	app.handleAdminConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res apiResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func acceptOneTCP(ln net.Listener) {
	conn, err := ln.Accept()
	if err == nil {
		_ = conn.Close()
	}
}
