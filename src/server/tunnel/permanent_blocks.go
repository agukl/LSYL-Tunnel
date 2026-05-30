package tunnel

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func loadPermanentBlockedIPs(path string) (map[string]struct{}, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	blocked := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ip := strings.TrimSpace(scanner.Text())
		if ip == "" || strings.HasPrefix(ip, "#") {
			continue
		}
		blocked[ip] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return blocked, nil
}

func appendPermanentBlockedIP(path, ip string) error {
	path = strings.TrimSpace(path)
	ip = strings.TrimSpace(ip)
	if path == "" || ip == "" {
		return nil
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(ip + "\n")
	return err
}
