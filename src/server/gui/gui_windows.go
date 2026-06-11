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
	"sync/atomic"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows/registry"
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
	web *walk.WebView

	mu            sync.Mutex
	logs          []string
	webLayoutSeq  uint64
	lastWebBounds walk.Rectangle
	initialShown  bool
}

func Run() error {
	app := NewApp()
	return app.Run()
}

func RunFromArgs(args []string) error {
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
			msg := fmt.Sprintf("panic: %v\r\n%s", r, debug.Stack())
			_ = os.WriteFile(a.crashLog, []byte(msg), 0o644)
			err = fmt.Errorf("服务端管理台启动异常，详情见: %s", a.crashLog)
		}
	}()
	return a.runUI()
}

func (a *App) runUI() error {
	enableWebViewIE11Mode()
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

	var web *walk.WebView
	ui := MainWindow{
		AssignTo:   &a.mw,
		Title:      windowTitle,
		Size:       Size{1180, 760},
		MinSize:    Size{980, 640},
		Background: SolidColorBrush{Color: walk.RGB(232, 246, 245)},
		Layout:     VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			WebView{
				AssignTo:                 &web,
				URL:                      a.panelURL,
				Background:               SolidColorBrush{Color: walk.RGB(232, 246, 245)},
				NativeContextMenuEnabled: false,
				StretchFactor:            1,
				OnDocumentCompleted: func(string) {
					a.showInitialWindow(web)
				},
			},
		},
	}
	if err := ui.Create(); err != nil {
		return err
	}
	a.web = web
	a.mw.ToolBar().SetVisible(false)
	a.mw.StatusBar().SetVisible(false)
	a.bindWebViewResizeGuard(web)
	a.scheduleInitialShow(web, 900*time.Millisecond)
	a.mw.Run()
	return nil
}

func (a *App) bindWebViewResizeGuard(web *walk.WebView) {
	if a.mw == nil || web == nil {
		return
	}
	resize := func() {
		a.requestWebViewLayout(web, false, 70*time.Millisecond, 180*time.Millisecond)
	}
	a.mw.SizeChanged().Attach(resize)
	a.mw.BoundsChanged().Attach(resize)
	a.requestWebViewLayout(web, true, 70*time.Millisecond)
}

func (a *App) scheduleInitialShow(web *walk.WebView, delay time.Duration) {
	time.AfterFunc(delay, func() {
		defer func() { _ = recover() }()
		if a.mw == nil || a.mw.Handle() == 0 {
			return
		}
		a.mw.Synchronize(func() {
			a.showInitialWindow(web)
		})
	})
}

func (a *App) showInitialWindow(web *walk.WebView) {
	if a.mw == nil || a.initialShown {
		return
	}
	a.initialShown = true
	a.mw.Show()
	a.requestWebViewLayout(web, true, 80*time.Millisecond)
	a.redrawWindow()
}

func (a *App) requestWebViewLayout(web *walk.WebView, force bool, delays ...time.Duration) {
	seq := atomic.AddUint64(&a.webLayoutSeq, 1)
	a.fillWebView(web, force)
	for _, delay := range delays {
		a.scheduleWebViewLayout(web, seq, force, delay)
	}
}

func (a *App) scheduleWebViewLayout(web *walk.WebView, seq uint64, force bool, delay time.Duration) {
	time.AfterFunc(delay, func() {
		defer func() { _ = recover() }()
		if atomic.LoadUint64(&a.webLayoutSeq) != seq || a.mw == nil || web == nil || a.mw.Handle() == 0 || web.Handle() == 0 {
			return
		}
		a.mw.Synchronize(func() {
			if atomic.LoadUint64(&a.webLayoutSeq) == seq {
				a.fillWebView(web, force)
				a.redrawWindow()
			}
		})
	})
}

func (a *App) fillWebView(web *walk.WebView, force bool) {
	if a.mw == nil || web == nil || a.mw.Handle() == 0 || web.Handle() == 0 {
		return
	}
	if !win.IsWindowVisible(a.mw.Handle()) || win.IsIconic(a.mw.Handle()) {
		return
	}
	bounds := a.mw.ClientBoundsPixels()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return
	}
	rect := walk.Rectangle{Width: bounds.Width, Height: bounds.Height}
	if !force && rect == a.lastWebBounds {
		return
	}
	if parent := web.Parent(); parent != nil {
		_ = parent.SetBoundsPixels(rect)
	}
	_ = web.SetBoundsPixels(rect)
	a.lastWebBounds = rect
}

