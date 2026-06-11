//go:build windows

package gui

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"lsyltunnel/src/server/tunnel"
)

func (a *App) loadBusinessLogs(cfg tunnel.Config, max int) []tunnel.BusinessLogEntry {
	if max <= 0 {
		max = 120
	}
	business := a.readBusinessLogEntries(cfg, max*4)
	requests := a.readRequestLogEntries(cfg, max*8)
	return buildBusinessFlow(business, requests, max)
}

func (a *App) readBusinessLogEntries(cfg tunnel.Config, max int) []tunnel.BusinessLogEntry {
	path := a.runtimeLogPath(cfg.Runtime.BusinessLogFile, filepath.Join("business", "business.jsonl"))
	return readJSONLRecent(path, max, func(line []byte) (tunnel.BusinessLogEntry, bool) {
		var entry tunnel.BusinessLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return entry, false
		}
		return entry, true
	})
}

func (a *App) loadRequestLogs(cfg tunnel.Config, max int) []tunnel.RequestLogEntry {
	return a.readRequestLogEntries(cfg, max)
}

func (a *App) readRequestLogEntries(cfg tunnel.Config, max int) []tunnel.RequestLogEntry {
	path := a.runtimeLogPath(cfg.Runtime.RequestLogFile, filepath.Join("request", "request.jsonl"))
	return readJSONLRecent(path, max, func(line []byte) (tunnel.RequestLogEntry, bool) {
		var entry tunnel.RequestLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return entry, false
		}
		return entry, true
	})
}

func (a *App) runtimeLogPath(configured, defaultName string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return a.defaultRuntimePath("logs", defaultName)
	}
	if filepath.IsAbs(configured) {
		return configured
	}
	return filepath.Clean(filepath.Join(filepath.Dir(a.configPath), configured))
}

func readJSONLRecent[T any](basePath string, max int, decode func([]byte) (T, bool)) []T {
	if max <= 0 {
		max = 120
	}
	out := make([]T, 0, max)
	for _, candidate := range jsonlRecentCandidates(basePath) {
		if len(out) >= max {
			break
		}
		lines, err := recentFileLines(candidate, max)
		if err != nil {
			continue
		}
		for i := len(lines) - 1; i >= 0 && len(out) < max; i-- {
			entry, ok := decode([]byte(lines[i]))
			if ok {
				out = append(out, entry)
			}
		}
	}
	return out
}

func jsonlRecentCandidates(basePath string) []string {
	basePath = filepath.Clean(basePath)
	ext := filepath.Ext(basePath)
	name := strings.TrimSuffix(filepath.Base(basePath), ext)
	if ext == "" {
		ext = ".jsonl"
	}
	pattern := filepath.Join(filepath.Dir(basePath), name+"-*"+ext)
	matches, _ := filepath.Glob(pattern)
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	if fileExists(basePath) {
		matches = append(matches, basePath)
	}
	return matches
}

func recentFileLines(path string, max int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size <= 0 {
		return nil, nil
	}

	const initialWindow int64 = 64 * 1024
	const maxWindow int64 = 4 * 1024 * 1024
	window := initialWindow
	if window > size {
		window = size
	}

	for {
		start := size - window
		buf := make([]byte, int(window))
		n, err := f.ReadAt(buf, start)
		if err != nil && err != io.EOF {
			return nil, err
		}
		lines := lastLines(buf[:n], max)
		if len(lines) >= max || start == 0 || window >= maxWindow {
			return lines, nil
		}
		window *= 2
		if window > maxWindow {
			window = maxWindow
		}
		if window > size {
			window = size
		}
	}
}

type businessFlowSet struct {
	rows               []tunnel.BusinessLogEntry
	lifecycleIndex     map[string]int
	businessRequestIDs map[string]bool
	seenRows           map[string]bool
	seenRequests       map[string]bool
}

func buildBusinessFlow(events []tunnel.BusinessLogEntry, requests []tunnel.RequestLogEntry, max int) []tunnel.BusinessLogEntry {
	if max <= 0 {
		max = 120
	}
	set := businessFlowSet{
		lifecycleIndex:     map[string]int{},
		businessRequestIDs: map[string]bool{},
		seenRows:           map[string]bool{},
		seenRequests:       map[string]bool{},
	}
	for _, event := range events {
		set.addBusinessEvent(event)
	}
	for _, request := range requests {
		set.addRequestLog(request)
	}
	sort.SliceStable(set.rows, func(i, j int) bool {
		return businessTimeAfter(set.rows[i].Time, set.rows[j].Time)
	})
	if len(set.rows) > max {
		set.rows = set.rows[:max]
	}
	return set.rows
}

