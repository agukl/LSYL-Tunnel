//go:build windows

package gui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"lsyltunnel/src/client/tunnel"
)

func (a *App) startClient(cfg tunnel.Config, serverVersion string) error {
	return a.startClientEmbedded(cfg, serverVersion)
}

func (a *App) startClientEmbedded(cfg tunnel.Config, serverVersion string) error {
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
	client.SetServerVersion(serverVersion)

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
	if message, ok := tunnel.VirtualForwardErrorText(text); ok {
		return forwardRuleConfigMessage(raw, message+"。")
	}
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
	case containsAnyText(text, "certificate has expired", "certificate is not yet valid", "has expired or is not yet valid"):
		return "服务端证书已过期或尚未生效，请联系管理员重新下发有效证书。"
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
	case strings.Contains(text, "user_stream_limit") || strings.Contains(text, "too many concurrent streams"):
		return "当前账号并发连接数已达到上限，请关闭部分连接后重试。"
	case strings.Contains(text, "auth_blocked") || strings.Contains(text, "too many login failures"):
		return "登录失败次数过多，当前来源 IP 已被临时封禁，请稍后再试。"
	case strings.Contains(text, "server_addr is required"):
		return "请输入服务端地址。"
	case strings.Contains(text, "client config_version must be >= 0"):
		return "客户端配置版本无效，请联系管理员重新下发配置。"
	case strings.Contains(text, "requires min_client_version >=") || strings.Contains(text, "requires requires.min_client_version"):
		return "客户端配置版本声明不完整或不匹配，请联系管理员重新下发配置。"
	case strings.Contains(text, "requires a newer client") || (strings.Contains(text, "client config requires version") && strings.Contains(text, "current version")):
		return "当前配置需要更高版本的客户端，请先升级客户端。"
	case (containsAnyText(text, "server_addr", "dial tcp") && containsAnyText(text, "missing port in address", "too many colons in address", "unknown port")):
		return "服务端地址格式不正确，请填写 地址:端口。"
	case strings.Contains(text, "no such host"):
		return "服务端地址无法解析，请检查域名或网络。"
	case strings.Contains(text, "listen tcp") && containsAnyText(text, "missing port in address", "too many colons in address", "unknown port"):
		return "本地监听地址格式不正确，请填写 地址:端口。"
	case strings.Contains(text, "already in use") || strings.Contains(text, "only one usage") || strings.Contains(text, "bind:"):
		return "本地端口已被占用，请关闭占用程序或调整端口。"
	case strings.Contains(text, "has unsupported direction"):
		return forwardRuleConfigMessage(raw, "direction 不受支持，请联系管理员检查客户端配置。")
	case strings.Contains(text, "contains unknown field"):
		return forwardRuleConfigMessage(raw, "包含当前客户端不支持的字段，请联系管理员重新生成配置。")
	case strings.Contains(text, "requires server_target"):
		return forwardRuleConfigMessage(raw, "缺少业务目标 server_target，请联系管理员重新生成客户端配置。")
	case strings.Contains(text, "server_target must include a host and port"):
		return forwardRuleConfigMessage(raw, "转发目标格式不正确，server_target 必须填写为 地址:端口。")
	case strings.Contains(text, "server_target port must be between"):
		return forwardRuleConfigMessage(raw, "转发目标端口必须在 1 到 65535 之间。")
	case strings.Contains(text, "client config must not include listen_port"):
		return forwardRuleConfigMessage(raw, "客户端反向规则使用 listen_addr 选择服务端端口，请删除 listen_port。")
	case strings.Contains(text, "server_to_client requires listen_addr"):
		return forwardRuleConfigMessage(raw, "反向转发缺少服务端监听地址 listen_addr。")
	case strings.Contains(text, "reverse listen_addr must include a loopback host and port"):
		return forwardRuleConfigMessage(raw, "反向 listen_addr 必须填写为服务端回环地址:端口。")
	case strings.Contains(text, "reverse listen_addr port must be between"):
		return forwardRuleConfigMessage(raw, "反向 listen_addr 端口必须在 1 到 65535 之间。")
	case strings.Contains(text, "reverse listen_addr must use a loopback address"):
		return forwardRuleConfigMessage(raw, "反向 listen_addr 必须使用服务端回环地址。")
	case strings.Contains(text, "reverse server_target must use a loopback address"):
		return forwardRuleConfigMessage(raw, "反向转发目标 server_target 必须使用客户端回环地址。")
	case strings.Contains(text, "reverse listen address is required"):
		return "反向转发缺少服务端监听地址 listen_addr，请检查客户端配置。"
	case strings.Contains(text, "reverse listen address is not configured on server"):
		return "反向 listen_addr 未在服务端配置，请联系管理员检查服务端保留端口。"
	case strings.Contains(text, "not allowed to activate this reverse listen address"):
		return "当前账号无权使用该反向端口，请联系管理员检查端口授权。"
	case strings.Contains(text, "requires listen_addr and server_target"):
		return forwardRuleConfigMessage(raw, "正向转发缺少客户端入口 listen_addr 或服务端目标 server_target。")
	case strings.Contains(text, "at least one forward"):
		return "客户端未配置任何转发规则，请联系管理员检查配置。"
	case strings.Contains(text, "no usable forward"):
		return "所有转发规则均启动失败，请检查规则配置、端口占用和客户端组件。"
	case strings.Contains(text, "password_file") || strings.Contains(text, "read password"):
		return "无法读取密码文件，请检查路径和权限。"
	case strings.Contains(text, "target_denied") || strings.Contains(text, "not allowed") || strings.Contains(text, "not allowed to access"):
		return "当前账号没有访问该目标的权限。"
	case strings.Contains(text, "target_unreachable") || strings.Contains(text, "target service is unreachable"):
		return "服务端无法访问目标服务，请联系管理员检查目标服务或防火墙。"
	case strings.Contains(text, "client_version_unsupported"):
		return "客户端版本不在服务端允许范围内，请联系管理员确认两端版本。"
	case strings.Contains(text, "protocol_version_unsupported"):
		return "客户端与服务端协议版本不一致，请联系管理员升级对应一端。"
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
	case (strings.Contains(text, "field ") && strings.Contains(text, "not found in type")) ||
		containsAnyText(text, "missing required field", "duplicate field", "must be a mapping", "must be a list", "must be a value", "exactly one yaml document"):
		return "配置文件结构与当前客户端不兼容，请联系管理员重新下发配置。"
	case strings.Contains(text, "yaml") || strings.Contains(text, "cannot unmarshal") || strings.Contains(text, "did not find expected"):
		return "配置文件格式不正确，请联系管理员检查配置。"
	case strings.Contains(text, "no such file") || strings.Contains(text, "cannot find") || strings.Contains(text, "找不到"):
		return "客户端文件不完整，请联系管理员检查安装包。"
	default:
		return "连接失败，暂时无法识别具体原因；请重试，若持续出现请联系管理员查看客户端日志。"
	}
}

func forwardRuleConfigMessage(raw, message string) string {
	name := ""
	marker := `forward "`
	if start := strings.Index(raw, marker); start >= 0 {
		rest := raw[start+len(marker):]
		if end := strings.Index(rest, `"`); end >= 0 {
			name = strings.TrimSpace(rest[:end])
		}
	}
	if name == "" || name == "<unnamed>" {
		return message
	}
	return "转发规则 \"" + name + "\" 配置错误：" + message
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
