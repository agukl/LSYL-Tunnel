package tunnel

import (
	"context"
	"fmt"
	"net"
	"time"

	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
)

func (c *Client) reverseLoop(ctx context.Context, name string, fwd ForwardConfig) {
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closed:
			return
		default:
		}
		if err := c.maintainReverseListen(ctx, name, fwd); err != nil {
			failures++
			c.recordForwardError(name, err)
			message := ForwardErrorMessage(err)
			if IsPermanentForwardError(err) {
				c.setForwardState(name, ForwardRejected, message)
				c.log("reverse forward %s stopped after non-retryable error: %v", name, err)
				return
			}
			c.setForwardState(name, ForwardRetrying, message)
			delay := ReverseRetryDelay(err, failures)
			select {
			case <-ctx.Done():
				return
			case <-c.closed:
				return
			case <-time.After(delay):
			}
			c.log("reverse forward %s retrying after error: %v", name, err)
		}
	}
}

func (c *Client) maintainReverseListen(ctx context.Context, name string, fwd ForwardConfig) error {
	tlsCfg, err := c.clientTLSConfig()
	if err != nil {
		return err
	}
	timeout := time.Duration(c.cfg.Connection.DialTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	remote, err := transport.DialTLSWithCertificateGate(ctx, &net.Dialer{Timeout: timeout}, "tcp", c.cfg.ServerAddr, tlsCfg, c.certificateGateKey, timeout)
	if err != nil {
		return fmt.Errorf("connect server failed: %w", err)
	}
	defer remote.Close()
	transport.EnableTCPKeepAlive(remote, timeout)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = remote.Close()
		case <-c.closed:
			_ = remote.Close()
		case <-done:
		}
	}()
	defer close(done)
	_ = remote.SetDeadline(time.Now().Add(timeout))
	req := newOpenRequest(c.cfg, "reverse_listen")
	req.ForwardName = name
	req.Direction = DirectionServerToClient
	req.ListenAddr = fwd.ListenAddr
	req.Target = fwd.ServerTarget
	if err := protocol.WriteJSON(remote, req); err != nil {
		return fmt.Errorf("send reverse listen request failed: %w", err)
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(remote, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		return fmt.Errorf("read reverse listen response failed: %w", err)
	}
	c.setServerVersionFromResponse(resp)
	if !resp.OK {
		return responseError(resp, "server rejected reverse forward")
	}
	c.setForwardState(name, ForwardReverseActive, "服务端被动端口已激活")
	c.log("reverse forward %s activated", name)
	_ = remote.SetDeadline(time.Time{})
	for {
		var event protocol.OpenResponse
		if err := protocol.ReadJSON(remote, &event, protocol.DefaultMaxHandshakeBytes); err != nil {
			return fmt.Errorf("reverse listen disconnected: %w", err)
		}
		if event.Code == "reverse_ping" {
			_ = remote.SetWriteDeadline(time.Now().Add(timeout))
			req := newOpenRequest(c.cfg, "reverse_pong")
			req.Password = ""
			req.Credential = nil
			req.ForwardName = name
			req.Direction = DirectionServerToClient
			req.ListenAddr = fwd.ListenAddr
			req.Target = fwd.ServerTarget
			err := protocol.WriteJSON(remote, req)
			_ = remote.SetWriteDeadline(time.Time{})
			if err != nil {
				return fmt.Errorf("send reverse heartbeat failed: %w", err)
			}
			continue
		}
		if event.Code != "reverse_connect" || event.StreamID == "" {
			continue
		}
		go c.openReverseStream(ctx, name, fwd, event.StreamID)
	}
}

func (c *Client) openReverseStream(ctx context.Context, name string, fwd ForwardConfig, streamID string) {
	tlsCfg, err := c.clientTLSConfig()
	if err != nil {
		c.recordForwardError(name, err)
		c.log("reverse stream %s tls config failed: %v", name, err)
		return
	}
	timeout := time.Duration(c.cfg.Connection.DialTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	remote, err := transport.DialTLSWithCertificateGate(ctx, &net.Dialer{Timeout: timeout}, "tcp", c.cfg.ServerAddr, tlsCfg, c.certificateGateKey, timeout)
	if err != nil {
		c.recordForwardError(name, err)
		c.log("reverse stream %s connect server failed: %v", name, err)
		return
	}
	defer remote.Close()
	transport.EnableTCPKeepAlive(remote, timeout)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = remote.Close()
		case <-c.closed:
			_ = remote.Close()
		case <-done:
		}
	}()
	defer close(done)
	_ = remote.SetDeadline(time.Now().Add(timeout))
	req := newOpenRequest(c.cfg, "reverse_stream")
	req.ForwardName = name
	req.Direction = DirectionServerToClient
	req.ListenAddr = fwd.ListenAddr
	req.StreamID = streamID
	req.Target = fwd.ServerTarget
	if err := protocol.WriteJSON(remote, req); err != nil {
		c.recordForwardError(name, err)
		c.log("send reverse stream request failed: %v", err)
		return
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(remote, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		c.recordForwardError(name, err)
		c.log("read reverse stream response failed: %v", err)
		return
	}
	c.setServerVersionFromResponse(resp)
	if !resp.OK {
		err := responseError(resp, "server rejected reverse stream")
		c.recordForwardError(name, err)
		if IsPermanentForwardError(err) {
			c.setForwardState(name, ForwardRejected, ForwardErrorMessage(err))
		}
		c.log("reverse stream %s rejected: %v", name, err)
		return
	}
	_ = remote.SetDeadline(time.Time{})
	target, err := net.DialTimeout("tcp", fwd.ServerTarget, timeout)
	if err != nil {
		targetErr := fmt.Errorf("connect client target %s: %w", fwd.ServerTarget, err)
		c.recordForwardError(name, targetErr)
		c.log("connect client target %s failed: %v", fwd.ServerTarget, err)
		return
	}
	defer target.Close()
	streamDone := c.beginForwardStream(name)
	defer streamDone()
	c.log("reverse stream open %s -> client target %s", name, fwd.ServerTarget)
	transport.ProxyPair(remote, target, nil, nil)
}
