package tunnel

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

const defaultPermanentBlockHitLogInterval = 10 * time.Second

type permanentBlockHitAggregator struct {
	mu       sync.Mutex
	interval time.Duration
	hits     map[string]int64
}

func newPermanentBlockHitAggregator(interval time.Duration) *permanentBlockHitAggregator {
	if interval <= 0 {
		interval = defaultPermanentBlockHitLogInterval
	}
	return &permanentBlockHitAggregator{
		interval: interval,
		hits:     map[string]int64{},
	}
}

func (a *permanentBlockHitAggregator) Observe(ip string) {
	if a == nil || ip == "" {
		return
	}
	a.mu.Lock()
	a.hits[ip]++
	a.mu.Unlock()
}

func (a *permanentBlockHitAggregator) Flush() map[string]int64 {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.hits) == 0 {
		return nil
	}
	out := a.hits
	a.hits = map[string]int64{}
	return out
}

func (s *Server) recordPermanentBlockedHit(remoteIP string) {
	if s == nil || remoteIP == "" {
		return
	}
	if s.permanentBlockHits == nil {
		s.recordEvent(RuntimeEvent{
			Kind:     "auth",
			Result:   "blocked",
			RemoteIP: remoteIP,
			Code:     "ip_permanently_blocked",
			Message:  "permanently blocked ip hit 1 time",
		})
		return
	}
	s.permanentBlockHits.Observe(remoteIP)
}

func (s *Server) permanentBlockHitLogLoop(done <-chan struct{}) {
	if s == nil || s.permanentBlockHits == nil {
		return
	}
	ticker := time.NewTicker(s.permanentBlockHits.interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			s.flushPermanentBlockHitLogs()
			return
		case <-ticker.C:
			s.flushPermanentBlockHitLogs()
		}
	}
}

func (s *Server) flushPermanentBlockHitLogs() {
	if s == nil || s.permanentBlockHits == nil {
		return
	}
	hits := s.permanentBlockHits.Flush()
	if len(hits) == 0 {
		return
	}
	ips := make([]string, 0, len(hits))
	for ip := range hits {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	interval := s.permanentBlockHits.interval
	for _, ip := range ips {
		count := hits[ip]
		if count <= 0 {
			continue
		}
		s.recordEvent(RuntimeEvent{
			Kind:     "auth",
			Result:   "blocked",
			RemoteIP: ip,
			Code:     "ip_permanently_blocked",
			Message:  permanentBlockedHitSummary(count, interval),
		})
	}
}

func permanentBlockedHitSummary(count int64, interval time.Duration) string {
	if count <= 1 {
		return "permanently blocked ip hit 1 time"
	}
	seconds := int64(interval / time.Second)
	if seconds <= 0 {
		seconds = 1
	}
	return fmt.Sprintf("permanently blocked ip hit %d times in last %ds", count, seconds)
}
