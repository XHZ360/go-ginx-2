package sdk

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
)

// Client is the SDK entry point. Create one with New, then call Connect to
// establish a control channel. After connecting, use Proxies to list available
// proxies and Dial/DialTCP to open data streams.
type Client struct {
	cfg     Config
	conn    *control.ClientConn
	mu      sync.Mutex
	proxies []ProxyInfo
}

// New creates a new SDK client with the given configuration. It does not
// establish any network connections.
func New(cfg Config) *Client {
	return &Client{cfg: cfg}
}

// Connect establishes the control channel to the GoGinX server using consumer
// credentials. It authenticates and receives the initial proxy list.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return ErrAlreadyConnected
	}
	if err := c.cfg.validate(); err != nil {
		return err
	}

	tlsConfig, err := c.buildTLSConfig()
	if err != nil {
		return err
	}

	authRequest := control.AuthRequest{
		ClientID:   c.cfg.ClientID,
		Credential: c.cfg.Credential,
		Protocols:  c.resolveProtocols(),
	}

	var clientConn *control.ClientConn
	var authResponse control.AuthResponse

	if c.shouldUseProtocol("quic") && c.cfg.ServerAddress != "" {
		clientConn, authResponse, err = control.DialAndAuthenticate(ctx, c.cfg.ServerAddress, tlsConfig, nil, authRequest)
		if err == nil && !authResponse.Accepted {
			if clientConn != nil {
				_ = clientConn.Close()
			}
			return ErrAuthenticationFailed
		}
	}
	if clientConn == nil && c.shouldUseProtocol("tcp_tls") && c.cfg.ServerTLSAddress != "" {
		clientConn, authResponse, err = control.DialTLSAndAuthenticate(ctx, c.cfg.ServerTLSAddress, tlsConfig, authRequest)
		if err == nil && !authResponse.Accepted {
			if clientConn != nil {
				_ = clientConn.Close()
			}
			return ErrAuthenticationFailed
		}
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAuthenticationFailed, err)
	}
	if clientConn == nil {
		return ErrNotConnected
	}
	if !authResponse.Accepted {
		_ = clientConn.Close()
		return ErrAuthenticationFailed
	}

	// Read the initial proxy list response from the server.
	envelope, readErr := control.ReadMessage(clientConn.RawStream())
	if readErr != nil {
		_ = clientConn.Close()
		return fmt.Errorf("sdk: read initial proxy list: %w", readErr)
	}
	if envelope.Type == control.MessageProxyListResponse {
		listResp, decodeErr := control.DecodePayload[control.ProxyListResponse](envelope)
		if decodeErr != nil {
			_ = clientConn.Close()
			return fmt.Errorf("sdk: decode proxy list: %w", decodeErr)
		}
		c.proxies = proxyListFromDomain(listResp.Proxies)
	} else if envelope.Type == control.MessageProxySnapshot {
		snapshot, decodeErr := control.DecodePayload[control.ProxySnapshot](envelope)
		if decodeErr != nil {
			_ = clientConn.Close()
			return fmt.Errorf("sdk: decode proxy snapshot: %w", decodeErr)
		}
		c.proxies = proxyListFromDomain(snapshot.Proxies)
	}

	// For TCP+TLS connections, start the mux after reading the initial
	// message. The server switches to mux framing after sending the initial
	// response; without this, Proxies() and Dial() would write/read
	// unframed bytes on a mux-framed connection.
	clientConn.StartMux()

	c.conn = clientConn
	return nil
}

// Close closes the control channel and releases resources.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	c.proxies = nil
	return err
}

// Proxies returns the list of proxies accessible to this consumer. The list
// is populated during Connect and can be refreshed by sending a proxy list
// request.
func (c *Client) Proxies(ctx context.Context) ([]ProxyInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil, ErrNotConnected
	}

	// Send a proxy list request to refresh the list.
	if err := control.WriteMessage(c.conn.RawStream(), control.MessageProxyListRequest, control.ProxyListRequest{}); err != nil {
		return nil, fmt.Errorf("sdk: send proxy list request: %w", err)
	}
	envelope, err := control.ReadMessage(c.conn.RawStream())
	if err != nil {
		return nil, fmt.Errorf("sdk: read proxy list response: %w", err)
	}
	if envelope.Type != control.MessageProxyListResponse {
		return nil, fmt.Errorf("sdk: unexpected message type: %s", envelope.Type)
	}
	listResp, err := control.DecodePayload[control.ProxyListResponse](envelope)
	if err != nil {
		return nil, fmt.Errorf("sdk: decode proxy list: %w", err)
	}
	c.proxies = proxyListFromDomain(listResp.Proxies)
	out := make([]ProxyInfo, len(c.proxies))
	copy(out, c.proxies)
	return out, nil
}

