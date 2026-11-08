package tunnel

import (
	"errors"
	"net"
	"time"

	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
)

var errUserStreamLimit = errors.New("too many concurrent streams for account")

func (s *Server) proxyOptions() transport.ProxyOptions {
	return transport.ProxyOptions{RateLimitBytesPerSec: s.cfg.Security.StreamRateLimitBytesPerSec}
}

func (s *Server) handleOpen(conn net.Conn, req protocol.OpenRequest, user UserConfig, request *requestContext) {
	requestID, remoteIP := request.id, request.remoteIP
	recordRequest := request.record
	if err := s.authorizeOpen(user, req); err != nil {
		s.policyRejected.Add(1)
		resp := protocol.OpenResponse{OK: false, Code: "target_denied", Message: err.Error()}
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "open", Result: "denied", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, Code: "target_denied", Message: err.Error()})
		recordRequest(req, "denied", "ok", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
	releaseUserStream, ok := s.userStreams.acquire(req.Username)
	if !ok {
		s.userStreamLimitRejected.Add(1)
		resp := protocol.OpenResponse{OK: false, Code: "user_stream_limit", Message: errUserStreamLimit.Error()}
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "open", Result: "denied", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, Code: "user_stream_limit", Message: resp.Message})
		recordRequest(req, "denied", "ok", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
	defer releaseUserStream()
	dialer := net.Dialer{Timeout: time.Duration(s.cfg.Security.DialTimeoutSec) * time.Second}
	targetConn, err := dialer.DialContext(s.runContext(), "tcp", req.Target)
	if err != nil {
		if s.runContext().Err() != nil {
			return
		}
		s.dialFailed.Add(1)
		resp := protocol.OpenResponse{OK: false, Code: "target_unreachable", Message: "target service is unreachable"}
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "open", Result: "failed", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, Code: "target_unreachable", Message: "target service is unreachable"})
		recordRequest(req, "failed", "ok", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
	defer targetConn.Close()
	resp := serverVersionResponse(protocol.OpenResponse{OK: true, Code: "ok", Message: "connected", CredentialKey: s.activeCredentialPublicKey()})
	recordRequest(req, "ok", "ok", resp, nil)
	if err := protocol.WriteJSON(conn, resp); err != nil {
		return
	}
	_ = conn.SetDeadline(time.Time{})
	s.totalStreams.Add(1)
	s.activeStreams.Add(1)
	defer s.activeStreams.Add(-1)
	started := time.Now()
	s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "open", Result: "connected", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, Code: "ok"})
	targetToClient, clientToTarget := transport.ProxyPairWithOptions(targetConn, conn, &s.bytesUp, &s.bytesDown, s.proxyOptions())
	durationMS := time.Since(started).Milliseconds()
	s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "open", Result: "closed", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, Code: "closed", DurationMS: durationMS, BytesUp: clientToTarget, BytesDown: targetToClient})
}
