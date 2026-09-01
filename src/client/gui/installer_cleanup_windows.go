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
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	clientInstallerCleanupFlag = "installer-cleanup"
	clientGUIExecutableName    = "lsyl-tunnel-client-gui.exe"
	winDivertServiceName       = "WinDivert"
)

type clientInstallerCleanupOptions struct {
	Cleanup            bool
	InstalledClientExe string
	ResultFile         string
}

func parseClientInstallerCleanupArgs(args []string) (clientInstallerCleanupOptions, bool, error) {
	hasCleanup := false
	for _, arg := range args {
		if arg == "-"+clientInstallerCleanupFlag || strings.HasPrefix(arg, "-"+clientInstallerCleanupFlag+"=") {
			hasCleanup = true
			break
		}
	}
	if !hasCleanup {
		return clientInstallerCleanupOptions{}, false, nil
	}

	opts := clientInstallerCleanupOptions{}
	fs := flag.NewFlagSet("lsyl-tunnel-client-gui-installer-cleanup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.Cleanup, clientInstallerCleanupFlag, false, "clean client upgrade conflicts")
	fs.StringVar(&opts.InstalledClientExe, "installed-client-exe", "", "installed client executable")
	fs.StringVar(&opts.ResultFile, "result-file", "", "write cleanup error to file")
	if err := fs.Parse(args); err != nil {
		return opts, true, err
	}
	if !opts.Cleanup {
		return opts, true, fmt.Errorf("-%s must be enabled", clientInstallerCleanupFlag)
	}
	if strings.TrimSpace(opts.InstalledClientExe) == "" {
		return opts, true, fmt.Errorf("missing -installed-client-exe")
	}
	if !strings.EqualFold(filepath.Base(filepath.Clean(opts.InstalledClientExe)), clientGUIExecutableName) {
		return opts, true, fmt.Errorf("installed client executable must be %s", clientGUIExecutableName)
	}
	return opts, true, nil
}

func runClientInstallerCleanup(opts clientInstallerCleanupOptions) error {
	target, err := filepath.Abs(strings.TrimSpace(opts.InstalledClientExe))
	if err != nil {
		return fmt.Errorf("解析客户端安装路径失败: %w", err)
	}
	target = filepath.Clean(target)
	if err := terminateInstalledClientProcesses(target, 8*time.Second); err != nil {
		return fmt.Errorf("无法结束旧客户端进程: %w", err)
	}
	installRoot := filepath.Dir(filepath.Dir(target))
	_ = os.Remove(filepath.Join(installRoot, "tmp", "gui", "client-gui.url"))
	if err := cleanupOwnedWinDivertService(installRoot, 8*time.Second); err != nil {
		return fmt.Errorf("无法释放旧 WinDivert 驱动: %w", err)
	}
	return nil
}

func writeClientInstallerCleanupResult(path string, err error) {
	if strings.TrimSpace(path) == "" || err == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(err.Error()), 0o644)
}

func terminateInstalledClientProcesses(target string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		matched, err := terminateInstalledClientProcessPass(target)
		if err != nil {
			return err
		}
		if matched == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("旧客户端进程在 %s 内未退出", timeout)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func terminateInstalledClientProcessPass(target string) (int, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return 0, nil
		}
		return 0, err
	}

	matched := 0
	for {
		if entry.ProcessID != uint32(os.Getpid()) && strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), filepath.Base(target)) {
			isTarget, terminateErr := terminateProcessIfPathMatches(entry.ProcessID, target)
			if terminateErr != nil {
				return matched, terminateErr
			}
			if isTarget {
				matched++
			}
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return matched, err
		}
	}
	return matched, nil
}

func terminateProcessIfPathMatches(pid uint32, target string) (bool, error) {
	queryHandle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return false, nil
		}
		return false, err
	}
	path, err := processImagePath(queryHandle)
	_ = windows.CloseHandle(queryHandle)
	if err != nil {
		return false, err
	}
	if !sameWindowsPath(path, target) {
		return false, nil
	}

	process, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return false, nil
		}
		return true, err
	}
	defer windows.CloseHandle(process)
	if err := windows.TerminateProcess(process, 0); err != nil {
		status, waitErr := windows.WaitForSingleObject(process, 0)
		if waitErr == nil && status == windows.WAIT_OBJECT_0 {
			return true, nil
		}
		return true, err
	}
	if _, err := windows.WaitForSingleObject(process, 3000); err != nil {
		return true, err
	}
	return true, nil
}

func processImagePath(process windows.Handle) (string, error) {
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

func cleanupOwnedWinDivertService(installRoot string, timeout time.Duration) error {
	queryService, queryCleanup, err := openClientUpgradeService(
		winDivertServiceName,
		windows.SERVICE_QUERY_CONFIG,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) || errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
			return nil
		}
		return err
	}
	cfg, err := queryService.Config()
	queryCleanup()
	if err != nil {
		return err
	}
	if !winDivertServiceOwnedByInstall(cfg.BinaryPathName, installRoot) {
		return nil
	}

	service, serviceCleanup, err := openClientUpgradeService(
		winDivertServiceName,
		windows.SERVICE_QUERY_STATUS|windows.SERVICE_STOP|windows.DELETE,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) || errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
			return nil
		}
		return err
	}

	status, err := service.Query()
	if err != nil {
		serviceCleanup()
		return err
	}
	if status.State != svc.Stopped && status.State != svc.StopPending {
		if _, err := service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			serviceCleanup()
			return err
		}
	}
	deadline := time.Now().Add(timeout)
	for status.State != svc.Stopped && time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		status, err = service.Query()
		if err != nil {
			serviceCleanup()
			return err
		}
	}
	if status.State != svc.Stopped {
		serviceCleanup()
		return fmt.Errorf("WinDivert 服务在 %s 内未停止", timeout)
	}
	deleteErr := service.Delete()
	serviceCleanup()
	if deleteErr != nil && !errors.Is(deleteErr, windows.ERROR_SERVICE_DOES_NOT_EXIST) && !errors.Is(deleteErr, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
		return deleteErr
	}
	return nil
}

func openClientUpgradeService(name string, access uint32) (*mgr.Service, func(), error) {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, nil, err
	}
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		_ = windows.CloseServiceHandle(manager)
		return nil, nil, err
	}
	handle, err := windows.OpenService(manager, namePtr, access)
	if err != nil {
		_ = windows.CloseServiceHandle(manager)
		return nil, nil, err
	}
	service := &mgr.Service{Name: name, Handle: handle}
	cleanup := func() {
		_ = service.Close()
		_ = windows.CloseServiceHandle(manager)
	}
	return service, cleanup, nil
}

func winDivertServiceOwnedByInstall(binaryPath, installRoot string) bool {
	path := normalizeWindowsServicePath(binaryPath)
	if !strings.EqualFold(filepath.Base(path), "WinDivert64.sys") {
		return false
	}
	root, err := filepath.Abs(installRoot)
	if err != nil {
		return false
	}
	root = strings.TrimRight(filepath.Clean(root), `\/`)
	path, err = filepath.Abs(path)
	if err != nil {
		return false
	}
	path = filepath.Clean(path)
	return strings.HasPrefix(strings.ToLower(path), strings.ToLower(root+string(filepath.Separator)))
}

func normalizeWindowsServicePath(path string) string {
	path = strings.Trim(strings.TrimSpace(path), `"`)
	for _, prefix := range []string{`\??\`, `\\?\`} {
		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
			break
		}
	}
	return filepath.Clean(path)
}

func sameWindowsPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}
