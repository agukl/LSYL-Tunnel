//go:build windows

package gui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lsyltunnel/src/server/tunnel"
)

func readBusinessLogsInRange(basePath string, start, end time.Time, visit func(tunnel.BusinessLogEntry, time.Time)) (int, error) {
	total := 0
	err := readJSONLRange(basePath, start, end, func(line []byte, t time.Time) {
		var entry tunnel.BusinessLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return
		}
		total++
		visit(entry, t)
	})
	return total, err
}

func readRequestLogsInRange(basePath string, start, end time.Time, visit func(tunnel.RequestLogEntry, time.Time)) (int, error) {
	total := 0
	err := readJSONLRange(basePath, start, end, func(line []byte, t time.Time) {
		var entry tunnel.RequestLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return
		}
		total++
		visit(entry, t)
	})
	return total, err
}

func readJSONLRange(basePath string, start, end time.Time, visit func([]byte, time.Time)) error {
	for _, path := range jsonlRangeCandidates(basePath, start, end) {
		if err := readJSONLRangeFile(path, start, end, visit); err != nil {
			return err
		}
	}
	return nil
}

func readJSONLRangeFile(path string, start, end time.Time, visit func([]byte, time.Time)) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var stamp struct {
			Time string `json:"time"`
		}
		if err := json.Unmarshal(line, &stamp); err != nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, stamp.Time)
		if err != nil {
			continue
		}
		if t.Before(start) || t.After(end) {
			continue
		}
		copied := append([]byte(nil), line...)
		visit(copied, t)
	}
	return scanner.Err()
}

func jsonlRangeCandidates(basePath string, start, end time.Time) []string {
	basePath = filepath.Clean(basePath)
	ext := filepath.Ext(basePath)
	name := strings.TrimSuffix(filepath.Base(basePath), ext)
	if ext == "" {
		ext = ".jsonl"
	}
	dir := filepath.Dir(basePath)
	out := []string{}
	seen := map[string]bool{}
	for day := midnight(start); !day.After(midnight(end)); day = day.AddDate(0, 0, 1) {
		path := filepath.Join(dir, name+"-"+day.Format("2006-01-02")+ext)
		if fileExists(path) && !seen[path] {
			out = append(out, path)
			seen[path] = true
		}
	}
	if fileExists(basePath) && !seen[basePath] {
		out = append(out, basePath)
	}
	return out
}

func midnight(t time.Time) time.Time {
	y, m, d := t.In(time.Local).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}
