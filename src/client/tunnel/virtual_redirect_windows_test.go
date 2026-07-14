//go:build windows && !386

package tunnel

import (
	"reflect"
	"testing"
	"unsafe"
)

func TestVirtualRedirectWindowsABILayouts(t *testing.T) {
	if got := unsafe.Sizeof(winDivertAddress{}); got != 80 {
		t.Fatalf("WINDIVERT_ADDRESS size = %d, want 80", got)
	}
	if got := unsafe.Sizeof(shellExecuteInfo{}); got != 112 {
		t.Fatalf("SHELLEXECUTEINFOW size = %d, want 112", got)
	}
}

func TestVirtualRedirectRuleHelperRoundTrip(t *testing.T) {
	want := []virtualRedirectRule{
		{VirtualIP: "203.0.113.10", VirtualPort: 443, LocalPort: 45001},
		{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000},
	}
	encoded, err := encodeVirtualRedirectRules(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVirtualRedirectRules(encoded)
	if err != nil {
		t.Fatal(err)
	}
	want, err = normalizeVirtualRedirectRules(want)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded rules = %+v, want %+v", got, want)
	}
}

func TestParseVirtualRedirectHelperArgs(t *testing.T) {
	rulesText, err := encodeVirtualRedirectRules([]virtualRedirectRule{{
		VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000,
	}})
	if err != nil {
		t.Fatal(err)
	}
	opts, handled, err := parseVirtualRedirectHelperArgs([]string{
		"-virtual-redirect-helper", "session",
		"-virtual-redirect-rules", rulesText,
		"-virtual-redirect-result", "0123456789abcdef0123456789abcdef",
		"-virtual-redirect-anchor-pid", "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || opts.Action != virtualRedirectHelperSession || opts.AnchorPID != 42 || len(opts.Rules) != 1 {
		t.Fatalf("parsed helper options = %+v, handled = %v", opts, handled)
	}
}
