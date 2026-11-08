//go:build windows

package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminAssetHandlerServesPageAndAPI(t *testing.T) {
	handler := (&App{}).adminHandler()
	for _, tc := range []struct {
		method string
		path   string
		origin string
		status int
		body   string
	}{
		{http.MethodGet, "/", "", http.StatusOK, "LSYL Tunnel"},
		{http.MethodGet, "/app.js", "", http.StatusOK, "bindWindowControls"},
		{http.MethodGet, "/modules/window.js", "", http.StatusOK, "ToggleMaximise"},
		{http.MethodPost, "/api/password/hash", "http://wails.localhost", http.StatusBadRequest, "密码请求格式不正确"},
		{http.MethodPost, "/api/password/hash", "https://other.example", http.StatusForbidden, "invalid panel origin"},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(tc.method, "http://wails.localhost"+tc.path, nil)
		if tc.origin != "" {
			request.Header.Set("Origin", tc.origin)
		}
		handler.ServeHTTP(response, request)
		if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.body) {
			t.Errorf("%s %s: status %d, body %q", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
}
