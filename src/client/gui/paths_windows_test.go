//go:build windows

package gui

import (
	"os"
	"path/filepath"
	"testing"
)

func writeClientPathFixture(t *testing.T, root, relative string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClientConfigPathLayouts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		want  string
	}{
		{"source", []string{"src/client/conf/client.yaml"}, "src/client/conf/client.yaml"},
		{"installed", []string{"conf/client.yaml"}, "conf/client.yaml"},
		{"source priority", []string{"src/client/conf/client.yaml", "conf/client.yaml"}, "src/client/conf/client.yaml"},
		{"ignore legacy", []string{"client/conf/client.yaml", "conf/client.yaml"}, "conf/client.yaml"},
		{"legacy only", []string{"client/conf/client.yaml"}, "src/client/conf/client.yaml"},
		{"missing", nil, "src/client/conf/client.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, file := range tc.files {
				writeClientPathFixture(t, root, file)
			}
			if got, want := detectClientConfigPath(root), filepath.Join(root, filepath.FromSlash(tc.want)); got != want {
				t.Fatalf("config path = %q, want %q", got, want)
			}
		})
	}
}

func TestClientWorkspaceMarkers(t *testing.T) {
	for _, marker := range []string{"go.mod", "src/client/conf/client.yaml", "conf/client.yaml"} {
		t.Run(marker, func(t *testing.T) {
			root := t.TempDir()
			writeClientPathFixture(t, root, marker)
			if got := findWorkspaceFrom(filepath.Join(root, "nested", "bin")); got != root {
				t.Fatalf("workspace = %q, want %q", got, root)
			}
		})
	}
	root := t.TempDir()
	writeClientPathFixture(t, root, "client/conf/client.yaml")
	if got := findWorkspaceFrom(root); got == root {
		t.Fatal("obsolete source layout must not identify a workspace")
	}
}

func TestClientWorkspaceOverrideAndTempPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LSYL_TUNNEL_WORKSPACE", root)
	if got := detectWorkspaceRoot(); got != root {
		t.Fatalf("explicit workspace = %q, want %q", got, root)
	}
	if got := appTmpDir(root); got != filepath.Join(root, "tmp") {
		t.Fatalf("installed temp directory = %q", got)
	}
	writeClientPathFixture(t, root, "go.mod")
	if got := appTmpDir(root); got != filepath.Join(root, "build", "tmp") {
		t.Fatalf("source temp directory = %q", got)
	}
	if fileExists(root) {
		t.Fatal("directory must not count as a config or executable file")
	}
}
