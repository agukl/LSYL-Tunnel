//go:build windows

package gui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"lsyltunnel/src/internal/localapi"
)

func TestClientQuitRequiresPanelToken(t *testing.T) {
	var gate *localapi.Gate
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gate.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(w, r)
	}))
	defer server.Close()
	var err error
	gate, err = localapi.New(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if err := postClientQuit(server.URL+"/api/quit", gate.Token()); err != nil {
		t.Fatalf("authorized quit failed: %v", err)
	}
	if err := postClientQuit(server.URL+"/api/quit", "invalid"); err == nil {
		t.Fatal("unauthorized quit succeeded")
	}
}

func TestClientQuitSupportsLegacyInstanceWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(localapi.TokenHeader) != "" {
			t.Error("legacy quit request unexpectedly included a token")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := postClientQuit(server.URL+"/api/quit", ""); err != nil {
		t.Fatalf("legacy quit failed: %v", err)
	}
}

func TestClientQuitArgs(t *testing.T) {
	for _, arg := range []string{"/quit", "-quit", "--quit", "/exit", "-exit", "--exit", "  /QUIT  "} {
		if !isClientQuitArg(arg) {
			t.Fatalf("isClientQuitArg(%q) = false", arg)
		}
		if !IsQuitCommand([]string{arg}) {
			t.Fatalf("IsQuitCommand(%q) = false", arg)
		}
	}
	for _, arg := range []string{"", "/start", "quit-now"} {
		if isClientQuitArg(arg) {
			t.Fatalf("isClientQuitArg(%q) = true", arg)
		}
		if IsQuitCommand([]string{arg}) {
			t.Fatalf("IsQuitCommand(%q) = true", arg)
		}
	}
}

func TestIsLocalClientHost(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "::1", "[::1]"} {
		if !isLocalClientHost(host) {
			t.Fatalf("isLocalClientHost(%q) = false", host)
		}
	}
	for _, host := range []string{"example.com", "192.168.1.10", "0.0.0.0"} {
		if isLocalClientHost(host) {
			t.Fatalf("isLocalClientHost(%q) = true", host)
		}
	}
}
