//go:build windows

package gui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lsyltunnel/src/internal/passutil"
	frontassets "lsyltunnel/src/server/front"
	"lsyltunnel/src/server/tunnel"

	"golang.org/x/sys/windows/svc"
	"gopkg.in/yaml.v3"
)

const serverLocalForwardHost = "127.0.0.1"

type adminState struct {
	OK              bool                      `json:"ok"`
	Message         string                    `json:"message"`
	Paths           adminPaths                `json:"paths"`
	Service         adminService              `json:"service"`
	Monitor         *monitorStatus            `json:"monitor,omitempty"`
	MonitorErr      string                    `json:"monitor_error,omitempty"`
	Config          adminConfig               `json:"config"`
	Validation      string                    `json:"validation"`
	ConfigWritable  bool                      `json:"config_writable"`
	ConfigWriteHint string                    `json:"config_write_hint,omitempty"`
	BusinessLogs    []tunnel.BusinessLogEntry `json:"business_logs,omitempty"`
	RequestLogs     []tunnel.RequestLogEntry  `json:"request_logs,omitempty"`
	PermanentBlocks []adminPermanentBlock     `json:"permanent_blocked_ips,omitempty"`
}

type adminPaths struct {
	Workspace string `json:"workspace"`
	Config    string `json:"config"`
	Service   string `json:"service"`
	Front     string `json:"front"`
}

type adminService struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Installed bool   `json:"installed"`
	Active    bool   `json:"active"`
	Running   bool   `json:"running"`
}

type adminPermanentBlock struct {
	IP     string `json:"ip"`
	Source string `json:"source,omitempty"`
	Line   int    `json:"line,omitempty"`
}

type monitorStatus struct {
	Service        string                  `json:"service"`
	ListenAddr     string                  `json:"listen_addr"`
	UptimeSec      int64                   `json:"uptime_sec"`
	ActiveStreams  int64                   `json:"active_streams"`
	TotalStreams   int64                   `json:"total_streams"`
	AuthOK         int64                   `json:"auth_ok"`
	AuthFailed     int64                   `json:"auth_failed"`
	PolicyRejected int64                   `json:"policy_rejected"`
	DialFailed     int64                   `json:"dial_failed"`
	BytesUp        int64                   `json:"bytes_up"`
	BytesDown      int64                   `json:"bytes_down"`
	BlockedIPs     []tunnel.BlockedIPState `json:"blocked_ips,omitempty"`
	RecentEvents   []tunnel.RuntimeEvent   `json:"recent_events,omitempty"`
}

type adminConfig struct {
	ListenAddr     string          `json:"listen_addr"`
	MonitorAddr    string          `json:"monitor_addr"`
	LogLevel       string          `json:"log_level"`
	TLS            adminTLS        `json:"tls"`
	Users          []adminUser     `json:"users"`
	Forwards       []adminForward  `json:"forwards"`
	Security       adminSecurity   `json:"security"`
	CredentialSeal adminCredential `json:"credential_seal"`
}

type adminTLS struct {
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	MinVersion string `json:"min_version"`
}

type adminUser struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Disabled     bool   `json:"disabled"`
}

type adminForward struct {
	Name         string   `json:"name"`
	Direction    string   `json:"direction"`
	Port         string   `json:"port"`
	ListenAddr   string   `json:"listen_addr"`
	ServerTarget string   `json:"server_target"`
	Owner        string   `json:"owner"`
	AllowedUsers []string `json:"allowed_users,omitempty"`
}

type adminSecurity struct {
	HandshakeTimeoutSec int `json:"handshake_timeout_sec"`
	DialTimeoutSec      int `json:"dial_timeout_sec"`
	MaxHandshakeBytes   int `json:"max_handshake_bytes"`
	AuthFailWindowSec   int `json:"auth_fail_window_sec"`
	AuthFailThreshold   int `json:"auth_fail_threshold"`
	AuthFailBlockSec    int `json:"auth_fail_block_sec"`
}

