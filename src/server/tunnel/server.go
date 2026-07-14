package tunnel

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
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
	cfg                 Config
	listener            net.Listener
	tlsConfig           *tls.Config
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
	eventMu             sync.Mutex
	recentEvents        []RuntimeEvent
	maxRecentEvents     int
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
	srv := &Server{
		cfg:                cfg,
		listener:           ln,
		tlsConfig:          tlsCfg,
		users:              make(map[string]UserConfig),
		fails:              fails,
		reverseListeners:   make(map[string]*reverseListener),
		configuredReverse:  map[string]bool{},
		started:            time.Now(),
		logf:               logf,
		maxRecentEvents:    cfg.Runtime.RecentEvents,
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
	go srv.acceptLoop(ctx)
	go srv.credentialSealRotationLoop(ctx.Done())
	go srv.permanentBlockHitLogLoop(ctx.Done())
	go srv.failureTrackerCleanupLoop(ctx.Done())
	go srv.connectionLimiterCleanupLoop(ctx.Done())
	if strings.TrimSpace(cfg.MonitorAddr) != "" {
		srv.startMonitor(ctx)
	}
	srv.log("server listening on %s", srv.Addr())
	go srv.auditConfiguredForwardAvailability(ctx)
	return srv, nil
}

func (s *Server) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var err error
	if s.listener != nil {
		err = s.listener.Close()
	}
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(ctx)
	}
	s.closeReverseListeners()
	s.flushPermanentBlockHitLogs()
	if s.entryTrafficWriter != nil {
		s.entryTrafficWriter.Close()
	}
	if s.requestLog != nil {
		_ = s.requestLog.Close()
	}
	if s.businessLog != nil {
		_ = s.businessLog.Close()
	}
	if s.entryTrafficLog != nil {
		_ = s.entryTrafficLog.Close()
	}
	if s.flowTrafficLog != nil {
		_ = s.flowTrafficLog.Close()
	}
	return err
}

func (s *Server) acceptLoop(ctx context.Context) {
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
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
		go func() {
			defer release()
			s.handleAcceptedConn(conn, acceptedAt)
		}()
	}
}

