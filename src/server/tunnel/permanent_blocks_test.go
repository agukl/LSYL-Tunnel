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