type adminCredential struct {
	KeyID          string `json:"key_id"`
	PrivateKeyFile string `json:"private_key_file"`
	PublicKeyFile  string `json:"public_key_file"`
	ExpiresAt      string `json:"expires_at"`
	Active         bool   `json:"active"`
}

type apiResult struct {
	OK      bool               `json:"ok"`
	Message string             `json:"message"`
	Issues  []adminConfigIssue `json:"issues,omitempty"`
	State   *adminState        `json:"state,omitempty"`
}

type passwordHashRequest struct {
	Password string `json:"password"`
}

type unblockIPRequest struct {
	IP string `json:"ip"`
}

type passwordHashResult struct {
	OK           bool   `json:"ok"`
	Message      string `json:"message"`
	PasswordHash string `json:"password_hash,omitempty"`
}

func (a *App) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/state", a.handleAdminState)
	mux.HandleFunc("/api/config", a.handleAdminConfig)
	mux.HandleFunc("/api/password/hash", a.handleAdminPasswordHash)
	mux.HandleFunc("/api/service/restart", a.handleAdminServiceRestart)
	mux.HandleFunc("/api/security/unblock", a.handleAdminUnblockIP)
	mux.HandleFunc("/api/log-analysis", a.handleAdminLogAnalysis)
	mux.Handle("/", noStore(http.FileServer(http.FS(frontassets.FS))))
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func (a *App) frontDir() string {
	return filepath.Join(a.workspace, "src", "server", "front")
}

func (a *App) handleAdminState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResult{OK: false, Message: "请求方法不支持"})
		return
	}
	state := a.buildAdminState("")
	writeJSON(w, http.StatusOK, state)
}

func (a *App) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResult{OK: false, Message: "请求方法不支持"})
		return
	}
	var form adminConfig
	if err := json.NewDecoder(r.Body).Decode(&form); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResult{OK: false, Message: "配置请求格式不正确"})
		return
	}
	existing, err := readRawServerConfig(a.configPath)
	if err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusOK, apiResult{OK: false, Message: friendlyError(err), State: a.buildAdminState("failed to read config")})
		return
	}
	if issues := validateAdminForwardsForSaveWithOptions(form, adminForwardValidationOptions{
		Existing:                       existing,
		ServiceRunning:                 a.isServerServiceRunning(),
		CheckForwardTargetReachability: false,
		CheckPassivePortAvailability:   true,
	}); issues.hasErrors() {
		writeJSON(w, http.StatusOK, apiResult{OK: false, Message: issues.summary(), Issues: issues, State: a.buildAdminState("config validation failed")})
		return
	}
	cfg, err := adminConfigToTunnel(form, existing)
	if err != nil {
		writeJSON(w, http.StatusOK, apiResult{OK: false, Message: friendlyError(err), State: a.buildAdminState("config validation failed")})
		return
	}
	if writable, hint := configWriteStatus(a.configPath); !writable {
		if hint == "" {
			hint = "配置文件不可写，请以管理员身份运行或检查安装目录权限"
		}
		writeJSON(w, http.StatusOK, apiResult{OK: false, Message: hint, State: a.buildAdminState(hint)})
		return
	}
	if err := tunnel.SaveConfig(a.configPath, cfg); err != nil {
		writeJSON(w, http.StatusOK, apiResult{OK: false, Message: friendlyError(err), State: a.buildAdminState("failed to save config")})
		return
	}
	a.appendLog("配置已保存: " + a.configPath)
	state := a.buildAdminState("配置已保存，重启服务后生效")
	writeJSON(w, http.StatusOK, apiResult{OK: true, Message: "配置已保存，重启服务后生效", State: state})
}

func (a *App) handleAdminPasswordHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, passwordHashResult{OK: false, Message: "请求方法不支持"})
		return
	}
	var req passwordHashRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, passwordHashResult{OK: false, Message: "密码请求格式不正确"})
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		writeJSON(w, http.StatusOK, passwordHashResult{OK: false, Message: "请输入新密码"})
		return
	}
	hash, err := passutil.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusOK, passwordHashResult{OK: false, Message: "密码生成失败，请重试"})
		return
	}
	writeJSON(w, http.StatusOK, passwordHashResult{OK: true, Message: "密码已重置，请保存配置后生效", PasswordHash: hash})
}

