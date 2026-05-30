package mobileprofile

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"lsyltunnel/src/client/tunnel"
)

const (
	profileJSONEntry = "profile.json"
	serverCertEntry  = "server.crt"
)

type ImportedProfile struct {
	Config    tunnel.Config
	CertPEM   []byte
	ExpiresAt time.Time
	ProfileID string
}

func ImportFile(path string) (ImportedProfile, error) {
	if err := requireFile(path); err != nil {
		return ImportedProfile{}, fmt.Errorf("invalid profile file: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ImportedProfile{}, fmt.Errorf("read profile file: %w", err)
	}
	return ImportBytes(data)
}

func ImportBytes(data []byte) (ImportedProfile, error) {
	profileJSON, certPEM, err := readProfileZip(data)
	if err != nil {
		return ImportedProfile{}, err
	}
	if err := rejectSecretProfileFields(profileJSON); err != nil {
		return ImportedProfile{}, err
	}
	var profile profileFile
	if err := json.Unmarshal(profileJSON, &profile); err != nil {
		return ImportedProfile{}, fmt.Errorf("parse profile.json: %w", err)
	}
	return profileToConfig(profile, certPEM)
}

func readProfileZip(data []byte) ([]byte, []byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("open .lsylprofile: %w", err)
	}
	var profileJSON []byte
	var certPEM []byte
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := normalizedZipEntryName(f.Name)
		switch name {
		case profileJSONEntry, serverCertEntry:
			data, err := readZipEntryBytes(f)
			if err != nil {
				return nil, nil, err
			}
			if name == profileJSONEntry {
				profileJSON = data
			} else {
				certPEM = data
			}
		}
	}
	if len(profileJSON) == 0 {
		return nil, nil, errors.New("profile package is missing profile.json")
	}
	if len(certPEM) == 0 {
		return nil, nil, errors.New("profile package is missing server.crt")
	}
	return profileJSON, certPEM, nil
}

func normalizedZipEntryName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

func readZipEntryBytes(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read zip entry %s: %w", f.Name, err)
	}
	return data, nil
}

func rejectSecretProfileFields(profileJSON []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(profileJSON, &raw); err != nil {
		return fmt.Errorf("parse profile.json: %w", err)
	}
	for _, key := range []string{"password", "password_env", "password_file"} {
		if _, ok := raw[key]; ok {
			return fmt.Errorf("mobile profile must not contain %s", key)
		}
	}
	return nil
}

