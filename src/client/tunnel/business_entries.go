package tunnel

import (
	"context"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"lsyltunnel/src/internal/transport"
)

const (
	maxBusinessEntries       = 64
	maxBusinessDialAttempts  = 2
	businessEntryRetryDelay  = 30 * time.Second
	businessEntryDialTimeout = 1200 * time.Millisecond
	businessQualityWindow    = time.Minute
)

type businessEndpointPool struct {
	mu             sync.Mutex
	entries        []string
	failedTo       map[string]time.Time
	quality        map[string]endpointDialQuality
	qualityStarted time.Time
	next           uint64
}

type endpointDialQuality struct {
	attempts    int
	successes   int
	totalDialMS int64
}

func newBusinessEndpointPool(entries []string, bootstrap string) *businessEndpointPool {
	pool := &businessEndpointPool{
		failedTo: make(map[string]time.Time), quality: make(map[string]endpointDialQuality),
		qualityStarted: time.Now(),
	}
	seen := make(map[string]bool)
	for _, value := range entries {
		if len(pool.entries) >= maxBusinessEntries {
			break
		}
		entry := strings.TrimSpace(value)
		addr, err := netip.ParseAddrPort(entry)
		if err != nil || addr.Port() == 0 || !businessIPv6Address(addr.Addr()) {
			continue
		}
		entry = addr.String()
		if entry == bootstrap || seen[entry] {
			continue
		}
		seen[entry] = true
		pool.entries = append(pool.entries, entry)
		pool.quality[entry] = endpointDialQuality{}
	}
	return pool
}

func businessIPv6Address(addr netip.Addr) bool {
	return addr.Is6() && addr.IsGlobalUnicast() && !addr.IsPrivate() && !addr.Is4In6() && addr.Zone() == ""
}

func (p *businessEndpointPool) recordDial(endpoint string, elapsed time.Duration, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rollQualityWindow(time.Now())
	quality := p.quality[endpoint]
	if quality.attempts >= 1000000 {
		return
	}
	quality.attempts++
	if err == nil {
		quality.successes++
		quality.totalDialMS += elapsed.Milliseconds()
	}
	p.quality[endpoint] = quality
}

func (p *businessEndpointPool) rollQualityWindow(now time.Time) {
	if now.Sub(p.qualityStarted) < businessQualityWindow {
		return
	}
	for endpoint := range p.quality {
		p.quality[endpoint] = endpointDialQuality{}
	}
	p.qualityStarted = now
}

func (c *Client) businessDialer(timeout time.Duration) transport.ContextDialer {
	if c.businessPool == nil || len(c.businessPool.entries) == 0 {
		return &net.Dialer{Timeout: timeout}
	}
	return &businessPoolDialer{pool: c.businessPool, timeout: timeout}
}

type businessPoolDialer struct {
	pool    *businessEndpointPool
	timeout time.Duration
}

func (d *businessPoolDialer) DialContext(ctx context.Context, network, bootstrap string) (net.Conn, error) {
	pool := d.pool
	entries := pool.orderedEntries()

	attempts := 0
	for _, entry := range entries {
		pool.mu.Lock()
		cooling := time.Now().Before(pool.failedTo[entry])
		pool.mu.Unlock()
		if cooling {
			continue
		}
		attempts++
		entryTimeout := d.timeout
		if entryTimeout <= 0 || entryTimeout > businessEntryDialTimeout {
			entryTimeout = businessEntryDialTimeout
		}
		started := time.Now()
		conn, err := (&net.Dialer{Timeout: entryTimeout}).DialContext(ctx, "tcp6", entry)
		if ctx.Err() == nil {
			pool.recordDial(entry, time.Since(started), err)
		}
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		pool.mu.Lock()
		pool.failedTo[entry] = time.Now().Add(businessEntryRetryDelay)
		pool.mu.Unlock()
		if attempts >= maxBusinessDialAttempts {
			break
		}
	}
	return (&net.Dialer{Timeout: d.timeout}).DialContext(ctx, network, bootstrap)
}

func (p *businessEndpointPool) orderedEntries() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rollQualityWindow(time.Now())
	start := int(p.next % uint64(len(p.entries)))
	p.next++
	entries := make([]string, len(p.entries))
	quality := make(map[string]endpointDialQuality, len(entries))
	for i := range entries {
		entries[i] = p.entries[(start+i)%len(entries)]
		quality[entries[i]] = p.quality[entries[i]]
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := quality[entries[i]], quality[entries[j]]
		if left.attempts == 0 || right.attempts == 0 {
			return left.attempts == 0 && right.attempts > 0
		}
		leftSuccess := left.successes * right.attempts
		rightSuccess := right.successes * left.attempts
		if leftSuccess != rightSuccess {
			return leftSuccess > rightSuccess
		}
		if left.successes > 0 && right.successes > 0 {
			return left.totalDialMS/int64(left.successes) < right.totalDialMS/int64(right.successes)
		}
		return false
	})
	return entries
}
