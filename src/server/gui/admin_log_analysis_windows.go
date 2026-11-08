//go:build windows

package gui

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"lsyltunnel/src/server/tunnel"
)

const logAnalysisDefaultDays = 7

type logAnalysisResult struct {
	OK             bool                  `json:"ok"`
	Message        string                `json:"message,omitempty"`
	Start          string                `json:"start"`
	End            string                `json:"end"`
	Paths          logAnalysisPaths      `json:"paths"`
	Summary        logAnalysisSummary    `json:"summary"`
	SuccessSources []logAnalysisIPSource `json:"success_sources"`
	BlockedSources []logAnalysisIPSource `json:"blocked_sources"`
	GeoLookupOK    bool                  `json:"geo_lookup_ok"`
	GeoLookupError string                `json:"geo_lookup_error,omitempty"`
}

type logAnalysisPaths struct {
	BusinessLog string `json:"business_log"`
	RequestLog  string `json:"request_log"`
}

type logAnalysisSummary struct {
	BusinessEvents int `json:"business_events"`
	RequestEvents  int `json:"request_events"`
	SuccessIPs     int `json:"success_ips"`
	BlockedIPs     int `json:"blocked_ips"`
}

type logAnalysisIPSource struct {
	IP                     string   `json:"ip"`
	Location               string   `json:"location,omitempty"`
	Network                string   `json:"network,omitempty"`
	NetworkType            string   `json:"network_type,omitempty"`
	Country                string   `json:"country,omitempty"`
	Region                 string   `json:"region,omitempty"`
	City                   string   `json:"city,omitempty"`
	ISP                    string   `json:"isp,omitempty"`
	Org                    string   `json:"org,omitempty"`
	AS                     string   `json:"as,omitempty"`
	ASName                 string   `json:"as_name,omitempty"`
	Hosting                bool     `json:"hosting,omitempty"`
	Proxy                  bool     `json:"proxy,omitempty"`
	Mobile                 bool     `json:"mobile,omitempty"`
	BusinessEvents         int      `json:"business_events"`
	Requests               int      `json:"requests"`
	FailedRequests         int      `json:"failed_requests"`
	SuccessEvents          int      `json:"success_events"`
	BlockedEvents          int      `json:"blocked_events"`
	AuthBlockedEvents      int      `json:"auth_blocked_events"`
	PermanentBlockedEvents int      `json:"permanent_blocked_events"`
	BytesUp                int64    `json:"bytes_up"`
	BytesDown              int64    `json:"bytes_down"`
	DurationMS             int64    `json:"duration_ms"`
	FirstSeen              string   `json:"first_seen,omitempty"`
	LastSeen               string   `json:"last_seen,omitempty"`
	Users                  []string `json:"users,omitempty"`
	Clients                []string `json:"clients,omitempty"`
	Targets                []string `json:"targets,omitempty"`
	Codes                  []string `json:"codes,omitempty"`
	Messages               []string `json:"messages,omitempty"`

	seen map[string]bool `json:"-"`
}

func (a *App) handleAdminLogAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, logAnalysisResult{OK: false, Message: "method not allowed"})
		return
	}
	cfg, err := readRawServerConfig(a.configPath)
	if err != nil {
		writeJSON(w, http.StatusOK, logAnalysisResult{OK: false, Message: friendlyError(err)})
		return
	}
	start, end, err := parseLogAnalysisWindow(r.URL.Query().Get("start"), r.URL.Query().Get("end"))
	if err != nil {
		writeJSON(w, http.StatusOK, logAnalysisResult{OK: false, Message: err.Error()})
		return
	}
	lookupGeo := strings.TrimSpace(r.URL.Query().Get("geo")) != "0"
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	result, err := a.buildLogAnalysis(ctx, cfg, start, end, lookupGeo)
	if err != nil {
		writeJSON(w, http.StatusOK, logAnalysisResult{OK: false, Message: friendlyError(err)})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseLogAnalysisWindow(startText, endText string) (time.Time, time.Time, error) {
	now := time.Now()
	end := now
	start := now.AddDate(0, 0, -logAnalysisDefaultDays)
	var err error
	if strings.TrimSpace(startText) != "" {
		start, err = parseLogAnalysisTime(startText, false)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("开始时间格式不正确")
		}
	}
	if strings.TrimSpace(endText) != "" {
		end, err = parseLogAnalysisTime(endText, true)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("结束时间格式不正确")
		}
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("结束时间不能早于开始时间")
	}
	return start, end, nil
}

