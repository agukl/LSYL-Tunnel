package tunnel

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"lsyltunnel/src/internal/transport"
)

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			s.log("accept failed: %v", err)
			continue
		}
		remoteIP := remoteHost(conn.RemoteAddr())
		if s.fails.blockKind(remoteIP) == blockedPermanent {
			s.recordPermanentBlockedHit(remoteIP)
			_ = conn.Close()
			continue
		}
		release, ok, reason := s.connLimiter.acquire(remoteIP)
		if !ok {
			s.recordConnectionRejected(reason)
			limit, windowSec := s.connectionRejectLimit(reason)
			s.recordEntryTrafficLog(EntryTrafficLogEntry{
				Event:      "connection_rejected",
				Result:     "rejected",
				RemoteAddr: addrString(conn.RemoteAddr()),
				RemoteIP:   remoteIP,
				LocalAddr:  addrString(conn.LocalAddr()),
				Code:       reason,
				Message:    "entry connection limit reached",
				Abnormal:   true,
				Limit:      limit,
				WindowSec:  windowSec,
			})
			_ = conn.Close()
			continue
		}
		acceptedAt := time.Now()
		tracked := s.trackEntryConn(conn, release)
		if tracked == nil {
			return
		}
		if !s.startWorker(func() { s.handleAcceptedConn(tracked, acceptedAt) }) {
			_ = tracked.Close()
			return
		}
	}
}

func (s *Server) handleAcceptedConn(conn net.Conn, acceptedAt time.Time) {
	peekConn := &entryPeekConn{Conn: conn}
	_ = peekConn.SetDeadline(time.Now().Add(time.Duration(s.cfg.Security.HandshakeTimeoutSec) * time.Second))
	if err := transport.ServerCertificateGate(peekConn, s.certificateGateKey); err != nil {
		if s.runContext().Err() != nil {
			_ = peekConn.Close()
			return
		}
		s.rejectEntryProtocol(peekConn, remoteHost(peekConn.RemoteAddr()), "", classifyEntryCertificateGateError(err, peekConn.Prefix()), "", true, time.Since(acceptedAt).Milliseconds())
		_ = peekConn.Close()
		return
	}
	peekConn.ResetPrefix()
	tlsConn := tls.Server(peekConn, s.tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		if s.runContext().Err() != nil {
			_ = tlsConn.Close()
			return
		}
		s.rejectEntryProtocol(tlsConn, remoteHost(tlsConn.RemoteAddr()), "", classifyEntryHandshakeError(err, peekConn.Prefix()), "", true, time.Since(acceptedAt).Milliseconds())
		_ = tlsConn.Close()
		return
	}
	s.handleConn(tlsConn)
}
