package passutil

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("secret", hash) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrong", hash) {
		t.Fatal("wrong password verified")
	}
}

func TestVerifyPlainPassword(t *testing.T) {
	if !VerifyPassword("secret", "plain:secret") {
		t.Fatal("plain password should verify")
	}
	if VerifyPassword("wrong", "plain:secret") {
		t.Fatal("wrong plain password verified")
	}
}
