//go:build windows

package gui

import (
	"crypto/tls"
	"encoding/pem"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lsyltunnel/src/client/tunnel"
	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
)

func TestAutoConnectUsesSavedCredentialAndHandlesRejections(t *testing.T) {
	for _, code := range []string{"", "credential_expired", "auth_blocked"} {
		t.Run("response_"+code, func(t *testing.T) {
			identityServer := httptest.NewTLSServer(nil)
			identity := identityServer.TLS.Certificates[0]
			identityServer.Close()
			key, err := transport.CertificateGateKeyFromDER(identity.Certificate[0])
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			requests := make(chan protocol.OpenRequest, 1)
			serverErrors := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					serverErrors <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				if err := transport.ServerCertificateGate(conn, key); err != nil {
					serverErrors <- err
					return
				}
				secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{identity}, MinVersion: tls.VersionTLS13})
				var req protocol.OpenRequest
				if err := protocol.ReadJSON(secured, &req, protocol.DefaultMaxHandshakeBytes); err != nil {
					serverErrors <- err
					return
				}
				requests <- req
				serverErrors <- protocol.WriteJSON(secured, protocol.OpenResponse{OK: code == "", Code: code, Message: "test response", ServerVersion: "3.1.1"})
			}()
			root := t.TempDir()
			certPath := filepath.Join(root, "server.crt")
			if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: identity.Certificate[0]}), 0600); err != nil {
				t.Fatal(err)
			}
			a := &App{configPath: filepath.Join(root, "client.yaml"), autostartLaunch: true}
			cfg := defaultClientConfig(a.configPath)
			cfg.ServerAddr, cfg.Username, cfg.Password = listener.Addr().String(), "alice", "must-not-be-sent"
			cfg.TLS.CACertFile = certPath
			cfg.SavedCredential = protocol.SealedCredential{Type: "server_sealed", KeyID: "test-key", ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339), Ciphertext: "c2VhbGVk"}
			cfg.Forwards = []tunnel.ForwardConfig{{Name: "test", Direction: tunnel.DirectionClientToServer, ListenAddr: "127.0.0.1:0", ServerTarget: "127.0.0.1:80"}}
			if err := a.saveClientConfig(cfg); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(a.configPath)
			a.autoConnect()
			t.Cleanup(func() {
				if a.isRunning() {
					_ = a.stopClient()
				}
			})
			if !a.autoLoginDone.Load() || a.connecting.Load() {
				t.Fatal("automatic login did not finish")
			}
			select {
			case req := <-requests:
				if req.Type != "login" || req.Password != "" || req.Credential == nil || req.Credential.Ciphertext != "c2VhbGVk" {
					t.Fatalf("request = %+v", req)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("no login request; notice=%s", a.notice)
			}
			if err := <-serverErrors; err != nil {
				t.Fatal(err)
			}
			if a.isRunning() != (code == "") {
				t.Fatalf("running=%v code=%s notice=%s", a.isRunning(), code, a.notice)
			}
			after, _ := os.ReadFile(a.configPath)
			if code == "credential_expired" {
				updated, err := readClientConfigRaw(a.configPath)
				if err != nil || updated.SavedCredential.Ciphertext != "" {
					t.Fatalf("expired credential retained: %v", err)
				}
			} else if string(before) != string(after) {
				t.Fatal("automatic login rewrote the profile")
			}
			if code != "" && !a.noticeBad {
				t.Fatal("failure was not shown")
			}
		})
	}
}

func TestAutoConnectMissingCredentialShowsLogin(t *testing.T) {
	a := &App{configPath: filepath.Join(t.TempDir(), "client.yaml"), autostartLaunch: true}
	if err := a.saveClientConfig(defaultClientConfig(a.configPath)); err != nil {
		t.Fatal(err)
	}
	a.autoConnect()
	if a.isRunning() || a.windowHidden.Load() || !a.noticeBad || !strings.Contains(a.notice, "未保存登录凭据") {
		t.Fatalf("missing credential state: %+v", a.currentAPIState())
	}
}
