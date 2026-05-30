//go:build windows

package gui

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"lsyltunnel/src/server/tunnel"
)

func (a *App) loadPermanentBlockedIPs(cfg tunnel.Config) []adminPermanentBlock {
	path := a.runtimePermanentBlockPath(cfg)
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil
	}
	defer file.Close()

	out := []adminPermanentBlock{}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		ip := strings.TrimSpace(scanner.Text())
		if ip == "" || strings.HasPrefix(ip, "#") || net.ParseIP(ip) == nil || seen[ip] {
			continue
		}
		seen[ip] = true
		out = append(out, adminPermanentBlock{IP: ip, Source: path, Line: lineNo})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].IP < out[j].IP
	})
	return out
}
