package tunnel

import (
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lsyltunnel/src/internal/credseal"
	"lsyltunnel/src/internal/protocol"

	"gopkg.in/yaml.v3"
)

type credentialSealKey struct {
	id        string
	expiresAt time.Time
	private   *rsa.PrivateKey
	publicPEM string
	active    bool
}

func (s *Server) loadCredentialSealKeys() error {
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()
	s.credentialKeys = map[string]*credentialSealKey{}
	now := time.Now()
	for _, item := range s.cfg.CredentialSeal.Keys {
		expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(item.ExpiresAt))
		if err != nil {
			return fmt.Errorf("parse credential seal key %s expires_at: %w", item.KeyID, err)
		}
		privateKey, publicPEM, err := credseal.EnsureKeyFiles(item.PrivateKeyFile, item.PublicKeyFile, 3072)
		if err != nil {
			return fmt.Errorf("load credential seal key %s: %w", item.KeyID, err)
		}
		key := &credentialSealKey{
			id:        strings.TrimSpace(item.KeyID),
			expiresAt: expiresAt,
			private:   privateKey,
			publicPEM: string(publicPEM),
			active:    item.Active,
		}
		s.credentialKeys[key.id] = key
		if item.Active {
			s.activeCredentialKey = key
		}
	}
	if err := s.rotateExpiredCredentialSealKeyLocked(now); err != nil {
		return err
	}
	return nil
}

func (s *Server) activeCredentialPublicKey() *protocol.CredentialPublicKey {
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()
	if err := s.rotateExpiredCredentialSealKeyLocked(time.Now()); err != nil {
		s.log("credential seal key rotation failed: %v", err)
		return nil
	}
	key := s.activeCredentialKey
	if key == nil || !key.expiresAt.After(time.Now()) {
		return nil
	}
	return &protocol.CredentialPublicKey{
		Type:         credseal.TypeServerSealed,
		KeyID:        key.id,
		ExpiresAt:    key.expiresAt.Format(time.RFC3339),
		PublicKeyPEM: key.publicPEM,
	}
}

func (s *Server) passwordFromRequest(req protocol.OpenRequest) (password, code, message string) {
	if req.Password != "" {
		return req.Password, "", ""
	}
	if req.Credential == nil || strings.TrimSpace(req.Credential.Ciphertext) == "" {
		return "", "auth_failed", "username or password is incorrect"
	}
	if req.Credential.Type != credseal.TypeServerSealed {
		return "", "auth_failed", "username or password is incorrect"
	}
	s.credentialMu.Lock()
	if err := s.rotateExpiredCredentialSealKeyLocked(time.Now()); err != nil {
		s.log("credential seal key rotation failed: %v", err)
	}
	key := s.credentialKeys[strings.TrimSpace(req.Credential.KeyID)]
	s.credentialMu.Unlock()
	if key == nil || !key.expiresAt.After(time.Now()) {
		return "", "credential_expired", "saved login has expired, please enter password again"
	}
	plain, err := credseal.Open(*req.Credential, key.private)
	if err != nil {
		return "", "auth_failed", "username or password is incorrect"
	}
	if plain.Username != req.Username {
		return "", "auth_failed", "username or password is incorrect"
	}
	if plain.ClientID != "" && req.ClientID != "" && plain.ClientID != req.ClientID {
		return "", "auth_failed", "username or password is incorrect"
	}
	if plain.ExpiresAt == "" {
		return "", "credential_expired", "saved login has expired, please enter password again"
	}
	plainExpiresAt, err := time.Parse(time.RFC3339, plain.ExpiresAt)
	if err != nil || !plainExpiresAt.After(time.Now()) {
		return "", "credential_expired", "saved login has expired, please enter password again"
	}
	return plain.Password, "", ""
}

func (s *Server) credentialSealRotationLoop(ctxDone <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctxDone:
			return
		case <-ticker.C:
			s.credentialMu.Lock()
			err := s.rotateExpiredCredentialSealKeyLocked(time.Now())
			s.credentialMu.Unlock()
			if err != nil {
				s.log("credential seal key rotation failed: %v", err)
			}
		}
	}
}

func (s *Server) rotateExpiredCredentialSealKeyLocked(now time.Time) error {
	current := s.activeCredentialKey
	if current == nil {
		return nil
	}
	if current.expiresAt.After(now) {
		return nil
	}
	newCfg, existingIndex := s.nextCredentialSealKeyConfigLocked(now, current)
	privateKey, publicPEM, err := credseal.EnsureKeyFiles(newCfg.PrivateKeyFile, newCfg.PublicKeyFile, 3072)
	if err != nil {
		return fmt.Errorf("rotate credential seal key %s: %w", newCfg.KeyID, err)
	}
	for i := range s.cfg.CredentialSeal.Keys {
		s.cfg.CredentialSeal.Keys[i].Active = false
	}
	if existingIndex >= 0 {
		s.cfg.CredentialSeal.Keys[existingIndex].Active = true
	} else {
		s.cfg.CredentialSeal.Keys = append(s.cfg.CredentialSeal.Keys, newCfg)
	}
	key := &credentialSealKey{
		id:        newCfg.KeyID,
		expiresAt: mustParseRFC3339(newCfg.ExpiresAt),
		private:   privateKey,
		publicPEM: string(publicPEM),
		active:    true,
	}
	if s.credentialKeys == nil {
		s.credentialKeys = map[string]*credentialSealKey{}
	}
	s.credentialKeys[key.id] = key
	s.activeCredentialKey = key
	if err := s.persistCredentialSealRotationLocked(newCfg); err != nil {
		s.log("persist credential seal key rotation failed: %v", err)
	}
	s.log("credential seal key rotated: %s -> %s", current.id, key.id)
	return nil
}

