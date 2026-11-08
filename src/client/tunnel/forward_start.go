package tunnel

import (
	"errors"
	"fmt"
)

// startForwards isolates per-rule failures while keeping a single helper batch.
// Cancelling UAC alone must not prevent a successful login without active rules.
func (c *Client) startForwards() error {
	usableForwards := 0
	blockingStartupFailures := 0
	virtualAuthorizationCancelled := false
	var unusableErr error
	virtualForwards := make([]preparedVirtualForward, 0)
	for _, fwd := range c.cfg.Forwards {
		name := forwardName(fwd)
		if fwd.Direction == DirectionServerToClient {
			c.initForward(name, fwd, ForwardReverseWait, "等待服务端被动端口激活")
			c.log("reverse forward %s waiting on server %s -> client %s", name, fwd.ListenAddr, fwd.ServerTarget)
			go c.reverseLoop(c.ctx, name, fwd)
			usableForwards++
			continue
		}
		c.initForward(name, fwd, ForwardStarting, "正在监听客户端入口")
		if fwd.Direction == DirectionVirtual {
			prepared, err := c.prepareVirtualForwardListener(name, fwd)
			if err != nil {
				blockingStartupFailures++
				unusableErr = errors.Join(unusableErr, err)
				continue
			}
			if prepared.loopback {
				usableForwards++
				continue
			}
			virtualForwards = append(virtualForwards, prepared)
			continue
		}
		if err := c.ensureForwardListener(name, fwd); err != nil {
			blockingStartupFailures++
			unusableErr = errors.Join(unusableErr, err)
			continue
		}
		usableForwards++
	}
	if len(virtualForwards) > 0 {
		if err := c.startVirtualForwardBatch(virtualForwards); err != nil {
			if isVirtualAuthorizationCancelled(err) {
				virtualAuthorizationCancelled = true
			} else {
				blockingStartupFailures++
			}
			unusableErr = errors.Join(unusableErr, err)
		} else {
			usableForwards += len(virtualForwards)
		}
	}
	allowConnectedWithoutForward := virtualAuthorizationCancelled && blockingStartupFailures == 0
	if usableForwards == 0 && !allowConnectedWithoutForward {
		if unusableErr != nil {
			return fmt.Errorf("no usable forward is available: %w", unusableErr)
		}
		return fmt.Errorf("no usable forward is available")
	}
	return nil
}
