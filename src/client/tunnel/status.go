package tunnel

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"lsyltunnel/src/internal/protocol"
)

const (
	HealthChecking          = "checking"
	HealthOK                = "ok"
	HealthServerUnavailable = "server_unavailable"
	HealthAuthError         = "auth_error"

	ForwardStarting      = "starting"
	ForwardListening     = "listening"
	ForwardListenFailed  = "listen_failed"
	ForwardReverseWait   = "reverse_waiting"
	ForwardReverseActive = "reverse_active"
	ForwardRetrying      = "retrying"
	ForwardRejected      = "rejected"
)

type HealthStatus struct {
	State               string `json:"state"`
	Message             string `json:"message,omitempty"`
	LastChecked         string `json:"last_checked,omitempty"`
	LastOK              string `json:"last_ok,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures,omitempty"`
	RetryDelaySec       int    `json:"retry_delay_sec,omitempty"`
	NextRetryAt         string `json:"next_retry_at,omitempty"`
	Terminal            bool   `json:"terminal,omitempty"`
}

type ForwardStatus struct {
	Name         string `json:"name"`
	Direction    string `json:"direction"`
	ListenAddr   string `json:"listen_addr"`
	ServerTarget string `json:"server_target"`
	State        string `json:"state"`
	Message      string `json:"message,omitempty"`
	Active       int64  `json:"active"`
	Total        int64  `json:"total"`
	LastOpen     string `json:"last_open,omitempty"`
	LastClose    string `json:"last_close,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

type ClientStats struct {
	Health HealthStatus    `json:"health"`
	Active int64           `json:"active"`
	Total  int64           `json:"total"`
	Items  []ForwardStatus `json:"items"`
}

type ForwardCheckSummary struct {
	Checked  int `json:"checked"`
	Allowed  int `json:"allowed"`
	Rejected int `json:"rejected"`
	Failed   int `json:"failed"`
}

type forwardRuntime struct {
	name         string
	direction    string
	listenAddr   string
	serverTarget string
	active       atomic.Int64
	total        atomic.Int64

	mu        sync.Mutex
	state     string
	message   string
	lastOpen  string
	lastClose string
	lastError string
}

func (c *Client) Stats() ClientStats {
	if c == nil {
		return ClientStats{}
	}
	c.healthMu.Lock()
	health := c.health
	c.healthMu.Unlock()

	c.forwardsMu.Lock()
	items := make([]ForwardStatus, 0, len(c.forwards))
	for _, item := range c.forwards {
		items = append(items, item.snapshot())
	}
	c.forwardsMu.Unlock()
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return ClientStats{
		Health: health,
		Active: c.active.Load(),
		Total:  c.total.Load(),
		Items:  items,
	}
}

func (f *forwardRuntime) snapshot() ForwardStatus {
	if f == nil {
		return ForwardStatus{}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return ForwardStatus{
		Name:         f.name,
		Direction:    f.direction,
		ListenAddr:   f.listenAddr,
		ServerTarget: f.serverTarget,
		State:        f.state,
		Message:      f.message,
		Active:       f.active.Load(),
		Total:        f.total.Load(),
		LastOpen:     f.lastOpen,
		LastClose:    f.lastClose,
		LastError:    f.lastError,
	}
}

func (c *Client) initForward(name string, fwd ForwardConfig, state, message string) {
	rt := &forwardRuntime{
		name:         name,
		direction:    fwd.Direction,
		listenAddr:   fwd.ListenAddr,
		serverTarget: fwd.ServerTarget,
		state:        state,
		message:      message,
	}
	c.forwardsMu.Lock()
	c.forwards[name] = rt
	c.forwardsMu.Unlock()
}

func (c *Client) forward(name string) *forwardRuntime {
	c.forwardsMu.Lock()
	defer c.forwardsMu.Unlock()
	return c.forwards[name]
}

func (c *Client) setForwardState(name, state, message string) {
	fwd := c.forward(name)
	if fwd == nil {
		return
	}
	fwd.mu.Lock()
	fwd.state = state
	fwd.message = message
	fwd.mu.Unlock()
}

func (c *Client) recordForwardError(name string, err error) {
	if err == nil {
		return
	}
	fwd := c.forward(name)
	if fwd == nil {
		return
	}
	fwd.mu.Lock()
	fwd.lastError = err.Error()
	fwd.message = ForwardErrorMessage(err)
	fwd.mu.Unlock()
}

func (c *Client) beginForwardStream(name string) func() {
	now := time.Now().Format(time.RFC3339)
	fwd := c.forward(name)
	if fwd != nil {
		fwd.active.Add(1)
		fwd.total.Add(1)
		fwd.mu.Lock()
		fwd.lastOpen = now
		fwd.lastError = ""
		fwd.mu.Unlock()
	}
	c.total.Add(1)
	c.active.Add(1)
	return func() {
		closedAt := time.Now().Format(time.RFC3339)
		if fwd != nil {
			fwd.active.Add(-1)
			fwd.mu.Lock()
			fwd.lastClose = closedAt
			fwd.mu.Unlock()
		}
		c.active.Add(-1)
	}
}

func (c *Client) setHealth(state, message, errText string, terminal bool) HealthStatus {
	now := time.Now().Format(time.RFC3339)
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	next := c.health
	next.State = state
	next.Message = message
	next.LastChecked = now
	next.Terminal = terminal
	if state == HealthOK {
		next.LastOK = now
		next.LastError = ""
		next.ConsecutiveFailures = 0
		next.RetryDelaySec = 0
		next.NextRetryAt = ""
	} else if state == HealthChecking {
		next.LastError = ""
	} else {
		if errText != "" {
			next.LastError = errText
		}
		next.ConsecutiveFailures++
	}
	c.health = next
	return next
}

func (c *Client) setHealthRetry(delay time.Duration) HealthStatus {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	next := c.health
	if next.State == HealthOK || next.Terminal || delay <= 0 {
		next.RetryDelaySec = 0
		next.NextRetryAt = ""
	} else {
		next.RetryDelaySec = int(delay.Round(time.Second).Seconds())
		next.NextRetryAt = time.Now().Add(delay).Format(time.RFC3339)
	}
	c.health = next
	return next
}

func (c *Client) markHealthTerminal(message string) HealthStatus {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	next := c.health
	next.Terminal = true
	next.RetryDelaySec = 0
	next.NextRetryAt = ""
	if message != "" {
		next.Message = message
	}
	c.health = next
	return next
}

func (c *Client) CheckHealthNow(ctx context.Context) HealthStatus {
	if c == nil {
		return HealthStatus{}
	}
	timeout := time.Duration(c.cfg.Connection.DialTimeoutSec+2) * time.Second
	if timeout < 5*time.Second {
		timeout = 5 * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := CheckHealthResponse(checkCtx, c.cfg)
	if err != nil {
		state, message := classifyHealthError(err)
		return c.finalizeHealthStatus(c.setHealth(state, message, err.Error(), state == HealthAuthError))
	}
	if !resp.OK {
		message := resp.Message
		if message == "" {
			message = "health check failed"
		}
		return c.finalizeHealthStatus(c.setHealth(HealthServerUnavailable, message, message, false))
	}
	return c.finalizeHealthStatus(c.setHealth(HealthOK, "服务端连接正常", "", false))
}

func (c *Client) CheckForwardsNow(ctx context.Context) ForwardCheckSummary {
	var summary ForwardCheckSummary
	if c == nil {
		return summary
	}
	timeout := time.Duration(c.cfg.Connection.DialTimeoutSec+2) * time.Second
	if timeout < 5*time.Second {
		timeout = 5 * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, fwd := range c.cfg.Forwards {
		fwd := fwd
		name := forwardName(fwd)
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := c.checkForwardNow(checkCtx, name, fwd)
			mu.Lock()
			summary.Checked++
			switch result {
			case "allowed":
				summary.Allowed++
			case "rejected":
				summary.Rejected++
			default:
				summary.Failed++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return summary
}

func (c *Client) checkForwardNow(ctx context.Context, name string, fwd ForwardConfig) string {
	resp, err := c.checkForwardResponse(ctx, name, fwd)
	if err != nil {
		c.recordForwardError(name, err)
		if IsPermanentForwardError(err) {
			c.setForwardState(name, ForwardRejected, ForwardErrorMessage(err))
			if forwardDirection(fwd) == DirectionClientToServer {
				c.stopForwardListener(name)
			}
			return "rejected"
		}
		return "failed"
	}
	if !resp.OK {
		err := responseError(resp, "server rejected forward check")
		c.recordForwardError(name, err)
		if IsPermanentForwardError(err) {
			c.setForwardState(name, ForwardRejected, ForwardErrorMessage(err))
			if forwardDirection(fwd) == DirectionClientToServer {
				c.stopForwardListener(name)
			}
			return "rejected"
		}
		return "failed"
	}
	switch forwardDirection(fwd) {
	case DirectionClientToServer:
		if err := c.ensureForwardListener(name, fwd); err != nil {
			c.recordForwardError(name, err)
			return "failed"
		}
	case DirectionServerToClient:
		if current := c.forward(name); current != nil {
			current.mu.Lock()
			wasRejected := current.state == ForwardRejected
			current.mu.Unlock()
			if wasRejected {
				c.setForwardState(name, ForwardReverseWait, "等待服务端被动端口激活")
				go c.reverseLoop(c.ctx, name, fwd)
			}
		}
	}
	return "allowed"
}

func (c *Client) checkForwardResponse(ctx context.Context, name string, fwd ForwardConfig) (protocol.OpenResponse, error) {
	var resp protocol.OpenResponse
	tlsCfg, err := c.clientTLSConfig()
	if err != nil {
		return resp, err
	}
	timeout := time.Duration(c.cfg.Connection.DialTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	dialer := tlsDialer(timeout, tlsCfg)
	conn, err := dialer.DialContext(ctx, "tcp", c.cfg.ServerAddr)
	if err != nil {
		return resp, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	req := protocol.OpenRequest{
		Type:        "forward_check",
		Username:    c.cfg.Username,
		Password:    c.cfg.Password,
		Credential:  credentialFromConfig(c.cfg),
		ClientID:    c.cfg.ClientID,
		ForwardName: name,
		Direction:   forwardDirection(fwd),
		ListenAddr:  fwd.ListenAddr,
		Target:      fwd.ServerTarget,
	}
	if err := protocol.WriteJSON(conn, req); err != nil {
		return resp, err
	}
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		return resp, err
	}
	if !resp.OK {
		return resp, responseError(resp, "server rejected forward check")
	}
	return resp, nil
}

func forwardDirection(fwd ForwardConfig) string {
	direction := strings.TrimSpace(fwd.Direction)
	if direction == "" {
		return DirectionClientToServer
	}
	return direction
}

func ForwardCheckMessage(summary ForwardCheckSummary) string {
	if summary.Checked == 0 {
		return "服务端连接正常"
	}
	if summary.Rejected > 0 || summary.Failed > 0 {
		return fmt.Sprintf("服务端连接正常，已检查 %d 个端口，%d 个异常", summary.Checked, summary.Rejected+summary.Failed)
	}
	return fmt.Sprintf("服务端连接正常，已检查 %d 个端口", summary.Checked)
}

func (c *Client) healthLoop(ctx context.Context) {
	for {
		status := c.CheckHealthNow(ctx)
		if c.shouldStopAfterHealth(status) {
			_ = c.Close()
			return
		}
		delay := c.nextHealthDelay()
		c.setHealthRetry(delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-c.closed:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *Client) nextHealthDelay() time.Duration {
	c.healthMu.Lock()
	state := c.health.State
	failures := c.health.ConsecutiveFailures
	c.healthMu.Unlock()
	switch state {
	case HealthOK:
		return healthOKInterval
	case HealthServerUnavailable:
		return ReconnectDelay(failures)
	default:
		return healthReconnectInitialInterval
	}
}

func (c *Client) shouldStopAfterHealth(status HealthStatus) bool {
	if status.Terminal {
		return true
	}
	return c.finalizeHealthStatus(status).Terminal
}

func (c *Client) finalizeHealthStatus(status HealthStatus) HealthStatus {
	switch status.State {
	case HealthAuthError:
		if status.Message == "" {
			status.Message = "认证异常，已取消连接状态，请重新连接"
		}
		return c.markHealthTerminal(status.Message)
	case HealthServerUnavailable:
		if status.ConsecutiveFailures >= healthMaxReconnectFailures {
			return c.markHealthTerminal("多次重连失败，已取消连接状态，请确认服务端恢复后重新连接")
		}
	}
	return status
}

func classifyHealthError(err error) (string, string) {
	if err == nil {
		return HealthOK, "服务端连接正常"
	}
	text := strings.ToLower(err.Error())
	switch {
	case containsAny(text, "auth_failed", "username or password"):
		return HealthAuthError, "账号或密码不正确，需要重新登录"
	case containsAny(text, "credential_expired", "saved login has expired"):
		return HealthAuthError, "保存的登录凭据已过期，需要重新登录"
	case containsAny(text, "auth_blocked", "too many"):
		return HealthAuthError, "登录失败次数过多，账号来源暂时被封禁"
	case containsAny(text, "no server tls trust data", "appendcertsfrompem"):
		return HealthAuthError, "服务端信任证书无效，请联系管理员重新下发"
	case isMissingServerCertError(text):
		return HealthAuthError, "缺少服务端信任证书 server.crt，请联系管理员重新下发客户端安装包"
	case containsAny(text, "ca_cert_file", "server verification"):
		return HealthAuthError, "缺少服务端信任证书 server.crt，请联系管理员重新下发客户端安装包"
	case containsAny(text, "certificate is valid for", "cannot validate certificate", "doesn't contain any ip sans"):
		return HealthAuthError, "服务端证书和当前地址不匹配，请检查服务端地址或重新下发证书"
	case containsAny(text, "unknown authority", "not trusted"):
		return HealthAuthError, "服务端证书不受信任，请联系管理员重新下发证书"
	case containsAny(text, "certificate", "x509", "tls"):
		return HealthAuthError, "服务端证书校验失败，请联系管理员检查证书"
	case containsAny(text, "no such host"):
		return HealthServerUnavailable, "服务端地址无法解析，请检查域名或网络"
	case containsAny(text, "connection refused", "actively refused", "connectex"):
		return HealthServerUnavailable, "连接不上服务端，请检查服务端是否启动或地址端口是否正确"
	case containsAny(text, "timeout", "deadline", "i/o timeout"):
		return HealthServerUnavailable, "连接超时，请检查网络或服务端防火墙"
	case containsAny(text, "network is unreachable", "no route to host"):
		return HealthServerUnavailable, "网络不可达，请检查本机网络"
	case containsAny(text, "connection reset", "forcibly closed", "wsarecv", "eof"):
		return HealthServerUnavailable, "连接被服务端断开，请稍后重试或联系管理员"
	default:
		return HealthServerUnavailable, "服务端暂时不可达"
	}
}

func ForwardErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	switch {
	case containsAny(text, "auth_failed", "username or password"):
		return "账号或密码不正确，请重新登录"
	case containsAny(text, "credential_expired", "saved login has expired"):
		return "保存的登录凭据已过期，请重新输入密码"
	case containsAny(text, "auth_blocked", "too many"):
		return "登录失败次数过多，请稍后再试"
	case containsAny(text, "certificate", "x509", "tls"):
		return "服务端证书校验失败，请联系管理员检查证书"
	case containsAny(text, "target_denied", "not allowed", "not configured on server"):
		return "当前账号没有访问该端口的权限，请联系管理员检查端口授权"
	case containsAny(text, "already activated"):
		return "该被动端口已被其他客户端占用"
	case containsAny(text, "server passive port is unavailable", "listen reverse"):
		return "服务端被动端口不可用，客户端会自动重试，请联系管理员检查服务端端口占用"
	case containsAny(text, "already in use", "only one usage", "bind:"):
		return "本地端口已被占用，请关闭占用程序或调整端口"
	case containsAny(text, "target_unreachable", "target service is unreachable"):
		return "目标服务暂时不可达，请确认对应服务已启动"
	case containsAny(text, "connection refused", "actively refused", "connectex"):
		return "服务端暂时不可达，客户端会自动重试"
	case containsAny(text, "timeout", "deadline", "i/o timeout"):
		return "连接超时，客户端会自动重试"
	case containsAny(text, "network is unreachable", "no route to host", "no such host"):
		return "网络不可达，请检查本机网络或服务端地址"
	case containsAny(text, "connection reset", "forcibly closed", "wsarecv", "eof"):
		return "连接被断开，客户端会自动重试"
	default:
		return "连接异常，客户端会自动重试"
	}
}

func IsPermanentForwardError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return containsAny(
		text,
		"auth_failed",
		"username or password",
		"credential_expired",
		"saved login has expired",
		"certificate",
		"x509",
		"tls",
		"target_denied",
		"not allowed",
		"not configured on server",
	)
}

func ReverseRetryDelay(err error, failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	if err != nil {
		text := strings.ToLower(err.Error())
		if containsAny(text, "auth_blocked", "too many", "already activated") {
			return 5 * time.Minute
		}
	}
	delay := time.Duration(failures) * 2 * time.Second
	if delay < 2*time.Second {
		return 2 * time.Second
	}
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func ReconnectDelay(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := healthReconnectInitialInterval
	for i := 1; i < failures; i++ {
		delay *= 2
		if delay >= healthReconnectMaxInterval {
			return healthReconnectMaxInterval
		}
	}
	if delay > healthReconnectMaxInterval {
		return healthReconnectMaxInterval
	}
	return delay
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func isMissingServerCertError(text string) bool {
	if !containsAny(text, "server.crt", "ca_cert_file") {
		return false
	}
	return containsAny(text, "no such file", "cannot find", "找不到", "系统找不到")
}
