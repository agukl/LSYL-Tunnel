//go:build windows

package gui

import (
	"context"
	"errors"
	"testing"

	"lsyltunnel/src/client/tunnel"
)

func TestStopClientWhenNotRunning(t *testing.T) {
	a := &App{}
	if a.isRunning() {
		t.Fatal("new app must not be running")
	}
	if err := a.stopClient(); err == nil {
		t.Fatal("stopping an idle client must report an error")
	}
}

func TestStartClientAfterShutdown(t *testing.T) {
	a := &App{}
	a.closing.Store(true)
	if err := a.startClient(tunnel.Config{}, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("start after shutdown = %v, want context.Canceled", err)
	}
	if a.isRunning() {
		t.Fatal("shutdown app must not attach a client")
	}
}