func (a *App) handleAdminUnblockIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResult{OK: false, Message: "method not allowed"})
		return
	}
	var req unblockIPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResult{OK: false, Message: "invalid unblock request"})
		return
	}
	ip := strings.TrimSpace(req.IP)
	if net.ParseIP(ip) == nil {
		writeJSON(w, http.StatusOK, apiResult{OK: false, Message: "IP 格式不正确", State: a.buildAdminState("IP 格式不正确")})
		return
	}
	cfg, err := readRawServerConfig(a.configPath)
	if err != nil {
		msg := friendlyError(err)
		writeJSON(w, http.StatusOK, apiResult{OK: false, Message: msg, State: a.buildAdminState(msg)})
		return
	}

	removed := false
	if a.isServerServiceRunning() {
		if strings.TrimSpace(cfg.MonitorAddr) == "" {
			msg := "服务运行中但 monitor_addr 为空，无法实时解封"
			writeJSON(w, http.StatusOK, apiResult{OK: false, Message: msg, State: a.buildAdminState(msg)})
			return
		}
		removed, err = unblockIPViaMonitor(cfg.MonitorAddr, ip)
	} else {
		removed, err = tunnel.UnblockBlockedIP(a.runtimeStatePath(cfg), ip)
	}
	if err != nil {
		msg := friendlyError(err)
		writeJSON(w, http.StatusOK, apiResult{OK: false, Message: msg, State: a.buildAdminState(msg)})
		return
	}
	msg := "未找到封禁 IP: " + ip
	if removed {
		msg = "已解封 IP: " + ip
	}
	writeJSON(w, http.StatusOK, apiResult{OK: true, Message: msg, State: a.buildAdminState(msg)})
}

func (a *App) handleAdminServiceRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResult{OK: false, Message: "请求方法不支持"})
		return
	}
	if err := a.restartServerService(); err != nil {
		msg := friendlyError(err)
		a.appendLog("服务重启失败: " + msg)
		writeJSON(w, http.StatusOK, apiResult{OK: false, Message: msg, State: a.buildAdminState("service restart failed")})
		return
	}
	a.appendLog("服务已重启")
	writeJSON(w, http.StatusOK, apiResult{OK: true, Message: "服务已重启", State: a.buildAdminState("服务已重启")})
}
func (a *App) buildAdminState(message string) *adminState {
	cfg, cfgErr := readRawServerConfig(a.configPath)
	if cfgErr != nil {
		cfg = tunnel.Config{}
		tunnel.ApplyDefaults(&cfg)
	}
	svcState, installed, svcErr := serverServiceState(a.serviceName)
	serviceActive := installed && isServiceActive(svcState)
	service := adminService{Name: a.serviceName, Status: serviceStatusText(svcState, installed, svcErr), Installed: installed, Active: serviceActive, Running: installed && svcState == svc.Running}
	configWritable, configWriteHint := configWriteStatus(a.configPath)

	validation := ""
	if cfgErr != nil {
		validation = friendlyError(cfgErr)
	}

	var monitor *monitorStatus
	monitorMessage := ""
	if service.Running {
		var monitorErr error
		monitor, monitorErr = loadMonitorStatus(cfg.MonitorAddr)
		if monitorErr != nil && strings.TrimSpace(cfg.MonitorAddr) != "" {
			monitorMessage = friendlyError(monitorErr)
		}
	}
	if monitor == nil {
		if blocked := a.loadPersistedBlockedIPs(cfg); len(blocked) > 0 {
			monitor = &monitorStatus{BlockedIPs: blocked}
		}
	}

	ok := cfgErr == nil && validation == ""
	if message == "" {
		if ok {
			message = "admin console ready"
		} else {
			message = validation
		}
	}
	return &adminState{
		OK:      ok,
		Message: message,
		Paths: adminPaths{
			Workspace: a.workspace,
			Config:    a.configPath,
			Service:   a.serviceExe,
			Front:     a.frontDir(),
		},
		Service:         service,
		Monitor:         monitor,
		MonitorErr:      monitorMessage,
		Config:          adminConfigFromTunnel(cfg),
		Validation:      validation,
		ConfigWritable:  configWritable,
		ConfigWriteHint: configWriteHint,
		BusinessLogs:    a.loadBusinessLogs(cfg, 120),
		RequestLogs:     a.loadRequestLogs(cfg, 120),
		PermanentBlocks: a.loadPermanentBlockedIPs(cfg),
	}
}

