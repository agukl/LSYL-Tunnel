package tunnel

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

// Registration and shutdown share a lock so no worker can be added after Wait starts.
type serverLifecycle struct {
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closing   bool
	conns     map[*entryConn]struct{}
	workers   sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

func (s *Server) runContext() context.Context {
	if s.lifecycle.ctx != nil {
		return s.lifecycle.ctx
	}
	return context.Background()
}

func (s *Server) beginWorker() bool {
	s.lifecycle.mu.Lock()
	defer s.lifecycle.mu.Unlock()
	if s.lifecycle.closing {
		return false
	}
	s.lifecycle.workers.Add(1)
	return true
}

func (s *Server) startWorker(work func()) bool {
	if !s.beginWorker() {
		return false
	}
	go func() {
		defer s.lifecycle.workers.Done()
		work()
	}()
	return true
}

// The lease belongs to the TCP connection, including reverse handoffs, not its first handler.
type entryConn struct {
	net.Conn
	server    *Server
	release   func()
	closeOnce sync.Once
	closeErr  error
}

func (s *Server) trackEntryConn(conn net.Conn, release func()) *entryConn {
	tracked := &entryConn{Conn: conn, server: s, release: release}
	s.lifecycle.mu.Lock()
	if s.lifecycle.closing {
		s.lifecycle.mu.Unlock()
		_ = tracked.Close()
		return nil
	}
	if s.lifecycle.conns == nil {
		s.lifecycle.conns = make(map[*entryConn]struct{})
	}
	s.lifecycle.conns[tracked] = struct{}{}
	s.lifecycle.mu.Unlock()
	return tracked
}

func (c *entryConn) NetConn() net.Conn { return c.Conn }

func (c *entryConn) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.Conn.Close()
		if c.release != nil {
			c.release()
		}
		c.server.lifecycle.mu.Lock()
		delete(c.server.lifecycle.conns, c)
		c.server.lifecycle.mu.Unlock()
	})
	return c.closeErr
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.lifecycle.closeOnce.Do(func() {
		s.lifecycle.mu.Lock()
		s.lifecycle.closing = true
		conns := make([]*entryConn, 0, len(s.lifecycle.conns))
		for conn := range s.lifecycle.conns {
			conns = append(conns, conn)
		}
		s.lifecycle.mu.Unlock()
		if s.lifecycle.cancel != nil {
			s.lifecycle.cancel()
		}
		if s.listener != nil {
			if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				s.lifecycle.closeErr = err
			}
		}
		// Close raw sockets first to unblock TLS reads/writes before closing reverse controls.
		for _, conn := range conns {
			_ = conn.Close()
		}
		s.closeReverseListeners()
		if s.httpSrv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := s.httpSrv.Shutdown(ctx); err != nil {
				_ = s.httpSrv.Close()
			}
			cancel()
		}
		s.lifecycle.workers.Wait()
		// Stream-close records must be written before draining the entry queue and closing files.
		s.flushPermanentBlockHitLogs()
		if s.entryTrafficWriter != nil {
			s.entryTrafficWriter.Close()
		}
		for _, log := range []*jsonlLog{s.requestLog, s.businessLog, s.entryTrafficLog, s.flowTrafficLog} {
			_ = log.Close()
		}
	})
	return s.lifecycle.closeErr
}
