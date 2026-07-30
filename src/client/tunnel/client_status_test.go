package tunnel

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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

func TestClientCachesTLSConfig(t *testing.T) {
	certFile := writeTestServerCertificate(t, "192.0.2.22")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         certFile,
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{{
			Name:         "cached-tls",
			ListenAddr:   "127.0.0.1:0",
			ServerTarget: "127.0.0.1:80",
		}},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := os.Remove(certFile); err != nil {
		t.Fatal(err)
	}

	first, err := client.clientTLSConfig()
	if err != nil {
		t.Fatalf("cached TLS config read the removed CA file: %v", err)
	}
	second, err := client.clientTLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("client returned different TLS config instances")
	}
}

func TestStartRejectsInvalidTLSBeforeListening(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenAddr := reserved.Addr().String()
	_ = reserved.Close()
	certFile := writeTestServerCertificate(t, "192.0.2.22")
	if err := os.WriteFile(certFile, []byte("invalid certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	client, err := Start(context.Background(), Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS: TLSConfig{
			CACertFile:         certFile,
			InsecureSkipVerify: true,
		},
		Forwards: []ForwardConfig{{
			Name:         "invalid-tls",
			ListenAddr:   listenAddr,
			ServerTarget: "127.0.0.1:80",
		}},
	}, t.Logf)
	if err == nil {
		_ = client.Close()
		t.Fatal("Start accepted invalid TLS trust data")
	}
	probe, listenErr := net.Listen("tcp", listenAddr)
	if listenErr != nil {
		t.Fatalf("failed start retained local listener: %v", listenErr)
	}
	_ = probe.Close()
}

func TestStartVerifiedDelaysInitialHealthCheck(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var accepted atomic.Int32
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			accepted.Add(1)
			_ = conn.Close()
		}
	}()

	originalInterval := healthOKInterval
	healthOKInterval = 150 * time.Millisecond
	t.Cleanup(func() { healthOKInterval = originalInterval })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := StartVerified(ctx, Config{
		ServerAddr: listener.Addr().String(),
		Username:   "alice",
		Password:   "secret",
		TLS:        TLSConfig{InsecureSkipVerify: true},
		Forwards: []ForwardConfig{{
			Name:         "verified",
			ListenAddr:   "127.0.0.1:0",
			ServerTarget: "127.0.0.1:80",
		}},
	}, "2.0.1", t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	time.Sleep(50 * time.Millisecond)
	if got := accepted.Load(); got != 0 {
		t.Fatalf("verified startup made %d immediate health connections", got)
	}
	deadline := time.Now().Add(time.Second)
	for accepted.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := accepted.Load(); got == 0 {
		t.Fatal("verified startup did not resume periodic health checks")
	}
	if stats := client.Stats(); stats.ServerVersion != "2.0.1" {
		t.Fatalf("server version = %q, want 2.0.1", stats.ServerVersion)
	}

	_ = listener.Close()
	<-acceptDone
}

