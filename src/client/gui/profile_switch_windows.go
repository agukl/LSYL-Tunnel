//go:build windows

package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"lsyltunnel/src/client/tunnel"
)

const currentClientProfileID = "__current__"

type clientProfile struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	ServerAddr string `json:"server_addr"`
	Current    bool   `json:"current"`
	Available  bool   `json:"available"`
	Message    string `json:"message,omitempty"`
}

type profileSwitchPaths struct {
	CurrentConfig string
	CurrentCert   string
	ArchiveConfig string
	ArchiveCert   string
	TargetConfig  string
	TargetCert    string
}

func (a *App) clientProfiles() []clientProfile {
	confDir := filepath.Dir(a.configPath)
	profiles := []clientProfile{a.currentClientProfile()}
	entries, err := filepath.Glob(filepath.Join(confDir, "client.*.yaml"))
	if err != nil {
		return profiles
	}
	sort.Strings(entries)
	for _, path := range entries {
		suffix, ok := archivedClientConfigSuffix(path)
		if !ok {
			continue
		}
		cfg, err := readClientConfigRaw(path)
		serverAddr := strings.TrimSpace(cfg.ServerAddr)
		label := serverAddr
		if label == "" {
			label = suffix
		}
		profile := clientProfile{
			ID:         suffix,
			Label:      label,
			ServerAddr: serverAddr,
			Available:  true,
		}
		if err != nil {
			profile.Available = false
			profile.Message = "配置文件格式不正确"
		}
		if err == nil {
			if err := tunnel.CheckConfigUpgradeCompatible(path); err != nil {
				profile.Available = false
				profile.Message = "config version is not compatible with this client"
			}
		}
		if !fileExists(archivedClientCertPath(a.configPath, suffix)) {
			profile.Available = false
			if profile.Message == "" {
				profile.Message = "缺少配套证书"
			}
		}
		profiles = append(profiles, profile)
	}
	return profiles
}

func (a *App) currentClientProfile() clientProfile {
	cfg, err := readClientConfigRaw(a.configPath)
	serverAddr := strings.TrimSpace(cfg.ServerAddr)
	label := serverAddr
	if label == "" {
		label = "当前配置"
	}
	profile := clientProfile{
		ID:         currentClientProfileID,
		Label:      label,
		ServerAddr: serverAddr,
		Current:    true,
		Available:  true,
	}
	if err != nil {
		profile.Available = false
		profile.Message = "配置文件不可读"
	}
	if err == nil {
		if err := tunnel.CheckConfigUpgradeCompatible(a.configPath); err != nil {
			profile.Available = false
			profile.Message = "config version is not compatible with this client"
		}
	}
	if !fileExists(currentClientCertPath(a.configPath)) {
		profile.Available = false
		if profile.Message == "" {
			profile.Message = "缺少当前证书"
		}
	}
	return profile
}

func (a *App) switchClientProfile(profileID string) error {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" || profileID == currentClientProfileID {
		return nil
	}
	currentCfg, err := readClientConfigRaw(a.configPath)
	if err != nil {
		return fmt.Errorf("读取当前配置失败: %w", err)
	}
	currentSuffix := clientProfileFileSuffix(currentCfg.ServerAddr)
	if currentSuffix == "" {
		return fmt.Errorf("当前配置缺少服务端地址，无法归档")
	}
	if strings.EqualFold(currentSuffix, profileID) {
		return fmt.Errorf("当前配置和目标配置服务地址相同，无法安全切换")
	}
	paths := profileSwitchPaths{
		CurrentConfig: a.configPath,
		CurrentCert:   currentClientCertPath(a.configPath),
		ArchiveConfig: archivedClientConfigPath(a.configPath, currentSuffix),
		ArchiveCert:   archivedClientCertPath(a.configPath, currentSuffix),
		TargetConfig:  archivedClientConfigPath(a.configPath, profileID),
		TargetCert:    archivedClientCertPath(a.configPath, profileID),
	}
	if !fileExists(paths.TargetConfig) {
		return fmt.Errorf("目标配置不存在")
	}
	if !fileExists(paths.TargetCert) {
		return fmt.Errorf("目标配置缺少配套证书")
	}
	if err := tunnel.CheckConfigUpgradeCompatible(paths.TargetConfig); err != nil {
		return fmt.Errorf("target config is not compatible with this client: %w", err)
	}
	return switchClientProfileFiles(paths)
}

