package tunnel

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"lsyltunnel/src/internal/passutil"
	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
	appversion "lsyltunnel/src/internal/version"
)

type Server struct {
	lifecycle           serverLifecycle
	cfg                 Config
	listener            net.Listener
	tlsConfig           *tls.Config
	certificateGateKey  transport.CertificateGateKey
	httpSrv             *http.Server
	users               map[string]UserConfig
	fails               *failTracker
	credentialMu        sync.Mutex
	credentialKeys      map[string]*credentialSealKey
	activeCredentialKey *credentialSealKey
	reverseMu           sync.Mutex
	reverseListeners    map[string]*reverseListener
	configuredReverse   map[string]bool
	started             time.Time
	logf                transport.LogFunc
	eventsOnce          sync.Once
	events              *runtimeEventRing
	requestLog          *jsonlLog
	businessLog         *jsonlLog
	entryTrafficLog     *jsonlLog
	entryTrafficWriter  *asyncEntryTrafficLog
	flowTrafficLog      *jsonlLog
	requestSeq          atomic.Uint64
	permanentBlockHits  *permanentBlockHitAggregator
	connLimiter         *connectionLimiter
	userStreams         *userStreamLimiter

	activeStreams                   atomic.Int64
	totalStreams                    atomic.Int64
	userStreamLimitRejected         atomic.Int64
	entryProtocolRejected           atomic.Int64
	entryHTTPProbeRejected          atomic.Int64
	entryNonTLSRejected             atomic.Int64
	entryUnsupportedTLSRejected     atomic.Int64
	entrySlowHandshakeRejected      atomic.Int64
	entryClosedHandshakeRejected    atomic.Int64
	entryOversizedHandshakeRejected atomic.Int64
	entryUnsupportedRequestRejected atomic.Int64
	entryInvalidHandshakeRejected   atomic.Int64
	entryPermanentBlocksCreated     atomic.Int64
	entryPermanentBlockHits         atomic.Int64
	connectionsRejected             atomic.Int64
	connectionsRejectedGlobal       atomic.Int64
	connectionsRejectedPerIPActive  atomic.Int64
	connectionsRejectedPerIPNewRate atomic.Int64
	connectionsRejectedIPCapacity   atomic.Int64
	authOK                          atomic.Int64
	authFailed                      atomic.Int64
	policyRejected                  atomic.Int64
	dialFailed                      atomic.Int64
	bytesUp                         atomic.Int64
	bytesDown                       atomic.Int64
}

var errUserStreamLimit = errors.New("too many concurrent streams for account")

func Run(ctx context.Context, cfg Config, logf transport.LogFunc) error {
	srv, err := Start(ctx, cfg, logf)
	if err != nil {
		return err
	}
	defer srv.Close()
	<-ctx.Done()
	return nil
}

