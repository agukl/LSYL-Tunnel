package credseal

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"lsyltunnel/src/internal/protocol"
)

func TestSealOpenRoundTrip(t *testing.T) {
	privateKey, _, publicPEM, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	key := protocol.CredentialPublicKey{
		Type:         TypeServerSealed,
		KeyID:        "test-key",
		ExpiresAt:    expiresAt,
		PublicKeyPEM: string(publicPEM),
	}
	want := PlainCredential{Username: "alice", Password: "secret", ClientID: "client-1"}

	sealed, err := Seal(key, want)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Type != TypeServerSealed || sealed.KeyID != key.KeyID || sealed.ExpiresAt != expiresAt {
		t.Fatalf("sealed metadata = %+v", sealed)
	}
	got, err := Open(sealed, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != want.Username || got.Password != want.Password || got.ClientID != want.ClientID {
		t.Fatalf("opened credential = %+v, want %+v", got, want)
	}
	if got.IssuedAt == "" || got.ExpiresAt != expiresAt || got.Nonce == "" {
		t.Fatalf("generated credential metadata is incomplete: %+v", got)
	}
}

func TestOpenRejectsTamperedEnvelope(t *testing.T) {
	privateKey, _, publicPEM, err := GenerateKeyPair(2048)
	if err != nil {
		t.Fatal(err)
	}
	key := protocol.CredentialPublicKey{KeyID: "test-key", PublicKeyPEM: string(publicPEM)}
	sealed, err := Seal(key, PlainCredential{Username: "alice", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := base64.RawStdEncoding.DecodeString(sealed.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	envelope[len(envelope)-2] ^= 1
	sealed.Ciphertext = base64.RawStdEncoding.EncodeToString(envelope)
	if _, err := Open(sealed, privateKey); err == nil {
		t.Fatal("tampered credential envelope was accepted")
	}
}

func TestParseKeyPEMRejectsInvalidInput(t *testing.T) {
	if _, err := ParsePublicKeyPEM([]byte("not pem")); err == nil || !strings.Contains(err.Error(), "PEM not found") {
		t.Fatalf("ParsePublicKeyPEM() error = %v", err)
	}
	if _, err := ParsePrivateKeyPEM([]byte("not pem")); err == nil || !strings.Contains(err.Error(), "PEM not found") {
		t.Fatalf("ParsePrivateKeyPEM() error = %v", err)
	}
}
