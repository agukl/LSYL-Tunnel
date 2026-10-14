//go:build windows

package gui

import (
	"errors"
	"strings"
	"testing"
)

func TestReversePortConflictMessageKeepsRuleAndFields(t *testing.T) {
	got := friendlyError(errors.New(`forward "reverse-web" listen_port conflicts with legacy listen_addr; keep only listen_port or use the same port`))
	for _, want := range []string{"reverse-web", "listen_port", "listen_addr", "端口冲突"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}
