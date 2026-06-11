package transport

import (
	"crypto/tls"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type LogFunc func(format string, args ...any)

func TLSMinVersion(version string) uint16 {
	switch strings.TrimSpace(version) {
	case "1.2", "TLS1.2", "tls1.2":
		return tls.VersionTLS12
	default:
		return tls.VersionTLS13
	}
}

func ProxyPair(left, right net.Conn, leftToRightBytes, rightToLeftBytes *atomic.Int64) (int64, int64) {
	return ProxyPairWithOptions(left, right, leftToRightBytes, rightToLeftBytes, ProxyOptions{})
}

type ProxyOptions struct {
	RateLimitBytesPerSec int
}

func ProxyPairWithOptions(left, right net.Conn, leftToRightBytes, rightToLeftBytes *atomic.Int64, opts ProxyOptions) (int64, int64) {
	var limiter *byteRateLimiter
	if opts.RateLimitBytesPerSec > 0 {
		limiter = newByteRateLimiter(opts.RateLimitBytesPerSec)
	}
	done := make(chan proxyCopyResult, 2)
	go copyAndClose("left_to_right", right, left, leftToRightBytes, limiter, done)
	go copyAndClose("right_to_left", left, right, rightToLeftBytes, limiter, done)
	first := <-done
	_ = left.Close()
	_ = right.Close()
	second := <-done
	var leftToRight int64
	var rightToLeft int64
	for _, result := range []proxyCopyResult{first, second} {
		if result.direction == "left_to_right" {
			leftToRight = result.bytes
		} else {
			rightToLeft = result.bytes
		}
	}
	return leftToRight, rightToLeft
}

type proxyCopyResult struct {
	direction string
	bytes     int64
}

func copyAndClose(direction string, dst, src net.Conn, counter *atomic.Int64, limiter *byteRateLimiter, done chan<- proxyCopyResult) {
	var reader io.Reader = src
	if limiter != nil {
		reader = &rateLimitedReader{reader: src, limiter: limiter}
	}
	n, _ := io.Copy(dst, reader)
	if counter != nil {
		counter.Add(n)
	}
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	} else {
		_ = dst.Close()
	}
	done <- proxyCopyResult{direction: direction, bytes: n}
}

type rateLimitedReader struct {
	reader  io.Reader
	limiter *byteRateLimiter
}

func (r *rateLimitedReader) Read(p []byte) (int, error) {
	if max := r.limiter.maxChunk(); max > 0 && len(p) > max {
		p = p[:max]
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		r.limiter.wait(n)
	}
	return n, err
}

type byteRateLimiter struct {
	mu    sync.Mutex
	rate  float64
	chunk int
	next  time.Time
}

func newByteRateLimiter(rateBytesPerSec int) *byteRateLimiter {
	chunk := rateBytesPerSec / 10
	if chunk < 1 {
		chunk = 1
	}
	if chunk > 32*1024 {
		chunk = 32 * 1024
	}
	initialBurst := time.Duration(float64(chunk) / float64(rateBytesPerSec) * float64(time.Second))
	return &byteRateLimiter{
		rate:  float64(rateBytesPerSec),
		chunk: chunk,
		next:  time.Now().Add(-initialBurst),
	}
}

func (l *byteRateLimiter) maxChunk() int {
	if l == nil {
		return 0
	}
	if l.chunk < 1 {
		return 1
	}
	return l.chunk
}

func (l *byteRateLimiter) wait(n int) {
	if l == nil || l.rate <= 0 || n <= 0 {
		return
	}
	remaining := n
	for remaining > 0 {
		chunk := remaining
		if chunk > l.chunk {
			chunk = l.chunk
		}
		l.waitChunk(chunk)
		remaining -= chunk
	}
}

func (l *byteRateLimiter) waitChunk(n int) {
	now := time.Now()
	duration := time.Duration(float64(n) / l.rate * float64(time.Second))
	l.mu.Lock()
	available := l.next
	if available.Before(now) {
		available = now
	}
	l.next = available.Add(duration)
	wait := available.Sub(now)
	l.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
}

// EnableTCPKeepAlive asks the OS to detect broken TCP peers faster than the
// platform default. Application-level timeouts still make the final decision.
func EnableTCPKeepAlive(conn net.Conn, period time.Duration) {
	if conn == nil {
		return
	}
	if period <= 0 {
		period = 30 * time.Second
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(period)
		return
	}
	type netConner interface {
		NetConn() net.Conn
	}
	if wrapped, ok := conn.(netConner); ok {
		EnableTCPKeepAlive(wrapped.NetConn(), period)
	}
}
