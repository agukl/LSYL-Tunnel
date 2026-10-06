//go:build windows

package gui

import (
	"net/http"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

const dwmWindowCornerPreferenceRound uint32 = 2
const dwmColorNone uint32 = 0xFFFFFFFE
const windowCornerRadius int32 = 24

var (
	user32SetWindowRgn      = windows.NewLazySystemDLL("user32.dll").NewProc("SetWindowRgn")
	gdi32CreateRoundRectRgn = windows.NewLazySystemDLL("gdi32.dll").NewProc("CreateRoundRectRgn")
)

func (a *App) applyFramelessWindow() {
	if a.mw == nil {
		return
	}
	a.runOnUI(func() {
		hwnd := a.mw.Handle()
		style := win.GetWindowLong(hwnd, win.GWL_STYLE)
		style &^= win.WS_CAPTION | win.WS_THICKFRAME
		win.SetWindowLong(hwnd, win.GWL_STYLE, style)
		win.SetWindowPos(hwnd, 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)

		a.regionFallback = false
		cornerPreference := dwmWindowCornerPreferenceRound
		if err := windows.DwmSetWindowAttribute(
			windows.HWND(hwnd),
			windows.DWMWA_WINDOW_CORNER_PREFERENCE,
			unsafe.Pointer(&cornerPreference),
			uint32(unsafe.Sizeof(cornerPreference)),
		); err != nil {
			a.regionFallback = true
		}
		borderColor := dwmColorNone
		_ = windows.DwmSetWindowAttribute(
			windows.HWND(hwnd),
			windows.DWMWA_BORDER_COLOR,
			unsafe.Pointer(&borderColor),
			uint32(unsafe.Sizeof(borderColor)),
		)
		// SetWindowRgn has no anti-aliasing and produces jagged corners.
		// Keep it only as a fallback for systems that reject DWM corner hints.
		if a.regionFallback {
			a.applyRoundedWindowRegion(hwnd, 0, 0)
		}
	})
}

func (a *App) minimizeWindow() {
	a.runOnUI(func() {
		if a.mw != nil {
			win.ShowWindow(a.mw.Handle(), win.SW_MINIMIZE)
		}
	})
}

func (a *App) bindBrowserResize(panel *walk.Composite) {
	if a.mw == nil || a.web == nil || panel == nil {
		return
	}
	panel.SizeChanged().Attach(a.resizeBrowser)
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
	if a.regionFallback {
		a.applyRoundedWindowRegion(a.mw.Handle(), int32(bounds.Width), int32(bounds.Height))
		a.redrawWindow()
	}
}

func (a *App) redrawWindow() {
	if a.mw == nil || a.mw.Handle() == 0 || !win.IsWindowVisible(a.mw.Handle()) || win.IsIconic(a.mw.Handle()) {
		return
	}
	win.RedrawWindow(
		a.mw.Handle(),
		nil,
		0,
		win.RDW_INVALIDATE|win.RDW_UPDATENOW|win.RDW_ALLCHILDREN|win.RDW_NOERASE,
	)
}

func (a *App) applyRoundedWindowRegion(hwnd win.HWND, width, height int32) {
	if hwnd == 0 {
		return
	}
	if width <= 0 || height <= 0 {
		var rect win.RECT
		if !win.GetClientRect(hwnd, &rect) {
			return
		}
		width = rect.Right - rect.Left
		height = rect.Bottom - rect.Top
	}
	if width <= 0 || height <= 0 {
		return
	}
	diameter := windowCornerRadius * 2
	hrgn, _, _ := gdi32CreateRoundRectRgn.Call(
		0,
		0,
		uintptr(width+1),
		uintptr(height+1),
		uintptr(diameter),
		uintptr(diameter),
	)
	if hrgn == 0 {
		return
	}
	ret, _, _ := user32SetWindowRgn.Call(uintptr(hwnd), hrgn, 1)
	if ret == 0 {
		win.DeleteObject(win.HGDIOBJ(hrgn))
	}
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
