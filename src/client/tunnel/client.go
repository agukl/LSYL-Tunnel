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
	appversion "lsyltunnel/src/internal/version"
)

type Client struct {
	cfg                    Config
	tlsConfig              *tls.Config
	ctx                    context.Context
	listeners              map[string]net.Listener
	virtualAddrs           map[string]string
	virtualResolver        *virtualAddressResolver
	virtualRedirectSession virtualRedirectSession
	virtualRedirectMembers map[string]struct{}
	forwards               map[string]*forwardRuntime
	forwardsMu             sync.Mutex
	healthMu               sync.Mutex
	health                 HealthStatus
	serverVersion          string
	logf                   transport.LogFunc
	closed                 chan struct{}
	closeOnce              sync.Once
	active                 atomic.Int64
	total                  atomic.Int64
}

type preparedVirtualForward struct {
	name        string
	fwd         ForwardConfig
	listener    net.Listener
	virtualAddr string
	rule        virtualRedirectRule
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

func newOpenRequest(cfg Config, reqType string) protocol.OpenRequest {
	return protocol.OpenRequest{
		Type:            reqType,
		Username:        cfg.Username,
		Password:        cfg.Password,
		Credential:      credentialFromConfig(cfg),
		ClientID:        cfg.ClientID,
		ClientVersion:   appversion.AppVersion,
		ProtocolVersion: appversion.ProtocolVersion,
	}
}

func checkServerResponse(ctx context.Context, cfg Config, reqType string) (protocol.OpenResponse, error) {
	ApplyDefaults(&cfg)
	tlsCfg, err := clientTLSConfig(cfg)
	if err != nil {
		return protocol.OpenResponse{}, err
	}
	return checkServerResponseWithTLS(ctx, cfg, reqType, tlsCfg)
}

func checkServerResponseWithTLS(ctx context.Context, cfg Config, reqType string, tlsCfg *tls.Config) (protocol.OpenResponse, error) {
	var resp protocol.OpenResponse
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
	req := newOpenRequest(cfg, reqType)
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
	return start(ctx, cfg, "", false, logf)
}

func StartVerified(ctx context.Context, cfg Config, serverVersion string, logf transport.LogFunc) (*Client, error) {
	return start(ctx, cfg, serverVersion, true, logf)
}

func start(ctx context.Context, cfg Config, serverVersion string, verified bool, logf transport.LogFunc) (*Client, error) {
	ApplyDefaults(&cfg)
	tlsCfg, err := clientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	client := &Client{
		cfg:                    cfg,
		tlsConfig:              tlsCfg,
		ctx:                    ctx,
		listeners:              map[string]net.Listener{},
		virtualAddrs:           map[string]string{},
		virtualRedirectMembers: map[string]struct{}{},
		forwards:               map[string]*forwardRuntime{},
		logf:                   logf,
		closed:                 make(chan struct{}),
	}
	if verified {
		client.setHealth(HealthOK, "服务端连接正常", "", false)
		client.SetServerVersion(serverVersion)
	} else {
		client.setHealth(HealthChecking, "等待服务端健康检查", "", false)
	}
	usableForwards := 0
	blockingStartupFailures := 0
	virtualAuthorizationCancelled := false
	var unusableErr error
	virtualForwards := make([]preparedVirtualForward, 0)
	for _, fwd := range cfg.Forwards {
		name := forwardName(fwd)
		if fwd.Direction == DirectionServerToClient {
			client.initForward(name, fwd, ForwardReverseWait, "等待服务端被动端口激活")
			client.log("reverse forward %s waiting on server %s -> client %s", name, fwd.ListenAddr, fwd.ServerTarget)
			go client.reverseLoop(ctx, name, fwd)
			usableForwards++
			continue
		}
		client.initForward(name, fwd, ForwardStarting, "正在监听客户端入口")
		if fwd.Direction == DirectionVirtual {
			prepared, err := client.prepareVirtualForwardListener(name, fwd)
			if err != nil {
				blockingStartupFailures++
				unusableErr = errors.Join(unusableErr, err)
				continue
			}
			virtualForwards = append(virtualForwards, prepared)
			continue
		}
		if err := client.ensureForwardListener(name, fwd); err != nil {
			blockingStartupFailures++
			unusableErr = errors.Join(unusableErr, err)
			continue
		}
		usableForwards++
	}
	if len(virtualForwards) > 0 {
		rules := make([]virtualRedirectRule, 0, len(virtualForwards))
		for _, prepared := range virtualForwards {
			rules = append(rules, prepared.rule)
		}
		session, redirectErr := startVirtualRedirectSessionFn(ctx, rules)
		if redirectErr == nil && session == nil {
			redirectErr = fmt.Errorf("virtual redirect helper did not start")
		}
		if redirectErr != nil {
			if isVirtualAuthorizationCancelled(redirectErr) {
				virtualAuthorizationCancelled = true
			} else {
				blockingStartupFailures++
			}
			unusableErr = errors.Join(unusableErr, redirectErr)
			for _, prepared := range virtualForwards {
				client.stopForwardListener(prepared.name)
				client.setForwardState(prepared.name, ForwardListenFailed, ForwardErrorMessage(redirectErr))
				client.log("forward %s virtual endpoint setup failed: %v", prepared.name, redirectErr)
			}
		} else if !client.attachVirtualRedirectSession(virtualForwards, session) {
			_ = session.Close()
			redirectErr = fmt.Errorf("virtual forward listeners are no longer active")
			blockingStartupFailures++
			unusableErr = errors.Join(unusableErr, redirectErr)
			for _, prepared := range virtualForwards {
				client.stopForwardListener(prepared.name)
				client.setForwardState(prepared.name, ForwardListenFailed, ForwardErrorMessage(redirectErr))
			}
		} else {
			for _, prepared := range virtualForwards {
				client.activateVirtualForward(prepared)
				usableForwards++
			}
			go client.watchVirtualRedirectSession(session)
		}
	}
	allowConnectedWithoutForward := virtualAuthorizationCancelled && blockingStartupFailures == 0
	if usableForwards == 0 && !allowConnectedWithoutForward {
		_ = client.Close()
		if unusableErr != nil {
			return nil, fmt.Errorf("no usable forward is available: %w", unusableErr)
		}
		return nil, fmt.Errorf("no usable forward is available")
	}
	go func() {
		<-ctx.Done()
		_ = client.Close()
	}()
	go client.healthLoop(ctx, verified)
	return client, nil
}

func (c *Client) ForwardAddr(name string) string {
	c.forwardsMu.Lock()
	virtualAddr := c.virtualAddrs[name]
	ln := c.listeners[name]
	c.forwardsMu.Unlock()
	if virtualAddr != "" {
		return virtualAddr
	}
	if ln == nil {
		return ""
	}
	return ln.Addr().String()
}

func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		if session := c.closeVirtualRedirectSession(); session != nil {
			if e := session.Close(); e != nil {
				err = e
			}
		}
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

	c.setForwardAddresses(name, ln.Addr().String(), "")
	c.setForwardState(name, ForwardListening, "本地端口监听中")
	c.log("forward %s listening on %s -> %s", name, ln.Addr(), fwd.ServerTarget)
	go c.acceptLoop(c.ctx, ln, name, fwd)
	return nil
}