func (a *App) redrawWindow() {
	if a.mw == nil || a.mw.Handle() == 0 || !win.IsWindowVisible(a.mw.Handle()) || win.IsIconic(a.mw.Handle()) {
		return
	}
	win.RedrawWindow(
		a.mw.Handle(),
		nil,
		0,
		win.RDW_INVALIDATE|win.RDW_UPDATENOW|win.RDW_ALLCHILDREN|win.RDW_NOERASE,
	)
}

func (a *App) startAdminServer() (*http.Server, error) {
	mux := http.NewServeMux()
	a.registerAdminRoutes(mux)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	a.panelURL = "http://" + ln.Addr().String() + "/"
	tmpDir := filepath.Join(appTmpDir(a.workspace), "gui")
	_ = os.MkdirAll(tmpDir, 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "server-gui.url"), []byte(a.panelURL), 0o644)

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			a.appendLog("管理台服务异常: " + serveErr.Error())
		}
	}()
	return srv, nil
}

func (a *App) appendLog(msg string) {
	line := fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), msg)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logs = append(a.logs, line)
	if len(a.logs) > 300 {
		a.logs = append([]string{}, a.logs[len(a.logs)-300:]...)
	}
}

func (a *App) snapshotLogs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	logs := append([]string{}, a.logs...)
	if tail := serviceLogTail(a.logService, 80); len(tail) > 0 {
		logs = append(logs, "---- 服务日志 ----")
		logs = append(logs, tail...)
	}
	return logs
}

func enableWebViewIE11Mode() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Internet Explorer\Main\FeatureControl\FEATURE_BROWSER_EMULATION`, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.SetDWordValue(filepath.Base(exe), 11001)
}

func detectWorkspaceRoot() string {
	if env := strings.TrimSpace(os.Getenv("LSYL_TUNNEL_WORKSPACE")); env != "" {
		if info, err := os.Stat(env); err == nil && info.IsDir() {
			return env
		}
	}
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		if root := findWorkspaceFrom(start); root != "" {
			return root
		}
	}
	if len(starts) > 0 {
		return starts[0]
	}
	return "."
}

func findWorkspaceFrom(start string) string {
	cur := filepath.Clean(start)
	for i := 0; i < 8; i++ {
		if fileExists(filepath.Join(cur, "go.mod")) ||
			fileExists(filepath.Join(cur, "src", "server", "conf", "server.yaml")) ||
			fileExists(filepath.Join(cur, "src", "client", "conf", "client.yaml")) ||
			fileExists(filepath.Join(cur, "server", "conf", "server.yaml")) ||
			fileExists(filepath.Join(cur, "client", "conf", "client.yaml")) ||
			fileExists(filepath.Join(cur, "conf", "server.yaml")) ||
			fileExists(filepath.Join(cur, "conf", "client.yaml")) {
			return cur
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return ""
}

func detectServerServiceExe(workspace string) string {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "lsyl-tunnel-server-svc.exe")
		if fileExists(candidate) {
			return candidate
		}
	}
	if candidate := filepath.Join(workspace, "bin", "lsyl-tunnel-server-svc.exe"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "build", "bin", "server", "lsyl-tunnel-server-svc.exe"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "server", "bin", "lsyl-tunnel-server-svc.exe"); fileExists(candidate) {
		return candidate
	}
	return filepath.Join(workspace, "build", "bin", "server", "lsyl-tunnel-server-svc.exe")
}

func detectServerConfigPath(workspace string) string {
	if candidate := filepath.Join(workspace, "src", "server", "conf", "server.yaml"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "server", "conf", "server.yaml"); fileExists(candidate) {
		return candidate
	}
	if candidate := filepath.Join(workspace, "conf", "server.yaml"); fileExists(candidate) {
		return candidate
	}
	return filepath.Join(workspace, "src", "server", "conf", "server.yaml")
}

func detectServerServiceLog(workspace string) string {
	if fileExists(filepath.Join(workspace, "bin", "lsyl-tunnel-server-svc.exe")) {
		return filepath.Join(workspace, "logs", "service", "server-service.log")
	}
	return filepath.Join(workspace, "runtime", "logs", "service", "server-service.log")
}

func appTmpDir(workspace string) string {
	if fileExists(filepath.Join(workspace, "go.mod")) {
		return filepath.Join(workspace, "build", "tmp")
	}
	return filepath.Join(workspace, "tmp")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
