//go:build windows

package webviewhost

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

func TestController2Layout(t *testing.T) {
	var table controller2VTable
	slot := unsafe.Sizeof(uintptr(0))
	if unsafe.Offsetof(table.GetBackgroundColor) != 26*slot || unsafe.Offsetof(table.PutBackgroundColor) != 27*slot {
		t.Fatal("Controller2 extension must follow the 26 base COM slots")
	}
}

func TestTransparentBackgroundHRESULTAndRelease(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hresult   uintptr
		wantError bool
	}{
		{"success", 0, false},
		{"failure", 0x80004005, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var released, writes int
			var color uint32
			table := &controller2VTable{}
			object := &controller2{vtbl: table}
			table.QueryInterface = windows.NewCallback(func(this *controller2, iid *windows.GUID, out **controller2) uintptr {
				*out = object
				return 0
			})
			table.Release = windows.NewCallback(func(this *controller2) uintptr {
				released++
				return 1
			})
			table.PutBackgroundColor = windows.NewCallback(func(this *controller2, value uint32) uintptr {
				writes++
				color = value
				return tc.hresult
			})
			err := setTransparentBackground((*edge.ICoreWebView2Controller)(unsafe.Pointer(object)))
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, want error = %v", err, tc.wantError)
			}
			if tc.wantError && !strings.Contains(err.Error(), "80004005") {
				t.Fatalf("missing HRESULT: %v", err)
			}
			if released != 1 || writes != 1 || color != 0 {
				t.Fatalf("release=%d, writes=%d, color=%08X", released, writes, color)
			}
			runtime.KeepAlive(object)
		})
	}
}

func TestTransparentBackgroundMissingController(t *testing.T) {
	if err := setTransparentBackground(nil); err == nil {
		t.Fatal("nil controller must fail")
	}
	table := &controller2VTable{
		QueryInterface: windows.NewCallback(func(this *controller2, iid *windows.GUID, out **controller2) uintptr {
			*out = nil
			return 0x80004002
		}),
	}
	object := &controller2{vtbl: table}
	if err := setTransparentBackground((*edge.ICoreWebView2Controller)(unsafe.Pointer(object))); err == nil {
		t.Fatal("missing Controller2 interface must fail")
	}
	runtime.KeepAlive(object)
}

func TestWindowTransparencyLifecycle(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := EnableWindowTransparency(0); err == nil {
		t.Fatal("zero window handle must fail")
	}
	// Keep this native test window hidden; no browser or visual inspection is required.
	hwnd := win.CreateWindowEx(0, windows.StringToUTF16Ptr("STATIC"), nil,
		win.WS_POPUP, 0, 0, 100, 100, 0, 0, win.GetModuleHandle(nil), nil)
	if hwnd == 0 {
		t.Fatal("create hidden test window failed")
	}
	defer win.DestroyWindow(hwnd)
	for i := 0; i < 3; i++ {
		if err := EnableWindowTransparency(uintptr(hwnd)); err != nil {
			t.Fatal(err)
		}
	}
	win.SendMessage(hwnd, 0x031E, 0, 0) // WM_DWMCOMPOSITIONCHANGED
	hdc := win.GetDC(hwnd)
	defer win.ReleaseDC(hwnd, hdc)
	if result := win.SendMessage(hwnd, win.WM_ERASEBKGND, uintptr(hdc), 0); result != 1 {
		t.Fatalf("background erase result = %d", result)
	}
}
