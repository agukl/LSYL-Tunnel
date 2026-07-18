//go:build windows

package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"lsyltunnel/src/client/tunnel"
)

const currentClientProfileID = "__current__"

const clientProfileCacheTTL = 30 * time.Second

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
	a.profileMu.Lock()
	defer a.profileMu.Unlock()

	if time.Now().Before(a.profileCacheUntil) {
		return append([]clientProfile(nil), a.profileCache...)
	}
	profiles := a.loadClientProfiles()
	a.profileCache = append(a.profileCache[:0], profiles...)
	a.profileCacheUntil = time.Now().Add(clientProfileCacheTTL)
	return append([]clientProfile(nil), profiles...)
}

func (a *App) loadClientProfiles() []clientProfile {
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
			profile.Message = clientProfileCompatibilityMessage(err)
		}
		certPath := archivedClientCertPath(a.configPath, suffix)
		if !fileExists(certPath) {
			profile.Available = false
			if profile.Message == "" {
				profile.Message = "缺少配套证书"
			}
		} else if err == nil {
			if err := tunnel.CheckConfigUpgradeCompatibleWithCACert(path, certPath); err != nil {
				profile.Available = false
				profile.Message = clientProfileCompatibilityMessage(err)
			}
		}
		profiles = append(profiles, profile)
	}
	return profiles
}

func (a *App) invalidateClientProfiles() {
	a.profileMu.Lock()
	a.profileCache = nil
	a.profileCacheUntil = time.Time{}
	a.profileMu.Unlock()
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
		profile.Message = clientProfileCompatibilityMessage(err)
	}
	certPath := currentClientCertPath(a.configPath)
	if !fileExists(certPath) {
		profile.Available = false
		if profile.Message == "" {
			profile.Message = "缺少当前证书"
		}
	} else if err == nil {
		if err := tunnel.CheckConfigUpgradeCompatible(a.configPath); err != nil {
			profile.Available = false
			profile.Message = clientProfileCompatibilityMessage(err)
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
	if err := tunnel.CheckConfigUpgradeCompatibleWithCACert(paths.TargetConfig, paths.TargetCert); err != nil {
		return fmt.Errorf("目标配置校验失败: %w", err)
	}
	if err := switchClientProfileFiles(paths); err != nil {
		return err
	}
	a.invalidateClientProfiles()
	return nil
}

func profileSwitchFriendlyError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if strings.Contains(text, "目标配置校验失败") {
		switch clientProfileCompatibilityMessage(err) {
		case "需要更高版本客户端":
			return "目标配置需要更高版本的客户端，请升级客户端后再切换。"
		case "配置结构不兼容":
			return "目标配置结构与当前客户端不兼容，请联系管理员重新下发配置。"
		case "配置与证书不匹配":
			return "目标配置与配套证书不匹配，请联系管理员重新下发配置。"
		case "配置文件或证书缺失":
			return "目标配置文件或配套证书不可读，请检查文件是否完整。"
		default:
			return "目标配置内容无效，请联系管理员检查转发规则和服务端地址。"
		}
	}
	switch {
	case containsAnyText(text, "requires a newer client", "requires version >="):
		return "目标配置需要更高版本的客户端，请升级客户端后再切换。"
	case containsAnyText(text, "请先断开"):
		return text
	case containsAnyText(text, "目标配置不存在", "目标配置缺少配套证书"):
		return text
	case containsAnyText(text, "服务端地址相同", "当前配置缺少服务端地址"):
		return text
	case containsAnyText(strings.ToLower(text), "access is denied", "拒绝访问", "permission"):
		return "没有权限切换配置文件，请以管理员身份运行或检查安装目录权限。"
	case containsAnyText(strings.ToLower(text), "读取当前配置失败") && containsAnyText(strings.ToLower(text), "no such file", "cannot find", "找不到"):
		return "当前配置文件不存在，请修复或重新安装客户端。"
	case containsAnyText(text, "读取当前配置失败") && containsAnyText(strings.ToLower(text), "yaml", "unmarshal", "配置文件格式"):
		return "当前配置文件格式不正确，请联系管理员检查配置。"
	case containsAnyText(text, "读取当前配置失败"):
		return "无法读取当前配置文件，请确认文件未被占用且内容完整。"
	case containsAnyText(text, "配置文件格式"):
		return "当前配置文件格式不正确，请联系管理员检查配置。"
	case containsAnyText(text, "目标文件已存在"):
		return "配置归档文件已存在，无法安全切换，请联系管理员清理重复配置。"
	case containsAnyText(text, "准备归档", "归档当前"):
		return "无法归档当前配置，请确认配置文件和证书未被其他程序占用。"
	case containsAnyText(text, "切换目标"):
		return "无法启用目标配置，请确认目标配置和证书未被其他程序占用。"
	default:
		return "切换客户端配置失败，未能完成配置文件替换；请重试，若持续出现请联系管理员。"
	}
}

func clientProfileCompatibilityMessage(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "requires a newer client"),
		strings.Contains(text, "current client supports config_version"),
		(strings.Contains(text, "client config requires version >=") && strings.Contains(text, "current version")):
		return "需要更高版本客户端"
	case errors.Is(err, os.ErrNotExist):
		return "配置文件或证书缺失"
	case containsAnyText(text, "access is denied", "permission denied", "拒绝访问"):
		return "配置文件不可读"
	case containsAnyText(text,
		"field ", "unknown field", "missing required field", "duplicate field",
		"must be a mapping", "must be a list", "must be a value", "exactly one yaml document",
		"yaml:", "cannot unmarshal", "did not find expected"):
		return "配置结构不兼容"
	case containsAnyText(text, "certificate", "x509", "no pem", "doesn't contain any ip sans"):
		return "配置与证书不匹配"
	default:
		return "配置内容不可用"
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