func configWriteStatus(path string) (bool, string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, "配置文件路径为空"
	}
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return false, "无法访问配置目录: " + friendlyError(err)
		}
		parent := nearestExistingParent(dir)
		if parent == "" {
			return false, "配置目录不存在，且无法找到可写的上级目录"
		}
		dir = parent
	} else if !info.IsDir() {
		return false, "配置文件所在路径不是目录"
	}

	probe := filepath.Join(dir, fmt.Sprintf(".lsyl-write-test-%d.tmp", os.Getpid()))
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if isPermissionError(err) {
			return false, "配置文件不可写，请以管理员身份运行或检查安装目录权限"
		}
		return false, "无法写入配置目录: " + friendlyError(err)
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return true, ""
}
func nearestExistingParent(path string) string {
	cur := filepath.Clean(path)
	for {
		if info, err := os.Stat(cur); err == nil && info.IsDir() {
			return cur
		}
		next := filepath.Dir(cur)
		if next == cur {
			return ""
		}
		cur = next
	}
}

func isPermissionError(err error) bool {
	if os.IsPermission(err) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "access is denied") || strings.Contains(err.Error(), "拒绝访问")
}

func readRawServerConfig(path string) (tunnel.Config, error) {
	var cfg tunnel.Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	tunnel.ApplyDefaults(&cfg)
	return cfg, nil
}

func (a *App) loadPersistedBlockedIPs(cfg tunnel.Config) []tunnel.BlockedIPState {
	blocked, err := tunnel.LoadBlockedIPs(a.runtimeStatePath(cfg))
	if err != nil {
		return nil
	}
	return blocked
}

func (a *App) runtimeStatePath(cfg tunnel.Config) string {
	stateFile := strings.TrimSpace(cfg.Runtime.StateFile)
	if stateFile == "" {
		stateFile = filepath.Join("..", "data", "server-state.json")
	}
	if !filepath.IsAbs(stateFile) {
		stateFile = filepath.Clean(filepath.Join(filepath.Dir(a.configPath), stateFile))
	}
	return stateFile
}

func (a *App) runtimePermanentBlockPath(cfg tunnel.Config) string {
	blockFile := strings.TrimSpace(cfg.Runtime.PermanentBlockFile)
	if blockFile == "" {
		blockFile = filepath.Join("..", "data", "server-permanent-block.txt")
	}
	if !filepath.IsAbs(blockFile) {
		blockFile = filepath.Clean(filepath.Join(filepath.Dir(a.configPath), blockFile))
	}
	return blockFile
}

func adminConfigFromTunnel(cfg tunnel.Config) adminConfig {
	tunnel.ApplyDefaults(&cfg)
	users := make([]adminUser, 0, len(cfg.Auth.Users))
	for _, user := range cfg.Auth.Users {
		users = append(users, adminUser{
			Username:     user.Username,
			PasswordHash: user.PasswordHash,
			Disabled:     user.Disabled,
		})
	}
	forwards := make([]adminForward, 0, len(cfg.Forwards))
	for _, fwd := range cfg.Forwards {
		allowedUsers := cleanAllowedUsers(fwd.AllowedUsers)
		forwards = append(forwards, adminForward{
			Name:         fwd.Name,
			Direction:    forwardDirectionOrDefault(fwd.Direction),
			Port:         forwardPortText(fwd),
			ListenAddr:   fwd.ListenAddr,
			ServerTarget: fwd.ServerTarget,
			Owner:        firstAllowedUser(allowedUsers),
			AllowedUsers: allowedUsers,
		})
	}
	return adminConfig{
		ListenAddr:  cfg.ListenAddr,
		MonitorAddr: cfg.MonitorAddr,
		LogLevel:    cfg.LogLevel,
		TLS: adminTLS{
			CertFile:   cfg.TLS.CertFile,
			KeyFile:    cfg.TLS.KeyFile,
			MinVersion: cfg.TLS.MinVersion,
		},
		Users:    users,
		Forwards: forwards,
		Security: adminSecurity{
			HandshakeTimeoutSec: cfg.Security.HandshakeTimeoutSec,
			DialTimeoutSec:      cfg.Security.DialTimeoutSec,
			MaxHandshakeBytes:   cfg.Security.MaxHandshakeBytes,
			AuthFailWindowSec:   cfg.Security.AuthFailWindowSec,
			AuthFailThreshold:   cfg.Security.AuthFailThreshold,
			AuthFailBlockSec:    cfg.Security.AuthFailBlockSec,
		},
		CredentialSeal: activeCredential(cfg.CredentialSeal),
	}
}

