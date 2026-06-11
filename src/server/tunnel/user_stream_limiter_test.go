package tunnel

import "testing"

func TestUserStreamLimiterRejectsPerUserConcurrency(t *testing.T) {
	limiter := newUserStreamLimiter(SecurityConfig{MaxConcurrentStreamsPerUser: 1})

	release, ok := limiter.acquire("alice")
	if !ok {
		t.Fatal("first acquire was rejected")
	}
	defer release()

	if _, ok := limiter.acquire("alice"); ok {
		t.Fatal("second acquire for same user should be rejected")
	}
	if releaseBob, ok := limiter.acquire("bob"); !ok {
		t.Fatal("different user should not be rejected")
	} else {
		releaseBob()
	}

	release()
	if releaseAgain, ok := limiter.acquire("alice"); !ok {
		t.Fatal("acquire after release was rejected")
	} else {
		releaseAgain()
	}
}

func TestUserStreamLimiterDisabled(t *testing.T) {
	limiter := newUserStreamLimiter(SecurityConfig{})
	for i := 0; i < 3; i++ {
		release, ok := limiter.acquire("alice")
		if !ok {
			t.Fatalf("disabled limiter rejected acquire %d", i)
		}
		defer release()
	}
	if got := limiter.snapshot(); got.Active != 0 || got.MaxPerUser != 0 {
		t.Fatalf("snapshot = %#v, want disabled empty snapshot", got)
	}
}
