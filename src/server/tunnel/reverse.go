package tunnel

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
)

type reverseListener struct {
	addr      string
	ln        net.Listener
	closeOnce sync.Once

	mu       sync.Mutex
	controls map[*reverseControl]struct{}
	pending  map[string]*reversePendingConn
	closed   bool
}

type reversePendingConn struct {
	net.Conn
	done chan struct{}
}

type reverseControl struct {
	conn        net.Conn
	requestID   string
	username    string
	clientID    string
	forwardName string
	target      string
	requests    chan reverseConnectRequest
	done        chan struct{}
	closeOnce   sync.Once
}

type reverseConnectRequest struct {
	streamID string
}

var (
	reverseControlHeartbeatInterval = 15 * time.Second
	reverseControlReadTimeout       = 45 * time.Second
	reverseControlWriteTimeout      = 5 * time.Second
)

func (s *Server) registerReverseControl(conn net.Conn, req protocol.OpenRequest, user UserConfig, requestID string) error {
	listenAddr := strings.TrimSpace(req.ListenAddr)
	if listenAddr == "" {
		return fmt.Errorf("reverse listen address is required")
	}
	if strings.TrimSpace(req.Target) == "" {
		return fmt.Errorf("reverse target address is required")
	}
	if !s.isConfiguredReverseListener(listenAddr) {
		return fmt.Errorf("reverse listen address is not configured on server")
	}
	if err := s.authorizeReverse(user, listenAddr); err != nil {
		return err
	}
	rl, err := s.ensureReverseListener(listenAddr)
	if err != nil {
		return err
	}
	control := &reverseControl{
		conn:        conn,
		requestID:   requestID,
		username:    req.Username,
		clientID:    req.ClientID,
		forwardName: req.ForwardName,
		target:      req.Target,
		requests:    make(chan reverseConnectRequest, 16),
		done:        make(chan struct{}),
	}
	if !rl.addControl(control) {
		return fmt.Errorf("reverse listen address is already activated")
	}
	transport.EnableTCPKeepAlive(conn, reverseControlHeartbeatInterval)
	_ = conn.SetDeadline(time.Time{})
	resp := serverVersionResponse(protocol.OpenResponse{OK: true, Code: "ok", Message: "reverse listen activated", CredentialKey: s.activeCredentialPublicKey()})
	if err := protocol.WriteJSON(conn, resp); err != nil {
		control.close()
		rl.removeControl(control)
		return err
	}
	if !s.startWorker(func() { s.runReverseControl(rl, control) }) {
		control.close()
		rl.removeControl(control)
		return net.ErrClosed
	}
	s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "reverse_listen", Result: "activated", Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, ListenAddr: listenAddr, Code: "ok"})
	return nil
}

func (s *Server) registerReverseStream(conn net.Conn, req protocol.OpenRequest, user UserConfig, requestID string) error {
	listenAddr := strings.TrimSpace(req.ListenAddr)
	streamID := strings.TrimSpace(req.StreamID)
	if listenAddr == "" || streamID == "" {
		return fmt.Errorf("reverse stream is invalid")
	}
	if !s.isConfiguredReverseListener(listenAddr) {
		return fmt.Errorf("reverse listen address is not configured on server")
	}
	if err := s.authorizeReverse(user, listenAddr); err != nil {
		return err
	}
	rl := s.reverseListener(listenAddr)
	if rl == nil {
		return fmt.Errorf("reverse listen is not active")
	}
	inbound := rl.takePending(streamID)
	if inbound == nil {
		return fmt.Errorf("reverse stream has expired")
	}
	releaseUserStream, ok := s.userStreams.acquire(req.Username)
	if !ok {
		s.userStreamLimitRejected.Add(1)
		_ = inbound.Close()
		s.closeReverseListenerIfIdle(rl)
		return errUserStreamLimit
	}
	s.closeReverseListenerIfIdle(rl)
	transport.EnableTCPKeepAlive(conn, reverseControlHeartbeatInterval)
	_ = conn.SetDeadline(time.Time{})
	resp := serverVersionResponse(protocol.OpenResponse{OK: true, Code: "ok", Message: "connected"})
	if err := protocol.WriteJSON(conn, resp); err != nil {
		releaseUserStream()
		_ = inbound.Close()
		return err
	}
	if !s.startWorker(func() {
		defer releaseUserStream()
		defer inbound.Close()
		defer conn.Close()
		s.totalStreams.Add(1)
		s.activeStreams.Add(1)
		defer s.activeStreams.Add(-1)
		started := time.Now()
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "reverse_stream", Result: "connected", Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, ListenAddr: listenAddr, Code: "ok"})
		inboundToClient, clientToInbound := transport.ProxyPairWithOptions(inbound, conn, &s.bytesUp, &s.bytesDown, s.proxyOptions())
		durationMS := time.Since(started).Milliseconds()
		flowLog := s.flowTrafficEntry(requestID, "stream_closed", "reverse_stream", "closed", remoteHost(conn.RemoteAddr()), req)
		flowLog.ListenAddr = listenAddr
		flowLog.Code = "closed"
		flowLog.DurationMS = durationMS
		flowLog.BytesUp = inboundToClient
		flowLog.BytesDown = clientToInbound
		s.recordFlowTrafficLog(flowLog)
		s.recordEvent(RuntimeEvent{RequestID: requestID, Kind: "reverse_stream", Result: "closed", Username: req.Username, ClientID: req.ClientID, ForwardName: req.ForwardName, Direction: req.Direction, Target: req.Target, ListenAddr: listenAddr, Code: "closed", DurationMS: durationMS, BytesUp: inboundToClient, BytesDown: clientToInbound})
	}) {
		releaseUserStream()
		_ = inbound.Close()
		return net.ErrClosed
	}
	return nil
}

