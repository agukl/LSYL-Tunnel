//go:build windows

package gui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

	"lsyltunnel/src/internal/webviewhost"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const windowTitle = "LSYL Tunnel Server"

type App struct {
	workspace   string
	serviceExe  string
	serviceName string
	configPath  string
	logService  string
	crashLog    string
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
	_ = os.Remove(filepath.Join(appTmpDir(workspace), "gui", "server-gui.url"))
	return app
}

func (a *App) Run() (err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
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
	log.Print("工作目录: " + a.workspace)
	log.Print("配置文件: " + a.configPath)
	log.Print("服务程序: " + a.serviceExe)

	ui := application.New(application.Options{
		Name: windowTitle,
		Assets: application.AssetOptions{
			Handler: a.adminHandler(),
		},
	})
	ui.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     windowTitle,
		Width:     1180,
		Height:    760,
		MinWidth:  980,
		MinHeight: 640,
		Frameless: true,
		Windows: application.WindowsWindow{
			NonClientRegionSupport: true,
		},
	})
	return ui.Run()
}
