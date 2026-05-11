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
	"time"

	"github.com/quic-go/quic-go"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
)

const ControlALPN = "go-ginx-control/1"

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
}

type ClientConn struct {
	conn   *quic.Conn
	stream *quic.Stream
}

type quicStreamOpener struct {
	conn *quic.Conn
}

func (opener quicStreamOpener) OpenStream(ctx context.Context) (io.ReadWriteCloser, error) {
	return opener.conn.OpenStreamSync(ctx)
}

func ListenAddr(addr string, server Server) (*Listener, error) {
	if server.Sessions == nil {
		return nil, errors.New("session manager is required")
	}
	if server.TLSConfig == nil {
		return nil, errors.New("tls config is required")
	}
	tlsConfig := server.TLSConfig.Clone()
	tlsConfig.NextProtos = ensureNextProto(tlsConfig.NextProtos)
	listener, err := quic.ListenAddr(addr, tlsConfig, quicConfig(server.QUICConfig))
	if err != nil {
		return nil, err
	}
	return &Listener{server: server, listener: listener}, nil
}

func (listener *Listener) Addr() net.Addr {
	return listener.listener.Addr()
}

func (listener *Listener) Close() error {
	return listener.listener.Close()
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

func (listener *Listener) handleConn(ctx context.Context, conn *quic.Conn) {
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		_ = conn.CloseWithError(1, "accept stream failed")
		return
	}
	defer stream.Close()

	envelope, err := ReadMessage(stream)
	if err != nil || envelope.Type != MessageAuthRequest {
		_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: "invalid auth request"})
		_ = conn.CloseWithError(2, "invalid auth request")
		return
	}
	request, err := DecodePayload[AuthRequest](envelope)
	if err != nil {
		_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: "invalid auth payload"})
		_ = conn.CloseWithError(3, "invalid auth payload")
		return
	}

	result, err := listener.server.Authenticator.Authenticate(ctx, request)
	if err != nil {
		_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: err.Error()})
		return
	}
	sessionID, err := listener.newSessionID()
	if err != nil {
		_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: "session id generation failed"})
		_ = conn.CloseWithError(5, "session id generation failed")
		return
	}
	registered, _, err := listener.server.Sessions.Register(session.RegisterInput{
		SessionID:     sessionID,
		ClientID:      result.Client.ID,
		UserID:        result.User.ID,
		Protocol:      result.SelectedProtocol,
		ConfigVersion: result.ConfigVersion,
		StreamOpener:  quicStreamOpener{conn: conn},
	})
	if err != nil {
		_ = WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: false, Reason: err.Error()})
		_ = conn.CloseWithError(6, "session registration failed")
		return
	}

	if err := WriteMessage(stream, MessageAuthResponse, AuthResponse{Accepted: true, SessionID: registered.ID, SelectedProtocol: result.SelectedProtocol, HeartbeatInterval: result.HeartbeatInterval, ConfigVersion: result.ConfigVersion}); err != nil {
		_ = conn.CloseWithError(7, "auth response failed")
		return
	}
	if err := listener.sendProxySnapshot(ctx, stream, result.Client.ID, result.ConfigVersion); err != nil {
		_ = conn.CloseWithError(8, "proxy snapshot failed")
		return
	}
	listener.handleHeartbeats(stream)
}

func (listener *Listener) sendProxySnapshot(ctx context.Context, stream *quic.Stream, clientID string, version int64) error {
	proxyRepository := listener.server.Authenticator.Store.Proxies()
	if proxyRepository == nil {
		return WriteMessage(stream, MessageProxySnapshot, ProxySnapshot{Version: version})
	}
	proxies, err := proxyRepository.ByClientID(ctx, clientID)
	if err != nil {
		return err
	}
	return WriteMessage(stream, MessageProxySnapshot, ProxySnapshot{Version: version, Proxies: proxies})
}

func (listener *Listener) handleHeartbeats(stream *quic.Stream) {
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
		_, _ = listener.server.Sessions.Heartbeat(session.HeartbeatInput{
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
	tlsConfig = tlsConfig.Clone()
	tlsConfig.NextProtos = ensureNextProto(tlsConfig.NextProtos)
	conn, err := quic.DialAddr(ctx, addr, tlsConfig, quicConfig(quicConfigValue))
	if err != nil {
		return nil, AuthResponse{}, err
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(1, "open stream failed")
		return nil, AuthResponse{}, err
	}
	if request.Timestamp.IsZero() {
		request.Timestamp = time.Now().UTC()
	}
	if err := WriteMessage(stream, MessageAuthRequest, request); err != nil {
		_ = conn.CloseWithError(2, "auth request failed")
		return nil, AuthResponse{}, err
	}
	envelope, err := ReadMessage(stream)
	if err != nil {
		_ = conn.CloseWithError(3, "auth response failed")
		return nil, AuthResponse{}, err
	}
	if envelope.Type != MessageAuthResponse {
		_ = conn.CloseWithError(4, "unexpected auth response")
		return nil, AuthResponse{}, fmt.Errorf("expected auth response, got %s", envelope.Type)
	}
	response, err := DecodePayload[AuthResponse](envelope)
	if err != nil {
		_ = conn.CloseWithError(5, "decode auth response failed")
		return nil, AuthResponse{}, err
	}
	if !response.Accepted {
		_ = conn.CloseWithError(6, "authentication rejected")
		return nil, response, nil
	}
	return &ClientConn{conn: conn, stream: stream}, response, nil
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

func handleProxyStream(stream *quic.Stream) {
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
	handleTCPStream(stream, request)
}

func handleTCPStream(stream *quic.Stream, request OpenStream) {
	target, err := net.Dial("tcp", net.JoinHostPort(request.TargetHost, strconv.Itoa(request.TargetPort)))
	if err != nil {
		return
	}
	defer target.Close()
	copyBidirectional(stream, target)
}

func handleHTTPStream(stream *quic.Stream, request OpenStream) {
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

func (client *ClientConn) ReadProxySnapshot() (ProxySnapshot, error) {
	envelope, err := ReadMessage(client.stream)
	if err != nil {
		return ProxySnapshot{}, err
	}
	if envelope.Type != MessageProxySnapshot {
		return ProxySnapshot{}, fmt.Errorf("expected proxy snapshot, got %s", envelope.Type)
	}
	return DecodePayload[ProxySnapshot](envelope)
}

func (client *ClientConn) Close() error {
	if client == nil || client.conn == nil {
		return nil
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

func (listener *Listener) newSessionID() (string, error) {
	if listener.server.NewSessionID != nil {
		return listener.server.NewSessionID()
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
