package tunnel

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"lsyltunnel/src/internal/netaddr"
	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
)

type ForwardCheckSummary struct {
	Checked  int `json:"checked"`
	Allowed  int `json:"allowed"`
	Rejected int `json:"rejected"`
	Failed   int `json:"failed"`
}

func (c *Client) CheckForwardsNow(ctx context.Context) ForwardCheckSummary {
	var summary ForwardCheckSummary
	if c == nil {
		return summary
	}
	timeout := time.Duration(c.cfg.Connection.DialTimeoutSec+2) * time.Second
	if timeout < 5*time.Second {
		timeout = 5 * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, fwd := range c.cfg.Forwards {
		fwd := fwd
		name := forwardName(fwd)
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := c.checkForwardNow(checkCtx, name, fwd)
			mu.Lock()
			summary.Checked++
			switch result {
			case "allowed":
				summary.Allowed++
			case "rejected":
				summary.Rejected++
			default:
				summary.Failed++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return summary
}

func (c *Client) checkForwardNow(ctx context.Context, name string, fwd ForwardConfig) string {
	resp, err := c.checkForwardResponse(ctx, name, fwd)
	if err != nil {
		c.recordForwardError(name, err)
		if IsPermanentForwardError(err) {
			c.setForwardState(name, ForwardRejected, ForwardErrorMessage(err))
			if forwardDirection(fwd) == DirectionClientToServer {
				c.stopForwardListener(name)
			}
			return "rejected"
		}
		return "failed"
	}
	if !resp.OK {
		err := responseError(resp, "server rejected forward check")
		c.recordForwardError(name, err)
		if IsPermanentForwardError(err) {
			c.setForwardState(name, ForwardRejected, ForwardErrorMessage(err))
			if forwardDirection(fwd) == DirectionClientToServer {
				c.stopForwardListener(name)
			}
			return "rejected"
		}
		return "failed"
	}
	if err := c.restoreCheckedForward(name, fwd); err != nil {
		c.recordForwardError(name, err)
		if fwd.Direction == DirectionVirtual {
			c.setForwardState(name, ForwardListenFailed, ForwardErrorMessage(err))
		}
		return "failed"
	}
	return "allowed"
}

func (c *Client) restoreCheckedForward(name string, fwd ForwardConfig) error {
	switch strings.TrimSpace(fwd.Direction) {
	case DirectionVirtual:
		if host, _, err := parseVirtualListenAddr(fwd.ListenAddr); err == nil && host == netaddr.LoopbackIPv4 {
			return c.ensureVirtualLoopbackListener(name, fwd)
		}
		if !c.virtualForwardActive(name) {
			return fmt.Errorf("virtual redirect is inactive; disconnect and reconnect to request administrator authorization")
		}
	case "", DirectionClientToServer:
		if err := c.ensureForwardListener(name, fwd); err != nil {
			return err
		}
	case DirectionServerToClient:
		if current := c.forward(name); current != nil {
			current.mu.Lock()
			wasRejected := current.state == ForwardRejected
			current.mu.Unlock()
			if wasRejected {
				c.setForwardState(name, ForwardReverseWait, "等待服务端被动端口激活")
				go c.reverseLoop(c.ctx, name, fwd)
			}
		}
	}
	return nil
}

func (c *Client) checkForwardResponse(ctx context.Context, name string, fwd ForwardConfig) (protocol.OpenResponse, error) {
	var resp protocol.OpenResponse
	tlsCfg, err := c.clientTLSConfig()
	if err != nil {
		return resp, err
	}
	timeout := time.Duration(c.cfg.Connection.DialTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	conn, err := transport.DialTLSWithCertificateGate(
		ctx,
		&net.Dialer{Timeout: timeout},
		"tcp",
		c.cfg.ServerAddr,
		tlsCfg,
		c.certificateGateKey,
		timeout,
	)
	if err != nil {
		return resp, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	req := newOpenRequest(c.cfg, "forward_check")
	req.ForwardName = name
	req.Direction = forwardDirection(fwd)
	if req.Direction == DirectionServerToClient {
		req.ListenAddr = fwd.ListenAddr
	}
	req.Target = fwd.ServerTarget
	if err := protocol.WriteJSON(conn, req); err != nil {
		return resp, err
	}
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		return resp, err
	}
	c.setServerVersionFromResponse(resp)
	if !resp.OK {
		return resp, responseError(resp, "server rejected forward check")
	}
	return resp, nil
}

func forwardDirection(fwd ForwardConfig) string {
	direction := strings.TrimSpace(fwd.Direction)
	if direction == "" || direction == DirectionVirtual {
		return DirectionClientToServer
	}
	return direction
}

func ForwardCheckMessage(summary ForwardCheckSummary) string {
	if summary.Checked == 0 {
		return "服务端连接正常"
	}
	if summary.Rejected > 0 || summary.Failed > 0 {
		return fmt.Sprintf("服务端连接正常，已检查 %d 个端口，%d 个异常", summary.Checked, summary.Rejected+summary.Failed)
	}
	return fmt.Sprintf("服务端连接正常，已检查 %d 个端口", summary.Checked)
}
