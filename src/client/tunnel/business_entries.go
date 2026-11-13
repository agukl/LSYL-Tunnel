package tunnel

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"syscall"
	"time"

	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
)

const (
	maxBusinessEntries       = 16
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
	failed      int
	totalDialMS int64
	lastError   string
}

func newBusinessEndpointPool(entries []string, bootstrap string, forwards []ForwardConfig) *businessEndpointPool {
	pool := &businessEndpointPool{
		failedTo: make(map[string]time.Time), quality: make(map[string]endpointDialQuality), qualityStarted: time.Now(),
	}
	seen := make(map[string]bool)
	redirectPorts := make(map[uint16]bool)
	for _, forward := range forwards {
		if forward.Direction != DirectionVirtual {
			continue
		}
		host, port, err := parseVirtualListenAddr(forward.ListenAddr)
		if err == nil && host != "127.0.0.1" {
			redirectPorts[uint16(port)] = true
		}
	}
	for _, value := range entries {
		if len(pool.entries) >= maxBusinessEntries {
			break
		}
		addr, err := netip.ParseAddrPort(strings.TrimSpace(value))
		if err != nil || addr.Port() == 0 || addr.Addr().IsUnspecified() || addr.Addr().IsMulticast() {
			continue
		}
		entry := addr.String()
		if entry == bootstrap || seen[entry] || redirectPorts[addr.Port()] {
			continue
		}
		seen[entry] = true
		pool.entries = append(pool.entries, entry)
		pool.quality[entry] = endpointDialQuality{}
	}
	pool.quality["bootstrap"] = endpointDialQuality{}
	return pool
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
	} else {
		quality.failed++
		quality.lastError = dialErrorKind(err)
	}
	p.quality[endpoint] = quality
}

func (p *businessEndpointPool) qualitySnapshot() []protocol.EntryQuality {
	if p == nil || len(p.entries) == 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rollQualityWindow(time.Now())
	result := make([]protocol.EntryQuality, 0, len(p.entries)+1)
	for _, endpoint := range append([]string{"bootstrap"}, p.entries...) {
		quality := p.quality[endpoint]
		average := 0
		if quality.successes > 0 {
			average = int(quality.totalDialMS / int64(quality.successes))
			if average > 60000 {
				average = 60000
			}
		}
		result = append(result, protocol.EntryQuality{
			Endpoint: endpoint, Attempts: quality.attempts, Successes: quality.successes,
			Failures: quality.failed, AvgDialMS: average, LastError: quality.lastError,
		})
	}
	return result
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

func dialErrorKind(err error) string {
	var networkError net.Error
	switch {
	case errors.As(err, &networkError) && networkError.Timeout():
		return "timeout"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "refused"
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
		return "unreachable"
	default:
		return "other"
	}
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
	pool.mu.Lock()
	start := int(pool.next % uint64(len(pool.entries)))
	pool.next++
	entries := append([]string(nil), pool.entries...)
	pool.mu.Unlock()

	attempts := 0
	for i := range entries {
		entry := entries[(start+i)%len(entries)]
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
		conn, err := (&net.Dialer{Timeout: entryTimeout}).DialContext(ctx, network, entry)
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
	started := time.Now()
	conn, err := (&net.Dialer{Timeout: d.timeout}).DialContext(ctx, network, bootstrap)
	if ctx.Err() == nil {
		pool.recordDial("bootstrap", time.Since(started), err)
	}
	return conn, err
}
