package tunnel

import (
	"context"
	"errors"
	"time"
)

var (
	healthOKInterval               = 30 * time.Second
	healthReconnectInitialInterval = 2 * time.Second
	healthReconnectMaxInterval     = 30 * time.Second
	healthMaxReconnectFailures     = 6
)

func (c *Client) setHealth(state, message string, cause error, terminal bool) HealthStatus {
	now := time.Now().Format(time.RFC3339)
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	next := c.health
	next.State = state
	next.Message = message
	next.LastChecked = now
	next.Terminal = terminal
	next.err = cause
	if state == HealthOK {
		next.LastOK = now
		next.LastError = ""
		next.err = nil
		next.ConsecutiveFailures = 0
		next.RetryDelaySec = 0
		next.NextRetryAt = ""
	} else if state == HealthChecking {
		next.LastError = ""
		next.err = nil
	} else {
		if cause != nil {
			next.LastError = cause.Error()
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
	tlsCfg, err := c.clientTLSConfig()
	if err != nil {
		state, message := classifyHealthError(err)
		return c.finalizeHealthStatus(c.setHealth(state, message, err, state == HealthAuthError))
	}
	resp, err := checkServerResponseWithTLS(checkCtx, c.cfg, "health", tlsCfg, c.certificateGateKey)
	if err != nil {
		state, message := classifyHealthError(err)
		return c.finalizeHealthStatus(c.setHealth(state, message, err, state == HealthAuthError))
	}
	c.setServerVersionFromResponse(resp)
	if !resp.OK {
		message := resp.Message
		if message == "" {
			message = "health check failed"
		}
		return c.finalizeHealthStatus(c.setHealth(HealthServerUnavailable, message, errors.New(message), false))
	}
	return c.finalizeHealthStatus(c.setHealth(HealthOK, "服务端连接正常", nil, false))
}

func (c *Client) healthLoop(ctx context.Context, delayInitialCheck bool) {
	if delayInitialCheck {
		timer := time.NewTimer(healthOKInterval)
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