func (s *Server) runReverseControl(rl *reverseListener, control *reverseControl) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(reverseControlHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case req := <-control.requests:
				resp := protocol.OpenResponse{
					OK:       true,
					Code:     "reverse_connect",
					Message:  "open reverse stream",
					StreamID: req.streamID,
				}
				if err := writeReverseControlResponse(control.conn, resp); err != nil {
					s.log("reverse control %s write failed: %v", control.forwardName, err)
					control.close()
					return
				}
			case <-ticker.C:
				if err := writeReverseControlResponse(control.conn, protocol.OpenResponse{OK: true, Code: "reverse_ping", Message: "keepalive"}); err != nil {
					s.log("reverse control %s heartbeat failed: %v", control.forwardName, err)
					control.close()
					return
				}
			case <-control.done:
				return
			}
		}
	}()

	var req protocol.OpenRequest
	closeCode := "closed"
	closeMessage := ""
	for {
		_ = control.conn.SetReadDeadline(time.Now().Add(reverseControlReadTimeout))
		if err := protocol.ReadJSON(control.conn, &req, s.cfg.Security.MaxHandshakeBytes); err != nil {
			if !errors.Is(err, net.ErrClosed) {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					closeCode = "heartbeat_timeout"
					closeMessage = "reverse control heartbeat timed out"
					s.log("reverse control %s heartbeat timeout", control.forwardName)
				} else {
					closeCode = "control_read_failed"
					closeMessage = "reverse control connection was interrupted"
					s.log("reverse control %s read failed: %v", control.forwardName, err)
				}
			}
			break
		}
	}
	_ = control.conn.SetReadDeadline(time.Time{})
	control.close()
	rl.removeControl(control)
	rl.closePending()
	<-done
	s.closeReverseListenerIfIdle(rl)
	s.recordEvent(RuntimeEvent{RequestID: control.requestID, Kind: "reverse_listen", Result: "closed", Username: control.username, ClientID: control.clientID, ForwardName: control.forwardName, Target: control.target, ListenAddr: rl.addr, Code: closeCode, Message: closeMessage})
}

func (s *Server) ensureReverseListener(addr string) (*reverseListener, error) {
	key := normalizeTarget(addr)
	s.reverseMu.Lock()
	defer s.reverseMu.Unlock()
	if s.reverseListeners == nil {
		s.reverseListeners = map[string]*reverseListener{}
	}
	if rl := s.reverseListeners[key]; rl != nil {
		return rl, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.log("reverse passive listener %s unavailable: %v", addr, err)
		return nil, fmt.Errorf("server passive port is unavailable")
	}
	rl := &reverseListener{
		addr:     addr,
		ln:       ln,
		controls: map[*reverseControl]struct{}{},
		pending:  map[string]*reversePendingConn{},
	}
	s.reverseListeners[key] = rl
	if !s.startWorker(func() { s.acceptReverseLoop(rl) }) {
		delete(s.reverseListeners, key)
		rl.close()
		return nil, net.ErrClosed
	}
	s.log("reverse passive listener %s ready", ln.Addr())
	return rl, nil
}

func (s *Server) reverseListener(addr string) *reverseListener {
	key := normalizeTarget(addr)
	s.reverseMu.Lock()
	defer s.reverseMu.Unlock()
	return s.reverseListeners[key]
}

func (s *Server) acceptReverseLoop(rl *reverseListener) {
	for {
		conn, err := rl.ln.Accept()
		if err != nil {
			return
		}
		if !s.startWorker(func() { s.handleReverseInbound(rl, conn) }) {
			_ = conn.Close()
			return
		}
	}
}

