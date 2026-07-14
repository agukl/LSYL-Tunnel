//go:build !windows || 386

package tunnel

import (
	"context"
	"fmt"
)

func startVirtualRedirectSession(_ context.Context, rules []virtualRedirectRule) (virtualRedirectSession, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	return nil, fmt.Errorf("virtual forwarding is only supported by the standard 64-bit Windows client")
}

func HandleVirtualRedirectHelperArgs([]string) (bool, error) {
	return false, nil
}
