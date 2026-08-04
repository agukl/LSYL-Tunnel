package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemovePermanentBlockedIPRemovesAllMatchingLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server-permanent-block.txt")
	input := "# keep comment\n203.0.113.10\ninvalid\n203.0.113.20\n203.0.113.10\n"
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}

	removed, err := RemovePermanentBlockedIP(path, "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected IP to be removed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "203.0.113.10") {
		t.Fatalf("removed IP still present: %q", text)
	}
	if !strings.Contains(text, "# keep comment") || !strings.Contains(text, "invalid") || !strings.Contains(text, "203.0.113.20") {
		t.Fatalf("unrelated lines were not preserved: %q", text)
	}
}

func TestRemovePermanentBlockedIPsPreservesUnrelatedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server-permanent-block.txt")
	input := "# keep comment\r\n203.0.113.10\r\n\r\n2001:0db8:0:0:0:0:0:1\r\ninvalid\r\n203.0.113.20\r\n203.0.113.10\r\n"
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}

	removed, err := removePermanentBlockedIPs(path, map[string]struct{}{
		"203.0.113.10": {},
		"2001:db8::1":  {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed IPs = %#v, want two canonical addresses", removed)
	}
	for _, ip := range []string{"203.0.113.10", "2001:db8::1"} {
		if _, ok := removed[ip]; !ok {
			t.Fatalf("removed IPs = %#v, missing %s", removed, ip)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "# keep comment\n\ninvalid\n203.0.113.20\n"
	if got := string(data); got != want {
		t.Fatalf("permanent block file = %q, want %q", got, want)
	}
}

func TestRemovePermanentBlockedIPsMissingFileIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "server-permanent-block.txt")
	removed, err := removePermanentBlockedIPs(path, map[string]struct{}{"203.0.113.10": {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed IPs = %#v, want empty", removed)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing permanent block file was created: %v", err)
	}
}

func TestFailTrackerUnblockPermanentRemovesMemoryAndFile(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "server-state.json")
	permanentFile := filepath.Join(t.TempDir(), "server-permanent-block.txt")
	tracker := newFailTracker(SecurityConfig{AuthFailWindowSec: 60, AuthFailThreshold: 1, AuthFailBlockSec: 60}, stateFile, permanentFile)
	tracker.permanent.Store("203.0.113.10", struct{}{})
	if err := os.WriteFile(permanentFile, []byte("203.0.113.10\n203.0.113.20\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	removed, err := tracker.unblockPermanent("203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected permanent block to be removed")
	}
	if tracker.isBlocked("203.0.113.10") {
		t.Fatal("IP should not remain blocked in memory")
	}
	data, err := os.ReadFile(permanentFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "203.0.113.10") || !strings.Contains(text, "203.0.113.20") {
		t.Fatalf("unexpected permanent block file content: %q", text)
	}
}

func TestFailTrackerUnblockPermanentNormalizesIPv6MemoryKey(t *testing.T) {
	permanentFile := filepath.Join(t.TempDir(), "server-permanent-block.txt")
	expanded := "2001:0db8:0:0:0:0:0:1"
	if err := os.WriteFile(permanentFile, []byte(expanded+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tracker := newFailTracker(SecurityConfig{}, "", permanentFile)
	if err := tracker.load(); err != nil {
		t.Fatal(err)
	}
	if !tracker.hasPermanent("2001:db8::1") {
		t.Fatal("canonical IPv6 permanent block was not loaded")
	}

	removed, err := tracker.unblockPermanent(expanded)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expanded IPv6 permanent block was not removed")
	}
	if tracker.hasPermanent("2001:db8::1") {
		t.Fatal("canonical IPv6 key remained blocked in memory")
	}
}
