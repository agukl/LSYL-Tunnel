//go:build windows

package gui

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lsyltunnel/src/internal/passutil"
	frontassets "lsyltunnel/src/server/front"
	"lsyltunnel/src/server/tunnel"

	"golang.org/x/sys/windows/svc"
	"gopkg.in/yaml.v3"
)

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

type apiResult struct {
	OK      bool               `json:"ok"`
	Message string             `json:"message"`
	Issues  []adminConfigIssue `json:"issues,omitempty"`
	State   *adminState        `json:"state,omitempty"`
}

type passwordHashRequest struct {
	Password string `json:"password"`
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
	mux.HandleFunc("/api/security/permanent-block/delete", a.handleAdminDeletePermanentBlock)
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

func (a *App) handleAdminDeletePermanentBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResult{OK: false, Message: "method not allowed"})
		return
	}
	var req unblockIPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResult{OK: false, Message: "invalid delete request"})
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

	serviceRunning := a.isServerServiceRunning()
	removed := false
	liveSynced := false
	if serviceRunning && strings.TrimSpace(cfg.MonitorAddr) != "" {
		removed, err = deletePermanentBlockViaMonitor(cfg.MonitorAddr, ip)
		if err == nil {
			liveSynced = true
		}
	}
	if !liveSynced {
		removed, err = tunnel.RemovePermanentBlockedIP(a.runtimePermanentBlockPath(cfg), ip)
	}
	if err != nil {
		msg := friendlyError(err)
		writeJSON(w, http.StatusOK, apiResult{OK: false, Message: msg, State: a.buildAdminState(msg)})
		return
	}
	msg := "未找到永久封禁 IP: " + ip
	if removed {
		msg = "已删除永久封禁 IP: " + ip
		if !liveSynced {
			msg = "已从永久封禁文件删除 IP: " + ip + "，重启服务后完全生效"
		}
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
		return a.defaultRuntimePath("data", "server-state.json")
	}
	if !filepath.IsAbs(stateFile) {
		stateFile = filepath.Clean(filepath.Join(filepath.Dir(a.configPath), stateFile))
	}
	return stateFile
}

func (a *App) runtimePermanentBlockPath(cfg tunnel.Config) string {
	blockFile := strings.TrimSpace(cfg.Runtime.PermanentBlockFile)
	if blockFile == "" {
		return a.defaultRuntimePath("data", "server-permanent-block.txt")
	}
	if !filepath.IsAbs(blockFile) {
		blockFile = filepath.Clean(filepath.Join(filepath.Dir(a.configPath), blockFile))
	}
	return blockFile
}

func (a *App) defaultRuntimePath(category, name string) string {
	if isSourceServerConfigPath(a.configPath) {
		return filepath.Join(a.workspace, "runtime", category, name)
	}
	return filepath.Join(filepath.Dir(a.configPath), "..", category, name)
}

func isSourceServerConfigPath(path string) bool {
	dir := filepath.Clean(filepath.Dir(path))
	return strings.EqualFold(filepath.Base(dir), "conf") &&
		strings.EqualFold(filepath.Base(filepath.Dir(dir)), "server") &&
		strings.EqualFold(filepath.Base(filepath.Dir(filepath.Dir(dir))), "src")
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
