package localapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGateProtectsAPIButServesPage(t *testing.T) {
	gate, err := New("http://127.0.0.1:43210/")
	if err != nil {
		t.Fatal(err)
	}
	if len(gate.Token()) != 64 || !strings.Contains(gate.Script(), gate.Token()) {
		t.Fatal("WebView2 injection does not contain a fresh token")
	}
	handler := gate.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tt := range []struct {
		path, host, origin, token string
		status                    int
	}{
		{"/", "127.0.0.1:43210", "", "", http.StatusNoContent},
		{"/api/state", "127.0.0.1:43210", "", "", http.StatusForbidden},
		{"/api/state", "other.example:43210", "", gate.Token(), http.StatusForbidden},
		{"/api/state", "127.0.0.1:43210", "https://other.example", gate.Token(), http.StatusForbidden},
		{"/api/state", "127.0.0.1:43210", "http://127.0.0.1:43210", gate.Token(), http.StatusNoContent},
	} {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43210"+tt.path, nil)
		req.Host = tt.host
		req.Header.Set("Origin", tt.origin)
		req.Header.Set(TokenHeader, tt.token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tt.status {
			t.Fatalf("%s host=%s origin=%s: status=%d, want %d", tt.path, tt.host, tt.origin, rec.Code, tt.status)
		}
	}
}

func TestGateRejectsLargeAPIRequest(t *testing.T) {
	gate, err := New("http://127.0.0.1:43210/")
	if err != nil {
		t.Fatal(err)
	}
	handler := gate.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43210/api/config", strings.NewReader(strings.Repeat("x", maxAPIRequestBytes+1)))
	req.Header.Set(TokenHeader, gate.Token())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", rec.Code)
	}
}
