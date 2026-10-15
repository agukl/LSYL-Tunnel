package tunnel

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"lsyltunnel/src/internal/protocol"
)

func TestHealthRetainsTypedCauseWithoutChangingJSON(t *testing.T) {
	c := &Client{}
	cause := fmt.Errorf("health: %w", &protocol.ResponseError{Code: "auth_blocked", Message: "try again"})
	status := c.setHealth(HealthAuthError, "display text", cause, true)
	status = c.finalizeHealthStatus(status)
	var response *protocol.ResponseError
	if !errors.As(status.Err(), &response) || response.Code != "auth_blocked" {
		t.Fatalf("health lost response error: %v", status.Err())
	}
	if c.Stats().Health.Err() != cause || status.LastError != cause.Error() {
		t.Fatal("snapshot lost original cause or log text")
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"state":"auth_error","message":"display text","last_checked":%q,"last_error":%q,"consecutive_failures":1,"terminal":true}`, status.LastChecked, cause.Error())
	if string(data) != want {
		t.Fatalf("health JSON changed: %s", data)
	}
	var decoded HealthStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Err() != nil {
		t.Fatal("in-process cause was serialized")
	}
	for _, state := range []string{HealthOK, HealthChecking} {
		c.setHealth(HealthAuthError, "failed", cause, true)
		if next := c.setHealth(state, "", nil, false); next.Err() != nil || next.LastError != "" {
			t.Fatalf("%s retained old failure: %+v", state, next)
		}
	}
}

func TestHealthRetryPreservesCauseUntilNextCheck(t *testing.T) {
	c := &Client{}
	cause := errors.New("network timeout")
	c.setHealth(HealthServerUnavailable, "reconnecting", cause, false)
	if c.setHealthRetry(time.Second).Err() != cause {
		t.Fatal("retry lost cause")
	}
	next := errors.New("connection refused")
	if c.setHealth(HealthServerUnavailable, "reconnecting", next, false).Err() != next {
		t.Fatal("next check retained stale cause")
	}
}
