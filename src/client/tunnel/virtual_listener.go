package tunnel

import (
	"fmt"
	"net"
	"runtime"
	"strconv"

	"lsyltunnel/src/internal/netaddr"
)

type preparedVirtualForward struct {
	name        string
	fwd         ForwardConfig
	listener    net.Listener
	virtualAddr string
	rule        virtualRedirectRule
	loopback    bool // Already listening; excluded from the elevated helper batch.
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
	if virtualHost == netaddr.LoopbackIPv4 {
		if err := c.ensureVirtualLoopbackListener(name, fwd); err != nil {
			c.setForwardState(name, ForwardListenFailed, ForwardErrorMessage(err))
			return preparedVirtualForward{}, err
		}
		return preparedVirtualForward{name: name, fwd: fwd, loopback: true}, nil
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

// Loopback already belongs to this host; bind the requested port without a driver bridge.
func (c *Client) ensureVirtualLoopbackListener(name string, fwd ForwardConfig) error {
	if runtime.GOOS != "windows" || runtime.GOARCH == "386" {
		return fmt.Errorf("virtual forwarding is only supported by the standard 64-bit Windows client")
	}
	host, port, err := parseVirtualListenAddr(fwd.ListenAddr)
	if err != nil {
		return err
	}
	if host != netaddr.LoopbackIPv4 {
		return fmt.Errorf("virtual loopback listener requires 127.0.0.1")
	}
	fwd.ListenAddr = net.JoinHostPort(host, strconv.Itoa(port))
	if err := c.ensureForwardListener(name, fwd); err != nil {
		return err
	}
	c.forwardsMu.Lock()
	listener := c.listeners[name]
	if listener != nil {
		c.virtualAddrs[name] = listener.Addr().String()
	}
	c.forwardsMu.Unlock()
	return nil
}

func (c *Client) activateVirtualForward(prepared preparedVirtualForward) {
	c.setForwardState(prepared.name, ForwardListening, "虚拟端点已接管")
	c.log("forward %s virtual endpoint %s -> local redirect %s -> %s", prepared.name, prepared.virtualAddr, prepared.listener.Addr(), prepared.fwd.ServerTarget)
	go c.acceptLoop(c.ctx, prepared.listener, prepared.name, prepared.fwd)
}