func Start(ctx context.Context, cfg Config, logf transport.LogFunc) (*Server, error) {
	ApplyDefaults(&cfg)
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	cert, err := tls.LoadX509KeyPair(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server TLS identity files: %w", err)
	}
	if len(cert.Certificate) == 0 {
		return nil, fmt.Errorf("load server TLS identity files: certificate chain is empty")
	}
	certificateGateKey, err := transport.CertificateGateKeyFromDER(cert.Certificate[0])
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return nil, err
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   transport.TLSMinVersion(cfg.TLS.MinVersion),
	}
	if strings.TrimSpace(cfg.Runtime.PermanentBlockFile) == "" && strings.TrimSpace(cfg.Runtime.StateFile) != "" {
		cfg.Runtime.PermanentBlockFile = filepath.Join(filepath.Dir(cfg.Runtime.StateFile), "server-permanent-block.txt")
	}
	fails := newFailTracker(cfg.Security, cfg.Runtime.StateFile, cfg.Runtime.PermanentBlockFile)
	if err := fails.load(); err != nil && logf != nil {
		logf("load runtime state failed: %v", err)
	}
	if err := reconcileSuccessfulIPHistory(fails, cfg.Runtime.RequestLogFile, time.Now()); err != nil && logf != nil {
		logf("reconcile successful IP history failed: %v", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	srv := &Server{
		lifecycle:          serverLifecycle{ctx: runCtx, cancel: cancel},
		cfg:                cfg,
		listener:           ln,
		tlsConfig:          tlsCfg,
		certificateGateKey: certificateGateKey,
		users:              make(map[string]UserConfig),
		fails:              fails,
		reverseListeners:   make(map[string]*reverseListener),
		configuredReverse:  map[string]bool{},
		started:            time.Now(),
		logf:               logf,
		events:             newRuntimeEventRing(cfg.Runtime.RecentEvents),
		requestLog:         newJSONLLog(cfg.Runtime.RequestLogFile),
		businessLog:        newJSONLLog(cfg.Runtime.BusinessLogFile),
		entryTrafficLog:    newJSONLLog(cfg.Runtime.EntryTrafficLogFile),
		flowTrafficLog:     newJSONLLog(cfg.Runtime.FlowTrafficLogFile),
		permanentBlockHits: newPermanentBlockHitAggregator(defaultPermanentBlockHitLogInterval),
		connLimiter:        newConnectionLimiter(cfg.Security),
		userStreams:        newUserStreamLimiter(cfg.Security),
	}
	srv.entryTrafficWriter = newAsyncEntryTrafficLog(cfg.Security.EntryTrafficLogQueueSize, srv.writeEntryTrafficLog)
	for _, user := range cfg.Auth.Users {
		srv.users[user.Username] = user
	}
	if err := srv.loadCredentialSealKeys(); err != nil {
		_ = srv.Close()
		return nil, err
	}
	if err := srv.prepareConfiguredForwards(); err != nil {
		_ = srv.Close()
		return nil, err
	}
	srv.startWorker(func() { srv.acceptLoop(runCtx) })
	srv.startWorker(func() { srv.credentialSealRotationLoop(runCtx.Done()) })
	srv.startWorker(func() { srv.permanentBlockHitLogLoop(runCtx.Done()) })
	srv.startWorker(func() { srv.failureTrackerCleanupLoop(runCtx.Done()) })
	srv.startWorker(func() { srv.connectionLimiterCleanupLoop(runCtx.Done()) })
	if strings.TrimSpace(cfg.MonitorAddr) != "" {
		srv.startMonitor(runCtx)
	}
	srv.log("server listening on %s", srv.Addr())
	srv.startWorker(func() { srv.auditConfiguredForwardAvailability(runCtx) })
	// This coordinator must not join the worker group that Close waits for.
	go func() {
		<-runCtx.Done()
		_ = srv.Close()
	}()
	return srv, nil
}

func (s *Server) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

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

func (s *Server) handleConn(conn net.Conn) {
	closeConn := true
	defer func() {
		if closeConn {
			_ = conn.Close()
		}
	}()
	requestID := s.nextRequestID()
	requestStarted := time.Now()
	remoteIP := remoteHost(conn.RemoteAddr())
	remoteAddr := ""
	if conn.RemoteAddr() != nil {
		remoteAddr = conn.RemoteAddr().String()
	}
	localAddr := ""
	if conn.LocalAddr() != nil {
		localAddr = conn.LocalAddr().String()
	}
	recordRequest := func(req protocol.OpenRequest, result, authResult string, resp protocol.OpenResponse, readErr error) {
		entry := RequestLogEntry{
			RequestID:  requestID,
			RemoteAddr: remoteAddr,
			RemoteIP:   remoteIP,
			LocalAddr:  localAddr,
			Request:    req,
			AuthResult: authResult,
			Response:   resp,
			Result:     result,
			DurationMS: time.Since(requestStarted).Milliseconds(),
		}
		if readErr != nil {
			entry.ReadError = readErr.Error()
		}
		s.recordRequestLog(entry)
	}
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
		s.rejectEntryProtocol(conn, remoteIP, requestID, classifyEntryReadError(err), "invalid tunnel request", true, time.Since(requestStarted).Milliseconds())
		return
	}
	if req.Type != "open" && req.Type != "login" && req.Type != "health" && req.Type != "forward_check" && req.Type != "reverse" && req.Type != "reverse_listen" && req.Type != "reverse_stream" {
		s.rejectEntryProtocol(conn, remoteIP, requestID, entryCodeUnsupportedRequest, "unsupported request", false, time.Since(requestStarted).Milliseconds())
		return
	}
	if resp, ok := s.checkRequestCompatibility(req); !ok {
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "compatibility", Result: "rejected", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, ListenAddr: req.ListenAddr, Code: resp.Code, Message: resp.Message})
		recordRequest(req, "rejected", "not_attempted", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
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
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "auth", Result: result, RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, ListenAddr: req.ListenAddr, Code: authCode, Message: authMessage})
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
	if req.Type == "reverse" || req.Type == "reverse_listen" {
		if err := s.registerReverseControl(conn, req, user, requestID); err != nil {
			s.policyRejected.Add(1)
			resp := protocol.OpenResponse{OK: false, Code: "reverse_failed", Message: err.Error()}
			s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "reverse_listen", Result: "failed", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, ListenAddr: req.ListenAddr, Code: "reverse_failed", Message: err.Error()})
			recordRequest(req, "failed", "ok", resp, nil)
			_ = protocol.WriteJSON(conn, resp)
			return
		}
		recordRequest(req, "ok", "ok", serverVersionResponse(protocol.OpenResponse{OK: true, Code: "ok", Message: "reverse listen activated", CredentialKey: s.activeCredentialPublicKey()}), nil)
		closeConn = false
		return
	}
	if req.Type == "reverse_stream" {
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
			return
		}
		recordRequest(req, "ok", "ok", serverVersionResponse(protocol.OpenResponse{OK: true, Code: "ok", Message: "connected"}), nil)
		closeConn = false
		return
	}
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
		flowLog := s.flowTrafficEntry(requestID, "stream_rejected", "open", "rejected", remoteIP, req)
		flowLog.Code = "user_stream_limit"
		flowLog.Message = resp.Message
		flowLog.Abnormal = true
		s.recordFlowTrafficLog(flowLog)
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
		flowLog := s.flowTrafficEntry(requestID, "stream_failed", "open", "failed", remoteIP, req)
		flowLog.Code = "target_unreachable"
		flowLog.Message = resp.Message
		flowLog.Abnormal = true
		s.recordFlowTrafficLog(flowLog)
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
	flowLog := s.flowTrafficEntry(requestID, "stream_closed", "open", "closed", remoteIP, req)
	flowLog.Code = "closed"
	flowLog.DurationMS = durationMS
	flowLog.BytesUp = clientToTarget
	flowLog.BytesDown = targetToClient
	s.recordFlowTrafficLog(flowLog)
	s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "open", Result: "closed", RemoteIP: remoteIP, Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, Code: "closed", DurationMS: durationMS, BytesUp: clientToTarget, BytesDown: targetToClient})
}

func (s *Server) proxyOptions() transport.ProxyOptions {
	return transport.ProxyOptions{RateLimitBytesPerSec: s.cfg.Security.StreamRateLimitBytesPerSec}
}

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

func (s *Server) flowTrafficEntry(requestID, event, kind, result, remoteIP string, req protocol.OpenRequest) FlowTrafficLogEntry {
	return FlowTrafficLogEntry{
		RequestID:   requestID,
		Event:       event,
		Kind:        kind,
		Result:      result,
		RemoteIP:    remoteIP,
		Username:    req.Username,
		ClientID:    req.ClientID,
		ForwardName: req.ForwardName,
		Direction:   req.Direction,
		Target:      req.Target,
		ListenAddr:  req.ListenAddr,
	}
}
