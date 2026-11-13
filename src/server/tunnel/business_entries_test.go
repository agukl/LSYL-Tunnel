package tunnel

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clienttunnel "lsyltunnel/src/client/tunnel"
)

func TestBusinessEntriesValidation(t *testing.T) {
	valid := Config{BusinessEntries: []BusinessEntryConfig{
		{ListenAddr: "0.0.0.0:50000", AdvertiseAddr: "203.0.113.10:50000"},
		{ListenAddr: "[::]:50001", AdvertiseAddr: "[2001:db8::10]:50001"},
	}}
	if err := validateBusinessEntries(valid); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		entry BusinessEntryConfig
	}{
		{"hostname", BusinessEntryConfig{ListenAddr: "0.0.0.0:50000", AdvertiseAddr: "example.com:50000"}},
		{"family mismatch", BusinessEntryConfig{ListenAddr: "0.0.0.0:50000", AdvertiseAddr: "[2001:db8::10]:50000"}},
		{"unspecified advertisement", BusinessEntryConfig{ListenAddr: "[::]:50000", AdvertiseAddr: "[::]:50000"}},
		{"missing port", BusinessEntryConfig{ListenAddr: "127.0.0.1:0", AdvertiseAddr: "127.0.0.1:50000"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateBusinessEntries(Config{BusinessEntries: []BusinessEntryConfig{tc.entry}}); err == nil {
				t.Fatal("expected invalid business entry")
			}
		})
	}
	if err := validateBusinessEntries(Config{BusinessEntries: []BusinessEntryConfig{valid.BusinessEntries[0], valid.BusinessEntries[0]}}); err == nil {
		t.Fatal("expected duplicate entry error")
	}
}

