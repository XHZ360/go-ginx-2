package daemon

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/admin"
	"github.com/simp-frp/go-ginx-2/internal/adminapi"
	"github.com/simp-frp/go-ginx-2/internal/adminquery"
	"github.com/simp-frp/go-ginx-2/internal/certmanager"
	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/enrollment"
	"github.com/simp-frp/go-ginx-2/internal/enrollmentapi"
	httpproxy "github.com/simp-frp/go-ginx-2/internal/proxy/http"
	httpsproxy "github.com/simp-frp/go-ginx-2/internal/proxy/https"
	tcpproxy "github.com/simp-frp/go-ginx-2/internal/proxy/tcp"
	udpproxy "github.com/simp-frp/go-ginx-2/internal/proxy/udp"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

var (
	newDaemonACMEIssuer  = func() certmanager.Issuer { return certmanager.ACMEIssuer{} }
	newDaemonDNSProvider = func(tokenEnv string) (certmanager.DNSChallengeProvider, error) {
		provider, err := certmanager.NewCloudflareDNSProviderFromEnv(tokenEnv)
		if err != nil {
			return nil, err
		}
		return provider, nil
	}
)

type ServerRuntime struct {
	Store              store.Store
	Sessions           *session.Manager
	Stats              *stats.Memory
	persistentStats    *stats.Persistent
	ControlListener    *control.Listener
	ControlTLSListener *control.TLSListener
	AdminServer        *adminapi.Server
	EnrollmentServer   *enrollmentapi.Server
	TCPListeners       []*tcpproxy.Listener
	UDPListeners       []*udpproxy.Listener
	HTTPServer         *httpproxy.Server
	HTTPSListener      *httpsproxy.Listener
	JoinService        config.JoinServiceDefaults
	proxyEntryDefaults domain.ProxyEntryDefaults
	certificateDir     string
	httpsEntryEnabled  bool
	defaultHTTPKey     listenerKey
	defaultHTTPSKey    listenerKey
	proxyListenerMu    sync.Mutex
	tcpListeners       map[listenerKey]*tcpproxy.Listener
	udpListeners       map[listenerKey]*udpproxy.Listener
	httpServers        map[listenerKey]*httpproxy.Server
	httpsListeners     map[listenerKey]*httpsproxy.Listener
	runtimeCtx         context.Context

	cancel context.CancelFunc
	once   sync.Once
}

func StartServer(ctx context.Context, cfg config.Server) (*ServerRuntime, error) {
	cfg = cfg.WithLogRotationDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	db, err := sqlite.Open(cfg.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	runtime, err := startServerWithStore(ctx, cfg, db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return runtime, nil
}

func startServerWithStore(parent context.Context, cfg config.Server, db store.Store) (*ServerRuntime, error) {
	tlsConfig, err := loadServerTLSConfig(cfg.ControlTLSCertFile, cfg.ControlTLSKeyFile)
	if err != nil {
		return nil, err
	}
	joinDefaults, err := config.ConfirmJoinServiceDefaults(cfg)
	if err != nil {
		return nil, err
	}
	proxyEntryDefaults, err := cfg.ProxyEntryDefaults()
	if err != nil {
		return nil, err
	}
	runtimeCtx, cancel := context.WithCancel(parent)
	sessions := session.NewManager()
	persistentStats, err := stats.NewPersistent(runtimeCtx, db.Stats(), 30*time.Second)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load persisted stats: %w", err)
	}
	memoryStats := persistentStats.Memory()
	controlListener, err := control.ListenAddr(cfg.ControlQUICListen, control.Server{
		Authenticator: control.Authenticator{Store: db},
		Sessions:      sessions,
		TLSConfig:     tlsConfig,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("listen control quic: %w", err)
	}
	runtime := &ServerRuntime{Store: db, Sessions: sessions, Stats: memoryStats, persistentStats: persistentStats, ControlListener: controlListener, JoinService: joinDefaults, proxyEntryDefaults: proxyEntryDefaults, certificateDir: cfg.CertificateDir, httpsEntryEnabled: cfg.HTTPSEntryListen != "", runtimeCtx: runtimeCtx, cancel: cancel}
	runtime.initProxyListenerRegistry()
	go func() { _ = controlListener.Serve(runtimeCtx) }()
	go runSessionExpiryLoop(runtimeCtx, sessions, db, cfg.HeartbeatTimeout)
	enrollmentServer, err := enrollmentapi.Listen(enrollmentapi.Entry{ListenAddress: cfg.ClientEnrollmentListen, Enrollment: enrollment.Service{Store: db}})
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("listen client enrollment: %w", err)
	}
	runtime.EnrollmentServer = enrollmentServer
	go func() { _ = enrollmentServer.Serve(runtimeCtx) }()
	if cfg.AdminEnabled || cfg.AdminCredentialsFile != "" {
		staticListenerClaims, err := cfg.RuntimeListenerClaims(true)
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("assemble runtime listener claims: %w", err)
		}
		adminService := admin.Service{Store: db, StaticListenerClaims: staticListenerClaims, ProxyEntryDefaults: proxyEntryDefaults, DefaultJoin: joinDefaults, ListenerReconciler: runtime}
		if cfg.ACMEEnabled || cfg.OriginCAEnabled {
			certificateService, err := managedCertificateService(cfg, db)
			if err != nil {
				_ = runtime.Close()
				return nil, err
			}
			adminService.Certificates = certificateService
		}
		adminJWTSecret, err := config.LoadAdminJWTSecret(cfg.AdminJWTSecretFile)
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("load admin jwt secret: %w", err)
		}
		adminServer, err := adminapi.Listen(adminapi.Entry{ListenAddress: cfg.AdminListen, AdminCredentialsFile: cfg.AdminCredentialsFile, AdminFrontendDir: cfg.AdminFrontendDir, AdminJWTSecret: adminJWTSecret, Query: adminquery.Service{Store: db, Sessions: sessions, Stats: memoryStats, CertificateDir: cfg.CertificateDir, RenewalWindow: cfg.ACMERenewalWindow, OriginCARotationWindow: cfg.OriginCARotationWindow}, Commands: adminService, Enrollment: enrollment.Service{Store: db}})
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("listen admin api: %w", err)
		}
		runtime.AdminServer = adminServer
		go func() { _ = adminServer.Serve(runtimeCtx) }()
	}
	if cfg.ControlTLSListen != "" {
		controlTLSListener, err := control.ListenTLSAddr(cfg.ControlTLSListen, control.Server{
			Authenticator: control.Authenticator{Store: db, AllowedProtocols: []domain.Protocol{domain.ProtocolTCPTLS}},
			Sessions:      sessions,
			TLSConfig:     tlsConfig,
		})
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("listen control tcp tls: %w", err)
		}
		runtime.ControlTLSListener = controlTLSListener
		go func() { _ = controlTLSListener.Serve(runtimeCtx) }()
	}

	if err := runtime.startDefaultProxyListeners(); err != nil {
		_ = runtime.Close()
		return nil, err
	}
	if err := runtime.ReconcileProxyListeners(runtimeCtx); err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("reconcile proxy listeners: %w", err)
	}
	if cfg.ACMEEnabled || cfg.OriginCAEnabled {
		certificateService, err := managedCertificateService(cfg, db)
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		go newManagedCertificateController(db, certificateService, cfg.ACMERenewalWindow, cfg.OriginCARotationWindow).Run(runtimeCtx)
	}
	return runtime, nil
}

