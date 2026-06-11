package tunnel

import (
	"sync"
	"time"
)

type connectionLimiter struct {
	mu          sync.Mutex
	maxActive   int
	maxPerIP    int
	maxRate     int
	rateWindow  time.Duration
	activeTotal int
	items       map[string]*connectionLimitState
	now         func() time.Time
}

type connectionLimitState struct {
	active   int
	attempts []time.Time
}

func newConnectionLimiter(cfg SecurityConfig) *connectionLimiter {
	window := time.Duration(cfg.ConnectionRateWindowSec) * time.Second
	if window <= 0 {
		window = time.Second
	}
	return &connectionLimiter{
		maxActive:  cfg.MaxConcurrentConnections,
		maxPerIP:   cfg.MaxConcurrentConnectionsPerIP,
		maxRate:    cfg.MaxNewConnectionsPerIPWindow,
		rateWindow: window,
		items:      make(map[string]*connectionLimitState),
		now:        time.Now,
	}
}

func (l *connectionLimiter) acquire(ip string) (func(), bool, string) {
	if l == nil {
		return noopConnectionRelease, true, ""
	}
	if ip == "" {
		ip = "unknown"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	st := l.items[ip]
	if st == nil {
		st = &connectionLimitState{}
		l.items[ip] = st
	}
	l.pruneLocked(st, now)

	if l.maxActive > 0 && l.activeTotal >= l.maxActive {
		l.cleanupLocked(ip, st)
		return noopConnectionRelease, false, "global_concurrent_connections"
	}
	if l.maxPerIP > 0 && st.active >= l.maxPerIP {
		l.cleanupLocked(ip, st)
		return noopConnectionRelease, false, "per_ip_concurrent_connections"
	}
	if l.maxRate > 0 && len(st.attempts) >= l.maxRate {
		l.cleanupLocked(ip, st)
		return noopConnectionRelease, false, "per_ip_new_connection_rate"
	}

	l.activeTotal++
	st.active++
	if l.maxRate > 0 {
		st.attempts = append(st.attempts, now)
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			l.release(ip)
		})
	}, true, ""
}

func (l *connectionLimiter) release(ip string) {
	if l == nil {
		return
	}
	if ip == "" {
		ip = "unknown"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	st := l.items[ip]
	if st == nil {
		return
	}
	if st.active > 0 {
		st.active--
	}
	if l.activeTotal > 0 {
		l.activeTotal--
	}
	l.pruneLocked(st, l.now())
	l.cleanupLocked(ip, st)
}

func (l *connectionLimiter) snapshot() map[string]int {
	if l == nil {
		return map[string]int{"active": 0, "tracked_ips": 0}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for ip, st := range l.items {
		l.pruneLocked(st, now)
		l.cleanupLocked(ip, st)
	}
	return map[string]int{
		"active":      l.activeTotal,
		"tracked_ips": len(l.items),
	}
}

func (l *connectionLimiter) pruneLocked(st *connectionLimitState, now time.Time) {
	if l.maxRate <= 0 || len(st.attempts) == 0 {
		st.attempts = nil
		return
	}
	cutoff := now.Add(-l.rateWindow)
	keep := 0
	for _, ts := range st.attempts {
		if ts.After(cutoff) {
			st.attempts[keep] = ts
			keep++
		}
	}
	st.attempts = st.attempts[:keep]
}

func (l *connectionLimiter) cleanupLocked(ip string, st *connectionLimitState) {
	if st.active == 0 && len(st.attempts) == 0 {
		delete(l.items, ip)
	}
}

func noopConnectionRelease() {}
