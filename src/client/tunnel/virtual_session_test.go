package tunnel

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type testVirtualRedirectSession struct {
	done chan struct{}
	once sync.Once
	mu   sync.Mutex
	err  error
}

func (s *testVirtualRedirectSession) Close() error {
	s.once.Do(func() { close(s.done) })
	return nil
}

func (s *testVirtualRedirectSession) Done() <-chan struct{} { return s.done }

func (s *testVirtualRedirectSession) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *testVirtualRedirectSession) fail(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
	s.once.Do(func() { close(s.done) })
}

func TestVirtualForwardUsesRandomLocalRedirectListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &Client{
		cfg: Config{
			TLS:        TLSConfig{CACertFile: writeTestServerCertificate(t, "192.0.2.22")},
			Connection: ConnectionConfig{DialTimeoutSec: 1},
		},
		ctx:          ctx,
		listeners:    map[string]net.Listener{},
		virtualAddrs: map[string]string{},
		forwards:     map[string]*forwardRuntime{},
		closed:       make(chan struct{}),
	}
	fwd := ForwardConfig{
		Name:         "virtual-ssh",
		Direction:    DirectionVirtual,
		ListenAddr:   ":22",
		ServerTarget: "10.20.30.40:22",
	}
	if got := forwardDirection(fwd); got != DirectionClientToServer {
		t.Fatalf("protocol direction = %q, want %q", got, DirectionClientToServer)
	}
	client.initForward(fwd.Name, fwd, ForwardStarting, "")
	prepared, err := client.prepareVirtualForwardListener(fwd.Name, fwd)
	if err != nil {
		t.Fatal(err)
	}
	client.activateVirtualForward(prepared)
	defer client.Close()

	entryAddr := client.ForwardAddr(fwd.Name)
	if entryAddr != "192.0.2.22:22" {
		t.Fatalf("virtual entry address = %q", entryAddr)
	}
	status, ok := forwardStatusForTest(client.Stats(), fwd.Name)
	if !ok {
		t.Fatal("virtual forward status is missing")
	}
	if status.VirtualAddr != entryAddr || status.VirtualIP != "192.0.2.22" || status.Direction != DirectionVirtual {
		t.Fatalf("virtual status = %+v, entry = %q", status, entryAddr)
	}
	host, port, err := net.SplitHostPort(status.ListenAddr)
	if err != nil || host != "127.0.0.1" || port == "0" {
		t.Fatalf("random local redirect address = %q, error = %v", status.ListenAddr, err)
	}
}

