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
	TCPListeners       []*tcpproxy.Listener
	UDPListeners       []*udpproxy.Listener
	HTTPServer         *httpproxy.Server
	HTTPSListener      *httpsproxy.Listener

	cancel context.CancelFunc
	once   sync.Once
}

func StartServer(ctx context.Context, cfg config.Server) (*ServerRuntime, error) {
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
	runtime := &ServerRuntime{Store: db, Sessions: sessions, Stats: memoryStats, persistentStats: persistentStats, ControlListener: controlListener, cancel: cancel}
	go func() { _ = controlListener.Serve(runtimeCtx) }()
	if cfg.AdminCredentialsFile != "" {
		staticListenerClaims, err := cfg.RuntimeListenerClaims(true)
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("assemble runtime listener claims: %w", err)
		}
		adminService := admin.Service{Store: db, StaticListenerClaims: staticListenerClaims}
		if cfg.ACMEEnabled {
			certificateService, err := managedCertificateService(cfg, db)
			if err != nil {
				_ = runtime.Close()
				return nil, err
			}
			adminService.Certificates = certificateService
		}
		adminServer, err := adminapi.Listen(adminapi.Entry{ListenAddress: cfg.AdminListen, AdminCredentialsFile: cfg.AdminCredentialsFile, Query: adminquery.Service{Store: db, Sessions: sessions, Stats: memoryStats}, Commands: adminService})
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

	tcpProxies, err := db.Proxies().EnabledByType(runtimeCtx, domain.ProxyTCP)
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("list tcp proxies: %w", err)
	}
	for _, proxy := range tcpProxies {
		if proxy.EntryPort == 0 {
			_ = runtime.Close()
			return nil, fmt.Errorf("tcp proxy %s entry port is required", proxy.ID)
		}
		listener, err := tcpproxy.Listen(tcpproxy.Entry{Store: db, Sessions: sessions, ListenAddress: tcpproxy.Address(cfg.TCPEntryHost, proxy.EntryPort), EntryPort: proxy.EntryPort, Stats: persistentStats})
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("listen tcp proxy %s: %w", proxy.ID, err)
		}
		runtime.TCPListeners = append(runtime.TCPListeners, listener)
		go func(listener *tcpproxy.Listener) { _ = listener.Serve(runtimeCtx) }(listener)
	}

	udpProxies, err := db.Proxies().EnabledByType(runtimeCtx, domain.ProxyUDP)
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("list udp proxies: %w", err)
	}
	for _, proxy := range udpProxies {
		if proxy.EntryPort == 0 {
			_ = runtime.Close()
			return nil, fmt.Errorf("udp proxy %s entry port is required", proxy.ID)
		}
		listener, err := udpproxy.Listen(udpproxy.Entry{Store: db, Sessions: sessions, ListenAddress: udpproxy.Address(cfg.TCPEntryHost, proxy.EntryPort), EntryPort: proxy.EntryPort, Stats: persistentStats})
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("listen udp proxy %s: %w", proxy.ID, err)
		}
		runtime.UDPListeners = append(runtime.UDPListeners, listener)
		go func(listener *udpproxy.Listener) { _ = listener.Serve(runtimeCtx) }(listener)
	}

	httpServer, err := httpproxy.Listen(httpproxy.Entry{Store: db, Sessions: sessions, ListenAddress: cfg.HTTPEntryListen, Stats: persistentStats})
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("listen http proxy: %w", err)
	}
	runtime.HTTPServer = httpServer
	go func() { _ = httpServer.Serve(runtimeCtx) }()

	if cfg.HTTPSEntryListen != "" {
		httpsListener, err := httpsproxy.Listen(httpsproxy.Entry{Store: db, Sessions: sessions, ListenAddress: cfg.HTTPSEntryListen, CertificateDir: cfg.CertificateDir})
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("listen https proxy: %w", err)
		}
		runtime.HTTPSListener = httpsListener
		go func() { _ = httpsListener.Serve(runtimeCtx) }()
	}
	if cfg.ACMEEnabled {
		certificateService, err := managedCertificateService(cfg, db)
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		go runCertificateRenewalLoop(runtimeCtx, db, certificateService, cfg.ACMERenewalWindow)
	}
	return runtime, nil
}

func managedCertificateService(cfg config.Server, db store.Store) (certmanager.Service, error) {
	provider, err := newDaemonDNSProvider(cfg.ACMECloudflareTokenEnv)
	if err != nil {
		return certmanager.Service{}, err
	}
	return certmanager.Service{Store: db, Issuer: newDaemonACMEIssuer(), DNSProvider: provider, Storage: httpsproxy.ManagedCertificateStorage{CertificateDir: cfg.CertificateDir}, Settings: domain.ACMEProviderSettings{DirectoryURL: cfg.ACMEDirectoryURL, AccountEmail: cfg.ACMEAccountEmail, TermsAccepted: cfg.ACMETermsAccepted, RenewalWindow: cfg.ACMERenewalWindow, DNSProvider: "cloudflare", DNSProviderTokenEnv: cfg.ACMECloudflareTokenEnv}}, nil
}

func runCertificateRenewalLoop(ctx context.Context, db store.Store, service certmanager.Service, renewalWindow time.Duration) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	renewManagedCertificates(ctx, db, service, renewalWindow)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewManagedCertificates(ctx, db, service, renewalWindow)
		}
	}
}

func renewManagedCertificates(ctx context.Context, db store.Store, service certmanager.Service, renewalWindow time.Duration) {
	if db == nil || renewalWindow <= 0 {
		return
	}
	certificates, err := db.Certificates().ListRenewable(ctx, time.Now().UTC().Add(renewalWindow))
	if err != nil {
		return
	}
	for _, certificate := range certificates {
		_, _ = service.Renew(ctx, certificate.ProxyID)
	}
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
		if runtime.AdminServer != nil {
			closeErr = errors.Join(closeErr, runtime.AdminServer.Close())
		}
		for _, listener := range runtime.TCPListeners {
			closeErr = errors.Join(closeErr, listener.Close())
		}
		for _, listener := range runtime.UDPListeners {
			closeErr = errors.Join(closeErr, listener.Close())
		}
		if runtime.HTTPServer != nil {
			closeErr = errors.Join(closeErr, runtime.HTTPServer.Close())
		}
		if runtime.HTTPSListener != nil {
			closeErr = errors.Join(closeErr, runtime.HTTPSListener.Close())
		}
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
