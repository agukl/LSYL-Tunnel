//go:build windows

package webviewhost

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

var (
	dwmEnableBlurBehindWindow = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmEnableBlurBehindWindow")
	setWindowSubclass         = windows.NewLazySystemDLL("comctl32.dll").NewProc("SetWindowSubclass")
	removeWindowSubclass      = windows.NewLazySystemDLL("comctl32.dll").NewProc("RemoveWindowSubclass")
	defSubclassProc           = windows.NewLazySystemDLL("comctl32.dll").NewProc("DefSubclassProc")
	fillRect                  = windows.NewLazySystemDLL("user32.dll").NewProc("FillRect")
	transparentWindowProc     = windows.NewCallback(transparentWindowMessage)
)

const (
	dwmBBEnable     = 1
	dwmBBBlurRegion = 2
)

type dwmBlurBehind struct {
	Flags                 uint32
	Enable                int32
	Region                win.HRGN
	TransitionOnMaximized int32
}

// EnableWindowTransparency enables alpha composition on a top-level window.
// Host the WebView directly in this window, with a transparent browser background.
// Call from the window's UI thread before creating the WebView.
func EnableWindowTransparency(hwnd uintptr) error {
	if hwnd == 0 {
		return fmt.Errorf("透明窗口句柄无效")
	}
	if err := enableWindowAlpha(hwnd); err != nil {
		return err
	}
	if ok, _, _ := setWindowSubclass.Call(hwnd, transparentWindowProc, transparentWindowProc, 0); ok == 0 {
		return fmt.Errorf("安装透明窗口消息处理失败")
	}
	return nil
}

func enableWindowAlpha(hwnd uintptr) error {
	// The region is outside the client area: enable alpha composition without blur.
	region := win.CreateRectRgn(-1, -1, 0, 0)
	if region == 0 {
		return fmt.Errorf("创建透明合成区域失败")
	}
	defer win.DeleteObject(win.HGDIOBJ(region))
	settings := dwmBlurBehind{Flags: dwmBBEnable | dwmBBBlurRegion, Enable: 1, Region: region}
	hr, _, _ := dwmEnableBlurBehindWindow.Call(hwnd, uintptr(unsafe.Pointer(&settings)))
	if int32(hr) < 0 {
		return fmt.Errorf("启用窗口透明合成失败: HRESULT 0x%08X", uint32(hr))
	}
	return nil
}

func transparentWindowMessage(hwnd uintptr, message uint32, wParam, lParam, id, refData uintptr) uintptr {
	switch message {
	case win.WM_ERASEBKGND:
		// GDI black clears RGB and alpha to zero, preventing opaque corner backing.
		var rect win.RECT
		if win.GetClientRect(win.HWND(hwnd), &rect) {
			fillRect.Call(wParam, uintptr(unsafe.Pointer(&rect)), uintptr(win.GetStockObject(win.BLACK_BRUSH)))
			return 1
		}
	case 0x031E: // WM_DWMCOMPOSITIONCHANGED
		_ = enableWindowAlpha(hwnd)
	case win.WM_NCDESTROY:
		// The subclass ID is the callback address supplied at registration.
		removeWindowSubclass.Call(hwnd, id, id)
	}
	result, _, _ := defSubclassProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func embedTransparentView(view *edge.Chromium, parent uintptr) (bool, error) {
	// Set the initial surface color before WebView2 starts; the setter alone can flash white.
	// Construction is on the UI thread, and the environment is restored before returning.
	const key = "WEBVIEW2_DEFAULT_BACKGROUND_COLOR"
	previous, existed := os.LookupEnv(key)
	if err := os.Setenv(key, "00000000"); err != nil {
		return false, fmt.Errorf("初始化 WebView2 透明背景失败: %w", err)
	}
	defer func() {
		if existed {
			_ = os.Setenv(key, previous)
		} else {
			_ = os.Unsetenv(key)
		}
	}()
	return view.Embed(parent), nil
}

// Only the Controller2 extension is needed. Keep the base COM slots in order.
type controller2VTable struct {
	QueryInterface     uintptr
	AddRef             uintptr
	Release            uintptr
	ControllerMethods  [23]uintptr
	GetBackgroundColor uintptr
	PutBackgroundColor uintptr
}

type controller2 struct {
	vtbl *controller2VTable
}

func setTransparentBackground(controller *edge.ICoreWebView2Controller) error {
	if controller == nil {
		return fmt.Errorf("WebView2 控制器未能创建")
	}
	ptr := controller.GetICoreWebView2Controller2()
	if ptr == nil {
		return fmt.Errorf("WebView2 Runtime 不支持透明背景，请更新运行时")
	}
	object := (*controller2)(unsafe.Pointer(ptr))
	defer syscall.SyscallN(object.vtbl.Release, uintptr(unsafe.Pointer(object)))
	// COM returns HRESULT, not GetLastError; the dependency's setter checks the latter.
	hr, _, _ := syscall.SyscallN(object.vtbl.PutBackgroundColor, uintptr(unsafe.Pointer(object)), 0)
	if int32(hr) < 0 {
		return fmt.Errorf("设置 WebView2 透明背景失败: HRESULT 0x%08X", uint32(hr))
	}
	return nil
}
