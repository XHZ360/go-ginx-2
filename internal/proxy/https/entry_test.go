package httpsproxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/simp-frp/go-ginx-2/internal/control"
	"github.com/simp-frp/go-ginx-2/internal/domain"
	"github.com/simp-frp/go-ginx-2/internal/session"
	"github.com/simp-frp/go-ginx-2/internal/store/sqlite"
)

func TestHTTPSEntryPassesThroughTLSBySNI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	originAddress, originPool := startTLSOrigin(t, ctx, "app.example.com")
	originHost, originPortText, err := net.SplitHostPort(originAddress)
	if err != nil {
		t.Fatal(err)
	}
	originPort, err := strconv.Atoi(originPortText)
	if err != nil {
		t.Fatal(err)
	}

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedHTTPSProxy(t, ctx, db, originHost, originPort)
	if err := db.Certificates().Create(ctx, domain.ManagedCertificate{ID: "cert-1", ProxyID: "proxy-1", Host: "app.example.com", Status: domain.CertificatePending, Provider: "cloudflare", CertFile: "pending.crt", KeyFile: "pending.key"}); err != nil {
		t.Fatalf("create pending managed certificate: %v", err)
	}

	sessions := session.NewManager()
	controlListener, err := control.ListenAddr("127.0.0.1:0", control.Server{
		Authenticator: control.Authenticator{Store: db, Now: func() time.Time { return time.Now().UTC() }},
		Sessions:      sessions,
		TLSConfig:     testServerTLSConfig(t),
		NewSessionID:  func() (string, error) { return "session-1", nil },
	})
	if err != nil {
		t.Fatalf("control listen: %v", err)
	}
	t.Cleanup(func() { _ = controlListener.Close() })
	go func() { _ = controlListener.Serve(ctx) }()

	client, response, err := control.DialAndAuthenticate(ctx, controlListener.Addr().String(), testClientTLSConfig(t), nil, control.AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: time.Now().UTC(), Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	if !response.Accepted {
		t.Fatalf("expected accepted auth response: %+v", response)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.ReadProxySnapshot(); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	go func() { _ = client.ServeProxyStreams(ctx) }()

	entry, err := Listen(Entry{Store: db, Sessions: sessions, ListenAddress: "127.0.0.1:0", NewConnection: func() (string, error) { return "conn-1", nil }})
	if err != nil {
		t.Fatalf("https listen: %v", err)
	}
	t.Cleanup(func() { _ = entry.Close() })
	go func() { _ = entry.Serve(ctx) }()

	dialer := &net.Dialer{Timeout: 2 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", entry.Addr().String(), &tls.Config{RootCAs: originPool, ServerName: "app.example.com", MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("dial proxied tls origin: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatalf("write tls payload: %v", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read tls payload: %v", err)
	}
	if line != "pong\n" {
		t.Fatalf("unexpected tls response %q", line)
	}
}

func TestHTTPSEntryTerminatesTLSBySelectedCertificate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	origin := startHTTPOrigin(t, "terminated response")
	originHost, originPortText, err := net.SplitHostPort(origin.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	originPort, err := strconv.Atoi(originPortText)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile, pool := writeCertificateFilesFor(t, "app.example.com")

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedHTTPSTerminationProxy(t, ctx, db, originHost, originPort, certFile, keyFile)

	sessions := session.NewManager()
	controlListener, err := control.ListenAddr("127.0.0.1:0", control.Server{
		Authenticator: control.Authenticator{Store: db, Now: func() time.Time { return time.Now().UTC() }},
		Sessions:      sessions,
		TLSConfig:     testServerTLSConfig(t),
		NewSessionID:  func() (string, error) { return "session-1", nil },
	})
	if err != nil {
		t.Fatalf("control listen: %v", err)
	}
	t.Cleanup(func() { _ = controlListener.Close() })
	go func() { _ = controlListener.Serve(ctx) }()

	client, response, err := control.DialAndAuthenticate(ctx, controlListener.Addr().String(), testClientTLSConfig(t), nil, control.AuthRequest{ClientID: "client-1", Credential: "secret", Timestamp: time.Now().UTC(), Protocols: []domain.Protocol{domain.ProtocolQUIC}})
	if err != nil {
		t.Fatalf("dial authenticate: %v", err)
	}
	if !response.Accepted {
		t.Fatalf("expected accepted auth response: %+v", response)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.ReadProxySnapshot(); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	go func() { _ = client.ServeProxyStreams(ctx) }()

	entry, err := Listen(Entry{Store: db, Sessions: sessions, ListenAddress: "127.0.0.1:0", NewConnection: func() (string, error) { return "conn-1", nil }})
	if err != nil {
		t.Fatalf("https listen: %v", err)
	}
	t.Cleanup(func() { _ = entry.Close() })
	go func() { _ = entry.Serve(ctx) }()

	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "app.example.com", MinVersion: tls.VersionTLS12}}
	httpClient := &http.Client{Transport: transport}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+entry.Addr().String()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "app.example.com"
	httpResponse, err := httpClient.Do(request)
	if err != nil {
		t.Fatalf("https request: %v", err)
	}
	defer httpResponse.Body.Close()
	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if httpResponse.StatusCode != http.StatusOK || string(body) != "terminated response" {
		t.Fatalf("unexpected response status=%d body=%q", httpResponse.StatusCode, string(body))
	}
}

func TestReadClientHelloSupportsFragmentedTLSRecords(t *testing.T) {
	clientHello := clientHelloRecord(t, "app.example.com")
	bodyLength := int(clientHello[3])<<8 | int(clientHello[4])
	body := clientHello[5 : 5+bodyLength]
	firstBody := body[:10]
	secondBody := body[10:]
	fragmented := append([]byte{tlsHandshakeRecord, 3, 1, byte(len(firstBody) >> 8), byte(len(firstBody))}, firstBody...)
	fragmented = append(fragmented, []byte{tlsHandshakeRecord, 3, 1, byte(len(secondBody) >> 8), byte(len(secondBody))}...)
	fragmented = append(fragmented, secondBody...)
	conn := &bufferConn{Reader: bytes.NewReader(fragmented)}

	prefix, serverName, err := readClientHello(conn)
	if err != nil {
		t.Fatalf("read fragmented client hello: %v", err)
	}
	if serverName != "app.example.com" {
		t.Fatalf("unexpected server name %q", serverName)
	}
	if string(prefix) != string(fragmented) {
		t.Fatal("expected original fragmented prefix to be preserved")
	}
}

func TestReadClientHelloSupportsSplitHandshakeHeader(t *testing.T) {
	clientHello := clientHelloRecord(t, "app.example.com")
	bodyLength := int(clientHello[3])<<8 | int(clientHello[4])
	body := clientHello[5 : 5+bodyLength]
	firstBody := body[:2]
	secondBody := body[2:]
	fragmented := append([]byte{tlsHandshakeRecord, 3, 1, byte(len(firstBody) >> 8), byte(len(firstBody))}, firstBody...)
	fragmented = append(fragmented, []byte{tlsHandshakeRecord, 3, 1, byte(len(secondBody) >> 8), byte(len(secondBody))}...)
	fragmented = append(fragmented, secondBody...)
	conn := &bufferConn{Reader: bytes.NewReader(fragmented)}

	prefix, serverName, err := readClientHello(conn)
	if err != nil {
		t.Fatalf("read split handshake header client hello: %v", err)
	}
	if serverName != "app.example.com" {
		t.Fatalf("unexpected server name %q", serverName)
	}
	if string(prefix) != string(fragmented) {
		t.Fatal("expected original fragmented prefix to be preserved")
	}
}

func TestMaxBytesReadCloserRejectsOverflow(t *testing.T) {
	body := &maxBytesReadCloser{reader: bytes.NewReader([]byte("abcd")), close: func() error { return nil }, remaining: 3}
	data, err := io.ReadAll(body)
	if !errors.Is(err, errHTTPBodyTooLarge) {
		t.Fatalf("expected body too large error, got data=%q err=%v", string(data), err)
	}
}

func TestReadResponseWithTimeoutClosesStalledStream(t *testing.T) {
	stream := newBlockingStream()
	request := &http.Request{Method: http.MethodGet}

	_, err := readResponseWithTimeout(stream, request, 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if !stream.isClosed() {
		t.Fatal("expected stalled stream to be closed")
	}
}

func TestWriteResponseWithTimeoutClosesStalledBodyStream(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })
	t.Cleanup(func() { _ = serverConn.Close() })
	go func() { _, _ = io.Copy(io.Discard, clientConn) }()
	stream := newBlockingStream()
	response := &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(stream), ContentLength: 1}

	err := writeResponseWithTimeout(serverConn, stream, response, 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if !stream.isClosed() {
		t.Fatal("expected stalled body stream to be closed")
	}
}

