//go:build windows

package gui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"lsyltunnel/src/client/tunnel"
)

func (a *App) startClient(cfg tunnel.Config) error {
	return a.startClientEmbedded(cfg)
}

func (a *App) startClientEmbedded(cfg tunnel.Config) error {
	a.mu.Lock()
	if a.tun != nil {
		a.mu.Unlock()
		return fmt.Errorf("客户端已经在后台值守")
	}
	a.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	client, err := tunnel.Start(ctx, cfg, func(format string, args ...any) {
		a.appendLog(fmt.Sprintf(format, args...))
	})
	if err != nil {
		cancel()
		return err
	}

	a.mu.Lock()
	if a.tun != nil {
		a.mu.Unlock()
		cancel()
		_ = client.Close()
		return fmt.Errorf("客户端已经在后台值守")
	}
	a.tun = client
	a.stop = cancel
	a.mu.Unlock()

	go a.watchClientDone(client)
	a.appendLog("客户端已在当前窗口内启动后台值守")
	a.updateTrayToolTip()
	return nil
}

func (a *App) watchClientDone(client *tunnel.Client) {
	if client == nil || client.Done() == nil {
		return
	}
	<-client.Done()
	stats := client.Stats()
	message := terminalDisconnectMessage(stats)
	a.detachClient(client, message, true)
}

func (a *App) stopClient() error {
	if a.isEmbeddedRunning() {
		return a.stopClientEmbedded()
	}
	return fmt.Errorf("客户端未运行")
}

func (a *App) stopClientEmbedded() error {
	a.mu.Lock()
	client := a.tun
	cancel := a.stop
	if client == nil {
		a.mu.Unlock()
		return fmt.Errorf("客户端未运行")
	}
	a.tun = nil
	a.stop = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	err := client.Close()
	a.appendLog("连接已停止")
	a.updateTrayToolTip()
	return err
}

func (a *App) detachClient(client *tunnel.Client, message string, bad bool) {
	a.mu.Lock()
	if a.tun != client {
		a.mu.Unlock()
		return
	}
	cancel := a.stop
	a.tun = nil
	a.stop = nil
	a.notice = message
	a.noticeBad = bad
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if message != "" {
		a.appendLog("连接状态已取消: " + message)
	}
	a.updateTrayToolTip()
	if shouldClearSavedPasswordStateText(message) {
		a.clearSavedPasswordStateAfterAuthFailure()
	}
}

func terminalDisconnectMessage(stats tunnel.ClientStats) string {
	if stats.Health.Message != "" {
		return stats.Health.Message
	}
	switch stats.Health.State {
	case tunnel.HealthAuthError:
		return "认证异常，已取消连接状态，请重新连接"
	case tunnel.HealthServerUnavailable:
		return "多次重连失败，已取消连接状态，请确认服务端恢复后重新连接"
	default:
		return "连接已取消，请重新连接"
	}
}

func (a *App) isRunning() bool {
	return a.isEmbeddedRunning()
}

func (a *App) isEmbeddedRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tun != nil
}

func (a *App) client() *tunnel.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tun
}

func (a *App) clientStats() tunnel.ClientStats {
	client := a.client()
	if client == nil {
		return tunnel.ClientStats{}
	}
	return client.Stats()
}

func (a *App) appendLog(msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	line := fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), msg)
	a.logs = append(a.logs, line)
	if len(a.logs) > 200 {
		a.logs = append([]string(nil), a.logs[len(a.logs)-200:]...)
	}
}

func (a *App) setNotice(message string, bad bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notice = message
	a.noticeBad = bad
}

func (a *App) noticeSnapshot() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.notice, a.noticeBad
}

func (a *App) runtimeStatus() string {
	client := a.client()
	if client != nil {
		return runtimeStatusText(client.Stats())
	}
	return "未连接"
}

func runtimeStatusText(stats tunnel.ClientStats) string {
	switch stats.Health.State {
	case tunnel.HealthOK:
		if hasForwardIssue(stats) {
			return "部分异常"
		}
		return "已连接"
	case tunnel.HealthServerUnavailable:
		return "正在重连"
	case tunnel.HealthAuthError:
		return "认证异常"
	case tunnel.HealthChecking:
		return "检查服务端状态中"
	default:
		if hasForwardIssue(stats) {
			return "部分异常"
		}
		return "后台值守中"
	}
}

func hasForwardIssue(stats tunnel.ClientStats) bool {
	for _, item := range stats.Items {
		switch item.State {
		case tunnel.ForwardListenFailed, tunnel.ForwardRetrying, tunnel.ForwardRejected:
			return true
		}
	}
	return false
}

func ptrAPIState(state apiState) *apiState {
	return &state
}

func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	return friendlyErrorText(err.Error())
}

