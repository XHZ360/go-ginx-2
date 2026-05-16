package control

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
)

const ControlALPN = "go-ginx-control/1"

const controlAuthTimeout = 10 * time.Second

type Server struct {
	Authenticator Authenticator
	Sessions      *session.Manager
	TLSConfig     *tls.Config
	QUICConfig    *quic.Config
	NewSessionID  func() (string, error)
}

type Listener struct {
	server   Server
	listener *quic.Listener
	mu       sync.Mutex
	conns    map[*quic.Conn]struct{}
}

type TLSListener struct {
	server     Server
	listener   net.Listener
	handshakes chan struct{}
	mu         sync.Mutex
	conns      map[net.Conn]struct{}
}

type ClientConn struct {
	conn     *quic.Conn
	stream   io.ReadWriteCloser
	mux      *tcpTLSMux
	protocol domain.Protocol
}

type quicStreamOpener struct {
	conn *quic.Conn
}

func (opener quicStreamOpener) OpenStream(ctx context.Context) (io.ReadWriteCloser, error) {
	return opener.conn.OpenStreamSync(ctx)
}

func ListenAddr(addr string, server Server) (*Listener, error) {
	if err := validateServer(server); err != nil {
		return nil, err
	}
	server.Authenticator.AllowedProtocols = []domain.Protocol{domain.ProtocolQUIC}
	tlsConfig := controlTLSConfig(server.TLSConfig)
	listener, err := quic.ListenAddr(addr, tlsConfig, quicConfig(server.QUICConfig))
	if err != nil {
		return nil, err
	}
	return &Listener{server: server, listener: listener, conns: make(map[*quic.Conn]struct{})}, nil
}

func ListenTLSAddr(addr string, server Server) (*TLSListener, error) {
	if err := validateServer(server); err != nil {
		return nil, err
	}
	server.Authenticator.AllowedProtocols = []domain.Protocol{domain.ProtocolTCPTLS}
	listener, err := tls.Listen("tcp", addr, controlTLSConfig(server.TLSConfig))
	if err != nil {
		return nil, err
	}
	return &TLSListener{server: server, listener: listener, handshakes: make(chan struct{}, 128), conns: make(map[net.Conn]struct{})}, nil
}

func validateServer(server Server) error {
	if server.Sessions == nil {
		return errors.New("session manager is required")
	}
	if server.TLSConfig == nil {
		return errors.New("tls config is required")
	}
	return nil
}

func controlTLSConfig(config *tls.Config) *tls.Config {
	tlsConfig := config.Clone()
	tlsConfig.NextProtos = ensureNextProto(tlsConfig.NextProtos)
	return tlsConfig
}

func (listener *Listener) Addr() net.Addr {
	return listener.listener.Addr()
}

func (listener *Listener) Close() error {
	err := listener.listener.Close()
	for _, conn := range listener.activeConns() {
		_ = conn.CloseWithError(0, "server shutdown")
	}
	return err
}

func (listener *Listener) Serve(ctx context.Context) error {
	for {
		conn, err := listener.listener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		go listener.handleConn(ctx, conn)
	}
}

func (listener *TLSListener) Addr() net.Addr {
	return listener.listener.Addr()
}

func (listener *TLSListener) Close() error {
	err := listener.listener.Close()
	for _, conn := range listener.activeConns() {
		_ = conn.Close()
	}
	return err
}

func (listener *TLSListener) Serve(ctx context.Context) error {
	for {
		conn, err := listener.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		go listener.handleConn(ctx, conn)
	}
}

func (listener *TLSListener) handleConn(ctx context.Context, conn net.Conn) {
	listener.trackConn(conn)
	defer listener.untrackConn(conn)
	defer conn.Close()
	select {
	case listener.handshakes <- struct{}{}:
		defer func() { <-listener.handshakes }()
	case <-ctx.Done():
		return
	}
	_ = conn.SetDeadline(time.Now().Add(controlAuthTimeout))
	if tlsConn, ok := conn.(*tls.Conn); ok {
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return
		}
		if tlsConn.ConnectionState().NegotiatedProtocol != ControlALPN {
			return
		}
	}
	_ = conn.SetDeadline(time.Now().Add(controlAuthTimeout))
	mux := newTCPTLSMux(conn, 1)
	listener.server.handleControl(ctx, conn, mux)
}

