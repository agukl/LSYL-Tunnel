//go:build windows

package gui

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

func (a *App) beginShutdown() {
	if !atomic.CompareAndSwapUint32(&a.closing, 0, 1) {
		return
	}
	a.mw = nil
	a.web = nil
}

func (a *App) isWalkShutdownPanic(r any, stack []byte) bool {
	if atomic.LoadUint32(&a.closing) == 0 {
		return false
	}
	msg := fmt.Sprint(r)
	if !strings.Contains(msg, "WindowGroup that has been removed from its manager") {
		return false
	}
	return strings.Contains(string(stack), "github.com/lxn/walk.(*FormBase).close")
}

func (a *App) bindBrowserResize(panel *walk.Composite) {
	if a.mw == nil || a.web == nil || panel == nil {
		return
	}
	web := a.web
	panel.SizeChanged().Attach(a.resizeBrowser)
	a.mw.SizeChanged().Attach(a.resizeBrowser)
	a.mw.BoundsChanged().Attach(func() {
		if atomic.LoadUint32(&a.closing) == 0 {
			web.NotifyPositionChanged()
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
	a.redrawWindow()
}

func (a *App) resizeBrowser() {
	if a.mw == nil || a.web == nil || a.mw.Handle() == 0 {
		return
	}
	if !win.IsWindowVisible(a.mw.Handle()) || win.IsIconic(a.mw.Handle()) {
		return
	}
	a.web.Resize()
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
