package tunnel

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	successfulIPHistoryDays         = 7
	successfulIPHistoryMaxLineBytes = 1024 * 1024
)

func isSuccessfulRequestForBlockProtection(entry RequestLogEntry) bool {
	if entry.Result != "ok" || entry.AuthResult != "ok" || !entry.Response.OK {
		return false
	}
	if _, ok := canonicalSuccessfulIP(entry.RemoteIP); !ok {
		return false
	}
	switch entry.Request.Type {
	case "login", "health", "open", "forward_check", "reverse", "reverse_listen", "reverse_stream":
		return true
	default:
		return false
	}
}

func scanRecentSuccessfulRequestIPs(basePath string, now time.Time, days int, visit func(ip string, occurredAt time.Time)) error {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" || days <= 0 || visit == nil {
		return nil
	}
	basePath = filepath.Clean(basePath)
	today := localMidnight(now)
	start := today.AddDate(0, 0, -(days - 1))
	end := today.AddDate(0, 0, 1)

	candidates := make([]string, 0, days+1)
	for offset := 0; offset < days; offset++ {
		date := today.AddDate(0, 0, -offset).Format("2006-01-02")
		candidates = append(candidates, datedJSONLPath(basePath, date))
	}
	candidates = append(candidates, basePath)

	seen := make(map[string]struct{}, len(candidates))
	var scanErr error
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if err := scanSuccessfulRequestFile(candidate, start, end, visit); err != nil {
			scanErr = errors.Join(scanErr, fmt.Errorf("scan successful request history %s: %w", candidate, err))
		}
	}
	return scanErr
}

func scanSuccessfulRequestFile(path string, start, end time.Time, visit func(string, time.Time)) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), successfulIPHistoryMaxLineBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry RequestLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil || !isSuccessfulRequestForBlockProtection(entry) {
			continue
		}
		occurredAt, err := time.Parse(time.RFC3339, entry.Time)
		if err != nil {
			continue
		}
		occurredAt = occurredAt.In(time.Local)
		if occurredAt.Before(start) || !occurredAt.Before(end) {
			continue
		}
		ip, _ := canonicalSuccessfulIP(entry.RemoteIP)
		visit(ip, occurredAt)
	}
	return scanner.Err()
}

func localMidnight(value time.Time) time.Time {
	year, month, day := value.In(time.Local).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func canonicalSuccessfulIP(value string) (string, bool) {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return "", false
	}
	return ip.String(), true
}

func (f *failTracker) markSuccessful(key string) bool {
	if f == nil {
		return false
	}
	key, ok := canonicalSuccessfulIP(key)
	if !ok {
		return false
	}
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rotateSuccessfulIPsLocked(now)
	if _, exists := f.successfulIPs[key]; !exists {
		if len(f.successfulIPs) >= f.maxItems {
			return false
		}
		f.successfulIPs[key] = struct{}{}
	}
	if state := f.items[key]; state != nil {
		state.protocolFailures = nil
	}
	return true
}

func (f *failTracker) successfulTodayLocked(key string, now time.Time) bool {
	f.rotateSuccessfulIPsLocked(now)
	_, ok := f.successfulIPs[key]
	return ok
}

func (f *failTracker) rotateSuccessfulIPsLocked(now time.Time) {
	date := now.In(time.Local).Format("2006-01-02")
	if f.successfulDate == date && f.successfulIPs != nil {
		return
	}
	f.successfulDate = date
	f.successfulIPs = map[string]struct{}{}
}

func reconcileSuccessfulIPHistory(fails *failTracker, requestLogPath string, now time.Time) error {
	if fails == nil {
		return nil
	}
	today := localMidnight(now)
	recentPermanent := map[string]struct{}{}
	scanErr := scanRecentSuccessfulRequestIPs(requestLogPath, now, successfulIPHistoryDays, func(ip string, occurredAt time.Time) {
		if !occurredAt.Before(today) {
			fails.markSuccessful(ip)
		}
		if fails.hasPermanent(ip) {
			recentPermanent[ip] = struct{}{}
		}
	})
	_, removeErr := fails.removePermanentSuccessful(recentPermanent)
	return errors.Join(scanErr, removeErr)
}

func (f *failTracker) removePermanentSuccessful(ips map[string]struct{}) (int, error) {
	if f == nil || len(ips) == 0 {
		return 0, nil
	}
	removed, err := removePermanentBlockedIPs(f.permanentFile, ips)
	if err != nil {
		return 0, err
	}
	for ip := range removed {
		f.permanent.Delete(ip)
	}
	return len(removed), nil
}
