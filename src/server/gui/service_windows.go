//go:build windows

package gui

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"lsyltunnel/src/server/tunnel"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serverServiceName        = "LSYLTunnelServer"
	serverServiceDisplayName = "LSYL Tunnel Server"
	serverServiceDescription = "LSYL Tunnel 服务端：提供账号认证的 TLS 加密隧道和端口转发，日志写入安装目录 logs。"
)

type serviceActionOptions struct {
	Action      string
	ConfigPath  string
	ServiceExe  string
	ServiceName string
	LogPath     string
	ResultFile  string
	Workspace   string
	StartType   string
}

func parseServiceActionArgs(args []string) (serviceActionOptions, bool, error) {
	hasServiceAction := false
	for _, arg := range args {
		if arg == "-service-action" || strings.HasPrefix(arg, "-service-action=") {
			hasServiceAction = true
			break
		}
	}
	if !hasServiceAction {
		return serviceActionOptions{}, false, nil
	}

	workspace := detectWorkspaceRoot()
	opts := serviceActionOptions{Workspace: workspace}
	fs := flag.NewFlagSet("lsyl-tunnel-server-gui-service-action", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.Action, "service-action", "", "service action")
	fs.StringVar(&opts.ConfigPath, "config", detectServerConfigPath(workspace), "server config file")
	fs.StringVar(&opts.ServiceExe, "service-exe", detectServerServiceExe(workspace), "server service executable")
	fs.StringVar(&opts.ServiceName, "service-name", serverServiceName, "Windows service name")
	fs.StringVar(&opts.LogPath, "log", detectServerServiceLog(workspace), "service log file")
	fs.StringVar(&opts.ResultFile, "result-file", "", "write service action error to file")
	fs.StringVar(&opts.StartType, "start-type", "", "service start type: manual, auto, or empty to preserve")
	if err := fs.Parse(args); err != nil {
		return opts, true, err
	}
	opts.Action = strings.ToLower(strings.TrimSpace(opts.Action))
	if opts.Action == "" {
		return opts, true, fmt.Errorf("缺少服务动作")
	}
	return opts, true, nil
}

func runServiceAction(opts serviceActionOptions) error {
	switch opts.Action {
	case "install", "register":
		return ensureServerService(opts)
	case "start":
		state, installed, err := serverServiceState(opts.ServiceName)
		if err == nil && installed && isServiceActive(state) {
			if !waitForServerServiceState(opts.ServiceName, true, 30*time.Second) {
				return fmt.Errorf("服务端服务正在启动，请稍后查看状态")
			}
			return nil
		}
		if err := checkServerStartConfig(opts.ConfigPath); err != nil {
			return err
		}
		if err := ensureServerService(opts); err != nil {
			return err
		}
		if err := startServerServiceByName(opts.ServiceName); err != nil {
			return err
		}
		if !waitForServerServiceState(opts.ServiceName, true, 30*time.Second) {
			return fmt.Errorf("服务端服务正在启动，请稍后查看状态")
		}
		return nil
	case "restart":
		state, installed, err := serverServiceState(opts.ServiceName)
		if err == nil && installed && isServiceActive(state) {
			if err := stopServerServiceByName(opts.ServiceName); err != nil {
				return err
			}
			if !waitForServerServiceState(opts.ServiceName, false, 30*time.Second) {
				return fmt.Errorf("服务端服务正在停止，请稍后查看状态")
			}
		}
		if err := checkServerStartConfig(opts.ConfigPath); err != nil {
			return err
		}
		if err := ensureServerService(opts); err != nil {
			return err
		}
		if err := startServerServiceByName(opts.ServiceName); err != nil {
			return err
		}
		if !waitForServerServiceState(opts.ServiceName, true, 30*time.Second) {
			return fmt.Errorf("服务端服务正在启动，请稍后查看状态")
		}
		return nil
	case "stop":
		if err := stopServerServiceByName(opts.ServiceName); err != nil {
			return err
		}
		if !waitForServerServiceState(opts.ServiceName, false, 30*time.Second) {
			return fmt.Errorf("服务端服务正在停止，请稍后查看状态")
		}
		return nil
	default:
		return fmt.Errorf("未知服务动作: %s", opts.Action)
	}
}

