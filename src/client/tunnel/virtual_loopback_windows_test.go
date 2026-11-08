//go:build windows && !386

package tunnel

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestVirtualLoopbackDoesNotRequireRedirectHelper(t *testing.T) {
	oldStart := startVirtualRedirectSessionFn
	defer func() { startVirtualRedirectSessionFn = oldStart }()
	helperCalls := 0
	startVirtualRedirectSessionFn = func(context.Context, []virtualRedirectRule) (virtualRedirectSession, error) {
		helperCalls++
		return nil, errors.New("unexpected administrator helper")
	}
	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reserved.Addr().String()
	reserved.Close()
	cfg := validVirtualForwardConfig(t)
	cfg.TLS.CACertFile = writeTestServerCertificate(t, "vpn.example.test")
	cfg.Forwards[0].ListenAddr = addr
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := StartVerified(ctx, cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	name := forwardName(cfg.Forwards[0])
	if helperCalls != 0 || client.ForwardAddr(name) != addr || !client.virtualForwardActive(name) {
		t.Fatalf("helperCalls=%d, ForwardAddr=%q", helperCalls, client.ForwardAddr(name))
	}
	client.stopForwardListener(name)
	if client.virtualForwardActive(name) {
		t.Fatal("stopped loopback rule remained active")
	}
	if err := client.restoreCheckedForward(name, cfg.Forwards[0]); err != nil {
		t.Fatal(err)
	}
	if helperCalls != 0 || client.ForwardAddr(name) != addr {
		t.Fatal("restoring loopback must not request elevation or change its endpoint")
	}
	client.Close()
	if client.virtualForwardActive(name) {
		t.Fatal("closed loopback rule remained active")
	}
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		t.Fatalf("loopback port was not released: %v", err)
	}
	listener.Close()
}

func TestVirtualLoopbackReportsPortConflict(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := validVirtualForwardConfig(t)
	cfg.Forwards[0].ListenAddr = listener.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if client, err := StartVerified(ctx, cfg, "", nil); err == nil {
		client.Close()
		t.Fatal("loopback virtual rule must not hijack a port owned by another listener")
	}
}

func TestPrivateVirtualForwardStillUsesRedirectHelper(t *testing.T) {
	oldStart := startVirtualRedirectSessionFn
	defer func() { startVirtualRedirectSessionFn = oldStart }()
	helperCalls := 0
	startVirtualRedirectSessionFn = func(_ context.Context, rules []virtualRedirectRule) (virtualRedirectSession, error) {
		helperCalls++
		if len(rules) != 1 || rules[0].VirtualIP != "192.168.1.7" || rules[0].VirtualPort != 3389 {
			t.Fatalf("unexpected redirect rules: %+v", rules)
		}
		return nil, errors.New("administrator authorization for virtual forwarding was cancelled")
	}
	cfg := validVirtualForwardConfig(t)
	cfg.TLS.CACertFile = writeTestServerCertificate(t, "vpn.example.test")
	cfg.Forwards[0].ListenAddr = "192.168.1.7:3389"
	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	loopbackAddr := reserved.Addr().String()
	reserved.Close()
	cfg.Forwards = append(cfg.Forwards, ForwardConfig{
		Name: "loopback", Direction: DirectionVirtual, ListenAddr: loopbackAddr, ServerTarget: "127.0.0.1:3389",
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := StartVerified(ctx, cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if helperCalls != 1 || client.virtualForwardActive(forwardName(cfg.Forwards[0])) {
		t.Fatal("a private endpoint must still require successful administrator authorization")
	}
	if !client.virtualForwardActive("loopback") || client.ForwardAddr("loopback") != loopbackAddr {
		t.Fatal("cancelling driver authorization must not stop the direct loopback rule")
	}
}
