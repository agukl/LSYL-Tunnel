package tunnel

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type failTracker struct {
	mu sync.RWMutex
	// File writes use a separate lock so entry checks never wait on disk under mu.
	persistMu               sync.Mutex
	window                  time.Duration
	limit                   int
	blockFor                time.Duration
	maxItems                int
	cleanupEvery            time.Duration
	now                     func() time.Time
	successfulDate          string
	successfulIPs           map[string]struct{}
	successCapacityExceeded bool
	items                   map[string]*failState
	permanent               sync.Map
	stateFile               string
	permanentFile           string
	evicted                 atomic.Uint64
	capacityDrop            atomic.Uint64
}

type blockKind int

const (
	blockedNone blockKind = iota
	blockedTemporary
	blockedPermanent
)

type failState struct {
	authFailures     []time.Time
	protocolFailures []time.Time
	blockedUntil     time.Time
	lastSeen         time.Time
}

func newFailTracker(cfg SecurityConfig, stateFile, permanentFile string) *failTracker {
	maxItems := cfg.MaxTrackedFailureIPs
	if maxItems <= 0 {
		maxItems = 8192
	}
	cleanupEvery := time.Duration(cfg.FailureTrackerCleanupSec) * time.Second
	if cleanupEvery <= 0 {
		cleanupEvery = time.Minute
	}
	return &failTracker{
		window:        time.Duration(cfg.AuthFailWindowSec) * time.Second,
		limit:         cfg.AuthFailThreshold,
		blockFor:      time.Duration(cfg.AuthFailBlockSec) * time.Second,
		maxItems:      maxItems,
		cleanupEvery:  cleanupEvery,
		now:           time.Now,
		successfulIPs: map[string]struct{}{},
		items:         map[string]*failState{},
		stateFile:     strings.TrimSpace(stateFile),
		permanentFile: strings.TrimSpace(permanentFile),
	}
}

func (f *failTracker) isBlocked(key string) bool {
	return f.blockKind(key) != blockedNone
}