func (s *Server) nextCredentialSealKeyConfigLocked(now time.Time, current *credentialSealKey) (CredentialSealKeyConfig, int) {
	bestIndex := -1
	var bestExpiry time.Time
	for i, key := range s.cfg.CredentialSeal.Keys {
		if strings.TrimSpace(key.KeyID) == "" || strings.TrimSpace(key.KeyID) == current.id {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(key.ExpiresAt))
		if err != nil || !expiresAt.After(now) {
			continue
		}
		if bestIndex < 0 || expiresAt.Before(bestExpiry) {
			bestIndex = i
			bestExpiry = expiresAt
		}
	}
	if bestIndex >= 0 {
		return s.cfg.CredentialSeal.Keys[bestIndex], bestIndex
	}
	expiry := current.expiresAt
	if expiry.IsZero() {
		expiry = now
	}
	for !expiry.After(now) {
		expiry = expiry.AddDate(0, 3, 0)
	}
	existing := map[string]bool{}
	for _, key := range s.cfg.CredentialSeal.Keys {
		existing[strings.TrimSpace(key.KeyID)] = true
	}
	baseID := "login-key-" + expiry.Format("2006-01")
	keyID := baseID
	for i := 2; existing[keyID]; i++ {
		keyID = fmt.Sprintf("%s-%02d", baseID, i)
	}
	privateDir := ""
	publicDir := ""
	for _, key := range s.cfg.CredentialSeal.Keys {
		if strings.TrimSpace(key.KeyID) == current.id {
			privateDir = filepath.Dir(key.PrivateKeyFile)
			publicDir = filepath.Dir(key.PublicKeyFile)
			break
		}
	}
	if privateDir == "" || privateDir == "." {
		privateDir = "certs"
	}
	if publicDir == "" || publicDir == "." {
		publicDir = privateDir
	}
	return CredentialSealKeyConfig{
		KeyID:          keyID,
		PrivateKeyFile: filepath.Join(privateDir, keyID+".key"),
		PublicKeyFile:  filepath.Join(publicDir, keyID+".pub"),
		ExpiresAt:      expiry.Format(time.RFC3339),
		Active:         true,
	}, -1
}

func (s *Server) persistCredentialSealRotationLocked(newCfg CredentialSealKeyConfig) error {
	configPath := strings.TrimSpace(s.cfg.ConfigPath)
	if configPath == "" {
		return nil
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var raw Config
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	rawNew := newCfg
	rawNew.PrivateKeyFile = relativeCredentialPath(configPath, newCfg.PrivateKeyFile)
	rawNew.PublicKeyFile = relativeCredentialPath(configPath, newCfg.PublicKeyFile)
	if previous := rawCredentialPathTemplate(raw.CredentialSeal.Keys); previous.KeyID != "" {
		rawNew.PrivateKeyFile = joinConfigPath(filepath.Dir(previous.PrivateKeyFile), newCfg.KeyID+".key")
		rawNew.PublicKeyFile = joinConfigPath(filepath.Dir(previous.PublicKeyFile), newCfg.KeyID+".pub")
	}
	found := false
	for i := range raw.CredentialSeal.Keys {
		if strings.TrimSpace(raw.CredentialSeal.Keys[i].KeyID) == strings.TrimSpace(newCfg.KeyID) {
			raw.CredentialSeal.Keys[i].Active = true
			found = true
			continue
		}
		raw.CredentialSeal.Keys[i].Active = false
	}
	if !found {
		raw.CredentialSeal.Keys = append(raw.CredentialSeal.Keys, rawNew)
	}
	out, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0o644)
}

func rawCredentialPathTemplate(keys []CredentialSealKeyConfig) CredentialSealKeyConfig {
	for _, key := range keys {
		if key.Active {
			return key
		}
	}
	if len(keys) > 0 {
		return keys[len(keys)-1]
	}
	return CredentialSealKeyConfig{}
}

func relativeCredentialPath(configPath, target string) string {
	if strings.TrimSpace(target) == "" {
		return target
	}
	if rel, err := filepath.Rel(filepath.Dir(configPath), target); err == nil && !strings.HasPrefix(rel, ".."+string(filepath.Separator)+"..") {
		return filepath.ToSlash(rel)
	}
	return target
}

func joinConfigPath(dir, name string) string {
	dir = strings.TrimRight(strings.TrimSpace(dir), `/\`)
	if dir == "" || dir == "." {
		return name
	}
	if strings.Contains(dir, "/") {
		return dir + "/" + name
	}
	return dir + `\` + name
}

func mustParseRFC3339(text string) time.Time {
	t, _ := time.Parse(time.RFC3339, text)
	return t
}
