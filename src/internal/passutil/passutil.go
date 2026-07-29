package passutil

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const (
	DefaultIterations = 210000
	DefaultKeyBytes   = 32
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2SHA256([]byte(password), salt, DefaultIterations, DefaultKeyBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256:%d:%s:%s", DefaultIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(password, encoded string) bool {
	encoded = strings.TrimSpace(encoded)
	if strings.HasPrefix(encoded, "plain:") {
		want := strings.TrimPrefix(encoded, "plain:")
		return subtle.ConstantTimeCompare([]byte(password), []byte(want)) == 1
	}
	if strings.HasPrefix(encoded, "sha256:") {
		wantHex := strings.TrimPrefix(encoded, "sha256:")
		want, err := hex.DecodeString(wantHex)
		if err != nil {
			return false
		}
		sum := sha256.Sum256([]byte(password))
		return subtle.ConstantTimeCompare(sum[:], want) == 1
	}
	parts := strings.Split(encoded, ":")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 10000 || iterations > 2000000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < 8 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 || len(want) > 128 {
		return false
	}
	got, err := pbkdf2SHA256([]byte(password), salt, iterations, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) ([]byte, error) {
	if iterations <= 0 || keyLen <= 0 {
		return nil, fmt.Errorf("PBKDF2 iterations and key length must be positive")
	}
	return pbkdf2.Key(sha256.New, string(password), salt, iterations, keyLen)
}
