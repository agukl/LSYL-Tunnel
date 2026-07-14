//go:build windows && !386

package tunnel

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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
		"-virtual-redirect-control", "127.0.0.1:41412",
		"-virtual-redirect-token", "0123456789abcdef0123456789abcdef",
		"-virtual-redirect-anchor-pid", "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || opts.Action != virtualRedirectHelperSession || opts.AnchorPID != 42 || len(opts.Rules) != 1 || opts.ControlAddr != "127.0.0.1:41412" {
		t.Fatalf("parsed helper options = %+v, handled = %v", opts, handled)
	}
}

func TestVirtualRedirectControlProtocol(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef"
	server, err := newVirtualRedirectControlServer(token)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client, err := connectVirtualRedirectControl(server.Address(), token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	waitResult := func() virtualRedirectHelperResult {
		t.Helper()
		select {
		case result := <-server.Results():
			return result
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for virtual redirect control result")
			return virtualRedirectHelperResult{}
		}
	}
	if result := waitResult(); !result.Connected || result.Token != token {
		t.Fatalf("connected result = %+v", result)
	}
	if err := client.Send(virtualRedirectHelperResult{Ready: true}); err != nil {
		t.Fatal(err)
	}
	if result := waitResult(); !result.Ready || result.Stopped {
		t.Fatalf("ready result = %+v", result)
	}
	if err := client.Send(virtualRedirectHelperResult{Ready: true, Stopped: true}); err != nil {
		t.Fatal(err)
	}
	if result := waitResult(); !result.Ready || !result.Stopped {
		t.Fatalf("stopped result = %+v", result)
	}
	select {
	case <-server.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("virtual redirect control server did not close")
	}
	if err := server.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestVirtualRedirectControlAddressRequiresIPv4Loopback(t *testing.T) {
	for _, value := range []string{"0.0.0.0:41412", "192.0.2.10:41412", "localhost:41412", "[::1]:41412"} {
		if _, err := normalizeVirtualRedirectControlAddress(value); err == nil {
			t.Fatalf("normalizeVirtualRedirectControlAddress(%q) succeeded", value)
		}
	}
}

func TestVendoredWinDivertRuntimeMatchesPinnedHashes(t *testing.T) {
	root := filepath.Join("..", "..", "..", "third_party", "windivert", "2.2.2", "x64")
	if err := verifyFileSHA256(filepath.Join(root, "WinDivert.dll"), winDivertDLLSHA256); err != nil {
		t.Fatalf("WinDivert.dll: %v", err)
	}
	if err := verifyFileSHA256(filepath.Join(root, "WinDivert64.sys"), winDivertDriverSHA256); err != nil {
		t.Fatalf("WinDivert64.sys: %v", err)
	}
}

func TestResolveWinDivertDLLRequiresPinnedExeAdjacentRuntime(t *testing.T) {
	sourceRoot := filepath.Join("..", "..", "..", "third_party", "windivert", "2.2.2", "x64")
	targetRoot := t.TempDir()
	for _, name := range []string{"WinDivert.dll", "WinDivert64.sys"} {
		data, err := os.ReadFile(filepath.Join(sourceRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(targetRoot, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want := filepath.Join(targetRoot, "WinDivert.dll")
	got, err := resolveWinDivertDLLForExecutable(filepath.Join(targetRoot, "lsyl-tunnel-client-gui.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolved DLL = %q, want %q", got, want)
	}

	file, err := os.OpenFile(want, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("modified")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = resolveWinDivertDLLForExecutable(filepath.Join(targetRoot, "lsyl-tunnel-client-gui.exe"))
	if err == nil || !strings.Contains(err.Error(), "integrity check failed") {
		t.Fatalf("modified runtime error = %v, want integrity failure", err)
	}
}
