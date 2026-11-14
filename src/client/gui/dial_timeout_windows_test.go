//go:build windows

package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestDialTimeoutAPI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	a := &App{configPath: path, panelURL: "http://127.0.0.1:43210/"}
	cfg := defaultClientConfig(path)
	cfg.ServerAddr = "example.com:443"
	cfg.Username = "alice"
	if err := a.saveClientConfig(cfg); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		method, body, origin string
		status               int
		ok                   bool
		want                 int
	}{
		{"GET", "", "", 405, false, 5},
		{"POST", `{}`, "", 200, false, 5},
		{"POST", `{"dial_timeout_sec":4}`, "", 200, false, 5},
		{"POST", `{"dial_timeout_sec":61}`, "", 200, false, 5},
		{"POST", `{"dial_timeout_sec":10,"other":1}`, "", 200, false, 5},
		{"POST", `{"dial_timeout_sec":10}`, "https://example.com", 403, false, 5},
		{"POST", `{"dial_timeout_sec":10}`, "http://127.0.0.1:43210", 200, true, 10},
		{"POST", `{"dial_timeout_sec":60}`, "", 200, true, 60},
	} {
		r := httptest.NewRequest(tt.method, a.panelURL+"api/dial-timeout", strings.NewReader(tt.body))
		r.Header.Set("Origin", tt.origin)
		w := httptest.NewRecorder()
		a.handleDialTimeout(w, r)
		var result apiResult
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		got, err := readClientConfigRaw(path)
		if err != nil {
			t.Fatal(err)
		}
		if w.Code != tt.status || result.OK != tt.ok || got.Connection.DialTimeoutSec != tt.want {
			t.Fatalf("%+v: status=%d body=%s saved=%d", tt, w.Code, w.Body.String(), got.Connection.DialTimeoutSec)
		}
		if result.OK && (result.State == nil || result.State.Config.DialTimeoutSec != tt.want) {
			t.Fatalf("response state does not contain saved timeout: %+v", result.State)
		}
	}
	a.connecting.Store(true)
	w := httptest.NewRecorder()
	a.handleDialTimeout(w, httptest.NewRequest(http.MethodPost, a.panelURL+"api/dial-timeout", strings.NewReader(`{"dial_timeout_sec":10}`)))
	if strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatal("timeout changed while connecting")
	}
	got, err := readClientConfigRaw(path)
	if err != nil || got.Connection.DialTimeoutSec != 60 {
		t.Fatalf("saved timeout changed while connecting: %d, %v", got.Connection.DialTimeoutSec, err)
	}
}
