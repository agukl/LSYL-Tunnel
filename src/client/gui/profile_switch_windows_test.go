//go:build windows

package gui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClientProfileFileSuffix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "vpn.example.com:3443", want: "vpn.example.com_3443"},
		{in: "  127.0.0.1:3443  ", want: "127.0.0.1_3443"},
		{in: "[::1]:3443", want: "1_3443"},
		{in: "bad/host:*?", want: "bad_host"},
	}
	for _, tt := range tests {
		if got := clientProfileFileSuffix(tt.in); got != tt.want {
			t.Fatalf("clientProfileFileSuffix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSwitchClientProfileFilesSwapsCurrentAndTarget(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf")
	certDir := filepath.Join(dir, "cert")
	mustWrite(t, filepath.Join(confDir, "client.yaml"), "current-config")
	mustWrite(t, filepath.Join(certDir, "server.crt"), "current-cert")
	mustWrite(t, filepath.Join(confDir, "client.target.yaml"), "target-config")
	mustWrite(t, filepath.Join(certDir, "server.target.crt"), "target-cert")
	mustWrite(t, filepath.Join(confDir, "client.current.yaml"), "old-current-archive")
	mustWrite(t, filepath.Join(certDir, "server.current.crt"), "old-current-cert")

	err := switchClientProfileFiles(profileSwitchPaths{
		CurrentConfig: filepath.Join(confDir, "client.yaml"),
		CurrentCert:   filepath.Join(certDir, "server.crt"),
		ArchiveConfig: filepath.Join(confDir, "client.current.yaml"),
		ArchiveCert:   filepath.Join(certDir, "server.current.crt"),
		TargetConfig:  filepath.Join(confDir, "client.target.yaml"),
		TargetCert:    filepath.Join(certDir, "server.target.crt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(confDir, "client.yaml"), "target-config")
	assertFile(t, filepath.Join(certDir, "server.crt"), "target-cert")
	assertFile(t, filepath.Join(confDir, "client.current.yaml"), "current-config")
	assertFile(t, filepath.Join(certDir, "server.current.crt"), "current-cert")
	assertMissing(t, filepath.Join(confDir, "client.target.yaml"))
	assertMissing(t, filepath.Join(certDir, "server.target.crt"))
}

func TestSwitchClientProfileFilesRollsBackOnFailure(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf")
	certDir := filepath.Join(dir, "cert")
	mustWrite(t, filepath.Join(confDir, "client.yaml"), "current-config")
	mustWrite(t, filepath.Join(certDir, "server.crt"), "current-cert")
	mustWrite(t, filepath.Join(confDir, "client.target.yaml"), "target-config")
	mustWrite(t, filepath.Join(confDir, "client.current.yaml"), "old-current-archive")
	mustWrite(t, filepath.Join(certDir, "server.current.crt"), "old-current-cert")

	err := switchClientProfileFiles(profileSwitchPaths{
		CurrentConfig: filepath.Join(confDir, "client.yaml"),
		CurrentCert:   filepath.Join(certDir, "server.crt"),
		ArchiveConfig: filepath.Join(confDir, "client.current.yaml"),
		ArchiveCert:   filepath.Join(certDir, "server.current.crt"),
		TargetConfig:  filepath.Join(confDir, "client.target.yaml"),
		TargetCert:    filepath.Join(certDir, "server.target.crt"),
	})
	if err == nil {
		t.Fatal("expected switch failure")
	}
	assertFile(t, filepath.Join(confDir, "client.yaml"), "current-config")
	assertFile(t, filepath.Join(certDir, "server.crt"), "current-cert")
	assertFile(t, filepath.Join(confDir, "client.target.yaml"), "target-config")
	assertFile(t, filepath.Join(confDir, "client.current.yaml"), "old-current-archive")
	assertFile(t, filepath.Join(certDir, "server.current.crt"), "old-current-cert")
}

func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists or stat failed with unexpected error: %v", path, err)
	}
}
