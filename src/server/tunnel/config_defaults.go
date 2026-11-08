package tunnel

import (
	"net"
	"strconv"
	"strings"

	appversion "lsyltunnel/src/internal/version"
)

func ApplyDefaults(cfg *Config) {
	if cfg.ConfigVersion == 0 {
		cfg.ConfigVersion = appversion.ServerConfigVersion
	}
	if strings.TrimSpace(cfg.Requires.MinServerVersion) == "" {
		cfg.Requires.MinServerVersion = appversion.ServerConfigRequiresServerVersion
	}
	if cfg.Compatibility.ProtocolVersion == 0 {
		cfg.Compatibility.ProtocolVersion = appversion.ProtocolVersion
	}
	if strings.TrimSpace(cfg.Compatibility.MinClientVersion) == "" {
		cfg.Compatibility.MinClientVersion = appversion.MinCompatibleClientVersion
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = DefaultListenAddr
	}
	if cfg.TLS.MinVersion == "" {
		cfg.TLS.MinVersion = "1.3"
	}
	for i := range cfg.Forwards {
		cfg.Forwards[i].Direction = strings.TrimSpace(cfg.Forwards[i].Direction)
		if cfg.Forwards[i].Direction == "" {
			cfg.Forwards[i].Direction = DirectionClientToServer
		}
		normalizeReverseListenPort(&cfg.Forwards[i])
	}
	if cfg.Security.HandshakeTimeoutSec <= 0 {
		cfg.Security.HandshakeTimeoutSec = 8
	}
	if cfg.Security.DialTimeoutSec <= 0 {
		cfg.Security.DialTimeoutSec = 5
	}
	if cfg.Security.MaxHandshakeBytes <= 0 {
		cfg.Security.MaxHandshakeBytes = 32768
	}
	if cfg.Security.MaxConcurrentConnections <= 0 {
		cfg.Security.MaxConcurrentConnections = 2048
	}
	if cfg.Security.MaxConcurrentConnectionsPerIP <= 0 {
		cfg.Security.MaxConcurrentConnectionsPerIP = 128
	}
	if cfg.Security.ConnectionRateWindowSec <= 0 {
		cfg.Security.ConnectionRateWindowSec = 1
	}
	if cfg.Security.MaxNewConnectionsPerIPWindow <= 0 && cfg.Security.MaxConnectionsPerIPPerWindow > 0 {
		cfg.Security.MaxNewConnectionsPerIPWindow = cfg.Security.MaxConnectionsPerIPPerWindow
	}
	if cfg.Security.MaxNewConnectionsPerIPWindow <= 0 {
		cfg.Security.MaxNewConnectionsPerIPWindow = 120
	}
	cfg.Security.MaxConnectionsPerIPPerWindow = 0
	if cfg.Security.MaxTrackedConnectionIPs <= 0 {
		cfg.Security.MaxTrackedConnectionIPs = 8192
	}
	if cfg.Security.ConnectionLimiterCleanupSec <= 0 {
		cfg.Security.ConnectionLimiterCleanupSec = 60
	}
	if cfg.Security.MaxTrackedFailureIPs <= 0 {
		cfg.Security.MaxTrackedFailureIPs = 8192
	}
	if cfg.Security.FailureTrackerCleanupSec <= 0 {
		cfg.Security.FailureTrackerCleanupSec = 60
	}
	if cfg.Security.EntryTrafficLogQueueSize <= 0 {
		cfg.Security.EntryTrafficLogQueueSize = 2048
	}
	if cfg.Security.AuthFailWindowSec <= 0 {
		cfg.Security.AuthFailWindowSec = 300
	}
	if cfg.Security.AuthFailThreshold <= 0 {
		cfg.Security.AuthFailThreshold = 8
	}
	if cfg.Security.AuthFailBlockSec <= 0 {
		cfg.Security.AuthFailBlockSec = 1800
	}
	if cfg.Runtime.RecentEvents <= 0 {
		cfg.Runtime.RecentEvents = 500
	}
}

func configForSave(cfg Config) Config {
	cfg.LogLevel = ""
	cfg.Runtime.FlowTrafficLogFile = ""
	for i := range cfg.Forwards {
		if cfg.Forwards[i].Direction == DirectionServerToClient && cfg.Forwards[i].ListenPort > 0 {
			cfg.Forwards[i].ListenAddr = ""
		}
	}
	return cfg
}

func normalizeReverseListenPort(forward *ForwardConfig) {
	if forward == nil || forward.Direction != DirectionServerToClient {
		return
	}
	// Preserve invalid or conflicting fields so validation can report the original input.
	if err := validateReverseListenFields(*forward); err != nil {
		return
	}
	if forward.ListenPort == 0 && strings.TrimSpace(forward.ListenAddr) != "" {
		_, port, _ := splitHostPort(forward.ListenAddr)
		forward.ListenPort = port
	}
	if forward.ListenPort > 0 && forward.ListenPort <= 65535 {
		forward.ListenAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(forward.ListenPort))
	}
}
