//go:build windows

package gui

import (
	"os"
	"path/filepath"
	"strings"
)

func (a *App) frontDir() string {
	return filepath.Join(a.workspace, "src", "server", "front")
}

func detectWorkspaceRoot() string {
	if env := strings.TrimSpace(os.Getenv("LSYL_TUNNEL_WORKSPACE")); env != "" {
		if info, err := os.Stat(env); err == nil && info.IsDir() {
			return env
		}
	}
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		if root := findWorkspaceFrom(start); root != "" {
			return root
		}
	}
	if len(starts) > 0 {
		return starts[0]
	}
	return "."
}

func findWorkspaceFrom(start string) string {
	cur := filepath.Clean(start)
	for i := 0; i < 8; i++ {
		if fileExists(filepath.Join(cur, "go.mod")) ||
			fileExists(filepath.Join(cur, "src", "server", "conf", "server.yaml")) ||
			fileExists(filepath.Join(cur, "src", "client", "conf", "client.yaml")) ||
			fileExists(filepath.Join(cur, "server", "conf", "server.yaml")) ||
			fileExists(filepath.Join(cur, "client", "conf", "client.yaml")) ||
			fileExists(filepath.Join(cur, "conf", "server.yaml")) ||
			fileExists(filepath.Join(cur, "conf", "client.yaml")) {
			return cur
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return ""
}

func detectServerServiceExe(workspace string) string {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "lsyl-tunnel-server-svc.exe")
		if fileExists(candidate) {
			return candidate
		}
	}
	if candidate := filepath.Join(workspace, "bin", "lsyl-tunnel-server-svc.exe"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "build", "bin", "server", "lsyl-tunnel-server-svc.exe"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "server", "bin", "lsyl-tunnel-server-svc.exe"); fileExists(candidate) {
		return candidate
	}
	return filepath.Join(workspace, "build", "bin", "server", "lsyl-tunnel-server-svc.exe")
}

func detectServerConfigPath(workspace string) string {
	if candidate := filepath.Join(workspace, "src", "server", "conf", "server.yaml"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "server", "conf", "server.yaml"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "conf", "server.yaml"); fileExists(candidate) {
		return candidate
	}
	return filepath.Join(workspace, "src", "server", "conf", "server.yaml")
}

func detectServerServiceLog(workspace string) string {
	if fileExists(filepath.Join(workspace, "bin", "lsyl-tunnel-server-svc.exe")) {
		return filepath.Join(workspace, "logs", "service", "server-service.log")
	}
	return filepath.Join(workspace, "runtime", "logs", "service", "server-service.log")
}

func appTmpDir(workspace string) string {
	if fileExists(filepath.Join(workspace, "go.mod")) {
		return filepath.Join(workspace, "build", "tmp")
	}
	return filepath.Join(workspace, "tmp")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
