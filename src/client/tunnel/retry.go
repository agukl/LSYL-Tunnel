package tunnel

import (
	"strings"
	"time"
)

func ReverseRetryDelay(err error, failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	if err != nil {
		if policy, ok := policyForResponse(err); ok {
			if policy.retryDelay > 0 {
				return policy.retryDelay
			}
		} else {
			text := strings.ToLower(err.Error())
			if isTemporaryAuthBlockText(text) || containsAny(text, "already activated") {
				return 5 * time.Minute
			}
		}
	}
	delay := time.Duration(failures) * 2 * time.Second
	if delay < 2*time.Second {
		return 2 * time.Second
	}
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func ReconnectDelay(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := healthReconnectInitialInterval
	for i := 1; i < failures; i++ {
		delay *= 2
		if delay >= healthReconnectMaxInterval {
			return healthReconnectMaxInterval
		}
	}
	if delay > healthReconnectMaxInterval {
		return healthReconnectMaxInterval
	}
	return delay
}
