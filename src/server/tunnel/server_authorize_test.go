package tunnel

import "testing"

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

func TestAuthorizeAllowsConfiguredReverseForwardAllowedUser(t *testing.T) {
	s := &Server{cfg: Config{
		Forwards: []ForwardConfig{{
			Direction:    DirectionServerToClient,
			ListenAddr:   "127.0.0.1:18080",
			AllowedUsers: []string{"alice"},
		}},
	}}

	if err := s.authorizeReverse(UserConfig{Username: "alice"}, "127.0.0.1:18080"); err != nil {
		t.Fatalf("authorize reverse listener returned error: %v", err)
	}
}
