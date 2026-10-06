//go:build windows

package gui

import (
	"context"
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
	regionFallback       bool
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

	initialShown bool
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
	return Run()
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
	workspace := detectWorkspaceRoot()
	app := &App{
		workspace:  workspace,
		configPath: detectClientConfigPath(workspace),
		crashLog:   filepath.Join(appTmpDir(workspace), "gui", "client-gui.crash.log"),
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
		return err
	}
	defer a.releaseSingleInstance()
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

	var browserPanel *walk.Composite
	ui := MainWindow{
		AssignTo:   &a.mw,
		Title:      windowTitle,
		Icon:       a.iconForState(),
		Size:       Size{Width: 520, Height: 640},
		MinSize:    Size{Width: 520, Height: 640},
		Background: SolidColorBrush{Color: walk.RGB(238, 248, 247)},
		Layout:     VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			Composite{
				AssignTo:      &browserPanel,
				Layout:        VBox{MarginsZero: true, SpacingZero: true},
				StretchFactor: 1,
			},
		},
	}
	if err := ui.Create(); err != nil {
		_ = srv.Close()
		return err
	}
	a.mw.ToolBar().SetVisible(false)
	a.mw.StatusBar().SetVisible(false)
	a.applyFramelessWindow()
	a.web, err = webviewhost.New(uintptr(browserPanel.Handle()), a.panelURL, "Client", a.showInitialWindow)
	if err != nil {
		a.mw.Dispose()
		return err
	}
	a.bindBrowserResize(browserPanel)
	defer a.disposeTray()

	a.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		if !a.exiting {
			if a.isRunning() {
				*canceled = true
				a.hideToTray("LSYL Tunnel 已进入后台值守，点击托盘图标可重新打开。")
				return
			}
			a.exiting = true
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
			return
		}
		a.exiting = true
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	a.scheduleInitialShow(900 * time.Millisecond)
	a.mw.Run()
	return nil
}
