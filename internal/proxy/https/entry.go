package httpsproxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/proxy/tunnel"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/store"
)

const tlsHandshakeRecord = 22

const (
	clientHelloTimeout = 10 * time.Second
	httpsReadTimeout   = 30 * time.Second
	maxClientHelloSize = 1 << 20
	maxHTTPBodyBytes   = 32 << 20
)

var errHTTPBodyTooLarge = errors.New("http body too large")

type Entry struct {
	Store                store.Store
	Sessions             *session.Manager
	ListenAddress        string
	EntryBindHost        string
	EntryPort            int
	IncludeDefaultRoutes bool
	CertificateDir       string
	NewConnection        func() (string, error)
}

type Listener struct {
	entry    Entry
	listener net.Listener
	resolver *CertificateResolver
}

func Listen(entry Entry) (*Listener, error) {
	if entry.Store == nil {
		return nil, errors.New("store is required")
	}
	if entry.Sessions == nil {
		return nil, errors.New("session manager is required")
	}
	listener, err := net.Listen("tcp", entry.ListenAddress)
	if err != nil {
		return nil, err
	}
	return &Listener{entry: entry, listener: listener, resolver: NewCertificateResolver(entry.Store, entry.CertificateDir)}, nil
}

func (listener *Listener) Addr() net.Addr {
	return listener.listener.Addr()
}

func (listener *Listener) SetEntryPort(port int) {
	listener.entry.EntryPort = port
}

func (listener *Listener) Close() error {
	return listener.listener.Close()
}

func (listener *Listener) Serve(ctx context.Context) error {
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

func (listener *Listener) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(clientHelloTimeout))
	prefix, serverName, err := readClientHello(conn)
	if err != nil || serverName == "" {
		return
	}
	proxy, err := listener.entry.Store.Proxies().ByHTTPSRoute(ctx, listener.entry.EntryBindHost, listener.entry.EntryPort, serverName, listener.entry.IncludeDefaultRoutes)
	if err != nil || proxy.Status != domain.ProxyEnabled {
		log.Printf("https proxy route failed: bind_host=%s port=%d sni=%s category=route_miss", displayBindHost(listener.entry.EntryBindHost), listener.entry.EntryPort, serverName)
		return
	}
	certificate, err := listener.resolver.Certificate(ctx, serverName, proxy)
	if err != nil {
		log.Printf("https proxy route failed: bind_host=%s port=%d sni=%s proxy_id=%s category=certificate_unavailable error=%v", displayBindHost(listener.entry.EntryBindHost), listener.entry.EntryPort, serverName, proxy.ID, err)
		return
	}
	if certificate == nil {
		log.Printf("https proxy route failed: bind_host=%s port=%d sni=%s proxy_id=%s category=certificate_missing", displayBindHost(listener.entry.EntryBindHost), listener.entry.EntryPort, serverName, proxy.ID)
		return
	}
	listener.handleTerminatedConn(ctx, conn, prefix, proxy, *certificate)
}

