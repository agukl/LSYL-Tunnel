package tunnel

import (
	"sync"
	"sync/atomic"
	"time"
)

type connectionLimiter struct {
	mu               sync.Mutex
	maxActive        int
	maxPerIP         int
	maxRate          int
	rateWindow       time.Duration
	maxItems         int
	cleanupEvery     time.Duration
	activeTotal      int
	items            map[string]*connectionLimitState
	now              func() time.Time
	evictedIPs       atomic.Uint64
	capacityRejected atomic.Uint64
}

type connectionLimitState struct {
	active   int
	attempts []time.Time
	lastSeen time.Time
}

func newConnectionLimiter(cfg SecurityConfig) *connectionLimiter {
	window := time.Duration(cfg.ConnectionRateWindowSec) * time.Second
	if window <= 0 {
		window = time.Second
	}
	maxItems := cfg.MaxTrackedConnectionIPs
	if maxItems <= 0 {
		maxItems = 8192
	}
	cleanupEvery := time.Duration(cfg.ConnectionLimiterCleanupSec) * time.Second
	if cleanupEvery <= 0 {
		cleanupEvery = time.Minute
	}
	return &connectionLimiter{
		maxActive:    cfg.MaxConcurrentConnections,
		maxPerIP:     cfg.MaxConcurrentConnectionsPerIP,
		maxRate:      cfg.MaxNewConnectionsPerIPWindow,
		rateWindow:   window,
		maxItems:     maxItems,
		cleanupEvery: cleanupEvery,
		items:        make(map[string]*connectionLimitState),
		now:          time.Now,
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
	if st != nil {
		l.pruneLocked(st, now)
		l.cleanupLocked(ip, st)
		st = l.items[ip]
	}

	if l.maxActive > 0 && l.activeTotal >= l.maxActive {
		return noopConnectionRelease, false, "global_concurrent_connections"
	}
	if st == nil {
		var ok bool
		st, ok = l.trackStateLocked(ip, now)
		if !ok {
			return noopConnectionRelease, false, "tracked_remote_ip_capacity"
		}
	}
	st.lastSeen = now
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
	now := l.now()
	l.pruneLocked(st, now)
	st.lastSeen = now
	l.cleanupLocked(ip, st)
}

func (l *connectionLimiter) snapshot() map[string]int {
	if l == nil {
		return map[string]int{"active": 0, "tracked_ips": 0, "max_tracked_ips": 0, "evicted_ips": 0, "capacity_rejected": 0}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupAllLocked(l.now())
	return map[string]int{
		"active":            l.activeTotal,
		"tracked_ips":       len(l.items),
		"max_tracked_ips":   l.maxItems,
		"evicted_ips":       int(l.evictedIPs.Load()),
		"capacity_rejected": int(l.capacityRejected.Load()),
	}
}

func (l *connectionLimiter) cleanup() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.cleanupAllLocked(l.now())
	l.mu.Unlock()
}

func (l *connectionLimiter) trackStateLocked(ip string, now time.Time) (*connectionLimitState, bool) {
	if state := l.items[ip]; state != nil {
		return state, true
	}
	if len(l.items) >= l.maxItems {
		l.cleanupAllLocked(now)
	}
	if len(l.items) >= l.maxItems {
		oldestIP := ""
		var oldest time.Time
		for candidate, state := range l.items {
			if state == nil || state.active > 0 {
				continue
			}
			if oldestIP == "" || state.lastSeen.Before(oldest) {
				oldestIP = candidate
				oldest = state.lastSeen
			}
		}
		if oldestIP == "" {
			l.capacityRejected.Add(1)
			return nil, false
		}
		delete(l.items, oldestIP)
		l.evictedIPs.Add(1)
	}
	state := &connectionLimitState{lastSeen: now}
	l.items[ip] = state
	return state, true
}

func (l *connectionLimiter) cleanupAllLocked(now time.Time) {
	for ip, state := range l.items {
		l.pruneLocked(state, now)
		l.cleanupLocked(ip, state)
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