func writeServiceActionResult(path string, err error) {
	if path == "" || err == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(err.Error()), 0o644)
}

func (a *App) restartServerService() error {
	state, installed, stateErr := serverServiceState(a.serviceName)
	if stateErr != nil && !shouldElevateServiceError(stateErr) {
		return stateErr
	}
	if err := a.checkServerStartConfig(); err != nil {
		return err
	}
	if stateErr != nil && shouldElevateServiceError(stateErr) {
		a.appendLog("需要管理员权限重启服务端服务")
		if err := a.runElevatedServiceAction("restart"); err != nil {
			return err
		}
		if !a.waitForServerServiceRunning(45 * time.Second) {
			return fmt.Errorf("已请求管理员权限重启服务，请授权后稍后查看状态")
		}
		return nil
	}
	if installed && isServiceActive(state) {
		if err := stopServerServiceByName(a.serviceName); err != nil {
			if !shouldElevateServiceError(err) {
				return err
			}
			a.appendLog("需要管理员权限重启服务端服务")
			if err := a.runElevatedServiceAction("restart"); err != nil {
				return err
			}
			if !a.waitForServerServiceRunning(45 * time.Second) {
				return fmt.Errorf("已请求管理员权限重启服务，请授权后稍后查看状态")
			}
			return nil
		}
		if !a.waitForServerServiceStopped(20 * time.Second) {
			return fmt.Errorf("服务端服务正在停止，请稍后重试")
		}
	}
	return a.startServerService()
}

func (a *App) startServerService() error {
	state, installed, stateErr := serverServiceState(a.serviceName)
	if stateErr == nil && installed && isServiceActive(state) {
		if !a.waitForServerServiceRunning(12 * time.Second) {
			return a.serverServiceStartError()
		}
		return nil
	}
	if err := a.checkServerStartConfig(); err != nil {
		return err
	}
	if stateErr != nil {
		if !shouldElevateServiceError(stateErr) {
			return stateErr
		}
		a.appendLog("需要管理员权限注册并启动服务端服务")
		if err := a.runElevatedServiceAction("start"); err != nil {
			return err
		}
		if !a.waitForServerServiceRunning(45 * time.Second) {
			return fmt.Errorf("已请求管理员权限启动服务，请授权后稍后查看状态")
		}
		return nil
	} else if installed {
		if err := startServerServiceByName(a.serviceName); err == nil || errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
			if !a.waitForServerServiceRunning(12 * time.Second) {
				return a.serverServiceStartError()
			}
			return nil
		} else if !shouldElevateServiceError(err) {
			return err
		}
		a.appendLog("需要管理员权限启动服务端服务")
		if err := a.runElevatedServiceAction("start"); err != nil {
			return err
		}
		if !a.waitForServerServiceRunning(45 * time.Second) {
			return fmt.Errorf("已请求管理员权限启动服务，请授权后稍后查看状态")
		}
		return nil
	}

	if err := ensureServerService(a.serviceActionOptions("start")); err != nil {
		if !shouldElevateServiceError(err) {
			return err
		}
		a.appendLog("需要管理员权限注册并启动服务端服务")
		if err := a.runElevatedServiceAction("start"); err != nil {
			return err
		}
		if !a.waitForServerServiceRunning(45 * time.Second) {
			return fmt.Errorf("已请求管理员权限启动服务，请授权后稍后查看状态")
		}
		return nil
	}

	if err := startServerServiceByName(a.serviceName); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		if !shouldElevateServiceError(err) {
			return err
		}
		a.appendLog("需要管理员权限启动服务端服务")
		if err := a.runElevatedServiceAction("start"); err != nil {
			return err
		}
		if !a.waitForServerServiceRunning(45 * time.Second) {
			return fmt.Errorf("已请求管理员权限启动服务，请授权后稍后查看状态")
		}
	}
	if !a.waitForServerServiceRunning(12 * time.Second) {
		return a.serverServiceStartError()
	}
	return nil
}

func (a *App) checkServerStartConfig() error {
	return checkServerStartConfig(a.configPath)
}

