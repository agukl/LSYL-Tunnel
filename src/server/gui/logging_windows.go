//go:build windows

package gui

import (
	"path/filepath"
	"strings"
	"time"
)

func serviceLogTail(path string, max int) []string {
	if max <= 0 {
		return nil
	}
	for _, candidate := range serviceLogTailCandidates(path) {
		lines, err := recentFileLines(candidate, max)
		if err != nil {
			continue
		}
		return lines
	}
	return nil
}

func serviceLogTailCandidates(path string) []string {
	date := time.Now().Format("2006-01-02")
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)
	daily := filepath.Join(filepath.Dir(path), name+"-"+date+ext)
	if daily == path {
		return []string{path}
	}
	return []string{daily, path}
}

func lastLines(data []byte, max int) []string {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	kept := make([]string, 0, max)
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		kept = append(kept, line)
		if len(kept) > max {
			kept = kept[len(kept)-max:]
		}
	}
	return kept
}