func (s *businessFlowSet) addBusinessEvent(entry tunnel.BusinessLogEntry) {
	entry = prepareBusinessFlowEntry(entry)
	if strings.TrimSpace(entry.Kind) == "" {
		return
	}
	if entry.RequestID != "" {
		s.businessRequestIDs[entry.RequestID] = true
	}
	if key := businessLifecycleKey(entry); key != "" {
		if idx, ok := s.lifecycleIndex[key]; ok {
			mergeBusinessLifecycle(&s.rows[idx], entry)
			return
		}
		s.lifecycleIndex[key] = len(s.rows)
		s.rows = append(s.rows, entry)
		return
	}
	key := businessRowKey(entry)
	if s.seenRows[key] {
		return
	}
	s.seenRows[key] = true
	s.rows = append(s.rows, entry)
}

func (s *businessFlowSet) addRequestLog(entry tunnel.RequestLogEntry) {
	if strings.TrimSpace(entry.Request.Type) == "health" {
		return
	}
	if entry.RequestID != "" {
		if s.seenRequests[entry.RequestID] || s.businessRequestIDs[entry.RequestID] {
			return
		}
	}
	flow, ok := businessFlowFromRequest(entry)
	if !ok {
		return
	}
	if entry.RequestID != "" {
		s.seenRequests[entry.RequestID] = true
	}
	s.addBusinessEvent(flow)
}

func businessFlowFromRequest(entry tunnel.RequestLogEntry) (tunnel.BusinessLogEntry, bool) {
	req := entry.Request
	kind := businessKindFromRequestType(req.Type)
	if kind == "health" {
		return tunnel.BusinessLogEntry{}, false
	}
	result := businessResultFromRequest(entry, kind)
	if kind == "request" && result == "ok" {
		return tunnel.BusinessLogEntry{}, false
	}
	if kind == "login" && result != "ok" {
		kind = "auth"
	}
	code := strings.TrimSpace(entry.Response.Code)
	message := strings.TrimSpace(entry.Response.Message)
	if strings.TrimSpace(entry.ReadError) != "" {
		if message == "" {
			message = entry.ReadError
		} else {
			message += " / " + entry.ReadError
		}
	}
	return tunnel.BusinessLogEntry{
		Time:        entry.Time,
		StartedAt:   entry.Time,
		RequestID:   entry.RequestID,
		Kind:        kind,
		Result:      result,
		RemoteIP:    requestRemoteIP(entry),
		Username:    req.Username,
		ClientID:    req.ClientID,
		ForwardName: req.ForwardName,
		Direction:   req.Direction,
		Target:      req.Target,
		ListenAddr:  req.ListenAddr,
		Code:        code,
		Message:     message,
		DurationMS:  entry.DurationMS,
	}, true
}

func businessKindFromRequestType(kind string) string {
	switch normalized := normalizeBusinessKind(kind); normalized {
	case "":
		return "request"
	case "health":
		return "health"
	case "login", "open", "reverse_listen", "reverse_stream", "forward_check":
		return normalized
	default:
		return "request"
	}
}

func prepareBusinessFlowEntry(entry tunnel.BusinessLogEntry) tunnel.BusinessLogEntry {
	entry.Kind = normalizeBusinessKind(entry.Kind)
	entry.Result = normalizeBusinessResult(entry.Result)
	if !isBusinessLifecycleKind(entry.Kind) {
		return entry
	}
	switch entry.Result {
	case "connected", "activated", "ok":
		if entry.StartedAt == "" {
			entry.StartedAt = entry.Time
		}
	case "closed", "failed", "denied", "blocked", "error":
		if entry.EndedAt == "" {
			entry.EndedAt = entry.Time
		}
		if entry.StartedAt == "" && entry.DurationMS > 0 {
			entry.StartedAt = businessStartFromDuration(entry.EndedAt, entry.DurationMS)
		}
	default:
		if entry.StartedAt == "" {
			entry.StartedAt = entry.Time
		}
	}
	return entry
}

func businessResultFromRequest(entry tunnel.RequestLogEntry, kind string) string {
	result := normalizeBusinessResult(entry.Result)
	code := strings.TrimSpace(entry.Response.Code)
	if result == "blocked" || strings.Contains(code, "blocked") {
		return "blocked"
	}
	if result == "denied" || strings.Contains(code, "denied") {
		return "denied"
	}
	if result == "failed" || result == "error" || entry.ReadError != "" || (!entry.Response.OK && code != "") {
		return "failed"
	}
	if result == "ok" || entry.Response.OK {
		switch kind {
		case "open", "reverse_stream":
			return "connected"
		case "reverse_listen":
			return "activated"
		default:
			return "ok"
		}
	}
	if result != "" {
		return result
	}
	return "failed"
}