func (s *Server) handleAcceptedConn(conn net.Conn, acceptedAt time.Time) {
	peekConn := &entryPeekConn{Conn: conn}
	tlsConn := tls.Server(peekConn, s.tlsConfig)
	_ = tlsConn.SetDeadline(time.Now().Add(time.Duration(s.cfg.Security.HandshakeTimeoutSec) * time.Second))
	if err := tlsConn.Handshake(); err != nil {
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
		resp := protocol.OpenResponse{OK: false, Code: "auth_blocked", Message: "too many login failures, temporarily blocked; try later"}
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "auth", Result: "blocked", RemoteIP: remoteIP, Code: "auth_blocked", Message: "too many login failures"})
		recordRequest(protocol.OpenRequest{}, "blocked", "blocked", resp, nil)
		_ = protocol.WriteJSON(conn, resp)
		return
	}
	_ = conn.SetDeadline(time.Now().Add(time.Duration(s.cfg.Security.HandshakeTimeoutSec) * time.Second))
	var req protocol.OpenRequest
	if err := protocol.ReadJSON(conn, &req, s.cfg.Security.MaxHandshakeBytes); err != nil {
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
	targetConn, err := net.DialTimeout("tcp", req.Target, time.Duration(s.cfg.Security.DialTimeoutSec)*time.Second)
	if err != nil {
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

func (s *Server) authorize(user UserConfig, target string) error {
	return s.authorizeTarget(user, target)
}

func (s *Server) authorizeOpen(user UserConfig, req protocol.OpenRequest) error {
	if direction := strings.TrimSpace(req.Direction); direction != "" && direction != DirectionClientToServer {
		return fmt.Errorf("request direction is invalid")
	}
	return s.authorizeTarget(user, req.Target)
}

func (s *Server) authorizeForwardCheck(user UserConfig, req protocol.OpenRequest) error {
	direction := strings.TrimSpace(req.Direction)
	if direction == "" {
		direction = DirectionClientToServer
	}
	switch direction {
	case DirectionClientToServer:
		return s.authorizeTarget(user, req.Target)
	case DirectionServerToClient:
		listenAddr := strings.TrimSpace(req.ListenAddr)
		if listenAddr == "" {
			return fmt.Errorf("reverse listen address is required")
		}
		if !s.isConfiguredReverseListener(listenAddr) {
			return fmt.Errorf("reverse listen address is not configured on server")
		}
		return s.authorizeReverse(user, listenAddr)
	default:
		return fmt.Errorf("request direction is invalid")
	}
}

func (s *Server) authorizeReverse(user UserConfig, listenAddr string) error {
	username := strings.TrimSpace(user.Username)
	normalizedListenAddr := normalizeTarget(listenAddr)
	for i := range s.cfg.Forwards {
		fwd := &s.cfg.Forwards[i]
		if strings.TrimSpace(fwd.Direction) != DirectionServerToClient {
			continue
		}
		configuredAddr := configuredReverseListenAddr(*fwd)
		if configuredAddr == "" || normalizeTarget(configuredAddr) != normalizedListenAddr {
			continue
		}
		if allowedUser(fwd.AllowedUsers, username) {
			return nil
		}
		break
	}
	return fmt.Errorf("user is not allowed to activate this reverse listen address")
}

func configuredReverseListenAddr(fwd ForwardConfig) string {
	if addr := strings.TrimSpace(fwd.ListenAddr); addr != "" {
		return addr
	}
	if fwd.ListenPort > 0 && fwd.ListenPort <= 65535 {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(fwd.ListenPort))
	}
	return ""
}

func (s *Server) authorizeTarget(user UserConfig, target string) error {
	host, _, err := splitHostPort(target)
	if err != nil {
		return fmt.Errorf("target address is invalid")
	}
	forward := s.allowedConfiguredForward(user.Username, target)
	if forward == nil {
		return fmt.Errorf("user is not allowed to access this target")
	}
	if isLoopbackName(host) {
		return nil
	}
	if isPrivateTargetIP(host) {
		return nil
	}
	return fmt.Errorf("target is not allowed")
}

func (s *Server) allowedConfiguredForward(username, target string) *ForwardConfig {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	normalizedTarget := normalizeTarget(target)
	for i := range s.cfg.Forwards {
		fwd := &s.cfg.Forwards[i]
		configuredDirection := strings.TrimSpace(fwd.Direction)
		if configuredDirection == "" {
			configuredDirection = DirectionClientToServer
		}
		if configuredDirection != DirectionClientToServer {
			continue
		}
		if !allowedUser(fwd.AllowedUsers, username) {
			continue
		}
		forwardTarget := strings.TrimSpace(fwd.ServerTarget)
		if forwardTarget != "" && normalizeTarget(forwardTarget) == normalizedTarget {
			return fwd
		}
	}
	return nil
}

func allowedUser(users []string, username string) bool {
	for _, user := range users {
		if strings.TrimSpace(user) == username {
			return true
		}
	}
	return false
}

func (s *Server) startMonitor(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.status())
	})
	mux.HandleFunc("/security/unblock", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "method not allowed"})
			return
		}
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid request"})
			return
		}
		ip := strings.TrimSpace(req.IP)
		if net.ParseIP(ip) == nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid ip"})
			return
		}
		removed, err := s.fails.unblock(ip)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "unblocked", "removed": removed})
	})
	mux.HandleFunc("/security/permanent-block/delete", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "method not allowed"})
			return
		}
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid request"})
			return
		}
		ip := strings.TrimSpace(req.IP)
		if net.ParseIP(ip) == nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "invalid ip"})
			return
		}
		removed, err := s.fails.unblockPermanent(ip)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "permanent block deleted", "removed": removed})
	})
	s.httpSrv = &http.Server{Addr: s.cfg.MonitorAddr, Handler: mux}
	go func() {
		<-ctx.Done()
		if s.httpSrv != nil {
			_ = s.httpSrv.Close()
		}
	}()
	go func() {
		err := s.httpSrv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log("monitor failed: %v", err)
		}
	}()
}

