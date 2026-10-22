//go:build windows

package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"lsyltunnel/src/client/tunnel"
	"lsyltunnel/src/internal/webviewhost"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
)

const windowTitle = "LSYL Tunnel Client"

type App struct {
	workspace  string
	configPath string
	crashLog   string
	panelURL   string

	mw  *walk.MainWindow
	web *webviewhost.Host
	ni  *walk.NotifyIcon

	iconIdle      *walk.Icon
	iconConnected *walk.Icon

	instanceMutex        windows.Handle
	exiting              bool
	windowHidden         atomic.Bool
	currentIconKnown     bool
	currentIconConnected bool
	trayToolTip          string

	mu        sync.Mutex
	tun       *tunnel.Client
	stop      context.CancelFunc
	logs      []string
	notice    string
	noticeBad bool

	profileMu         sync.Mutex
	profileCache      []clientProfile
	profileCacheUntil time.Time

	initialShown    bool
	autostartLaunch bool
	autostart       *clientAutostart
	connecting      atomic.Bool
	autoLoginDone   atomic.Bool
	closing         atomic.Bool
	loginCancel     context.CancelFunc
	loginMu         sync.Mutex
}

func Run() error {
	app := NewApp()
	return app.Run()
}

func RunFromArgs(args []string) error {
	if opts, ok, err := parseClientInstallerCleanupArgs(args); ok || err != nil {
		if err != nil {
			return err
		}
		err := runClientInstallerCleanup(opts)
		writeClientInstallerCleanupResult(opts.ResultFile, err)
		return err
	}
	if opts, ok, err := parseClientConfigCompatArgs(args); ok || err != nil {
		if err != nil {
			return err
		}
		err := runClientConfigCompatCheck(opts)
		writeClientConfigCompatResult(opts.ResultFile, err)
		return err
	}
	if IsQuitCommand(args) {
		return requestQuitRunningInstance()
	}
	app := newApp(isAutostartLaunch(args))
	return app.Run()
}

func IsQuitCommand(args []string) bool {
	for _, arg := range args {
		if isClientQuitArg(arg) {
			return true
		}
	}
	return false
}

func isClientQuitArg(arg string) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "/quit", "-quit", "--quit", "/exit", "-exit", "--exit":
		return true
	default:
		return false
	}
}

func NewApp() *App {
	return newApp(false)
}

func newApp(autostartLaunch bool) *App {
	workspace := detectWorkspaceRoot()
	executable, _ := os.Executable()
	// A Run entry has no working directory; resolve the installation from the executable.
	if autostartLaunch && executable != "" {
		workspace = autostartWorkspace(executable)
	}
	app := &App{
		workspace:       workspace,
		configPath:      detectClientConfigPath(workspace),
		crashLog:        filepath.Join(appTmpDir(workspace), "gui", "client-gui.crash.log"),
		autostart:       newClientAutostart(executable),
		autostartLaunch: autostartLaunch,
	}
	_ = os.MkdirAll(filepath.Dir(app.crashLog), 0o755)
	return app
}

func (a *App) Run() (err error) {
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("panic: %v\r\n%s", r, debug.Stack())
			_ = os.WriteFile(a.crashLog, []byte(msg), 0o644)
			err = fmt.Errorf("客户端界面启动异常，请查看 %s", a.crashLog)
		}
	}()
	if err := a.acquireSingleInstance(); err != nil {
		if a.autostartLaunch && errors.Is(err, errClientAlreadyRunning) {
			return nil
		}
		return err
	}
	defer a.releaseSingleInstance()
	defer a.cancelPendingLogin()
	return a.run()
}

func (a *App) run() error {
	if err := webviewhost.CheckRuntime(); err != nil {
		return err
	}
	if err := a.loadClientIcons(); err != nil {
		a.appendLog("图标初始化失败: " + friendlyError(err))
	}
	defer a.disposeClientIcons()

	a.appendLog("工作目录: " + a.workspace)
	a.appendLog("配置文件: " + a.configPath)
	a.appendLog("隧道引擎: 内置后台值守")

	srv, err := a.startAdminServer()
	if err != nil {
		return err
	}
	defer srv.Close()

	// GDI black provides zero-alpha backing; the page paints the visible surface.
	ui := MainWindow{
		AssignTo:   &a.mw,
		Title:      windowTitle,
		Icon:       a.iconForState(),
		Size:       Size{Width: 520, Height: 640},
		MinSize:    Size{Width: 520, Height: 640},
		Visible:    false,
		Layout:     VBox{MarginsZero: true, SpacingZero: true},
		Background: SolidColorBrush{Color: walk.RGB(0, 0, 0)},
	}
	if err := ui.Create(); err != nil {
		_ = srv.Close()
		return err
	}
	a.mw.ToolBar().SetVisible(false)
	a.mw.StatusBar().SetVisible(false)
	if err := a.applyFramelessWindow(); err != nil {
		a.mw.Dispose()
		return err
	}
	a.web, err = webviewhost.NewWithOptions(uintptr(a.mw.Handle()), a.panelURL, "Client", a.showInitialWindow, webviewhost.Options{Transparent: true})
	if err != nil {
		a.mw.Dispose()
		return err
	}
	a.bindBrowserResize()
	defer a.disposeTray()

	a.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		if !a.exiting {
			if a.isRunning() {
				*canceled = true
				a.hideToTray("LSYL Tunnel 已进入后台值守，点击托盘图标可重新打开。")
				return
			}
			a.exiting = true
			a.cancelPendingLogin()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
			return
		}
		a.exiting = true
		a.cancelPendingLogin()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	a.scheduleInitialShow(900 * time.Millisecond)
	a.mw.Run()
	return nil
}
