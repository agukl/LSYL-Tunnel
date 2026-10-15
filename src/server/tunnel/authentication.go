package tunnel

import (
	"lsyltunnel/src/internal/passutil"
	"lsyltunnel/src/internal/protocol"
)

// authenticateRequest keeps failure accounting independent of request dispatch.
// An empty result denotes success; authOK is counted by the business dispatcher.
func (s *Server) authenticateRequest(req protocol.OpenRequest, remoteIP string) (UserConfig, protocol.OpenResponse, string) {
	user, ok := s.users[req.Username]
	password, authCode, authMessage := s.passwordFromRequest(req)
	if !ok || user.Disabled || authCode != "" || !passutil.VerifyPassword(password, user.PasswordHash) {
		s.authFailed.Add(1)
		s.fails.addFailure(remoteIP)
		temporarilyBlocked := s.fails.blockKind(remoteIP) == blockedTemporary
		if temporarilyBlocked {
			authCode = "auth_blocked"
			authMessage = "too many login failures, temporarily blocked; try later"
		} else if authCode == "" {
			authCode = "auth_failed"
			authMessage = "username or password is incorrect"
		}
		result := "failed"
		if temporarilyBlocked {
			result = "blocked"
		}
		resp := protocol.OpenResponse{OK: false, Code: authCode, Message: authMessage}
		return UserConfig{}, resp, result
	}
	return user, protocol.OpenResponse{}, ""
}