func (listener *Listener) handleTerminatedConn(ctx context.Context, conn net.Conn, prefix []byte, proxy domain.Proxy, certificate tls.Certificate) {
	tlsConn := tls.Server(&prefixedConn{Conn: conn, reader: bytes.NewReader(prefix)}, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
	_ = tlsConn.SetDeadline(time.Now().Add(httpsReadTimeout))
	if err := tlsConn.Handshake(); err != nil {
		return
	}
	requestReader := bufio.NewReader(tlsConn)
	request, err := http.ReadRequest(requestReader)
	if err != nil {
		return
	}
	defer request.Body.Close()
	if request.ContentLength > maxHTTPBodyBytes {
		_ = writeSimpleResponse(tlsConn, http.StatusRequestEntityTooLarge, "request body too large\n")
		return
	}
	request.Body = &maxBytesReadCloser{reader: request.Body, close: request.Body.Close, remaining: maxHTTPBodyBytes}
	if isAccessActivationPath(request.URL.Path) {
		listener.handleAccessActivation(ctx, tlsConn, request, proxy)
		return
	}
	if proxy.AccessAuthEnabled {
		if err := listener.authenticateAccessRequest(ctx, request, proxy); err != nil {
			_ = writeSimpleResponse(tlsConn, http.StatusUnauthorized, "access activation required\n")
			return
		}
	}
	if !domain.ValidProxyRequestPath(request.URL.Path, request.URL.RawPath) {
		_ = writeSimpleResponse(tlsConn, http.StatusBadRequest, "invalid request path\n")
		return
	}
	targetHost, targetPort := proxy.TargetHost, proxy.TargetPort
	clientID := proxy.ClientID
	if routes, ok := store.Routes(listener.entry.Store); ok {
		configured, routeErr := routes.ListByProxyID(ctx, proxy.ID)
		if routeErr != nil {
			_ = writeSimpleResponse(tlsConn, http.StatusBadGateway, "proxy route unavailable\n")
			return
		}
		if route, matched := domain.SelectProxyRoute(configured, request.URL.Path); matched {
			clientID = route.ClientID
			targetHost, targetPort = route.TargetHost, route.TargetPort
			request.URL.Path = domain.RewriteProxyRoutePath(request.URL.Path, route)
			request.URL.RawPath = ""
		}
	}

	latest, ok := listener.entry.Sessions.Latest(clientID)
	if !ok || latest.StreamOpener == nil {
		log.Printf("https proxy route failed: bind_host=%s port=%d sni=%s proxy_id=%s category=client_offline", displayBindHost(listener.entry.EntryBindHost), listener.entry.EntryPort, proxy.EntryHost, proxy.ID)
		_ = writeSimpleResponse(tlsConn, http.StatusServiceUnavailable, "client offline\n")
		return
	}
	stream, err := latest.StreamOpener.OpenStream(ctx)
	if err != nil {
		log.Printf("https proxy route failed: bind_host=%s port=%d sni=%s proxy_id=%s category=open_stream_failed error=%v", displayBindHost(listener.entry.EntryBindHost), listener.entry.EntryPort, proxy.EntryHost, proxy.ID, err)
		_ = writeSimpleResponse(tlsConn, http.StatusBadGateway, "open proxy stream failed\n")
		return
	}
	defer stream.Close()
	connectionID, err := listener.connectionID()
	if err != nil {
		_ = writeSimpleResponse(tlsConn, http.StatusInternalServerError, "request id failed\n")
		return
	}
	if err := control.WriteMessage(stream, control.MessageOpenStream, control.OpenStream{Kind: "http", ProxyID: proxy.ID, ConnectionID: connectionID, TargetHost: targetHost, TargetPort: targetPort}); err != nil {
		_ = writeSimpleResponse(tlsConn, http.StatusBadGateway, "write proxy stream failed\n")
		return
	}
	if err := request.Write(stream); errors.Is(err, errHTTPBodyTooLarge) {
		_ = writeSimpleResponse(tlsConn, http.StatusRequestEntityTooLarge, "request body too large\n")
		return
	} else if err != nil {
		_ = writeSimpleResponse(tlsConn, http.StatusBadGateway, "write proxy request failed\n")
		return
	}
	response, responseReader, err := readResponseWithTimeout(stream, request, httpsReadTimeout)
	if err != nil {
		_ = writeSimpleResponse(tlsConn, http.StatusBadGateway, "read proxy response failed\n")
		return
	}
	defer response.Body.Close()
	if tunnel.IsWebSocketUpgrade(request.Header) && response.StatusCode == http.StatusSwitchingProtocols {
		_ = tlsConn.SetDeadline(time.Now().Add(httpsReadTimeout))
		if err := response.Write(tlsConn); err != nil {
			return
		}
		_ = tlsConn.SetDeadline(time.Time{})
		publicSide := tunnel.WithBufferedReader(tlsConn, requestReader)
		streamSide := tunnel.WithBufferedReader(stream, responseReader)
		tunnel.CopyBidirectional(publicSide, streamSide)
		return
	}
	_ = writeResponseWithTimeout(tlsConn, stream, response, httpsReadTimeout)
}

func isAccessActivationPath(path string) bool {
	return strings.HasPrefix(path, "/.well-known/goginx/activate/")
}

func (listener *Listener) authenticateAccessRequest(ctx context.Context, request *http.Request, proxy domain.Proxy) error {
	access, ok := store.Access(listener.entry.Store)
	if !ok {
		return errors.New("proxy access repository is unavailable")
	}
	cookie, err := request.Cookie(accessCookieName(proxy.ID))
	if err != nil || cookie.Value == "" {
		return errors.New("access cookie is missing")
	}
	if err := access.ValidateAccessCredential(ctx, proxy.ID, proxy.AccessAuthVersion, hashAccessValue(cookie.Value), time.Now().UTC()); err != nil {
		return err
	}
	removeCookie(request, cookie.Name)
	return nil
}

func (listener *Listener) handleAccessActivation(ctx context.Context, conn net.Conn, request *http.Request, proxy domain.Proxy) {
	access, ok := store.Access(listener.entry.Store)
	if !ok {
		_ = writeSimpleResponse(conn, http.StatusInternalServerError, "access storage unavailable\n")
		return
	}
	tokenValue := strings.TrimPrefix(request.URL.Path, "/.well-known/goginx/activate/")
	if tokenValue == "" || strings.Contains(tokenValue, "/") {
		_ = writeSimpleResponse(conn, http.StatusNotFound, "activation link unavailable\n")
		return
	}
	token, err := access.ActivationToken(ctx, proxy.ID, hashAccessValue(tokenValue), time.Now().UTC())
	if err != nil || token.AuthVersion != proxy.AccessAuthVersion {
		_ = writeSimpleResponse(conn, http.StatusNotFound, "activation link unavailable\n")
		return
	}
	if request.Method != http.MethodPost {
		writeAccessResponse(conn, http.StatusOK, "<!doctype html><meta charset=utf-8><title>Activate access</title><form method=post><button type=submit>Activate access</button></form>", nil)
		return
	}
	secret := newAccessSecret()
	if secret == "" {
		_ = writeSimpleResponse(conn, http.StatusInternalServerError, "access secret generation failed\n")
		return
	}
	credential := domain.ProxyAccessCredential{ID: newAccessID(), ProxyID: proxy.ID, AuthVersion: proxy.AccessAuthVersion}
	if credential.ID == "" {
		_ = writeSimpleResponse(conn, http.StatusInternalServerError, "access credential generation failed\n")
		return
	}
	if err := access.RedeemActivationToken(ctx, token.ID, hashAccessValue(secret), credential, time.Now().UTC()); err != nil {
		_ = writeSimpleResponse(conn, http.StatusNotFound, "activation link unavailable\n")
		return
	}
	header := http.Header{}
	header.Set("Set-Cookie", (&http.Cookie{Name: accessCookieName(proxy.ID), Value: secret, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}).String())
	header.Set("Location", "/")
	writeAccessResponse(conn, http.StatusSeeOther, "", header)
}

func accessCookieName(proxyID string) string {
	return "__Host-goginx-access-" + hashAccessValue(proxyID)[:12]
}

func hashAccessValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func newAccessSecret() string {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(value[:])
}

func newAccessID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(value[:])
}

