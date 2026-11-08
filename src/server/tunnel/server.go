package tunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"lsyltunnel/src/internal/transport"
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
