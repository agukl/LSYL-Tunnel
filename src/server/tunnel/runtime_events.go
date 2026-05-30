package tunnel

import (
	"fmt"
	"strings"
	"time"
)

const defaultRecentEvents = 500

type RuntimeEvent struct {
	Time        string `json:"time"`
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

func (s *Server) recordEvent(event RuntimeEvent) {
	if s == nil {
		return
	}
	if event.Time == "" {
		event.Time = time.Now().Format(time.RFC3339)
	}
	maxEvents := s.maxRecentEvents
	if maxEvents <= 0 {
		maxEvents = defaultRecentEvents
	}
	s.eventMu.Lock()
	s.recentEvents = append(s.recentEvents, event)
	if len(s.recentEvents) > maxEvents {
		s.recentEvents = append([]RuntimeEvent{}, s.recentEvents[len(s.recentEvents)-maxEvents:]...)
	}
	s.eventMu.Unlock()
	s.recordBusinessLogFromEvent(event)
	s.log("event kind=%s result=%s remote_ip=%s user=%s client=%s forward=%s direction=%s target=%s listen=%s code=%s duration_ms=%d message=%s",
		logValue(event.Kind),
		logValue(event.Result),
		logValue(event.RemoteIP),
		logValue(event.Username),
		logValue(event.ClientID),
		logValue(event.ForwardName),
		logValue(event.Direction),
		logValue(event.Target),
		logValue(event.ListenAddr),
		logValue(event.Code),
		event.DurationMS,
		logValue(event.Message),
	)
}

func (s *Server) recentEventSnapshot() []RuntimeEvent {
	if s == nil {
		return nil
	}
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	out := make([]RuntimeEvent, len(s.recentEvents))
	copy(out, s.recentEvents)
	return out
}

func logValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Sprintf("%q", value)
	}
	return value
}