func (listener *Listener) handleConn(ctx context.Context, conn *quic.Conn) {
	listener.trackConn(conn)
	defer listener.untrackConn(conn)
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		_ = conn.CloseWithError(1, "accept stream failed")
		return
	}
	defer stream.Close()
	listener.handleControl(ctx, stream, quicStreamOpener{conn: conn})
}

func (listener *Listener) trackConn(conn *quic.Conn) {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	listener.conns[conn] = struct{}{}
}

func (listener *Listener) untrackConn(conn *quic.Conn) {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	delete(listener.conns, conn)
}

func (listener *Listener) activeConns() []*quic.Conn {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	conns := make([]*quic.Conn, 0, len(listener.conns))
	for conn := range listener.conns {
		conns = append(conns, conn)
	}
	return conns
}

func (listener *TLSListener) trackConn(conn net.Conn) {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	listener.conns[conn] = struct{}{}
}

func (listener *TLSListener) untrackConn(conn net.Conn) {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	delete(listener.conns, conn)
}

func (listener *TLSListener) activeConns() []net.Conn {
	listener.mu.Lock()
	defer listener.mu.Unlock()
	conns := make([]net.Conn, 0, len(listener.conns))
	for conn := range listener.conns {
		conns = append(conns, conn)
	}
	return conns
}

func (listener *Listener) handleControl(ctx context.Context, stream io.ReadWriteCloser, opener session.StreamOpener) {
	listener.server.handleControl(ctx, stream, opener)
}

func (server Server) handleControl(ctx context.Context, stream io.ReadWriteCloser, opener session.StreamOpener) {
	envelope, err := ReadMessage(stream)
	if err != nil || envelope.Type != MessageAuthRequest {
		_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: "invalid auth request"})
		return
	}
	request, err := DecodePayload[AuthRequest](envelope)
	if err != nil {
		_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: "invalid auth payload"})
		return
	}

	result, err := server.Authenticator.Authenticate(ctx, request)
	if err != nil {
		_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: err.Error()})
		return
	}
	sessionID, err := server.newSessionID()
	if err != nil {
		_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: "session id generation failed"})
		return
	}
	registerInput := session.RegisterInput{
		SessionID:     sessionID,
		ClientID:      result.Client.ID,
		UserID:        result.User.ID,
		Protocol:      result.SelectedProtocol,
		ConfigVersion: result.ConfigVersion,
		StreamOpener:  opener,
	}
	registered := session.Session{ID: sessionID}
	if _, ok := opener.(*tcpTLSMux); !ok {
		registered, _, err = server.Sessions.Register(registerInput)
		if err != nil {
			_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: err.Error()})
			return
		}
	}
	muxReady := false
	if mux, ok := opener.(*tcpTLSMux); ok {
		defer func() {
			if !muxReady {
				_ = mux.Close()
			}
		}()
	}

	if err := WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: true, SessionID: registered.ID, SelectedProtocol: result.SelectedProtocol, HeartbeatInterval: result.HeartbeatInterval, ConfigVersion: result.ConfigVersion}); err != nil {
		return
	}
	if err := server.sendProxySnapshot(ctx, stream, result.Client.ID, result.ConfigVersion); err != nil {
		return
	}
	if conn, ok := stream.(interface{ SetDeadline(time.Time) error }); ok {
		_ = conn.SetDeadline(time.Time{})
	}
	if mux, ok := opener.(*tcpTLSMux); ok {
		mux.Start()
		registered, _, err = server.Sessions.Register(registerInput)
		if err != nil {
			return
		}
		muxReady = true
		stream = mux.ControlStream()
	}
	server.handleHeartbeats(stream)
}

func (server Server) sendProxySnapshot(ctx context.Context, stream io.Writer, clientID string, version int64) error {
	proxyRepository := server.Authenticator.Store.Proxies()
	if proxyRepository == nil {
		return WriteMessage(stream, MessageProxySnapshot, ProxySnapshot{Version: version})
	}
	proxies, err := proxyRepository.ByClientID(ctx, clientID)
	if err != nil {
		return err
	}
	return WriteMessage(stream, MessageProxySnapshot, ProxySnapshot{Version: version, Proxies: proxies})
}

