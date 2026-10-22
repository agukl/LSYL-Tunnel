//go:build windows

package gui

import (
	"net/http"
	"time"
	"unsafe"

	"lsyltunnel/src/internal/webviewhost"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

const dwmWindowCornerPreferenceDoNotRound uint32 = 1
const dwmColorNone uint32 = 0xFFFFFFFE

// Called on the UI thread before WebView2 is created.
func (a *App) applyFramelessWindow() error {
	if a.mw == nil {
		return nil
	}
	hwnd := a.mw.Handle()
	style := win.GetWindowLong(hwnd, win.GWL_STYLE)
	style &^= win.WS_CAPTION | win.WS_THICKFRAME
	win.SetWindowLong(hwnd, win.GWL_STYLE, style)
	win.SetWindowPos(hwnd, 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)

	// The page owns the corner radius, including on Windows 11.
	cornerPreference := dwmWindowCornerPreferenceDoNotRound
	_ = windows.DwmSetWindowAttribute(
		windows.HWND(hwnd),
		windows.DWMWA_WINDOW_CORNER_PREFERENCE,
		unsafe.Pointer(&cornerPreference),
		uint32(unsafe.Sizeof(cornerPreference)),
	)
	borderColor := dwmColorNone
	_ = windows.DwmSetWindowAttribute(
		windows.HWND(hwnd),
		windows.DWMWA_BORDER_COLOR,
		unsafe.Pointer(&borderColor),
		uint32(unsafe.Sizeof(borderColor)),
	)
	return webviewhost.EnableWindowTransparency(uintptr(hwnd))
}

func (a *App) minimizeWindow() {
	a.runOnUI(func() {
		if a.mw != nil {
			win.ShowWindow(a.mw.Handle(), win.SW_MINIMIZE)
		}
	})
}

func (a *App) bindBrowserResize() {
	if a.mw == nil || a.web == nil {
		return
	}
	a.mw.SizeChanged().Attach(a.resizeBrowser)
	a.mw.BoundsChanged().Attach(func() {
		if !a.exiting {
			a.web.NotifyPositionChanged()
		}
	})
	a.resizeBrowser()
}

func (a *App) scheduleInitialShow(delay time.Duration) {
	time.AfterFunc(delay, func() {
		defer func() { _ = recover() }()
		if a.mw == nil || a.mw.Handle() == 0 {
			return
		}
		a.mw.Synchronize(a.showInitialWindow)
	})
}

func (a *App) showInitialWindow() {
	if a.mw == nil || a.initialShown {
		return
	}
	a.initialShown = true
	a.mw.Show()
	a.resizeBrowser()
	if a.autostartLaunch {
		go a.autoConnect()
	}
}

func (a *App) resizeBrowser() {
	if a.exiting || a.mw == nil || a.web == nil || a.mw.Handle() == 0 {
		return
	}
	if !win.IsWindowVisible(a.mw.Handle()) || win.IsIconic(a.mw.Handle()) {
		return
	}
	bounds := a.mw.ClientBoundsPixels()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return
	}
	a.web.Resize()
}

func (a *App) beginWindowDrag() {
	a.runOnUI(func() {
		if a.mw == nil {
			return
		}
		win.ReleaseCapture()
		win.SendMessage(a.mw.Handle(), win.WM_NCLBUTTONDOWN, win.HTCAPTION, 0)
	})
}

func (a *App) handleWindowMinimize(w http.ResponseWriter, r *http.Request) {
	a.minimizeWindow()
	a.writeJSON(w, apiResult{OK: true, Message: "已最小化"})
}

func (a *App) handleWindowClose(w http.ResponseWriter, r *http.Request) {
	if a.isRunning() {
		a.hideToTray("LSYL Tunnel 已进入后台值守，点击托盘图标可重新打开。")
		a.writeJSON(w, apiResult{OK: true, Message: "已进入后台值守"})
		return
	}
	a.writeJSON(w, apiResult{OK: true, Message: "正在退出"})
	go func() {
		a.runOnUI(func() { a.exitApp() })
	}()
}

func (a *App) handleWindowDrag(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, apiResult{OK: true, Message: ""})
	go a.beginWindowDrag()
}