func (c *Client) prepareVirtualForwardListener(name string, fwd ForwardConfig) (preparedVirtualForward, error) {
	virtualHost, virtualPort, err := parseVirtualListenAddr(fwd.ListenAddr)
	if err != nil {
		c.setForwardState(name, ForwardListenFailed, ForwardErrorMessage(err))
		return preparedVirtualForward{}, err
	}
	if c.virtualResolver == nil {
		c.virtualResolver, err = newVirtualAddressResolver(c.cfg)
		if err != nil {
			c.setForwardState(name, ForwardListenFailed, ForwardErrorMessage(err))
			return preparedVirtualForward{}, err
		}
	}
	virtualAddr, err := c.virtualResolver.resolve(virtualHost, virtualPort)
	if err != nil {
		c.setForwardState(name, ForwardListenFailed, ForwardErrorMessage(err))
		return preparedVirtualForward{}, err
	}
	internal, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		listenerErr := fmt.Errorf("allocate virtual local redirect listener: %w", err)
		c.setForwardState(name, ForwardListenFailed, ForwardErrorMessage(listenerErr))
		c.log("forward %s random local redirect listen failed: %v", name, err)
		return preparedVirtualForward{}, listenerErr
	}
	localAddr, ok := internal.Addr().(*net.TCPAddr)
	if !ok || localAddr.Port <= 0 || localAddr.Port > 65535 {
		_ = internal.Close()
		err := fmt.Errorf("forward %s did not allocate a valid local redirect port", name)
		c.setForwardState(name, ForwardListenFailed, ForwardErrorMessage(err))
		return preparedVirtualForward{}, err
	}
	rule, err := newVirtualRedirectRule(virtualAddr, localAddr.Port)
	if err != nil {
		_ = internal.Close()
		c.setForwardState(name, ForwardListenFailed, ForwardErrorMessage(err))
		return preparedVirtualForward{}, err
	}

	c.forwardsMu.Lock()
	if c.listeners[name] != nil {
		c.forwardsMu.Unlock()
		_ = internal.Close()
		return preparedVirtualForward{}, fmt.Errorf("virtual forward %s listener already exists", name)
	}
	c.listeners[name] = internal
	c.virtualAddrs[name] = virtualAddr
	c.forwardsMu.Unlock()

	c.setForwardAddresses(name, internal.Addr().String(), virtualAddr)
	c.log("forward %s prepared virtual endpoint %s on local redirect port %d", name, virtualAddr, localAddr.Port)
	return preparedVirtualForward{name: name, fwd: fwd, listener: internal, virtualAddr: virtualAddr, rule: rule}, nil
}