func (server Server) handleHeartbeats(stream io.Reader) {
	for {
		envelope, err := ReadMessage(stream)
		if err != nil {
			return
		}
		if envelope.Type != MessageHeartbeat {
			continue
		}
		heartbeat, err := DecodePayload[Heartbeat](envelope)
		if err != nil {
			continue
		}
		_, _ = server.Sessions.Heartbeat(session.HeartbeatInput{
			SessionID:     heartbeat.SessionID,
			ConfigVersion: heartbeat.ConfigVersion,
			Stats: session.HeartbeatStats{
				ActiveProxies: heartbeat.ActiveProxies,
				ActiveStreams: heartbeat.ActiveStreams,
				UploadBytes:   heartbeat.UploadBytes,
				DownloadBytes: heartbeat.DownloadBytes,
				ErrorSummary:  heartbeat.ErrorSummary,
			},
		})
	}
}

func DialAndAuthenticate(ctx context.Context, addr string, tlsConfig *tls.Config, quicConfigValue *quic.Config, request AuthRequest) (*ClientConn, AuthResponse, error) {
	if tlsConfig == nil {
		return nil, AuthResponse{}, errors.New("tls config is required")
	}
	tlsConfig = controlTLSConfig(tlsConfig)
	conn, err := quic.DialAddr(ctx, addr, tlsConfig, quicConfig(quicConfigValue))
	if err != nil {
		return nil, AuthResponse{}, err
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(1, "open stream failed")
		return nil, AuthResponse{}, err
	}
	client, response, err := authenticateClient(stream, request, domain.ProtocolQUIC)
	if err != nil {
		_ = conn.CloseWithError(2, "auth request failed")
		return nil, response, err
	}
	if !response.Accepted {
		_ = conn.CloseWithError(6, "authentication rejected")
		return nil, response, nil
	}
	client.conn = conn
	return client, response, nil
}

func DialTLSAndAuthenticate(ctx context.Context, addr string, tlsConfig *tls.Config, request AuthRequest) (*ClientConn, AuthResponse, error) {
	if tlsConfig == nil {
		return nil, AuthResponse{}, errors.New("tls config is required")
	}
	dialer := tls.Dialer{Config: controlTLSConfig(tlsConfig)}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, AuthResponse{}, err
	}
	client, response, err := authenticateClient(conn, request, domain.ProtocolTCPTLS)
	if err != nil || !response.Accepted {
		_ = conn.Close()
		return client, response, err
	}
	client.mux = newTCPTLSMux(conn, 2)
	return client, response, nil
}

func authenticateClient(stream io.ReadWriteCloser, request AuthRequest, protocol domain.Protocol) (*ClientConn, AuthResponse, error) {
	if request.Timestamp.IsZero() {
		request.Timestamp = time.Now().UTC()
	}
	if err := WriteMessage(stream, MessageAuthRequest, request); err != nil {
		return nil, AuthResponse{}, err
	}
	envelope, err := ReadMessage(stream)
	if err != nil {
		return nil, AuthResponse{}, err
	}
	if envelope.Type != MessageAuthResponse {
		return nil, AuthResponse{}, fmt.Errorf("expected auth response, got %s", envelope.Type)
	}
	response, err := DecodePayload[AuthResponse](envelope)
	if err != nil {
		return nil, AuthResponse{}, err
	}
	if !response.Accepted {
		return nil, response, nil
	}
	return &ClientConn{stream: stream, protocol: protocol}, response, nil
}

func (client *ClientConn) SendHeartbeat(heartbeat Heartbeat) error {
	if heartbeat.ObservedAt.IsZero() {
		heartbeat.ObservedAt = time.Now().UTC()
	}
	return WriteMessage(client.stream, MessageHeartbeat, heartbeat)
}

func (client *ClientConn) ServeTCPStreams(ctx context.Context) error {
	return client.ServeProxyStreams(ctx)
}