func normalizeBusinessKind(kind string) string {
	kind = strings.TrimSpace(kind)
	switch kind {
	case "reverse":
		return "reverse_listen"
	default:
		return kind
	}
}

func normalizeBusinessResult(result string) string {
	return strings.TrimSpace(result)
}

func businessLifecycleKey(entry tunnel.BusinessLogEntry) string {
	if entry.RequestID == "" || !isBusinessLifecycleKind(entry.Kind) {
		return ""
	}
	return entry.RequestID + "\x00" + entry.Kind
}

func isBusinessLifecycleKind(kind string) bool {
	switch kind {
	case "open", "reverse_stream", "reverse_listen":
		return true
	default:
		return false
	}
}

func mergeBusinessLifecycle(dst *tunnel.BusinessLogEntry, src tunnel.BusinessLogEntry) {
	srcPriority := businessResultPriority(src.Result)
	dstPriority := businessResultPriority(dst.Result)
	if srcPriority > dstPriority || (srcPriority == dstPriority && businessTimeAfter(src.Time, dst.Time)) {
		prev := *dst
		*dst = src
		carryBusinessFields(dst, prev)
		return
	}
	carryBusinessFields(dst, src)
}

func carryBusinessFields(dst *tunnel.BusinessLogEntry, src tunnel.BusinessLogEntry) {
	if dst.Time == "" {
		dst.Time = src.Time
	}
	if dst.StartedAt == "" || (src.StartedAt != "" && businessTimeBefore(src.StartedAt, dst.StartedAt)) {
		dst.StartedAt = src.StartedAt
	}
	if dst.EndedAt == "" || (src.EndedAt != "" && businessTimeAfter(src.EndedAt, dst.EndedAt)) {
		dst.EndedAt = src.EndedAt
	}
	if dst.RequestID == "" {
		dst.RequestID = src.RequestID
	}
	if dst.Kind == "" {
		dst.Kind = src.Kind
	}
	if dst.Result == "" {
		dst.Result = src.Result
	}
	if dst.RemoteIP == "" {
		dst.RemoteIP = src.RemoteIP
	}
	if dst.Username == "" {
		dst.Username = src.Username
	}
	if dst.ClientID == "" {
		dst.ClientID = src.ClientID
	}
	if dst.ForwardName == "" {
		dst.ForwardName = src.ForwardName
	}
	if dst.Direction == "" {
		dst.Direction = src.Direction
	}
	if dst.Target == "" {
		dst.Target = src.Target
	}
	if dst.ListenAddr == "" {
		dst.ListenAddr = src.ListenAddr
	}
	if dst.Code == "" {
		dst.Code = src.Code
	}
	if dst.Message == "" {
		dst.Message = src.Message
	}
	if dst.DurationMS == 0 {
		dst.DurationMS = src.DurationMS
	}
	if dst.BytesUp == 0 {
		dst.BytesUp = src.BytesUp
	}
	if dst.BytesDown == 0 {
		dst.BytesDown = src.BytesDown
	}
}

func businessResultPriority(result string) int {
	switch result {
	case "closed", "failed", "denied", "blocked", "error":
		return 4
	case "connected", "activated":
		return 3
	case "ok":
		return 2
	default:
		if result == "" {
			return 0
		}
		return 1
	}
}

func businessRowKey(entry tunnel.BusinessLogEntry) string {
	if entry.RequestID != "" {
		return entry.RequestID + "\x00" + entry.Kind + "\x00" + entry.Result
	}
	return entry.Time + "\x00" + entry.Kind + "\x00" + entry.Result + "\x00" + entry.RemoteIP + "\x00" + entry.Code + "\x00" + entry.Message
}

func businessTimeAfter(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	if errA == nil && errB == nil {
		return ta.After(tb)
	}
	return a > b
}

func businessTimeBefore(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	if errA == nil && errB == nil {
		return ta.Before(tb)
	}
	return a < b
}

func businessStartFromDuration(end string, durationMS int64) string {
	if end == "" || durationMS <= 0 {
		return ""
	}
	t, err := time.Parse(time.RFC3339, end)
	if err != nil {
		return ""
	}
	return t.Add(-time.Duration(durationMS) * time.Millisecond).Format(time.RFC3339)
}

func requestRemoteIP(entry tunnel.RequestLogEntry) string {
	if strings.TrimSpace(entry.RemoteIP) != "" {
		return entry.RemoteIP
	}
	host, _, err := net.SplitHostPort(entry.RemoteAddr)
	if err == nil {
		return host
	}
	return entry.RemoteAddr
}
