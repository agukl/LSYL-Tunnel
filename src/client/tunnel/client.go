package tunnel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
)

type Client struct {
	cfg        Config
	ctx        context.Context
	listeners  map[string]net.Listener
	forwards   map[string]*forwardRuntime
	forwardsMu sync.Mutex
	healthMu   sync.Mutex
	health     HealthStatus
	logf       transport.LogFunc
	closed     chan struct{}
	closeOnce  sync.Once
	active     atomic.Int64
	total      atomic.Int64
}

var (
	healthOKInterval               = 30 * time.Second
	healthReconnectInitialInterval = 2 * time.Second
	healthReconnectMaxInterval     = 30 * time.Second
	healthMaxReconnectFailures     = 6
)

func Run(ctx context.Context, cfg Config, logf transport.LogFunc) error {
	client, err := Start(ctx, cfg, logf)
	if err != nil {
		return err
	}
	defer client.Close()
	<-ctx.Done()
	return nil
}

func CheckLogin(ctx context.Context, cfg Config) error {
	_, err := CheckLoginResponse(ctx, cfg)
	return err
}

func CheckLoginResponse(ctx context.Context, cfg Config) (protocol.OpenResponse, error) {
	return checkServerResponse(ctx, cfg, "login")
}

func CheckHealthResponse(ctx context.Context, cfg Config) (protocol.OpenResponse, error) {
	return checkServerResponse(ctx, cfg, "health")
}

func checkServerResponse(ctx context.Context, cfg Config, reqType string) (protocol.OpenResponse, error) {
	var resp protocol.OpenResponse
	ApplyDefaults(&cfg)
	tlsCfg, err := clientTLSConfig(cfg)
	if err != nil {
		return resp, err
	}
	timeout := time.Duration(cfg.Connection.DialTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	dialer := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: timeout},
		Config:    tlsCfg,
	}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.ServerAddr)
	if err != nil {
		return resp, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	req := protocol.OpenRequest{
		Type:       reqType,
		Username:   cfg.Username,
		Password:   cfg.Password,
		Credential: credentialFromConfig(cfg),
		ClientID:   cfg.ClientID,
	}
	if err := protocol.WriteJSON(conn, req); err != nil {
		return resp, err
	}
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		return resp, err
	}
	if !resp.OK {
		if resp.Message == "" {
			resp.Message = reqType + " failed"
		}
		if resp.Code != "" {
			return resp, fmt.Errorf("%s: %s", resp.Code, resp.Message)
		}
		return resp, errors.New(resp.Message)
	}
	return resp, nil
}

func tlsDialer(timeout time.Duration, tlsCfg *tls.Config) tls.Dialer {
	return tls.Dialer{
		NetDialer: &net.Dialer{Timeout: timeout},
		Config:    tlsCfg,
	}
}

func Start(ctx context.Context, cfg Config, logf transport.LogFunc) (*Client, error) {
	ApplyDefaults(&cfg)
	client := &Client{
		cfg:       cfg,
		ctx:       ctx,
		listeners: map[string]net.Listener{},
		forwards:  map[string]*forwardRuntime{},
		logf:      logf,
		closed:    make(chan struct{}),
	}
	client.setHealth(HealthChecking, "等待服务端健康检查", "", false)
	usableForwards := 0
	for _, fwd := range cfg.Forwards {
		name := forwardName(fwd)
		if fwd.Direction == DirectionServerToClient {
			client.initForward(name, fwd, ForwardReverseWait, "等待服务端被动端口激活")
			client.log("reverse forward %s waiting on server %s -> client %s", name, fwd.ListenAddr, fwd.ServerTarget)
			go client.reverseLoop(ctx, name, fwd)
			usableForwards++
			continue
		}
		client.initForward(name, fwd, ForwardStarting, "正在监听本地端口")
		if err := client.ensureForwardListener(name, fwd); err != nil {
			continue
		}
		usableForwards++
	}
	if usableForwards == 0 {
		_ = client.Close()
		return nil, fmt.Errorf("no usable forward is available")
	}
	go func() {
		<-ctx.Done()
		_ = client.Close()
	}()
	go client.healthLoop(ctx)
	return client, nil
}

func (c *Client) ForwardAddr(name string) string {
	c.forwardsMu.Lock()
	ln := c.listeners[name]
	c.forwardsMu.Unlock()
	if ln == nil {
		return ""
	}
	return ln.Addr().String()
}

func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		for _, ln := range c.closeAllListeners() {
			if e := ln.Close(); e != nil && err == nil {
				err = e
			}
		}
	})
	return err
}

func (c *Client) Done() <-chan struct{} {
	if c == nil {
		return nil
	}
	return c.closed
}

func forwardName(fwd ForwardConfig) string {
	if fwd.Name != "" {
		return fwd.Name
	}
	return fwd.ListenAddr
}

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
	return listeners
}

