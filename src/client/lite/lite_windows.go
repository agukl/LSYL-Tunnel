//go:build windows

package lite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"lsyltunnel/src/client/mobileprofile"
	"lsyltunnel/src/client/tunnel"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const windowTitle = "LSYL Tunnel Lite"

type profileStore struct {
	Root       string
	ConfigPath string
	CertPath   string
}

type App struct {
	store      profileStore
	rootCtx    context.Context
	rootCancel context.CancelFunc

	mw               *walk.MainWindow
	statusLabel      *walk.TextLabel
	profileLabel     *walk.TextLabel
	routesEdit       *walk.TextEdit
	logEdit          *walk.TextEdit
	importButton     *walk.PushButton
	connectButton    *walk.PushButton
	disconnectButton *walk.PushButton

	mu      sync.Mutex
	client  *tunnel.Client
	stop    context.CancelFunc
	busy    bool
	closing bool
}

func Run() error {
	app, err := NewApp()
	if err != nil {
		return err
	}
	return app.Run()
}

func NewApp() (*App, error) {
	store, err := defaultProfileStore()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &App{store: store, rootCtx: ctx, rootCancel: cancel}, nil
}

func (a *App) Run() error {
	ui := MainWindow{
		AssignTo:   &a.mw,
		Title:      windowTitle,
		Size:       Size{480, 520},
		MinSize:    Size{440, 460},
		Background: SolidColorBrush{Color: walk.RGB(244, 249, 248)},
		Font:       Font{Family: "Microsoft YaHei", PointSize: 9},
		Layout:     VBox{Margins: Margins{18, 16, 18, 16}, Spacing: 8},
		Children: []Widget{
			TextLabel{
				Text:      "LSYL Tunnel Lite",
				Font:      Font{Family: "Microsoft YaHei", PointSize: 14, Bold: true},
				TextColor: walk.RGB(20, 88, 92),
			},
			TextLabel{
				AssignTo:  &a.statusLabel,
				Text:      "未连接",
				TextColor: walk.RGB(46, 74, 78),
			},
			TextLabel{
				Text:      "配置",
				Font:      Font{Family: "Microsoft YaHei", PointSize: 10, Bold: true},
				TextColor: walk.RGB(55, 64, 68),
			},
			TextLabel{
				AssignTo: &a.profileLabel,
				MinSize:  Size{400, 54},
				NoPrefix: true,
				Text:     "未导入配置",
			},
			TextLabel{
				Text:      "端口映射",
				Font:      Font{Family: "Microsoft YaHei", PointSize: 10, Bold: true},
				TextColor: walk.RGB(55, 64, 68),
			},
			TextEdit{
				AssignTo: &a.routesEdit,
				ReadOnly: true,
				MinSize:  Size{400, 90},
				VScroll:  true,
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					PushButton{
						AssignTo:  &a.importButton,
						Text:      "导入配置",
						MinSize:   Size{120, 34},
						OnClicked: a.importProfile,
					},
					PushButton{
						AssignTo:  &a.connectButton,
						Text:      "连接",
						MinSize:   Size{100, 34},
						OnClicked: a.connect,
					},
					PushButton{
						AssignTo:  &a.disconnectButton,
						Text:      "断开",
						MinSize:   Size{100, 34},
						OnClicked: a.disconnect,
					},
				},
			},
			TextLabel{
				Text:      "日志",
				Font:      Font{Family: "Microsoft YaHei", PointSize: 10, Bold: true},
				TextColor: walk.RGB(55, 64, 68),
			},
			TextEdit{
				AssignTo:      &a.logEdit,
				ReadOnly:      true,
				VScroll:       true,
				StretchFactor: 1,
			},
		},
	}
	if err := ui.Create(); err != nil {
		return err
	}
	a.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		a.shutdown()
	})
	a.reloadProfileUI()
	a.appendLogUI("配置目录: " + a.store.Root)
	a.refreshButtonsUI()
	a.mw.Run()
	return nil
}

func defaultProfileStore() (profileStore, error) {
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		base = strings.TrimSpace(os.Getenv("APPDATA"))
	}
	if strings.TrimSpace(base) == "" {
		return profileStore{}, errors.New("无法定位当前用户配置目录")
	}
	root := filepath.Join(base, "LSYL Tunnel Lite")
	return profileStore{
		Root:       root,
		ConfigPath: filepath.Join(root, "conf", "client.yaml"),
		CertPath:   filepath.Join(root, "cert", "server.crt"),
	}, nil
}

