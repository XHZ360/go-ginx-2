package daemon

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	httpproxy "github.com/simp-frp/go-ginx-2/internal/proxy/http"
	tcpproxy "github.com/simp-frp/go-ginx-2/internal/proxy/tcp"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/stats"
	"github.com/simp-frp/go-ginx-2/internal/store"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

type ServerRuntime struct {
	Store           store.Store
	Sessions        *session.Manager
	Stats           *stats.Memory
	ControlListener *control.Listener
	TCPListeners    []*tcpproxy.Listener
	HTTPServer      *httpproxy.Server

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
	memoryStats := stats.NewMemory()
	controlListener, err := control.ListenAddr(cfg.ControlQUICListen, control.Server{
		Authenticator: control.Authenticator{Store: db},
		Sessions:      sessions,
		TLSConfig:     tlsConfig,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("listen control quic: %w", err)
	}
	runtime := &ServerRuntime{Store: db, Sessions: sessions, Stats: memoryStats, ControlListener: controlListener, cancel: cancel}
	go func() { _ = controlListener.Serve(runtimeCtx) }()

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
		listener, err := tcpproxy.Listen(tcpproxy.Entry{Store: db, Sessions: sessions, ListenAddress: tcpproxy.Address(cfg.TCPEntryHost, proxy.EntryPort), EntryPort: proxy.EntryPort, Stats: memoryStats})
		if err != nil {
			_ = runtime.Close()
			return nil, fmt.Errorf("listen tcp proxy %s: %w", proxy.ID, err)
		}
		runtime.TCPListeners = append(runtime.TCPListeners, listener)
		go func(listener *tcpproxy.Listener) { _ = listener.Serve(runtimeCtx) }(listener)
	}

	httpServer, err := httpproxy.Listen(httpproxy.Entry{Store: db, Sessions: sessions, ListenAddress: cfg.HTTPEntryListen, Stats: memoryStats})
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("listen http proxy: %w", err)
	}
	runtime.HTTPServer = httpServer
	go func() { _ = httpServer.Serve(runtimeCtx) }()
	return runtime, nil
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
		for _, listener := range runtime.TCPListeners {
			closeErr = errors.Join(closeErr, listener.Close())
		}
		if runtime.HTTPServer != nil {
			closeErr = errors.Join(closeErr, runtime.HTTPServer.Close())
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