func TestCertificateFileRequiresCertificateDir(t *testing.T) {
	certFile, _, _ := writeCertificateFilesFor(t, "app.example.com")
	listener := &Listener{entry: Entry{CertificateDir: t.TempDir()}}
	if _, err := listener.certificateFile(certFile); err == nil {
		t.Fatal("expected certificate file outside certificate dir to be rejected")
	}
}

func TestCertificateFileRejectsSymlink(t *testing.T) {
	certFile, _, _ := writeCertificateFilesFor(t, "app.example.com")
	dir := t.TempDir()
	symlink := filepath.Join(dir, "server.crt")
	if err := os.Symlink(certFile, symlink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	listener := &Listener{entry: Entry{CertificateDir: dir}}
	if _, err := listener.certificateFile(symlink); err == nil {
		t.Fatal("expected symlink certificate file to be rejected")
	}
}

func TestManagedCertificateStorageWritesActiveAndRetainsPrevious(t *testing.T) {
	certificateDir := t.TempDir()
	firstCert, firstKey, _, err := generateTestCertificatePEMAt(t, "app.example.com", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	secondCert, secondKey, _, err := generateTestCertificatePEMAt(t, "app.example.com", time.Now().Add(-time.Hour), time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	storage := ManagedCertificateStorage{CertificateDir: certificateDir, Now: func() time.Time { return time.Now().UTC() }}

	first, err := storage.Store("App.Example.com", firstCert, firstKey)
	if err != nil {
		t.Fatalf("store first certificate: %v", err)
	}
	if filepath.Dir(first.CertFile) != filepath.Join(certificateDir, managedCertificateDir, "app.example.com") || filepath.Base(first.CertFile) != activeCertFile || filepath.Base(first.KeyFile) != activeKeyFile {
		t.Fatalf("unexpected active paths: %+v", first)
	}
	activeCert, err := os.ReadFile(first.CertFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(activeCert, firstCert) {
		t.Fatal("expected active certificate to contain first certificate")
	}

	second, err := storage.Store("app.example.com", secondCert, secondKey)
	if err != nil {
		t.Fatalf("store replacement certificate: %v", err)
	}
	previousCert, err := os.ReadFile(second.PreviousCertFile)
	if err != nil {
		t.Fatal(err)
	}
	activeCert, err = os.ReadFile(second.CertFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(previousCert, firstCert) || !bytes.Equal(activeCert, secondCert) {
		t.Fatal("expected replacement to retain previous and activate new certificate")
	}
}

func TestValidateCertificatePairRejectsWrongHostExpiredAndKeyMismatch(t *testing.T) {
	certPEM, keyPEM, _, err := generateTestCertificatePEMAt(t, "app.example.com", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCertificatePair("other.example.com", certPEM, keyPEM, time.Now().UTC()); err == nil {
		t.Fatal("expected wrong host to be rejected")
	}
	expiredCert, expiredKey, _, err := generateTestCertificatePEMAt(t, "app.example.com", time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCertificatePair("app.example.com", expiredCert, expiredKey, time.Now().UTC()); err == nil {
		t.Fatal("expected expired certificate to be rejected")
	}
	_, otherKey, _, err := generateTestCertificatePEMAt(t, "app.example.com", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCertificatePair("app.example.com", certPEM, otherKey, time.Now().UTC()); err == nil {
		t.Fatal("expected mismatched key to be rejected")
	}
}

func TestCertificateResolverSupportsStaticAndManagedCertificates(t *testing.T) {
	ctx := context.Background()
	certificateDir := t.TempDir()
	staticCertFile, staticKeyFile, _ := writeCertificateFilesInDir(t, certificateDir, "static.example.com")
	staticResolver := NewCertificateResolver(nil, certificateDir)
	staticCert, err := staticResolver.Certificate(ctx, "static.example.com", domain.Proxy{CertFile: staticCertFile, KeyFile: staticKeyFile})
	if err != nil {
		t.Fatalf("resolve static certificate: %v", err)
	}
	if staticCert == nil || staticCert.Leaf == nil || staticCert.Leaf.DNSNames[0] != "static.example.com" {
		t.Fatalf("unexpected static certificate: %+v", staticCert)
	}

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedHTTPSTerminationProxy(t, ctx, db, "127.0.0.1", 8080, "", "")
	managedCertPEM, managedKeyPEM, _, err := generateTestCertificatePEMAt(t, "managed.example.com", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := ManagedCertificateStorage{CertificateDir: certificateDir}.Store("managed.example.com", managedCertPEM, managedKeyPEM)
	if err != nil {
		t.Fatalf("store managed certificate: %v", err)
	}
	if err := db.Certificates().Create(ctx, domain.ManagedCertificate{ID: "managed-cert-1", ProxyID: "proxy-1", Host: "managed.example.com", Status: domain.CertificateValid, Provider: "cloudflare", CertFile: stored.CertFile, KeyFile: stored.KeyFile, NotAfter: &stored.NotAfter}); err != nil {
		t.Fatalf("create managed certificate metadata: %v", err)
	}
	managedResolver := NewCertificateResolver(db, certificateDir)
	managedCert, err := managedResolver.Certificate(ctx, "managed.example.com", domain.Proxy{ID: "proxy-1"})
	if err != nil {
		t.Fatalf("resolve managed certificate: %v", err)
	}
	if managedCert == nil || managedCert.Leaf == nil || managedCert.Leaf.DNSNames[0] != "managed.example.com" {
		t.Fatalf("unexpected managed certificate: %+v", managedCert)
	}
	managedResolver.Reload("managed.example.com")
	managedCert, err = managedResolver.Certificate(ctx, "managed.example.com", domain.Proxy{ID: "proxy-1"})
	if err != nil || managedCert == nil {
		t.Fatalf("resolve managed certificate after reload: cert=%+v err=%v", managedCert, err)
	}
}

func TestCertificateResolverIgnoresInactiveManagedCertificate(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedHTTPSTerminationProxy(t, ctx, db, "127.0.0.1", 8080, "", "")
	if err := db.Certificates().Create(ctx, domain.ManagedCertificate{ID: "cert-1", ProxyID: "proxy-1", Host: "app.example.com", Status: domain.CertificatePending, Provider: "cloudflare", CertFile: filepath.Join(t.TempDir(), "active.crt"), KeyFile: filepath.Join(t.TempDir(), "active.key")}); err != nil {
		t.Fatalf("create inactive certificate: %v", err)
	}
	certificate, err := NewCertificateResolver(db, t.TempDir()).Certificate(ctx, "app.example.com", domain.Proxy{})
	if err != nil {
		t.Fatalf("resolve inactive certificate: %v", err)
	}
	if certificate != nil {
		t.Fatal("expected inactive managed certificate to preserve passthrough")
	}
}

func seedHTTPSProxy(t *testing.T, ctx context.Context, db *sqlite.Store, targetHost string, targetPort int) {
	t.Helper()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	proxy := domain.Proxy{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: targetHost, TargetPort: targetPort}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
}

func seedHTTPSTerminationProxy(t *testing.T, ctx context.Context, db *sqlite.Store, targetHost string, targetPort int, certFile string, keyFile string) {
	t.Helper()
	user := domain.User{ID: "user-1", Username: "alice", Role: domain.RoleUser, Status: domain.UserEnabled}
	client := domain.Client{ID: "client-1", UserID: user.ID, Name: "home", Status: domain.ClientOffline, CredentialHash: domain.HashCredential("secret")}
	proxy := domain.Proxy{ID: "proxy-1", UserID: user.ID, ClientID: client.ID, Name: "secure", Type: domain.ProxyHTTPS, Status: domain.ProxyEnabled, EntryHost: "app.example.com", TargetHost: targetHost, TargetPort: targetPort, CertFile: certFile, KeyFile: keyFile}
	if err := db.Users().Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Clients().Create(ctx, client); err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Proxies().Create(ctx, proxy); err != nil {
		t.Fatalf("create proxy: %v", err)
	}
}

func startHTTPOrigin(t *testing.T, body string) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.Serve(listener) }()
	return listener
}

type bufferConn struct {
	*bytes.Reader
}

type blockingStream struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingStream() *blockingStream {
	return &blockingStream{closed: make(chan struct{})}
}

func (stream *blockingStream) Read([]byte) (int, error) {
	<-stream.closed
	return 0, io.EOF
}

func (stream *blockingStream) Write(p []byte) (int, error) { return len(p), nil }

func (stream *blockingStream) Close() error {
	stream.once.Do(func() { close(stream.closed) })
	return nil
}

func (stream *blockingStream) isClosed() bool {
	select {
	case <-stream.closed:
		return true
	default:
		return false
	}
}

func (conn *bufferConn) Write(p []byte) (int, error)      { return len(p), nil }
func (conn *bufferConn) Close() error                     { return nil }
func (conn *bufferConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (conn *bufferConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (conn *bufferConn) SetDeadline(time.Time) error      { return nil }
func (conn *bufferConn) SetReadDeadline(time.Time) error  { return nil }
func (conn *bufferConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr string

func (addr dummyAddr) Network() string { return string(addr) }
func (addr dummyAddr) String() string  { return string(addr) }

func clientHelloRecord(t *testing.T, serverName string) []byte {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	done := make(chan error, 1)
	go func() {
		tlsConn := tls.Client(clientConn, &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12})
		done <- tlsConn.Handshake()
	}()
	header := make([]byte, 5)
	if _, err := io.ReadFull(serverConn, header); err != nil {
		t.Fatal(err)
	}
	body := make([]byte, int(header[3])<<8|int(header[4]))
	if _, err := io.ReadFull(serverConn, body); err != nil {
		t.Fatal(err)
	}
	return append(header, body...)
}

func startTLSOrigin(t *testing.T, ctx context.Context, serverName string) (string, *x509.CertPool) {
	t.Helper()
	cert, pool := testCertificateFor(t, serverName)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				line, err := bufio.NewReader(conn).ReadString('\n')
				if err != nil || line != "ping\n" {
					return
				}
				_, _ = conn.Write([]byte("pong\n"))
			}(conn)
		}
	}()
	return listener.Addr().String(), pool
}

func testServerTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	cert, _ := testCertificateFor(t, "localhost")
	return &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{control.ControlALPN}, MinVersion: tls.VersionTLS13}
}

func testClientTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	_, pool := testCertificateFor(t, "localhost")
	return &tls.Config{RootCAs: pool, ServerName: "localhost", NextProtos: []string{control.ControlALPN}, MinVersion: tls.VersionTLS13}
}

func testCertificateFor(t *testing.T, serverName string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	certCacheMu.Lock()
	defer certCacheMu.Unlock()
	if cached, ok := certCache[serverName]; ok {
		return cached.cert, cached.pool.Clone()
	}
	cert, pool, err := generateTestCertificate(serverName)
	if err != nil {
		t.Fatal(err)
	}
	certCache[serverName] = cachedCertificate{cert: cert, pool: pool}
	return cert, pool.Clone()
}

func writeCertificateFilesFor(t *testing.T, serverName string) (string, string, *x509.CertPool) {
	t.Helper()
	certPEM, keyPEM, caPEM, err := generateTestCertificatePEM(serverName)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	return writeCertificateFiles(t, dir, certPEM, keyPEM, caPEM)
}

func writeCertificateFilesInDir(t *testing.T, dir string, serverName string) (string, string, *x509.CertPool) {
	t.Helper()
	certPEM, keyPEM, caPEM, err := generateTestCertificatePEM(serverName)
	if err != nil {
		t.Fatal(err)
	}
	return writeCertificateFiles(t, dir, certPEM, keyPEM, caPEM)
}

func writeCertificateFiles(t *testing.T, dir string, certPEM []byte, keyPEM []byte, caPEM []byte) (string, string, *x509.CertPool) {
	t.Helper()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("append CA cert")
	}
	return certFile, keyFile, pool
}

type cachedCertificate struct {
	cert tls.Certificate
	pool *x509.CertPool
}

var (
	certCacheMu sync.Mutex
	certCache   = make(map[string]cachedCertificate)
)

func generateTestCertificate(serverName string) (tls.Certificate, *x509.CertPool, error) {
	certPEM, keyPEM, caPEM, err := generateTestCertificatePEM(serverName)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return tls.Certificate{}, nil, errors.New("append CA cert")
	}
	return cert, pool, nil
}

func generateTestCertificatePEM(serverName string) ([]byte, []byte, []byte, error) {
	return generateTestCertificatePEMWithValidity(serverName, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
}

func generateTestCertificatePEMAt(t *testing.T, serverName string, notBefore time.Time, notAfter time.Time) ([]byte, []byte, []byte, error) {
	t.Helper()
	return generateTestCertificatePEMWithValidity(serverName, notBefore, notAfter)
}

func generateTestCertificatePEMWithValidity(serverName string, notBefore time.Time, notAfter time.Time) ([]byte, []byte, []byte, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, err
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "go-ginx-test-ca"}, NotBefore: time.Now().Add(-24 * time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, BasicConstraintsValid: true, IsCA: true}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, err
	}
	serverTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: serverName}, DNSNames: []string{serverName}, NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	return certPEM, keyPEM, caPEM, nil
}
