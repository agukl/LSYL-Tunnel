package tunnel

import (
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"lsyltunnel/src/internal/protocol"
)

const (
	HealthChecking          = "checking"
	HealthOK                = "ok"
	HealthServerUnavailable = "server_unavailable"
	HealthAuthError         = "auth_error"

	ForwardStarting      = "starting"
	ForwardListening     = "listening"
	ForwardListenFailed  = "listen_failed"
	ForwardReverseWait   = "reverse_waiting"
	ForwardReverseActive = "reverse_active"
	ForwardRetrying      = "retrying"
	ForwardRejected      = "rejected"
)

type HealthStatus struct {
	err                 error
	State               string `json:"state"`
	Message             string `json:"message,omitempty"`
	LastChecked         string `json:"last_checked,omitempty"`
	LastOK              string `json:"last_ok,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures,omitempty"`
	RetryDelaySec       int    `json:"retry_delay_sec,omitempty"`
	NextRetryAt         string `json:"next_retry_at,omitempty"`
	Terminal            bool   `json:"terminal,omitempty"`
}

// Err retains the original failure for in-process consumers, without changing JSON.
func (s HealthStatus) Err() error { return s.err }

type ForwardStatus struct {
	Name         string `json:"name"`
	Direction    string `json:"direction"`
	VirtualIP    string `json:"virtual_ip,omitempty"`
	VirtualAddr  string `json:"virtual_addr,omitempty"`
	ListenAddr   string `json:"listen_addr"`
	ServerTarget string `json:"server_target"`
	State        string `json:"state"`
	Message      string `json:"message,omitempty"`
	Active       int64  `json:"active"`
	Total        int64  `json:"total"`
	LastOpen     string `json:"last_open,omitempty"`
	LastClose    string `json:"last_close,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

type ClientStats struct {
	Health        HealthStatus    `json:"health"`
	ServerVersion string          `json:"server_version,omitempty"`
	Active        int64           `json:"active"`
	Total         int64           `json:"total"`
	Items         []ForwardStatus `json:"items"`
}

type forwardRuntime struct {
	name         string
	direction    string
	virtualIP    string
	virtualAddr  string
	listenAddr   string
	serverTarget string
	active       atomic.Int64
	total        atomic.Int64

	mu        sync.Mutex
	state     string
	message   string
	lastOpen  string
	lastClose string
	lastError string
}

func (c *Client) Stats() ClientStats {
	if c == nil {
		return ClientStats{}
	}
	c.healthMu.Lock()
	health := c.health
	serverVersion := c.serverVersion
	c.healthMu.Unlock()

	c.forwardsMu.Lock()
	items := make([]ForwardStatus, 0, len(c.forwards))
	for _, item := range c.forwards {
		items = append(items, item.snapshot())
	}
	c.forwardsMu.Unlock()
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return ClientStats{
		Health:        health,
		ServerVersion: serverVersion,
		Active:        c.active.Load(),
		Total:         c.total.Load(),
		Items:         items,
	}
}

func (c *Client) SetServerVersion(version string) {
	if c == nil {
		return
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	c.healthMu.Lock()
	c.serverVersion = version
	c.healthMu.Unlock()
}

func (c *Client) setServerVersionFromResponse(resp protocol.OpenResponse) {
	c.SetServerVersion(resp.ServerVersion)
}

func (f *forwardRuntime) snapshot() ForwardStatus {
	if f == nil {
		return ForwardStatus{}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return ForwardStatus{
		Name:         f.name,
		Direction:    f.direction,
		VirtualIP:    f.virtualIP,
		VirtualAddr:  f.virtualAddr,
		ListenAddr:   f.listenAddr,
		ServerTarget: f.serverTarget,
		State:        f.state,
		Message:      f.message,
		Active:       f.active.Load(),
		Total:        f.total.Load(),
		LastOpen:     f.lastOpen,
		LastClose:    f.lastClose,
		LastError:    f.lastError,
	}
}

func (c *Client) initForward(name string, fwd ForwardConfig, state, message string) {
	rt := &forwardRuntime{
		name:         name,
		direction:    fwd.Direction,
		listenAddr:   fwd.ListenAddr,
		serverTarget: fwd.ServerTarget,
		state:        state,
		message:      message,
	}
	c.forwardsMu.Lock()
	c.forwards[name] = rt
	c.forwardsMu.Unlock()
}

func (c *Client) setForwardAddresses(name, listenAddr, virtualAddr string) {
	fwd := c.forward(name)
	if fwd == nil {
		return
	}
	fwd.mu.Lock()
	fwd.listenAddr = listenAddr
	fwd.virtualAddr = virtualAddr
	fwd.virtualIP = ""
	if host, _, err := net.SplitHostPort(virtualAddr); err == nil {
		fwd.virtualIP = strings.Trim(host, "[]")
	}
	fwd.mu.Unlock()
}

func (c *Client) forward(name string) *forwardRuntime {
	c.forwardsMu.Lock()
	defer c.forwardsMu.Unlock()
	return c.forwards[name]
}

func (c *Client) setForwardState(name, state, message string) {
	fwd := c.forward(name)
	if fwd == nil {
		return
	}
	fwd.mu.Lock()
	fwd.state = state
	fwd.message = message
	if state == ForwardListening || state == ForwardReverseWait || state == ForwardReverseActive {
		fwd.lastError = ""
	}
	fwd.mu.Unlock()
}

func (c *Client) recordForwardError(name string, err error) {
	if err == nil {
		return
	}
	fwd := c.forward(name)
	if fwd == nil {
		return
	}
	fwd.mu.Lock()
	fwd.lastError = err.Error()
	fwd.message = ForwardErrorMessage(err)
	fwd.mu.Unlock()
}

func (c *Client) beginForwardStream(name string) func() {
	now := time.Now().Format(time.RFC3339)
	fwd := c.forward(name)
	if fwd != nil {
		fwd.active.Add(1)
		fwd.total.Add(1)
		fwd.mu.Lock()
		fwd.lastOpen = now
		fwd.lastError = ""
		fwd.mu.Unlock()
	}
	c.total.Add(1)
	c.active.Add(1)
	return func() {
		closedAt := time.Now().Format(time.RFC3339)
		if fwd != nil {
			fwd.active.Add(-1)
			fwd.mu.Lock()
			fwd.lastClose = closedAt
			fwd.mu.Unlock()
		}
		c.active.Add(-1)
	}
}