func checkServerStartConfig(configPath string) error {
	_, err := tunnel.LoadConfig(configPath)
	return err
}

func (a *App) stopServerService() error {
	_, installed, err := serverServiceState(a.serviceName)
	if err != nil {
		return err
	}
	if !installed {
		return nil
	}
	if err := stopServerServiceByName(a.serviceName); err != nil {
		if !shouldElevateServiceError(err) {
			return err
		}
		a.appendLog("需要管理员权限停止服务端服务")
		if err := a.runElevatedServiceAction("stop"); err != nil {
			return err
		}
		if !a.waitForServerServiceStopped(45 * time.Second) {
			return fmt.Errorf("已请求管理员权限停止服务，请授权后稍后查看状态")
		}
		return nil
	}
	if !a.waitForServerServiceStopped(12 * time.Second) {
		return fmt.Errorf("服务端服务正在停止，请稍后查看状态")
	}
	return nil
}

func (a *App) serverServiceStartError() error {
	if line := lastNonEmptyLogLine(a.logService); line != "" {
		return fmt.Errorf("服务端服务启动失败: %s", line)
	}
	return fmt.Errorf("服务端服务启动失败")
}
func (a *App) serviceActionOptions(action string) serviceActionOptions {
	return serviceActionOptions{
		Action:      action,
		ConfigPath:  a.configPath,
		ServiceExe:  a.serviceExe,
		ServiceName: a.serviceName,
		LogPath:     a.logService,
		Workspace:   a.workspace,
	}
}

func ensureServerService(opts serviceActionOptions) error {
	if opts.ServiceName == "" {
		opts.ServiceName = serverServiceName
	}
	if _, err := os.Stat(opts.ServiceExe); err != nil {
		return fmt.Errorf("找不到服务端服务程序: %s", opts.ServiceExe)
	}
	if _, err := os.Stat(opts.ConfigPath); err != nil {
		return fmt.Errorf("找不到服务端配置文件: %s", opts.ConfigPath)
	}
	if err := os.MkdirAll(filepath.Dir(opts.LogPath), 0o755); err != nil {
		return fmt.Errorf("无法创建服务日志目录: %w", err)
	}
	m, err := mgr.Connect()
	if err != nil {
		return serviceManagerError("连接 Windows 服务管理器失败", err)
	}
	defer m.Disconnect()

	args := serverServiceArgs(opts)
	startType, explicitStartType, err := serviceStartType(opts.StartType)
	if err != nil {
		return err
	}
	s, err := m.OpenService(opts.ServiceName)
	if err != nil {
		if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return serviceManagerError("读取服务状态失败", err)
		}
		if !explicitStartType {
			startType = mgr.StartManual
		}
		s, err = m.CreateService(opts.ServiceName, opts.ServiceExe, mgr.Config{
			DisplayName:  serverServiceDisplayName,
			StartType:    startType,
			ErrorControl: mgr.ErrorNormal,
			Description:  serverServiceDescription,
		}, args...)
		if err != nil {
			return serviceManagerError("创建 Windows 服务失败", err)
		}
		defer s.Close()
		return nil
	}
	defer s.Close()

	cfg, err := s.Config()
	if err != nil {
		return err
	}
	cfg.DisplayName = serverServiceDisplayName
	cfg.Description = serverServiceDescription
	if explicitStartType {
		cfg.StartType = startType
	}
	cfg.BinaryPathName = commandLine(append([]string{opts.ServiceExe}, args...)...)
	if err := s.UpdateConfig(cfg); err != nil {
		return serviceManagerError("更新 Windows 服务配置失败", err)
	}
	return nil
}

func serviceStartType(text string) (uint32, bool, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "":
		return 0, false, nil
	case "manual", "demand":
		return mgr.StartManual, true, nil
	case "auto", "automatic":
		return mgr.StartAutomatic, true, nil
	default:
		return 0, false, fmt.Errorf("未知服务启动类型: %s", text)
	}
}

