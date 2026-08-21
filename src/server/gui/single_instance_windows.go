//go:build windows

package gui

import (
	"fmt"
	"time"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

const (
	serverGUISingleInstanceMutexName = `Local\LSYLTunnelServerGuiSingleInstance`
	serverGUIActivationAttempts      = 30
	serverGUIActivationRetryDelay    = 100 * time.Millisecond
)

var (
	claimServerGUIInstanceFn    = claimServerGUIInstance
	activateExistingServerGUIFn = activateExistingServerGUI
	runServerGUIAppFn           = func() error { return NewApp().Run() }
	findServerGUIWindowFn       = findServerGUIWindow
	restoreServerGUIWindowFn    = restoreServerGUIWindow
	serverGUIActivationSleepFn  = time.Sleep
)

func runSingleServerGUI() error {
	release, existing, err := claimServerGUIInstanceFn()
	if err != nil {
		return fmt.Errorf("服务端管理台单实例初始化失败: %w", err)
	}
	if release != nil {
		defer release()
	}
	if existing {
		activateExistingServerGUIFn()
		return nil
	}
	return runServerGUIAppFn()
}

func claimServerGUIInstance() (func(), bool, error) {
	name, err := windows.UTF16PtrFromString(serverGUISingleInstanceMutexName)
	if err != nil {
		return nil, false, err
	}
	handle, err := windows.CreateMutex(nil, true, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		return func() {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
		}, true, nil
	}
	if err != nil {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return nil, false, err
	}
	return func() {
		_ = windows.ReleaseMutex(handle)
		_ = windows.CloseHandle(handle)
	}, false, nil
}

func activateExistingServerGUI() bool {
	for attempt := 0; attempt < serverGUIActivationAttempts; attempt++ {
		if hwnd := findServerGUIWindowFn(); hwnd != 0 {
			restoreServerGUIWindowFn(hwnd)
			return true
		}
		if attempt+1 < serverGUIActivationAttempts {
			serverGUIActivationSleepFn(serverGUIActivationRetryDelay)
		}
	}
	return false
}

func findServerGUIWindow() win.HWND {
	title, err := windows.UTF16PtrFromString(windowTitle)
	if err != nil {
		return 0
	}
	return win.FindWindow(nil, title)
}

func restoreServerGUIWindow(hwnd win.HWND) {
	win.ShowWindow(hwnd, win.SW_RESTORE)
	win.BringWindowToTop(hwnd)
	win.SetForegroundWindow(hwnd)
}
