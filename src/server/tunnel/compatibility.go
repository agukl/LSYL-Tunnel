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
	if req.ProtocolVersion != appversion.ProtocolVersion {
		return protocol.OpenResponse{
			OK:      false,
			Code:    "protocol_version_unsupported",
			Message: fmt.Sprintf("protocol_version %d is not supported by this server; required protocol_version is %d", req.ProtocolVersion, appversion.ProtocolVersion),
		}, false
	}

	clientVersion := strings.TrimSpace(req.ClientVersion)
	if err := appversion.Validate(clientVersion); err != nil {
		return protocol.OpenResponse{
			OK:      false,
			Code:    "client_version_unsupported",
			Message: fmt.Sprintf("client_version %q is invalid", clientVersion),
		}, false
	}
	less, err := appversion.Less(clientVersion, appversion.MinCompatibleClientVersion)
	if err != nil {
		return protocol.OpenResponse{OK: false, Code: "client_version_unsupported", Message: err.Error()}, false
	}
	if less {
		return protocol.OpenResponse{
			OK:      false,
			Code:    "client_version_unsupported",
			Message: fmt.Sprintf("client_version %s is not supported by this server; minimum client_version is %s", clientVersion, appversion.MinCompatibleClientVersion),
		}, false
	}
	if belowNextMajor, err := appversion.Less(clientVersion, "4.0.0"); err != nil || !belowNextMajor {
		return protocol.OpenResponse{OK: false, Code: "client_version_unsupported", Message: "only 3.x clients are supported"}, false
	}
	return protocol.OpenResponse{}, true
}
