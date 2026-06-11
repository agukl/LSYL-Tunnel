package tunnel

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lsyltunnel/src/client/tunnel"
	"lsyltunnel/src/internal/protocol"
)

func TestAccountTunnelForwarding(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echoLn := startEchoServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Forwards: []ForwardConfig{{
			Name:         "echo",
			Direction:    DirectionClientToServer,
			ServerTarget: echoLn.Addr().String(),
			AllowedUsers: []string{"alice"},
		}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	client, err := tunnel.Start(ctx, tunnel.Config{
		ServerAddr: server.Addr(),
		Username:   "alice",
		Password:   "secret",
		ClientID:   "test-client",
		TLS:        tunnel.TLSConfig{CACertFile: certFile, ServerName: "localhost", MinVersion: "1.2"},
		Connection: tunnel.ConnectionConfig{DialTimeoutSec: 3},
		Forwards: []tunnel.ForwardConfig{{
			Name:         "echo",
			ListenAddr:   "127.0.0.1:0",
			ServerTarget: echoLn.Addr().String(),
		}},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	conn, err := net.DialTimeout("tcp", client.ForwardAddr("echo"), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("hello-lsyl-tunnel")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("hello-lsyl-tunnel"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "hello-lsyl-tunnel" {
		t.Fatalf("unexpected echo: %q", string(buf))
	}
	if server.totalStreams.Load() != 1 {
		t.Fatalf("expected one stream, got %d", server.totalStreams.Load())
	}
}

func TestServerLimitsConcurrentStreamsPerUser(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echoLn := startEchoServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Forwards: []ForwardConfig{{
			Name:         "echo",
			Direction:    DirectionClientToServer,
			ServerTarget: echoLn.Addr().String(),
			AllowedUsers: []string{"alice"},
		}},
		Security: SecurityConfig{
			HandshakeTimeoutSec:         3,
			DialTimeoutSec:              3,
			MaxHandshakeBytes:           32768,
			MaxConcurrentStreamsPerUser: 1,
			AuthFailThreshold:           3,
			AuthFailWindowSec:           60,
			AuthFailBlockSec:            60,
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	first, resp := openTargetConnForTest(t, server.Addr(), certFile, "alice", "secret", "echo", echoLn.Addr().String())
	defer first.Close()
	if !resp.OK {
		t.Fatalf("first open failed: %+v", resp)
	}
	waitForUserStreamActive(t, server, "alice", 1, time.Second)

	second := openTargetForTest(t, server.Addr(), certFile, "alice", "secret", "echo", echoLn.Addr().String())
	if second.OK || second.Code != "user_stream_limit" {
		t.Fatalf("second open response = %+v, want user_stream_limit", second)
	}
	if got := server.userStreamLimitRejected.Load(); got != 1 {
		t.Fatalf("userStreamLimitRejected = %d, want 1", got)
	}
}

func TestAccountTunnelReverseForwarding(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echoLn := startEchoServer(t)
	reverseAddr := freeTCPAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Forwards: []ForwardConfig{{
			Name:         "reverse-echo",
			Direction:    DirectionServerToClient,
			ListenAddr:   reverseAddr,
			AllowedUsers: []string{"alice"},
		}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	client, err := tunnel.Start(ctx, tunnel.Config{
		ServerAddr: server.Addr(),
		Username:   "alice",
		Password:   "secret",
		ClientID:   "reverse-client",
		TLS:        tunnel.TLSConfig{CACertFile: certFile, ServerName: "localhost", MinVersion: "1.2"},
		Connection: tunnel.ConnectionConfig{DialTimeoutSec: 3},
		Forwards: []tunnel.ForwardConfig{{
			Name:         "reverse-echo",
			Direction:    tunnel.DirectionServerToClient,
			ListenAddr:   reverseAddr,
			ServerTarget: echoLn.Addr().String(),
		}},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	echoTCPWithRetry(t, reverseAddr, []byte("hello-reverse"), 5*time.Second)
	if server.totalStreams.Load() != 1 {
		t.Fatalf("expected one reverse stream, got %d", server.totalStreams.Load())
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	assertTCPListenFails(t, reverseAddr)
}

func TestReverseControlHeartbeatReleasesStaleActivation(t *testing.T) {
	oldInterval := reverseControlHeartbeatInterval
	oldReadTimeout := reverseControlReadTimeout
	oldWriteTimeout := reverseControlWriteTimeout
	reverseControlHeartbeatInterval = 20 * time.Millisecond
	reverseControlReadTimeout = 80 * time.Millisecond
	reverseControlWriteTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		reverseControlHeartbeatInterval = oldInterval
		reverseControlReadTimeout = oldReadTimeout
		reverseControlWriteTimeout = oldWriteTimeout
	})

	certFile, keyFile := writeTestCertificate(t)
	echoLn := startEchoServer(t)
	reverseAddr := freeTCPAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Forwards: []ForwardConfig{{
			Name:         "reverse-echo",
			Direction:    DirectionServerToClient,
			ListenAddr:   reverseAddr,
			AllowedUsers: []string{"alice"},
		}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 1, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	stale := activateReverseControlForTest(t, server.Addr(), certFile, "stale-client", reverseAddr, echoLn.Addr().String())
	defer stale.Close()

	deadline := time.Now().Add(2 * time.Second)
	for {
		fresh := dialTLSForTest(t, server.Addr(), certFile)
		err := protocol.WriteJSON(fresh, protocol.OpenRequest{
			Type:       "reverse_listen",
			Username:   "alice",
			Password:   "secret",
			ClientID:   "fresh-client",
			ListenAddr: reverseAddr,
			Target:     echoLn.Addr().String(),
		})
		if err != nil {
			_ = fresh.Close()
			t.Fatal(err)
		}
		var resp protocol.OpenResponse
		err = protocol.ReadJSON(fresh, &resp, protocol.DefaultMaxHandshakeBytes)
		if err != nil {
			_ = fresh.Close()
			t.Fatal(err)
		}
		if resp.OK {
			_ = fresh.Close()
			return
		}
		_ = fresh.Close()
		if time.Now().After(deadline) {
			t.Fatalf("fresh reverse activation was not released from stale client, last response: %+v", resp)
		}
		time.Sleep(30 * time.Millisecond)
	}
}

func TestServerAllowsNonLocalReverseListenDuringValidation(t *testing.T) {
	err := ValidateConfig(Config{
		TLS: TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Forwards: []ForwardConfig{{
			Name:       "public-reverse",
			Direction:  DirectionServerToClient,
			ListenAddr: "0.0.0.0:18080",
		}},
	})
	if err != nil {
		t.Fatalf("ValidateConfig returned error: %v", err)
	}
}

func TestServerAllowsForwardAllowedUsersDuringValidation(t *testing.T) {
	err := ValidateConfig(Config{
		TLS: TLSConfig{CertFile: "server.crt", KeyFile: "server.key"},
		Auth: AuthConfig{Users: []UserConfig{
			{Username: "alice", PasswordHash: "plain:secret"},
			{Username: "bob", PasswordHash: "plain:secret"},
		}},
		Forwards: []ForwardConfig{{
			Name:         "reverse-web",
			Direction:    DirectionServerToClient,
			ListenAddr:   "127.0.0.1:18080",
			AllowedUsers: []string{"alice"},
		}},
	})
	if err != nil {
		t.Fatalf("ValidateConfig returned error: %v", err)
	}
}

func TestServerStartsWithUnreachableConfiguredForwardTarget(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	unreachable := freeTCPAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Forwards: []ForwardConfig{{
			Name:         "unreachable",
			Direction:    DirectionClientToServer,
			ServerTarget: unreachable,
			AllowedUsers: []string{"alice"},
		}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 1, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer server.Close()
}

func TestServerRejectsUnconfiguredReverseActivation(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echoLn := startEchoServer(t)
	reverseAddr := freeTCPAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	conn := dialTLSForTest(t, server.Addr(), certFile)
	defer conn.Close()
	if err := protocol.WriteJSON(conn, protocol.OpenRequest{
		Type:       "reverse_listen",
		Username:   "alice",
		Password:   "secret",
		ClientID:   "rogue-reverse-client",
		ListenAddr: reverseAddr,
		Target:     echoLn.Addr().String(),
	}); err != nil {
		t.Fatal(err)
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Code != "reverse_failed" {
		t.Fatalf("expected unconfigured reverse activation to fail, got %+v", resp)
	}
}

func TestAccountTunnelRejectsWrongPassword(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echoLn := startEchoServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	conn := dialTLSForTest(t, server.Addr(), certFile)
	defer conn.Close()
	if err := protocol.WriteJSON(conn, protocol.OpenRequest{Type: "open", Username: "alice", Password: "wrong", Target: echoLn.Addr().String()}); err != nil {
		t.Fatal(err)
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Code != "auth_failed" {
		t.Fatalf("expected auth_failed, got %+v", resp)
	}
}

func TestAccountTunnelLoginProbeDoesNotDialTarget(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 1, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	err = tunnel.CheckLogin(ctx, tunnel.Config{
		ServerAddr: server.Addr(),
		Username:   "alice",
		Password:   "secret",
		ClientID:   "probe-client",
		TLS:        tunnel.TLSConfig{CACertFile: certFile, ServerName: "localhost", MinVersion: "1.2"},
		Connection: tunnel.ConnectionConfig{DialTimeoutSec: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if server.totalStreams.Load() != 0 {
		t.Fatalf("login probe should not open target streams, got %d", server.totalStreams.Load())
	}
}

func TestAccountTunnelHealthProbeDoesNotDialTargetOrCountAuthOK(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 1, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	resp, err := tunnel.CheckHealthResponse(ctx, tunnel.Config{
		ServerAddr: server.Addr(),
		Username:   "alice",
		Password:   "secret",
		ClientID:   "health-client",
		TLS:        tunnel.TLSConfig{CACertFile: certFile, ServerName: "localhost", MinVersion: "1.2"},
		Connection: tunnel.ConnectionConfig{DialTimeoutSec: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Code != "ok" {
		t.Fatalf("unexpected health response: %+v", resp)
	}
	if server.totalStreams.Load() != 0 {
		t.Fatalf("health probe should not open target streams, got %d", server.totalStreams.Load())
	}
	if server.authOK.Load() != 0 {
		t.Fatalf("health probe should not increment auth_ok, got %d", server.authOK.Load())
	}
}

func TestAccountTunnelUsesSealedCredential(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echoLn := startEchoServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Forwards: []ForwardConfig{{
			Name:         "echo",
			Direction:    DirectionClientToServer,
			ServerTarget: echoLn.Addr().String(),
			AllowedUsers: []string{"alice"},
		}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
		CredentialSeal: CredentialSealConfig{Keys: []CredentialSealKeyConfig{{
			KeyID:          "test-login-key",
			PrivateKeyFile: filepath.Join(dir, "login.key"),
			PublicKeyFile:  filepath.Join(dir, "login.pub"),
			ExpiresAt:      time.Now().Add(time.Hour).Format(time.RFC3339),
			Active:         true,
		}}},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	loginCfg := tunnel.Config{
		ServerAddr: server.Addr(),
		Username:   "alice",
		Password:   "secret",
		ClientID:   "sealed-client",
		TLS:        tunnel.TLSConfig{CACertFile: certFile, ServerName: "localhost", MinVersion: "1.2"},
		Connection: tunnel.ConnectionConfig{DialTimeoutSec: 3},
	}
	resp, err := tunnel.CheckLoginResponse(ctx, loginCfg)
	if err != nil {
		t.Fatal(err)
	}
	if resp.CredentialKey == nil {
		t.Fatal("server did not return credential public key")
	}
	sealed, err := tunnel.SealSavedCredential(*resp.CredentialKey, loginCfg, "secret")
	if err != nil {
		t.Fatal(err)
	}
	client, err := tunnel.Start(ctx, tunnel.Config{
		ServerAddr:      server.Addr(),
		Username:        "alice",
		SavedCredential: sealed,
		ClientID:        "sealed-client",
		TLS:             tunnel.TLSConfig{CACertFile: certFile, ServerName: "localhost", MinVersion: "1.2"},
		Connection:      tunnel.ConnectionConfig{DialTimeoutSec: 3},
		Forwards: []tunnel.ForwardConfig{{
			Name:         "echo",
			ListenAddr:   "127.0.0.1:0",
			ServerTarget: echoLn.Addr().String(),
		}},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	conn, err := net.DialTimeout("tcp", client.ForwardAddr("echo"), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("sealed-hello")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("sealed-hello"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "sealed-hello" {
		t.Fatalf("unexpected echo: %q", string(buf))
	}
}

func TestAccountTunnelRejectsDeniedTarget(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echoLn := startEchoServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{{
			Username:     "alice",
			PasswordHash: "plain:secret",
		}}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	conn := dialTLSForTest(t, server.Addr(), certFile)
	defer conn.Close()
	if err := protocol.WriteJSON(conn, protocol.OpenRequest{Type: "open", Username: "alice", Password: "secret", Target: echoLn.Addr().String()}); err != nil {
		t.Fatal(err)
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Code != "target_denied" {
		t.Fatalf("expected target_denied, got %+v", resp)
	}
}

func TestAccountTunnelSeparatesAllowedUsersByForwardTarget(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	aliceTarget := startEchoServer(t)
	bobTarget := startEchoServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{
			{Username: "alice", PasswordHash: "plain:alice-secret"},
			{Username: "bob", PasswordHash: "plain:bob-secret"},
		}},
		Forwards: []ForwardConfig{
			{
				Name:         "alice-port",
				Direction:    DirectionClientToServer,
				ServerTarget: aliceTarget.Addr().String(),
				AllowedUsers: []string{"alice"},
			},
			{
				Name:         "bob-port",
				Direction:    DirectionClientToServer,
				ServerTarget: bobTarget.Addr().String(),
				AllowedUsers: []string{"bob"},
			},
		},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	aliceOwn := openTargetForTest(t, server.Addr(), certFile, "alice", "alice-secret", "alice-port", aliceTarget.Addr().String())
	if !aliceOwn.OK {
		t.Fatalf("alice own target rejected: %+v", aliceOwn)
	}
	aliceDenied := openTargetForTest(t, server.Addr(), certFile, "alice", "alice-secret", "bob-port", bobTarget.Addr().String())
	if aliceDenied.OK || aliceDenied.Code != "target_denied" {
		t.Fatalf("expected alice to be denied from bob target, got %+v", aliceDenied)
	}
	bobOwn := openTargetForTest(t, server.Addr(), certFile, "bob", "bob-secret", "bob-port", bobTarget.Addr().String())
	if !bobOwn.OK {
		t.Fatalf("bob own target rejected: %+v", bobOwn)
	}
}

func TestForwardCheckChecksPolicyWithoutOpeningTarget(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	unreachableTarget := freeTCPAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{
			{Username: "alice", PasswordHash: "plain:alice-secret"},
			{Username: "bob", PasswordHash: "plain:bob-secret"},
		}},
		Forwards: []ForwardConfig{{
			Name:         "alice-port",
			Direction:    DirectionClientToServer,
			ServerTarget: unreachableTarget,
			AllowedUsers: []string{"alice"},
		}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 1, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	aliceCheck := checkForwardForTest(t, server.Addr(), certFile, "alice", "alice-secret", "alice-port", DirectionClientToServer, "", unreachableTarget)
	if !aliceCheck.OK {
		t.Fatalf("alice forward check rejected: %+v", aliceCheck)
	}
	bobCheck := checkForwardForTest(t, server.Addr(), certFile, "bob", "bob-secret", "alice-port", DirectionClientToServer, "", unreachableTarget)
	if bobCheck.OK || bobCheck.Code != "target_denied" {
		t.Fatalf("expected bob forward check to be denied, got %+v", bobCheck)
	}
	if server.totalStreams.Load() != 0 {
		t.Fatalf("forward_check should not open target streams, got %d", server.totalStreams.Load())
	}
	if server.dialFailed.Load() != 0 {
		t.Fatalf("forward_check should not dial unreachable targets, got dial_failed=%d", server.dialFailed.Load())
	}
}

func TestClientStopsUnauthorizedLocalForwardAfterPermanentReject(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	aliceTarget := startEchoServer(t)
	bobTarget := startEchoServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{
			{Username: "alice", PasswordHash: "plain:alice-secret"},
			{Username: "bob", PasswordHash: "plain:bob-secret"},
		}},
		Forwards: []ForwardConfig{
			{
				Name:         "alice-port",
				Direction:    DirectionClientToServer,
				ServerTarget: aliceTarget.Addr().String(),
				AllowedUsers: []string{"alice"},
			},
			{
				Name:         "bob-port",
				Direction:    DirectionClientToServer,
				ServerTarget: bobTarget.Addr().String(),
				AllowedUsers: []string{"bob"},
			},
		},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	client, err := tunnel.Start(ctx, tunnel.Config{
		ServerAddr: server.Addr(),
		Username:   "alice",
		Password:   "alice-secret",
		ClientID:   "alice-client",
		TLS:        tunnel.TLSConfig{CACertFile: certFile, ServerName: "localhost", MinVersion: "1.2"},
		Connection: tunnel.ConnectionConfig{DialTimeoutSec: 3},
		Forwards: []tunnel.ForwardConfig{
			{
				Name:         "allowed",
				ListenAddr:   "127.0.0.1:0",
				ServerTarget: aliceTarget.Addr().String(),
			},
			{
				Name:         "denied",
				ListenAddr:   "127.0.0.1:0",
				ServerTarget: bobTarget.Addr().String(),
			},
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	deniedAddr := client.ForwardAddr("denied")
	if deniedAddr == "" {
		t.Fatal("denied forward should initially have a local listener")
	}
	conn, err := net.DialTimeout("tcp", deniedAddr, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	_, _ = conn.Write([]byte("probe"))
	_, _ = conn.Read(make([]byte, 1))
	_ = conn.Close()

	waitForClientForwardState(t, client, "denied", tunnel.ForwardRejected, 2*time.Second)
	waitForTCPDialFails(t, deniedAddr, 2*time.Second)
	echoTCPWithRetry(t, client.ForwardAddr("allowed"), []byte("allowed-still-works"), 3*time.Second)
}

func TestClientManualForwardCheckRejectsUnauthorizedLocalForward(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	aliceTarget := startEchoServer(t)
	bobTarget := startEchoServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth: AuthConfig{Users: []UserConfig{
			{Username: "alice", PasswordHash: "plain:alice-secret"},
			{Username: "bob", PasswordHash: "plain:bob-secret"},
		}},
		Forwards: []ForwardConfig{
			{
				Name:         "alice-port",
				Direction:    DirectionClientToServer,
				ServerTarget: aliceTarget.Addr().String(),
				AllowedUsers: []string{"alice"},
			},
			{
				Name:         "bob-port",
				Direction:    DirectionClientToServer,
				ServerTarget: bobTarget.Addr().String(),
				AllowedUsers: []string{"bob"},
			},
		},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	client, err := tunnel.Start(ctx, tunnel.Config{
		ServerAddr: server.Addr(),
		Username:   "alice",
		Password:   "alice-secret",
		ClientID:   "alice-client",
		TLS:        tunnel.TLSConfig{CACertFile: certFile, ServerName: "localhost", MinVersion: "1.2"},
		Connection: tunnel.ConnectionConfig{DialTimeoutSec: 3},
		Forwards: []tunnel.ForwardConfig{
			{
				Name:         "allowed",
				ListenAddr:   "127.0.0.1:0",
				ServerTarget: aliceTarget.Addr().String(),
			},
			{
				Name:         "denied",
				ListenAddr:   "127.0.0.1:0",
				ServerTarget: bobTarget.Addr().String(),
			},
		},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	deniedAddr := client.ForwardAddr("denied")
	if deniedAddr == "" {
		t.Fatal("denied forward should initially have a local listener")
	}
	summary := client.CheckForwardsNow(ctx)
	if summary.Checked != 2 || summary.Allowed != 1 || summary.Rejected != 1 || summary.Failed != 0 {
		t.Fatalf("unexpected manual forward check summary: %+v", summary)
	}
	waitForClientForwardState(t, client, "denied", tunnel.ForwardRejected, 2*time.Second)
	waitForTCPDialFails(t, deniedAddr, 2*time.Second)
	if server.totalStreams.Load() != 0 {
		t.Fatalf("manual forward check should not create target streams, got %d", server.totalStreams.Load())
	}
	echoTCPWithRetry(t, client.ForwardAddr("allowed"), []byte("manual-check-allowed"), 3*time.Second)
}

func startEchoServer(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()
	return ln
}

func openTargetForTest(t *testing.T, serverAddr, caFile, username, password, forwardName, target string) protocol.OpenResponse {
	t.Helper()
	conn := dialTLSForTest(t, serverAddr, caFile)
	defer conn.Close()
	if err := protocol.WriteJSON(conn, protocol.OpenRequest{
		Type:        "open",
		Username:    username,
		Password:    password,
		ForwardName: forwardName,
		Direction:   DirectionClientToServer,
		Target:      target,
	}); err != nil {
		t.Fatal(err)
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		t.Fatal(err)
	}
	return resp
}

func openTargetConnForTest(t *testing.T, serverAddr, caFile, username, password, forwardName, target string) (*tls.Conn, protocol.OpenResponse) {
	t.Helper()
	conn := dialTLSForTest(t, serverAddr, caFile)
	if err := protocol.WriteJSON(conn, protocol.OpenRequest{
		Type:        "open",
		Username:    username,
		Password:    password,
		ForwardName: forwardName,
		Direction:   DirectionClientToServer,
		Target:      target,
	}); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	return conn, resp
}

func checkForwardForTest(t *testing.T, serverAddr, caFile, username, password, forwardName, direction, listenAddr, target string) protocol.OpenResponse {
	t.Helper()
	conn := dialTLSForTest(t, serverAddr, caFile)
	defer conn.Close()
	if err := protocol.WriteJSON(conn, protocol.OpenRequest{
		Type:        "forward_check",
		Username:    username,
		Password:    password,
		ForwardName: forwardName,
		Direction:   direction,
		ListenAddr:  listenAddr,
		Target:      target,
	}); err != nil {
		t.Fatal(err)
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		t.Fatal(err)
	}
	return resp
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func mustTCPPort(t *testing.T, addr string) int {
	t.Helper()
	_, port, err := splitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func echoTCPWithRetry(t *testing.T, addr string, payload []byte, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
		if _, err = conn.Write(payload); err != nil {
			lastErr = err
			_ = conn.Close()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		buf := make([]byte, len(payload))
		if _, err = io.ReadFull(conn, buf); err != nil {
			lastErr = err
			_ = conn.Close()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		_ = conn.Close()
		if string(buf) != string(payload) {
			t.Fatalf("unexpected echo: %q", string(buf))
		}
		return
	}
	t.Fatalf("echo %s failed: %v", addr, lastErr)
}

func waitForClientForwardState(t *testing.T, client *tunnel.Client, name, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, item := range client.Stats().Items {
			if item.Name == name && item.State == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for client forward %s to become %s; stats: %+v", name, want, client.Stats())
}

func waitForUserStreamActive(t *testing.T, server *Server, username string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got := server.userStreams.snapshot().ActiveByUser[username]
		if got == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for user stream count %s=%d; snapshot: %+v", username, want, server.userStreams.snapshot())
}

func waitForTCPDialFails(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastConn net.Conn
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err != nil {
			return
		}
		lastConn = conn
		_ = conn.Close()
		time.Sleep(20 * time.Millisecond)
	}
	if lastConn != nil {
		_ = lastConn.Close()
	}
	t.Fatalf("expected TCP dial to %s to fail", addr)
}

func assertTCPListenFails(t *testing.T, addr string) {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		_ = ln.Close()
		t.Fatalf("expected %s to be reserved", addr)
	}
}

func activateReverseControlForTest(t *testing.T, serverAddr, caFile, clientID, listenAddr, target string) *tls.Conn {
	t.Helper()
	conn := dialTLSForTest(t, serverAddr, caFile)
	if err := protocol.WriteJSON(conn, protocol.OpenRequest{
		Type:       "reverse_listen",
		Username:   "alice",
		Password:   "secret",
		ClientID:   clientID,
		ListenAddr: listenAddr,
		Target:     target,
	}); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	var resp protocol.OpenResponse
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	if !resp.OK {
		_ = conn.Close()
		t.Fatalf("reverse control activation failed: %+v", resp)
	}
	return conn
}

func dialTLSForTest(t *testing.T, addr, caFile string) *tls.Conn {
	t.Helper()
	pemData, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		t.Fatal("append CA failed")
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp", addr, &tls.Config{
		RootCAs:    pool,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func writeTestCertificate(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
