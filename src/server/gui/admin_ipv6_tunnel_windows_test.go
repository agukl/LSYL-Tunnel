//go:build windows

package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lsyltunnel/src/server/tunnel"
)

func TestAdminIPv6TunnelToggleSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yaml")
	if err := tunnel.SaveConfig(path, tunnel.Config{ListenAddr: "127.0.0.1:9443", TLS: tunnel.TLSConfig{CertFile: "server.crt", KeyFile: "server.key"}}); err != nil {
		t.Fatal(err)
	}
	app := newAdminTestApp(t)
	app.configPath, app.serviceName = path, "LSYL Tunnel Test Missing Service"
	form := adminConfig{
		ListenAddr: "127.0.0.1:9443", IPv6TunnelEnabled: true,
		TLS:   adminTLS{CertFile: "server.crt", KeyFile: "server.key", MinVersion: "1.3"},
		Users: []adminUser{{Username: "alice", PasswordHash: "plain:secret"}},
	}
	if res := postAdminConfig(t, app, form); !res.OK {
		t.Fatalf("enable IPv6 tunnel: %#v", res)
	}
	cfg, err := tunnel.LoadConfig(path)
	if err != nil || !cfg.IPv6TunnelEnabled {
		t.Fatalf("IPv6 tunnel enabled = %v, %v", cfg.IPv6TunnelEnabled, err)
	}
	form.IPv6TunnelEnabled = false
	if res := postAdminConfig(t, app, form); !res.OK {
		t.Fatalf("disable IPv6 tunnel: %#v", res)
	}
	cfg, err = tunnel.LoadConfig(path)
	if err != nil || cfg.IPv6TunnelEnabled {
		t.Fatalf("IPv6 tunnel enabled = %v, %v", cfg.IPv6TunnelEnabled, err)
	}
}

func TestAdminIPv6TunnelStatusRoute(t *testing.T) {
	app := newAdminTestApp(t)
	request := httptest.NewRequest(http.MethodGet, "http://wails.localhost/api/ipv6-tunnel", nil)
	recorder := httptest.NewRecorder()
	app.adminHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var view ipv6TunnelView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.MonitorAvailable || view.Listening {
		t.Fatalf("unexpected active tunnel: %+v", view)
	}
}

func TestAdminIPv6TunnelStatusIncludesMonitorListening(t *testing.T) {
	monitor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tunnel.MonitorStatus{
			IPv6TunnelListening: true,
		})
	}))
	defer monitor.Close()
	path := filepath.Join(t.TempDir(), "server.yaml")
	if err := tunnel.SaveConfig(path, tunnel.Config{
		ListenAddr: "0.0.0.0:3443", MonitorAddr: strings.TrimPrefix(monitor.URL, "http://"),
		TLS: tunnel.TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
	}); err != nil {
		t.Fatal(err)
	}
	app := newAdminTestApp(t)
	app.configPath = path
	request := httptest.NewRequest(http.MethodGet, "http://wails.localhost/api/ipv6-tunnel", nil)
	recorder := httptest.NewRecorder()
	app.adminHandler().ServeHTTP(recorder, request)
	var view ipv6TunnelView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.MonitorAvailable || !view.Listening {
		t.Fatalf("IPv6 tunnel status = %+v", view)
	}
}
