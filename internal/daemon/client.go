package daemon

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
)

type permanentClientError struct {
	err error
}

func (err permanentClientError) Error() string { return err.err.Error() }

func (err permanentClientError) Unwrap() error { return err.err }

func RunClient(ctx context.Context, cfg config.Client) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	tlsConfig, err := loadClientTLSConfig(cfg.ServerCAFile, cfg.ServerName)
	if err != nil {
		return err
	}
	delay := cfg.Reconnect.InitialDelay
	for {
		connected, err := runClientSession(ctx, cfg, tlsConfig)
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}
		if connected {
			delay = cfg.Reconnect.InitialDelay
		}
		var permanent permanentClientError
		if errors.As(err, &permanent) {
			log.Printf("client session stopped permanently: client_id=%s error=%v", cfg.ClientID, err)
			return permanent
		}
		log.Printf("client session failed: client_id=%s connected=%t error=%v retry_in=%s", cfg.ClientID, connected, err, delay)
		if err := waitReconnect(ctx, delay); err != nil {
			return nil
		}
		delay = nextReconnectDelay(delay, cfg.Reconnect.MaxDelay)
	}
}

func runClientSession(ctx context.Context, cfg config.Client, tlsConfig *tls.Config) (bool, error) {
	client, response, err := dialControl(ctx, cfg, tlsConfig)
	if err != nil {
		return false, fmt.Errorf("dial control server: %w", err)
	}
	if !response.Accepted {
		return false, permanentClientError{err: fmt.Errorf("authentication rejected: %s", response.Reason)}
	}
	defer func() { _ = client.Close() }()
	snapshot, err := client.ReadProxySnapshot()
	if err != nil {
		return true, fmt.Errorf("read proxy snapshot: %w", err)
	}
	log.Printf("client control session established: client_id=%s protocol=%s session_id=%s config_version=%d proxies=%d heartbeat_interval=%s", cfg.ClientID, response.SelectedProtocol, response.SessionID, response.ConfigVersion, len(snapshot.Proxies), response.HeartbeatInterval)
	heartbeatDone := make(chan error, 1)
	go func() { heartbeatDone <- sendHeartbeats(ctx, client, response, cfg.ClientID, len(snapshot.Proxies)) }()
	serveDone := make(chan error, 1)
	go func() { serveDone <- client.ServeProxyStreams(ctx) }()
	select {
	case <-ctx.Done():
		_ = client.Close()
		return true, nil
	case err := <-heartbeatDone:
		_ = client.Close()
		if errors.Is(err, context.Canceled) {
			return true, nil
		}
		log.Printf("client heartbeat loop stopped: client_id=%s error=%v", cfg.ClientID, err)
		return true, err
	case err := <-serveDone:
		_ = client.Close()
		if errors.Is(err, context.Canceled) {
			return true, nil
		}
		log.Printf("client proxy stream loop stopped: client_id=%s error=%v", cfg.ClientID, err)
		return true, err
	}
}

func waitReconnect(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nextReconnectDelay(current time.Duration, maxDelay time.Duration) time.Duration {
	if current >= maxDelay {
		return maxDelay
	}
	next := current * 2
	if next < current || next > maxDelay {
		return maxDelay
	}
	return next
}

func dialControl(ctx context.Context, cfg config.Client, tlsConfig *tls.Config) (*control.ClientConn, control.AuthResponse, error) {
	var quicErr error
	if slices.Contains(cfg.AllowedProtocols, domain.ProtocolQUIC) {
		quicCtx := ctx
		cancel := func() {}
		if cfg.ServerTLSAddress != "" && slices.Contains(cfg.AllowedProtocols, domain.ProtocolTCPTLS) {
			quicCtx, cancel = context.WithTimeout(ctx, 500*time.Millisecond)
		}
		client, response, err := control.DialAndAuthenticate(quicCtx, cfg.ServerAddress, tlsConfig, nil, control.NewAuthRequest(cfg.ClientID, cfg.Credential, []domain.Protocol{domain.ProtocolQUIC}))
		cancel()
		if err == nil || response.Reason != "" {
			return client, response, err
		}
		quicErr = err
	}
	if slices.Contains(cfg.AllowedProtocols, domain.ProtocolTCPTLS) {
		address := cfg.ServerTLSAddress
		if address == "" {
			address = cfg.ServerAddress
		}
		client, response, err := control.DialTLSAndAuthenticate(ctx, address, tlsConfig, control.NewAuthRequest(cfg.ClientID, cfg.Credential, []domain.Protocol{domain.ProtocolTCPTLS}))
		if err == nil || quicErr == nil {
			return client, response, err
		}
		return client, response, fmt.Errorf("quic failed: %w; tcp tls failed: %w", quicErr, err)
	}
	if quicErr != nil {
		return nil, control.AuthResponse{}, quicErr
	}
	return nil, control.AuthResponse{}, errors.New("no supported control protocol configured")
}

func sendHeartbeats(ctx context.Context, client *control.ClientConn, response control.AuthResponse, clientID string, activeProxies int) error {
	interval := response.HeartbeatInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if err := client.SendHeartbeat(control.Heartbeat{SessionID: response.SessionID, ClientID: clientID, ConfigVersion: response.ConfigVersion, ActiveProxies: activeProxies}); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := client.SendHeartbeat(control.Heartbeat{SessionID: response.SessionID, ClientID: clientID, ConfigVersion: response.ConfigVersion, ActiveProxies: activeProxies}); err != nil {
				return err
			}
		}
	}
}

func loadClientTLSConfig(caFile string, serverName string) (*tls.Config, error) {
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read server ca file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("server ca file contains no certificates")
	}
	return &tls.Config{RootCAs: pool, ServerName: serverName, MinVersion: tls.VersionTLS13}, nil
}
