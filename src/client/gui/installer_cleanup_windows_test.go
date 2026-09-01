//go:build windows

package gui

import (
	"path/filepath"
	"testing"
)

func TestParseClientInstallerCleanupArgs(t *testing.T) {
	target := filepath.Join(`C:\Program Files`, "LSYL Tunnel Client", "bin", clientGUIExecutableName)
	opts, handled, err := parseClientInstallerCleanupArgs([]string{
		"-installer-cleanup",
		"-installed-client-exe", target,
		"-result-file", `C:\Temp\cleanup.txt`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || !opts.Cleanup || opts.InstalledClientExe != target {
		t.Fatalf("unexpected cleanup options: handled=%v opts=%+v", handled, opts)
	}
	if !IsNonInteractiveCommand([]string{"-installer-cleanup"}) {
		t.Fatal("installer cleanup command must be non-interactive")
	}
}

func TestParseClientInstallerCleanupArgsRejectsUnsafeTarget(t *testing.T) {
	_, handled, err := parseClientInstallerCleanupArgs([]string{
		"-installer-cleanup",
		"-installed-client-exe", `C:\Windows\System32\notepad.exe`,
	})
	if !handled || err == nil {
		t.Fatalf("handled=%v err=%v, want handled error", handled, err)
	}
}

func TestParseClientInstallerCleanupArgsRejectsDisabledCommand(t *testing.T) {
	_, handled, err := parseClientInstallerCleanupArgs([]string{
		"-installer-cleanup=false",
		"-installed-client-exe", filepath.Join(`C:\Program Files`, "LSYL Tunnel Client", "bin", clientGUIExecutableName),
	})
	if !handled || err == nil {
		t.Fatalf("handled=%v err=%v, want handled error", handled, err)
	}
}

func TestWinDivertServiceOwnedByInstall(t *testing.T) {
	root := filepath.Join(`C:\Program Files`, "LSYL Tunnel Client")
	owned := `\??\C:\Program Files\LSYL Tunnel Client\bin\WinDivert64.sys`
	if !winDivertServiceOwnedByInstall(owned, root) {
		t.Fatalf("owned service path was not recognized: %q", owned)
	}
	if winDivertServiceOwnedByInstall(`\??\C:\Other App\WinDivert64.sys`, root) {
		t.Fatal("unrelated WinDivert service path was treated as owned")
	}
	if winDivertServiceOwnedByInstall(`\??\C:\Program Files\LSYL Tunnel Client Other\WinDivert64.sys`, root) {
		t.Fatal("sibling directory was treated as the client install root")
	}
}

func TestSameWindowsPathIgnoresCaseAndCleans(t *testing.T) {
	left := filepath.Join(t.TempDir(), "bin", "..", "bin", clientGUIExecutableName)
	right := filepath.Join(filepath.Dir(left), "LSYL-TUNNEL-CLIENT-GUI.EXE")
	if !sameWindowsPath(left, right) {
		t.Fatalf("paths should match: %q %q", left, right)
	}
}
