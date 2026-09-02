package main

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"lsyltunnel/src/client/tunnel"
)

func printConfigSummary(out io.Writer, configFile string) {
	fmt.Fprintf(out, "Config file: %s\n", configFile)
	cfg, err := tunnel.LoadConfigRaw(configFile)
	if err != nil {
		fmt.Fprintf(out, "Config:      not readable: %v\n", err)
		return
	}
	fmt.Fprintf(out, "Server:      %s\n", cfg.ServerAddr)
	fmt.Fprintf(out, "Username:    %s\n", cfg.Username)
	fmt.Fprintf(out, "TLS name:    %s\n", cfg.TLS.ServerName)
	fmt.Fprintf(out, "Credential:  %s\n", credentialState(cfg))
	fmt.Fprintf(out, "Forwards:    %d\n", len(cfg.Forwards))
	for _, fwd := range cfg.Forwards {
		name := strings.TrimSpace(fwd.Name)
		if name == "" {
			name = "forward"
		}
		fmt.Fprintf(out, "  - %s %s %s -> %s\n", name, fwd.Direction, fwd.ListenAddr, fwd.ServerTarget)
	}
}

func credentialState(cfg tunnel.Config) string {
	switch {
	case strings.TrimSpace(cfg.SavedCredential.Ciphertext) != "":
		return "saved_credential"
	case strings.TrimSpace(cfg.PasswordFile) != "":
		return "password_file"
	case strings.TrimSpace(cfg.PasswordEnv) != "":
		return "password_env"
	case cfg.Password != "":
		return "inline_password"
	default:
		return "none"
	}
}

func printCertSummary(out io.Writer, certFile string) {
	fmt.Fprintf(out, "Cert file:   %s\n", certFile)
	cert, raw, err := readFirstCert(certFile)
	if err != nil {
		fmt.Fprintf(out, "Cert:        not readable: %v\n", err)
		return
	}
	fp := sha256.Sum256(raw)
	fmt.Fprintf(out, "Subject:     %s\n", cert.Subject.String())
	fmt.Fprintf(out, "Not before:  %s\n", cert.NotBefore.Format(time.RFC3339))
	fmt.Fprintf(out, "Not after:   %s\n", cert.NotAfter.Format(time.RFC3339))
	fmt.Fprintf(out, "DNS names:   %s\n", strings.Join(cert.DNSNames, ","))
	ips := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		ips = append(ips, ip.String())
	}
	fmt.Fprintf(out, "IP addrs:    %s\n", strings.Join(ips, ","))
	fmt.Fprintf(out, "SHA256:      %s\n", strings.ToUpper(hex.EncodeToString(fp[:])))
}

func readFirstCert(path string) (*x509.Certificate, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func samePath(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err == nil {
		a = aa
	}
	bb, err := filepath.Abs(b)
	if err == nil {
		b = bb
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func ensureChild(root, child string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	childAbs, err := filepath.Abs(child)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, childAbs)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes root: %s", child)
	}
	return nil
}

func nextBackupPath(path string) (string, error) {
	base := filepath.Clean(path) + ".profile-backup-" + time.Now().Format("20060102-150405")
	for i := 0; i < 100; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%02d", base, i)
		}
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("cannot allocate backup path for %s", path)
}