func TestStartSoftFailsOccupiedForward(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS:        TLSConfig{InsecureSkipVerify: true},
		Forwards: []ForwardConfig{
			{Name: "bad", ListenAddr: occupied.Addr().String(), ServerTarget: "127.0.0.1:80"},
			{Name: "good", ListenAddr: "127.0.0.1:0", ServerTarget: "127.0.0.1:80"},
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.ForwardAddr("good") == "" {
		t.Fatal("good forward should be listening")
	}
	stats := client.Stats()
	if got := forwardStateForTest(stats, "bad"); got != ForwardListenFailed {
		t.Fatalf("bad forward state = %q, want %q", got, ForwardListenFailed)
	}
	if got := forwardStateForTest(stats, "good"); got != ForwardListening {
		t.Fatalf("good forward state = %q, want %q", got, ForwardListening)
	}
}

func TestStartFailsWhenNoForwardUsable(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := Start(ctx, Config{
		ServerAddr: "127.0.0.1:1",
		Username:   "alice",
		Password:   "secret",
		TLS:        TLSConfig{InsecureSkipVerify: true},
		Forwards: []ForwardConfig{{
			Name:         "bad",
			ListenAddr:   occupied.Addr().String(),
			ServerTarget: "127.0.0.1:80",
		}},
	}, t.Logf)
	if err == nil {
		_ = client.Close()
		t.Fatal("expected no usable forward error")
	}
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

func TestForwardErrorMessageAndRetryPolicy(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		want      string
		permanent bool
	}{
		{
			name:      "permission denied",
			err:       errors.New("target_denied: user is not allowed to access this target"),
			want:      "当前账号没有访问该端口的权限，请联系管理员检查端口授权",
			permanent: true,
		},
		{
			name:      "temporary auth block",
			err:       errors.New("auth_blocked: too many login failures, temporarily blocked; try later"),
			want:      "登录失败次数过多，当前来源 IP 已被临时封禁，请稍后再试",
			permanent: false,
		},
		{
			name:      "account stream limit",
			err:       errors.New("user_stream_limit: too many concurrent streams for account"),
			want:      "当前账号并发连接数已达到上限，请关闭部分连接后重试",
			permanent: false,
		},
		{
			name:      "certificate",
			err:       errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
			want:      "服务端证书不受信任，请联系管理员重新下发证书",
			permanent: true,
		},
		{
			name:      "expired certificate",
			err:       errors.New("x509: certificate has expired or is not yet valid"),
			want:      "服务端证书已过期或尚未生效，请联系管理员重新下发有效证书",
			permanent: true,
		},
		{
			name:      "client version",
			err:       errors.New("client_version_unsupported: maximum client_version is 2.0.1"),
			want:      "客户端版本不在服务端允许范围内，请联系管理员确认两端版本",
			permanent: true,
		},
		{
			name:      "protocol version",
			err:       errors.New("protocol_version_unsupported: required protocol_version is 2"),
			want:      "客户端与服务端协议版本不一致，请联系管理员升级对应一端",
			permanent: true,
		},
		{
			name:      "network reset",
			err:       errors.New("wsarecv: An existing connection was forcibly closed by the remote host."),
			want:      "连接被断开，客户端会自动重试",
			permanent: false,
		},
		{
			name:      "server passive port unavailable",
			err:       errors.New("reverse_failed: server passive port is unavailable: listen reverse 127.0.0.1:18080: bind: Only one usage of each socket address is normally permitted."),
			want:      "服务端被动端口不可用，客户端会自动重试，请联系管理员检查服务端端口占用",
			permanent: false,
		},
		{
			name:      "reverse listener is not configured",
			err:       errors.New("reverse_failed: reverse listen address is not configured on server"),
			want:      "反向监听地址未在服务端配置，请联系管理员检查保留端口",
			permanent: true,
		},
		{
			name:      "reverse listener is not authorized",
			err:       errors.New("reverse_failed: user is not allowed to activate this reverse listen address"),
			want:      "当前账号无权使用该反向端口，请联系管理员检查端口授权",
			permanent: true,
		},
		{
			name:      "client target unavailable",
			err:       errors.New("connect client target 127.0.0.1:22: connectex: actively refused"),
			want:      "客户端目标服务不可达，请确认对应服务已启动并监听配置端口",
			permanent: false,
		},
		{
			name:      "server target unavailable",
			err:       errors.New("target_unreachable: target service is unreachable"),
			want:      "服务端无法访问目标服务，请联系管理员检查目标服务或防火墙",
			permanent: false,
		},
		{
			name:      "virtual domain name",
			err:       errors.New("virtual listen_addr does not support domain names; use an IPv4 address from the server certificate SAN"),
			want:      "虚拟入口不支持域名，请填写 :端口，或填写服务端证书中的 IPv4:端口",
			permanent: true,
		},
		{
			name:      "ambiguous virtual IP",
			err:       errors.New("server certificate has multiple usable IPv4 SANs; specify virtual listen_addr as IPv4:port"),
			want:      "服务端证书包含多个可用 IPv4，请将虚拟 listen_addr 填写为完整的 IPv4:端口",
			permanent: true,
		},
		{
			name:      "no usable virtual IP",
			err:       errors.New("server certificate has no usable non-local IPv4 SAN for automatic virtual forwarding"),
			want:      "服务端证书没有可用于虚拟入口的非本地 IPv4 SAN，请重新下发证书",
			permanent: true,
		},
		{
			name:      "virtual IP outside certificate SAN",
			err:       errors.New("virtual listen_addr 192.0.2.22:22 is not authorized by the server certificate IPv4 SAN"),
			want:      "填写的虚拟 IP 不在服务端证书 IPv4 SAN 中，请修正 listen_addr 或重新下发证书",
			permanent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForwardErrorMessage(tt.err); got != tt.want {
				t.Fatalf("ForwardErrorMessage() = %q, want %q", got, tt.want)
			}
			if got := IsPermanentForwardError(tt.err); got != tt.permanent {
				t.Fatalf("IsPermanentForwardError() = %v, want %v", got, tt.permanent)
			}
		})
	}
	if got := ReverseRetryDelay(errors.New("auth_blocked: too many login failures"), 1); got != 5*time.Minute {
		t.Fatalf("ReverseRetryDelay(auth_blocked) = %s, want 5m", got)
	}
	if got := ReverseRetryDelay(errors.New("user_stream_limit: too many concurrent streams for account"), 1); got != 2*time.Second {
		t.Fatalf("ReverseRetryDelay(user_stream_limit) = %s, want 2s", got)
	}
	if got := ReverseRetryDelay(errors.New("connect server failed"), 100); got != 30*time.Second {
		t.Fatalf("ReverseRetryDelay(max) = %s, want 30s", got)
	}
	if got := ReconnectDelay(1); got != 2*time.Second {
		t.Fatalf("ReconnectDelay(1) = %s, want 2s", got)
	}
	if got := ReconnectDelay(4); got != 16*time.Second {
		t.Fatalf("ReconnectDelay(4) = %s, want 16s", got)
	}
	if got := ReconnectDelay(10); got != 30*time.Second {
		t.Fatalf("ReconnectDelay(10) = %s, want 30s", got)
	}
}

