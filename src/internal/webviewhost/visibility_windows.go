//go:build windows

package webviewhost

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
)

// Only the IUnknown prefix and visibility methods are needed here.
type visibilityVTable struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetIsVisible   uintptr
	PutIsVisible   uintptr
}

type visibilityController struct {
	vtbl *visibilityVTable
}

func setControllerVisible(controller *edge.ICoreWebView2Controller, visible bool) error {
	if controller == nil {
		return fmt.Errorf("WebView2 控制器未能创建")
	}
	var value uintptr
	if visible {
		value = 1
	}
	object := (*visibilityController)(unsafe.Pointer(controller))
	// COM reports HRESULT; GetLastError may still contain an unrelated Win32 error.
	hr, _, _ := syscall.SyscallN(object.vtbl.PutIsVisible, uintptr(unsafe.Pointer(object)), value)
	if int32(hr) < 0 {
		return fmt.Errorf("设置 WebView2 显示状态失败: HRESULT 0x%08X", uint32(hr))
	}
	return nil
}