func (s *Server) status() map[string]any {
	connectionLimits := s.connLimiter.snapshot()
	userStreamLimits := s.userStreams.snapshot()
	failureStats := s.fails.stats()
	entryTrafficStats := s.entryTrafficWriter.stats()
	return map[string]any{
		"service":              "lsyl-tunnel-server",
		"listen_addr":          s.Addr(),
		"uptime_sec":           int64(time.Since(s.started).Seconds()),
		"active_connections":   connectionLimits["active"],
		"tracked_remote_ips":   connectionLimits["tracked_ips"],
		"connections_rejected": s.connectionsRejected.Load(),
		"connection_limits": map[string]any{
			"active":                              connectionLimits["active"],
			"tracked_remote_ips":                  connectionLimits["tracked_ips"],
			"max_tracked_remote_ips":              connectionLimits["max_tracked_ips"],
			"tracked_remote_ip_evictions":         connectionLimits["evicted_ips"],
			"tracked_remote_ip_capacity_rejected": connectionLimits["capacity_rejected"],
			"max_concurrent_connections":          s.cfg.Security.MaxConcurrentConnections,
			"max_concurrent_connections_per_ip":   s.cfg.Security.MaxConcurrentConnectionsPerIP,
			"new_connection_rate_window_sec":      s.cfg.Security.ConnectionRateWindowSec,
			"max_new_connections_per_ip_window":   s.cfg.Security.MaxNewConnectionsPerIPWindow,
		},
		"connection_rejections": map[string]any{
			"total":                      s.connectionsRejected.Load(),
			"global_concurrent":          s.connectionsRejectedGlobal.Load(),
			"per_ip_concurrent":          s.connectionsRejectedPerIPActive.Load(),
			"per_ip_new_connection_rate": s.connectionsRejectedPerIPNewRate.Load(),
			"tracked_remote_ip_capacity": s.connectionsRejectedIPCapacity.Load(),
		},
		"entry_security": map[string]any{
			"protocol_rejected_total":       s.entryProtocolRejected.Load(),
			"http_probe_rejected":           s.entryHTTPProbeRejected.Load(),
			"non_tls_rejected":              s.entryNonTLSRejected.Load(),
			"unsupported_tls_rejected":      s.entryUnsupportedTLSRejected.Load(),
			"slow_handshake_rejected":       s.entrySlowHandshakeRejected.Load(),
			"closed_handshake_rejected":     s.entryClosedHandshakeRejected.Load(),
			"oversized_handshake_rejected":  s.entryOversizedHandshakeRejected.Load(),
			"unsupported_request_rejected":  s.entryUnsupportedRequestRejected.Load(),
			"invalid_handshake_rejected":    s.entryInvalidHandshakeRejected.Load(),
			"permanent_blocks_created":      s.entryPermanentBlocksCreated.Load(),
			"permanent_block_hits_observed": s.entryPermanentBlockHits.Load(),
		},
		"failure_tracker": map[string]any{
			"tracked_ips":     failureStats.TrackedIPs,
			"max_tracked_ips": failureStats.MaxTrackedIPs,
			"evicted_ips":     failureStats.EvictedIPs,
			"capacity_drops":  failureStats.CapacityDrops,
		},
		"entry_traffic_log": map[string]any{
			"queue_capacity": entryTrafficStats.Capacity,
			"queue_pending":  entryTrafficStats.Queued,
			"accepted":       entryTrafficStats.Accepted,
			"written":        entryTrafficStats.Written,
			"dropped":        entryTrafficStats.Dropped,
		},
		"user_stream_limits": map[string]any{
			"active":                             userStreamLimits.Active,
			"tracked_users":                      userStreamLimits.TrackedUsers,
			"active_by_user":                     userStreamLimits.ActiveByUser,
			"max_concurrent_streams_per_user":    userStreamLimits.MaxPerUser,
			"stream_rate_limit_bytes_per_sec":    s.cfg.Security.StreamRateLimitBytesPerSec,
			"user_stream_limit_rejections_total": s.userStreamLimitRejected.Load(),
		},
		"user_stream_limit_rejected": s.userStreamLimitRejected.Load(),
		"active_streams":             s.activeStreams.Load(),
		"total_streams":              s.totalStreams.Load(),
		"auth_ok":                    s.authOK.Load(),
		"auth_failed":                s.authFailed.Load(),
		"policy_rejected":            s.policyRejected.Load(),
		"dial_failed":                s.dialFailed.Load(),
		"bytes_up":                   s.bytesUp.Load(),
		"bytes_down":                 s.bytesDown.Load(),
		"blocked_ips":                s.fails.snapshotBlocked(),
		"recent_events":              s.recentEventSnapshot(),
	}
}

func (s *Server) failureTrackerCleanupLoop(done <-chan struct{}) {
	if s == nil || s.fails == nil {
		return
	}
	ticker := time.NewTicker(s.fails.cleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := s.fails.cleanup(); err != nil {
				s.log("failure tracker cleanup failed: %v", err)
			}
		}
	}
}

func (s *Server) connectionLimiterCleanupLoop(done <-chan struct{}) {
	if s == nil || s.connLimiter == nil {
		return
	}
	ticker := time.NewTicker(s.connLimiter.cleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s.connLimiter.cleanup()
		}
	}
}