func TestBusinessEntryCarriesForwardAndHealth(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echo := startEchoServer(t)
	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entryAddr := reserved.Addr().String()
	_ = reserved.Close()
	requestLogPath := filepath.Join(t.TempDir(), "request.jsonl")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Start(ctx, Config{
		ListenAddr:      "127.0.0.1:0",
		BusinessEntries: []BusinessEntryConfig{{ListenAddr: entryAddr, AdvertiseAddr: entryAddr}},
		TLS:             TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: "1.2"},
		Auth:            AuthConfig{Users: []UserConfig{{Username: "alice", PasswordHash: "plain:secret"}}},
		Forwards:        []ForwardConfig{{Name: "echo", Direction: DirectionClientToServer, ServerTarget: echo.Addr().String(), AllowedUsers: []string{"alice"}}},
		Security:        SecurityConfig{HandshakeTimeoutSec: 3, DialTimeoutSec: 3, MaxHandshakeBytes: 32768, AuthFailThreshold: 3, AuthFailWindowSec: 60, AuthFailBlockSec: 60},
		Runtime:         RuntimeConfig{RequestLogFile: requestLogPath},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	cfg := clienttunnel.Config{
		ServerAddr: server.Addr(), Username: "alice", Password: "secret", ClientID: "test-client",
		TLS:        clienttunnel.TLSConfig{CACertFile: certFile, ServerName: "127.0.0.1", MinVersion: "1.2"},
		Connection: clienttunnel.ConnectionConfig{DialTimeoutSec: 3},
		Forwards:   []clienttunnel.ForwardConfig{{Name: "echo", ListenAddr: "127.0.0.1:0", ServerTarget: echo.Addr().String()}},
	}
	login, err := clienttunnel.CheckLoginResponse(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(login.BusinessEntries) != 1 || login.BusinessEntries[0] != entryAddr {
		t.Fatalf("login advertised entries = %v", login.BusinessEntries)
	}
	_ = server.listener.Close()
	client, err := clienttunnel.StartVerifiedWithEntries(ctx, cfg, login.ServerVersion, login.BusinessEntries, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if status := client.CheckHealthNow(ctx); status.State != clienttunnel.HealthOK {
		t.Fatalf("health through pool = %+v", status)
	}
	quality := server.status().EntryQuality
	if len(quality) != 1 || quality[0].ClientID != "test-client" || len(quality[0].Entries) != 2 || quality[0].Entries[1].Successes != 1 {
		t.Fatalf("reported entry quality = %+v", quality)
	}
	logData, err := os.ReadFile(datedJSONLPath(requestLogPath, time.Now().Format("2006-01-02")))
	if err != nil || strings.Contains(string(logData), "entry_quality") {
		t.Fatalf("request log contains quality report or cannot be read: %v", err)
	}
	if summary := client.CheckForwardsNow(ctx); summary.Allowed != 1 {
		t.Fatalf("forward check through pool = %+v", summary)
	}
	conn, err := net.DialTimeout("tcp", client.ForwardAddr("echo"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("pool-echo")); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, len("pool-echo"))
	if _, err := io.ReadFull(conn, data); err != nil || string(data) != "pool-echo" {
		t.Fatalf("echo = %q, err = %v", data, err)
	}
	if got := server.totalStreams.Load(); got != 1 {
		t.Fatalf("business streams = %d", got)
	}
}

func TestBusinessEntrySchemaRejectsWrongLayer(t *testing.T) {
	data := []byte(validServerConfigV1YAML(""))
	data = append(data, []byte("business_entries:\n  listen_addr: 127.0.0.1:50000\n")...)
	if _, err := decodeServerConfig(data); err == nil {
		t.Fatalf("expected business_entries structure error, got %v", err)
	}
}

func TestBusinessEntrySurvivesConfigMigration(t *testing.T) {
	dir := t.TempDir()
	data := validServerConfigV1YAML("") + "business_entries:\n  - listen_addr: 0.0.0.0:50000\n    advertise_addr: 203.0.113.10:50000\n"
	path := writeTestConfig(t, dir, "server.yaml", data)
	template := writeTestConfig(t, dir, "template.yaml", validServerConfigV1YAML("")+"business_entries:\n  - listen_addr: 0.0.0.0:50001\n    advertise_addr: 203.0.113.11:50001\n")
	file, err := PrepareConfigMigration(path, template)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := decodeServerConfig(file.Data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BusinessEntries) != 1 || cfg.BusinessEntries[0].ListenAddr != "0.0.0.0:50000" || cfg.BusinessEntries[0].AdvertiseAddr != "203.0.113.10:50000" {
		t.Fatalf("migrated entries = %+v", cfg.BusinessEntries)
	}
}

func TestBusinessEntryMigrationUsesTemplateWhenOldConfigHasNoPool(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, "server.yaml", validServerConfigV1YAML(""))
	template := writeTestConfig(t, dir, "template.yaml", validServerConfigV1YAML("")+"business_entries:\n  - listen_addr: 0.0.0.0:50001\n    advertise_addr: 203.0.113.11:50001\n")
	file, err := PrepareConfigMigration(path, template)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := decodeServerConfig(file.Data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.BusinessEntries) != 1 || cfg.BusinessEntries[0].AdvertiseAddr != "203.0.113.11:50001" {
		t.Fatalf("migrated entries = %+v", cfg.BusinessEntries)
	}
}

func TestBusinessEntryMigrationRejectsInvalidOldPool(t *testing.T) {
	dir := t.TempDir()
	template := writeTestConfig(t, dir, "template.yaml", validServerConfigV1YAML(""))
	for _, bad := range []string{
		"business_entries: {}\n",
		"business_entries:\n  - listen_addr: 0.0.0.0:50000\n    advertise_addr: 203.0.113.10:0\n",
		"business_entries:\n  - listen_addr: 0.0.0.0:50000\n    advertise_addr: 203.0.113.10:50000\n    unknown: true\n",
	} {
		path := writeTestConfig(t, dir, "server.yaml", validServerConfigV1YAML("")+bad)
		if _, err := PrepareConfigMigration(path, template); err == nil {
			t.Fatalf("invalid pool was accepted: %s", bad)
		}
	}
}
