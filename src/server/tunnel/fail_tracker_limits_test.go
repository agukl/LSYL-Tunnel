package tunnel

import (
	"testing"
	"time"
)

func TestFailTrackerCleanupExpiresIncompleteFailures(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	tracker := newFailTracker(SecurityConfig{
		AuthFailWindowSec:        60,
		AuthFailThreshold:        3,
		AuthFailBlockSec:         60,
		MaxTrackedFailureIPs:     2,
		FailureTrackerCleanupSec: 1,
	}, "", "")
	tracker.now = func() time.Time { return now }
	tracker.addFailure("203.0.113.10")
	if got := tracker.stats().TrackedIPs; got != 1 {
		t.Fatalf("tracked IPs before cleanup = %d, want 1", got)
	}
	now = now.Add(61 * time.Second)
	if err := tracker.cleanup(); err != nil {
		t.Fatal(err)
	}
	if got := tracker.stats().TrackedIPs; got != 0 {
		t.Fatalf("tracked IPs after cleanup = %d, want 0", got)
	}
}

func TestFailTrackerEvictsOldestIncompleteFailureAtCapacity(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	tracker := newFailTracker(SecurityConfig{
		AuthFailWindowSec:    300,
		AuthFailThreshold:    3,
		AuthFailBlockSec:     60,
		MaxTrackedFailureIPs: 2,
	}, "", "")
	tracker.now = func() time.Time { return now }
	tracker.addFailure("203.0.113.10")
	now = now.Add(time.Second)
	tracker.addFailure("203.0.113.11")
	now = now.Add(time.Second)
	tracker.addFailure("203.0.113.12")

	tracker.mu.RLock()
	_, hasOldest := tracker.items["203.0.113.10"]
	_, hasSecond := tracker.items["203.0.113.11"]
	_, hasNewest := tracker.items["203.0.113.12"]
	tracker.mu.RUnlock()
	if hasOldest || !hasSecond || !hasNewest {
		t.Fatalf("tracked items = oldest:%t second:%t newest:%t", hasOldest, hasSecond, hasNewest)
	}
	if got := tracker.stats().EvictedIPs; got != 1 {
		t.Fatalf("evicted IPs = %d, want 1", got)
	}
}

func TestFailTrackerDoesNotEvictActiveTemporaryBlock(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	tracker := newFailTracker(SecurityConfig{
		AuthFailWindowSec:    300,
		AuthFailThreshold:    1,
		AuthFailBlockSec:     60,
		MaxTrackedFailureIPs: 1,
	}, "", "")
	tracker.now = func() time.Time { return now }
	tracker.addFailure("203.0.113.10")
	now = now.Add(time.Second)
	tracker.addFailure("203.0.113.11")

	if !tracker.isBlocked("203.0.113.10") {
		t.Fatal("active temporary block was evicted")
	}
	if got := tracker.stats().CapacityDrops; got != 1 {
		t.Fatalf("capacity drops = %d, want 1", got)
	}
}
