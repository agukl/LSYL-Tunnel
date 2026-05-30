package tunnel

import (
	"strings"
	"time"

	"lsyltunnel/src/internal/credseal"
	"lsyltunnel/src/internal/protocol"
)

func SealSavedCredential(key protocol.CredentialPublicKey, cfg Config, password string) (protocol.SealedCredential, error) {
	return credseal.Seal(key, credseal.PlainCredential{
		Username:  cfg.Username,
		Password:  password,
		ClientID:  cfg.ClientID,
		IssuedAt:  time.Now().Format(time.RFC3339),
		ExpiresAt: key.ExpiresAt,
	})
}

func credentialFromConfig(cfg Config) *protocol.SealedCredential {
	if strings.TrimSpace(cfg.SavedCredential.Ciphertext) == "" {
		return nil
	}
	credential := cfg.SavedCredential
	return &credential
}
