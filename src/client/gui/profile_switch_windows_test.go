//go:build windows

package gui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileSwitchFriendlyError(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "newer client", raw: "目标配置校验失败: client config_version 4 requires a newer client", want: "目标配置需要更高版本的客户端，请升级客户端后再切换。"},
		{name: "incompatible structure", raw: "目标配置校验失败: field legacy_virtual_ip not found in type tunnel.Config", want: "目标配置结构与当前客户端不兼容，请联系管理员重新下发配置。"},
		{name: "certificate mismatch", raw: "目标配置校验失败: x509: certificate doesn't contain any IP SANs", want: "目标配置与配套证书不匹配，请联系管理员重新下发配置。"},
		{name: "invalid content", raw: "目标配置校验失败: forward web requires server_target", want: "目标配置内容无效，请联系管理员检查转发规则和服务端地址。"},
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

func TestClientProfileLabelAppendsOptionalRemark(t *testing.T) {
	tests := []struct {
		name       string
		serverAddr string
		fallback   string
		remark     string
		want       string
	}{
		{name: "without remark", serverAddr: "vpn.example.com:3443", fallback: "archive", want: "vpn.example.com:3443"},
		{name: "with remark", serverAddr: "203.0.113.10:3443", fallback: "archive", remark: "上海生产节点", want: "203.0.113.10:3443 · 上海生产节点"},
		{name: "normalizes whitespace", serverAddr: "vpn.example.com:3443", fallback: "archive", remark: "  备用\n节点  ", want: "vpn.example.com:3443 · 备用 节点"},
		{name: "fallback", fallback: "当前配置", remark: "测试", want: "当前配置 · 测试"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientProfileLabel(tt.serverAddr, tt.fallback, tt.remark); got != tt.want {
				t.Fatalf("clientProfileLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadClientProfilesDisplaysConfigRemarks(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "conf", "client.yaml")
	mustWrite(t, configPath, clientProfileConfigForTest("current.example.com:3443", "当前节点"))
	mustWrite(t, filepath.Join(dir, "cert", "server.crt"), "certificate-present")
	mustWrite(t, filepath.Join(dir, "conf", "client.backup.yaml"), clientProfileConfigForTest("backup.example.com:4443", "备用节点"))
	mustWrite(t, filepath.Join(dir, "cert", "server.backup.crt"), "certificate-present")

	profiles := (&App{configPath: configPath}).loadClientProfiles()
	if len(profiles) != 2 {
		t.Fatalf("loadClientProfiles() returned %d profiles, want 2", len(profiles))
	}
	if profiles[0].Label != "current.example.com:3443 · 当前节点" {
		t.Fatalf("current profile label = %q", profiles[0].Label)
	}
	if profiles[1].Label != "backup.example.com:4443 · 备用节点" {
		t.Fatalf("archived profile label = %q", profiles[1].Label)
	}
}

func clientProfileConfigForTest(serverAddr, remark string) string {
	return "server_addr: " + serverAddr + "\n" +
		"remark: " + remark + "\n" +
		"username: alice\n" +
		"tls:\n" +
		"  ca_cert_file: ../cert/server.crt\n" +
		"forwards:\n" +
		"  - name: rdp\n" +
		"    direction: client_to_server\n" +
		"    listen_addr: 127.0.0.1:3389\n" +
		"    server_target: 127.0.0.1:3389\n"
}

func TestClientProfilesCacheReturnsCopiesAndCanBeInvalidated(t *testing.T) {
	app := &App{
		configPath:        filepath.Join(t.TempDir(), "conf", "client.yaml"),
		profileCache:      []clientProfile{{ID: "cached", Label: "Cached profile"}},
		profileCacheUntil: time.Now().Add(time.Minute),
	}

	first := app.clientProfiles()
	if len(first) != 1 || first[0].ID != "cached" {
		t.Fatalf("clientProfiles() = %#v, want cached profile", first)
	}
	first[0].Label = "modified"
	second := app.clientProfiles()
	if second[0].Label != "Cached profile" {
		t.Fatalf("cached profile was modified through returned slice: %#v", second)
	}

	app.invalidateClientProfiles()
	loaded := app.clientProfiles()
	if len(loaded) != 1 || loaded[0].ID != currentClientProfileID {
		t.Fatalf("clientProfiles() after invalidation = %#v, want current profile from disk", loaded)
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