func (c *Client) stopForwardListener(name string) {
	c.forwardsMu.Lock()
	ln := c.listeners[name]
	if ln != nil {
		delete(c.listeners, name)
	}
	c.forwardsMu.Unlock()
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
	dialer := &net.Dialer{Timeout: time.Duration(c.cfg.Connection.DialTimeoutSec) * time.Second}
	remote, err := tls.DialWithDialer(dialer, "tcp", c.cfg.ServerAddr, tlsCfg)
	if err != nil {
		c.recordForwardError(name, err)
		c.log("connect server failed: %v", err)
		return
	}
	defer remote.Close()
	req := protocol.OpenRequest{
		Type:        "open",
		Username:    c.cfg.Username,
		Password:    c.cfg.Password,
		Credential:  credentialFromConfig(c.cfg),
		ClientID:    c.cfg.ClientID,
		ForwardName: name,
		Direction:   DirectionClientToServer,
		Target:      fwd.ServerTarget,
	}
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
	dialer := &net.Dialer{Timeout: timeout}
	remote, err := tls.DialWithDialer(dialer, "tcp", c.cfg.ServerAddr, tlsCfg)
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
	req := protocol.OpenRequest{
		Type:        "reverse_listen",
		Username:    c.cfg.Username,
		Password:    c.cfg.Password,
		Credential:  credentialFromConfig(c.cfg),
		ClientID:    c.cfg.ClientID,
		ForwardName: name,
		Direction:   DirectionServerToClient,
		ListenAddr:  fwd.ListenAddr,
		Target:      fwd.ServerTarget,
	}
	if err := protocol.WriteJSON(remote, req); err != nil {
		return fmt.Errorf("send reverse listen request failed: %w", err)
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(remote, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		return fmt.Errorf("read reverse listen response failed: %w", err)
	}
	if !resp.OK {
		return responseError(resp, "server rejected reverse forward")
	}
	c.setForwardState(name, ForwardReverseActive, "服务端被动端口已激活")
	c.log("reverse listen %s activated on server %s", name, fwd.ListenAddr)
	_ = remote.SetDeadline(time.Time{})
	for {
		var event protocol.OpenResponse
		if err := protocol.ReadJSON(remote, &event, protocol.DefaultMaxHandshakeBytes); err != nil {
			return fmt.Errorf("reverse listen disconnected: %w", err)
		}
		if event.Code == "reverse_ping" {
			_ = remote.SetWriteDeadline(time.Now().Add(timeout))
			err := protocol.WriteJSON(remote, protocol.OpenRequest{
				Type:        "reverse_pong",
				Username:    c.cfg.Username,
				ClientID:    c.cfg.ClientID,
				ForwardName: name,
				Direction:   DirectionServerToClient,
				ListenAddr:  fwd.ListenAddr,
				Target:      fwd.ServerTarget,
			})
			_ = remote.SetWriteDeadline(time.Time{})
			if err != nil {
				return fmt.Errorf("send reverse heartbeat failed: %w", err)
			}
			continue
		}
		if event.Code != "reverse_connect" || event.StreamID == "" {
			continue
		}
		listenAddr := event.ListenAddr
		if listenAddr == "" {
			listenAddr = fwd.ListenAddr
		}
		go c.openReverseStream(ctx, name, fwd, listenAddr, event.StreamID)
	}
}

func (c *Client) openReverseStream(ctx context.Context, name string, fwd ForwardConfig, listenAddr, streamID string) {
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
	dialer := &net.Dialer{Timeout: timeout}
	remote, err := tls.DialWithDialer(dialer, "tcp", c.cfg.ServerAddr, tlsCfg)
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
	req := protocol.OpenRequest{
		Type:        "reverse_stream",
		Username:    c.cfg.Username,
		Password:    c.cfg.Password,
		Credential:  credentialFromConfig(c.cfg),
		ClientID:    c.cfg.ClientID,
		ForwardName: name,
		Direction:   DirectionServerToClient,
		ListenAddr:  listenAddr,
		StreamID:    streamID,
		Target:      fwd.ServerTarget,
	}
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
		c.recordForwardError(name, err)
		c.log("connect client target %s failed: %v", fwd.ServerTarget, err)
		return
	}
	defer target.Close()
	streamDone := c.beginForwardStream(name)
	defer streamDone()
	c.log("reverse stream open %s server %s -> client %s", name, fwd.ListenAddr, fwd.ServerTarget)
	transport.ProxyPair(remote, target, nil, nil)
}

func (c *Client) clientTLSConfig() (*tls.Config, error) {
	return clientTLSConfig(c.cfg)
}

func clientTLSConfig(cfg Config) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion:         transport.TLSMinVersion(cfg.TLS.MinVersion),
		ServerName:         cfg.TLS.ServerName,
		InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
	}
	if cfg.TLS.CACertFile != "" {
		pemData, err := os.ReadFile(cfg.TLS.CACertFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("no server TLS trust data found in %s", cfg.TLS.CACertFile)
		}
		tlsCfg.RootCAs = pool
	}
	return tlsCfg, nil
}

func responseError(resp protocol.OpenResponse, fallback string) error {
	message := resp.Message
	if message == "" {
		message = fallback
	}
	if resp.Code != "" {
		return fmt.Errorf("%s: %s", resp.Code, message)
	}
	return errors.New(message)
}

func (c *Client) log(format string, args ...any) {
	if c.logf != nil {
		c.logf(format, args...)
	}
}
