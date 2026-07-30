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
	serverVersion := ""
	if err == nil {
		timeout := time.Duration(cfg.Connection.DialTimeoutSec+3) * time.Second
		if timeout < 5*time.Second {
			timeout = 5 * time.Second
		}
		checkCtx, cancel := context.WithTimeout(a.rootCtx, timeout)
		resp, checkErr := tunnel.CheckLoginResponse(checkCtx, cfg)
		err = checkErr
		serverVersion = resp.ServerVersion
		cancel()
	}

	var client *tunnel.Client
	var stop context.CancelFunc
	if err == nil {
		runCtx, cancel := context.WithCancel(a.rootCtx)
		client, err = tunnel.StartVerified(runCtx, cfg, serverVersion, func(format string, args ...any) {
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
		if fwd.Direction == tunnel.DirectionServerToClient {
			lines = append(lines, fmt.Sprintf("%s: %s <- %s", name, fwd.ServerTarget, fwd.ListenAddr))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s -> 服务端 %s", name, fwd.ListenAddr, fwd.ServerTarget))
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
	case strings.Contains(text, "auth_blocked") || strings.Contains(text, "too many login failures"):
		return "登录失败次数过多，当前来源 IP 已被临时封禁，请稍后再试。"
	case strings.Contains(text, "user_stream_limit") || strings.Contains(text, "too many concurrent streams"):
		return "当前账号并发连接数已达到上限，请关闭部分连接后重试。"
	case strings.Contains(text, "client_version_unsupported"):
		return "客户端版本不在服务端允许范围内，请联系管理员确认两端版本。"
	case strings.Contains(text, "client config_version") && strings.Contains(text, "requires a newer client") ||
		(strings.Contains(text, "client config requires version") && strings.Contains(text, "current version")):
		return "当前配置需要更高版本的客户端，请升级后重新导入配置。"
	case (strings.Contains(text, "field ") && strings.Contains(text, "not found in type")) ||
		strings.Contains(text, "contains unknown field") || strings.Contains(text, "missing required field") ||
		strings.Contains(text, "duplicate field") || strings.Contains(text, "must be a mapping") ||
		strings.Contains(text, "must be a list") || strings.Contains(text, "exactly one yaml document"):
		return "配置文件结构与当前客户端不兼容，请重新导出并导入配置。"
	case strings.Contains(text, "protocol_version_unsupported") || strings.Contains(text, "bad_request"):
		return "客户端与服务端协议版本不一致，请联系管理员升级对应一端。"
	case strings.Contains(text, "server_sealed") || strings.Contains(text, "saved_credential"):
		return "配置里的登录凭据不可用，请重新导出 .lsylprofile。"
	case strings.Contains(text, "username is required"):
		return "导入文件缺少用户名，请重新导出 .lsylprofile。"
	case strings.Contains(text, "insecure_skip_verify") || strings.Contains(text, "tls.min_version"):
		return "配置不符合轻量客户端安全要求，请重新导出 .lsylprofile。"
	case strings.Contains(text, "server certificate") && (strings.Contains(text, "no such file") || strings.Contains(text, "cannot find") || strings.Contains(text, "系统找不到")):
		return "服务端证书文件缺失，请重新导入 .lsylprofile。"
	case strings.Contains(text, "certificate has expired") || strings.Contains(text, "not yet valid"):
		return "服务端证书已过期或尚未生效，请重新导出 .lsylprofile。"
	case strings.Contains(text, "certificate is valid for") || strings.Contains(text, "cannot validate certificate"):
		return "服务端证书和当前地址不匹配，请联系管理员检查服务端地址或证书。"
	case strings.Contains(text, "unknown authority") || strings.Contains(text, "not trusted"):
		return "服务端证书不受信任，请重新导出 .lsylprofile。"
	case strings.Contains(text, "certificate") || strings.Contains(text, "x509") || strings.Contains(text, "tls"):
		return "服务端证书校验失败，请确认配置和服务器地址。"
	case strings.Contains(text, "target_denied") || strings.Contains(text, "not allowed to access"):
		return "当前账号没有访问该目标的权限。"
	case strings.Contains(text, "target_unreachable") || strings.Contains(text, "target service is unreachable"):
		return "服务端无法访问目标服务，请联系管理员检查目标服务或防火墙。"
	case strings.Contains(text, "server_addr is invalid") || (strings.Contains(text, "dial tcp") && (strings.Contains(text, "missing port in address") || strings.Contains(text, "too many colons in address"))):
		return "服务端地址格式不正确，应填写为 地址:端口。"
	case strings.Contains(text, "already in use") || strings.Contains(text, "only one usage") || strings.Contains(text, "bind:"):
		return "本地端口已被占用，请关闭占用程序后重试。"
	case strings.Contains(text, "connection refused") || strings.Contains(text, "actively refused") || strings.Contains(text, "connectex"):
		return "连接不上服务端，请检查服务端是否启动、地址和端口是否正确。"
	case strings.Contains(text, "timeout") || strings.Contains(text, "deadline"):
		return "连接超时，请检查网络或防火墙。"
	case strings.Contains(text, "no such host"):
		return "服务端地址无法解析，请检查域名或网络。"
	case strings.Contains(text, "network is unreachable") || strings.Contains(text, "no route to host"):
		return "网络不可达，请检查本机网络和服务端地址。"
	case strings.Contains(text, "connection reset") || strings.Contains(text, "forcibly closed") || strings.Contains(text, "wsarecv") || strings.Contains(text, "eof"):
		return "连接被服务端断开，请稍后重试或联系管理员。"
	case strings.Contains(text, "profile package is missing"):
		return "导入文件不完整，缺少 profile.json 或 server.crt。"
	case strings.Contains(text, "mobile profile only supports client_to_server"):
		return "轻量客户端只支持正向转发，请重新导出 .lsylprofile。"
	case strings.Contains(text, "at least one client_to_server forward"):
		return "导入文件没有可用的正向转发规则，请重新导出 .lsylprofile。"
	case strings.Contains(text, "listen_addr must use 127.0.0.1"):
		return "轻量客户端的本地监听必须使用 127.0.0.1，请重新导出 .lsylprofile。"
	case strings.Contains(text, "below 1024"):
		return "轻量客户端的本地监听端口必须大于等于 1024，请重新导出 .lsylprofile。"
	case strings.Contains(text, "listen_addr is invalid"):
		return "本地监听地址格式不正确，应填写为 127.0.0.1:端口。"
	case strings.Contains(text, "server_target is invalid"):
		return "转发目标地址格式不正确，应填写为 地址:端口。"
	case strings.Contains(text, "duplicate forward name"):
		return "导入文件包含重名的转发规则，请重新导出 .lsylprofile。"
	case strings.Contains(text, "duplicate mobile listen address"):
		return "导入文件包含重复的本地监听端口，请重新导出 .lsylprofile。"
	case strings.Contains(text, "parse profile.json") || strings.Contains(text, "unsupported profile version") || strings.Contains(text, "mobile profile must not contain"):
		return "导入文件格式或版本不受支持，请使用当前客户端重新导出 .lsylprofile。"
	case strings.Contains(text, "invalid profile file") || strings.Contains(text, "read profile file") || strings.Contains(text, "open .lsylprofile"):
		return "无法读取 .lsylprofile，请确认文件未损坏且未被其他程序占用。"
	case strings.Contains(text, "no such file") || strings.Contains(text, "cannot find") || strings.Contains(text, "系统找不到"):
		return "配置文件或证书文件缺失，请重新导入配置。"
	default:
		return "操作失败，暂时无法识别具体原因；请重试，若持续出现请联系管理员查看客户端日志。"
	}
}
