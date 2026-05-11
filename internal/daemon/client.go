package daemon

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/config"
	"github.com/simp-frp/go-ginx-2/internal/control"
)

func RunClient(ctx context.Context, cfg config.Client) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	tlsConfig, err := loadClientTLSConfig(cfg.ServerCAFile, cfg.ServerName)
	if err != nil {
		return err
	}
	client, response, err := control.DialAndAuthenticate(ctx, cfg.ServerAddress, tlsConfig, nil, control.NewAuthRequest(cfg.ClientID, cfg.Credential, cfg.AllowedProtocols))
	if err != nil {
		return fmt.Errorf("dial control server: %w", err)
	}
	if !response.Accepted {
		return fmt.Errorf("authentication rejected: %s", response.Reason)
	}
	defer client.Close()
	snapshot, err := client.ReadProxySnapshot()
	if err != nil {
		return fmt.Errorf("read proxy snapshot: %w", err)
	}
	heartbeatDone := make(chan error, 1)
	go func() { heartbeatDone <- sendHeartbeats(ctx, client, response, cfg.ClientID, len(snapshot.Proxies)) }()
	serveDone := make(chan error, 1)
	go func() { serveDone <- client.ServeProxyStreams(ctx) }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-heartbeatDone:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case err := <-serveDone:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
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