func profileToConfig(profile profileFile, certPEM []byte) (ImportedProfile, error) {
	if profile.Version == 0 {
		profile.Version = 1
	}
	if profile.Version != 1 {
		return ImportedProfile{}, fmt.Errorf("unsupported profile version: %d", profile.Version)
	}
	if _, _, err := splitHostPort(profile.ServerAddr); err != nil {
		return ImportedProfile{}, fmt.Errorf("server_addr is invalid: %w", err)
	}
	if strings.TrimSpace(profile.Username) == "" {
		return ImportedProfile{}, errors.New("username is required")
	}
	expiresAt, err := credentialExpiry(tunnel.Config{SavedCredential: profile.SavedCredential})
	if err != nil {
		return ImportedProfile{}, err
	}
	if profile.TLS.InsecureSkipVerify {
		return ImportedProfile{}, errors.New("mobile profile cannot use tls.insecure_skip_verify")
	}
	minVersion := strings.TrimSpace(profile.TLS.MinVersion)
	if minVersion == "" {
		minVersion = "1.3"
	}
	if !tls13(minVersion) {
		return ImportedProfile{}, errors.New("mobile profile requires tls.min_version 1.3")
	}
	if _, _, err := readFirstCert(certPEM); err != nil {
		return ImportedProfile{}, fmt.Errorf("server certificate is not valid PEM: %w", err)
	}

	forwards := make([]tunnel.ForwardConfig, 0, len(profile.Forwards))
	seenNames := map[string]bool{}
	seenListens := map[string]bool{}
	for _, fwd := range profile.Forwards {
		cfgFwd, err := configForwardFromProfile(fwd)
		if err != nil {
			return ImportedProfile{}, err
		}
		if seenNames[cfgFwd.Name] {
			return ImportedProfile{}, fmt.Errorf("duplicate forward name: %s", cfgFwd.Name)
		}
		if seenListens[cfgFwd.ListenAddr] {
			return ImportedProfile{}, fmt.Errorf("duplicate mobile listen address: %s", cfgFwd.ListenAddr)
		}
		seenNames[cfgFwd.Name] = true
		seenListens[cfgFwd.ListenAddr] = true
		forwards = append(forwards, cfgFwd)
	}
	if len(forwards) == 0 {
		return ImportedProfile{}, errors.New("at least one client_to_server forward is required")
	}

	timeout := profile.Connection.DialTimeoutSec
	if timeout <= 0 {
		timeout = 5
	}
	caCertFile := strings.TrimSpace(profile.TLS.CACertFile)
	if caCertFile == "" {
		caCertFile = serverCertEntry
	}
	cfg := tunnel.Config{
		ServerAddr: strings.TrimSpace(profile.ServerAddr),
		Username:   strings.TrimSpace(profile.Username),
		ClientID:   strings.TrimSpace(profile.ClientID),
		LogLevel:   "info",
		TLS: tunnel.TLSConfig{
			CACertFile:         caCertFile,
			ServerName:         strings.TrimSpace(profile.TLS.ServerName),
			MinVersion:         "1.3",
			InsecureSkipVerify: false,
		},
		Connection:      tunnel.ConnectionConfig{DialTimeoutSec: timeout},
		SavedCredential: profile.SavedCredential,
		Forwards:        forwards,
	}
	tunnel.ApplyDefaults(&cfg)
	return ImportedProfile{
		Config:    cfg,
		CertPEM:   append([]byte(nil), certPEM...),
		ExpiresAt: expiresAt,
		ProfileID: strings.TrimSpace(profile.ProfileID),
	}, nil
}

func configForwardFromProfile(fwd forwardConfig) (tunnel.ForwardConfig, error) {
	direction := strings.TrimSpace(fwd.Direction)
	if direction == "" {
		direction = tunnel.DirectionClientToServer
	}
	if direction != tunnel.DirectionClientToServer {
		return tunnel.ForwardConfig{}, fmt.Errorf("forward %q is %s; mobile profile only supports client_to_server", profileForwardNameForError(fwd), direction)
	}
	host, port, err := splitHostPort(fwd.ListenAddr)
	if err != nil {
		return tunnel.ForwardConfig{}, fmt.Errorf("forward %q listen_addr is invalid: %w", profileForwardNameForError(fwd), err)
	}
	if normalizeProfileHost(host) != "127.0.0.1" {
		return tunnel.ForwardConfig{}, fmt.Errorf("forward %q listen_addr must use 127.0.0.1 for mobile", profileForwardNameForError(fwd))
	}
	if port < 1024 {
		return tunnel.ForwardConfig{}, fmt.Errorf("forward %q listen port %d is below 1024 and cannot be used by Android", profileForwardNameForError(fwd), port)
	}
	if _, _, err := splitHostPort(fwd.ServerTarget); err != nil {
		return tunnel.ForwardConfig{}, fmt.Errorf("forward %q server_target is invalid: %w", profileForwardNameForError(fwd), err)
	}
	listenAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	name := strings.TrimSpace(fwd.Name)
	if name == "" {
		name = listenAddr
	}
	return tunnel.ForwardConfig{
		Name:         name,
		Direction:    tunnel.DirectionClientToServer,
		ListenAddr:   listenAddr,
		ServerTarget: strings.TrimSpace(fwd.ServerTarget),
	}, nil
}

func normalizeProfileHost(host string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
}

func profileForwardNameForError(fwd forwardConfig) string {
	if name := strings.TrimSpace(fwd.Name); name != "" {
		return name
	}
	if addr := strings.TrimSpace(fwd.ListenAddr); addr != "" {
		return addr
	}
	return strings.TrimSpace(fwd.ServerTarget)
}
