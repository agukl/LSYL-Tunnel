package tunnel

import "sync"

type userStreamLimiter struct {
	mu         sync.Mutex
	maxPerUser int
	active     map[string]int
}

func newUserStreamLimiter(cfg SecurityConfig) *userStreamLimiter {
	return &userStreamLimiter{
		maxPerUser: cfg.MaxConcurrentStreamsPerUser,
		active:     map[string]int{},
	}
}

func (l *userStreamLimiter) acquire(username string) (func(), bool) {
	if l == nil || l.maxPerUser <= 0 {
		return noopUserStreamRelease, true
	}
	if username == "" {
		username = "unknown"
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[username] >= l.maxPerUser {
		return noopUserStreamRelease, false
	}
	l.active[username]++

	var once sync.Once
	return func() {
		once.Do(func() {
			l.release(username)
		})
	}, true
}

func (l *userStreamLimiter) release(username string) {
	if l == nil || l.maxPerUser <= 0 {
		return
	}
	if username == "" {
		username = "unknown"
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[username] <= 1 {
		delete(l.active, username)
		return
	}
	l.active[username]--
}

func (l *userStreamLimiter) snapshot() userStreamLimitSnapshot {
	if l == nil {
		return userStreamLimitSnapshot{ActiveByUser: map[string]int{}}
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	activeByUser := make(map[string]int, len(l.active))
	total := 0
	for username, active := range l.active {
		if active <= 0 {
			continue
		}
		activeByUser[username] = active
		total += active
	}
	return userStreamLimitSnapshot{
		Active:       total,
		TrackedUsers: len(activeByUser),
		MaxPerUser:   l.maxPerUser,
		ActiveByUser: activeByUser,
	}
}

type userStreamLimitSnapshot struct {
	Active       int
	TrackedUsers int
	MaxPerUser   int
	ActiveByUser map[string]int
}

func noopUserStreamRelease() {}
