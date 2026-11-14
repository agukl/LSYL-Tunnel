package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	clienttunnel "lsyltunnel/src/client/tunnel"
)

func TestIPv6TunnelAdvertisementUsesMainPort(t *testing.T) {
	main, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer main.Close()
	server := &Server{listener: main, ipv6TunnelListening: true}
	port := main.Addr().(*net.TCPAddr).Port
	got := server.advertisedBusinessEntriesForAddresses([]netip.Addr{
		netip.MustParseAddr("2001:db8::10"), netip.MustParseAddr("2001:db8::11"),
	})
	want := []string{"[2001:db8::10]:" + strconv.Itoa(port), "[2001:db8::11]:" + strconv.Itoa(port)}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("advertised entries = %v, want %v", got, want)
	}
	if got := server.advertisedBusinessEntriesForAddresses([]netip.Addr{netip.MustParseAddr("::1")}); len(got) != 0 {
		t.Fatalf("advertised local address: %v", got)
	}
	server.ipv6TunnelListening = false
	if got := server.advertisedBusinessEntriesForAddresses([]netip.Addr{netip.MustParseAddr("2001:db8::10")}); len(got) != 0 {
		t.Fatalf("advertised disabled entry: %v", got)
	}
	server.ipv6TunnelListening = true
	addresses := make([]netip.Addr, maxAdvertisedBusinessEntries+1)
	for i := range addresses {
		addresses[i] = netip.MustParseAddr(fmt.Sprintf("2001:db8::%x", i+1))
	}
	if got := server.advertisedBusinessEntriesForAddresses(addresses); len(got) != maxAdvertisedBusinessEntries {
		t.Fatalf("advertised %d addresses, want %d", len(got), maxAdvertisedBusinessEntries)
	}
}

func TestIPv6TunnelNeverExposesLoopbackListener(t *testing.T) {
	main, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer main.Close()
	if listener, listening := listenIPv6TunnelEntry(main, true, nil); listener != nil || listening {
		t.Fatal("loopback listener exposed on IPv6")
	}
}

func TestIPv6TunnelFallbackCarriesForward(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echo := startEchoServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr: "127.0.0.1:0", IPv6TunnelEnabled: true,
		TLS:      TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth:     AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
		Forwards: []ForwardConfig{{Name: "echo", Direction: DirectionClientToServer, ServerTarget: echo.Addr().String(), AllowedUsers: []string{"alice"}}},
		Security: SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
		Runtime:  RuntimeConfig{RequestLogFile: filepath.Join(t.TempDir(), "request.jsonl")},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	port := server.listener.Addr().(*net.TCPAddr).Port
	cfg := clienttunnel.Config{
		ServerAddr: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), Username: "alice", Password: "secret", ClientID: "test-client",
		TLS:        clienttunnel.TLSConfig{CACertFile: certFile, ServerName: "127.0.0.1", MinVersion: "1.2"},
		Connection: clienttunnel.ConnectionConfig{DialTimeoutSec: 3},
		Forwards:   []clienttunnel.ForwardConfig{{Name: "echo", ListenAddr: "127.0.0.1:0", ServerTarget: echo.Addr().String()}},
	}
	login, err := clienttunnel.CheckLoginResponse(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(login.BusinessEntries) != 0 {
		t.Fatalf("loopback server advertised IPv6 entries: %v", login.BusinessEntries)
	}
	client, err := clienttunnel.StartVerifiedWithEntries(ctx, cfg, login.ServerVersion, login.BusinessEntries, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if status := client.CheckHealthNow(ctx); status.State != clienttunnel.HealthOK {
		t.Fatalf("health = %+v", status)
	}
	if summary := client.CheckForwardsNow(ctx); summary.Allowed != 1 {
		t.Fatalf("forward check = %+v", summary)
	}
	conn, err := net.DialTimeout("tcp", client.ForwardAddr("echo"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("ipv4-fallback")); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, len("ipv4-fallback"))
	if _, err := io.ReadFull(conn, data); err != nil || string(data) != "ipv4-fallback" {
		t.Fatalf("echo = %q, %v", data, err)
	}
}

func TestOldBusinessPortsAreNotMigrated(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, "server.yaml", validServerConfigV1YAML("")+"business_ports: [50000, 50001]\n")
	template := writeTestConfig(t, dir, "template.yaml", validServerConfigV1YAML(""))
	file, err := PrepareConfigMigration(path, template)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(file.Data), "business_ports") {
		t.Fatal("old business ports were migrated")
	}
}