func TestClassifyHealthErrorMessages(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantState string
		wantMsg   string
	}{
		{
			name:      "temporary auth block",
			err:       errors.New("auth_blocked: too many login failures, temporarily blocked; try later"),
			wantState: HealthAuthError,
			wantMsg:   "登录失败次数过多，当前来源 IP 已被临时封禁",
		},
		{
			name:      "missing server cert",
			err:       errors.New(`open C:\Program Files\LSYL Tunnel Client\cert\server.crt: The system cannot find the file specified.`),
			wantState: HealthAuthError,
			wantMsg:   "缺少服务端信任证书 server.crt，请联系管理员重新下发客户端安装包",
		},
		{
			name:      "invalid cert file",
			err:       errors.New(`no server TLS trust data found in C:\Program Files\LSYL Tunnel Client\cert\server.crt`),
			wantState: HealthAuthError,
			wantMsg:   "服务端信任证书无效，请联系管理员重新下发",
		},
		{
			name:      "name mismatch",
			err:       errors.New("x509: certificate is valid for localhost, not vpn.example.com"),
			wantState: HealthAuthError,
			wantMsg:   "服务端证书和当前地址不匹配，请检查服务端地址或重新下发证书",
		},
		{
			name:      "expired certificate",
			err:       errors.New("x509: certificate has expired or is not yet valid"),
			wantState: HealthAuthError,
			wantMsg:   "服务端证书已过期或尚未生效，请联系管理员重新下发有效证书",
		},
		{
			name:      "client version",
			err:       errors.New("client_version_unsupported: maximum client_version is 2.0.1"),
			wantState: HealthAuthError,
			wantMsg:   "客户端版本不在服务端允许范围内，请联系管理员确认两端版本",
		},
		{
			name:      "protocol version",
			err:       errors.New("protocol_version_unsupported: required protocol_version is 2"),
			wantState: HealthAuthError,
			wantMsg:   "客户端与服务端协议版本不一致，请联系管理员升级对应一端",
		},
		{
			name:      "invalid server address",
			err:       errors.New("dial tcp: address vpn.example.com: missing port in address"),
			wantState: HealthAuthError,
			wantMsg:   "服务端地址格式不正确，请填写 地址:端口",
		},
		{
			name:      "dns",
			err:       errors.New("lookup vpn.example.com: no such host"),
			wantState: HealthServerUnavailable,
			wantMsg:   "服务端地址无法解析，请检查域名或网络",
		},
		{
			name:      "refused",
			err:       errors.New("dial tcp 127.0.0.1:3443: connectex: No connection could be made because the target machine actively refused it."),
			wantState: HealthServerUnavailable,
			wantMsg:   "连接不上服务端，请检查服务端是否启动或地址端口是否正确",
		},
		{
			name:      "timeout",
			err:       errors.New("i/o timeout"),
			wantState: HealthServerUnavailable,
			wantMsg:   "连接超时，请检查网络或服务端防火墙",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, msg := classifyHealthError(tt.err)
			if state != tt.wantState || msg != tt.wantMsg {
				t.Fatalf("classifyHealthError() = (%q, %q), want (%q, %q)", state, msg, tt.wantState, tt.wantMsg)
			}
		})
	}
}

