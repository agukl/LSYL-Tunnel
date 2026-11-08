package tunnel

import (
	"fmt"
	"net"

	"lsyltunnel/src/internal/netaddr"
)

func (c *Client) startVirtualForwardBatch(prepared []preparedVirtualForward) error {
	rules := make([]virtualRedirectRule, 0, len(prepared))
	for _, forward := range prepared {
		rules = append(rules, forward.rule)
	}
	session, err := startVirtualRedirectSessionFn(c.ctx, rules)
	if err == nil && session == nil {
		err = fmt.Errorf("virtual redirect helper did not start")
	}
	if err != nil {
		for _, forward := range prepared {
			c.stopForwardListener(forward.name)
			c.setForwardState(forward.name, ForwardListenFailed, ForwardErrorMessage(err))
			c.log("forward %s virtual endpoint setup failed: %v", forward.name, err)
		}
		return err
	}
	if !c.attachVirtualRedirectSession(prepared, session) {
		_ = session.Close()
		err := fmt.Errorf("virtual forward listeners are no longer active")
		for _, forward := range prepared {
			c.stopForwardListener(forward.name)
			c.setForwardState(forward.name, ForwardListenFailed, ForwardErrorMessage(err))
		}
		return err
	}
	for _, forward := range prepared {
		c.activateVirtualForward(forward)
	}
	go c.watchVirtualRedirectSession(session)
	return nil
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
	if host, _, err := net.SplitHostPort(c.virtualAddrs[name]); err == nil && host == netaddr.LoopbackIPv4 {
		return c.listeners[name] != nil
	}
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
