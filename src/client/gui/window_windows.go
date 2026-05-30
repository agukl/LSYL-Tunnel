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

func (a *App) bindWebViewResizeGuard(web *walk.WebView) {
	if a.mw == nil || web == nil {
		return
	}
	resize := func() {
		a.requestWebViewLayout(web, false, 90*time.Millisecond)
	}
	a.mw.SizeChanged().Attach(resize)
	a.requestWebViewLayout(web, true, 90*time.Millisecond)
}

func (a *App) scheduleInitialShow(web *walk.WebView, delay time.Duration) {
	time.AfterFunc(delay, func() {
		defer func() { _ = recover() }()
		if a.mw == nil || a.mw.Handle() == 0 {
			return
		}
		a.mw.Synchronize(func() {
			a.showInitialWindow(web)
		})
	})
}

func (a *App) showInitialWindow(web *walk.WebView) {
	if a.mw == nil || a.initialShown {
		return
	}
	a.initialShown = true
	a.mw.Show()
	a.requestWebViewLayout(web, true, 90*time.Millisecond)
}

func (a *App) requestWebViewLayout(web *walk.WebView, force bool, delays ...time.Duration) {
	if a.fillWebViewToClient(web, force) && a.regionFallback {
		a.redrawWindow()
	}
	if delay := maxLayoutDelay(delays...); delay > 0 {
		a.scheduleWebViewLayout(web, force, delay)
	}
}

func (a *App) startWebLayoutLoop() {
	if a.webLayoutReq != nil {
		return
	}
	a.webLayoutReq = make(chan webLayoutRequest, 1)
	go func(reqCh <-chan webLayoutRequest) {
		defer func() { _ = recover() }()
		var (
			timer   *time.Timer
			timerCh <-chan time.Time
			pending webLayoutRequest
		)
		for {
			select {
			case req, ok := <-reqCh:
				if !ok {
					if timer != nil {
						timer.Stop()
					}
					return
				}
				if req.web == nil || req.delay <= 0 {
					continue
				}
				if pending.web == nil {
					pending = req
				} else {
					pending.web = req.web
					pending.force = pending.force || req.force
					pending.delay = req.delay
				}
				if timer == nil {
					timer = time.NewTimer(req.delay)
				} else {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(req.delay)
				}
				timerCh = timer.C
			case <-timerCh:
				req := pending
				pending = webLayoutRequest{}
				timerCh = nil
				if a.mw == nil || req.web == nil || a.mw.Handle() == 0 || req.web.Handle() == 0 {
					continue
				}
				a.mw.Synchronize(func() {
					if a.fillWebViewToClient(req.web, req.force) && a.regionFallback {
						a.redrawWindow()
					}
				})
			}
		}
	}(a.webLayoutReq)
}

func (a *App) scheduleWebViewLayout(web *walk.WebView, force bool, delay time.Duration) {
	if a.webLayoutReq == nil || web == nil || delay <= 0 {
		return
	}
	req := webLayoutRequest{web: web, force: force, delay: delay}
	select {
	case a.webLayoutReq <- req:
	default:
		select {
		case <-a.webLayoutReq:
		default:
		}
		a.webLayoutReq <- req
	}
}

func maxLayoutDelay(delays ...time.Duration) time.Duration {
	var max time.Duration
	for _, delay := range delays {
		if delay > max {
			max = delay
		}
	}
	return max
}

func (a *App) fillWebViewToClient(web *walk.WebView, force bool) bool {
	if a.mw == nil || web == nil || a.mw.Handle() == 0 || web.Handle() == 0 {
		return false
	}
	if !win.IsWindowVisible(a.mw.Handle()) || win.IsIconic(a.mw.Handle()) {
		return false
	}
	bounds := a.mw.ClientBoundsPixels()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return false
	}
	rect := walk.Rectangle{Width: bounds.Width, Height: bounds.Height}
	if !force && rect == a.lastWebBounds {
		return false
	}
	if parent := web.Parent(); parent != nil {
		_ = parent.SetBoundsPixels(rect)
	}
	_ = web.SetBoundsPixels(rect)
	a.lastWebBounds = rect
	if a.regionFallback {
		a.applyRoundedWindowRegion(a.mw.Handle(), int32(bounds.Width), int32(bounds.Height))
	}
	return true
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
