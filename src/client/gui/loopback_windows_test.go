//go:build windows

package gui

import (
	"strings"
	"testing"
)

func TestLoopbackAliasErrorKeepsFieldAndCorrection(t *testing.T) {
	raw := "forward \"rdp\" listen_addr: loopback host \"localhost\" is not supported; use 127.0.0.1"
	got := friendlyErrorText(raw)
	for _, text := range []string{"rdp", "listen_addr", "localhost", "不再支持，请使用 127.0.0.1"} {
		if !strings.Contains(got, text) {
			t.Fatalf("message %q missing %q", got, text)
		}
	}
}

func TestCompactDisplayDoesNotHideUnsupportedLoopbackAliases(t *testing.T) {
	for _, addr := range []string{"localhost:3389", "[::1]:3389", "127.0.0.2:3389"} {
		if got := compactDisplayAddr(addr); got != addr {
			t.Fatalf("%q displayed as %q", addr, got)
		}
	}
	if got := compactDisplayAddr("127.0.0.1:3389"); got != "3389" {
		t.Fatal(got)
	}
}
