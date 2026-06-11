package tunnel

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"

	"lsyltunnel/src/internal/protocol"
)

const (
	entryCodeHTTPProbe          = "http_probe"
	entryCodeNonTLSProbe        = "non_tls_probe"
	entryCodeUnsupportedTLS     = "unsupported_tls_version"
	entryCodeSlowHandshake      = "slow_handshake"
	entryCodeClosedHandshake    = "handshake_closed"
	entryCodeOversizedHandshake = "oversized_tunnel_handshake"
	entryCodeUnsupportedRequest = "unsupported_tunnel_request"
	entryCodeInvalidHandshake   = "invalid_tunnel_handshake"
)

func classifyEntryReadError(err error) string {
	if err == nil {
		return entryCodeInvalidHandshake
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "client sent an HTTP request to an HTTPS server"):
		return entryCodeHTTPProbe
	case strings.Contains(text, "first record does not look like a TLS handshake"):
		return entryCodeNonTLSProbe
	case strings.Contains(text, "client offered only unsupported versions"):
		return entryCodeUnsupportedTLS
	case strings.Contains(text, "handshake too large:"):
		if oversizedHandshakeHTTPMethod(text) != "" {
			return entryCodeHTTPProbe
		}
		return entryCodeOversizedHandshake
	case strings.Contains(text, "i/o timeout"):
		return entryCodeSlowHandshake
	case strings.Contains(text, "forcibly closed") || strings.Contains(text, "EOF"):
		return entryCodeClosedHandshake
	default:
		return entryCodeInvalidHandshake
	}
}

func classifyEntryHandshakeError(err error, prefix []byte) string {
	if entryHTTPMethodFromPrefix(prefix) != "" {
		return entryCodeHTTPProbe
	}
	if err == nil {
		return entryCodeInvalidHandshake
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "client sent an HTTP request to an HTTPS server"):
		return entryCodeHTTPProbe
	case strings.Contains(text, "first record does not look like a TLS handshake"):
		return entryCodeNonTLSProbe
	case strings.Contains(text, "client offered only unsupported versions"):
		return entryCodeUnsupportedTLS
	case strings.Contains(text, "i/o timeout"):
		return entryCodeSlowHandshake
	case strings.Contains(text, "forcibly closed") || strings.Contains(text, "EOF"):
		return entryCodeClosedHandshake
	default:
		if len(prefix) > 0 && !entryLooksLikeTLSHandshake(prefix) {
			return entryCodeNonTLSProbe
		}
		return entryCodeInvalidHandshake
	}
}

func oversizedHandshakeHTTPMethod(text string) string {
	const marker = "handshake too large:"
	idx := strings.Index(text, marker)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(text[idx+len(marker):])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	value, err := strconv.ParseUint(fields[0], 10, 32)
	if err != nil {
		return ""
	}
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(value))
	token := strings.ToUpper(string(prefix[:]))
	switch token {
	case "GET ", "POST", "HEAD", "PUT ", "DELE", "OPTI", "CONN", "TRAC", "PATC", "PROP":
		return strings.TrimSpace(token)
	default:
		return ""
	}
}

func entryHTTPMethodFromPrefix(prefix []byte) string {
	if len(prefix) == 0 {
		return ""
	}
	token := strings.ToUpper(string(prefix))
	for _, method := range []string{"GET ", "POST", "HEAD", "PUT ", "DELE", "OPTI", "CONN", "TRAC", "PATC", "PROP"} {
		if strings.HasPrefix(token, method) {
			return strings.TrimSpace(method)
		}
	}
	return ""
}

func entryLooksLikeTLSHandshake(prefix []byte) bool {
	return len(prefix) > 0 && prefix[0] == 0x16
}

type entryPeekConn struct {
	net.Conn
	prefix []byte
}

func (c *entryPeekConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && len(c.prefix) < 16 {
		limit := 16 - len(c.prefix)
		if n < limit {
			limit = n
		}
		c.prefix = append(c.prefix, p[:limit]...)
	}
	return n, err
}

func (c *entryPeekConn) Prefix() []byte {
	if c == nil || len(c.prefix) == 0 {
		return nil
	}
	out := make([]byte, len(c.prefix))
	copy(out, c.prefix)
	return out
}

func (s *Server) countEntryProtocolRejected(code string) {
	if s == nil {
		return
	}
	s.entryProtocolRejected.Add(1)
	switch code {
	case entryCodeHTTPProbe:
		s.entryHTTPProbeRejected.Add(1)
	case entryCodeNonTLSProbe:
		s.entryNonTLSRejected.Add(1)
	case entryCodeUnsupportedTLS:
		s.entryUnsupportedTLSRejected.Add(1)
	case entryCodeSlowHandshake:
		s.entrySlowHandshakeRejected.Add(1)
	case entryCodeClosedHandshake:
		s.entryClosedHandshakeRejected.Add(1)
	case entryCodeOversizedHandshake:
		s.entryOversizedHandshakeRejected.Add(1)
	case entryCodeUnsupportedRequest:
		s.entryUnsupportedRequestRejected.Add(1)
	default:
		s.entryInvalidHandshakeRejected.Add(1)
	}
}

func (s *Server) countEntryPermanentBlockCreated() {
	if s != nil {
		s.entryPermanentBlocksCreated.Add(1)
	}
}

func (s *Server) countEntryPermanentBlockHit() {
	if s != nil {
		s.entryPermanentBlockHits.Add(1)
	}
}

func (s *Server) rejectEntryProtocol(conn net.Conn, remoteIP, requestID, entryCode, responseMessage string, countAuthFailed bool, durationMS int64) {
	if s == nil {
		return
	}
	if countAuthFailed {
		s.authFailed.Add(1)
	}
	s.countEntryProtocolRejected(entryCode)
	permanentBlockCreated := s.fails.addProtocolFailure(remoteIP)
	if permanentBlockCreated {
		s.countEntryPermanentBlockCreated()
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "auth", Result: "blocked", RemoteIP: remoteIP, Code: "ip_permanently_blocked", Message: "too many invalid tunnel requests"})
	}
	remoteAddr := ""
	localAddr := ""
	if conn != nil {
		remoteAddr = addrString(conn.RemoteAddr())
		localAddr = addrString(conn.LocalAddr())
	}
	s.recordEntryTrafficLog(EntryTrafficLogEntry{
		RequestID:             requestID,
		Event:                 "protocol_rejected",
		Result:                "rejected",
		RemoteAddr:            remoteAddr,
		RemoteIP:              remoteIP,
		LocalAddr:             localAddr,
		Code:                  entryCode,
		Message:               responseMessage,
		Abnormal:              true,
		DurationMS:            durationMS,
		PermanentBlockCreated: permanentBlockCreated,
	})
	if conn != nil && responseMessage != "" {
		_ = protocol.WriteJSON(conn, protocol.OpenResponse{OK: false, Code: "bad_request", Message: responseMessage})
	}
}
