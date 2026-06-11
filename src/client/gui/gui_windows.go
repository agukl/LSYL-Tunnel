//go:build windows

package gui

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"lsyltunnel/src/client/tunnel"

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
	web *walk.WebView
	ni  *walk.NotifyIcon

	iconIdle      *walk.Icon
	iconConnected *walk.Icon

	instanceMutex        windows.Handle
	exiting              bool
	regionFallback       bool
	windowHidden         bool
	currentIconKnown     bool
	currentIconConnected bool
	trayToolTip          string

	mu        sync.Mutex
	tun       *tunnel.Client
	stop      context.CancelFunc
	logs      []string
	notice    string
	noticeBad bool

	webLayoutReq  chan webLayoutRequest
	lastWebBounds walk.Rectangle
	initialShown  bool
}

type webLayoutRequest struct {
	web   *walk.WebView
	force bool
	delay time.Duration
}

func Run() error {
	app := NewApp()
	return app.Run()
}

func RunFromArgs(args []string) error {
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
	enableWebViewIE11Mode()
	if err := a.loadClientIcons(); err != nil {
		a.appendLog("图标初始化失败: " + friendlyError(err))
	}
	defer a.disposeClientIcons()

	a.appendLog("工作目录: " + a.workspace)
	a.appendLog("配置文件: " + a.configPath)
	a.appendLog("隧道引擎: 内置后台值守")

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleHome)
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/health/check", a.handleHealthCheck)
	mux.HandleFunc("/api/start", a.handleStart)
	mux.HandleFunc("/api/stop", a.handleStop)
	mux.HandleFunc("/api/mobile/export", a.handleMobileExport)
	mux.HandleFunc("/api/hide", a.handleHide)
	mux.HandleFunc("/api/quit", a.handleQuit)
	mux.HandleFunc("/api/window/minimize", a.handleWindowMinimize)
	mux.HandleFunc("/api/window/close", a.handleWindowClose)
	mux.HandleFunc("/api/window/drag", a.handleWindowDrag)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	a.panelURL = "http://" + ln.Addr().String() + "/"
	tmpDir := filepath.Join(appTmpDir(a.workspace), "gui")
	_ = os.MkdirAll(tmpDir, 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "client-gui.url"), []byte(a.panelURL), 0o644)

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			a.appendLog("界面服务异常: " + serveErr.Error())
		}
	}()

	var web *walk.WebView
	ui := MainWindow{
		AssignTo:   &a.mw,
		Title:      windowTitle,
		Icon:       a.iconForState(),
		Size:       Size{520, 640},
		MinSize:    Size{520, 640},
		Background: SolidColorBrush{Color: walk.RGB(238, 248, 247)},
		Layout:     VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			WebView{
				AssignTo:                 &web,
				URL:                      a.panelURL,
				Background:               SolidColorBrush{Color: walk.RGB(238, 248, 247)},
				NativeContextMenuEnabled: false,
				StretchFactor:            1,
				OnDocumentCompleted: func(string) {
					a.showInitialWindow(web)
				},
			},
		},
	}
	if err := ui.Create(); err != nil {
		_ = srv.Close()
		return err
	}
	a.web = web
	a.startWebLayoutLoop()
	a.mw.ToolBar().SetVisible(false)
	a.mw.StatusBar().SetVisible(false)
	a.applyFramelessWindow()
	a.bindWebViewResizeGuard(web)
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
	a.scheduleInitialShow(web, 900*time.Millisecond)
	a.mw.Run()
	return nil
}

func appTmpDir(workspace string) string {
	if fileExists(filepath.Join(workspace, "go.mod")) {
		return filepath.Join(workspace, "build", "tmp")
	}
	return filepath.Join(workspace, "tmp")
}
