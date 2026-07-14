//go:build windows

package gui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileSwitchFriendlyError(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "newer client", raw: "target config is not compatible with this client: client config_version 4 requires a newer client", want: "目标配置需要更高版本的客户端，请升级客户端后再切换。"},
		{name: "permission", raw: "归档当前配置失败: Access is denied", want: "没有权限切换配置文件，请以管理员身份运行或检查安装目录权限。"},
		{name: "read permission", raw: "读取当前配置失败: Access is denied", want: "没有权限切换配置文件，请以管理员身份运行或检查安装目录权限。"},
		{name: "current config missing", raw: "读取当前配置失败: The system cannot find the file specified", want: "当前配置文件不存在，请修复或重新安装客户端。"},
		{name: "current config malformed", raw: "读取当前配置失败: yaml: cannot unmarshal", want: "当前配置文件格式不正确，请联系管理员检查配置。"},
		{name: "archive collision", raw: "切换目标配置失败: 目标文件已存在", want: "配置归档文件已存在，无法安全切换，请联系管理员清理重复配置。"},
		{name: "current files busy", raw: "归档当前证书失败: file is in use", want: "无法归档当前配置，请确认配置文件和证书未被其他程序占用。"},
		{name: "target files busy", raw: "切换目标配置失败: file is in use", want: "无法启用目标配置，请确认目标配置和证书未被其他程序占用。"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := profileSwitchFriendlyError(errors.New(tt.raw)); got != tt.want {
				t.Fatalf("profileSwitchFriendlyError() = %q, want %q", got, tt.want)
			}
		})
	}
}

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
