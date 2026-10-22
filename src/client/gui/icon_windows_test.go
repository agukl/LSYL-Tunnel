//go:build windows

package gui

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestClientIconCandidatesCoverCurrentLayouts(t *testing.T) {
	a := &App{workspace: t.TempDir()}
	for _, name := range []string{"client.ico", "client-connected.ico"} {
		candidates := a.clientIconCandidates(name)
		for _, want := range []string{
			filepath.Join(a.workspace, "assets", name),
			filepath.Join(a.workspace, "src", "client", "assets", name),
		} {
			if !slices.Contains(candidates, want) {
				t.Fatalf("icon candidates %v missing %q", candidates, want)
			}
		}
	}
}
