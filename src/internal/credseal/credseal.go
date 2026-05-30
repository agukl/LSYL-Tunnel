package credseal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"lsyltunnel/src/internal/protocol"
)

const (
	TypeServerSealed = "server_sealed"
	EnvelopeAlg      = "rsa-oaep-sha256+a256gcm"
)

type PlainCredential struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	ClientID  string `json:"client_id,omitempty"`
	IssuedAt  string `json:"issued_at"`
	ExpiresAt string `json:"expires_at"`
	Nonce     string `json:"nonce"`
}

type encryptedEnvelope struct {
	Alg   string `json:"alg"`
	EK    string `json:"ek"`
	Nonce string `json:"nonce"`
	CT    string `json:"ct"`
}

func Seal(key protocol.CredentialPublicKey, plain PlainCredential) (protocol.SealedCredential, error) {
	pub, err := ParsePublicKeyPEM([]byte(key.PublicKeyPEM))
	if err != nil {
		return protocol.SealedCredential{}, err
	}
	if plain.IssuedAt == "" {
		plain.IssuedAt = time.Now().Format(time.RFC3339)
	}
	if plain.ExpiresAt == "" {
		plain.ExpiresAt = key.ExpiresAt
	}
	if plain.Nonce == "" {
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return protocol.SealedCredential{}, err
		}
		plain.Nonce = base64.RawStdEncoding.EncodeToString(nonce)
	}
	payload, err := json.Marshal(plain)
	if err != nil {
		return protocol.SealedCredential{}, err
	}
	contentKey := make([]byte, 32)
	if _, err := rand.Read(contentKey); err != nil {
		return protocol.SealedCredential{}, err
	}
	block, err := aes.NewCipher(contentKey)
	if err != nil {
		return protocol.SealedCredential{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return protocol.SealedCredential{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return protocol.SealedCredential{}, err
	}
	ct := gcm.Seal(nil, nonce, payload, []byte(key.KeyID))
	ek, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, contentKey, []byte(key.KeyID))
	if err != nil {
		return protocol.SealedCredential{}, err
	}
	env, err := json.Marshal(encryptedEnvelope{
		Alg:   EnvelopeAlg,
		EK:    base64.RawStdEncoding.EncodeToString(ek),
		Nonce: base64.RawStdEncoding.EncodeToString(nonce),
		CT:    base64.RawStdEncoding.EncodeToString(ct),
	})
	if err != nil {
		return protocol.SealedCredential{}, err
	}
	return protocol.SealedCredential{
		Type:       TypeServerSealed,
		KeyID:      key.KeyID,
		ExpiresAt:  key.ExpiresAt,
		Ciphertext: base64.RawStdEncoding.EncodeToString(env),
	}, nil
}

func Open(sealed protocol.SealedCredential, privateKey *rsa.PrivateKey) (PlainCredential, error) {
	var plain PlainCredential
	if sealed.Type != TypeServerSealed {
		return plain, fmt.Errorf("unsupported credential type")
	}
	envJSON, err := base64.RawStdEncoding.DecodeString(sealed.Ciphertext)
	if err != nil {
		return plain, fmt.Errorf("decode credential envelope: %w", err)
	}
	var env encryptedEnvelope
	if err := json.Unmarshal(envJSON, &env); err != nil {
		return plain, fmt.Errorf("parse credential envelope: %w", err)
	}
	if env.Alg != EnvelopeAlg {
		return plain, fmt.Errorf("unsupported credential envelope")
	}
	ek, err := base64.RawStdEncoding.DecodeString(env.EK)
	if err != nil {
		return plain, fmt.Errorf("decode credential key: %w", err)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return plain, fmt.Errorf("decode credential nonce: %w", err)
	}
	ct, err := base64.RawStdEncoding.DecodeString(env.CT)
	if err != nil {
		return plain, fmt.Errorf("decode credential ciphertext: %w", err)
	}
	contentKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, ek, []byte(sealed.KeyID))
	if err != nil {
		return plain, fmt.Errorf("open credential key: %w", err)
	}
	block, err := aes.NewCipher(contentKey)
	if err != nil {
		return plain, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return plain, err
	}
	payload, err := gcm.Open(nil, nonce, ct, []byte(sealed.KeyID))
	if err != nil {
		return plain, fmt.Errorf("open credential payload: %w", err)
	}
	if err := json.Unmarshal(payload, &plain); err != nil {
		return plain, fmt.Errorf("parse credential payload: %w", err)
	}
	return plain, nil
}

func ParsePublicKeyPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("public key PEM not found")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return pub, nil
}

func ParsePrivateKeyPEM(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("private key PEM not found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return key, nil
}

func GenerateKeyPair(bits int) (*rsa.PrivateKey, []byte, []byte, error) {
	if bits < 2048 {
		bits = 3072
	}
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, nil, err
	}
	privateDER := x509.MarshalPKCS1PrivateKey(key)
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, nil, err
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateDER})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	return key, privatePEM, publicPEM, nil
}

func EnsureKeyFiles(privatePath, publicPath string, bits int) (*rsa.PrivateKey, []byte, error) {
	if privateData, err := os.ReadFile(privatePath); err == nil {
		key, err := ParsePrivateKeyPEM(privateData)
		if err != nil {
			return nil, nil, err
		}
		publicData, err := os.ReadFile(publicPath)
		if err == nil {
			pub, err := ParsePublicKeyPEM(publicData)
			if err == nil && pub.N.Cmp(key.PublicKey.N) == 0 && pub.E == key.PublicKey.E {
				return key, publicData, nil
			}
		}
		publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			return nil, nil, err
		}
		publicData = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
		if err := writeFileSecure(publicPath, publicData, 0o644); err != nil {
			return nil, nil, err
		}
		return key, publicData, nil
	}
	key, privatePEM, publicPEM, err := GenerateKeyPair(bits)
	if err != nil {
		return nil, nil, err
	}
	if err := writeFileSecure(privatePath, privatePEM, 0o600); err != nil {
		return nil, nil, err
	}
	if err := writeFileSecure(publicPath, publicPEM, 0o644); err != nil {
		return nil, nil, err
	}
	return key, publicPEM, nil
}

func writeFileSecure(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}
