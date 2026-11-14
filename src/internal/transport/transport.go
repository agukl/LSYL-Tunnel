package transport

import (
	"crypto/tls"
	"io"
	"net"
	"strings"
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
	done := make(chan proxyCopyResult, 2)
	go copyAndClose("left_to_right", right, left, leftToRightBytes, done)
	go copyAndClose("right_to_left", left, right, rightToLeftBytes, done)
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

func copyAndClose(direction string, dst, src net.Conn, counter *atomic.Int64, done chan<- proxyCopyResult) {
	n, _ := io.Copy(dst, src)
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
