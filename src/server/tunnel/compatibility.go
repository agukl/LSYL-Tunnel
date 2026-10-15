package tunnel

import (
	"fmt"
	"strings"

	"lsyltunnel/src/internal/protocol"
	appversion "lsyltunnel/src/internal/version"
)

func serverVersionResponse(resp protocol.OpenResponse) protocol.OpenResponse {
	if resp.OK && strings.TrimSpace(resp.ServerVersion) == "" {
		resp.ServerVersion = appversion.AppVersion
	}
	return resp
}

func (s *Server) checkRequestCompatibility(req protocol.OpenRequest) (protocol.OpenResponse, bool) {
	serverProtocol := s.cfg.Compatibility.ProtocolVersion
	if serverProtocol == 0 {
		serverProtocol = appversion.ProtocolVersion
	}
	requestProtocol := req.ProtocolVersion
	if requestProtocol == 0 {
		requestProtocol = appversion.LegacyProtocolVersion
	}
	if requestProtocol != serverProtocol {
		return protocol.OpenResponse{
			OK:      false,
			Code:    "protocol_version_unsupported",
			Message: fmt.Sprintf("protocol_version %d is not supported by this server; required protocol_version is %d", requestProtocol, serverProtocol),
		}, false
	}

	clientVersion := strings.TrimSpace(req.ClientVersion)
	if clientVersion == "" {
		clientVersion = appversion.LegacyClientVersion
	}
	if err := appversion.Validate(clientVersion); err != nil {
		return protocol.OpenResponse{
			OK:      false,
			Code:    "client_version_unsupported",
			Message: fmt.Sprintf("client_version %q is invalid", clientVersion),
		}, false
	}
	minClientVersion := strings.TrimSpace(s.cfg.Compatibility.MinClientVersion)
	if minClientVersion != "" {
		less, err := appversion.Less(clientVersion, minClientVersion)
		if err != nil {
			return protocol.OpenResponse{OK: false, Code: "client_version_unsupported", Message: err.Error()}, false
		}
		if less {
			return protocol.OpenResponse{
				OK:      false,
				Code:    "client_version_unsupported",
				Message: fmt.Sprintf("client_version %s is not supported by this server; minimum client_version is %s", clientVersion, minClientVersion),
			}, false
		}
	}
	maxClientVersion := strings.TrimSpace(s.cfg.Compatibility.MaxClientVersion)
	if maxClientVersion != "" {
		greater, err := appversion.Greater(clientVersion, maxClientVersion)
		if err != nil {
			return protocol.OpenResponse{OK: false, Code: "client_version_unsupported", Message: err.Error()}, false
		}
		if greater {
			return protocol.OpenResponse{
				OK:      false,
				Code:    "client_version_unsupported",
				Message: fmt.Sprintf("client_version %s is not supported by this server; maximum client_version is %s", clientVersion, maxClientVersion),
			}, false
		}
	}
	return protocol.OpenResponse{}, true
}