func (a *App) importProfile() {
	if a.isRunningOrBusy() {
		walk.MsgBox(a.mw, windowTitle, "请先断开当前连接，再导入新配置。", walk.MsgBoxIconInformation)
		return
	}
	dlg := new(walk.FileDialog)
	dlg.Title = "导入 LSYL Profile"
	dlg.Filter = "LSYL Profile (*.lsylprofile)|*.lsylprofile|All files (*.*)|*.*"
	accepted, err := dlg.ShowOpen(a.mw)
	if err != nil {
		walk.MsgBox(a.mw, windowTitle, friendlyLiteError(err), walk.MsgBoxIconError)
		return
	}
	if !accepted {
		return
	}
	imported, err := mobileprofile.ImportFile(dlg.FilePath)
	if err != nil {
		walk.MsgBox(a.mw, windowTitle, friendlyLiteError(err), walk.MsgBoxIconError)
		return
	}
	if err := a.saveImportedProfile(imported); err != nil {
		walk.MsgBox(a.mw, windowTitle, friendlyLiteError(err), walk.MsgBoxIconError)
		return
	}
	a.reloadProfileUI()
	a.appendLogUI("已导入配置: " + filepath.Base(dlg.FilePath))
	a.refreshButtonsUI()
	walk.MsgBox(a.mw, windowTitle, "配置已导入，可以点击“连接”。", walk.MsgBoxIconInformation)
}

func (a *App) saveImportedProfile(imported mobileprofile.ImportedProfile) error {
	cfg := imported.Config
	cfg.Password = ""
	cfg.PasswordEnv = ""
	cfg.PasswordFile = ""
	cfg.TLS.CACertFile = "../cert/server.crt"
	tunnel.ApplyDefaults(&cfg)
	if err := os.MkdirAll(filepath.Dir(a.store.CertPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(a.store.CertPath, imported.CertPEM, 0o644); err != nil {
		return fmt.Errorf("写入服务端证书失败: %w", err)
	}
	if err := tunnel.SaveConfig(a.store.ConfigPath, cfg); err != nil {
		return fmt.Errorf("写入客户端配置失败: %w", err)
	}
	return nil
}

func (a *App) connect() {
	a.mu.Lock()
	if a.closing || a.busy || a.client != nil {
		a.mu.Unlock()
		return
	}
	a.busy = true
	a.mu.Unlock()

	a.statusLabel.SetText("正在连接...")
	a.refreshButtonsUI()
	a.appendLogUI("开始连接")

	go a.connectAsync()
}

func (a *App) connectAsync() {
	cfg, err := tunnel.LoadConfig(a.store.ConfigPath)
	if err == nil {
		timeout := time.Duration(cfg.Connection.DialTimeoutSec+3) * time.Second
		if timeout < 5*time.Second {
			timeout = 5 * time.Second
		}
		checkCtx, cancel := context.WithTimeout(a.rootCtx, timeout)
		err = tunnel.CheckLogin(checkCtx, cfg)
		cancel()
	}

	var client *tunnel.Client
	var stop context.CancelFunc
	if err == nil {
		runCtx, cancel := context.WithCancel(a.rootCtx)
		client, err = tunnel.Start(runCtx, cfg, func(format string, args ...any) {
			a.appendLog("%s", fmt.Sprintf(format, args...))
		})
		if err != nil {
			cancel()
		} else {
			stop = cancel
		}
	}

	if err != nil {
		a.finishConnectError(err)
		return
	}

	a.mu.Lock()
	if a.closing {
		a.busy = false
		a.mu.Unlock()
		if stop != nil {
			stop()
		}
		_ = client.Close()
		return
	}
	a.client = client
	a.stop = stop
	a.busy = false
	a.mu.Unlock()

	a.runUI(func() {
		a.statusLabel.SetText("已连接")
		a.appendLogUI("连接成功")
		a.refreshButtonsUI()
	})
	go a.watchClientDone(client)
}

func (a *App) finishConnectError(err error) {
	if a.isClosing() {
		return
	}
	a.mu.Lock()
	a.busy = false
	a.mu.Unlock()
	a.runUI(func() {
		a.statusLabel.SetText("连接失败")
		a.appendLogUI("连接失败: " + friendlyLiteError(err))
		a.refreshButtonsUI()
		walk.MsgBox(a.mw, windowTitle, friendlyLiteError(err), walk.MsgBoxIconError)
	})
}

func (a *App) disconnect() {
	client, stop, _ := a.detachClient(nil)
	if stop != nil {
		stop()
	}
	if client != nil {
		_ = client.Close()
		a.appendLogUI("已断开连接")
	}
	a.statusLabel.SetText("未连接")
	a.refreshButtonsUI()
}

func (a *App) watchClientDone(client *tunnel.Client) {
	if client == nil || client.Done() == nil {
		return
	}
	<-client.Done()
	if a.isClosing() {
		return
	}
	stats := client.Stats()
	_, stop, ok := a.detachClient(client)
	if !ok {
		return
	}
	if stop != nil {
		stop()
	}
	message := stats.Health.Message
	if message == "" {
		message = "连接已断开"
	}
	a.runUI(func() {
		a.statusLabel.SetText("未连接")
		a.appendLogUI(message)
		a.refreshButtonsUI()
	})
}

func (a *App) detachClient(expected *tunnel.Client) (*tunnel.Client, context.CancelFunc, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if expected != nil && a.client != expected {
		return nil, nil, false
	}
	client := a.client
	stop := a.stop
	a.client = nil
	a.stop = nil
	a.busy = false
	return client, stop, true
}

func (a *App) shutdown() {
	a.mu.Lock()
	if a.closing {
		a.mu.Unlock()
		return
	}
	a.closing = true
	client := a.client
	stop := a.stop
	rootCancel := a.rootCancel
	a.client = nil
	a.stop = nil
	a.busy = false
	a.mu.Unlock()

	if rootCancel != nil {
		rootCancel()
	}
	if stop != nil {
		stop()
	}
	if client != nil {
		_ = client.Close()
	}
}

func (a *App) reloadProfileUI() {
	cfg, err := tunnel.LoadConfig(a.store.ConfigPath)
	if err != nil {
		a.profileLabel.SetText("未导入配置")
		a.routesEdit.SetText("请导入管理员下发的 .lsylprofile 文件。")
		return
	}
	a.profileLabel.SetText(profileSummary(cfg))
	a.routesEdit.SetText(routesSummary(cfg))
}

func profileSummary(cfg tunnel.Config) string {
	expiry := "未知"
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(cfg.SavedCredential.ExpiresAt)); err == nil {
		expiry = t.Local().Format("2006-01-02 15:04")
	}
	return fmt.Sprintf("用户: %s\r\n服务端: %s\r\n凭据有效期: %s", cfg.Username, cfg.ServerAddr, expiry)
}

func routesSummary(cfg tunnel.Config) string {
	if len(cfg.Forwards) == 0 {
		return "未配置端口映射。"
	}
	lines := make([]string, 0, len(cfg.Forwards))
	for _, fwd := range cfg.Forwards {
		name := strings.TrimSpace(fwd.Name)
		if name == "" {
			name = "forward"
		}
		lines = append(lines, fmt.Sprintf("%s: 本机 %s -> 服务端 %s", name, fwd.ListenAddr, fwd.ServerTarget))
	}
	return strings.Join(lines, "\r\n")
}

func (a *App) refreshButtonsUI() {
	a.mu.Lock()
	running := a.client != nil
	busy := a.busy
	closing := a.closing
	a.mu.Unlock()
	hasProfile := fileExists(a.store.ConfigPath) && fileExists(a.store.CertPath)
	a.importButton.SetEnabled(!running && !busy && !closing)
	a.connectButton.SetEnabled(hasProfile && !running && !busy && !closing)
	a.disconnectButton.SetEnabled(running && !busy && !closing)
}

func (a *App) appendLog(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	a.runUI(func() {
		a.appendLogUI(message)
	})
}

func (a *App) appendLogUI(message string) {
	if a.logEdit == nil {
		return
	}
	line := fmt.Sprintf("[%s] %s\r\n", time.Now().Format("15:04:05"), message)
	a.logEdit.AppendText(line)
	a.logEdit.ScrollToCaret()
}

func (a *App) runUI(f func()) {
	if a.isClosing() || a.mw == nil {
		return
	}
	a.mw.Synchronize(func() {
		if !a.isClosing() {
			f()
		}
	})
}

func (a *App) isRunningOrBusy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client != nil || a.busy
}