func (client *ClientConn) ServeProxyStreams(ctx context.Context) error {
	if client.mux != nil {
		for {
			stream, err := client.mux.AcceptStream(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
			go handleProxyStream(stream)
		}
	}
	if client.conn == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	for {
		stream, err := client.conn.AcceptStream(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		go handleProxyStream(stream)
	}
}

func handleProxyStream(stream io.ReadWriteCloser) {
	defer stream.Close()
	envelope, err := ReadMessage(stream)
	if err != nil || envelope.Type != MessageOpenStream {
		return
	}
	request, err := DecodePayload[OpenStream](envelope)
	if err != nil {
		return
	}
	if request.Kind == "http" {
		handleHTTPStream(stream, request)
		return
	}
	if request.Kind == "udp" {
		handleUDPStream(stream, request)
		return
	}
	handleTCPStream(stream, request)
}

func handleTCPStream(stream io.ReadWriteCloser, request OpenStream) {
	target, err := net.Dial("tcp", net.JoinHostPort(request.TargetHost, strconv.Itoa(request.TargetPort)))
	if err != nil {
		return
	}
	defer target.Close()
	copyBidirectional(stream, target)
}

func handleHTTPStream(stream io.ReadWriteCloser, request OpenStream) {
	inbound, err := http.ReadRequest(bufio.NewReader(stream))
	if err != nil {
		return
	}
	defer inbound.Body.Close()
	inbound.RequestURI = ""
	inbound.URL.Scheme = "http"
	inbound.URL.Host = net.JoinHostPort(request.TargetHost, strconv.Itoa(request.TargetPort))
	response, err := http.DefaultTransport.RoundTrip(inbound)
	if err != nil {
		response = &http.Response{StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway", Body: io.NopCloser(strings.NewReader("target unreachable\n")), Header: make(http.Header)}
	}
	defer response.Body.Close()
	_ = response.Write(stream)
}

func handleUDPStream(stream io.ReadWriteCloser, request OpenStream) {
	target, err := net.Dial("udp", net.JoinHostPort(request.TargetHost, strconv.Itoa(request.TargetPort)))
	if err != nil {
		return
	}
	defer target.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			payload, err := ReadDatagramFrame(stream)
			if err != nil {
				return
			}
			if _, err := target.Write(payload); err != nil {
				return
			}
		}
	}()
	buffer := make([]byte, 64*1024)
	for {
		select {
		case <-done:
			return
		default:
		}
		_ = target.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		n, err := target.Read(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}
		if err := WriteDatagramFrame(stream, buffer[:n]); err != nil {
			return
		}
	}
}

func (client *ClientConn) ReadProxySnapshot() (ProxySnapshot, error) {
	envelope, err := ReadMessage(client.stream)
	if err != nil {
		return ProxySnapshot{}, err
	}
	if envelope.Type != MessageProxySnapshot {
		return ProxySnapshot{}, fmt.Errorf("expected proxy snapshot, got %s", envelope.Type)
	}
	snapshot, err := DecodePayload[ProxySnapshot](envelope)
	if err != nil {
		return ProxySnapshot{}, err
	}
	if client.protocol == domain.ProtocolTCPTLS && client.mux != nil {
		client.mux.Start()
		client.stream = client.mux.ControlStream()
	}
	return snapshot, nil
}

func (client *ClientConn) Close() error {
	if client == nil {
		return nil
	}
	if client.conn == nil {
		if client.mux != nil {
			return client.mux.Close()
		}
		if client.stream == nil {
			return nil
		}
		return client.stream.Close()
	}
	return client.conn.CloseWithError(0, "closed")
}

func copyBidirectional(left io.ReadWriteCloser, right io.ReadWriteCloser) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(left, right)
		_ = left.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(right, left)
		_ = right.Close()
		done <- struct{}{}
	}()
	<-done
}

func (server Server) newSessionID() (string, error) {
	if server.NewSessionID != nil {
		return server.NewSessionID()
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func ensureNextProto(nextProtos []string) []string {
	if slices.Contains(nextProtos, ControlALPN) {
		return nextProtos
	}
	return append(append([]string{}, nextProtos...), ControlALPN)
}

func quicConfig(config *quic.Config) *quic.Config {
	if config != nil {
		return config.Clone()
	}
	return DefaultQUICConfig()
}

func NewAuthRequest(clientID string, credential string, protocols []domain.Protocol) AuthRequest {
	return AuthRequest{ClientID: clientID, Credential: credential, Timestamp: time.Now().UTC(), Protocols: protocols}
}