func adminConfigToTunnel(form adminConfig, existing tunnel.Config) (tunnel.Config, error) {
	cfg := existing
	cfg.ListenAddr = strings.TrimSpace(form.ListenAddr)
	cfg.MonitorAddr = strings.TrimSpace(form.MonitorAddr)
	cfg.LogLevel = strings.TrimSpace(form.LogLevel)
	cfg.TLS = tunnel.TLSConfig{
		CertFile:   strings.TrimSpace(form.TLS.CertFile),
		KeyFile:    strings.TrimSpace(form.TLS.KeyFile),
		MinVersion: strings.TrimSpace(form.TLS.MinVersion),
	}
	cfg.Auth.Users = make([]tunnel.UserConfig, 0, len(form.Users))
	for _, item := range form.Users {
		username := strings.TrimSpace(item.Username)
		if username == "" && strings.TrimSpace(item.PasswordHash) == "" {
			continue
		}
		cfg.Auth.Users = append(cfg.Auth.Users, tunnel.UserConfig{
			Username:     username,
			PasswordHash: strings.TrimSpace(item.PasswordHash),
			Disabled:     item.Disabled,
		})
	}
	cfg.Forwards = make([]tunnel.ForwardConfig, 0, len(form.Forwards))
	for _, item := range form.Forwards {
		fwd, empty, err := adminForwardToTunnel(item)
		if err != nil {
			return cfg, err
		}
		if empty {
			continue
		}
		fwd.AllowedUsers = cleanAllowedUsers(append(item.AllowedUsers, item.Owner))
		cfg.Forwards = append(cfg.Forwards, fwd)
	}
	cfg.Security = tunnel.SecurityConfig{
		HandshakeTimeoutSec: form.Security.HandshakeTimeoutSec,
		DialTimeoutSec:      form.Security.DialTimeoutSec,
		MaxHandshakeBytes:   form.Security.MaxHandshakeBytes,
		AuthFailWindowSec:   form.Security.AuthFailWindowSec,
		AuthFailThreshold:   form.Security.AuthFailThreshold,
		AuthFailBlockSec:    form.Security.AuthFailBlockSec,
	}
	credentialSeal, err := credentialSealFromAdmin(form.CredentialSeal, existing.CredentialSeal)
	if err != nil {
		return cfg, err
	}
	cfg.CredentialSeal = credentialSeal
	tunnel.ApplyDefaults(&cfg)
	return cfg, nil
}

