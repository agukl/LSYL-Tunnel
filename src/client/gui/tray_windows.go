//go:build windows

package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const clientSingleInstanceMutexName = `Local\LSYLTunnelGuiSingleInstance`

func (a *App) setupTray() error {
	if a.ni != nil {
		return nil
	}
	ni, err := walk.NewNotifyIcon(a.mw)
	if err != nil {
		return err
	}
	created := false
	defer func() {
		if !created {
			_ = ni.Dispose()
			if a.ni == ni {
				a.ni = nil
			}
		}
	}()
	a.ni = ni

	openAction := walk.NewAction()
	_ = openAction.SetText("打开窗口")
	openAction.Triggered().Attach(func() { a.showWindow() })
	_ = ni.ContextMenu().Actions().Add(openAction)

	stopAction := walk.NewAction()
	_ = stopAction.SetText("断开连接")
	stopAction.Triggered().Attach(func() {
		if err := a.stopClient(); err != nil {
			a.notifyInfo("LSYL Tunnel", friendlyError(err))
		}
	})
	_ = ni.ContextMenu().Actions().Add(stopAction)

	exitAction := walk.NewAction()
	_ = exitAction.SetText("退出客户端")
	exitAction.Triggered().Attach(func() { a.exitApp() })
	_ = ni.ContextMenu().Actions().Add(exitAction)

	ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			a.showWindow()
		}
	})
	a.updateTrayToolTipNow()
	created = true
	return nil
}

func (a *App) disposeTray() {
	if a.ni != nil {
		_ = a.ni.Dispose()
		a.ni = nil
	}
	a.trayToolTip = ""
	a.currentIconKnown = false
	a.windowHidden = false
}

func (a *App) showWindow() {
	if a.mw == nil {
		return
	}
	a.runOnUI(func() {
		if a.mw == nil {
			return
		}
		if a.web != nil && a.mw.Visible() && !win.IsIconic(a.mw.Handle()) {
			a.requestWebViewLayout(a.web, false, 90*time.Millisecond)
		} else {
			win.ShowWindow(a.mw.Handle(), win.SW_RESTORE)
			a.requestWebViewLayout(a.web, true, 120*time.Millisecond)
		}
		a.windowHidden = false
	})
}

func (a *App) hideToTray(message string) {
	a.runOnUI(func() {
		if !a.ensureTrayVisibleNow() {
			return
		}
		if a.mw == nil {
			return
		}
		if !a.mw.Visible() && a.windowHidden {
			return
		}
		a.mw.Hide()
		a.windowHidden = true
	})
}

func (a *App) notifyInfo(title, message string) {
	a.runOnUI(func() { a.notifyInfoNow(title, message) })
}

func (a *App) notifyInfoNow(title, message string) {
	if a.ni == nil || message == "" || !a.ni.Visible() {
		return
	}
	_ = a.ni.ShowInfo(title, message)
}

func (a *App) updateTrayToolTip() {
	a.runOnUI(func() { a.updateTrayToolTipNow() })
}

func (a *App) ensureTrayVisibleNow() bool {
	if a.ni == nil {
		if err := a.setupTray(); err != nil {
			a.appendLog("托盘图标不可用: " + friendlyError(err))
			return false
		}
	}
	if a.ni != nil && !a.ni.Visible() {
		_ = a.ni.SetVisible(true)
	}
	return a.ni != nil
}

func (a *App) updateTrayToolTipNow() {
	if a.ni == nil {
		return
	}
	a.refreshClientIconNow()
	tip := "LSYL Tunnel 未连接"
	if a.isRunning() {
		tip = "LSYL Tunnel 已连接，后台值守中"
	}
	if tip == a.trayToolTip {
		return
	}
	_ = a.ni.SetToolTip(tip)
	a.trayToolTip = tip
}

func (a *App) exitApp() {
	a.exiting = true
	if a.isRunning() {
		_ = a.stopClient()
	}
	if a.mw != nil {
		a.mw.Close()
	}
}

func (a *App) runOnUI(f func()) {
	if a.mw == nil {
		f()
		return
	}
	a.mw.Synchronize(f)
}

func (a *App) acquireSingleInstance() error {
	h, acquired, err := acquireClientSingleInstanceMutex()
	if err != nil {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
		return err
	}
	if !acquired {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
		return fmt.Errorf("LSYL Tunnel Client 已在运行，请从托盘图标打开")
	}
	a.instanceMutex = h
	return nil
}

func (a *App) releaseSingleInstance() {
	if a.instanceMutex == 0 {
		return
	}
	_ = windows.ReleaseMutex(a.instanceMutex)
	_ = windows.CloseHandle(a.instanceMutex)
	a.instanceMutex = 0
}

func enableWebViewIE11Mode() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Internet Explorer\Main\FeatureControl\FEATURE_BROWSER_EMULATION`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.SetDWordValue(filepath.Base(exe), 11001)
}

func detectWorkspaceRoot() string {
	if env := strings.TrimSpace(os.Getenv("LSYL_TUNNEL_WORKSPACE")); env != "" {
		if info, err := os.Stat(env); err == nil && info.IsDir() {
			return env
		}
	}
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		if root := findWorkspaceFrom(start); root != "" {
			return root
		}
	}
	if len(starts) > 0 {
		return starts[0]
	}
	return "."
}

func findWorkspaceFrom(start string) string {
	cur := filepath.Clean(start)
	for i := 0; i < 8; i++ {
		if fileExists(filepath.Join(cur, "go.mod")) ||
			fileExists(filepath.Join(cur, "src", "client", "conf", "client.yaml")) ||
			fileExists(filepath.Join(cur, "client", "conf", "client.yaml")) ||
			fileExists(filepath.Join(cur, "conf", "client.yaml")) {
			return cur
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return ""
}

func detectClientConfigPath(workspace string) string {
	if candidate := filepath.Join(workspace, "src", "client", "conf", "client.yaml"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "client", "conf", "client.yaml"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "conf", "client.yaml"); fileExists(candidate) {
		return candidate
	}
	return filepath.Join(workspace, "src", "client", "conf", "client.yaml")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
