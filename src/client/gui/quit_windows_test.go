//go:build windows

package gui

import "testing"

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
