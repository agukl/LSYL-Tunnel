package tunnel

import (
	"context"
	"crypto/tls"
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
	if err := client.startForwards(); err != nil {
		_ = client.Close()
		return nil, err
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
