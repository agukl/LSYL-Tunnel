//go:build windows

package gui

import (
	"context"
	"fmt"
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
	client, err := tunnel.StartVerified(ctx, cfg, serverVersion, func(format string, args ...any) {
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
	a.detachClient(client, stats.Health)
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

func (a *App) detachClient(client *tunnel.Client, health tunnel.HealthStatus) {
	message := terminalDisconnectMessage(tunnel.ClientStats{Health: health})
	a.mu.Lock()
	if a.tun != client {
		a.mu.Unlock()
		return
	}
	cancel := a.stop
	a.tun = nil
	a.stop = nil
	a.notice = message
	a.noticeBad = true
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if message != "" {
		a.appendLog("连接状态已取消: " + message)
	}
	a.updateTrayToolTip()
	if shouldClearSavedPasswordState(health.Err()) {
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
