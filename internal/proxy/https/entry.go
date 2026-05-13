package httpsproxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
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
	Store          store.Store
	Sessions       *session.Manager
	ListenAddress  string
	CertificateDir string
	NewConnection  func() (string, error)
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
	proxy, err := listener.entry.Store.Proxies().ByHTTPSHost(ctx, serverName)
	if err != nil || proxy.Status != domain.ProxyEnabled {
		return
	}
	certificate, err := listener.resolver.Certificate(ctx, serverName, proxy)
	if err != nil {
		return
	}
	if certificate != nil {
		listener.handleTerminatedConn(ctx, conn, prefix, proxy, *certificate)
		return
	}
	_ = conn.SetDeadline(time.Time{})
	latest, ok := listener.entry.Sessions.Latest(proxy.ClientID)
	if !ok || latest.StreamOpener == nil {
		return
	}
	stream, err := latest.StreamOpener.OpenStream(ctx)
	if err != nil {
		return
	}
	defer stream.Close()
	connectionID, err := listener.connectionID()
	if err != nil {
		return
	}
	if err := control.WriteMessage(stream, control.MessageOpenStream, control.OpenStream{Kind: "tcp", ProxyID: proxy.ID, ConnectionID: connectionID, TargetHost: proxy.TargetHost, TargetPort: proxy.TargetPort}); err != nil {
		return
	}
	copyBidirectional(&prefixedConn{Conn: conn, reader: bytes.NewReader(prefix)}, stream)
}

func (listener *Listener) handleTerminatedConn(ctx context.Context, conn net.Conn, prefix []byte, proxy domain.Proxy, certificate tls.Certificate) {
	tlsConn := tls.Server(&prefixedConn{Conn: conn, reader: bytes.NewReader(prefix)}, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
	_ = tlsConn.SetDeadline(time.Now().Add(httpsReadTimeout))
	if err := tlsConn.Handshake(); err != nil {
		return
	}
	request, err := http.ReadRequest(bufio.NewReader(tlsConn))
	if err != nil {
		return
	}
	defer request.Body.Close()
	if request.ContentLength > maxHTTPBodyBytes {
		_ = writeSimpleResponse(tlsConn, http.StatusRequestEntityTooLarge, "request body too large\n")
		return
	}
	request.Body = &maxBytesReadCloser{reader: request.Body, close: request.Body.Close, remaining: maxHTTPBodyBytes}

	latest, ok := listener.entry.Sessions.Latest(proxy.ClientID)
	if !ok || latest.StreamOpener == nil {
		_ = writeSimpleResponse(tlsConn, http.StatusServiceUnavailable, "client offline\n")
		return
	}
	stream, err := latest.StreamOpener.OpenStream(ctx)
	if err != nil {
		_ = writeSimpleResponse(tlsConn, http.StatusBadGateway, "open proxy stream failed\n")
		return
	}
	defer stream.Close()
	connectionID, err := listener.connectionID()
	if err != nil {
		_ = writeSimpleResponse(tlsConn, http.StatusInternalServerError, "request id failed\n")
		return
	}
	if err := control.WriteMessage(stream, control.MessageOpenStream, control.OpenStream{Kind: "http", ProxyID: proxy.ID, ConnectionID: connectionID, TargetHost: proxy.TargetHost, TargetPort: proxy.TargetPort}); err != nil {
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
	response, err := readResponseWithTimeout(stream, request, httpsReadTimeout)
	if err != nil {
		_ = writeSimpleResponse(tlsConn, http.StatusBadGateway, "read proxy response failed\n")
		return
	}
	defer response.Body.Close()
	_ = writeResponseWithTimeout(tlsConn, stream, response, httpsReadTimeout)
}

type responseResult struct {
	response *http.Response
	err      error
}

func readResponseWithTimeout(stream io.ReadWriteCloser, request *http.Request, timeout time.Duration) (*http.Response, error) {
	result := make(chan responseResult, 1)
	go func() {
		response, err := http.ReadResponse(bufio.NewReader(stream), request)
		result <- responseResult{response: response, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case response := <-result:
		return response.response, response.err
	case <-timer.C:
		_ = stream.Close()
		return nil, context.DeadlineExceeded
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
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("certificate file symlinks are not allowed")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(listener.entry.CertificateDir) == "" {
		return resolved, nil
	}
	certificateDir, err := filepath.EvalSymlinks(listener.entry.CertificateDir)
	if err != nil {
		return "", err
	}
	certificateDir, err = filepath.Abs(certificateDir)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(certificateDir, resolved)
	if err != nil {
		return "", err
	}
	if relative == "." || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return "", fmt.Errorf("certificate file must be under %s", certificateDir)
	}
	return resolved, nil
}

func writeSimpleResponse(writer io.Writer, statusCode int, body string) error {
	response := &http.Response{StatusCode: statusCode, Status: http.StatusText(statusCode), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	response.Header.Set("Content-Type", "text/plain; charset=utf-8")
	response.ContentLength = int64(len(body))
	return response.Write(writer)
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
	<-done
}
