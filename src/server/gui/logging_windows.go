//go:build windows

package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) appendLog(msg string) {
	line := fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), msg)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logs = append(a.logs, line)
	if len(a.logs) > 300 {
		a.logs = append([]string{}, a.logs[len(a.logs)-300:]...)
	}
}

func (a *App) snapshotLogs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	logs := append([]string{}, a.logs...)
	if tail := serviceLogTail(a.logService, 80); len(tail) > 0 {
		logs = append(logs, "---- 服务日志 ----")
		logs = append(logs, tail...)
	}
	return logs
}

func serviceLogTail(path string, max int) []string {
	for _, candidate := range serviceLogTailCandidates(path) {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		return lastLines(data, max)
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
