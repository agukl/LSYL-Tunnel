//go:build windows

package gui

import (
	"testing"
	"time"

	"github.com/lxn/win"
)

func TestRunActivatesExistingServerGUIWithoutStartingAnotherApp(t *testing.T) {
	originalClaim := claimServerGUIInstanceFn
	originalActivate := activateExistingServerGUIFn
	originalRun := runServerGUIAppFn
	t.Cleanup(func() {
		claimServerGUIInstanceFn = originalClaim
		activateExistingServerGUIFn = originalActivate
		runServerGUIAppFn = originalRun
	})

	released := false
	activated := false
	started := false
	claimServerGUIInstanceFn = func() (func(), bool, error) {
		return func() { released = true }, true, nil
	}
	activateExistingServerGUIFn = func() bool {
		activated = true
		return true
	}
	runServerGUIAppFn = func() error {
		started = true
		return nil
	}

	if err := Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !activated {
		t.Fatal("Run() did not activate the existing server GUI")
	}
	if started {
		t.Fatal("Run() started a second server GUI")
	}
	if !released {
		t.Fatal("Run() did not release the instance claim")
	}
}

func TestRunOwnsServerGUIInstanceUntilAppStops(t *testing.T) {
	originalClaim := claimServerGUIInstanceFn
	originalActivate := activateExistingServerGUIFn
	originalRun := runServerGUIAppFn
	t.Cleanup(func() {
		claimServerGUIInstanceFn = originalClaim
		activateExistingServerGUIFn = originalActivate
		runServerGUIAppFn = originalRun
	})

	released := false
	activated := false
	started := false
	claimServerGUIInstanceFn = func() (func(), bool, error) {
		return func() { released = true }, false, nil
	}
	activateExistingServerGUIFn = func() bool {
		activated = true
		return true
	}
	runServerGUIAppFn = func() error {
		if released {
			t.Fatal("instance claim released before the app stopped")
		}
		started = true
		return nil
	}

	if err := Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !started {
		t.Fatal("Run() did not start the first server GUI")
	}
	if activated {
		t.Fatal("Run() tried to activate another GUI for the first instance")
	}
	if !released {
		t.Fatal("Run() did not release the instance claim after the app stopped")
	}
}

func TestActivateExistingServerGUIRetriesUntilWindowExists(t *testing.T) {
	originalFind := findServerGUIWindowFn
	originalRestore := restoreServerGUIWindowFn
	originalSleep := serverGUIActivationSleepFn
	t.Cleanup(func() {
		findServerGUIWindowFn = originalFind
		restoreServerGUIWindowFn = originalRestore
		serverGUIActivationSleepFn = originalSleep
	})

	findCalls := 0
	restored := win.HWND(0)
	sleepCalls := 0
	findServerGUIWindowFn = func() win.HWND {
		findCalls++
		if findCalls < 2 {
			return 0
		}
		return win.HWND(42)
	}
	restoreServerGUIWindowFn = func(hwnd win.HWND) {
		restored = hwnd
	}
	serverGUIActivationSleepFn = func(time.Duration) {
		sleepCalls++
	}

	if !activateExistingServerGUI() {
		t.Fatal("activateExistingServerGUI() = false, want true")
	}
	if restored != win.HWND(42) {
		t.Fatalf("restored window = %v, want 42", restored)
	}
	if findCalls != 2 || sleepCalls != 1 {
		t.Fatalf("find calls = %d, sleep calls = %d, want 2 and 1", findCalls, sleepCalls)
	}
}
