//go:build windows

package gui

import (
	"os"
	"path/filepath"
	"strings"
)

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
			fileExists(filepath.Join(cur, "src", "client", "conf", "client.yaml")) ||
			fileExists(filepath.Join(cur, "client", "conf", "client.yaml")) ||
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

func detectClientConfigPath(workspace string) string {
	if candidate := filepath.Join(workspace, "src", "client", "conf", "client.yaml"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "client", "conf", "client.yaml"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "conf", "client.yaml"); fileExists(candidate) {
		return candidate
	}
	return filepath.Join(workspace, "src", "client", "conf", "client.yaml")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func appTmpDir(workspace string) string {
	if fileExists(filepath.Join(workspace, "go.mod")) {
		return filepath.Join(workspace, "build", "tmp")
	}
	return filepath.Join(workspace, "tmp")
}
