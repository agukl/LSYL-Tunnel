package tunnel

import (
	"net"
	"sync/atomic"
)

type meteredBusinessConn struct {
	net.Conn
	up   *atomic.Int64
	down *atomic.Int64
}

func (c *meteredBusinessConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.down.Add(int64(n))
	}
	return n, err
}

func (c *meteredBusinessConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.up.Add(int64(n))
	}
	return n, err
}

func (c *meteredBusinessConn) CloseWrite() error {
	if conn, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return conn.CloseWrite()
	}
	return c.Conn.Close()
}
