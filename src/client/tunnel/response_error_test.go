package tunnel

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"lsyltunnel/src/internal/protocol"
)

func TestResponseErrorPreservesCodeAndText(t *testing.T) {
	for _, tc := range []struct {
		code, message, fallback, want string
	}{
		{"auth_failed", "denied", "fallback", "auth_failed: denied"},
		{"auth_failed", "", "fallback", "auth_failed: fallback"},
		{"", "denied", "fallback", "denied"},
		{"", "", "fallback", "fallback"},
	} {
		err := responseError(protocol.OpenResponse{Code: tc.code, Message: tc.message}, tc.fallback)
		if err.Error() != tc.want {
			t.Fatalf("error = %q, want %q", err, tc.want)
		}
		var response *protocol.ResponseError
		if !errors.As(fmt.Errorf("request: %w", err), &response) || response.Code != tc.code {
			t.Fatalf("wrapped error lost code %q: %v", tc.code, err)
		}
	}
}

func TestResponsePoliciesMatchExistingMessages(t *testing.T) {
	for _, tc := range []struct{ code, message string }{
		{"auth_failed", "username or password is incorrect"},
		{"credential_expired", "saved login has expired"},
		{"auth_blocked", "too many login failures, temporarily blocked; try later"},
		{"client_version_unsupported", "unsupported client version"},
		{"protocol_version_unsupported", "unsupported protocol version"},
		{"bad_request", "invalid tunnel request"},
		{"target_denied", "user is not allowed to access this target"},
		{"target_denied", "reverse listen address is not configured on server"},
		{"target_denied", "user is not allowed to activate this reverse listen address"},
		{"target_unreachable", "target service is unreachable"},
		{"user_stream_limit", "too many concurrent streams for account"},
		{"reverse_failed", "reverse listen address is not configured on server"},
		{"reverse_failed", "already activated"},
		{"future_code", "connection reset"},
	} {
		t.Run(tc.code+"/"+tc.message, func(t *testing.T) {
			typed := fmt.Errorf("request: %w", responseError(protocol.OpenResponse{Code: tc.code, Message: tc.message}, ""))
			legacy := errors.New(typed.Error())
			state, message := classifyHealthError(typed)
			wantState, wantMessage := classifyHealthError(legacy)
			if state != wantState || message != wantMessage {
				t.Fatalf("health = %q / %q, legacy = %q / %q", state, message, wantState, wantMessage)
			}
			if ForwardErrorMessage(typed) != ForwardErrorMessage(legacy) {
				t.Fatalf("forward message changed: %q != %q", ForwardErrorMessage(typed), ForwardErrorMessage(legacy))
			}
			if IsPermanentForwardError(typed) != IsPermanentForwardError(legacy) {
				t.Fatal("forward retry policy changed")
			}
			if ReverseRetryDelay(typed, 2) != ReverseRetryDelay(legacy, 2) {
				t.Fatal("reverse retry delay changed")
			}
		})
	}
}

func TestResponsePolicyDoesNotDependOnDiagnosticWords(t *testing.T) {
	for _, tc := range []struct {
		code      string
		permanent bool
		state     string
		delay     time.Duration
	}{
		{"auth_failed", true, HealthAuthError, 2 * time.Second},
		{"credential_expired", true, HealthAuthError, 2 * time.Second},
		{"auth_blocked", false, HealthAuthError, 5 * time.Minute},
		{"target_denied", true, HealthServerUnavailable, 2 * time.Second},
		{"target_unreachable", false, HealthServerUnavailable, 2 * time.Second},
		{"user_stream_limit", false, HealthServerUnavailable, 2 * time.Second},
	} {
		// Words that used to override the actual response code must not affect policy.
		err := fmt.Errorf("context deadline: %w", responseError(protocol.OpenResponse{
			Code: tc.code, Message: "certificate auth_failed timeout already activated",
		}, ""))
		if got := IsPermanentForwardError(err); got != tc.permanent {
			t.Errorf("%s permanent = %v, want %v", tc.code, got, tc.permanent)
		}
		if state, _ := classifyHealthError(err); state != tc.state {
			t.Errorf("%s health = %s, want %s", tc.code, state, tc.state)
		}
		if got := ReverseRetryDelay(err, 1); got != tc.delay {
			t.Errorf("%s retry = %v, want %v", tc.code, got, tc.delay)
		}
	}
}
