package tunnel

import (
	"fmt"
	"net"
)

type preparedVirtualForward struct {
	name        string
	fwd         ForwardConfig
	listener    net.Listener
	virtualAddr string
	rule        virtualRedirectRule
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
