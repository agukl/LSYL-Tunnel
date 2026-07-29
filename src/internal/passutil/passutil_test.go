package passutil

import (
	"encoding/hex"
	"testing"
)

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

func TestPBKDF2SHA256MatchesLegacyVector(t *testing.T) {
	got, err := pbkdf2SHA256([]byte("secret"), []byte("0123456789abcdef"), 1000, 32)
	if err != nil {
		t.Fatal(err)
	}
	const want = "b622961f2e0500609613c827e86b4a85ac43d8e79e0145165c54ffa7569a365f"
	if encoded := hex.EncodeToString(got); encoded != want {
		t.Fatalf("derived key = %s, want %s", encoded, want)
	}
}

func TestPBKDF2SHA256AllocationsDoNotScaleWithIterations(t *testing.T) {
	allocs := testing.AllocsPerRun(5, func() {
		if _, err := pbkdf2SHA256([]byte("secret"), []byte("0123456789abcdef"), 1000, 32); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 20 {
		t.Fatalf("allocations = %.0f, want at most 20", allocs)
	}
}