func profileSwitchFriendlyError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	switch {
	case containsAnyText(text, "not compatible with this client", "requires a newer client"):
		return "target config is not compatible with this client"
	case containsAnyText(text, "请先断开"):
		return text
	case containsAnyText(text, "目标配置不存在", "目标配置缺少配套证书"):
		return text
	case containsAnyText(text, "服务端地址相同"):
		return text
	case containsAnyText(text, "读取当前配置失败", "配置文件格式"):
		return "当前配置文件格式不正确，请联系管理员检查配置。"
	case containsAnyText(strings.ToLower(text), "access is denied", "拒绝访问", "permission"):
		return "没有权限切换配置文件，请以管理员身份运行或检查安装目录权限。"
	default:
		return "切换客户端配置失败，请检查配置文件和证书是否完整。"
	}
}

func currentClientCertPath(configPath string) string {
	return filepath.Clean(filepath.Join(filepath.Dir(configPath), "..", "cert", "server.crt"))
}

func archivedClientConfigPath(configPath, suffix string) string {
	return filepath.Join(filepath.Dir(configPath), "client."+suffix+".yaml")
}

func archivedClientCertPath(configPath, suffix string) string {
	return filepath.Join(filepath.Dir(currentClientCertPath(configPath)), "server."+suffix+".crt")
}

func archivedClientConfigSuffix(path string) (string, bool) {
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "client.") || !strings.HasSuffix(base, ".yaml") {
		return "", false
	}
	suffix := strings.TrimSuffix(strings.TrimPrefix(base, "client."), ".yaml")
	if suffix == "" || strings.EqualFold(suffix, "bak") {
		return "", false
	}
	return suffix, true
}

func clientProfileFileSuffix(serverAddr string) string {
	serverAddr = strings.TrimSpace(serverAddr)
	if serverAddr == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range serverAddr {
		ok := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '-' || r == '_'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "._-")
	if len(out) > 120 {
		out = strings.Trim(out[:120], "._-")
	}
	if out == "" {
		return "unknown"
	}
	return out
}

func switchClientProfileFiles(paths profileSwitchPaths) error {
	var moved []renamePair
	var backups []backupPair
	rollback := func() {
		for i := len(moved) - 1; i >= 0; i-- {
			_ = os.Rename(moved[i].to, moved[i].from)
		}
		for i := len(backups) - 1; i >= 0; i-- {
			_ = os.Rename(backups[i].backup, backups[i].original)
		}
	}
	cleanup := func() {
		for _, backup := range backups {
			_ = os.Remove(backup.backup)
		}
	}
	addBackup := func(path string) error {
		backup, ok, err := moveExistingAside(path)
		if err != nil {
			return err
		}
		if ok {
			backups = append(backups, backupPair{original: path, backup: backup})
		}
		return nil
	}
	move := func(from, to string, optional bool) error {
		if optional && !fileExists(from) {
			return nil
		}
		if err := renameNoReplace(from, to); err != nil {
			return err
		}
		moved = append(moved, renamePair{from: from, to: to})
		return nil
	}

	currentCertExists := fileExists(paths.CurrentCert)
	if err := addBackup(paths.ArchiveConfig); err != nil {
		return fmt.Errorf("准备归档配置失败: %w", err)
	}
	if currentCertExists {
		if err := addBackup(paths.ArchiveCert); err != nil {
			rollback()
			return fmt.Errorf("准备归档证书失败: %w", err)
		}
	}
	if err := move(paths.CurrentConfig, paths.ArchiveConfig, false); err != nil {
		rollback()
		return fmt.Errorf("归档当前配置失败: %w", err)
	}
	if err := move(paths.CurrentCert, paths.ArchiveCert, !currentCertExists); err != nil {
		rollback()
		return fmt.Errorf("归档当前证书失败: %w", err)
	}
	if err := move(paths.TargetConfig, paths.CurrentConfig, false); err != nil {
		rollback()
		return fmt.Errorf("切换目标配置失败: %w", err)
	}
	if err := move(paths.TargetCert, paths.CurrentCert, false); err != nil {
		rollback()
		return fmt.Errorf("切换目标证书失败: %w", err)
	}
	cleanup()
	return nil
}

type renamePair struct {
	from string
	to   string
}

type backupPair struct {
	original string
	backup   string
}

func moveExistingAside(path string) (string, bool, error) {
	if !fileExists(path) {
		return "", false, nil
	}
	backup, err := temporarySiblingPath(path)
	if err != nil {
		return "", false, err
	}
	if err := os.Rename(path, backup); err != nil {
		return "", false, err
	}
	return backup, true, nil
}

func renameNoReplace(from, to string) error {
	if fileExists(to) {
		return fmt.Errorf("目标文件已存在: %s", to)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.Rename(from, to)
}

func temporarySiblingPath(path string) (string, error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	for i := 0; i < 20; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf(".%s.lsyl-switch-%d-%d.tmp", base, time.Now().UnixNano(), i))
		if !fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("无法创建临时文件名: %s", path)
}