func (a *App) isClosing() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closing
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func friendlyLiteError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case errors.Is(err, context.Canceled):
		return "操作已取消。"
	case strings.Contains(text, "credential_expired") || strings.Contains(text, "saved login has expired") || strings.Contains(text, "saved_credential has expired"):
		return "登录凭据已过期，请让客户端重新生成配置后再导入。"
	case strings.Contains(text, "auth_failed") || strings.Contains(text, "username or password"):
		return "登录凭据无效，请确认账号或重新导入配置。"
	case strings.Contains(text, "server_sealed") || strings.Contains(text, "saved_credential"):
		return "配置里的登录凭据不可用，请重新导出 .lsylprofile。"
	case strings.Contains(text, "insecure_skip_verify") || strings.Contains(text, "tls.min_version"):
		return "配置不符合移动端安全要求，请重新导出。"
	case strings.Contains(text, "certificate") || strings.Contains(text, "x509") || strings.Contains(text, "tls"):
		return "服务端证书校验失败，请确认配置和服务器地址。"
	case strings.Contains(text, "already in use") || strings.Contains(text, "only one usage") || strings.Contains(text, "bind:"):
		return "本地端口已被占用，请关闭占用程序后重试。"
	case strings.Contains(text, "connection refused") || strings.Contains(text, "actively refused") || strings.Contains(text, "connectex"):
		return "连接不上服务端，请检查服务端是否启动、地址和端口是否正确。"
	case strings.Contains(text, "timeout") || strings.Contains(text, "deadline"):
		return "连接超时，请检查网络或防火墙。"
	case strings.Contains(text, "no such host"):
		return "服务端地址无法解析，请检查域名或网络。"
	case strings.Contains(text, "no such file") || strings.Contains(text, "cannot find") || strings.Contains(text, "系统找不到"):
		return "配置文件或证书文件缺失，请重新导入配置。"
	case strings.Contains(text, "profile package is missing"):
		return "导入文件不完整，缺少 profile.json 或 server.crt。"
	case strings.Contains(text, "client_to_server") || strings.Contains(text, "127.0.0.1") || strings.Contains(text, "below 1024"):
		return "导入文件包含轻量客户端不支持的端口映射，请重新导出移动端配置。"
	default:
		return err.Error()
	}
}
