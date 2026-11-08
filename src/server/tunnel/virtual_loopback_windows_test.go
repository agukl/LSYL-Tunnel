//go:build windows && !386

package tunnel

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	clienttunnel "lsyltunnel/src/client/tunnel"
)

func TestVirtualLoopbackTunnelForwarding(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echo := startEchoServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0",
		TLS:        TLSConfig{CertFile: certFile, KeyFile: keyFile},
		Auth:       AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
		Forwards:   []ForwardConfig{{Direction: DirectionClientToServer, ServerTarget: echo.Addr().String(), AllowedUsers: []string{"alice"}}},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reserved.Addr().String()
	reserved.Close()
	cfg := clienttunnel.Config{
		ServerAddr: server.Addr(), Username: "alice", Password: "secret",
		TLS:      clienttunnel.TLSConfig{CACertFile: certFile},
		Forwards: []clienttunnel.ForwardConfig{{Name: "echo", Direction: clienttunnel.DirectionVirtual, ListenAddr: addr, ServerTarget: echo.Addr().String()}},
	}
	if err := clienttunnel.ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	client, err := clienttunnel.Start(ctx, cfg, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if summary := client.CheckForwardsNow(ctx); summary.Allowed != 1 {
		t.Fatalf("forward authorization = %+v", summary)
	}
	conn, err := net.DialTimeout("tcp4", client.ForwardAddr("echo"), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	message := "loopback-through-authenticated-tunnel"
	if _, err := io.WriteString(conn, message); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, len(message))
	if _, err := io.ReadFull(conn, buffer); err != nil || string(buffer) != message {
		t.Fatalf("echo = %q, error = %v", buffer, err)
	}
}
