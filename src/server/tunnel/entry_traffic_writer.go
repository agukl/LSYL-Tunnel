package tunnel

import (
	"sync"
	"sync/atomic"
)

type entryTrafficWriterStats struct {
	Capacity int
	Queued   int
	Accepted uint64
	Written  uint64
	Dropped  uint64
}

// asyncEntryTrafficLog keeps rejection paths independent from log-file latency.
type asyncEntryTrafficLog struct {
	mu       sync.Mutex
	queue    chan EntryTrafficLogEntry
	stop     chan struct{}
	done     chan struct{}
	closed   bool
	write    func(EntryTrafficLogEntry)
	accepted atomic.Uint64
	written  atomic.Uint64
	dropped  atomic.Uint64
}

func newAsyncEntryTrafficLog(capacity int, write func(EntryTrafficLogEntry)) *asyncEntryTrafficLog {
	if capacity <= 0 {
		capacity = 2048
	}
	writer := &asyncEntryTrafficLog{
		queue: make(chan EntryTrafficLogEntry, capacity),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
		write: write,
	}
	go writer.run()
	return writer
}

func (w *asyncEntryTrafficLog) Enqueue(entry EntryTrafficLogEntry) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	select {
	case w.queue <- entry:
		w.accepted.Add(1)
	default:
		w.dropped.Add(1)
	}
}

func (w *asyncEntryTrafficLog) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	close(w.stop)
	w.mu.Unlock()
	<-w.done
}

func (w *asyncEntryTrafficLog) stats() entryTrafficWriterStats {
	if w == nil {
		return entryTrafficWriterStats{}
	}
	return entryTrafficWriterStats{
		Capacity: cap(w.queue),
		Queued:   len(w.queue),
		Accepted: w.accepted.Load(),
		Written:  w.written.Load(),
		Dropped:  w.dropped.Load(),
	}
}

func (w *asyncEntryTrafficLog) run() {
	defer close(w.done)
	for {
		select {
		case entry := <-w.queue:
			w.writeEntry(entry)
		case <-w.stop:
			for {
				select {
				case entry := <-w.queue:
					w.writeEntry(entry)
				default:
					return
				}
			}
		}
	}
}

func (w *asyncEntryTrafficLog) writeEntry(entry EntryTrafficLogEntry) {
	if w.write != nil {
		w.write(entry)
	}
	w.written.Add(1)
}
