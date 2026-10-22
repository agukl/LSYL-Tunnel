//go:build windows

package webviewhost

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

func TestControllerVisibilityLayout(t *testing.T) {
	var table visibilityVTable
	if unsafe.Offsetof(table.PutIsVisible) != 4*unsafe.Sizeof(uintptr(0)) {
		t.Fatal("put_IsVisible must follow IUnknown and get_IsVisible")
	}
}

func TestControllerVisibility(t *testing.T) {
	for _, visible := range []bool{false, true} {
		for _, hresult := range []uintptr{0, 0x80004005} {
			var got uintptr
			calls := 0
			table := &visibilityVTable{
				PutIsVisible: windows.NewCallback(func(this uintptr, value uintptr) uintptr {
					got = value
					calls++
					return hresult
				}),
			}
			object := &visibilityController{vtbl: table}
			err := setControllerVisible((*edge.ICoreWebView2Controller)(unsafe.Pointer(object)), visible)
			if calls != 1 || (got == 1) != visible || got > 1 {
				t.Fatalf("visible=%v, calls=%d, BOOL=%d", visible, calls, got)
			}
			if hresult == 0 && err != nil {
				t.Fatal(err)
			}
			if hresult != 0 && (err == nil || !strings.Contains(err.Error(), "80004005")) {
				t.Fatalf("HRESULT failure missing: %v", err)
			}
			runtime.KeepAlive(object)
		}
	}
}

func TestControllerVisibilityMissingController(t *testing.T) {
	if err := setControllerVisible(nil, true); err == nil {
		t.Fatal("missing controller must fail")
	}
}