func TestStartVirtualForwardsUseSharedRedirectSession(t *testing.T) {
	originalStart := startVirtualRedirectSessionFn
	t.Cleanup(func() { startVirtualRedirectSessionFn = originalStart })

	var session *testVirtualRedirectSession
	var capturedRules []virtualRedirectRule
	startCalls := 0
	startVirtualRedirectSessionFn = func(_ context.Context, rules []virtualRedirectRule) (virtualRedirectSession, error) {
		startCalls++
		capturedRules = append([]virtualRedirectRule(nil), rules...)
		session = &testVirtualRedirectSession{done: make(chan struct{})}
		return session, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         writeTestServerCertificate(t, "192.0.2.22"),
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{
			{Name: "virtual-ssh", Direction: DirectionVirtual, ListenAddr: ":22", ServerTarget: "10.20.30.40:22"},
			{Name: "virtual-web", Direction: DirectionVirtual, ListenAddr: ":443", ServerTarget: "10.20.30.41:8443"},
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	if startCalls != 1 {
		t.Fatalf("redirect session start calls = %d, want 1", startCalls)
	}
	if len(capturedRules) != 2 {
		t.Fatalf("redirect session rules = %+v, want 2 rules", capturedRules)
	}
	if capturedRules[0].VirtualIP != "192.0.2.22" || capturedRules[0].VirtualPort != 22 || capturedRules[0].LocalPort == 0 {
		t.Fatalf("first redirect rule = %+v", capturedRules[0])
	}
	if capturedRules[1].VirtualIP != "192.0.2.22" || capturedRules[1].VirtualPort != 443 || capturedRules[1].LocalPort == 0 {
		t.Fatalf("second redirect rule = %+v", capturedRules[1])
	}
	if capturedRules[0].LocalPort == capturedRules[1].LocalPort {
		t.Fatalf("virtual forwards share local redirect port %d", capturedRules[0].LocalPort)
	}
	if got := client.ForwardAddr("virtual-ssh"); got != "192.0.2.22:22" {
		t.Fatalf("virtual ssh address = %q", got)
	}
	if got := client.ForwardAddr("virtual-web"); got != "192.0.2.22:443" {
		t.Fatalf("virtual web address = %q", got)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.done:
	default:
		t.Fatal("shared redirect session was not closed with the client")
	}
}

func TestSharedVirtualRedirectSessionFailureStopsAllVirtualRules(t *testing.T) {
	originalStart := startVirtualRedirectSessionFn
	t.Cleanup(func() { startVirtualRedirectSessionFn = originalStart })

	var session *testVirtualRedirectSession
	startVirtualRedirectSessionFn = func(context.Context, []virtualRedirectRule) (virtualRedirectSession, error) {
		session = &testVirtualRedirectSession{done: make(chan struct{})}
		return session, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         writeTestServerCertificate(t, "192.0.2.22"),
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{
			{Name: "local-web", ListenAddr: "127.0.0.1:0", ServerTarget: "10.20.30.40:80"},
			{Name: "virtual-ssh", Direction: DirectionVirtual, ListenAddr: ":22", ServerTarget: "10.20.30.40:22"},
			{Name: "virtual-web", Direction: DirectionVirtual, ListenAddr: ":443", ServerTarget: "10.20.30.41:8443"},
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if session == nil {
		t.Fatal("shared redirect session was not started")
	}

	session.fail(errors.New("WinDivert receive failed"))
	deadline := time.Now().Add(2 * time.Second)
	for (forwardStateForTest(client.Stats(), "virtual-ssh") != ForwardListenFailed || forwardStateForTest(client.Stats(), "virtual-web") != ForwardListenFailed) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	for _, name := range []string{"virtual-ssh", "virtual-web"} {
		if got := forwardStateForTest(client.Stats(), name); got != ForwardListenFailed {
			t.Fatalf("failed virtual rule %s state = %q, want %q", name, got, ForwardListenFailed)
		}
		if got := client.ForwardAddr(name); got != "" {
			t.Fatalf("failed virtual rule %s address = %q, want empty", name, got)
		}
	}
	if got := forwardStateForTest(client.Stats(), "local-web"); got != ForwardListening {
		t.Fatalf("ordinary forward state = %q, want %q", got, ForwardListening)
	}
	if got := client.ForwardAddr("local-web"); got == "" {
		t.Fatal("ordinary forward listener was removed")
	}
	select {
	case <-client.Done():
		t.Fatal("client stopped after one virtual redirect session failed")
	default:
	}
}

func TestVirtualAuthorizationCancellationDisablesAllVirtualRules(t *testing.T) {
	originalStart := startVirtualRedirectSessionFn
	t.Cleanup(func() { startVirtualRedirectSessionFn = originalStart })

	startCalls := 0
	startVirtualRedirectSessionFn = func(context.Context, []virtualRedirectRule) (virtualRedirectSession, error) {
		startCalls++
		return nil, errors.New("administrator authorization for virtual forwarding was cancelled")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         writeTestServerCertificate(t, "192.0.2.22"),
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{
			{Name: "virtual-ssh", Direction: DirectionVirtual, ListenAddr: ":22", ServerTarget: "10.20.30.40:22"},
			{Name: "virtual-web", Direction: DirectionVirtual, ListenAddr: ":443", ServerTarget: "10.20.30.41:8443"},
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if startCalls != 1 {
		t.Fatalf("redirect starts = %d, want 1", startCalls)
	}
	for _, name := range []string{"virtual-ssh", "virtual-web"} {
		if got := forwardStateForTest(client.Stats(), name); got != ForwardListenFailed {
			t.Fatalf("cancelled virtual rule %s state = %q, want %q", name, got, ForwardListenFailed)
		}
		if got := client.ForwardAddr(name); got != "" {
			t.Fatalf("cancelled virtual rule %s address = %q, want empty", name, got)
		}
	}
}

func TestRestoreCheckedVirtualForwardDoesNotCreateOrdinaryListener(t *testing.T) {
	client := &Client{
		listeners:              map[string]net.Listener{},
		virtualAddrs:           map[string]string{},
		virtualRedirectMembers: map[string]struct{}{},
		forwards:               map[string]*forwardRuntime{},
		closed:                 make(chan struct{}),
	}
	fwd := ForwardConfig{
		Name:         "virtual-ssh",
		Direction:    DirectionVirtual,
		ListenAddr:   "127.0.0.1:0",
		ServerTarget: "10.20.30.40:22",
	}
	client.initForward(fwd.Name, fwd, ForwardListenFailed, "")

	err := client.restoreCheckedForward(fwd.Name, fwd)
	if err == nil || !strings.Contains(err.Error(), "virtual redirect is inactive") {
		t.Fatalf("restoreCheckedForward() error = %v, want inactive virtual redirect", err)
	}
	if got := client.ForwardAddr(fwd.Name); got != "" {
		t.Fatalf("inactive virtual rule created ordinary listener %q", got)
	}
}

func TestStopVirtualForwardClosesRedirectSessionAfterLastRule(t *testing.T) {
	sshListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	webListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = sshListener.Close()
		t.Fatal(err)
	}
	session := &testVirtualRedirectSession{done: make(chan struct{})}
	client := &Client{
		listeners: map[string]net.Listener{
			"virtual-ssh": sshListener,
			"virtual-web": webListener,
		},
		virtualAddrs: map[string]string{
			"virtual-ssh": "192.0.2.22:22",
			"virtual-web": "192.0.2.22:443",
		},
		virtualRedirectSession: session,
		virtualRedirectMembers: map[string]struct{}{
			"virtual-ssh": {},
			"virtual-web": {},
		},
	}

	client.stopForwardListener("virtual-ssh")

	select {
	case <-session.done:
		t.Fatal("shared redirect session stopped while another virtual rule remained")
	default:
	}
	if got := client.ForwardAddr("virtual-ssh"); got != "" {
		t.Fatalf("stopped virtual rule address = %q, want empty", got)
	}
	if got := client.ForwardAddr("virtual-web"); got != "192.0.2.22:443" {
		t.Fatalf("remaining virtual rule address = %q", got)
	}

	client.stopForwardListener("virtual-web")

	select {
	case <-session.done:
	default:
		t.Fatal("shared redirect session remained active after the last virtual rule stopped")
	}
}

func TestStartKeepsClientConnectedWhenVirtualAuthorizationIsCancelled(t *testing.T) {
	originalStart := startVirtualRedirectSessionFn
	t.Cleanup(func() { startVirtualRedirectSessionFn = originalStart })

	startVirtualRedirectSessionFn = func(context.Context, []virtualRedirectRule) (virtualRedirectSession, error) {
		return nil, errors.New("administrator authorization for virtual forwarding was cancelled")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         writeTestServerCertificate(t, "192.0.2.22"),
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{{
			Name:         "virtual-ssh",
			Direction:    DirectionVirtual,
			ListenAddr:   ":22",
			ServerTarget: "10.20.30.40:22",
		}},
	}, t.Logf)
	if err != nil {
		t.Fatalf("Start() error = %v, want connected client", err)
	}
	defer client.Close()

	status, ok := forwardStatusForTest(client.Stats(), "virtual-ssh")
	if !ok {
		t.Fatal("virtual forward status is missing")
	}
	if status.State != ForwardListenFailed {
		t.Fatalf("virtual forward state = %q, want %q", status.State, ForwardListenFailed)
	}
	if status.Message != "IP 接管需要管理员授权，本次授权已取消" {
		t.Fatalf("virtual forward message = %q", status.Message)
	}
	if got := client.ForwardAddr("virtual-ssh"); got != "" {
		t.Fatalf("cancelled virtual forward address = %q, want empty", got)
	}
	select {
	case <-client.Done():
		t.Fatal("client stopped after virtual authorization was cancelled")
	default:
	}
}

func TestVirtualAuthorizationCancellationDoesNotAffectOtherForward(t *testing.T) {
	originalStart := startVirtualRedirectSessionFn
	t.Cleanup(func() { startVirtualRedirectSessionFn = originalStart })

	startVirtualRedirectSessionFn = func(context.Context, []virtualRedirectRule) (virtualRedirectSession, error) {
		return nil, errors.New("administrator authorization for virtual forwarding was cancelled")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         writeTestServerCertificate(t, "192.0.2.22"),
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{
			{Name: "local-web", ListenAddr: "127.0.0.1:0", ServerTarget: "10.20.30.40:80"},
			{Name: "virtual-ssh", Direction: DirectionVirtual, ListenAddr: ":22", ServerTarget: "10.20.30.40:22"},
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	stats := client.Stats()
	if got := forwardStateForTest(stats, "local-web"); got != ForwardListening {
		t.Fatalf("ordinary forward state = %q, want %q", got, ForwardListening)
	}
	if got := forwardStateForTest(stats, "virtual-ssh"); got != ForwardListenFailed {
		t.Fatalf("virtual forward state = %q, want %q", got, ForwardListenFailed)
	}
	if got := client.ForwardAddr("local-web"); got == "" {
		t.Fatal("ordinary forward listener was removed")
	}
}

func TestVirtualBatchFailureReleasesPreparedListeners(t *testing.T) {
	for _, test := range []struct {
		name   string
		detach bool
		want   string
	}{
		{name: "missing helper", want: "virtual redirect helper did not start"},
		{name: "listener stopped during elevation", detach: true, want: "virtual forward listeners are no longer active"},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalStart := startVirtualRedirectSessionFn
			t.Cleanup(func() { startVirtualRedirectSessionFn = originalStart })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &Client{
				cfg: Config{
					ServerAddr: "127.0.0.1:1",
					TLS:        TLSConfig{CACertFile: writeTestServerCertificate(t, "192.0.2.22")},
				},
				ctx:          ctx,
				listeners:    map[string]net.Listener{},
				virtualAddrs: map[string]string{},
				forwards:     map[string]*forwardRuntime{},
				closed:       make(chan struct{}),
			}
			defer client.Close()
			fwd := ForwardConfig{Name: "virtual-ssh", Direction: DirectionVirtual, ListenAddr: ":22", ServerTarget: "10.20.30.40:22"}
			client.initForward(fwd.Name, fwd, ForwardStarting, "")
			prepared, err := client.prepareVirtualForwardListener(fwd.Name, fwd)
			if err != nil {
				t.Fatal(err)
			}
			bridgeAddr := prepared.listener.Addr().String()
			session := &testVirtualRedirectSession{done: make(chan struct{})}
			startVirtualRedirectSessionFn = func(context.Context, []virtualRedirectRule) (virtualRedirectSession, error) {
				if test.detach {
					client.stopForwardListener(fwd.Name)
					return session, nil
				}
				return nil, nil
			}
			if err := client.startVirtualForwardBatch([]preparedVirtualForward{prepared}); err == nil || err.Error() != test.want {
				t.Fatalf("startVirtualForwardBatch() error = %v, want %q", err, test.want)
			}
			if client.virtualForwardActive(fwd.Name) || client.ForwardAddr(fwd.Name) != "" {
				t.Fatal("failed batch retained a virtual endpoint")
			}
			if got := forwardStateForTest(client.Stats(), fwd.Name); got != ForwardListenFailed {
				t.Fatalf("forward state = %q, want %q", got, ForwardListenFailed)
			}
			probe, err := net.Listen("tcp4", bridgeAddr)
			if err != nil {
				t.Fatalf("failed batch retained bridge port: %v", err)
			}
			_ = probe.Close()
			if test.detach {
				select {
				case <-session.Done():
				default:
					t.Fatal("unattached helper session was not closed")
				}
			}
		})
	}
}

func TestStartStillFailsForNonAuthorizationVirtualError(t *testing.T) {
	originalStart := startVirtualRedirectSessionFn
	t.Cleanup(func() { startVirtualRedirectSessionFn = originalStart })

	startVirtualRedirectSessionFn = func(context.Context, []virtualRedirectRule) (virtualRedirectSession, error) {
		return nil, errors.New("WinDivert.dll is missing; repair the standard 64-bit Windows client installation")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         writeTestServerCertificate(t, "192.0.2.22"),
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{{
			Name:         "virtual-ssh",
			Direction:    DirectionVirtual,
			ListenAddr:   ":22",
			ServerTarget: "10.20.30.40:22",
		}},
	}, t.Logf)
	if err == nil {
		_ = client.Close()
		t.Fatal("Start() succeeded for missing WinDivert component")
	}
	if !strings.Contains(err.Error(), "WinDivert.dll is missing") {
		t.Fatalf("Start() error = %v, want WinDivert component error", err)
	}
}