// Dial opens a multiplexed data stream to the specified proxy. The returned
// io.ReadWriteCloser is backed by the control channel multiplexer; data written
// is forwarded to the proxy's fixed target, and data from the target can be
// read back.
func (c *Client) Dial(ctx context.Context, proxyID string) (io.ReadWriteCloser, error) {
	c.mu.Lock()
	conn := c.conn
	knownProxy := false
	for _, proxy := range c.proxies {
		if proxy.ID == proxyID {
			knownProxy = true
			break
		}
	}
	c.mu.Unlock()

	if conn == nil {
		return nil, ErrNotConnected
	}
	if !knownProxy {
		return nil, ErrProxyNotFound
	}

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDialFailed, err)
	}

	openMsg := control.OpenStream{
		ProxyID:      proxyID,
		ConnectionID: proxyID,
	}
	if err := control.WriteMessage(stream, control.MessageOpenStream, openMsg); err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("%w: %v", ErrDialFailed, err)
	}

	return newProxyConn(stream, proxyID), nil
}

// DialTCP opens a data stream to the proxy and returns a net.Conn-compatible
// connection. The returned connection's LocalAddr and RemoteAddr return
// synthetic addresses identifying the proxy.
func (c *Client) DialTCP(ctx context.Context, proxyID string) (net.Conn, error) {
	conn, err := c.Dial(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	if tc, ok := conn.(net.Conn); ok {
		return tc, nil
	}
	return newProxyConn(conn.(io.ReadWriteCloser), proxyID), nil
}

// HTTPTransport returns an *http.Transport that sends HTTP requests through
// the specified proxy. All requests made via the returned transport are
// forwarded to the proxy's fixed target.
func (c *Client) HTTPTransport(proxyID string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return c.DialTCP(ctx, proxyID)
		},
	}
}

func (c *Client) buildTLSConfig() (*tls.Config, error) {
	pool := x509.NewCertPool()
	if c.cfg.ServerCAFile != "" {
		pem, err := os.ReadFile(c.cfg.ServerCAFile)
		if err != nil {
			return nil, fmt.Errorf("sdk: read CA file: %w", err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("sdk: no certificates found in CA file")
		}
	}
	return &tls.Config{
		RootCAs:    pool,
		ServerName: c.cfg.ServerName,
		NextProtos: []string{control.ControlALPN},
		MinVersion: tls.VersionTLS13,
	}, nil
}

func (c *Client) resolveProtocols() []domain.Protocol {
	if len(c.cfg.AllowedProtocols) == 0 {
		return []domain.Protocol{domain.ProtocolQUIC, domain.ProtocolTCPTLS}
	}
	protocols := make([]domain.Protocol, 0, len(c.cfg.AllowedProtocols))
	for _, p := range c.cfg.AllowedProtocols {
		protocols = append(protocols, domain.Protocol(p))
	}
	return protocols
}

func (c *Client) shouldUseProtocol(name string) bool {
	if len(c.cfg.AllowedProtocols) == 0 {
		return true
	}
	for _, p := range c.cfg.AllowedProtocols {
		if p == name {
			return true
		}
	}
	return false
}

func proxyListFromDomain(proxies []domain.Proxy) []ProxyInfo {
	out := make([]ProxyInfo, 0, len(proxies))
	for _, p := range proxies {
		out = append(out, ProxyInfo{
			ID:          p.ID,
			Name:        p.Name,
			Type:        string(p.Type),
			EntryHost:   p.EntryHost,
			EntryPort:   p.EntryPort,
			TargetHost:  p.TargetHost,
			TargetPort:  p.TargetPort,
			Description: p.Description,
		})
	}
	return out
}