func parseLogAnalysisTime(text string, endOfDay bool) (time.Time, error) {
	text = strings.TrimSpace(text)
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		var t time.Time
		var err error
		if layout == time.RFC3339 || layout == time.RFC3339Nano {
			t, err = time.Parse(layout, text)
		} else {
			t, err = time.ParseInLocation(layout, text, time.Local)
		}
		if err == nil {
			if layout == "2006-01-02" && endOfDay {
				t = t.Add(24*time.Hour - time.Nanosecond)
			}
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

func (a *App) buildLogAnalysis(ctx context.Context, cfg tunnel.Config, start, end time.Time, lookupGeo bool) (*logAnalysisResult, error) {
	businessPath := a.runtimeLogPath(cfg.Runtime.BusinessLogFile, filepath.Join("business", "business.jsonl"))
	requestPath := a.runtimeLogPath(cfg.Runtime.RequestLogFile, filepath.Join("request", "request.jsonl"))
	result := &logAnalysisResult{
		OK:      true,
		Message: "日志分析完成",
		Start:   start.Format(time.RFC3339),
		End:     end.Format(time.RFC3339),
		Paths: logAnalysisPaths{
			BusinessLog: businessPath,
			RequestLog:  requestPath,
		},
	}

	success := map[string]*logAnalysisIPSource{}
	blocked := map[string]*logAnalysisIPSource{}
	seenSuccess := map[string]bool{}
	seenBlocked := map[string]bool{}

	businessTotal, err := readBusinessLogsInRange(businessPath, start, end, func(entry tunnel.BusinessLogEntry, t time.Time) {
		ip := strings.TrimSpace(entry.RemoteIP)
		if net.ParseIP(ip) == nil {
			return
		}
		if isSuccessfulBusinessLog(entry) {
			src := logAnalysisSource(success, ip)
			recordAnalysisBusiness(src, entry, t, "success", seenSuccess)
		}
		if isBlockedBusinessLog(entry) {
			src := logAnalysisSource(blocked, ip)
			recordAnalysisBusiness(src, entry, t, "blocked", seenBlocked)
		}
		if entry.Kind == "open" && entry.Result == "closed" {
			if src := success[ip]; src != nil {
				src.BytesUp += entry.BytesUp
				src.BytesDown += entry.BytesDown
				src.DurationMS += entry.DurationMS
				updateAnalysisTime(src, t)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	result.Summary.BusinessEvents = businessTotal

	requestTotal, err := readRequestLogsInRange(requestPath, start, end, func(entry tunnel.RequestLogEntry, t time.Time) {
		ip := strings.TrimSpace(entry.RemoteIP)
		if net.ParseIP(ip) == nil {
			return
		}
		if isSuccessfulRequestLog(entry) && !seenSuccess[analysisEventKey(entry.RequestID, "request", t)] {
			src := logAnalysisSource(success, ip)
			recordAnalysisRequest(src, entry, t, "success", seenSuccess)
		} else if src := success[ip]; src != nil {
			recordAnalysisRequestMetrics(src, entry, t)
		}
		if isBlockedRequestLog(entry) && !seenBlocked[analysisEventKey(entry.RequestID, "request", t)] {
			src := logAnalysisSource(blocked, ip)
			recordAnalysisRequest(src, entry, t, "blocked", seenBlocked)
		} else if src := blocked[ip]; src != nil {
			recordAnalysisRequestMetrics(src, entry, t)
		}
	})
	if err != nil {
		return nil, err
	}
	result.Summary.RequestEvents = requestTotal

	result.SuccessSources = analysisSourcesSorted(success, func(a, b logAnalysisIPSource) bool {
		if a.SuccessEvents != b.SuccessEvents {
			return a.SuccessEvents > b.SuccessEvents
		}
		return a.Requests > b.Requests
	})
	result.BlockedSources = analysisSourcesSorted(blocked, func(a, b logAnalysisIPSource) bool {
		if a.BlockedEvents != b.BlockedEvents {
			return a.BlockedEvents > b.BlockedEvents
		}
		return a.FailedRequests > b.FailedRequests
	})
	result.Summary.SuccessIPs = len(result.SuccessSources)
	result.Summary.BlockedIPs = len(result.BlockedSources)

	if lookupGeo {
		ips := append(analysisIPs(result.SuccessSources), analysisIPs(result.BlockedSources)...)
		geo, geoErr := lookupIPAPIBatch(ctx, uniqueStrings(ips))
		if geoErr != nil {
			result.GeoLookupError = geoErr.Error()
		} else {
			result.GeoLookupOK = true
			attachAnalysisGeo(result.SuccessSources, geo)
			attachAnalysisGeo(result.BlockedSources, geo)
		}
	}
	return result, nil
}

func isSuccessfulBusinessLog(entry tunnel.BusinessLogEntry) bool {
	switch entry.Result {
	case "ok", "connected", "activated":
		return entry.Kind != "health" && strings.TrimSpace(entry.RemoteIP) != ""
	default:
		return false
	}
}

func isBlockedBusinessLog(entry tunnel.BusinessLogEntry) bool {
	result := strings.ToLower(entry.Result)
	code := strings.ToLower(entry.Code)
	return result == "blocked" || strings.Contains(code, "blocked")
}

func isSuccessfulRequestLog(entry tunnel.RequestLogEntry) bool {
	if entry.Result != "ok" || entry.AuthResult != "ok" {
		return false
	}
	switch entry.Request.Type {
	case "login", "open", "forward_check", "reverse_listen", "reverse", "reverse_stream":
		return true
	default:
		return false
	}
}

func isBlockedRequestLog(entry tunnel.RequestLogEntry) bool {
	code := strings.ToLower(entry.Response.Code)
	return strings.ToLower(entry.Result) == "blocked" ||
		strings.ToLower(entry.AuthResult) == "blocked" ||
		strings.Contains(code, "blocked")
}

func logAnalysisSource(items map[string]*logAnalysisIPSource, ip string) *logAnalysisIPSource {
	src := items[ip]
	if src == nil {
		src = &logAnalysisIPSource{IP: ip, seen: map[string]bool{}}
		items[ip] = src
	}
	if src.seen == nil {
		src.seen = map[string]bool{}
	}
	return src
}

func recordAnalysisBusiness(src *logAnalysisIPSource, entry tunnel.BusinessLogEntry, t time.Time, category string, seen map[string]bool) {
	key := analysisEventKey(entry.RequestID, "business:"+entry.Kind+":"+entry.Result+":"+entry.Code, t)
	if seen[key] {
		return
	}
	seen[key] = true
	src.BusinessEvents++
	updateAnalysisTime(src, t)
	addLimitedUnique(&src.Users, entry.Username, 8)
	addLimitedUnique(&src.Clients, entry.ClientID, 8)
	addLimitedUnique(&src.Targets, entry.ForwardName, 8)
	addLimitedUnique(&src.Targets, entry.Target, 8)
	addLimitedUnique(&src.Targets, entry.ListenAddr, 8)
	addLimitedUnique(&src.Codes, entry.Code, 8)
	addLimitedUnique(&src.Messages, entry.Message, 8)
	if category == "success" {
		src.SuccessEvents++
	}
	if category == "blocked" {
		src.BlockedEvents++
		code := strings.ToLower(entry.Code)
		if strings.Contains(code, "auth_blocked") {
			src.AuthBlockedEvents++
		}
		if strings.Contains(code, "permanently_blocked") {
			src.PermanentBlockedEvents++
		}
	}
}

func recordAnalysisRequest(src *logAnalysisIPSource, entry tunnel.RequestLogEntry, t time.Time, category string, seen map[string]bool) {
	key := analysisEventKey(entry.RequestID, "request:"+entry.Request.Type+":"+entry.Result+":"+entry.Response.Code, t)
	if seen[key] {
		recordAnalysisRequestMetrics(src, entry, t)
		return
	}
	seen[key] = true
	recordAnalysisRequestMetrics(src, entry, t)
	addLimitedUnique(&src.Users, entry.Request.Username, 8)
	addLimitedUnique(&src.Clients, entry.Request.ClientID, 8)
	addLimitedUnique(&src.Targets, entry.Request.ForwardName, 8)
	addLimitedUnique(&src.Targets, entry.Request.Target, 8)
	addLimitedUnique(&src.Targets, entry.Request.ListenAddr, 8)
	addLimitedUnique(&src.Codes, entry.Response.Code, 8)
	addLimitedUnique(&src.Messages, entry.Response.Message, 8)
	addLimitedUnique(&src.Messages, entry.ReadError, 8)
	if category == "success" {
		src.SuccessEvents++
	}
	if category == "blocked" {
		src.BlockedEvents++
		code := strings.ToLower(entry.Response.Code)
		if strings.Contains(code, "auth_blocked") {
			src.AuthBlockedEvents++
		}
		if strings.Contains(code, "permanently_blocked") {
			src.PermanentBlockedEvents++
		}
	}
}

func recordAnalysisRequestMetrics(src *logAnalysisIPSource, entry tunnel.RequestLogEntry, t time.Time) {
	src.Requests++
	if isRequestFailure(entry) {
		src.FailedRequests++
	}
	updateAnalysisTime(src, t)
}

func isRequestFailure(entry tunnel.RequestLogEntry) bool {
	result := strings.ToLower(entry.Result)
	code := strings.ToLower(entry.Response.Code)
	if result == "failed" || result == "denied" || result == "blocked" || result == "error" {
		return true
	}
	return strings.Contains(code, "failed") ||
		strings.Contains(code, "denied") ||
		strings.Contains(code, "blocked") ||
		strings.Contains(code, "bad_request") ||
		strings.Contains(code, "timeout")
}

func analysisEventKey(requestID, fallback string, t time.Time) string {
	requestID = strings.TrimSpace(requestID)
	if requestID != "" {
		return requestID
	}
	return fallback + ":" + t.Format(time.RFC3339Nano)
}

func updateAnalysisTime(src *logAnalysisIPSource, t time.Time) {
	text := t.Format(time.RFC3339)
	if src.FirstSeen == "" || text < src.FirstSeen {
		src.FirstSeen = text
	}
	if src.LastSeen == "" || text > src.LastSeen {
		src.LastSeen = text
	}
}

func addLimitedUnique(items *[]string, value string, max int) {
	value = strings.TrimSpace(value)
	if value == "" || max <= 0 {
		return
	}
	for _, item := range *items {
		if item == value {
			return
		}
	}
	if len(*items) >= max {
		return
	}
	*items = append(*items, value)
}

func analysisSourcesSorted(items map[string]*logAnalysisIPSource, less func(a, b logAnalysisIPSource) bool) []logAnalysisIPSource {
	out := make([]logAnalysisIPSource, 0, len(items))
	for _, src := range items {
		copy := *src
		copy.seen = nil
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		if less(out[i], out[j]) {
			return true
		}
		if less(out[j], out[i]) {
			return false
		}
		return out[i].IP < out[j].IP
	})
	return out
}

func analysisIPs(items []logAnalysisIPSource) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if isPublicIP(item.IP) {
			out = append(out, item.IP)
		}
	}
	return out
}