func friendlyErrorText(raw string) string {
	text := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case text == "":
		return ""
	case strings.Contains(text, "客户端已经") || (strings.Contains(text, "already") && strings.Contains(text, "running")):
		return "已经连接，正在后台值守"
	case strings.Contains(text, "客户端未运行"):
		return "当前没有正在运行的连接"
	case strings.Contains(text, "管理员授权已取消"):
		return "管理员授权已取消"
	case strings.Contains(text, "no server tls trust data") || strings.Contains(text, "appendcertsfrompem"):
		return "服务端信任证书无效，请联系管理员重新下发。"
	case isMissingServerCertText(text) || strings.Contains(text, "ca_cert_file") || strings.Contains(text, "server verification"):
		return "缺少服务端信任证书 server.crt，请联系管理员重新下发客户端安装包。"
	case strings.Contains(text, "certificate is valid for") || strings.Contains(text, "cannot validate certificate") || strings.Contains(text, "doesn't contain any ip sans"):
		return "服务端证书和当前地址不匹配，请检查服务端地址或重新下发证书。"
	case strings.Contains(text, "unknown authority") || strings.Contains(text, "not trusted"):
		return "服务端证书不受信任，请联系管理员重新下发证书。"
	case strings.Contains(text, "certificate") || strings.Contains(text, "x509") || strings.Contains(text, "tls"):
		return "服务端证书校验失败，请联系管理员检查证书和地址。"
	case strings.Contains(text, "username or password") || strings.Contains(text, "auth_failed"):
		return "用户名或密码不正确。"
	case strings.Contains(text, "credential_expired") || strings.Contains(text, "saved login has expired"):
		return "保存的登录凭据已过期，请重新输入密码。"
	case strings.Contains(text, "username and password are required"):
		return "请输入用户名和密码。"
	case strings.Contains(text, "too many") || strings.Contains(text, "blocked"):
		return "登录失败次数过多，请稍后再试。"
	case strings.Contains(text, "server_addr is required"):
		return "请输入服务端地址。"
	case strings.Contains(text, "no such host"):
		return "服务端地址无法解析，请检查域名或网络。"
	case strings.Contains(text, "already in use") || strings.Contains(text, "only one usage") || strings.Contains(text, "bind:"):
		return "本地端口已被占用，请关闭占用程序或调整端口。"
	case strings.Contains(text, "at least one forward") || strings.Contains(text, "requires listen_addr"):
		return "未配置端口转发，请联系管理员检查配置。"
	case strings.Contains(text, "no usable forward"):
		return "没有可用的端口映射，请检查本地端口是否被占用。"
	case strings.Contains(text, "password_file") || strings.Contains(text, "read password"):
		return "无法读取密码文件，请检查路径和权限。"
	case strings.Contains(text, "target_denied") || strings.Contains(text, "not allowed") || strings.Contains(text, "not allowed to access"):
		return "当前账号没有访问该目标的权限。"
	case strings.Contains(text, "target_unreachable") || strings.Contains(text, "target service is unreachable"):
		return "服务端无法访问目标服务，请联系管理员检查目标服务或防火墙。"
	case strings.Contains(text, "invalid tunnel request") || strings.Contains(text, "unsupported request") || strings.Contains(text, "bad_request"):
		return "客户端和服务端协议不匹配，请确认版本一致。"
	case strings.Contains(text, "connection refused") || strings.Contains(text, "actively refused") || strings.Contains(text, "connectex"):
		return "连接不上服务端，请检查服务端是否启动或地址端口是否正确。"
	case strings.Contains(text, "timeout") || strings.Contains(text, "deadline") || strings.Contains(text, "i/o timeout"):
		return "连接超时，请检查网络或服务端防火墙。"
	case strings.Contains(text, "network is unreachable") || strings.Contains(text, "no route to host"):
		return "网络不可达，请检查本机网络。"
	case strings.Contains(text, "connection reset") || strings.Contains(text, "forcibly closed") || strings.Contains(text, "wsarecv") || strings.Contains(text, "eof"):
		return "连接被服务端断开，请稍后重试或联系管理员。"
	case strings.Contains(text, "access is denied") || strings.Contains(text, "拒绝访问"):
		return "权限不足，请以管理员身份运行或检查安装目录权限。"
	case strings.Contains(text, "yaml") || strings.Contains(text, "cannot unmarshal") || strings.Contains(text, "did not find expected"):
		return "配置文件格式不正确，请联系管理员检查配置。"
	case strings.Contains(text, "no such file") || strings.Contains(text, "cannot find") || strings.Contains(text, "找不到"):
		return "客户端文件不完整，请联系管理员检查安装包。"
	default:
		return "连接失败，请检查服务端地址、账号密码和网络后重试。"
	}
}

func shouldClearSavedPasswordState(err error) bool {
	if err == nil {
		return false
	}
	return shouldClearSavedPasswordStateText(err.Error())
}

func shouldClearSavedPasswordStateText(raw string) bool {
	text := strings.ToLower(strings.TrimSpace(raw))
	return containsAnyText(
		text,
		"username or password",
		"auth_failed",
		"credential_expired",
		"saved login has expired",
		"账号或密码",
		"用户名或密码",
		"保存的登录凭据",
	)
}

func (a *App) clearSavedPasswordStateAfterAuthFailure() {
	if err := a.clearSavedPasswordState(); err != nil {
		a.appendLog("清理已保存登录凭据失败: " + err.Error())
		return
	}
	a.appendLog("已清理本地保存的登录凭据，请重新输入密码")
}

func isMissingServerCertText(text string) bool {
	if !containsAnyText(text, "server.crt", "ca_cert_file") {
		return false
	}
	return containsAnyText(text, "no such file", "cannot find", "找不到", "系统找不到")
}

func containsAnyText(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