func (s *Server) recordConnectionRejected(reason string) {
	s.connectionsRejected.Add(1)
	switch reason {
	case "global_concurrent_connections":
		s.connectionsRejectedGlobal.Add(1)
	case "per_ip_concurrent_connections":
		s.connectionsRejectedPerIPActive.Add(1)
	case "per_ip_new_connection_rate":
		s.connectionsRejectedPerIPNewRate.Add(1)
	case "tracked_remote_ip_capacity":
		s.connectionsRejectedIPCapacity.Add(1)
	}
}

func (s *Server) connectionRejectLimit(reason string) (int, int) {
	switch reason {
	case "global_concurrent_connections":
		return s.cfg.Security.MaxConcurrentConnections, 0
	case "per_ip_concurrent_connections":
		return s.cfg.Security.MaxConcurrentConnectionsPerIP, 0
	case "per_ip_new_connection_rate":
		return s.cfg.Security.MaxNewConnectionsPerIPWindow, s.cfg.Security.ConnectionRateWindowSec
	case "tracked_remote_ip_capacity":
		return s.cfg.Security.MaxTrackedConnectionIPs, 0
	default:
		return 0, 0
	}
}

func (s *Server) log(format string, args ...any) {
	if s.logf != nil {
		s.logf(format, args...)
	}
}

type failTracker struct {
	mu            sync.RWMutex
	window        time.Duration
	limit         int
	blockFor      time.Duration
	maxItems      int
	cleanupEvery  time.Duration
	now           func() time.Time
	items         map[string]*failState
	permanent     sync.Map
	stateFile     string
	permanentFile string
	evicted       atomic.Uint64
	capacityDrop  atomic.Uint64
}

type blockKind int

const (
	blockedNone blockKind = iota
	blockedTemporary
	blockedPermanent
)

type failState struct {
	authFailures     []time.Time
	protocolFailures []time.Time
	blockedUntil     time.Time
	lastSeen         time.Time
}

func newFailTracker(cfg SecurityConfig, stateFile, permanentFile string) *failTracker {
	maxItems := cfg.MaxTrackedFailureIPs
	if maxItems <= 0 {
		maxItems = 8192
	}
	cleanupEvery := time.Duration(cfg.FailureTrackerCleanupSec) * time.Second
	if cleanupEvery <= 0 {
		cleanupEvery = time.Minute
	}
	return &failTracker{
		window:        time.Duration(cfg.AuthFailWindowSec) * time.Second,
		limit:         cfg.AuthFailThreshold,
		blockFor:      time.Duration(cfg.AuthFailBlockSec) * time.Second,
		maxItems:      maxItems,
		cleanupEvery:  cleanupEvery,
		now:           time.Now,
		items:         map[string]*failState{},
		stateFile:     strings.TrimSpace(stateFile),
		permanentFile: strings.TrimSpace(permanentFile),
	}
}

func (f *failTracker) isBlocked(key string) bool {
	return f.blockKind(key) != blockedNone
}

