package mobileprofile

import (
	"strings"
	"testing"

	"lsyltunnel/src/client/tunnel"
)

func TestExportRejectsUnsupportedTargetHosts(t *testing.T) {
	for _, target := range []string{"db.internal:3306", "203.0.113.1:3306", "0.0.0.0:3306"} {
		_, err := forwardFromConfig(tunnel.ForwardConfig{Name: "db", ListenAddr: "127.0.0.1:13306", ServerTarget: target})
		if err == nil || !strings.Contains(err.Error(), "server_target must use") {
			t.Fatalf("target %s: %v", target, err)
		}
	}
}
