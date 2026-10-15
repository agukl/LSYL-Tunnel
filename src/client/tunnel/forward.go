package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
)

func (c *Client) ensureForwardListener(name string, fwd ForwardConfig) error {
	select {
	case <-c.closed:
		return fmt.Errorf("client is closed")
	default:
	}
	c.forwardsMu.Lock()
	if c.listeners[name] != nil {
		c.forwardsMu.Unlock()
		return nil
	}
	c.forwardsMu.Unlock()
	ln, err := net.Listen("tcp", fwd.ListenAddr)
	if err != nil {
		c.setForwardState(name, ForwardListenFailed, ForwardErrorMessage(err))
		c.log("forward %s listen %s failed: %v", name, fwd.ListenAddr, err)
		return err
	}

	c.forwardsMu.Lock()
	if c.listeners[name] != nil {
		c.forwardsMu.Unlock()
		_ = ln.Close()
		return nil
	}
	c.listeners[name] = ln
	c.forwardsMu.Unlock()

	c.setForwardAddresses(name, ln.Addr().String(), "")
	c.setForwardState(name, ForwardListening, "本地端口监听中")
	c.log("forward %s listening on %s -> %s", name, ln.Addr(), fwd.ServerTarget)
	go c.acceptLoop(c.ctx, ln, name, fwd)
	return nil
}

func (c *Client) closeAllListeners() []net.Listener {
	c.forwardsMu.Lock()
	defer c.forwardsMu.Unlock()
	listeners := make([]net.Listener, 0, len(c.listeners))
	for _, ln := range c.listeners {
		listeners = append(listeners, ln)
	}
	c.listeners = map[string]net.Listener{}
	c.virtualAddrs = map[string]string{}
	return listeners
}

func (c *Client) stopForwardListener(name string) {
	c.forwardsMu.Lock()
	var session virtualRedirectSession
	if _, member := c.virtualRedirectMembers[name]; member {
		delete(c.virtualRedirectMembers, name)
		if len(c.virtualRedirectMembers) == 0 {
			session = c.virtualRedirectSession
			c.virtualRedirectSession = nil
		}
	}
	ln := c.listeners[name]
	if ln != nil {
		delete(c.listeners, name)
	}
	delete(c.virtualAddrs, name)
	c.forwardsMu.Unlock()
	if session != nil {
		_ = session.Close()
	}
	if ln != nil {
		_ = ln.Close()
	}
}

func (c *Client) forwardListenerActive(name string, ln net.Listener) bool {
	c.forwardsMu.Lock()
	defer c.forwardsMu.Unlock()
	return c.listeners[name] == ln
}

func (c *Client) acceptLoop(ctx context.Context, ln net.Listener, name string, fwd ForwardConfig) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-c.closed:
				return
			default:
			}
			if errors.Is(err, net.ErrClosed) || !c.forwardListenerActive(name, ln) {
				return
			}
			c.log("accept %s failed: %v", name, err)
			continue
		}
		go c.handleLocal(conn, name, fwd)
	}
}

func (c *Client) handleLocal(local net.Conn, name string, fwd ForwardConfig) {
	defer local.Close()
	tlsCfg, err := c.clientTLSConfig()
	if err != nil {
		c.recordForwardError(name, err)
		c.log("tls config failed: %v", err)
		return
	}
	timeout := time.Duration(c.cfg.Connection.DialTimeoutSec) * time.Second
	remote, err := transport.DialTLSWithCertificateGate(c.ctx, &net.Dialer{Timeout: timeout}, "tcp", c.cfg.ServerAddr, tlsCfg, c.certificateGateKey, timeout)
	if err != nil {
		c.recordForwardError(name, err)
		c.log("connect server failed: %v", err)
		return
	}
	defer remote.Close()
	req := newOpenRequest(c.cfg, "open")
	req.ForwardName = name
	req.Direction = DirectionClientToServer
	req.Target = fwd.ServerTarget
	if err := protocol.WriteJSON(remote, req); err != nil {
		c.recordForwardError(name, err)
		c.log("send open request failed: %v", err)
		return
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(remote, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		c.recordForwardError(name, err)
		c.log("read open response failed: %v", err)
		return
	}
	c.setServerVersionFromResponse(resp)
	if !resp.OK {
		err := responseError(resp, "server rejected the connection")
		c.recordForwardError(name, err)
		if IsPermanentForwardError(err) {
			c.setForwardState(name, ForwardRejected, ForwardErrorMessage(err))
			c.stopForwardListener(name)
		}
		c.log("open %s rejected: %v", name, err)
		return
	}
	c.setForwardState(name, ForwardListening, "本地端口监听中")
	streamDone := c.beginForwardStream(name)
	defer streamDone()
	c.log("stream open %s -> %s", name, fwd.ServerTarget)
	transport.ProxyPair(remote, local, nil, nil)
}