func (s *Server) handleReverseInbound(rl *reverseListener, inbound net.Conn) {
	control := rl.pickControl()
	if control == nil {
		s.dialFailed.Add(1)
		s.recordFlowTrafficLog(FlowTrafficLogEntry{
			Event:      "reverse_inbound_failed",
			Kind:       "reverse_inbound",
			Result:     "failed",
			RemoteIP:   remoteHost(inbound.RemoteAddr()),
			ListenAddr: rl.addr,
			Code:       "no_active_client",
			Message:    "reverse stream has no active client",
			Abnormal:   true,
		})
		s.recordEvent(RuntimeEvent{Kind: "reverse_inbound", Result: "failed", ListenAddr: rl.addr, Code: "no_active_client", Message: "reverse stream has no active client"})
		_ = inbound.Close()
		s.closeReverseListenerIfIdle(rl)
		return
	}
	streamID, err := newReverseStreamID()
	if err != nil {
		_ = inbound.Close()
		return
	}
	pendingDone, ok := rl.addPending(streamID, inbound)
	if !ok {
		_ = inbound.Close()
		return
	}
	defer func() {
		if pending := rl.takePending(streamID); pending != nil {
			_ = pending.Close()
		}
		s.closeReverseListenerIfIdle(rl)
	}()
	timeout := time.Duration(s.cfg.Security.DialTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case control.requests <- reverseConnectRequest{streamID: streamID}:
	case <-control.done:
		return
	case <-s.runContext().Done():
		return
	case <-timer.C:
		s.expireReversePending(rl, streamID)
		return
	}
	// Keep the timeout under this worker's ownership until handoff or cancellation.
	select {
	case <-pendingDone:
	case <-control.done:
	case <-s.runContext().Done():
	case <-timer.C:
		s.expireReversePending(rl, streamID)
	}
}

func (s *Server) expireReversePending(rl *reverseListener, streamID string) {
	if pending := rl.takePending(streamID); pending != nil {
		defer pending.Close()
		if s.runContext().Err() != nil {
			return
		}
		s.dialFailed.Add(1)
		s.recordFlowTrafficLog(FlowTrafficLogEntry{
			Event:      "reverse_inbound_failed",
			Kind:       "reverse_inbound",
			Result:     "failed",
			RemoteIP:   remoteHost(pending.RemoteAddr()),
			ListenAddr: rl.addr,
			Code:       "client_stream_timeout",
			Message:    "timed out waiting for client stream",
			Abnormal:   true,
		})
		s.recordEvent(RuntimeEvent{Kind: "reverse_inbound", Result: "failed", ListenAddr: rl.addr, Code: "client_stream_timeout", Message: "timed out waiting for client stream"})
	}
}

func (s *Server) closeReverseListeners() {
	s.reverseMu.Lock()
	listeners := s.reverseListeners
	s.reverseListeners = nil
	s.reverseMu.Unlock()
	for _, rl := range listeners {
		rl.close()
	}
}

func (s *Server) closeReverseListenerIfIdle(rl *reverseListener) {
	if !rl.isIdle() {
		return
	}
	if s.isConfiguredReverseListener(rl.addr) {
		return
	}
	key := normalizeTarget(rl.addr)
	s.reverseMu.Lock()
	if s.reverseListeners[key] == rl && rl.isIdle() {
		delete(s.reverseListeners, key)
		s.reverseMu.Unlock()
		rl.close()
		s.log("reverse passive listener %s deactivated", rl.addr)
		return
	}
	s.reverseMu.Unlock()
}

func (rl *reverseListener) addControl(control *reverseControl) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.closed || len(rl.controls) > 0 {
		return false
	}
	rl.controls[control] = struct{}{}
	return true
}

func (rl *reverseListener) removeControl(control *reverseControl) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if _, ok := rl.controls[control]; ok {
		delete(rl.controls, control)
		control.close()
	}
}

func (rl *reverseListener) pickControl() *reverseControl {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for control := range rl.controls {
		return control
	}
	return nil
}

func (rl *reverseListener) addPending(streamID string, conn net.Conn) (<-chan struct{}, bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.closed {
		return nil, false
	}
	pending := &reversePendingConn{Conn: conn, done: make(chan struct{})}
	rl.pending[streamID] = pending
	return pending.done, true
}

func (rl *reverseListener) takePending(streamID string) net.Conn {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	pending := rl.pending[streamID]
	if pending == nil {
		return nil
	}
	delete(rl.pending, streamID)
	close(pending.done)
	return pending.Conn
}

func (rl *reverseListener) closePending() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for streamID, conn := range rl.pending {
		_ = conn.Close()
		close(conn.done)
		delete(rl.pending, streamID)
	}
}

func (rl *reverseListener) isIdle() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.controls) == 0 && len(rl.pending) == 0
}

func (rl *reverseListener) close() {
	rl.closeOnce.Do(func() {
		rl.mu.Lock()
		rl.closed = true
		for control := range rl.controls {
			control.close()
			delete(rl.controls, control)
		}
		for streamID, conn := range rl.pending {
			_ = conn.Close()
			close(conn.done)
			delete(rl.pending, streamID)
		}
		rl.mu.Unlock()
		if rl.ln != nil {
			_ = rl.ln.Close()
		}
	})
}

func (control *reverseControl) close() {
	control.closeOnce.Do(func() {
		close(control.done)
		_ = control.conn.Close()
	})
}

func newReverseStreamID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func writeReverseControlResponse(conn net.Conn, resp protocol.OpenResponse) error {
	_ = conn.SetWriteDeadline(time.Now().Add(reverseControlWriteTimeout))
	err := protocol.WriteJSON(conn, serverVersionResponse(resp))
	_ = conn.SetWriteDeadline(time.Time{})
	return err
}
