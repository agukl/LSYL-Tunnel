package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const DefaultMaxHandshakeBytes = 32 * 1024

type OpenRequest struct {
	Type        string            `json:"type"`
	Username    string            `json:"username"`
	Password    string            `json:"password"`
	Credential  *SealedCredential `json:"credential,omitempty"`
	ClientID    string            `json:"client_id,omitempty"`
	ForwardName string            `json:"forward_name,omitempty"`
	Direction   string            `json:"direction,omitempty"`
	ListenAddr  string            `json:"listen_addr,omitempty"`
	StreamID    string            `json:"stream_id,omitempty"`
	Target      string            `json:"target"`
}

type OpenResponse struct {
	OK            bool                 `json:"ok"`
	Code          string               `json:"code,omitempty"`
	Message       string               `json:"message,omitempty"`
	ListenAddr    string               `json:"listen_addr,omitempty"`
	StreamID      string               `json:"stream_id,omitempty"`
	CredentialKey *CredentialPublicKey `json:"credential_key,omitempty"`
}

type SealedCredential struct {
	Type       string `json:"type" yaml:"type"`
	KeyID      string `json:"key_id" yaml:"key_id"`
	ExpiresAt  string `json:"expires_at" yaml:"expires_at"`
	Ciphertext string `json:"ciphertext" yaml:"ciphertext"`
}

type CredentialPublicKey struct {
	Type         string `json:"type" yaml:"type"`
	KeyID        string `json:"key_id" yaml:"key_id"`
	ExpiresAt    string `json:"expires_at" yaml:"expires_at"`
	PublicKeyPEM string `json:"public_key_pem" yaml:"public_key_pem"`
}

func WriteJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(data) > DefaultMaxHandshakeBytes {
		return fmt.Errorf("handshake too large: %d bytes", len(data))
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func ReadJSON(r io.Reader, v any, maxBytes int) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxHandshakeBytes
	}
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n == 0 {
		return fmt.Errorf("empty handshake")
	}
	if int(n) > maxBytes {
		return fmt.Errorf("handshake too large: %d bytes", n)
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
