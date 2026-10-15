//go:build windows && !386

package tunnel

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	virtualRedirectControlMaxBytes  = 64 * 1024
	virtualRedirectHandshakeTimeout = 3 * time.Second
)

type virtualRedirectHelperResult struct {
	Token     string `json:"token"`
	Connected bool   `json:"connected,omitempty"`
	Ready     bool   `json:"ready,omitempty"`
	Stopped   bool   `json:"stopped,omitempty"`
	Error     string `json:"error,omitempty"`
}

type virtualRedirectControlServer struct {
	listener  net.Listener
	token     string
	results   chan virtualRedirectHelperResult
	done      chan struct{}
	closing   atomic.Bool
	closeOnce sync.Once
	mu        sync.Mutex
	conn      net.Conn
	decoder   *json.Decoder
	last      virtualRedirectHelperResult
	err       error
}

type virtualRedirectControlClient struct {
	conn  net.Conn
	token string
}

func newVirtualRedirectControlServer(token string) (*virtualRedirectControlServer, error) {
	token = strings.ToLower(strings.TrimSpace(token))
	if !validVirtualRedirectToken(token) {
		return nil, fmt.Errorf("invalid virtual redirect helper control token")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for virtual redirect helper control: %w", err)
	}
	server := &virtualRedirectControlServer{
		listener: listener,
		token:    token,
		results:  make(chan virtualRedirectHelperResult, 4),
		done:     make(chan struct{}),
	}
	go server.run()
	return server, nil
}

func (s *virtualRedirectControlServer) run() {
	defer close(s.done)
	defer close(s.results)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if !s.closing.Load() {
				s.setError(fmt.Errorf("accept virtual redirect helper control: %w", err))
			}
			return
		}
		if !s.authenticate(conn) {
			_ = conn.Close()
			continue
		}
		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()
		_ = s.listener.Close()
		s.readAuthenticated()
		_ = conn.Close()
		return
	}
}

func (s *virtualRedirectControlServer) authenticate(conn net.Conn) bool {
	_ = conn.SetReadDeadline(time.Now().Add(virtualRedirectHandshakeTimeout))
	decoder := json.NewDecoder(io.LimitReader(conn, virtualRedirectControlMaxBytes))
	var result virtualRedirectHelperResult
	if err := decoder.Decode(&result); err != nil || !result.Connected || result.Token != s.token {
		return false
	}
	_ = conn.SetReadDeadline(time.Time{})
	s.recordResult(result)
	s.decoder = decoder
	return true
}

func (s *virtualRedirectControlServer) readAuthenticated() {
	decoder := s.decoder
	for {
		var result virtualRedirectHelperResult
		err := decoder.Decode(&result)
		if err != nil {
			if !s.closing.Load() && !errors.Is(err, io.EOF) {
				s.setError(fmt.Errorf("read virtual redirect helper control: %w", err))
			} else if !s.closing.Load() && !s.LastResult().Stopped {
				s.setError(fmt.Errorf("virtual redirect helper control connection closed unexpectedly"))
			}
			return
		}
		if result.Token != s.token {
			s.setError(fmt.Errorf("virtual redirect helper control token changed during the session"))
			return
		}
		s.recordResult(result)
		if result.Stopped {
			return
		}
	}
}

func (s *virtualRedirectControlServer) recordResult(result virtualRedirectHelperResult) {
	s.mu.Lock()
	s.last = result
	s.mu.Unlock()
	s.results <- result
}

func (s *virtualRedirectControlServer) setError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.err = errors.Join(s.err, err)
	s.mu.Unlock()
}

func (s *virtualRedirectControlServer) Address() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *virtualRedirectControlServer) Results() <-chan virtualRedirectHelperResult {
	return s.results
}

func (s *virtualRedirectControlServer) Done() <-chan struct{} {
	return s.done
}

func (s *virtualRedirectControlServer) LastResult() virtualRedirectHelperResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func (s *virtualRedirectControlServer) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *virtualRedirectControlServer) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closing.Store(true)
		if s.listener != nil {
			_ = s.listener.Close()
		}
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
	})
	return nil
}

func connectVirtualRedirectControl(address, token string) (*virtualRedirectControlClient, error) {
	address, err := normalizeVirtualRedirectControlAddress(address)
	if err != nil {
		return nil, err
	}
	token = strings.ToLower(strings.TrimSpace(token))
	if !validVirtualRedirectToken(token) {
		return nil, fmt.Errorf("invalid virtual redirect helper control token")
	}
	conn, err := net.DialTimeout("tcp4", address, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect virtual redirect helper control: %w", err)
	}
	client := &virtualRedirectControlClient{conn: conn, token: token}
	if err := client.Send(virtualRedirectHelperResult{Connected: true}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

func (c *virtualRedirectControlClient) Send(result virtualRedirectHelperResult) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("virtual redirect helper control is unavailable")
	}
	result.Token = c.token
	if err := json.NewEncoder(c.conn).Encode(result); err != nil {
		return fmt.Errorf("send virtual redirect helper control result: %w", err)
	}
	return nil
}

func (c *virtualRedirectControlClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func normalizeVirtualRedirectControlAddress(value string) (string, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("virtual redirect helper control must use loopback IPv4:port")
	}
	ip := net.ParseIP(host)
	port, portErr := strconv.Atoi(portText)
	if portErr != nil || port < 1 || port > 65535 || ip == nil || ip.To4() == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("virtual redirect helper control must use loopback IPv4:port")
	}
	return net.JoinHostPort(ip.To4().String(), strconv.Itoa(port)), nil
}

func newVirtualRedirectToken() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func validVirtualRedirectToken(token string) bool {
	if len(token) != 32 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}
