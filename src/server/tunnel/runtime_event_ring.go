package tunnel

import "sync"

type runtimeEventRing struct {
	mu     sync.Mutex
	items  []RuntimeEvent
	start  int
	length int
}

func newRuntimeEventRing(capacity int) *runtimeEventRing {
	if capacity <= 0 {
		capacity = defaultRecentEvents
	}
	return &runtimeEventRing{items: make([]RuntimeEvent, capacity)}
}

func (r *runtimeEventRing) Append(event RuntimeEvent) {
	if r == nil || len(r.items) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.length < len(r.items) {
		index := (r.start + r.length) % len(r.items)
		r.items[index] = event
		r.length++
		return
	}
	r.items[r.start] = event
	r.start = (r.start + 1) % len(r.items)
}

func (r *runtimeEventRing) Snapshot() []RuntimeEvent {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RuntimeEvent, r.length)
	for i := 0; i < r.length; i++ {
		out[i] = r.items[(r.start+i)%len(r.items)]
	}
	return out
}
