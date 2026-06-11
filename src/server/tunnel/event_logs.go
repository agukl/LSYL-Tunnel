package tunnel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"lsyltunnel/src/internal/protocol"
)

type RequestLogEntry struct {
	Time       string                `json:"time"`
	RequestID  string                `json:"request_id"`
	RemoteAddr string                `json:"remote_addr,omitempty"`
	RemoteIP   string                `json:"remote_ip,omitempty"`
	LocalAddr  string                `json:"local_addr,omitempty"`
	Request    protocol.OpenRequest  `json:"request"`
	ReadError  string                `json:"read_error,omitempty"`
	AuthResult string                `json:"auth_result,omitempty"`
	Response   protocol.OpenResponse `json:"response"`
	Result     string                `json:"result"`
	DurationMS int64                 `json:"duration_ms"`
}

type BusinessLogEntry struct {
	Time        string `json:"time"`
	StartedAt   string `json:"started_at,omitempty"`
	EndedAt     string `json:"ended_at,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
	Kind        string `json:"kind"`
	Result      string `json:"result"`
	RemoteIP    string `json:"remote_ip,omitempty"`
	Username    string `json:"username,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
	ForwardName string `json:"forward_name,omitempty"`
	Direction   string `json:"direction,omitempty"`
	Target      string `json:"target,omitempty"`
	ListenAddr  string `json:"listen_addr,omitempty"`
	Code        string `json:"code,omitempty"`
	Message     string `json:"message,omitempty"`
	DurationMS  int64  `json:"duration_ms,omitempty"`
	BytesUp     int64  `json:"bytes_up,omitempty"`
	BytesDown   int64  `json:"bytes_down,omitempty"`
}

type EntryTrafficLogEntry struct {
	Time                  string `json:"time"`
	RequestID             string `json:"request_id,omitempty"`
	Event                 string `json:"event"`
	Result                string `json:"result"`
	RemoteAddr            string `json:"remote_addr,omitempty"`
	RemoteIP              string `json:"remote_ip,omitempty"`
	LocalAddr             string `json:"local_addr,omitempty"`
	Code                  string `json:"code,omitempty"`
	Message               string `json:"message,omitempty"`
	Abnormal              bool   `json:"abnormal,omitempty"`
	DurationMS            int64  `json:"duration_ms,omitempty"`
	Limit                 int    `json:"limit,omitempty"`
	WindowSec             int    `json:"window_sec,omitempty"`
	PermanentBlockCreated bool   `json:"permanent_block_created,omitempty"`
}

type FlowTrafficLogEntry struct {
	Time                     string `json:"time"`
	RequestID                string `json:"request_id,omitempty"`
	Event                    string `json:"event"`
	Kind                     string `json:"kind"`
	Result                   string `json:"result"`
	RemoteIP                 string `json:"remote_ip,omitempty"`
	Username                 string `json:"username,omitempty"`
	ClientID                 string `json:"client_id,omitempty"`
	ForwardName              string `json:"forward_name,omitempty"`
	Direction                string `json:"direction,omitempty"`
	Target                   string `json:"target,omitempty"`
	ListenAddr               string `json:"listen_addr,omitempty"`
	Code                     string `json:"code,omitempty"`
	Message                  string `json:"message,omitempty"`
	Abnormal                 bool   `json:"abnormal,omitempty"`
	DurationMS               int64  `json:"duration_ms,omitempty"`
	BytesUp                  int64  `json:"bytes_up,omitempty"`
	BytesDown                int64  `json:"bytes_down,omitempty"`
	BytesTotal               int64  `json:"bytes_total,omitempty"`
	AverageBytesPerSec       int64  `json:"average_bytes_per_sec,omitempty"`
	RateLimitBytesPerSec     int    `json:"rate_limit_bytes_per_sec,omitempty"`
	MaxConcurrentStreamsUser int    `json:"max_concurrent_streams_per_user,omitempty"`
}

type jsonlLog struct {
	mu          sync.Mutex
	basePath    string
	currentDate string
	file        *os.File
}

func newJSONLLog(basePath string) *jsonlLog {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" {
		return nil
	}
	return &jsonlLog{basePath: basePath}
}

