//go:build windows

package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
	"lsyltunnel/src/internal/protocol"
)

func testAutostartStore(t *testing.T) *clientAutostart {
	t.Helper()
	key := fmt.Sprintf(`Software\LSYL-Tunnel-Tests\Autostart-%d`, time.Now().UnixNano())
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, key) })
	return &clientAutostart{root: registry.CURRENT_USER, key: key, executable: `C:\Program Files\LSYL Tunnel Client\bin\lsyl-tunnel-client-gui.exe`}
}

func TestAutostartRegistryRoundTrip(t *testing.T) {
	s := testAutostartStore(t)
	if enabled, err := s.enabled(); err != nil || enabled {
		t.Fatalf("initial state = %v, %v", enabled, err)
	}
	for _, want := range []bool{true, true, false, false} {
		if err := s.setEnabled(want); err != nil {
			t.Fatal(err)
		}
		if enabled, err := s.enabled(); err != nil || enabled != want {
			t.Fatalf("enabled = %v, %v; want %v", enabled, err, want)
		}
	}
	if err := s.setEnabled(true); err != nil {
		t.Fatal(err)
	}
	k, err := registry.OpenKey(s.root, s.key, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	value, _, err := k.GetStringValue(clientAutostartValue)
	if err != nil || value != `"C:\Program Files\LSYL Tunnel Client\bin\lsyl-tunnel-client-gui.exe" --autostart` {
		t.Fatalf("command = %q, %v", value, err)
	}
	other := `"D:\Other Client\bin\lsyl-tunnel-client-gui.exe" --autostart`
	if err := k.SetStringValue(clientAutostartValue, other); err != nil {
		t.Fatal(err)
	}
	if err := s.setEnabled(false); err != nil {
		t.Fatal(err)
	}
	if got, _, err := k.GetStringValue(clientAutostartValue); err != nil || got != other {
		t.Fatal("another installation was modified")
	}
}

func TestAutostartRejectsInvalidCommands(t *testing.T) {
	for _, path := range []string{"", "client.exe", `C:\bad"name.exe`, "C:\\bad\x00.exe", `C:\` + strings.Repeat("a", 260) + `.exe`} {
		if _, err := (&clientAutostart{executable: path}).command(); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	for _, args := range [][]string{nil, {"--autostart=false"}, {"--autostart", "--quit"}, {"--quit"}} {
		if isAutostartLaunch(args) {
			t.Fatalf("unexpected autostart for %v", args)
		}
	}
	if !isAutostartLaunch([]string{"--autostart"}) {
		t.Fatal("autostart flag rejected")
	}
}

func TestAutostartWorkspaceUsesExecutable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "conf"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "conf", "client.yaml"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := autostartWorkspace(filepath.Join(root, "bin", clientGUIExecutableName)); got != root {
		t.Fatalf("root = %s", got)
	}
}

func TestAutostartAPI(t *testing.T) {
	a := &App{autostart: testAutostartStore(t), configPath: filepath.Join(t.TempDir(), "client.yaml"), panelURL: "http://127.0.0.1:43210/"}
	for _, tt := range []struct {
		method, body, origin string
		status               int
		ok                   bool
	}{
		{"GET", "", "", 405, false},
		{"POST", `{}`, "", 200, false},
		{"POST", `{"enabled":true,"extra":1}`, "", 200, false},
		{"POST", `{"enabled":true} {}`, "", 200, false},
		{"POST", `{"enabled":true}`, "https://example.com", 403, false},
		{"POST", `{"enabled":true}`, "http://127.0.0.1:43210", 200, true},
		{"POST", `{"enabled":false}`, "", 200, true},
	} {
		r := httptest.NewRequest(tt.method, a.panelURL+"api/autostart", strings.NewReader(tt.body))
		r.Header.Set("Origin", tt.origin)
		w := httptest.NewRecorder()
		a.handleAutostart(w, r)
		var result apiResult
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if w.Code != tt.status || result.OK != tt.ok {
			t.Fatalf("%+v: status=%d body=%s", tt, w.Code, w.Body.String())
		}
	}
}

func TestSavedLoginDoesNotUseOtherPasswordSources(t *testing.T) {
	a := &App{configPath: filepath.Join(t.TempDir(), "client.yaml")}
	cfg := defaultClientConfig(a.configPath)
	cfg.Username = "alice"
	cfg.Password, cfg.PasswordEnv, cfg.PasswordFile = "plain", "LSYL_TEST_PASSWORD", "password.txt"
	if err := a.saveClientConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := a.savedLoginConfig(); !errors.Is(err, errNoSavedLogin) {
		t.Fatalf("missing sealed credential: %v", err)
	}
	cfg.SavedCredential = protocol.SealedCredential{Ciphertext: "sealed"}
	if err := a.saveClientConfig(cfg); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(a.configPath)
	got, err := a.savedLoginConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Password != "" || got.PasswordEnv != "" || got.PasswordFile != "" || got.SavedCredential.Ciphertext != "sealed" {
		t.Fatalf("credentials = %+v", got)
	}
	after, _ := os.ReadFile(a.configPath)
	if string(before) != string(after) {
		t.Fatal("autostart rewrote the profile")
	}
}

func TestLoginGuardCancelsAndExcludesConcurrentOperations(t *testing.T) {
	a := &App{}
	ctx, done, ok := a.beginLogin(context.Background())
	if !ok || !a.connecting.Load() {
		t.Fatal("login did not start")
	}
	if _, _, ok := a.beginLogin(context.Background()); ok {
		t.Fatal("concurrent login accepted")
	}
	w := httptest.NewRecorder()
	a.handleProfileSwitch(w, httptest.NewRequest("POST", "/api/profile/switch", strings.NewReader(`{"id":"other"}`)))
	if !strings.Contains(w.Body.String(), "正在连接") {
		t.Fatal("profile switch was not blocked")
	}
	a.cancelPendingLogin()
	if ctx.Err() != context.Canceled {
		t.Fatal("exit did not cancel login")
	}
	done()
	if a.connecting.Load() {
		t.Fatal("connecting state leaked")
	}
	if _, _, ok := a.beginLogin(context.Background()); ok {
		t.Fatal("login accepted after exit")
	}
}