func managedCertificateService(cfg config.Server, db store.Store) (certmanager.Service, error) {
	var issuer certmanager.Issuer
	var provider certmanager.DNSChallengeProvider
	if cfg.ACMEEnabled {
		issuer = newDaemonACMEIssuer()
		loadedProvider, err := newDaemonDNSProvider(cfg.ACMECloudflareTokenEnv)
		if err != nil {
			provider = failedDNSProvider{err: err}
			issuer = failedCertificateIssuer{err: err}
		} else {
			provider = loadedProvider
		}
	}
	return certmanager.Service{
		Store:               db,
		Issuer:              issuer,
		DNSProvider:         provider,
		OriginCAClient:      certmanager.CloudflareOriginCAClient{},
		ProviderSecretStore: certmanager.FileSecretStore{Dir: cfg.OriginCASecretStorePath},
		Storage:             httpsproxy.ManagedCertificateStorage{CertificateDir: cfg.CertificateDir},
		Settings:            domain.ACMEProviderSettings{DirectoryURL: cfg.ACMEDirectoryURL, AccountEmail: cfg.ACMEAccountEmail, TermsAccepted: cfg.ACMETermsAccepted, RenewalWindow: cfg.ACMERenewalWindow, DNSProvider: "cloudflare", DNSProviderTokenEnv: cfg.ACMECloudflareTokenEnv},
		OriginCASettings:    domain.OriginCAProviderSettings{Enabled: cfg.OriginCAEnabled, SecretStorePath: cfg.OriginCASecretStorePath, DefaultRequestType: cfg.OriginCADefaultRequestType, RequestedValidity: cfg.OriginCARequestedValidity, RotationWindow: cfg.OriginCARotationWindow},
	}, nil
}

func (runtime *ServerRuntime) Close() error {
	if runtime == nil {
		return nil
	}
	var closeErr error
	runtime.once.Do(func() {
		if runtime.cancel != nil {
			runtime.cancel()
		}
		if runtime.ControlListener != nil {
			closeErr = errors.Join(closeErr, runtime.ControlListener.Close())
		}
		if runtime.ControlTLSListener != nil {
			closeErr = errors.Join(closeErr, runtime.ControlTLSListener.Close())
		}
		if runtime.EnrollmentServer != nil {
			closeErr = errors.Join(closeErr, runtime.EnrollmentServer.Close())
		}
		if runtime.AdminServer != nil {
			closeErr = errors.Join(closeErr, runtime.AdminServer.Close())
		}
		closeErr = errors.Join(closeErr, runtime.closeProxyListeners())
		if runtime.persistentStats != nil {
			closeErr = errors.Join(closeErr, runtime.persistentStats.Close(context.Background()))
		}
		if runtime.Store != nil {
			closeErr = errors.Join(closeErr, runtime.Store.Close())
		}
	})
	return closeErr
}

func loadServerTLSConfig(certFile string, keyFile string) (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load control tls certificate: %w", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13}, nil
}
