package tunnel

import (
	"strings"
	"testing"

	"lsyltunnel/src/internal/protocol"
)

func TestAuthorizeAllowsConfiguredForwardAllowedUser(t *testing.T) {
	s := &Server{cfg: Config{
		Forwards: []ForwardConfig{{
			Direction:    DirectionClientToServer,
			ServerTarget: "127.0.0.1:3389",
			AllowedUsers: []string{
				"alice",
				"bob",
			},
		}},
	}}

	if err := s.authorize(UserConfig{Username: "bob"}, "127.0.0.1:3389"); err != nil {
		t.Fatalf("authorize bob returned error: %v", err)
	}
	if err := s.authorize(UserConfig{Username: "carol"}, "127.0.0.1:3389"); err == nil {
		t.Fatal("authorize carol unexpectedly succeeded")
	}
}

func TestAuthorizeOpenIgnoresClientRuleName(t *testing.T) {
	s := &Server{cfg: Config{Forwards: []ForwardConfig{{
		Name:         "server-rdp",
		Direction:    DirectionClientToServer,
		ServerTarget: "127.0.0.1:3389",
		AllowedUsers: []string{"alice"},
	}}}}
	req := protocol.OpenRequest{
		ForwardName: "client-local-rdp",
		Direction:   DirectionClientToServer,
		Target:      "127.0.0.1:3389",
	}

	if err := s.authorizeOpen(UserConfig{Username: "alice"}, req); err != nil {
		t.Fatalf("authorizeOpen() rejected different client rule name: %v", err)
	}
}

func TestAuthorizeAllowsConfiguredPrivateTarget(t *testing.T) {
	s := &Server{cfg: Config{
		Forwards: []ForwardConfig{{
			Direction:    DirectionClientToServer,
			ServerTarget: "192.168.10.20:3389",
			AllowedUsers: []string{"alice"},
		}},
	}}

	if err := s.authorize(UserConfig{Username: "alice"}, "192.168.10.20:3389"); err != nil {
		t.Fatalf("authorize private target returned error: %v", err)
	}
	if err := s.authorize(UserConfig{Username: "alice"}, "192.168.10.21:3389"); err == nil {
		t.Fatal("authorize unexpected private target unexpectedly succeeded")
	}
}

func TestAuthorizeRejectsConfiguredPublicTarget(t *testing.T) {
	s := &Server{cfg: Config{
		Forwards: []ForwardConfig{{
			Direction:    DirectionClientToServer,
			ServerTarget: "203.0.113.20:3389",
			AllowedUsers: []string{"alice"},
		}},
	}}

	if err := s.authorize(UserConfig{Username: "alice"}, "203.0.113.20:3389"); err == nil {
		t.Fatal("authorize public target unexpectedly succeeded")
	}
}

func TestAuthorizeSeparatesForwardTargetsByAllowedUser(t *testing.T) {
	s := &Server{cfg: Config{
		Forwards: []ForwardConfig{
			{
				Direction:    DirectionClientToServer,
				ServerTarget: "127.0.0.1:3389",
				AllowedUsers: []string{"alice"},
			},
			{
				Direction:    DirectionClientToServer,
				ServerTarget: "127.0.0.1:5432",
				AllowedUsers: []string{"bob"},
			},
		},
	}}

	if err := s.authorize(UserConfig{Username: "alice"}, "127.0.0.1:3389"); err != nil {
		t.Fatalf("authorize alice for her target returned error: %v", err)
	}
	if err := s.authorize(UserConfig{Username: "alice"}, "127.0.0.1:5432"); err == nil {
		t.Fatal("authorize alice for bob target unexpectedly succeeded")
	}
	if err := s.authorize(UserConfig{Username: "bob"}, "127.0.0.1:5432"); err != nil {
		t.Fatalf("authorize bob for his target returned error: %v", err)
	}
	if err := s.authorize(UserConfig{Username: "bob"}, "127.0.0.1:3389"); err == nil {
		t.Fatal("authorize bob for alice target unexpectedly succeeded")
	}
}

func TestAuthorizeOpenIgnoresReverseForwardPermissions(t *testing.T) {
	s := &Server{cfg: Config{
		Forwards: []ForwardConfig{{
			Direction:    DirectionServerToClient,
			ListenAddr:   "127.0.0.1:18080",
			AllowedUsers: []string{"alice"},
		}},
	}}

	if err := s.authorize(UserConfig{Username: "alice"}, "127.0.0.1:18080"); err == nil {
		t.Fatal("client-to-server authorize unexpectedly matched a reverse forward")
	}
}

func TestAuthorizeReverseUsesClientSelectedListenerAndOwnership(t *testing.T) {
	s := &Server{cfg: Config{Forwards: []ForwardConfig{
		{Name: "mysql", Direction: DirectionServerToClient, ListenPort: 13306, AllowedUsers: []string{"alice"}},
		{Name: "web", Direction: DirectionServerToClient, ListenPort: 18080, AllowedUsers: []string{"bob"}},
	}}}

	if err := s.authorizeReverse(UserConfig{Username: "alice"}, "127.0.0.1:13306"); err != nil {
		t.Fatalf("authorizeReverse() rejected alice listener: %v", err)
	}
	if err := s.authorizeReverse(UserConfig{Username: "bob"}, "127.0.0.1:18080"); err != nil {
		t.Fatalf("authorizeReverse() rejected bob listener: %v", err)
	}
	if err := s.authorizeReverse(UserConfig{Username: "bob"}, "127.0.0.1:13306"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("authorizeReverse() error = %v, want listener ownership rejection", err)
	}
}

func TestAuthorizeReverseAllowsMultipleListenersForAccount(t *testing.T) {
	s := &Server{cfg: Config{Forwards: []ForwardConfig{
		{Name: "mysql", Direction: DirectionServerToClient, ListenPort: 13306, AllowedUsers: []string{"alice"}},
		{Name: "web", Direction: DirectionServerToClient, ListenPort: 18080, AllowedUsers: []string{"alice"}},
	}}}

	for _, listenAddr := range []string{"127.0.0.1:13306", "127.0.0.1:18080"} {
		if err := s.authorizeReverse(UserConfig{Username: "alice"}, listenAddr); err != nil {
			t.Fatalf("authorizeReverse(%q) rejected shared account ownership: %v", listenAddr, err)
		}
	}
}

func TestAuthorizeForwardCheckRequiresConfiguredReverseListener(t *testing.T) {
	s := &Server{cfg: Config{Forwards: []ForwardConfig{{
		Name: "server-name", Direction: DirectionServerToClient, ListenPort: 13306, AllowedUsers: []string{"alice"},
	}}}}
	s.markConfiguredReverseListener("127.0.0.1:13306")
	user := UserConfig{Username: "alice"}

	if err := s.authorizeForwardCheck(user, protocol.OpenRequest{Direction: DirectionServerToClient}); err == nil || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("authorizeForwardCheck() error = %v, want required listener", err)
	}
	if err := s.authorizeForwardCheck(user, protocol.OpenRequest{
		Direction: DirectionServerToClient, ListenAddr: "127.0.0.1:18080",
	}); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("authorizeForwardCheck() error = %v, want unconfigured listener", err)
	}
	if err := s.authorizeForwardCheck(user, protocol.OpenRequest{
		ForwardName: "different-client-name", Direction: DirectionServerToClient, ListenAddr: "127.0.0.1:13306",
	}); err != nil {
		t.Fatalf("authorizeForwardCheck() matched by listener should ignore rule name: %v", err)
	}
}
