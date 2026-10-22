//go:build windows

package gui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestServiceLogTailDailyAndFallback(t *testing.T) {
	base := filepath.Join(t.TempDir(), "server-service.log")
	if err := os.WriteFile(base, []byte("legacy log\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := serviceLogTail(base, 1); !reflect.DeepEqual(got, []string{"legacy log"}) {
		t.Fatalf("base fallback = %q", got)
	}
	daily := serviceLogTailCandidates(base)[0]
	if err := os.WriteFile(daily, []byte("first\r\n\r\nsecond\r\nlast\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := serviceLogTail(base, 2); !reflect.DeepEqual(got, []string{"second", "last"}) {
		t.Fatalf("daily log tail = %q", got)
	}
	if got := lastNonEmptyLogLine(base); got != "last" {
		t.Fatalf("startup failure detail = %q", got)
	}
	for _, max := range []int{0, -1} {
		if got := serviceLogTail(base, max); len(got) != 0 {
			t.Fatalf("max=%d returned %q", max, got)
		}
	}
	if err := os.WriteFile(daily, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got := serviceLogTail(base, 1); len(got) != 0 {
		t.Fatalf("empty current log must not expose stale fallback: %q", got)
	}
}

func TestServiceLogTailBoundsOversizedLine(t *testing.T) {
	base := filepath.Join(t.TempDir(), "server-service.log")
	if err := os.WriteFile(base, []byte(strings.Repeat("x", 8*1024*1024)), 0600); err != nil {
		t.Fatal(err)
	}
	got := serviceLogTail(base, 1)
	if len(got) != 1 || len(got[0]) > 4*1024*1024 {
		t.Fatal("service diagnostics must keep the shared tail reader's 4 MiB limit")
	}
}

func TestServiceLogTailMissingFile(t *testing.T) {
	if got := serviceLogTail(filepath.Join(t.TempDir(), "missing.log"), 1); len(got) != 0 {
		t.Fatalf("missing log returned %q", got)
	}
}
