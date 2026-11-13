package tunnel

import (
	"net"
	"path/filepath"
	"strconv"
	"strings"

	appversion "lsyltunnel/src/internal/version"
)

const (
	defaultMaxHandshakeBytes           = 32768
	defaultConnectionRateWindowSec     = 1
	defaultMaxTrackedConnectionIPs     = 8192
	defaultConnectionLimiterCleanupSec = 60
	defaultMaxTrackedFailureIPs        = 8192
	defaultFailureTrackerCleanupSec    = 60
	defaultEntryTrafficLogQueueSize    = 2048
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
		cfg.Security.MaxHandshakeBytes = defaultMaxHandshakeBytes
	}
	if cfg.Security.MaxConcurrentConnections <= 0 {
		cfg.Security.MaxConcurrentConnections = 2048
	}
	if cfg.Security.MaxConcurrentConnectionsPerIP <= 0 {
		cfg.Security.MaxConcurrentConnectionsPerIP = 128
	}
	if cfg.Security.ConnectionRateWindowSec <= 0 {
		cfg.Security.ConnectionRateWindowSec = defaultConnectionRateWindowSec
	}
	if cfg.Security.MaxNewConnectionsPerIPWindow <= 0 && cfg.Security.MaxConnectionsPerIPPerWindow > 0 {
		cfg.Security.MaxNewConnectionsPerIPWindow = cfg.Security.MaxConnectionsPerIPPerWindow
	}
	if cfg.Security.MaxNewConnectionsPerIPWindow <= 0 {
		cfg.Security.MaxNewConnectionsPerIPWindow = 120
	}
	cfg.Security.MaxConnectionsPerIPPerWindow = 0
	if cfg.Security.MaxTrackedConnectionIPs <= 0 {
		cfg.Security.MaxTrackedConnectionIPs = defaultMaxTrackedConnectionIPs
	}
	if cfg.Security.ConnectionLimiterCleanupSec <= 0 {
		cfg.Security.ConnectionLimiterCleanupSec = defaultConnectionLimiterCleanupSec
	}
	if cfg.Security.MaxTrackedFailureIPs <= 0 {
		cfg.Security.MaxTrackedFailureIPs = defaultMaxTrackedFailureIPs
	}
	if cfg.Security.FailureTrackerCleanupSec <= 0 {
		cfg.Security.FailureTrackerCleanupSec = defaultFailureTrackerCleanupSec
	}
	if cfg.Security.EntryTrafficLogQueueSize <= 0 {
		cfg.Security.EntryTrafficLogQueueSize = defaultEntryTrafficLogQueueSize
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
		cfg.Runtime.RecentEvents = defaultRecentEvents
	}
}

func configForSave(cfg Config, path string) Config {
	cfg.LogLevel = ""
	if cfg.Requires.MinServerVersion == appversion.ServerConfigRequiresServerVersion {
		cfg.Requires.MinServerVersion = ""
	}
	if cfg.Compatibility.ProtocolVersion == appversion.ProtocolVersion {
		cfg.Compatibility.ProtocolVersion = 0
	}
	if cfg.Security.MaxHandshakeBytes == defaultMaxHandshakeBytes {
		cfg.Security.MaxHandshakeBytes = 0
	}
	if cfg.Security.ConnectionRateWindowSec == defaultConnectionRateWindowSec {
		cfg.Security.ConnectionRateWindowSec = 0
	}
	if cfg.Security.MaxTrackedConnectionIPs == defaultMaxTrackedConnectionIPs {
		cfg.Security.MaxTrackedConnectionIPs = 0
	}
	if cfg.Security.ConnectionLimiterCleanupSec == defaultConnectionLimiterCleanupSec {
		cfg.Security.ConnectionLimiterCleanupSec = 0
	}
	if cfg.Security.MaxTrackedFailureIPs == defaultMaxTrackedFailureIPs {
		cfg.Security.MaxTrackedFailureIPs = 0
	}
	if cfg.Security.FailureTrackerCleanupSec == defaultFailureTrackerCleanupSec {
		cfg.Security.FailureTrackerCleanupSec = 0
	}
	if cfg.Security.EntryTrafficLogQueueSize == defaultEntryTrafficLogQueueSize {
		cfg.Security.EntryTrafficLogQueueSize = 0
	}
	base := filepath.Dir(path)
	cfg.Runtime.StateFile = omitDefaultRuntimePath(base, cfg.Runtime.StateFile, "data", "server-state.json")
	cfg.Runtime.PermanentBlockFile = omitDefaultRuntimePath(base, cfg.Runtime.PermanentBlockFile, "data", "server-permanent-block.txt")
	cfg.Runtime.RequestLogFile = omitDefaultRuntimePath(base, cfg.Runtime.RequestLogFile, "logs", filepath.Join("request", "request.jsonl"))
	cfg.Runtime.BusinessLogFile = omitDefaultRuntimePath(base, cfg.Runtime.BusinessLogFile, "logs", filepath.Join("business", "business.jsonl"))
	cfg.Runtime.EntryTrafficLogFile = omitDefaultRuntimePath(base, cfg.Runtime.EntryTrafficLogFile, "logs", filepath.Join("entry-traffic", "entry-traffic.jsonl"))
	cfg.Runtime.FlowTrafficLogFile = ""
	if cfg.Runtime.RecentEvents == defaultRecentEvents {
		cfg.Runtime.RecentEvents = 0
	}
	for i := range cfg.Forwards {
		if cfg.Forwards[i].Direction == DirectionServerToClient && cfg.Forwards[i].ListenPort > 0 {
			cfg.Forwards[i].ListenAddr = ""
		}
	}
	return cfg
}

func omitDefaultRuntimePath(base, value, category, name string) string {
	if value != "" && filepath.Clean(resolveConfigPath(base, value)) == filepath.Clean(defaultRuntimePath(base, category, name)) {
		return ""
	}
	return value
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
