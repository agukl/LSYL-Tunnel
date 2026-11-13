package tunnel

import (
	"fmt"
	"testing"
	"time"

	"lsyltunnel/src/internal/protocol"
)

func TestEntryQualityTrackerValidatesAndExpiresReports(t *testing.T) {
	tracker := newEntryQualityTracker()
	entries := []BusinessEntryConfig{{AdvertiseAddr: "[2001:DB8::1]:50000"}}
	report := []protocol.EntryQuality{
		{Endpoint: "bootstrap", Attempts: 0},
		{Endpoint: "[2001:db8::1]:50000", Attempts: 2, Successes: 1, Failures: 1, AvgDialMS: 12, LastError: "timeout"},
	}
	tracker.observe("alice", "desktop", "192.0.2.1", report, entries)
	if got := tracker.snapshot(); len(got) != 1 || got[0].Entries[1].Failures != 1 {
		t.Fatalf("stored quality = %+v", got)
	}
	tracker.observe("alice", "desktop", "192.0.2.1", []protocol.EntryQuality{{Endpoint: "unconfigured", Attempts: 1, Successes: 1}}, entries)
	if got := tracker.snapshot(); len(got) != 1 || got[0].Entries[1].Failures != 1 {
		t.Fatalf("invalid report replaced quality: %+v", got)
	}
	tracker.mu.Lock()
	status := tracker.clients["alice\x00desktop"]
	status.UpdatedAt = time.Now().Add(-entryQualityTTL - time.Second)
	tracker.clients["alice\x00desktop"] = status
	tracker.mu.Unlock()
	if got := tracker.snapshot(); len(got) != 0 {
		t.Fatalf("expired reports = %+v", got)
	}
}

func TestEntryQualityTrackerHasClientLimit(t *testing.T) {
	tracker := newEntryQualityTracker()
	report := []protocol.EntryQuality{{Endpoint: "bootstrap", Attempts: 1, Successes: 1}}
	for i := 0; i < entryQualityMaxClients+4; i++ {
		tracker.observe("alice", fmt.Sprintf("client-%d", i), "192.0.2.1", report, nil)
	}
	if got := len(tracker.snapshot()); got != entryQualityMaxClients {
		t.Fatalf("tracked clients = %d", got)
	}
}
