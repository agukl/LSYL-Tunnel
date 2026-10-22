//go:build windows

package gui

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

func TestFramelessWindowHidesUnusedWalkContainer(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// go test has no embedded GUI manifest. Use the application's common-controls
	// activation context while creating the hidden Walk window.
	manifest, err := filepath.Abs("../cmd/lsyl-tunnel-client-gui/app.manifest")
	if err != nil {
		t.Fatal(err)
	}
	ctx := win.CreateActCtx(&win.ACTCTX{Source: windows.StringToUTF16Ptr(manifest)})
	if uintptr(ctx) == ^uintptr(0) {
		t.Fatal("create GUI activation context failed")
	}
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	defer kernel32.NewProc("ReleaseActCtx").Call(uintptr(ctx))
	cookie, ok := win.ActivateActCtx(ctx)
	if !ok {
		t.Fatal("activate GUI manifest failed")
	}
	defer kernel32.NewProc("DeactivateActCtx").Call(0, cookie)

	a := &App{}
	ui := declarative.MainWindow{
		AssignTo: &a.mw,
		Visible:  false,
		Size:     declarative.Size{Width: 520, Height: 640},
		Layout:   declarative.VBox{MarginsZero: true, SpacingZero: true},
	}
	if err := ui.Create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	container := a.mw.AsContainerBase()
	if container.Handle() == a.mw.Handle() {
		t.Fatal("expected a separate Walk client container")
	}
	if err := a.applyFramelessWindow(); err != nil {
		t.Fatal(err)
	}
	// Inspect WS_VISIBLE rather than IsWindowVisible: the root stays hidden in tests.
	if win.GetWindowLong(container.Handle(), win.GWL_STYLE)&win.WS_VISIBLE != 0 {
		t.Fatal("unused Walk container can cover the root-hosted WebView2")
	}
	if win.IsWindowVisible(a.mw.Handle()) {
		t.Fatal("preparing the browser host must not show the window early")
	}
}
