package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailTrackerPersistsBlockedIP(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "server-state.json")
	permanentFile := filepath.Join(t.TempDir(), "server-permanent-block.txt")
	cfg := SecurityConfig{
		AuthFailWindowSec: 60,
		AuthFailThreshold: 2,
		AuthFailBlockSec:  60,
	}
	tracker := newFailTracker(cfg, stateFile, permanentFile)
	tracker.addFailure("203.0.113.10")
	if tracker.isBlocked("203.0.113.10") {
		t.Fatal("IP should not be blocked before threshold")
	}
	tracker.addFailure("203.0.113.10")
	if !tracker.isBlocked("203.0.113.10") {
		t.Fatal("IP should be blocked after threshold")
	}

	reloaded := newFailTracker(cfg, stateFile, permanentFile)
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if !reloaded.isBlocked("203.0.113.10") {
		t.Fatal("persisted blocked IP was not restored")
	}
	if got := reloaded.snapshotBlocked(); len(got) != 1 || got[0].IP != "203.0.113.10" {
		t.Fatalf("snapshotBlocked = %#v", got)
	}
	if got, err := LoadBlockedIPs(stateFile); err != nil || len(got) != 1 || got[0].IP != "203.0.113.10" {
		t.Fatalf("LoadBlockedIPs = %#v, %v", got, err)
	}
}

func TestUnblockBlockedIPRemovesPersistedState(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "server-state.json")
	permanentFile := filepath.Join(t.TempDir(), "server-permanent-block.txt")
	cfg := SecurityConfig{
		AuthFailWindowSec: 60,
		AuthFailThreshold: 1,
		AuthFailBlockSec:  60,
	}
	tracker := newFailTracker(cfg, stateFile, permanentFile)
	tracker.addFailure("203.0.113.10")

	removed, err := UnblockBlockedIP(stateFile, "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected persisted blocked IP to be removed")
	}
	if got, err := LoadBlockedIPs(stateFile); err != nil || len(got) != 0 {
		t.Fatalf("LoadBlockedIPs = %#v, %v", got, err)
	}
}

func TestFailTrackerUnblockClearsMemoryAndPersistence(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "server-state.json")
	permanentFile := filepath.Join(t.TempDir(), "server-permanent-block.txt")
	cfg := SecurityConfig{
		AuthFailWindowSec: 60,
		AuthFailThreshold: 1,
		AuthFailBlockSec:  60,
	}
	tracker := newFailTracker(cfg, stateFile, permanentFile)
	tracker.addFailure("203.0.113.10")

	removed, err := tracker.unblock("203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected blocked IP to be removed")
	}
	if tracker.isBlocked("203.0.113.10") {
		t.Fatal("IP should not remain blocked after unblock")
	}
	if got, err := LoadBlockedIPs(stateFile); err != nil || len(got) != 0 {
		t.Fatalf("LoadBlockedIPs = %#v, %v", got, err)
	}
}

func TestFailTrackerPermanentBlockUsesDedicatedFile(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "server-state.json")
	permanentFile := filepath.Join(t.TempDir(), "server-permanent-block.txt")
	cfg := SecurityConfig{
		AuthFailWindowSec: 60,
		AuthFailThreshold: 2,
		AuthFailBlockSec:  60,
	}
	tracker := newFailTracker(cfg, stateFile, permanentFile)
	if tracker.addProtocolFailure("203.0.113.10") {
		t.Fatal("IP should not be permanently blocked before threshold")
	}
	if !tracker.addProtocolFailure("203.0.113.10") {
		t.Fatal("IP should be permanently blocked after threshold")
	}
	if !tracker.isBlocked("203.0.113.10") {
		t.Fatal("permanently blocked IP should be denied immediately")
	}
	if got, err := LoadBlockedIPs(stateFile); err != nil || len(got) != 0 {
		t.Fatalf("LoadBlockedIPs = %#v, %v", got, err)
	}

	reloaded := newFailTracker(cfg, stateFile, permanentFile)
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	if !reloaded.isBlocked("203.0.113.10") {
		t.Fatal("permanent blocked IP was not restored")
	}
}

func TestFailTrackerLoadRemovesLoopbackPermanentBlocks(t *testing.T) {
	permanentFile := filepath.Join(t.TempDir(), "server-permanent-block.txt")
	input := "# preserved\n127.0.0.1\n127.23.45.67\n::1\n203.0.113.10\n"
	if err := os.WriteFile(permanentFile, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}

	tracker := newFailTracker(SecurityConfig{}, "", permanentFile)
	tracker.permanent.Store("127.0.0.2", struct{}{})
	if err := tracker.load(); err != nil {
		t.Fatal(err)
	}

	for _, ip := range []string{"127.0.0.1", "127.23.45.67", "127.0.0.2", "::1"} {
		if tracker.hasPermanent(ip) {
			t.Fatalf("loopback IP %q remained permanently blocked", ip)
		}
	}
	if !tracker.hasPermanent("203.0.113.10") {
		t.Fatal("non-loopback permanent block was not restored")
	}

	data, err := os.ReadFile(permanentFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, ip := range []string{"127.0.0.1", "127.23.45.67", "::1"} {
		if strings.Contains(text, ip) {
			t.Fatalf("loopback IP %q remained in permanent block file: %q", ip, text)
		}
	}
	if !strings.Contains(text, "# preserved") || !strings.Contains(text, "203.0.113.10") {
		t.Fatalf("unrelated permanent block entries were not preserved: %q", text)
	}
}
