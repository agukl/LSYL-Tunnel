//go:build windows

package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientFrontendRoutes(t *testing.T) {
	mux := http.NewServeMux()
	app := &App{}
	app.registerAdminRoutes(mux)
	for _, tc := range []struct {
		path        string
		status      int
		contentType string
		contains    string
	}{
		{"/", http.StatusOK, "text/html", `<link rel="stylesheet" href="/styles.css">`},
		{"/styles.css", http.StatusOK, "text/css", ".mini-logo:focus-visible"},
		{"/app.js", http.StatusOK, "javascript", "function showLogoMenu"},
		{"/app.js?v=1", http.StatusOK, "javascript", "installLogoMenuDismiss();"},
		{"/front.go", http.StatusNotFound, "text/plain", ""},
		{"/conf/client.yaml", http.StatusNotFound, "text/plain", ""},
		{"/api/unknown", http.StatusNotFound, "text/plain", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if got := response.Header().Get("Content-Type"); !strings.Contains(got, tc.contentType) {
				t.Fatalf("content type = %q, want %q", got, tc.contentType)
			}
			if got := response.Header().Get("Cache-Control"); tc.status == http.StatusOK && got != "no-store" {
				t.Fatalf("cache control = %q", got)
			}
			if !strings.Contains(response.Body.String(), tc.contains) {
				t.Fatalf("response missing %q", tc.contains)
			}
		})
	}
}

func TestClientAPIRoutesTakePrecedenceOverAssets(t *testing.T) {
	mux := http.NewServeMux()
	app := &App{}
	app.registerAdminRoutes(mux)
	for _, path := range []string{
		"/api/state", "/api/health/check", "/api/start", "/api/stop",
		"/api/profile/switch", "/api/mobile/export", "/api/hide", "/api/quit",
		"/api/window/minimize", "/api/window/close", "/api/window/drag",
	} {
		_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, path, nil))
		if pattern != path {
			t.Errorf("route %s handled by %q", path, pattern)
		}
	}
}