func TestSetForwardStateClearsRecoveredError(t *testing.T) {
	client := &Client{forwards: map[string]*forwardRuntime{}}
	fwd := ForwardConfig{Name: "web", ListenAddr: "127.0.0.1:8080", ServerTarget: "127.0.0.1:80"}
	client.initForward("web", fwd, ForwardStarting, "")
	client.recordForwardError("web", errors.New("connectex: actively refused"))
	client.setForwardState("web", ForwardListening, "本地端口监听中")

	status, ok := forwardStatusForTest(client.Stats(), "web")
	if !ok {
		t.Fatal("forward status is missing")
	}
	if status.LastError != "" {
		t.Fatalf("recovered forward LastError = %q, want empty", status.LastError)
	}
	if status.State != ForwardListening {
		t.Fatalf("recovered forward state = %q, want %q", status.State, ForwardListening)
	}
}

func TestFinalizeHealthStatusCancelsAfterReconnectLimit(t *testing.T) {
	client := &Client{}
	status := client.finalizeHealthStatus(HealthStatus{
		State:               HealthServerUnavailable,
		Message:             "连接不上服务端，请检查服务端是否启动或地址端口是否正确",
		ConsecutiveFailures: healthMaxReconnectFailures - 1,
	})
	if status.Terminal {
		t.Fatal("status below reconnect limit should not be terminal")
	}

	status = client.finalizeHealthStatus(HealthStatus{
		State:               HealthServerUnavailable,
		Message:             "连接不上服务端，请检查服务端是否启动或地址端口是否正确",
		ConsecutiveFailures: healthMaxReconnectFailures,
	})
	if !status.Terminal {
		t.Fatal("status at reconnect limit should be terminal")
	}
	if status.Message != "多次重连失败，已取消连接状态，请确认服务端恢复后重新连接" {
		t.Fatalf("terminal message = %q", status.Message)
	}
	if got := client.Stats().Health; !got.Terminal || got.Message != status.Message {
		t.Fatalf("stored health = %+v, want terminal message %q", got, status.Message)
	}
}

func forwardStateForTest(stats ClientStats, name string) string {
	item, _ := forwardStatusForTest(stats, name)
	return item.State
}

func forwardStatusForTest(stats ClientStats, name string) (ForwardStatus, bool) {
	for _, item := range stats.Items {
		if item.Name == name {
			return item, true
		}
	}
	return ForwardStatus{}, false
}