func credentialSealFromAdmin(form adminCredential, existing tunnel.CredentialSealConfig) (tunnel.CredentialSealConfig, error) {
	keyID := strings.TrimSpace(form.KeyID)
	privateFile := strings.TrimSpace(form.PrivateKeyFile)
	publicFile := strings.TrimSpace(form.PublicKeyFile)
	expiresAt := strings.TrimSpace(form.ExpiresAt)
	if keyID == "" && privateFile == "" && publicFile == "" && expiresAt == "" {
		return tunnel.CredentialSealConfig{}, nil
	}
	if keyID == "" || privateFile == "" || publicFile == "" || expiresAt == "" {
		return tunnel.CredentialSealConfig{}, fmt.Errorf("密封凭据配置不完整，请填写密钥ID、私钥、公钥和过期时间")
	}
	if _, err := time.Parse(time.RFC3339, expiresAt); err != nil {
		return tunnel.CredentialSealConfig{}, fmt.Errorf("密封凭据过期时间格式不正确，请使用 RFC3339")
	}
	keys := make([]tunnel.CredentialSealKeyConfig, 0, len(existing.Keys)+1)
	for _, key := range existing.Keys {
		if strings.TrimSpace(key.KeyID) == keyID {
			continue
		}
		key.Active = false
		keys = append(keys, key)
	}
	keys = append(keys, tunnel.CredentialSealKeyConfig{
		KeyID:          keyID,
		PrivateKeyFile: privateFile,
		PublicKeyFile:  publicFile,
		ExpiresAt:      expiresAt,
		Active:         true,
	})
	return tunnel.CredentialSealConfig{Keys: keys}, nil
}
func activeCredential(cfg tunnel.CredentialSealConfig) adminCredential {
	if len(cfg.Keys) == 0 {
		return adminCredential{}
	}
	selected := cfg.Keys[0]
	for _, item := range cfg.Keys {
		if item.Active {
			selected = item
			break
		}
	}
	return adminCredential{
		KeyID:          selected.KeyID,
		PrivateKeyFile: selected.PrivateKeyFile,
		PublicKeyFile:  selected.PublicKeyFile,
		ExpiresAt:      selected.ExpiresAt,
		Active:         selected.Active,
	}
}

func loadMonitorStatus(addr string) (*monitorStatus, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, nil
	}
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Get("http://" + addr + "/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("monitor returned %s", resp.Status)
	}
	var status monitorStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