func (l *jsonlLog) Write(entry any) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	date := time.Now().Format("2006-01-02")
	if l.file == nil || l.currentDate != date {
		if err := l.rotateLocked(date); err != nil {
			return err
		}
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := l.file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (l *jsonlLog) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

func (l *jsonlLog) rotateLocked(date string) error {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	path := datedJSONLPath(l.basePath, date)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	l.file = file
	l.currentDate = date
	return nil
}

func datedJSONLPath(basePath, date string) string {
	ext := filepath.Ext(basePath)
	name := strings.TrimSuffix(filepath.Base(basePath), ext)
	if ext == "" {
		ext = ".jsonl"
	}
	return filepath.Join(filepath.Dir(basePath), fmt.Sprintf("%s-%s%s", name, date, ext))
}

func (s *Server) nextRequestID() string {
	seq := s.requestSeq.Add(1)
	return fmt.Sprintf("req-%s-%06d", time.Now().Format("20060102T150405"), seq)
}

func (s *Server) recordRequestLog(entry RequestLogEntry) {
	if entry.Time == "" {
		entry.Time = time.Now().Format(time.RFC3339)
	}
	data, err := json.Marshal(entry)
	if err == nil {
		s.log("request_log %s", string(data))
	}
	if s.requestLog == nil {
		return
	}
	if err := s.requestLog.Write(entry); err != nil {
		s.log("write request log failed: %v", err)
	}
}

func (s *Server) recordBusinessLog(entry BusinessLogEntry) {
	if entry.Time == "" {
		entry.Time = time.Now().Format(time.RFC3339)
	}
	data, err := json.Marshal(entry)
	if err == nil {
		s.log("business_log %s", string(data))
	}
	if s.businessLog == nil {
		return
	}
	if err := s.businessLog.Write(entry); err != nil {
		s.log("write business log failed: %v", err)
	}
}

func (s *Server) recordEntryTrafficLog(entry EntryTrafficLogEntry) {
	if entry.Time == "" {
		entry.Time = time.Now().Format(time.RFC3339)
	}
	data, err := json.Marshal(entry)
	if err == nil {
		s.log("entry_traffic_log %s", string(data))
	}
	if s.entryTrafficLog == nil {
		return
	}
	if err := s.entryTrafficLog.Write(entry); err != nil {
		s.log("write entry traffic log failed: %v", err)
	}
}

func (s *Server) recordFlowTrafficLog(entry FlowTrafficLogEntry) {
	if entry.Time == "" {
		entry.Time = time.Now().Format(time.RFC3339)
	}
	if entry.BytesTotal == 0 {
		entry.BytesTotal = entry.BytesUp + entry.BytesDown
	}
	if entry.AverageBytesPerSec == 0 {
		entry.AverageBytesPerSec = averageBytesPerSec(entry.BytesTotal, entry.DurationMS)
	}
	if entry.RateLimitBytesPerSec == 0 {
		entry.RateLimitBytesPerSec = s.cfg.Security.StreamRateLimitBytesPerSec
	}
	if entry.MaxConcurrentStreamsUser == 0 {
		entry.MaxConcurrentStreamsUser = s.cfg.Security.MaxConcurrentStreamsPerUser
	}
	data, err := json.Marshal(entry)
	if err == nil {
		s.log("flow_traffic_log %s", string(data))
	}
	if s.flowTrafficLog == nil {
		return
	}
	if err := s.flowTrafficLog.Write(entry); err != nil {
		s.log("write flow traffic log failed: %v", err)
	}
}

func averageBytesPerSec(bytesTotal, durationMS int64) int64 {
	if bytesTotal <= 0 || durationMS <= 0 {
		return 0
	}
	return bytesTotal * 1000 / durationMS
}

func (s *Server) recordBusinessLogFromEvent(event RuntimeEvent) {
	if !isBusinessRuntimeEvent(event) {
		return
	}
	s.recordBusinessLog(BusinessLogEntry{
		Time:        event.Time,
		RequestID:   event.RequestID,
		Kind:        event.Kind,
		Result:      event.Result,
		RemoteIP:    event.RemoteIP,
		Username:    event.Username,
		ClientID:    event.ClientID,
		ForwardName: event.ForwardName,
		Direction:   event.Direction,
		Target:      event.Target,
		ListenAddr:  event.ListenAddr,
		Code:        event.Code,
		Message:     event.Message,
		DurationMS:  event.DurationMS,
		BytesUp:     event.BytesUp,
		BytesDown:   event.BytesDown,
	})
}

func isBusinessRuntimeEvent(event RuntimeEvent) bool {
	switch event.Kind {
	case "auth", "login", "open", "reverse_listen", "reverse_stream", "reverse_inbound":
		return true
	default:
		return false
	}
}
