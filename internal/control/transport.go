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
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/proxy/tunnel"
	"github.com/simp-frp/go-ginx-2/internal/session"
)

const ControlALPN = "go-ginx-control/1"

const controlAuthTimeout = 10 * time.Second
const httpTargetDialTimeout = 10 * time.Second
const httpUpgradeHandshakeTimeout = 30 * time.Second

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
	registeredSessionID := ""
	markedOnline := false
	defer func() {
		if registeredSessionID == "" {
			return
		}
		closed, latest, err := server.Sessions.Close(registeredSessionID)
		if err != nil {
			log.Printf("control session close failed: session_id=%s error=%v", registeredSessionID, err)
			return
		}
		if markedOnline && latest {
			server.setClientRuntimeStatus(context.Background(), closed.ClientID, domain.ClientOffline)
		}
		log.Printf("control session closed: client_id=%s protocol=%s session_id=%s latest=%t", closed.ClientID, closed.Protocol, closed.ID, latest)
	}()

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
		log.Printf("control authentication rejected: client_id=%s error=%v", request.ClientID, err)
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
		var replaced *session.Session
		registered, replaced, err = server.Sessions.Register(registerInput)
		if err != nil {
			_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: err.Error()})
			return
		}
		if replaced != nil {
			log.Printf("control session replaced: client_id=%s old_session_id=%s new_session_id=%s protocol=%s", replaced.ClientID, replaced.ID, registered.ID, result.SelectedProtocol)
		}
		registeredSessionID = registered.ID
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
	log.Printf("control client authenticated: client_id=%s user_id=%s protocol=%s session_id=%s config_version=%d", result.Client.ID, result.User.ID, result.SelectedProtocol, registered.ID, result.ConfigVersion)
	if err := server.sendProxySnapshot(ctx, stream, result.Client.ID, result.ConfigVersion); err != nil {
		log.Printf("control proxy snapshot failed: client_id=%s session_id=%s error=%v", result.Client.ID, registered.ID, err)
		return
	}
	if conn, ok := stream.(interface{ SetDeadline(time.Time) error }); ok {
		_ = conn.SetDeadline(time.Time{})
	}
	if mux, ok := opener.(*tcpTLSMux); ok {
		mux.Start()
		var replaced *session.Session
		registered, replaced, err = server.Sessions.Register(registerInput)
		if err != nil {
			log.Printf("control tcp tls session register failed: client_id=%s session_id=%s error=%v", result.Client.ID, sessionID, err)
			return
		}
		if replaced != nil {
			log.Printf("control session replaced: client_id=%s old_session_id=%s new_session_id=%s protocol=%s", replaced.ClientID, replaced.ID, registered.ID, result.SelectedProtocol)
		}
		registeredSessionID = registered.ID
		muxReady = true
		stream = mux.ControlStream()
	}
	if server.setClientRuntimeStatus(ctx, result.Client.ID, domain.ClientOnline) {
		markedOnline = true
	}
	server.handleHeartbeats(stream)
}

func (server Server) setClientRuntimeStatus(ctx context.Context, clientID string, status domain.ClientStatus) bool {
	if server.Authenticator.Store == nil || server.Authenticator.Store.Clients() == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := server.Authenticator.Store.Clients().ByID(ctx, clientID)
	if err != nil {
		log.Printf("control client status lookup failed: client_id=%s status=%s error=%v", clientID, status, err)
		return false
	}
	if client.Status == domain.ClientDisabled {
		return false
	}
	if err := server.Authenticator.Store.Clients().SetStatus(ctx, clientID, status); err != nil {
		log.Printf("control client status update failed: client_id=%s status=%s error=%v", clientID, status, err)
		return false
	}
	return true
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
	defer CloseStream(stream)
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
	tunnel.CopyBidirectional(stream, target)
}

func handleHTTPStream(stream io.ReadWriteCloser, request OpenStream) {
	inboundReader := bufio.NewReader(stream)
	inbound, err := http.ReadRequest(inboundReader)
	if err != nil {
		return
	}
	defer inbound.Body.Close()
	targetAuthority := net.JoinHostPort(request.TargetHost, strconv.Itoa(request.TargetPort))
	inbound.RequestURI = ""
	inbound.URL.Scheme = "http"
	inbound.URL.Host = targetAuthority
	inbound.Host = targetAuthority
	rewriteHTTPOrigin(inbound.Header, targetAuthority)
	if tunnel.IsWebSocketUpgrade(inbound.Header) {
		handleHTTPUpgradeStream(stream, inboundReader, inbound, targetAuthority)
		return
	}
	response, err := http.DefaultTransport.RoundTrip(inbound)
	if err != nil {
		response = &http.Response{StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway", Body: io.NopCloser(strings.NewReader("target unreachable\n")), Header: make(http.Header)}
	}
	defer response.Body.Close()
	_ = response.Write(stream)
}

func handleHTTPUpgradeStream(stream io.ReadWriteCloser, inboundReader *bufio.Reader, inbound *http.Request, targetAuthority string) {
	tunnel.NormalizeWebSocketRequest(inbound)
	ctx, cancel := context.WithTimeout(context.Background(), httpTargetDialTimeout)
	defer cancel()
	dialer := net.Dialer{Timeout: httpTargetDialTimeout}
	target, err := dialer.DialContext(ctx, "tcp", targetAuthority)
	if err != nil {
		_ = writeBadGateway(stream)
		return
	}
	defer target.Close()
	_ = target.SetDeadline(time.Now().Add(httpUpgradeHandshakeTimeout))
	if err := inbound.Write(target); err != nil {
		_ = writeBadGateway(stream)
		return
	}
	targetReader := bufio.NewReader(target)
	response, err := http.ReadResponse(targetReader, inbound)
	if err != nil {
		_ = writeBadGateway(stream)
		return
	}
	defer response.Body.Close()
	if err := response.Write(stream); err != nil {
		return
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		return
	}
	_ = target.SetDeadline(time.Time{})
	controlSide := tunnel.WithBufferedReader(stream, inboundReader)
	targetSide := tunnel.WithBufferedReader(target, targetReader)
	tunnel.CopyBidirectional(controlSide, targetSide)
}

func writeBadGateway(writer io.Writer) error {
	response := &http.Response{StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway", Body: io.NopCloser(strings.NewReader("target unreachable\n")), Header: make(http.Header)}
	return response.Write(writer)
}

func rewriteHTTPOrigin(header http.Header, targetAuthority string) {
	origin := header.Get("Origin")
	if origin == "" {
		return
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return
	}
	header.Set("Origin", (&url.URL{Scheme: "http", Host: targetAuthority}).String())
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

func CloseStream(stream io.ReadWriteCloser) error {
	if stream == nil {
		return nil
	}
	if canceler, ok := stream.(interface{ CancelRead(quic.StreamErrorCode) }); ok {
		canceler.CancelRead(0)
	}
	return stream.Close()
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