func unblockIPViaMonitor(addr, ip string) (bool, error) {
	addr = strings.TrimSpace(addr)
	ip = strings.TrimSpace(ip)
	if addr == "" {
		return false, fmt.Errorf("monitor address is empty")
	}
	body, err := json.Marshal(unblockIPRequest{IP: ip})
	if err != nil {
		return false, err
	}
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Post("http://"+addr+"/security/unblock", "application/json; charset=utf-8", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var result struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
		Removed bool   `json:"removed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !result.OK {
		if result.Message == "" {
			result.Message = fmt.Sprintf("monitor returned %s", resp.Status)
		}
		return false, fmt.Errorf("%s", result.Message)
	}
	return result.Removed, nil
}

func serviceStatusText(state svc.State, installed bool, err error) string {
	if err != nil {
		return "状态读取失败"
	}
	if !installed {
		return "未安装"
	}
	switch state {
	case svc.Stopped:
		return "已停止"
	case svc.StartPending:
		return "启动中"
	case svc.StopPending:
		return "停止中"
	case svc.Running:
		return "运行中"
	case svc.ContinuePending:
		return "继续中"
	case svc.PausePending:
		return "暂停中"
	case svc.Paused:
		return "已暂停"
	default:
		return "未知状态"
	}
}
func splitList(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		item := strings.TrimSpace(field)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func cleanAllowedUsers(items []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		for _, user := range splitList(item) {
			if seen[user] {
				continue
			}
			seen[user] = true
			out = append(out, user)
		}
	}
	return out
}

func firstAllowedUser(users []string) string {
	for _, user := range users {
		user = strings.TrimSpace(user)
		if user != "" {
			return user
		}
	}
	return ""
}

func forwardDirectionOrDefault(direction string) string {
	direction = strings.TrimSpace(direction)
	if direction == tunnel.DirectionServerToClient {
		return tunnel.DirectionServerToClient
	}
	return tunnel.DirectionClientToServer
}

func adminForwardToTunnel(item adminForward) (tunnel.ForwardConfig, bool, error) {
	name := strings.TrimSpace(item.Name)
	direction := forwardDirectionOrDefault(item.Direction)
	portText := strings.TrimSpace(item.Port)
	if portText == "" {
		portText = forwardPortText(tunnel.ForwardConfig{
			Direction:    direction,
			ListenAddr:   strings.TrimSpace(item.ListenAddr),
			ServerTarget: strings.TrimSpace(item.ServerTarget),
		})
	}
	if name == "" && portText == "" && strings.TrimSpace(item.ListenAddr) == "" && strings.TrimSpace(item.ServerTarget) == "" {
		return tunnel.ForwardConfig{}, true, nil
	}
	if _, err := parseForwardPort(portText); err != nil {
		return tunnel.ForwardConfig{}, false, fmt.Errorf("forward %q port is invalid", forwardDisplayName(item))
	}
	addr := net.JoinHostPort(serverLocalForwardHost, portText)
	fwd := tunnel.ForwardConfig{Name: name, Direction: direction}
	if direction == tunnel.DirectionServerToClient {
		fwd.ListenAddr = addr
	} else {
		fwd.ServerTarget = addr
	}
	return fwd, false, nil
}

func forwardPortText(fwd tunnel.ForwardConfig) string {
	target := strings.TrimSpace(fwd.ServerTarget)
	if forwardDirectionOrDefault(fwd.Direction) == tunnel.DirectionServerToClient {
		target = strings.TrimSpace(fwd.ListenAddr)
	}
	if target == "" {
		return ""
	}
	_, portText, err := net.SplitHostPort(target)
	if err != nil {
		return ""
	}
	return portText
}

func parseForwardPort(portText string) (int, error) {
	if portText == "" {
		return 0, fmt.Errorf("empty port")
	}
	for _, ch := range portText {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("port must be numeric")
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("port out of range")
	}
	return port, nil
}

func forwardDisplayName(fwd adminForward) string {
	if name := strings.TrimSpace(fwd.Name); name != "" {
		return name
	}
	if addr := strings.TrimSpace(fwd.ListenAddr); addr != "" {
		return addr
	}
	if target := strings.TrimSpace(fwd.ServerTarget); target != "" {
		return target
	}
	return "未命名转发"
}

func serviceLogTail(path string, max int) []string {
	for _, candidate := range serviceLogTailCandidates(path) {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		return lastLines(data, max)
	}
	return nil
}

func serviceLogTailCandidates(path string) []string {
	date := time.Now().Format("2006-01-02")
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)
	daily := filepath.Join(filepath.Dir(path), name+"-"+date+ext)
	if daily == path {
		return []string{path}
	}
	return []string{daily, path}
}

func lastLines(data []byte, max int) []string {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	kept := make([]string, 0, max)
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		kept = append(kept, line)
		if len(kept) > max {
			kept = kept[len(kept)-max:]
		}
	}
	return kept
}

func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "access is denied") || strings.Contains(text, "拒绝访问"):
		return "没有权限执行该操作，请以管理员身份运行管理台或检查目录权限"
	case strings.Contains(lower, "global access grants"):
		return "全局访问授权已废弃，请在“转发端口”页面按用户配置放通端口"
	case strings.Contains(lower, "duplicate auth user"):
		return "用户名重复，请保留唯一用户"
	case strings.Contains(lower, "password_hash is required"):
		return "用户缺少密码哈希，请重置密码后保存"
	case strings.Contains(lower, "port is invalid"):
		return "端口格式不正确，请填写 1-65535 的数字"
	case strings.Contains(lower, "listen_addr must be a server-local loopback address"):
		return "反向代理只能监听服务端本机回环地址，例如 127.0.0.1:18080"
	case strings.Contains(lower, "cert_file") || strings.Contains(lower, "key_file") || strings.Contains(lower, "tls identity"):
		return "TLS 证书或私钥配置不正确，请检查证书文件路径"
	case strings.Contains(lower, "actively refused") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "connectex"):
		return "目标端口不可访问，请确认目标服务已启动且地址端口正确"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
		return "连接超时，请检查网络、防火墙或目标服务状态"
	case os.IsNotExist(err):
		return "文件不存在，请检查配置、证书或程序路径"
	default:
		return text
	}
}

func displayName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "未命名"
	}
	return name
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