func (f *failTracker) blockKind(key string) blockKind {
	if f.hasPermanent(key) {
		return blockedPermanent
	}
	f.mu.RLock()
	state := f.items[key]
	if state == nil {
		f.mu.RUnlock()
		return blockedNone
	}
	blockedUntil := state.blockedUntil
	if state.blockedUntil.IsZero() {
		f.mu.RUnlock()
		return blockedNone
	}
	f.mu.RUnlock()
	now := f.now()
	if now.Before(blockedUntil) {
		return blockedTemporary
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	state = f.items[key]
	if state == nil {
		return blockedNone
	}
	if f.hasPermanent(key) {
		return blockedPermanent
	}
	if state.blockedUntil.IsZero() || now.Before(state.blockedUntil) {
		if state.blockedUntil.IsZero() {
			return blockedNone
		}
		return blockedTemporary
	}
	state.blockedUntil = time.Time{}
	if len(state.authFailures) == 0 && len(state.protocolFailures) == 0 {
		delete(f.items, key)
	}
	_ = f.saveLocked()
	return blockedNone
}

func (f *failTracker) addFailure(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	state, ok := f.trackStateLocked(key, now)
	if !ok {
		return
	}
	if f.hasPermanent(key) {
		return
	}
	state.authFailures = appendRecentFailures(state.authFailures, now.Add(-f.window))
	state.authFailures = append(state.authFailures, now)
	state.lastSeen = now
	if len(state.authFailures) >= f.limit {
		state.blockedUntil = now.Add(f.blockFor)
		state.authFailures = nil
		_ = f.saveLocked()
	}
}

func (f *failTracker) addProtocolFailure(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	state, ok := f.trackStateLocked(key, now)
	if !ok {
		return false
	}
	if f.hasPermanent(key) {
		return false
	}
	state.protocolFailures = appendRecentFailures(state.protocolFailures, now.Add(-f.window))
	state.protocolFailures = append(state.protocolFailures, now)
	state.lastSeen = now
	if len(state.protocolFailures) >= f.limit {
		state.blockedUntil = time.Time{}
		state.authFailures = nil
		state.protocolFailures = nil
		return f.blockPermanentLocked(key)
	}
	return false
}

func (f *failTracker) blockPermanentLocked(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if f.hasPermanent(key) {
		return false
	}
	if err := appendPermanentBlockedIP(f.permanentFile, key); err != nil {
		return false
	}
	f.permanent.Store(key, struct{}{})
	delete(f.items, key)
	_ = f.saveLocked()
	return true
}

func (f *failTracker) hasPermanent(key string) bool {
	if f == nil {
		return false
	}
	_, ok := f.permanent.Load(key)
	return ok
}

type failTrackerStats struct {
	TrackedIPs    int
	MaxTrackedIPs int
	EvictedIPs    uint64
	CapacityDrops uint64
}

func (f *failTracker) trackStateLocked(key string, now time.Time) (*failState, bool) {
	key = strings.TrimSpace(key)
	if key == "" || f.hasPermanent(key) {
		return nil, false
	}
	if state := f.items[key]; state != nil {
		state.lastSeen = now
		return state, true
	}
	_ = f.cleanupLocked(now)
	if len(f.items) >= f.maxItems {
		oldestKey := ""
		var oldest time.Time
		for candidate, state := range f.items {
			if state == nil || (!state.blockedUntil.IsZero() && state.blockedUntil.After(now)) {
				continue
			}
			if oldestKey == "" || state.lastSeen.Before(oldest) {
				oldestKey = candidate
				oldest = state.lastSeen
			}
		}
		if oldestKey == "" {
			f.capacityDrop.Add(1)
			return nil, false
		}
		delete(f.items, oldestKey)
		f.evicted.Add(1)
	}
	state := &failState{lastSeen: now}
	f.items[key] = state
	return state, true
}

func appendRecentFailures(items []time.Time, cutoff time.Time) []time.Time {
	kept := items[:0]
	for _, item := range items {
		if item.After(cutoff) {
			kept = append(kept, item)
		}
	}
	return kept
}

func (f *failTracker) cleanup() error {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cleanupLocked(f.now()) {
		return nil
	}
	return f.saveLocked()
}

func (f *failTracker) cleanupLocked(now time.Time) bool {
	if f == nil {
		return false
	}
	changed := false
	cutoff := now.Add(-f.window)
	for key, state := range f.items {
		if state == nil {
			delete(f.items, key)
			changed = true
			continue
		}
		beforeAuth := len(state.authFailures)
		beforeProtocol := len(state.protocolFailures)
		state.authFailures = appendRecentFailures(state.authFailures, cutoff)
		state.protocolFailures = appendRecentFailures(state.protocolFailures, cutoff)
		if len(state.authFailures) != beforeAuth || len(state.protocolFailures) != beforeProtocol {
			changed = true
		}
		if !state.blockedUntil.IsZero() && !state.blockedUntil.After(now) {
			state.blockedUntil = time.Time{}
			changed = true
		}
		if state.blockedUntil.IsZero() && len(state.authFailures) == 0 && len(state.protocolFailures) == 0 {
			delete(f.items, key)
			changed = true
		}
	}
	return changed
}

func (f *failTracker) stats() failTrackerStats {
	if f == nil {
		return failTrackerStats{}
	}
	f.mu.Lock()
	changed := f.cleanupLocked(f.now())
	stats := failTrackerStats{
		TrackedIPs:    len(f.items),
		MaxTrackedIPs: f.maxItems,
		EvictedIPs:    f.evicted.Load(),
		CapacityDrops: f.capacityDrop.Load(),
	}
	if changed {
		_ = f.saveLocked()
	}
	f.mu.Unlock()
	return stats
}

func splitHostPort(target string) (string, int, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(target))
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port")
	}
	return strings.Trim(host, "[]"), port, nil
}

func normalizeTarget(target string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(target))
	if err != nil {
		return strings.ToLower(strings.TrimSpace(target))
	}
	return net.JoinHostPort(strings.ToLower(strings.Trim(host, "[]")), port)
}

func isLoopbackName(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isPrivateTargetIP(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsPrivate()
}

func remoteHost(addr net.Addr) string {
	if addr == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

func addrString(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	return addr.String()
}