func removeCookie(request *http.Request, name string) {
	cookies := request.Cookies()
	values := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Name != name {
			values = append(values, cookie.Name+"="+cookie.Value)
		}
	}
	if len(values) == 0 {
		request.Header.Del("Cookie")
	} else {
		request.Header.Set("Cookie", strings.Join(values, "; "))
	}
}

func writeAccessResponse(conn net.Conn, status int, body string, headers http.Header) error {
	response := &http.Response{StatusCode: status, Status: http.StatusText(status), Header: headers, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
	if response.Header == nil {
		response.Header = make(http.Header)
	}
	response.Header.Set("Cache-Control", "no-store")
	response.Header.Set("Referrer-Policy", "no-referrer")
	response.Header.Set("X-Content-Type-Options", "nosniff")
	response.Header.Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; style-src 'unsafe-inline'")
	if body != "" {
		response.Header.Set("Content-Type", "text/html; charset=utf-8")
	}
	return response.Write(conn)
}

type responseResult struct {
	response *http.Response
	reader   *bufio.Reader
	err      error
}

func readResponseWithTimeout(stream io.ReadWriteCloser, request *http.Request, timeout time.Duration) (*http.Response, *bufio.Reader, error) {
	result := make(chan responseResult, 1)
	go func() {
		reader := bufio.NewReader(stream)
		response, err := http.ReadResponse(reader, request)
		result <- responseResult{response: response, reader: reader, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case response := <-result:
		return response.response, response.reader, response.err
	case <-timer.C:
		_ = stream.Close()
		return nil, nil, context.DeadlineExceeded
	}
}

func writeResponseWithTimeout(conn net.Conn, stream io.Closer, response *http.Response, timeout time.Duration) error {
	result := make(chan error, 1)
	_ = conn.SetDeadline(time.Now().Add(timeout))
	go func() {
		result <- response.Write(conn)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		_ = stream.Close()
		_ = conn.Close()
		return context.DeadlineExceeded
	}
}

func (listener *Listener) certificateFile(path string) (string, error) {
	return resolveCertificateFile(path, listener.entry.CertificateDir)
}

func writeSimpleResponse(writer io.Writer, statusCode int, body string) error {
	response := &http.Response{StatusCode: statusCode, Status: http.StatusText(statusCode), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	response.Header.Set("Content-Type", "text/plain; charset=utf-8")
	response.ContentLength = int64(len(body))
	return response.Write(writer)
}

func displayBindHost(host string) string {
	host = domain.NormalizeBindHost(host)
	if host == "" {
		return "*"
	}
	return host
}

func (listener *Listener) connectionID() (string, error) {
	if listener.entry.NewConnection != nil {
		return listener.entry.NewConnection()
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func readClientHello(conn net.Conn) ([]byte, string, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, "", err
	}
	if header[0] != tlsHandshakeRecord {
		return nil, "", errors.New("expected tls handshake record")
	}
	body := make([]byte, int(binary.BigEndian.Uint16(header[3:5])))
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, "", err
	}
	prefix := append(header, body...)
	handshake := append([]byte(nil), body...)
	for len(handshake) < 4 {
		var additionalHeader []byte
		var additionalBody []byte
		var err error
		additionalHeader, additionalBody, err = readHandshakeContinuation(conn)
		if err != nil {
			return nil, "", err
		}
		prefix = append(prefix, additionalHeader...)
		prefix = append(prefix, additionalBody...)
		handshake = append(handshake, additionalBody...)
	}
	if handshake[0] != 1 {
		return nil, "", errors.New("expected client hello")
	}
	handshakeLength := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
	if handshakeLength > maxClientHelloSize {
		return nil, "", errors.New("client hello too large")
	}
	for len(handshake) < 4+handshakeLength {
		additionalHeader, additionalBody, err := readHandshakeContinuation(conn)
		if err != nil {
			return nil, "", err
		}
		prefix = append(prefix, additionalHeader...)
		prefix = append(prefix, additionalBody...)
		handshake = append(handshake, additionalBody...)
	}
	parseRecord := append(append([]byte(nil), header...), handshake...)
	serverName, err := parseServerName(parseRecord)
	if err != nil {
		return nil, "", err
	}
	return prefix, strings.ToLower(serverName), nil
}

func readHandshakeContinuation(conn net.Conn) ([]byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, nil, err
	}
	if header[0] != tlsHandshakeRecord {
		return nil, nil, errors.New("expected tls handshake continuation")
	}
	body := make([]byte, int(binary.BigEndian.Uint16(header[3:5])))
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, nil, err
	}
	return header, body, nil
}

func parseServerName(record []byte) (string, error) {
	if len(record) < 9 || record[0] != tlsHandshakeRecord || record[5] != 1 {
		return "", errors.New("expected client hello")
	}
	length := int(record[6])<<16 | int(record[7])<<8 | int(record[8])
	body := record[9:]
	if length > len(body) {
		return "", errors.New("truncated client hello")
	}
	body = body[:length]
	position := 34
	if len(body) < position+1 {
		return "", errors.New("client hello missing session id")
	}
	sessionIDLength := int(body[position])
	position += 1 + sessionIDLength
	if len(body) < position+2 {
		return "", errors.New("client hello missing cipher suites")
	}
	cipherSuiteLength := int(binary.BigEndian.Uint16(body[position : position+2]))
	position += 2 + cipherSuiteLength
	if len(body) < position+1 {
		return "", errors.New("client hello missing compression methods")
	}
	compressionLength := int(body[position])
	position += 1 + compressionLength
	if len(body) < position+2 {
		return "", errors.New("client hello missing extensions")
	}
	extensionsLength := int(binary.BigEndian.Uint16(body[position : position+2]))
	position += 2
	if len(body) < position+extensionsLength {
		return "", errors.New("truncated client hello extensions")
	}
	extensions := body[position : position+extensionsLength]
	for len(extensions) >= 4 {
		extensionType := binary.BigEndian.Uint16(extensions[:2])
		extensionLength := int(binary.BigEndian.Uint16(extensions[2:4]))
		extensions = extensions[4:]
		if len(extensions) < extensionLength {
			return "", errors.New("truncated extension")
		}
		extension := extensions[:extensionLength]
		extensions = extensions[extensionLength:]
		if extensionType == 0 {
			return parseServerNameExtension(extension)
		}
	}
	return "", errors.New("server name extension not found")
}

func parseServerNameExtension(extension []byte) (string, error) {
	if len(extension) < 2 {
		return "", errors.New("empty server name extension")
	}
	listLength := int(binary.BigEndian.Uint16(extension[:2]))
	names := extension[2:]
	if len(names) < listLength {
		return "", errors.New("truncated server name list")
	}
	names = names[:listLength]
	for len(names) >= 3 {
		nameType := names[0]
		nameLength := int(binary.BigEndian.Uint16(names[1:3]))
		names = names[3:]
		if len(names) < nameLength {
			return "", errors.New("truncated server name")
		}
		name := string(names[:nameLength])
		if nameType == 0 && name != "" {
			return name, nil
		}
		names = names[nameLength:]
	}
	return "", errors.New("dns server name not found")
}

type prefixedConn struct {
	net.Conn
	reader *bytes.Reader
}

type maxBytesReadCloser struct {
	reader    io.Reader
	close     func() error
	remaining int64
}

func (reader *maxBytesReadCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if reader.remaining <= 0 {
		var probe [1]byte
		n, err := reader.reader.Read(probe[:])
		if n > 0 {
			return 0, errHTTPBodyTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > reader.remaining {
		p = p[:int(reader.remaining)]
	}
	n, err := reader.reader.Read(p)
	reader.remaining -= int64(n)
	return n, err
}

func (reader *maxBytesReadCloser) Close() error {
	return reader.close()
}

func (conn *prefixedConn) Read(p []byte) (int, error) {
	if conn.reader.Len() > 0 {
		return conn.reader.Read(p)
	}
	return conn.Conn.Read(p)
}