func serviceManagerError(action string, err error) error {
	switch {
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return fmt.Errorf("%s: 没有管理员权限，请以管理员身份运行安装器或管理台", action)
	case errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE):
		return fmt.Errorf("%s: 服务正在删除中，请关闭服务端管理台并等待 10 秒后重试", action)
	case errors.Is(err, windows.ERROR_INVALID_NAME):
		return fmt.Errorf("%s: 服务名称无效", action)
	case errors.Is(err, windows.ERROR_PATH_NOT_FOUND):
		return fmt.Errorf("%s: 服务程序、配置或日志路径不存在", action)
	default:
		return fmt.Errorf("%s: %w", action, err)
	}
}

func serverServiceArgs(opts serviceActionOptions) []string {
	return []string{
		"-service-name", opts.ServiceName,
		"-config", opts.ConfigPath,
		"-log", opts.LogPath,
	}
}

func startServerServiceByName(name string) error {
	s, cleanup, err := openServerService(name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_START)
	if err != nil {
		return err
	}
	defer cleanup()
	status, err := s.Query()
	if err == nil && isServiceActive(status.State) {
		return nil
	}
	err = s.Start()
	if errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return nil
	}
	return err
}

func stopServerServiceByName(name string) error {
	s, cleanup, err := openServerService(name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_STOP)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		return err
	}
	defer cleanup()
	status, err := s.Query()
	if err == nil && status.State == svc.Stopped {
		return nil
	}
	_, err = s.Control(svc.Stop)
	if errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return nil
	}
	return err
}

func serverServiceState(name string) (svc.State, bool, error) {
	s, cleanup, err := openServerService(name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return svc.Stopped, false, nil
		}
		return svc.Stopped, false, err
	}
	defer cleanup()
	status, err := s.Query()
	if err != nil {
		return svc.Stopped, true, err
	}
	return status.State, true, nil
}

func openServerService(name string, access uint32) (*mgr.Service, func(), error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, nil, err
	}
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		_ = windows.CloseServiceHandle(scm)
		return nil, nil, err
	}
	handle, err := windows.OpenService(scm, namePtr, access)
	if err != nil {
		_ = windows.CloseServiceHandle(scm)
		return nil, nil, err
	}
	s := &mgr.Service{Name: name, Handle: handle}
	cleanup := func() {
		_ = s.Close()
		_ = windows.CloseServiceHandle(scm)
	}
	return s, cleanup, nil
}

func (a *App) isServerServiceRunning() bool {
	state, installed, err := serverServiceState(a.serviceName)
	return err == nil && installed && isServiceActive(state)
}

func (a *App) isServerServiceInstalled() bool {
	_, installed, err := serverServiceState(a.serviceName)
	return err == nil && installed
}

func isServiceActive(state svc.State) bool {
	return state == svc.Running || state == svc.StartPending || state == svc.ContinuePending
}

func (a *App) waitForServerServiceRunning(timeout time.Duration) bool {
	return waitForServerServiceState(a.serviceName, true, timeout)
}

func (a *App) waitForServerServiceStopped(timeout time.Duration) bool {
	return waitForServerServiceState(a.serviceName, false, timeout)
}

func waitForServerServiceState(name string, running bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		state, installed, err := serverServiceState(name)
		if err == nil {
			if running && installed && state == svc.Running {
				return true
			}
			if !running && (!installed || state == svc.Stopped) {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func lastNonEmptyLogLine(path string) string {
	lines := serviceLogTail(path, 1)
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

func (a *App) runElevatedServiceAction(action string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	opts := a.serviceActionOptions(action)
	args := []string{
		"-service-action", action,
		"-config", opts.ConfigPath,
		"-service-exe", opts.ServiceExe,
		"-service-name", opts.ServiceName,
		"-log", opts.LogPath,
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(commandLine(args...))
	cwd, _ := windows.UTF16PtrFromString(a.workspace)
	err = windows.ShellExecute(0, verb, file, params, cwd, windows.SW_HIDE)
	if errors.Is(err, windows.ERROR_CANCELLED) {
		return fmt.Errorf("管理员授权已取消")
	}
	return err
}

func shouldElevateServiceError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		strings.Contains(strings.ToLower(err.Error()), "access is denied")
}

func commandLine(args ...string) string {
	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, syscall.EscapeArg(arg))
	}
	return strings.Join(escaped, " ")
}
