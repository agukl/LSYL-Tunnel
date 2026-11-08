package tunnel

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEntryTrafficLogSkipsAcceptedConnections(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echoLn := startEchoServer(t)
	dir := t.TempDir()
	entryLog := filepath.Join(dir, "entry-traffic.jsonl")
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
			HandshakeTimeoutSec: 3,
			DialTimeoutSec:      1,
			MaxHandshakeBytes:   32768,
			AuthFailThreshold:   3,
			AuthFailWindowSec:   60,
			AuthFailBlockSec:    60,
		},
		Runtime: RuntimeConfig{EntryTrafficLogFile: entryLog},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	resp := openTargetForTest(t, server.Addr(), certFile, "alice", "secret", "echo", echoLn.Addr().String())
	if !resp.OK {
		t.Fatalf("open response = %+v", resp)
	}

	path := datedJSONLPath(entryLog, time.Now().Format("2006-01-02"))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no entry traffic log for accepted connection, got err=%v", err)
	}
}

func TestBusinessLogRecordsOpenBytesWithoutLegacyFlowLog(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	echoLn := startEchoServer(t)
	dir := t.TempDir()
	businessLog := filepath.Join(dir, "business.jsonl")
	legacyFlowLog := filepath.Join(dir, "flow-traffic.jsonl")
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
			DialTimeoutSec:              1,
			MaxHandshakeBytes:           32768,
			AuthFailThreshold:           3,
			AuthFailWindowSec:           60,
			AuthFailBlockSec:            60,
			StreamRateLimitBytesPerSec:  1024,
			MaxConcurrentStreamsPerUser: 4,
		},
		Runtime: RuntimeConfig{BusinessLogFile: businessLog, FlowTrafficLogFile: legacyFlowLog},
	}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	conn, resp := openTargetConnForTest(t, server.Addr(), certFile, "alice", "secret", "echo", echoLn.Addr().String())
	if !resp.OK {
		_ = conn.Close()
		t.Fatalf("open response = %+v", resp)
	}
	payload := []byte("flow-log")
	if _, err := conn.Write(payload); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, buf); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	_ = conn.Close()

	path := datedJSONLPath(businessLog, time.Now().Format("2006-01-02"))
	waitForFileContains(t, path, `"result":"closed"`, 2*time.Second)
	waitForFileContains(t, path, `"bytes_up":8`, 2*time.Second)
	waitForFileContains(t, path, `"bytes_down":8`, 2*time.Second)
	waitForFileContains(t, path, `"bytes_total":16`, 2*time.Second)
	waitForFileContains(t, path, `"rate_limit_bytes_per_sec":1024`, 2*time.Second)
	if _, err := os.Stat(datedJSONLPath(legacyFlowLog, time.Now().Format("2006-01-02"))); !os.IsNotExist(err) {
		t.Fatalf("legacy flow log should not be written, got %v", err)
	}
}
