package tunnel

import (
	"net"
	"time"

	"lsyltunnel/src/internal/protocol"
)

// requestContext carries immutable metadata across authentication and forwarding.
// Connection ownership remains with handleConn until a reverse handler accepts it.
type requestContext struct {
	server     *Server
	id         string
	started    time.Time
	remoteIP   string
	remoteAddr string
	localAddr  string
}

func (s *Server) newRequestContext(conn net.Conn) *requestContext {
	return &requestContext{
		server:     s,
		id:         s.nextRequestID(),
		started:    time.Now(),
		remoteIP:   remoteHost(conn.RemoteAddr()),
		remoteAddr: addrString(conn.RemoteAddr()),
		localAddr:  addrString(conn.LocalAddr()),
	}
}

func (r *requestContext) record(req protocol.OpenRequest, result, authResult string, resp protocol.OpenResponse, readErr error) {
	entry := RequestLogEntry{
		RequestID:  r.id,
		RemoteAddr: r.remoteAddr,
		RemoteIP:   r.remoteIP,
		LocalAddr:  r.localAddr,
		Request:    req,
		AuthResult: authResult,
		Response:   resp,
		Result:     result,
		DurationMS: time.Since(r.started).Milliseconds(),
	}
	if readErr != nil {
		entry.ReadError = readErr.Error()
	}
	r.server.recordRequestLog(entry)
}

func (s *Server) handleConn(conn net.Conn) {
	closeConn := true
	defer func() {
		if closeConn {
			_ = conn.Close()
		}
	}()
	request := s.newRequestContext(conn)
	requestID, remoteIP := request.id, request.remoteIP
	recordRequest := request.record
	switch s.fails.blockKind(remoteIP) {
	case blockedPermanent:
		s.recordPermanentBlockedHit(remoteIP)
		return
	case blockedTemporary:
		_ = conn.SetDeadline(time.Now().Add(time.Duration(s.cfg.Security.HandshakeTimeoutSec) * time.Second))
		var ignored protocol.OpenRequest
		_ = protocol.ReadJSON(conn, &ignored, s.cfg.Security.MaxHandshakeBytes)
		_ = conn.SetWriteDeadline(time.Now().Add(time.Duration(s.cfg.Security.HandshakeTimeoutSec) * time.Second))
		resp := protocol.OpenResponse{OK: false, Code: "auth_blocked", Message: "too many login failures, temporarily blocked; try later"}
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "auth", Result: "blocked", RemoteIP: remoteIP, Code: "auth_blocked", Message: "too many login failures"})
		recordRequest(protocol.OpenRequest{}, "blocked", "blocked", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
	_ = conn.SetDeadline(time.Now().Add(time.Duration(s.cfg.Security.HandshakeTimeoutSec) * time.Second))
	var req protocol.OpenRequest
	if err := protocol.ReadJSON(conn, &req, s.cfg.Security.MaxHandshakeBytes); err != nil {
		if s.runContext().Err() != nil {
			return
		}
		s.rejectEntryProtocol(conn, remoteIP, requestID, classifyEntryReadError(err), "invalid tunnel request", true, time.Since(request.started).Milliseconds())
		return
	}
	if req.Type != "open" && req.Type != "login" && req.Type != "health" && req.Type != "forward_check" && req.Type != "reverse" && req.Type != "reverse_listen" && req.Type != "reverse_stream" {
		s.rejectEntryProtocol(conn, remoteIP, requestID, entryCodeUnsupportedRequest, "unsupported request", false, time.Since(request.started).Milliseconds())
		return
	}
	if resp, ok := s.checkRequestCompatibility(req); !ok {
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "compatibility", Result: "rejected", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, ListenAddr: req.ListenAddr, Code: resp.Code, Message: resp.Message})
		recordRequest(req, "rejected", "not_attempted", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
	user, resp, result := s.authenticateRequest(req, remoteIP)
	if result != "" {
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "auth", Result: result, RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, ListenAddr: req.ListenAddr, Code: resp.Code, Message: resp.Message})
		recordRequest(req, result, result, resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
	if req.Type == "health" {
		resp := serverVersionResponse(protocol.OpenResponse{OK: true, Code: "ok", Message: "server healthy"})
		recordRequest(req, "ok", "ok", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
	if req.Type == "forward_check" {
		if err := s.authorizeForwardCheck(user, req); err != nil {
			resp := protocol.OpenResponse{OK: false, Code: "target_denied", Message: err.Error()}
			recordRequest(req, "denied", "ok", resp, nil)
			_ = protocol.WriteJSON(conn, resp)
			return
		}
		resp := serverVersionResponse(protocol.OpenResponse{OK: true, Code: "ok", Message: "forward allowed"})
		recordRequest(req, "ok", "ok", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
	s.authOK.Add(1)
	if req.Type == "login" {
		resp := serverVersionResponse(protocol.OpenResponse{OK: true, Code: "ok", Message: "login verified", CredentialKey: s.activeCredentialPublicKey()})
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "login", Result: "ok", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, Code: "ok"})
		recordRequest(req, "ok", "ok", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
	switch req.Type {
	case "reverse", "reverse_listen":
		closeConn = !s.handleReverseControl(conn, req, user, request)
	case "reverse_stream":
		closeConn = !s.handleReverseStream(conn, req, user, request)
	default:
		s.handleOpen(conn, req, user, request)
	}
}
