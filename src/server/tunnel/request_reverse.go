package tunnel

import (
	"errors"
	"net"

	"lsyltunnel/src/internal/protocol"
)

// A true return transfers connection ownership to the reverse listener or stream.
func (s *Server) handleReverseControl(conn net.Conn, req protocol.OpenRequest, user UserConfig, request *requestContext) bool {
	requestID, remoteIP := request.id, request.remoteIP
	recordRequest := request.record
	if err := s.registerReverseControl(conn, req, user, requestID); err != nil {
		s.policyRejected.Add(1)
		resp := protocol.OpenResponse{OK: false, Code: "reverse_failed", Message: err.Error()}
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "reverse_listen", Result: "failed", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, ListenAddr: req.ListenAddr, Code: "reverse_failed", Message: err.Error()})
		recordRequest(req, "failed", "ok", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return false
	}
	recordRequest(req, "ok", "ok", serverVersionResponse(protocol.OpenResponse{OK: true, Code: "ok", Message: "reverse listen activated", CredentialKey: s.activeCredentialPublicKey()}), nil)
	return true
}

func (s *Server) handleReverseStream(conn net.Conn, req protocol.OpenRequest, user UserConfig, request *requestContext) bool {
	requestID, remoteIP := request.id, request.remoteIP
	recordRequest := request.record
	if err := s.registerReverseStream(conn, req, user, requestID); err != nil {
		code := "reverse_failed"
		if errors.Is(err, errUserStreamLimit) {
			code = "user_stream_limit"
		} else {
			s.dialFailed.Add(1)
		}
		resp := protocol.OpenResponse{OK: false, Code: code, Message: err.Error()}
		flowLog := s.flowTrafficEntry(requestID, "stream_failed", "reverse_stream", "failed", remoteIP, req)
		flowLog.Code = code
		flowLog.Message = err.Error()
		flowLog.Abnormal = true
		s.recordFlowTrafficLog(flowLog)
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "reverse_stream", Result: "failed", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, ListenAddr: req.ListenAddr, Code: code, Message: err.Error()})
		recordRequest(req, "failed", "ok", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return false
	}
	recordRequest(req, "ok", "ok", serverVersionResponse(protocol.OpenResponse{OK: true, Code: "ok", Message: "connected"}), nil)
	return true
}