func (f *failTracker) blockKind(key string) blockKind {
	if f.hasPermanent(key) {
		return blockedPermanent
	}
	f.mu.RLock()
	state := f.items[key]
	if state == nil {
		f.mu.RUnlock()
		return blockedNone
	}
	blockedUntil := state.blockedUntil
	if state.blockedUntil.IsZero() {
		f.mu.RUnlock()
		return blockedNone
	}
	f.mu.RUnlock()
	now := f.now()
	if now.Before(blockedUntil) {
		return blockedTemporary
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	state = f.items[key]
	if state == nil {
		return blockedNone
	}
	if f.hasPermanent(key) {
		return blockedPermanent
	}
	if state.blockedUntil.IsZero() || now.Before(state.blockedUntil) {
		if state.blockedUntil.IsZero() {
			return blockedNone
		}
		return blockedTemporary
	}
	state.blockedUntil = time.Time{}
	if len(state.authFailures) == 0 && len(state.protocolFailures) == 0 {
		delete(f.items, key)
	}
	return blockedNone
}

func (f *failTracker) addFailure(key string) {
	f.mu.Lock()
	now := f.now()
	state, ok := f.trackStateLocked(key, now)
	if !ok {
		f.mu.Unlock()
		return
	}
	if f.hasPermanent(key) {
		f.mu.Unlock()
		return
	}
	state.authFailures = appendRecentFailures(state.authFailures, now.Add(-f.window))
	state.authFailures = append(state.authFailures, now)
	state.lastSeen = now
	if len(state.authFailures) >= f.limit {
		state.blockedUntil = now.Add(f.blockFor)
		state.authFailures = nil
		f.mu.Unlock()
		_ = f.save()
		return
	}
	f.mu.Unlock()
}

func (f *failTracker) addProtocolFailure(key string) bool {
	if canonical, ok := canonicalSuccessfulIP(key); ok {
		key = canonical
	}
	f.mu.Lock()
	now := f.now()
	if f.successfulTodayLocked(key, now) || f.successCapacityExceeded {
		f.mu.Unlock()
		return false
	}
	state, ok := f.trackStateLocked(key, now)
	if !ok {
		f.mu.Unlock()
		return false
	}
	if f.hasPermanent(key) {
		f.mu.Unlock()
		return false
	}
	state.protocolFailures = appendRecentFailures(state.protocolFailures, now.Add(-f.window))
	state.protocolFailures = append(state.protocolFailures, now)
	state.lastSeen = now
	if len(state.protocolFailures) >= f.limit {
		state.blockedUntil = time.Time{}
		state.authFailures = nil
		state.protocolFailures = nil
		f.mu.Unlock()
		return f.blockPermanent(key)
	}
	f.mu.Unlock()
	return false
}

func (f *failTracker) blockPermanent(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	f.persistMu.Lock()
	if f.hasPermanent(key) {
		f.persistMu.Unlock()
		return false
	}
	f.mu.Lock()
	if f.successfulTodayLocked(key, f.now()) || f.successCapacityExceeded {
		f.mu.Unlock()
		f.persistMu.Unlock()
		return false
	}
	f.mu.Unlock()
	if err := appendPermanentBlockedIP(f.permanentFile, key); err != nil {
		f.persistMu.Unlock()
		return false
	}
	f.permanent.Store(key, struct{}{})
	f.mu.Lock()
	delete(f.items, key)
	f.mu.Unlock()
	f.persistMu.Unlock()
	_ = f.save()
	return true
}

func (f *failTracker) hasPermanent(key string) bool {
	if f == nil {
		return false
	}
	_, ok := f.permanent.Load(key)
	return ok
}

type failTrackerStats struct {
	TrackedIPs    int
	MaxTrackedIPs int
	EvictedIPs    uint64
	CapacityDrops uint64
}

func (f *failTracker) trackStateLocked(key string, now time.Time) (*failState, bool) {
	key = strings.TrimSpace(key)
	if key == "" || f.hasPermanent(key) {
		return nil, false
	}
	if state := f.items[key]; state != nil {
		state.lastSeen = now
		return state, true
	}
	if len(f.items) >= f.maxItems {
		_ = f.cleanupLocked(now)
	}
	if len(f.items) >= f.maxItems {
		oldestKey := ""
		var oldest time.Time
		for candidate, state := range f.items {
			if state == nil || (!state.blockedUntil.IsZero() && state.blockedUntil.After(now)) {
				continue
			}
			if oldestKey == "" || state.lastSeen.Before(oldest) {
				oldestKey = candidate
				oldest = state.lastSeen
			}
		}
		if oldestKey == "" {
			f.capacityDrop.Add(1)
			return nil, false
		}
		delete(f.items, oldestKey)
		f.evicted.Add(1)
	}
	state := &failState{lastSeen: now}
	f.items[key] = state
	return state, true
}

func appendRecentFailures(items []time.Time, cutoff time.Time) []time.Time {
	kept := items[:0]
	for _, item := range items {
		if item.After(cutoff) {
			kept = append(kept, item)
		}
	}
	return kept
}

func (f *failTracker) cleanup() error {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	changed := f.cleanupLocked(f.now())
	f.mu.Unlock()
	if !changed {
		return nil
	}
	return f.save()
}

func (f *failTracker) cleanupLocked(now time.Time) bool {
	if f == nil {
		return false
	}
	changed := false
	cutoff := now.Add(-f.window)
	for key, state := range f.items {
		if state == nil {
			delete(f.items, key)
			changed = true
			continue
		}
		beforeAuth := len(state.authFailures)
		beforeProtocol := len(state.protocolFailures)
		state.authFailures = appendRecentFailures(state.authFailures, cutoff)
		state.protocolFailures = appendRecentFailures(state.protocolFailures, cutoff)
		if len(state.authFailures) != beforeAuth || len(state.protocolFailures) != beforeProtocol {
			changed = true
		}
		if !state.blockedUntil.IsZero() && !state.blockedUntil.After(now) {
			state.blockedUntil = time.Time{}
			changed = true
		}
		if state.blockedUntil.IsZero() && len(state.authFailures) == 0 && len(state.protocolFailures) == 0 {
			delete(f.items, key)
			changed = true
		}
	}
	return changed
}

func (f *failTracker) stats() failTrackerStats {
	if f == nil {
		return failTrackerStats{}
	}
	f.mu.RLock()
	stats := failTrackerStats{
		TrackedIPs:    len(f.items),
		MaxTrackedIPs: f.maxItems,
		EvictedIPs:    f.evicted.Load(),
		CapacityDrops: f.capacityDrop.Load(),
	}
	f.mu.RUnlock()
	return stats
}