func (c *Client) activateVirtualForward(prepared preparedVirtualForward) {
	c.setForwardState(prepared.name, ForwardListening, "虚拟端点已接管")
	c.log("forward %s virtual endpoint %s -> local redirect %s -> %s", prepared.name, prepared.virtualAddr, prepared.listener.Addr(), prepared.fwd.ServerTarget)
	go c.acceptLoop(c.ctx, prepared.listener, prepared.name, prepared.fwd)
}

func (c *Client) watchVirtualRedirectSession(session virtualRedirectSession) {
	if session == nil || session.Done() == nil {
		return
	}
	<-session.Done()
	select {
	case <-c.closed:
		return
	default:
	}
	names := c.detachVirtualRedirectSession(session)
	if len(names) == 0 {
		return
	}
	err := session.Err()
	if err == nil {
		err = fmt.Errorf("virtual redirect helper stopped unexpectedly")
	}
	for _, name := range names {
		c.stopForwardListener(name)
		c.setForwardState(name, ForwardListenFailed, ForwardErrorMessage(err))
		c.log("forward %s virtual redirect session stopped: %v", name, err)
	}
}

func (c *Client) attachVirtualRedirectSession(prepared []preparedVirtualForward, session virtualRedirectSession) bool {
	if session == nil {
		return false
	}
	c.forwardsMu.Lock()
	defer c.forwardsMu.Unlock()
	if c.virtualRedirectSession != nil {
		return false
	}
	for _, forward := range prepared {
		if c.listeners[forward.name] != forward.listener {
			return false
		}
	}
	c.virtualRedirectSession = session
	if c.virtualRedirectMembers == nil {
		c.virtualRedirectMembers = map[string]struct{}{}
	}
	for _, forward := range prepared {
		c.virtualRedirectMembers[forward.name] = struct{}{}
	}
	return true
}

func (c *Client) detachVirtualRedirectSession(session virtualRedirectSession) []string {
	c.forwardsMu.Lock()
	defer c.forwardsMu.Unlock()
	if c.virtualRedirectSession != session {
		return nil
	}
	names := make([]string, 0, len(c.virtualRedirectMembers))
	for name := range c.virtualRedirectMembers {
		names = append(names, name)
	}
	c.virtualRedirectSession = nil
	c.virtualRedirectMembers = map[string]struct{}{}
	return names
}

func (c *Client) virtualForwardActive(name string) bool {
	c.forwardsMu.Lock()
	defer c.forwardsMu.Unlock()
	_, member := c.virtualRedirectMembers[name]
	return member && c.virtualRedirectSession != nil && c.listeners[name] != nil && c.virtualAddrs[name] != ""
}

func (c *Client) closeVirtualRedirectSession() virtualRedirectSession {
	c.forwardsMu.Lock()
	defer c.forwardsMu.Unlock()
	session := c.virtualRedirectSession
	c.virtualRedirectSession = nil
	c.virtualRedirectMembers = map[string]struct{}{}
	return session
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
	dialer := &net.Dialer{Timeout: time.Duration(c.cfg.Connection.DialTimeoutSec) * time.Second}
	remote, err := tls.DialWithDialer(dialer, "tcp", c.cfg.ServerAddr, tlsCfg)
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

func (c *Client) clientTLSConfig() (*tls.Config, error) {
	if c != nil && c.tlsConfig != nil {
		return c.tlsConfig, nil
	}
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
