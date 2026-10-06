//go:build windows

package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"lsyltunnel/src/internal/webviewhost"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const windowTitle = "LSYL Tunnel Server"

type App struct {
	workspace   string
	serviceExe  string
	serviceName string
	configPath  string
	logService  string
	crashLog    string
	panelURL    string

	mw  *walk.MainWindow
	web *webviewhost.Host

	mu           sync.Mutex
	logs         []string
	closing      uint32
	initialShown bool
}

func Run() error {
	return runSingleServerGUI()
}

func RunFromArgs(args []string) error {
	if opts, ok, err := parseConfigCompatArgs(args); ok || err != nil {
		if err != nil {
			return err
		}
		err := runConfigCompatCheck(opts)
		writeConfigCompatResult(opts.ResultFile, err)
		return err
	}
	if opts, ok, err := parseServiceActionArgs(args); ok || err != nil {
		if err != nil {
			return err
		}
		err := runServiceAction(opts)
		writeServiceActionResult(opts.ResultFile, err)
		return err
	}
	return Run()
}

func NewApp() *App {
	workspace := detectWorkspaceRoot()
	app := &App{
		workspace:   workspace,
		serviceExe:  detectServerServiceExe(workspace),
		serviceName: serverServiceName,
		configPath:  detectServerConfigPath(workspace),
		logService:  detectServerServiceLog(workspace),
		crashLog:    filepath.Join(appTmpDir(workspace), "gui", "server-gui.crash.log"),
	}
	_ = os.MkdirAll(filepath.Dir(app.logService), 0o755)
	_ = os.MkdirAll(filepath.Dir(app.crashLog), 0o755)
	return app
}

func (a *App) Run() (err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			if a.isWalkShutdownPanic(r, stack) {
				err = nil
				return
			}
			msg := fmt.Sprintf("panic: %v\r\n%s", r, stack)
			_ = os.WriteFile(a.crashLog, []byte(msg), 0o644)
			err = fmt.Errorf("服务端管理台启动异常，详情见: %s", a.crashLog)
		}
	}()
	return a.runUI()
}

func (a *App) runUI() error {
	if err := webviewhost.CheckRuntime(); err != nil {
		return err
	}
	a.appendLog("工作目录: " + a.workspace)
	a.appendLog("配置文件: " + a.configPath)
	a.appendLog("服务程序: " + a.serviceExe)

	srv, err := a.startAdminServer()
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	var browserPanel *walk.Composite
	ui := MainWindow{
		AssignTo:   &a.mw,
		Title:      windowTitle,
		Size:       Size{Width: 1180, Height: 760},
		MinSize:    Size{Width: 980, Height: 640},
		Background: SolidColorBrush{Color: walk.RGB(232, 246, 245)},
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
		return err
	}
	a.mw.ToolBar().SetVisible(false)
	a.mw.StatusBar().SetVisible(false)
	a.web, err = webviewhost.New(uintptr(browserPanel.Handle()), a.panelURL, "Server", a.showInitialWindow)
	if err != nil {
		a.mw.Dispose()
		return err
	}
	a.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		a.beginShutdown()
	})
	a.bindBrowserResize(browserPanel)
	a.scheduleInitialShow(900 * time.Millisecond)
	a.mw.Run()
	return nil
}
