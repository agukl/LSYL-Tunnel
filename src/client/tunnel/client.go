package tunnel

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"lsyltunnel/src/internal/transport"
)

type Client struct {
	cfg                    Config
	tlsConfig              *tls.Config
	certificateGateKey     transport.CertificateGateKey
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

func Run(ctx context.Context, cfg Config, logf transport.LogFunc) error {
	client, err := Start(ctx, cfg, logf)
	if err != nil {
		return err
	}
	defer client.Close()
	<-ctx.Done()
	return nil
}

func Start(ctx context.Context, cfg Config, logf transport.LogFunc) (*Client, error) {
	return start(ctx, cfg, "", false, logf)
}

func StartVerified(ctx context.Context, cfg Config, serverVersion string, logf transport.LogFunc) (*Client, error) {
	return start(ctx, cfg, serverVersion, true, logf)
}

func start(ctx context.Context, cfg Config, serverVersion string, verified bool, logf transport.LogFunc) (*Client, error) {
	ApplyDefaults(&cfg)
	tlsCfg, certificateGateKey, err := clientTLSMaterials(cfg)
	if err != nil {
		return nil, err
	}
	client := &Client{
		cfg:                    cfg,
		tlsConfig:              tlsCfg,
		certificateGateKey:     certificateGateKey,
		ctx:                    ctx,
		listeners:              map[string]net.Listener{},
		virtualAddrs:           map[string]string{},
		virtualRedirectMembers: map[string]struct{}{},
		forwards:               map[string]*forwardRuntime{},
		logf:                   logf,
		closed:                 make(chan struct{}),
	}
	if verified {
		client.setHealth(HealthOK, "服务端连接正常", nil, false)
		client.SetServerVersion(serverVersion)
	} else {
		client.setHealth(HealthChecking, "等待服务端健康检查", nil, false)
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

func (c *Client) log(format string, args ...any) {
	if c.logf != nil {
		c.logf(format, args...)
	}
}
