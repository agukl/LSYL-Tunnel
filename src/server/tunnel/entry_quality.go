package tunnel

import (
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"lsyltunnel/src/internal/protocol"
)

const (
	entryQualityMaxClients = 256
	entryQualityTTL        = 5 * time.Minute
	entryQualityMaxCount   = 1000000
)

type EntryQualityClientStatus struct {
	Username  string                  `json:"username"`
	ClientID  string                  `json:"client_id"`
	RemoteIP  string                  `json:"remote_ip"`
	UpdatedAt time.Time               `json:"updated_at"`
	Entries   []protocol.EntryQuality `json:"entries"`
}

type entryQualityTracker struct {
	mu      sync.Mutex
	clients map[string]EntryQualityClientStatus
}

func newEntryQualityTracker() *entryQualityTracker {
	return &entryQualityTracker{clients: make(map[string]EntryQualityClientStatus)}
}

func (t *entryQualityTracker) observe(username, clientID, remoteIP string, report []protocol.EntryQuality, configured []BusinessEntryConfig) {
	if t == nil || !validEntryQuality(report, configured) || len(clientID) > 128 {
		return
	}
	key := username + "\x00" + clientID
	if clientID == "" {
		key += "\x00" + remoteIP
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.clients[key]; !exists && len(t.clients) >= entryQualityMaxClients {
		t.pruneLocked(now)
		if len(t.clients) >= entryQualityMaxClients {
			oldestKey := ""
			var oldest time.Time
			for candidate, status := range t.clients {
				if oldestKey == "" || status.UpdatedAt.Before(oldest) {
					oldestKey, oldest = candidate, status.UpdatedAt
				}
			}
			delete(t.clients, oldestKey)
		}
	}
	t.clients[key] = EntryQualityClientStatus{
		Username: username, ClientID: clientID, RemoteIP: remoteIP, UpdatedAt: now,
		Entries: append([]protocol.EntryQuality(nil), report...),
	}
}

func validEntryQuality(report []protocol.EntryQuality, configured []BusinessEntryConfig) bool {
	if len(report) == 0 || len(report) > len(configured)+1 || len(report) > maxBusinessEntries+1 {
		return false
	}
	allowed := map[string]bool{"bootstrap": true}
	for _, entry := range configured {
		addr, err := netip.ParseAddrPort(strings.TrimSpace(entry.AdvertiseAddr))
		if err == nil {
			allowed[addr.String()] = true
		}
	}
	seen := make(map[string]bool, len(report))
	for _, entry := range report {
		if !allowed[entry.Endpoint] || seen[entry.Endpoint] || entry.Attempts < 0 || entry.Attempts > entryQualityMaxCount ||
			entry.Successes < 0 || entry.Failures < 0 || entry.Successes+entry.Failures != entry.Attempts ||
			entry.AvgDialMS < 0 || entry.AvgDialMS > 60000 {
			return false
		}
		switch entry.LastError {
		case "", "timeout", "refused", "unreachable", "other":
		default:
			return false
		}
		seen[entry.Endpoint] = true
	}
	return true
}

func (t *entryQualityTracker) snapshot() []EntryQualityClientStatus {
	result := make([]EntryQualityClientStatus, 0)
	if t == nil {
		return result
	}
	t.mu.Lock()
	t.pruneLocked(time.Now())
	for _, status := range t.clients {
		status.Entries = append([]protocol.EntryQuality(nil), status.Entries...)
		result = append(result, status)
	}
	t.mu.Unlock()
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Username != right.Username {
			return left.Username < right.Username
		}
		return strings.Compare(left.ClientID, right.ClientID) < 0
	})
	return result
}

func (t *entryQualityTracker) pruneLocked(now time.Time) {
	for key, status := range t.clients {
		if now.Sub(status.UpdatedAt) > entryQualityTTL {
			delete(t.clients, key)
		}
	}
}
