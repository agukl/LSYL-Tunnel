//go:build windows

package gui

import (
	"errors"
	"net"
	"strings"
	"testing"

	"lsyltunnel/src/server/tunnel"
)

func TestAdminForwardRejectsLoopbackAliases(t *testing.T) {
	for _, host := range []string{"localhost", "::1", "127.0.0.2", "::ffff:127.0.0.1"} {
		for _, direction := range []string{tunnel.DirectionClientToServer, tunnel.DirectionServerToClient} {
			fwd := adminForward{Direction: direction, AllowedUsers: []string{"alice"}}
			if direction == tunnel.DirectionClientToServer {
				fwd.ServerTarget = net.JoinHostPort(host, "3389")
			} else {
				fwd.ListenAddr = net.JoinHostPort(host, "13389")
			}
			issues := validateAdminForwardsForSave(adminConfig{
				Users: []adminUser{{Username: "alice"}}, Forwards: []adminForward{fwd},
			})
			if !issues.hasErrors() || !strings.Contains(issues.summary(), "127.0.0.1") {
				t.Fatalf("alias %q (%s): %v", host, direction, issues)
			}
		}
	}
}

func TestLoopbackAliasErrorKeepsFieldAndCorrection(t *testing.T) {
	got := friendlyError(errors.New("monitor_addr: loopback host \"::1\" is not supported; use 127.0.0.1"))
	if !strings.Contains(got, "monitor_addr") || !strings.Contains(got, "不再支持，请使用 127.0.0.1") {
		t.Fatal(got)
	}
}
