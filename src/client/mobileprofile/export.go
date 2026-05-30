package mobileprofile

import (
	"archive/zip"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"lsyltunnel/src/client/tunnel"
	"lsyltunnel/src/internal/protocol"
)

type Result struct {
	Path      string
	FileName  string
	ExpiresAt time.Time
}

type profileFile struct {
	Version         int                       `json:"version"`
	ProfileID       string                    `json:"profile_id,omitempty"`
	ServerAddr      string                    `json:"server_addr"`
	Username        string                    `json:"username"`
	ClientID        string                    `json:"client_id,omitempty"`
	SavedCredential protocol.SealedCredential `json:"saved_credential"`
	TLS             tlsConfig                 `json:"tls"`
	Connection      connectionConfig          `json:"connection"`
	Forwards        []forwardConfig           `json:"forwards"`
}

type tlsConfig struct {
	CACertFile         string `json:"ca_cert_file"`
	ServerName         string `json:"server_name"`
	MinVersion         string `json:"min_version"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
}

type connectionConfig struct {
	DialTimeoutSec int `json:"dial_timeout_sec"`
}

type forwardConfig struct {
	Name         string `json:"name"`
	Direction    string `json:"direction"`
	ListenAddr   string `json:"listen_addr"`
	ServerTarget string `json:"server_target"`
}

func Export(configFile, certFile, target string, force bool) (Result, error) {
	if err := requireFile(configFile); err != nil {
		return Result{}, fmt.Errorf("invalid client config: %w", err)
	}
	if err := requireFile(certFile); err != nil {
		return Result{}, fmt.Errorf("invalid server certificate: %w", err)
	}
	cfg, err := LoadConfig(configFile)
	if err != nil {
		return Result{}, err
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return Result{}, fmt.Errorf("read server certificate: %w", err)
	}
	if _, _, err := readFirstCert(certPEM); err != nil {
		return Result{}, fmt.Errorf("server certificate is not valid PEM: %w", err)
	}
	profile, expiresAt, err := ProfileFromConfig(cfg)
	if err != nil {
		return Result{}, err
	}
	body, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return Result{}, err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return Result{}, err
	}
	if err := writeZip(targetAbs, body, certPEM, force); err != nil {
		return Result{}, err
	}
	return Result{Path: targetAbs, FileName: filepath.Base(targetAbs), ExpiresAt: expiresAt}, nil
}

func LoadConfig(path string) (tunnel.Config, error) {
	var cfg tunnel.Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read client config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse client config: %w", err)
	}
	tunnel.ApplyDefaults(&cfg)
	return cfg, nil
}

func FileNameFromConfig(cfg tunnel.Config) (string, error) {
	if strings.TrimSpace(cfg.Username) == "" {
		return "", errors.New("username is required")
	}
	expiresAt, err := credentialExpiry(cfg)
	if err != nil {
		return "", err
	}
	return sanitizeFilePart(cfg.Username) + "_" + expiresAt.Format("2006-01-02") + ".lsylprofile", nil
}

func ProfileFromConfig(cfg tunnel.Config) (profileFile, time.Time, error) {
	tunnel.ApplyDefaults(&cfg)
	if _, _, err := splitHostPort(cfg.ServerAddr); err != nil {
		return profileFile{}, time.Time{}, fmt.Errorf("server_addr is invalid: %w", err)
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return profileFile{}, time.Time{}, errors.New("username is required")
	}
	expiresAt, err := credentialExpiry(cfg)
	if err != nil {
		return profileFile{}, time.Time{}, err
	}
	if cfg.TLS.InsecureSkipVerify {
		return profileFile{}, time.Time{}, errors.New("mobile profile cannot use tls.insecure_skip_verify")
	}
	if !tls13(cfg.TLS.MinVersion) {
		return profileFile{}, time.Time{}, errors.New("mobile profile requires tls.min_version 1.3")
	}
	forwards := make([]forwardConfig, 0, len(cfg.Forwards))
	seenNames := map[string]bool{}
	seenListens := map[string]bool{}
	for _, fwd := range cfg.Forwards {
		mobileFwd, err := forwardFromConfig(fwd)
		if err != nil {
			return profileFile{}, time.Time{}, err
		}
		if seenNames[mobileFwd.Name] {
			return profileFile{}, time.Time{}, fmt.Errorf("duplicate forward name: %s", mobileFwd.Name)
		}
		if seenListens[mobileFwd.ListenAddr] {
			return profileFile{}, time.Time{}, fmt.Errorf("duplicate mobile listen address: %s", mobileFwd.ListenAddr)
		}
		seenNames[mobileFwd.Name] = true
		seenListens[mobileFwd.ListenAddr] = true
		forwards = append(forwards, mobileFwd)
	}
	if len(forwards) == 0 {
		return profileFile{}, time.Time{}, errors.New("at least one client_to_server forward is required")
	}
	timeout := cfg.Connection.DialTimeoutSec
	if timeout <= 0 {
		timeout = 5
	}
	return profileFile{
		Version:         1,
		ProfileID:       profileID(cfg),
		ServerAddr:      strings.TrimSpace(cfg.ServerAddr),
		Username:        strings.TrimSpace(cfg.Username),
		ClientID:        strings.TrimSpace(cfg.ClientID),
		SavedCredential: cfg.SavedCredential,
		TLS: tlsConfig{
			CACertFile:         "server.crt",
			ServerName:         strings.TrimSpace(cfg.TLS.ServerName),
			MinVersion:         "1.3",
			InsecureSkipVerify: false,
		},
		Connection: connectionConfig{DialTimeoutSec: timeout},
		Forwards:   forwards,
	}, expiresAt, nil
}

func credentialExpiry(cfg tunnel.Config) (time.Time, error) {
	if strings.TrimSpace(cfg.SavedCredential.Ciphertext) == "" {
		return time.Time{}, errors.New("saved_credential is required; connect successfully in the client once before exporting")
	}
	if strings.TrimSpace(cfg.SavedCredential.Type) != "server_sealed" {
		return time.Time{}, errors.New("saved_credential.type must be server_sealed")
	}
	if strings.TrimSpace(cfg.SavedCredential.KeyID) == "" {
		return time.Time{}, errors.New("saved_credential.key_id is required")
	}
	if strings.TrimSpace(cfg.SavedCredential.ExpiresAt) == "" {
		return time.Time{}, errors.New("saved_credential.expires_at is required")
	}
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(cfg.SavedCredential.ExpiresAt))
	if err != nil {
		return time.Time{}, fmt.Errorf("saved_credential.expires_at must be RFC3339: %w", err)
	}
	if !expiresAt.After(time.Now()) {
		return time.Time{}, errors.New("saved_credential has expired; reconnect in the client before exporting")
	}
	return expiresAt, nil
}

func forwardFromConfig(fwd tunnel.ForwardConfig) (forwardConfig, error) {
	direction := strings.TrimSpace(fwd.Direction)
	if direction == "" {
		direction = tunnel.DirectionClientToServer
	}
	if direction != tunnel.DirectionClientToServer {
		return forwardConfig{}, fmt.Errorf("forward %q is %s; mobile export only supports client_to_server", forwardNameForError(fwd), direction)
	}
	host, port, err := splitHostPort(fwd.ListenAddr)
	if err != nil {
		return forwardConfig{}, fmt.Errorf("forward %q listen_addr is invalid: %w", forwardNameForError(fwd), err)
	}
	if !isLoopback(host) {
		return forwardConfig{}, fmt.Errorf("forward %q listen_addr must use 127.0.0.1 for mobile", forwardNameForError(fwd))
	}
	if port < 1024 {
		return forwardConfig{}, fmt.Errorf("forward %q listen port %d is below 1024 and cannot be used by Android", forwardNameForError(fwd), port)
	}
	if _, _, err := splitHostPort(fwd.ServerTarget); err != nil {
		return forwardConfig{}, fmt.Errorf("forward %q server_target is invalid: %w", forwardNameForError(fwd), err)
	}
	listenAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	name := strings.TrimSpace(fwd.Name)
	if name == "" {
		name = listenAddr
	}
	return forwardConfig{
		Name:         name,
		Direction:    tunnel.DirectionClientToServer,
		ListenAddr:   listenAddr,
		ServerTarget: strings.TrimSpace(fwd.ServerTarget),
	}, nil
}

func writeZip(target string, profileJSON, certPEM []byte, force bool) error {
	if _, err := os.Stat(target); err == nil && !force {
		return fmt.Errorf("output already exists: %s", target)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := writeZipTo(tmp, profileJSON, certPEM); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if force {
		_ = os.Remove(target)
	}
	return os.Rename(tmpName, target)
}

func writeZipTo(w io.Writer, profileJSON, certPEM []byte) error {
	zw := zip.NewWriter(w)
	if err := writeZipFile(zw, "profile.json", profileJSON); err != nil {
		_ = zw.Close()
		return err
	}
	if err := writeZipFile(zw, "server.crt", certPEM); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetModTime(time.Now())
	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func readFirstCert(data []byte) (*x509.Certificate, []byte, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, nil, errors.New("no PEM certificate found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, block.Bytes, nil
}

func requireFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return errors.New("is a directory")
	}
	return nil
}

func splitHostPort(value string) (string, int, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port")
	}
	return strings.Trim(host, "[]"), port, nil
}

func isLoopback(host string) bool {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func tls13(version string) bool {
	switch strings.TrimSpace(strings.ToLower(version)) {
	case "", "1.3", "tls1.3":
		return true
	default:
		return false
	}
}

func profileID(cfg tunnel.Config) string {
	id := strings.TrimSpace(cfg.ClientID)
	if id == "" {
		id = strings.TrimSpace(cfg.Username)
	}
	return "mobile-" + sanitizeFilePart(id)
}

func sanitizeFilePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "client"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
			continue
		}
		b.WriteRune(r)
		lastDash = false
	}
	clean := strings.Trim(b.String(), " .-")
	if clean == "" {
		return "client"
	}
	return clean
}

func forwardNameForError(fwd tunnel.ForwardConfig) string {
	if name := strings.TrimSpace(fwd.Name); name != "" {
		return name
	}
	if addr := strings.TrimSpace(fwd.ListenAddr); addr != "" {
		return addr
	}
	return strings.TrimSpace(fwd.ServerTarget)
}
