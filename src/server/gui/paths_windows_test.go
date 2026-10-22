//go:build windows

package gui

import (
	"os"
	"path/filepath"
	"testing"
)

func writeServerPathFixture(t *testing.T, root, relative string) string {
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

func TestServerConfigPathLayouts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		want  string
	}{
		{"source", []string{"src/server/conf/server.yaml"}, "src/server/conf/server.yaml"},
		{"installed", []string{"conf/server.yaml"}, "conf/server.yaml"},
		{"source priority", []string{"src/server/conf/server.yaml", "conf/server.yaml"}, "src/server/conf/server.yaml"},
		{"ignore legacy", []string{"server/conf/server.yaml", "conf/server.yaml"}, "conf/server.yaml"},
		{"legacy only", []string{"server/conf/server.yaml"}, "src/server/conf/server.yaml"},
		{"missing", nil, "src/server/conf/server.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, file := range tc.files {
				writeServerPathFixture(t, root, file)
			}
			if got, want := detectServerConfigPath(root), filepath.Join(root, filepath.FromSlash(tc.want)); got != want {
				t.Fatalf("config path = %q, want %q", got, want)
			}
		})
	}
}

func TestServerWorkspaceMarkers(t *testing.T) {
	for _, marker := range []string{"go.mod", "src/server/conf/server.yaml", "conf/server.yaml"} {
		t.Run(marker, func(t *testing.T) {
			root := t.TempDir()
			writeServerPathFixture(t, root, marker)
			if got := findWorkspaceFrom(filepath.Join(root, "nested", "bin")); got != root {
				t.Fatalf("workspace = %q, want %q", got, root)
			}
		})
	}
	for _, marker := range []string{"server/conf/server.yaml", "client/conf/client.yaml", "src/client/conf/client.yaml", "conf/client.yaml"} {
		t.Run("ignore "+marker, func(t *testing.T) {
			root := t.TempDir()
			writeServerPathFixture(t, root, marker)
			if got := findWorkspaceFrom(root); got == root {
				t.Fatal("legacy or client-only directory must not identify a server workspace")
			}
		})
	}
}

func TestServerRuntimePathLayouts(t *testing.T) {
	for _, tc := range []struct {
		name, executable, wantExecutable, wantLog string
	}{
		{"installed", "bin/lsyl-tunnel-server-svc.exe", "bin/lsyl-tunnel-server-svc.exe", "logs/service/server-service.log"},
		{"source", "build/bin/server/lsyl-tunnel-server-svc.exe", "build/bin/server/lsyl-tunnel-server-svc.exe", "runtime/logs/service/server-service.log"},
		{"ignore legacy", "server/bin/lsyl-tunnel-server-svc.exe", "build/bin/server/lsyl-tunnel-server-svc.exe", "runtime/logs/service/server-service.log"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeServerPathFixture(t, root, tc.executable)
			if got, want := detectServerServiceExe(root), filepath.Join(root, filepath.FromSlash(tc.wantExecutable)); got != want {
				t.Fatalf("service exe = %q, want %q", got, want)
			}
			if got, want := detectServerServiceLog(root), filepath.Join(root, filepath.FromSlash(tc.wantLog)); got != want {
				t.Fatalf("service log = %q, want %q", got, want)
			}
		})
	}
}

func TestServerWorkspaceOverrideAndTempPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LSYL_TUNNEL_WORKSPACE", root)
	if got := detectWorkspaceRoot(); got != root {
		t.Fatalf("explicit workspace = %q, want %q", got, root)
	}
	if got := appTmpDir(root); got != filepath.Join(root, "tmp") {
		t.Fatalf("installed temp directory = %q", got)
	}
	writeServerPathFixture(t, root, "go.mod")
	if got := appTmpDir(root); got != filepath.Join(root, "build", "tmp") {
		t.Fatalf("source temp directory = %q", got)
	}
	if fileExists(root) {
		t.Fatal("directory must not count as a config or executable file")
	}
}
